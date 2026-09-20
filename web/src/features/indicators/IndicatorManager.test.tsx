import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { IndicatorManager } from './IndicatorManager'

describe('IndicatorManager', () => {
  it('区分叠加指标与副图指标并允许添加和移除', () => {
    const onChange = vi.fn()
    render(<IndicatorManager indicators={[{ kind: 'SMA', period: 5 }]} onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: /指标/ }))
    expect(screen.getByText('叠加指标')).toBeVisible()
    expect(screen.getByText('副图指标')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '添加 EMA 10' }))
    expect(onChange).toHaveBeenLastCalledWith([
      { kind: 'SMA', period: 5 },
      { kind: 'EMA', period: 10 },
    ])

    fireEvent.click(screen.getByRole('button', { name: '添加 MACD 12, 26, 9' }))
    expect(onChange).toHaveBeenLastCalledWith([
      { kind: 'SMA', period: 5 },
      { kind: 'MACD', fast: 12, slow: 26, signal: 9 },
    ])

    fireEvent.click(screen.getByRole('button', { name: '移除 SMA 5' }))
    expect(onChange).toHaveBeenLastCalledWith([])
  })
})

it('adds standard deviation preset and edits an existing period', () => {
  const change = vi.fn()
  render(<IndicatorManager indicators={[{ kind: 'SMA', period: 5 }]} onChange={change} />)
  fireEvent.click(screen.getByRole('button', { name: /指标/ }))
  fireEvent.click(screen.getByRole('button', { name: '添加 STD 20' }))
  expect(change).toHaveBeenCalledWith([
    { kind: 'SMA', period: 5 },
    { kind: 'STD', period: 20 },
  ])
  fireEvent.change(screen.getByRole('spinbutton', { name: 'SMA 5 period' }), { target: { value: '10' } })
  fireEvent.click(screen.getByRole('button', { name: '应用 SMA 5 参数' }))
  expect(change).toHaveBeenLastCalledWith([{ kind: 'SMA', period: 10 }])
})

it('rejects duplicates and invalid MACD order, supports reorder/removal', () => {
  const onChange = vi.fn()
  render(
    <IndicatorManager
      indicators={[
        { kind: 'SMA', period: 5 },
        { kind: 'SMA', period: 20 },
        { kind: 'MACD', fast: 12, slow: 26, signal: 9 },
      ]}
      onChange={onChange}
    />,
  )
  fireEvent.click(screen.getByRole('button', { name: /指标/ }))
  fireEvent.change(screen.getByRole('spinbutton', { name: 'SMA 5 period' }), { target: { value: '20' } })
  fireEvent.click(screen.getByRole('button', { name: '应用 SMA 5 参数' }))
  expect(screen.getByRole('alert')).toHaveTextContent('重复')
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.change(screen.getByRole('spinbutton', { name: 'MACD 12, 26, 9 fast' }), {
    target: { value: '30' },
  })
  fireEvent.click(screen.getByRole('button', { name: '应用 MACD 12, 26, 9 参数' }))
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: '上移 SMA 5' }))
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: '下移 SMA 5' }))
  expect(onChange.mock.calls.at(-1)![0][0]).toEqual({ kind: 'SMA', period: 20 })
  fireEvent.click(screen.getByRole('button', { name: '删除 SMA 20' }))
  expect(onChange.mock.calls.at(-1)![0]).toHaveLength(2)
  fireEvent.click(screen.getByRole('button', { name: '关闭指标管理' }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
