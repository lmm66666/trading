import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FinancialChart, computeBarSpacingLimits } from './FinancialChart'

const mocks = vi.hoisted(() => {
  let visibleRangeHandler: ((range: { from: number; to: number } | null) => void) | null = null
  const setData = vi.fn()
  const applyOptions = vi.fn()
  const series = { setData, priceScale: () => ({ applyOptions }) }
  const setStretchFactor = vi.fn()
  const timeScale = {
    applyOptions: vi.fn(),
    fitContent: vi.fn(() => visibleRangeHandler?.({ from: 0, to: 100 })),
    getVisibleLogicalRange: vi.fn(() => ({ from: 10, to: 30 })),
    setVisibleLogicalRange: vi.fn(),
    subscribeVisibleLogicalRangeChange: vi.fn((handler: (range: { from: number; to: number } | null) => void) => { visibleRangeHandler = handler }),
    unsubscribeVisibleLogicalRangeChange: vi.fn(),
  }
  const chart = {
    addSeries: vi.fn(() => series),
    panes: () => [{ setStretchFactor }, { setStretchFactor }, { setStretchFactor }],
    remove: vi.fn(),
    timeScale: () => timeScale,
  }
  const createChart = vi.fn(() => chart)
  return { chart, createChart, setData, setStretchFactor, timeScale }
})

vi.mock('lightweight-charts', () => ({
  CandlestickSeries: 'candlestick',
  HistogramSeries: 'histogram',
  LineSeries: 'line',
  ColorType: { Solid: 'solid' },
  createChart: mocks.createChart,
}))

describe('computeBarSpacingLimits', () => {
  it('桌面宽度按 15–400 根换算 barSpacing 边界', () => {
    expect(computeBarSpacingLimits(1200)).toEqual({ minBarSpacing: 3, maxBarSpacing: 80 })
    expect(computeBarSpacingLimits(1920)).toEqual({ minBarSpacing: 4.8, maxBarSpacing: 128 })
  })

  it('移动宽度按 10–160 根换算 barSpacing 边界', () => {
    expect(computeBarSpacingLimits(760)).toEqual({ minBarSpacing: 4.75, maxBarSpacing: 76 })
    expect(computeBarSpacingLimits(320)).toEqual({ minBarSpacing: 3, maxBarSpacing: 32 })
  })
})

describe('FinancialChart', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.unstubAllGlobals())

  it('绘制主图、成交量、叠加指标与副图，并在靠近左边界时翻页', () => {
    const onLoadMore = vi.fn()
    const bars = [{
      open_time: '2026-09-11T01:00:00Z', close_time: '2026-09-11T07:00:00Z',
      open: 33, high: 34, low: 32, close: 33.5, volume: 100, amount: 3300, trading_status: 0,
    }]
    const series = [
      { key: 'sma/day/qfq/close/p=5', kind: 'SMA' as const, component: 'value', points: [{ time: bars[0].close_time, value: 33.2 }] },
      { key: 'macd-h', kind: 'MACD' as const, component: 'histogram', points: [{ time: bars[0].close_time, value: -0.2 }] },
      { key: 'macd-d', kind: 'MACD' as const, component: 'dif', points: [{ time: bars[0].close_time, value: 0.1 }] },
      { key: 'kdj-k', kind: 'KDJ' as const, component: 'k', points: [{ time: bars[0].close_time, value: 50 }] },
    ]
    const { unmount } = render(<FinancialChart bars={bars} series={series} onLoadMore={onLoadMore} />)

    expect(mocks.chart.addSeries).toHaveBeenCalledTimes(6)
    expect(mocks.chart.addSeries).toHaveBeenCalledWith('line', expect.objectContaining({ title: 'SMA 5' }), 0)
    expect(mocks.timeScale.fitContent).toHaveBeenCalled()
    expect(onLoadMore).not.toHaveBeenCalled()
    const rangeHandler = mocks.timeScale.subscribeVisibleLogicalRangeChange.mock.calls[0][0]
    rangeHandler({ from: 3, to: 20 })
    rangeHandler(null)
    expect(onLoadMore).toHaveBeenCalledTimes(1)

    unmount()
    expect(mocks.timeScale.unsubscribeVisibleLogicalRangeChange).toHaveBeenCalledWith(rangeHandler)
    expect(mocks.chart.remove).toHaveBeenCalled()
  })

  it('前插历史数据时复用图表并保持当前逻辑视口', () => {
    const bar = {
      open_time: '2026-09-11T01:00:00Z', close_time: '2026-09-11T07:00:00Z',
      open: 33, high: 34, low: 32, close: 33.5, volume: 100, amount: 3300, trading_status: 0,
    }
    const { rerender } = render(<FinancialChart bars={[bar]} series={[]} onLoadMore={vi.fn()} />)
    rerender(<FinancialChart bars={[{ ...bar, close_time: '2026-09-10T07:00:00Z' }, bar]} series={[]} onLoadMore={vi.fn()} />)

    expect(mocks.createChart).toHaveBeenCalledTimes(1)
    expect(mocks.timeScale.setVisibleLogicalRange).toHaveBeenCalledWith({ from: 11, to: 31 })
  })

  it('容器宽度变化时重算并应用 barSpacing 边界', () => {
    const callbacks: ResizeObserverCallback[] = []
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: ResizeObserverCallback) { callbacks.push(callback) }
      observe() {}
      unobserve() {}
      disconnect() {}
    })
    const bar = {
      open_time: '2026-09-11T01:00:00Z', close_time: '2026-09-11T07:00:00Z',
      open: 33, high: 34, low: 32, close: 33.5, volume: 100, amount: 3300, trading_status: 0,
    }
    const { unmount } = render(<FinancialChart bars={[bar]} series={[]} onLoadMore={vi.fn()} />)
    expect(callbacks.length).toBe(1)
    expect(mocks.timeScale.applyOptions).not.toHaveBeenCalled()

    const container = document.querySelector('.financial-chart') as HTMLDivElement
    Object.defineProperty(container, 'clientWidth', { value: 1200, configurable: true })
    act(() => callbacks[0]([], {} as ResizeObserver))
    expect(mocks.timeScale.applyOptions).toHaveBeenCalledWith({ minBarSpacing: 3, maxBarSpacing: 80 })

    Object.defineProperty(container, 'clientWidth', { value: 320, configurable: true })
    act(() => callbacks[0]([], {} as ResizeObserver))
    expect(mocks.timeScale.applyOptions).toHaveBeenLastCalledWith({ minBarSpacing: 3, maxBarSpacing: 32 })

    unmount()
  })
})
