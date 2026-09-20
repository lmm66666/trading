import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FinancialChart, computeBarSpacingLimits } from './FinancialChart'

const mocks = vi.hoisted(() => {
  let visibleRangeHandler: ((range: { from: number; to: number } | null) => void) | null = null
  const setData = vi.fn()
  const applyOptions = vi.fn()
  const series = { setData, priceScale: () => ({ applyOptions }) }
  const setStretchFactor = vi.fn()
  const getStretchFactor = () => 1
  const timeScale = {
    applyOptions: vi.fn(),
    fitContent: vi.fn(() => visibleRangeHandler?.({ from: 0, to: 100 })),
    getVisibleLogicalRange: vi.fn(() => ({ from: 10, to: 30 })),
    setVisibleLogicalRange: vi.fn(),
    subscribeVisibleLogicalRangeChange: vi.fn(
      (handler: (range: { from: number; to: number } | null) => void) => {
        visibleRangeHandler = handler
      },
    ),
    unsubscribeVisibleLogicalRangeChange: vi.fn(),
  }
  const chart = {
    subscribeCrosshairMove: vi.fn(),
    unsubscribeCrosshairMove: vi.fn(),
    addSeries: vi.fn(() => series),
    panes: () => [
      { setStretchFactor, getStretchFactor, getHeight: () => 100, getHTMLElement: () => null },
      { setStretchFactor, getStretchFactor, getHeight: () => 100, getHTMLElement: () => null },
      { setStretchFactor, getStretchFactor, getHeight: () => 100, getHTMLElement: () => null },
    ],
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
    const bars = [
      {
        open_time: '2026-09-11T01:00:00Z',
        close_time: '2026-09-11T07:00:00Z',
        open: 33,
        high: 34,
        low: 32,
        close: 33.5,
        volume: 100,
        amount: 3300,
        trading_status: 0,
      },
    ]
    const series = [
      {
        key: 'sma/day/qfq/close/p=5',
        kind: 'SMA' as const,
        component: 'value',
        points: [{ time: bars[0].close_time, value: 33.2 }],
      },
      {
        key: 'macd-h',
        kind: 'MACD' as const,
        component: 'histogram',
        points: [{ time: bars[0].close_time, value: -0.2 }],
      },
      {
        key: 'macd-d',
        kind: 'MACD' as const,
        component: 'dif',
        points: [{ time: bars[0].close_time, value: 0.1 }],
      },
      {
        key: 'kdj-k',
        kind: 'KDJ' as const,
        component: 'k',
        points: [{ time: bars[0].close_time, value: 50 }],
      },
    ]
    const { unmount } = render(<FinancialChart bars={bars} series={series} onLoadMore={onLoadMore} />)

    expect(mocks.chart.addSeries).toHaveBeenCalledTimes(6)
    // K 线红涨绿跌，右轴值标签统一关闭，指标值由图例展示
    expect(mocks.chart.addSeries).toHaveBeenCalledWith(
      'candlestick',
      expect.objectContaining({ upColor: '#ef5350', downColor: '#26a69a' }),
    )
    expect(mocks.chart.addSeries).toHaveBeenCalledWith(
      'histogram',
      expect.objectContaining({ lastValueVisible: false }),
      1,
    )
    expect(mocks.chart.addSeries).toHaveBeenCalledWith(
      'line',
      expect.objectContaining({ lastValueVisible: false }),
      0,
    )
    expect(mocks.chart.addSeries).toHaveBeenCalledWith(
      'line',
      expect.objectContaining({ lastValueVisible: false }),
      2,
    )
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

  it('关闭 TV 水印', () => {
    const bar = {
      open_time: '2026-09-11T01:00:00Z',
      close_time: '2026-09-11T07:00:00Z',
      open: 33,
      high: 34,
      low: 32,
      close: 33.5,
      volume: 100,
      amount: 3300,
      trading_status: 0,
    }
    render(<FinancialChart bars={[bar]} series={[]} onLoadMore={vi.fn()} />)
    const options = (mocks.createChart.mock.calls[0] as unknown[])[1] as {
      layout: { attributionLogo?: boolean }
    }
    expect(options.layout.attributionLogo).toBe(false)
  })

  it('主图图例展示日期与 OHLC，副图图例定位到对应 pane 并随十字光标更新', () => {
    const bars = [
      {
        open_time: '2026-09-10T01:00:00Z',
        close_time: '2026-09-10T07:00:00Z',
        open: 30,
        high: 31,
        low: 29,
        close: 30.5,
        volume: 100,
        amount: 3000,
        trading_status: 0,
      },
      {
        open_time: '2026-09-11T01:00:00Z',
        close_time: '2026-09-11T07:00:00Z',
        open: 33.5,
        high: 35,
        low: 33,
        close: 34.5,
        volume: 200,
        amount: 6900,
        trading_status: 0,
      },
    ]
    const series = [
      {
        key: 'sma/day/qfq/close/p=5',
        kind: 'SMA' as const,
        component: 'value',
        points: [{ time: '2026-09-11T07:00:00Z', value: 33.2 }],
      },
      {
        key: 'macd/day/qfq/close/f=12/s=26/sig=9',
        kind: 'MACD' as const,
        component: 'histogram',
        points: [{ time: '2026-09-11T07:00:00Z', value: -0.2 }],
      },
      {
        key: 'macd/day/qfq/close/f=12/s=26/sig=9',
        kind: 'MACD' as const,
        component: 'dif',
        points: [{ time: '2026-09-11T07:00:00Z', value: 0.1 }],
      },
    ]
    const { container } = render(<FinancialChart bars={bars} series={series} onLoadMore={vi.fn()} />)

    const legends = container.querySelectorAll('.chart-legend')
    expect(legends.length).toBe(2) // 主图图例 + MACD 副图图例
    const mainLegend = legends[0] as HTMLElement
    // 日期独占一行，光标未悬停时显示最新一根
    expect(mainLegend.querySelector('.chart-legend-date')!.textContent).toContain('2026-09-11')
    // OHLC 原始值 + 涨跌 + 量
    expect(mainLegend.textContent).toContain('34.50')
    expect(mainLegend.textContent).toContain('+4.00 (+13.11%)')
    expect(mainLegend.textContent).toContain('200')
    // MACD 副图图例定位到 pane 2 左上角：mock pane 高 100 → top 206
    const paneLegend = legends[1] as HTMLElement
    expect(paneLegend.style.top).toBe('206px')
    expect(paneLegend.textContent).toContain('DIF')

    // 十字光标移到 09-10：图例切到该 K 线的值
    const hover = mocks.chart.subscribeCrosshairMove.mock.calls[0][0] as (p: { time?: string }) => void
    act(() => hover({ time: '2026-09-10' }))
    expect(mainLegend.querySelector('.chart-legend-date')!.textContent).toContain('2026-09-10')
    expect(mainLegend.textContent).toContain('30.50')
    // 光标离开：回退到最新一根
    act(() => hover({}))
    expect(mainLegend.querySelector('.chart-legend-date')!.textContent).toContain('2026-09-11')
  })

  it('前插历史数据时复用图表并保持当前逻辑视口', () => {
    const bar = {
      open_time: '2026-09-11T01:00:00Z',
      close_time: '2026-09-11T07:00:00Z',
      open: 33,
      high: 34,
      low: 32,
      close: 33.5,
      volume: 100,
      amount: 3300,
      trading_status: 0,
    }
    const { rerender } = render(<FinancialChart bars={[bar]} series={[]} onLoadMore={vi.fn()} />)
    rerender(
      <FinancialChart
        bars={[{ ...bar, close_time: '2026-09-10T07:00:00Z' }, bar]}
        series={[]}
        onLoadMore={vi.fn()}
      />,
    )

    expect(mocks.createChart).toHaveBeenCalledTimes(1)
    expect(mocks.timeScale.setVisibleLogicalRange).toHaveBeenCalledWith({ from: 11, to: 31 })
  })

  it('容器宽度变化时重算并应用 barSpacing 边界', () => {
    const callbacks: ResizeObserverCallback[] = []
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: ResizeObserverCallback) {
          callbacks.push(callback)
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    )
    const bar = {
      open_time: '2026-09-11T01:00:00Z',
      close_time: '2026-09-11T07:00:00Z',
      open: 33,
      high: 34,
      low: 32,
      close: 33.5,
      volume: 100,
      amount: 3300,
      trading_status: 0,
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

it('uses percentage OHLC, pale yellow MA5 and a single normalized comparison', () => {
  const bar = {
    open_time: '2026-09-01',
    close_time: '2026-09-01',
    open: 9,
    high: 11,
    low: 8,
    close: 10,
    volume: 100,
    amount: 1000,
    trading_status: 0,
  }
  render(
    <FinancialChart
      bars={[bar, { ...bar, close_time: '2026-09-02', close: 12 }]}
      comparison={[
        { ...bar, close: 100 },
        { ...bar, close_time: '2026-09-02', close: 110 },
      ]}
      comparisonLabel="原油"
      series={[
        {
          key: 'sma/day/raw/close/p=5',
          kind: 'SMA',
          component: 'value',
          points: [{ time: '2026-09-02', value: 11 }],
        },
      ]}
      onLoadMore={vi.fn()}
    />,
  )
  expect(mocks.setData).toHaveBeenCalledWith(
    expect.arrayContaining([
      expect.objectContaining({ time: '2026-09-01', close: 0, high: expect.closeTo(10) }),
    ]),
  )
  expect(mocks.chart.addSeries).toHaveBeenCalledWith(
    'line',
    expect.objectContaining({ color: '#FDE68A', lineWidth: 1 }),
    0,
  )
  expect(mocks.setData).toHaveBeenCalledWith([
    { time: '2026-09-01', value: 0 },
    { time: '2026-09-02', value: expect.closeTo(10) },
  ])
})

it('keeps the baseline during prepends and normalizes MA even without overlapping comparison dates', () => {
  const bar = {
    open_time: '2026-09-02',
    close_time: '2026-09-02',
    open: 10,
    high: 12,
    low: 9,
    close: 10,
    volume: 100,
    amount: 1000,
    trading_status: 0,
  }
  const series = [
    {
      key: 'sma/day/raw/close/p=5',
      kind: 'SMA' as const,
      component: 'value',
      points: [{ time: '2026-09-02', value: 11 }],
    },
  ]
  const { rerender } = render(
    <FinancialChart
      bars={[bar]}
      series={series}
      onLoadMore={vi.fn()}
      comparison={[{ ...bar, close_time: '2020-01-01' }]}
      comparisonLabel="原油"
    />,
  )
  expect(mocks.setData).toHaveBeenCalledWith([{ time: '2026-09-02', value: expect.closeTo(10) }])
  rerender(
    <FinancialChart
      bars={[{ ...bar, close_time: '2026-09-01', close: 5 }, bar]}
      series={series}
      onLoadMore={vi.fn()}
    />,
  )
  expect(mocks.setData).toHaveBeenCalledWith(
    expect.arrayContaining([expect.objectContaining({ time: '2026-09-02', close: 0 })]),
  )
})

it('captures actual layout for manual save and only marks real interactions dirty', () => {
  vi.useFakeTimers()
  const bar = {
    open_time: '2026-09-01',
    close_time: '2026-09-01',
    open: 10,
    high: 12,
    low: 9,
    close: 10,
    volume: 100,
    amount: 1000,
    trading_status: 0,
  }
  const change = vi.fn(),
    capture = vi.fn()
  const config = {
    defaultSymbol: 'SSE:600938',
    timeframe: 'DAY' as const,
    priceView: 'QFQ' as const,
    indicators: [],
    comparison: null,
    paneWeights: { price: 4, volume: 1.35 },
    visibleBars: 120,
  }
  const { container, unmount } = render(
    <FinancialChart
      bars={[bar]}
      series={[]}
      onLoadMore={vi.fn()}
      config={config}
      onLayoutChange={change}
      onCaptureLayout={capture}
    />,
  )
  expect(mocks.timeScale.setVisibleLogicalRange).toHaveBeenCalledWith({ from: -116, to: 4 })
  expect(capture.mock.calls[0][0]()).toEqual({
    paneWeights: { price: 1, volume: 1, undefined: 1 },
    visibleBars: 20,
  })
  const plot = container.querySelector('.financial-chart')!
  act(() => {
    plot.dispatchEvent(new Event('pointerdown'))
    window.dispatchEvent(new Event('pointerup'))
  })
  expect(change).not.toHaveBeenCalled()
  act(() => plot.dispatchEvent(new Event('pointerdown')))
  mocks.timeScale.getVisibleLogicalRange.mockReturnValueOnce({ from: 0, to: 80 })
  act(() => window.dispatchEvent(new Event('pointerup')))
  expect(change).toHaveBeenCalledWith(expect.objectContaining({ visibleBars: 80 }))
  act(() => plot.dispatchEvent(new Event('wheel')))
  mocks.timeScale.getVisibleLogicalRange.mockReturnValueOnce({ from: 0, to: 60 })
  act(() => vi.advanceTimersByTime(151))
  expect(change).toHaveBeenLastCalledWith(expect.objectContaining({ visibleBars: 60 }))
  const hover = mocks.chart.subscribeCrosshairMove.mock.calls.at(-1)![0] as unknown as (p: {
    time: string
  }) => void
  act(() => hover({ time: '2026-09-01' }))
  unmount()
  expect(capture).toHaveBeenLastCalledWith(null)
  vi.useRealTimers()
})

it('uses the initial window even when comparison arrives after historical paging', () => {
  const bar = {
    open_time: '2026-09-02',
    close_time: '2026-09-02',
    open: 20,
    high: 22,
    low: 19,
    close: 20,
    volume: 100,
    amount: 1000,
    trading_status: 0,
  }
  const older = { ...bar, close_time: '2026-09-01', close: 10 }
  const props = { series: [], onLoadMore: vi.fn(), comparisonLabel: '原油' }
  const { rerender } = render(<FinancialChart {...props} bars={[bar]} />)
  rerender(<FinancialChart {...props} bars={[older, bar]} />)
  rerender(
    <FinancialChart
      {...props}
      bars={[older, bar]}
      comparison={[
        { ...older, close: 50 },
        { ...bar, close: 100 },
      ]}
    />,
  )
  expect(mocks.setData).toHaveBeenCalledWith([
    { time: '2026-09-01', value: -50 },
    { time: '2026-09-02', value: 0 },
  ])
})

it('detects wheel zoom before the chart child handles the bubbling event', () => {
  vi.useFakeTimers()
  let range = { from: 0, to: 100 }
  mocks.timeScale.getVisibleLogicalRange.mockImplementation(() => range)
  const bar = {
    open_time: '2026-09-01',
    close_time: '2026-09-01',
    open: 10,
    high: 12,
    low: 9,
    close: 10,
    volume: 100,
    amount: 1000,
    trading_status: 0,
  }
  const change = vi.fn()
  const { container, unmount } = render(
    <FinancialChart bars={[bar]} series={[]} onLoadMore={vi.fn()} onLayoutChange={change} />,
  )
  const child = document.createElement('div')
  container.querySelector('.financial-chart')!.append(child)
  child.addEventListener('wheel', () => {
    range = { from: 10, to: 80 }
  })
  act(() => child.dispatchEvent(new Event('wheel', { bubbles: true })))
  act(() => vi.advanceTimersByTime(151))
  expect(change).toHaveBeenCalledWith(expect.objectContaining({ visibleBars: 70 }))
  unmount()
  vi.useRealTimers()
  mocks.timeScale.getVisibleLogicalRange.mockImplementation(() => ({ from: 10, to: 30 }))
})
