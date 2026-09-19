import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { StrategyDefinition } from '../../api/client'
import { StrategyForm, type StrategyFormValue } from './StrategyForm'

const definition: StrategyDefinition = {
  strategy: 'daily_b1_buy',
  version: '1',
  primary_timeframe: 'daily',
  warmup_bars: 60,
  default_hold_bars: 10,
  parameters: { lookback_days: { default: 20, min: 5, max: 120, integer: true } },
  features: ['close'],
  auxiliary: [],
}

const baseValue: StrategyFormValue = { strategy: 'daily_b1_buy', version: '1', parameters: {} }

function setup(onChange = vi.fn()) {
  render(<StrategyForm definitions={[definition]} value={baseValue} onChange={onChange} />)
  return onChange
}

describe('StrategyForm', () => {
  it('渲染策略目录与参数定义', () => {
    setup()
    expect(screen.getByRole('combobox')).toBeVisible()
    expect(screen.getByText(/主周期 日线/)).toBeVisible()
    expect(screen.getByText(/预热 60 根/)).toBeVisible()
    expect(screen.getByText(/默认持有 10 根/)).toBeVisible()
    expect(screen.getByLabelText('参数 lookback_days')).toHaveAttribute('placeholder', '默认 20')
  })

  it('目录加载失败时显示错误', () => {
    render(<StrategyForm definitions={[]} error="策略目录加载失败" value={baseValue} onChange={vi.fn()} />)
    expect(screen.getByText('策略目录加载失败')).toBeVisible()
  })

  it('参数超出 min/max 时上报无效并提示范围', () => {
    const onChange = setup()
    const input = screen.getByLabelText('参数 lookback_days')
    fireEvent.change(input, { target: { value: '3' } })
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ parameters: {} }), false)
    expect(screen.getByText('范围 5–120 的整数')).toBeVisible()

    fireEvent.change(input, { target: { value: '999' } })
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ parameters: {} }), false)

    fireEvent.change(input, { target: { value: '30' } })
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ parameters: { lookback_days: 30 } }),
      true,
    )
  })

  it('整数参数自动取整', () => {
    const onChange = setup()
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '30.6' } })
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ parameters: { lookback_days: 31 } }),
      true,
    )
  })

  it('已填参数清空后不进入提交值', () => {
    const onChange = setup()
    const input = screen.getByLabelText('参数 lookback_days')
    fireEvent.change(input, { target: { value: '30' } })
    fireEvent.change(input, { target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ parameters: {} }), true)
  })

  it('切换策略时清空已填参数', () => {
    const onChange = vi.fn()
    const second: StrategyDefinition = { ...definition, strategy: 'weekly_b1_buy', version: '2' }
    const { rerender } = render(
      <StrategyForm definitions={[definition, second]} value={baseValue} onChange={onChange} />,
    )
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '30' } })
    rerender(
      <StrategyForm
        definitions={[definition, second]}
        value={{ strategy: 'weekly_b1_buy', version: '2', parameters: {} }}
        onChange={onChange}
      />,
    )
    expect(screen.getByLabelText('参数 lookback_days')).toHaveValue(null)
  })
})
