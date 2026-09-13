import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FinancialChart } from './FinancialChart'

const mocks = vi.hoisted(() => {
  const setData = vi.fn()
  const applyOptions = vi.fn()
  const series = { setData, priceScale: () => ({ applyOptions }) }
  const setStretchFactor = vi.fn()
  const timeScale = {
    fitContent: vi.fn(),
    subscribeVisibleLogicalRangeChange: vi.fn(),
    unsubscribeVisibleLogicalRangeChange: vi.fn(),
  }
  const chart = {
    addSeries: vi.fn(() => series),
    panes: () => [{ setStretchFactor }, { setStretchFactor }, { setStretchFactor }],
    remove: vi.fn(),
    timeScale: () => timeScale,
  }
  return { chart, setData, setStretchFactor, timeScale }
})

vi.mock('lightweight-charts', () => ({
  CandlestickSeries: 'candlestick',
  HistogramSeries: 'histogram',
  LineSeries: 'line',
  ColorType: { Solid: 'solid' },
  createChart: vi.fn(() => mocks.chart),
}))

describe('FinancialChart', () => {
  it('绘制主图、成交量、叠加指标与副图，并在靠近左边界时翻页', () => {
    const onLoadMore = vi.fn()
    const bars = [{
      open_time: '2026-09-11T01:00:00Z', close_time: '2026-09-11T07:00:00Z',
      open: 33, high: 34, low: 32, close: 33.5, volume: 100, amount: 3300, trading_status: 0,
    }]
    const series = [
      { key: 'sma?period=5', kind: 'SMA' as const, component: 'value', points: [{ time: bars[0].close_time, value: 33.2 }] },
      { key: 'macd-h', kind: 'MACD' as const, component: 'histogram', points: [{ time: bars[0].close_time, value: -0.2 }] },
      { key: 'macd-d', kind: 'MACD' as const, component: 'dif', points: [{ time: bars[0].close_time, value: 0.1 }] },
      { key: 'kdj-k', kind: 'KDJ' as const, component: 'k', points: [{ time: bars[0].close_time, value: 50 }] },
    ]
    const { unmount } = render(<FinancialChart bars={bars} series={series} onLoadMore={onLoadMore} />)

    expect(mocks.chart.addSeries).toHaveBeenCalledTimes(6)
    expect(mocks.timeScale.fitContent).toHaveBeenCalled()
    const rangeHandler = mocks.timeScale.subscribeVisibleLogicalRangeChange.mock.calls[0][0]
    rangeHandler({ from: 3, to: 20 })
    rangeHandler(null)
    expect(onLoadMore).toHaveBeenCalledTimes(1)

    unmount()
    expect(mocks.timeScale.unsubscribeVisibleLogicalRangeChange).toHaveBeenCalledWith(rangeHandler)
    expect(mocks.chart.remove).toHaveBeenCalled()
  })
})
