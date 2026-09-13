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
