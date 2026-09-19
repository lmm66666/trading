import { useEffect, useMemo, useRef } from 'react'
import {
  CandlestickSeries,
  ColorType,
  createChart,
  HistogramSeries,
  LineSeries,
  type HistogramData,
  type IChartApi,
  type ISeriesApi,
  type LineData,
  type Time,
} from 'lightweight-charts'
import type { ChartBar, ChartSeries } from '../../api/client'

interface FinancialChartProps {
  bars: ChartBar[]
  series: ChartSeries[]
  onLoadMore: () => void
}

const lineColors = ['#f5a623', '#3b82f6', '#a855f7', '#22d3ee', '#f472b6', '#84cc16']

const chartTime = (value: string): Time => value.slice(0, 10) as Time

/** 可见 K 线数量上下限：桌面 15–400 根；容器宽 < 768px（移动设备）10–160 根 */
const MOBILE_MAX_WIDTH = 768
const DESKTOP_MIN_BARS = 15
const DESKTOP_MAX_BARS = 400
const MOBILE_MIN_BARS = 10
const MOBILE_MAX_BARS = 160
const MIN_BAR_SPACING_PX = 3

/** 按容器宽度换算 barSpacing 边界：maxBarSpacing 防止单根过粗，minBarSpacing 限制最多可见根数 */
export function computeBarSpacingLimits(width: number): { minBarSpacing: number; maxBarSpacing: number } {
  const mobile = width < MOBILE_MAX_WIDTH
  const minBars = mobile ? MOBILE_MIN_BARS : DESKTOP_MIN_BARS
  const maxBars = mobile ? MOBILE_MAX_BARS : DESKTOP_MAX_BARS
  return {
    maxBarSpacing: width / minBars,
    minBarSpacing: Math.max(width / maxBars, MIN_BAR_SPACING_PX),
  }
}

export function FinancialChart({ bars, series, onLoadMore }: FinancialChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const loadMoreRef = useRef(onLoadMore)
  const chartRef = useRef<IChartApi | null>(null)
  const candlesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const volumeRef = useRef<ISeriesApi<'Histogram'> | null>(null)
  const indicatorUpdatersRef = useRef(new Map<string, (item: ChartSeries) => void>())
  const previousBarsCountRef = useRef(0)
  const rangeHandlerRef = useRef<(range: { from: number; to: number } | null) => void>(() => undefined)
  const rangeSubscribedRef = useRef(false)
  loadMoreRef.current = onLoadMore
  const definitionKey = useMemo(
    () => series.map((item) => `${item.key}:${item.kind}:${item.component}`).join('|'),
    [series],
  )

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const chart = createChart(container, {
      autoSize: true,
      layout: {
        background: { type: ColorType.Solid, color: '#0b0e14' },
        textColor: '#8792a6',
        panes: { separatorColor: '#1d2432', separatorHoverColor: '#2a3550', enableResize: true },
      },
      grid: { vertLines: { color: '#161c28' }, horzLines: { color: '#161c28' } },
      crosshair: { vertLine: { color: '#7c8ba1', labelBackgroundColor: '#273142' }, horzLine: { color: '#7c8ba1', labelBackgroundColor: '#273142' } },
      rightPriceScale: { borderColor: '#1d2432', scaleMargins: { top: 0.08, bottom: 0.23 } },
      timeScale: { borderColor: '#1d2432', timeVisible: false, rightOffset: 3, barSpacing: 8, minBarSpacing: 3 },
      localization: { locale: 'zh-CN' },
    })
    chartRef.current = chart
    // TWR-011：可见 K 线数量按设备上下限换算为 barSpacing 边界，容器尺寸变化时重算
    const applyBarSpacingLimits = () => {
      const width = container.clientWidth
      if (width <= 0) return
      const { minBarSpacing, maxBarSpacing } = computeBarSpacingLimits(width)
      chart.timeScale().applyOptions({ minBarSpacing, maxBarSpacing })
    }
    applyBarSpacingLimits()
    let resizeObserver: ResizeObserver | null = null
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(applyBarSpacingLimits)
      resizeObserver.observe(container)
    }
    const candles = chart.addSeries(CandlestickSeries, {
      upColor: '#ef5350', downColor: '#26a69a', borderVisible: false,
      wickUpColor: '#ef5350', wickDownColor: '#26a69a', priceLineColor: '#ef5350',
    })
    candlesRef.current = candles

    const volume = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' }, priceScaleId: 'volume', lastValueVisible: false, priceLineVisible: false,
    })
    volumeRef.current = volume
    volume.priceScale().applyOptions({ scaleMargins: { top: 0.8, bottom: 0 } })

    let colorIndex = 0
    const paneByKind = new Map<string, number>()
    for (const item of series) {
      const isOverlay = item.kind === 'SMA' || item.kind === 'EMA'
      let paneIndex = 0
      if (!isOverlay) {
        paneIndex = paneByKind.get(item.kind) ?? paneByKind.size + 1
        paneByKind.set(item.kind, paneIndex)
      }
      const color = lineColors[colorIndex++ % lineColors.length]
      if (item.component === 'histogram') {
        const indicator = chart.addSeries(HistogramSeries, {
          color, priceLineVisible: false, lastValueVisible: true,
        }, paneIndex)
        indicatorUpdatersRef.current.set(item.key, (updated) => indicator.setData(updated.points.map((point): HistogramData => ({
          time: chartTime(point.time), value: point.value, color: point.value >= 0 ? 'rgba(239, 83, 80, .72)' : 'rgba(38, 166, 154, .72)',
        }))))
      } else {
        const indicator = chart.addSeries(LineSeries, {
          color, lineWidth: isOverlay ? 2 : 1, priceLineVisible: false,
          lastValueVisible: true, title: isOverlay ? `${item.kind} ${item.key.match(/\/p=(\d+)/)?.[1] ?? ''}` : item.component.toUpperCase(),
        }, paneIndex)
        indicatorUpdatersRef.current.set(item.key, (updated) => indicator.setData(updated.points.map((point): LineData => ({ time: chartTime(point.time), value: point.value }))))
      }
    }
    const panes = chart.panes()
    if (panes[0]) panes[0].setStretchFactor(4)
    for (let index = 1; index < panes.length; index++) panes[index].setStretchFactor(1.35)
    const rangeHandler = (range: { from: number; to: number } | null) => {
      if (range && range.from < 12) loadMoreRef.current()
    }
    rangeHandlerRef.current = rangeHandler
    return () => {
      if (rangeSubscribedRef.current) chart.timeScale().unsubscribeVisibleLogicalRangeChange(rangeHandler)
      resizeObserver?.disconnect()
      chart.remove()
      chartRef.current = null
      candlesRef.current = null
      volumeRef.current = null
      indicatorUpdatersRef.current.clear()
      previousBarsCountRef.current = 0
      rangeSubscribedRef.current = false
    }
  }, [definitionKey])

  useEffect(() => {
    const chart = chartRef.current
    const candles = candlesRef.current
    const volume = volumeRef.current
    if (!chart || !candles || !volume) return
    const previousCount = previousBarsCountRef.current
    const visibleRange = previousCount > 0 ? chart.timeScale().getVisibleLogicalRange() : null
    candles.setData(bars.map((bar) => ({
      time: chartTime(bar.close_time), open: bar.open, high: bar.high, low: bar.low, close: bar.close,
    })))
    volume.setData(bars.map((bar) => ({
      time: chartTime(bar.close_time), value: bar.volume,
      color: bar.close >= bar.open ? 'rgba(239, 83, 80, .5)' : 'rgba(38, 166, 154, .5)',
    })))
    for (const item of series) indicatorUpdatersRef.current.get(item.key)?.(item)
    const prepended = bars.length - previousCount
    if (previousCount === 0) {
      chart.timeScale().fitContent()
      chart.timeScale().subscribeVisibleLogicalRangeChange(rangeHandlerRef.current)
      rangeSubscribedRef.current = true
    } else if (visibleRange && prepended > 0) {
      chart.timeScale().setVisibleLogicalRange({ from: visibleRange.from + prepended, to: visibleRange.to + prepended })
    }
    previousBarsCountRef.current = bars.length
  }, [bars, definitionKey, series])

  return <div className="financial-chart" ref={containerRef} role="img" aria-label="K 线、成交量与技术指标图表" />
}
