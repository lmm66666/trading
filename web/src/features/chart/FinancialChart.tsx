import { useEffect, useMemo, useRef, useState } from 'react'
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
import type { ChartBar, ChartSeries, Timeframe } from '../../api/client'

import type { BoardConfig } from './boards'
import { alignComparison, comparisonBasis, percent, periodKey, type ComparisonBasis } from './comparison'

interface FinancialChartProps {
  bars: ChartBar[]
  series: ChartSeries[]
  onLoadMore: () => void
  comparison?: ChartBar[]
  comparisonLabel?: string
  timeframe?: Timeframe
  config?: BoardConfig
  onCaptureLayout?: (capture: (() => Pick<BoardConfig, 'paneWeights' | 'visibleBars'>) | null) => void
  onLayoutChange?: (layout: Pick<BoardConfig, 'paneWeights' | 'visibleBars'>) => void
}

const lineColors = ['#FDE68A', '#60A5FA', '#C084FC', '#22d3ee', '#f472b6', '#84cc16']

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

export function FinancialChart({
  bars,
  series,
  onLoadMore,
  comparison,
  comparisonLabel,
  timeframe = 'DAY',
  config,
  onLayoutChange,
  onCaptureLayout,
}: FinancialChartProps) {
  const initialBarsRef = useRef(bars)
  const basisRef = useRef<ComparisonBasis | null>(null)
  const fallbackBasisRef = useRef<ComparisonBasis | null>(null)
  if (!fallbackBasisRef.current) fallbackBasisRef.current = comparisonBasis(bars, undefined, timeframe)
  if (!basisRef.current || (comparison && basisRef.current.comparison === undefined))
    basisRef.current = comparisonBasis(initialBarsRef.current, comparison, timeframe)
  const basis = basisRef.current ?? fallbackBasisRef.current
  const displayBasisRef = useRef(basis)
  displayBasisRef.current = basis
  const [hoverTime, setHoverTime] = useState<string | null>(null)
  const comparisonRef = useRef<ISeriesApi<'Line'> | null>(null)
  const layoutRef = useRef({ config, onLayoutChange })
  layoutRef.current = { config, onLayoutChange }
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
    () =>
      series.map((item) => `${item.key}:${item.kind}:${item.component}`).join('|') + (comparisonLabel ?? ''),
    [series, comparisonLabel],
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
      crosshair: {
        vertLine: { color: '#7c8ba1', labelBackgroundColor: '#273142' },
        horzLine: { color: '#7c8ba1', labelBackgroundColor: '#273142' },
      },
      rightPriceScale: { autoScale: true, borderColor: '#1d2432', scaleMargins: { top: 0.08, bottom: 0.08 } },
      handleScale: { axisPressedMouseMove: { price: false, time: true } },
      timeScale: {
        borderColor: '#1d2432',
        timeVisible: false,
        rightOffset: 3,
        barSpacing: 8,
        minBarSpacing: 3,
      },
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
      upColor: '#F87171',
      downColor: '#34D399',
      borderVisible: false,
      wickUpColor: '#F87171',
      wickDownColor: '#34D399',
      priceLineColor: '#F87171',
      priceFormat: { type: 'custom', formatter: (v: number) => `${v.toFixed(2)}%`, minMove: 0.01 },
    })
    candlesRef.current = candles

    const volume = chart.addSeries(
      HistogramSeries,
      {
        title: '成交量',
        priceFormat: { type: 'volume' },
        lastValueVisible: false,
        priceLineVisible: false,
      },
      1,
    )
    volumeRef.current = volume
    volume.priceScale().applyOptions({ scaleMargins: { top: 0.15, bottom: 0.05 } })

    if (comparisonLabel)
      comparisonRef.current = chart.addSeries(
        LineSeries,
        {
          color: '#E2E8F0',
          lineWidth: 2,
          lineStyle: 2,
          title: comparisonLabel,
          priceLineVisible: false,
          lastValueVisible: false,
          priceFormat: { type: 'custom', formatter: (v: number) => `${v.toFixed(2)}%`, minMove: 0.01 },
        },
        0,
      )
    let colorIndex = 0
    const paneByKind = new Map<string, number>()
    for (const item of series) {
      const isOverlay = item.kind === 'SMA' || item.kind === 'EMA'
      let paneIndex = 0
      if (!isOverlay) {
        paneIndex = paneByKind.get(item.kind + item.key.split('/').slice(4).join('/')) ?? paneByKind.size + 2
        paneByKind.set(item.kind + item.key.split('/').slice(4).join('/'), paneIndex)
      }
      const period = item.key.match(/\/p=(\d+)/)?.[1]
      const color =
        item.kind === 'SMA' && ['5', '20', '60'].includes(period ?? '')
          ? lineColors[['5', '20', '60'].indexOf(period!)]
          : lineColors[colorIndex++ % lineColors.length]
      if (item.component === 'histogram') {
        const indicator = chart.addSeries(
          HistogramSeries,
          {
            color,
            priceLineVisible: false,
            lastValueVisible: true,
          },
          paneIndex,
        )
        indicatorUpdatersRef.current.set(item.key, (updated) =>
          indicator.setData(
            updated.points.map((point): HistogramData => ({
              time: chartTime(point.time),
              value: point.value,
              color: point.value >= 0 ? 'rgba(239, 83, 80, .72)' : 'rgba(38, 166, 154, .72)',
            })),
          ),
        )
      } else {
        const indicator = chart.addSeries(
          LineSeries,
          {
            color,
            lineWidth: 1,
            priceLineVisible: false,
            lastValueVisible: !isOverlay,
            ...(isOverlay
              ? {
                  priceFormat: {
                    type: 'custom' as const,
                    formatter: (v: number) => `${v.toFixed(2)}%`,
                    minMove: 0.01,
                  },
                }
              : {}),
            title: isOverlay
              ? `${item.kind} ${item.key.match(/\/p=(\d+)/)?.[1] ?? ''}`
              : `${item.kind} ${item.component.toUpperCase()} ${period ?? ''}`,
          },
          paneIndex,
        )
        indicatorUpdatersRef.current.set(item.key, (updated) =>
          indicator.setData(
            updated.points.map((point): LineData => ({
              time: chartTime(point.time),
              value:
                isOverlay && displayBasisRef.current
                  ? percent(point.value, displayBasisRef.current.main)
                  : point.value,
            })),
          ),
        )
      }
    }
    const panes = chart.panes()
    const paneKeys = ['price', 'volume', ...paneByKind.keys()]
    panes.forEach((pane, index) =>
      pane.setStretchFactor(
        layoutRef.current.config?.paneWeights[paneKeys[index]] ?? (index === 0 ? 4 : 1.35),
      ),
    )
    const captureLayout = () => {
      const range = chart.timeScale().getVisibleLogicalRange()
      return {
        paneWeights: Object.fromEntries(
          chart.panes().map((pane, index) => [paneKeys[index], pane.getStretchFactor()]),
        ),
        visibleBars: range
          ? Math.max(10, Math.min(400, Math.round(range.to - range.from)))
          : (layoutRef.current.config?.visibleBars ?? 120),
      }
    }
    onCaptureLayout?.(captureLayout)
    let layoutTimer = 0
    let beforeInteraction: string | null = null
    const startInteraction = () => {
      beforeInteraction = JSON.stringify(captureLayout())
    }
    const rememberLayout = () => {
      const next = captureLayout()
      if (beforeInteraction !== null && JSON.stringify(next) !== beforeInteraction)
        layoutRef.current.onLayoutChange?.(next)
      beforeInteraction = null
    }
    const wheel = () => {
      if (beforeInteraction === null) startInteraction()
      window.clearTimeout(layoutTimer)
      layoutTimer = window.setTimeout(rememberLayout, 150)
    }
    container.addEventListener('pointerdown', startInteraction)
    window.addEventListener('pointerup', rememberLayout)
    container.addEventListener('wheel', wheel, { passive: true, capture: true })
    const crosshair = (param: { time?: Time }) =>
      setHoverTime(typeof param.time === 'string' ? param.time : null)
    chart.subscribeCrosshairMove(crosshair)
    const rangeHandler = (range: { from: number; to: number } | null) => {
      if (range && range.from < 12) loadMoreRef.current()
    }
    rangeHandlerRef.current = rangeHandler
    return () => {
      if (rangeSubscribedRef.current) chart.timeScale().unsubscribeVisibleLogicalRangeChange(rangeHandler)
      window.clearTimeout(layoutTimer)
      container.removeEventListener('pointerdown', startInteraction)
      window.removeEventListener('pointerup', rememberLayout)
      container.removeEventListener('wheel', wheel, true)
      onCaptureLayout?.(null)
      chart.unsubscribeCrosshairMove(crosshair)
      resizeObserver?.disconnect()
      chart.remove()
      chartRef.current = null
      candlesRef.current = null
      volumeRef.current = null
      comparisonRef.current = null
      indicatorUpdatersRef.current.clear()
      previousBarsCountRef.current = 0
      rangeSubscribedRef.current = false
    }
  }, [definitionKey])

  useEffect(() => {
    const chart = chartRef.current
    const candles = candlesRef.current
    const volume = volumeRef.current
    if (!chart || !candles || !volume || !basis) return
    const previousCount = previousBarsCountRef.current
    const visibleRange = previousCount > 0 ? chart.timeScale().getVisibleLogicalRange() : null
    candles.setData(
      bars.map((bar) => ({
        time: chartTime(bar.close_time),
        open: percent(bar.open, basis.main),
        high: percent(bar.high, basis.main),
        low: percent(bar.low, basis.main),
        close: percent(bar.close, basis.main),
      })),
    )
    volume.setData(
      bars.map((bar) => ({
        time: chartTime(bar.close_time),
        value: bar.volume,
        color: bar.close >= bar.open ? 'rgba(239, 83, 80, .5)' : 'rgba(38, 166, 154, .5)',
      })),
    )
    comparisonRef.current?.setData(
      comparison && basis.comparison
        ? alignComparison(bars, comparison, timeframe).map((p) =>
            p.value === undefined
              ? { time: chartTime(p.time) }
              : { time: chartTime(p.time), value: percent(p.value!, basis.comparison!) },
          )
        : [],
    )
    for (const item of series) indicatorUpdatersRef.current.get(item.key)?.(item)
    const prepended = bars.length - previousCount
    if (previousCount === 0) {
      if (config)
        chart
          .timeScale()
          .setVisibleLogicalRange({ from: bars.length + 3 - config.visibleBars, to: bars.length + 3 })
      else chart.timeScale().fitContent()
      chart.timeScale().subscribeVisibleLogicalRangeChange(rangeHandlerRef.current)
      rangeSubscribedRef.current = true
    } else if (visibleRange && prepended > 0) {
      chart
        .timeScale()
        .setVisibleLogicalRange({ from: visibleRange.from + prepended, to: visibleRange.to + prepended })
    }
    previousBarsCountRef.current = bars.length
  }, [bars, definitionKey, series, comparison, basis, timeframe])

  const selectedBar = bars.find((b) => b.close_time.slice(0, 10) === hoverTime) ?? bars.at(-1)
  return (
    <>
      <div className="chart-legend">
        <span>涨跌幅 % · 基准 {basis?.date ?? '不可用'}</span>
        <span>
          {selectedBar?.close_time.slice(0, 10)} 收 {selectedBar?.close.toFixed(2)}
        </span>
        {series
          .filter((s) => s.kind === 'SMA' || s.kind === 'EMA')
          .map((item) => (
            <span key={item.key}>
              {item.kind} {item.key.match(/\/p=(\d+)/)?.[1]}{' '}
              {item.points
                .find((p) => p.time.slice(0, 10) === selectedBar?.close_time.slice(0, 10))
                ?.value?.toFixed(2) ?? '—'}
            </span>
          ))}
        {comparisonLabel && (
          <span>
            {comparisonLabel}{' '}
            {comparison
              ?.find(
                (b) =>
                  periodKey(b.close_time, timeframe) ===
                  (selectedBar ? periodKey(selectedBar.close_time, timeframe) : ''),
              )
              ?.close.toFixed(2) ?? '—'}
          </span>
        )}
        {comparison && !basisRef.current && <span role="alert">没有共同有效基准日，仅显示股票</span>}
      </div>
      <div
        className="financial-chart"
        ref={containerRef}
        role="img"
        aria-label="K 线、成交量与技术指标图表"
      />
    </>
  )
}
