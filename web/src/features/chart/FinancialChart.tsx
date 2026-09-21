import { useEffect, useMemo, useRef, useState } from 'react'
import {
  CandlestickSeries,
  ColorType,
  createChart,
  HistogramSeries,
  LineSeries,
  type AutoscaleInfoProvider,
  type HistogramData,
  type IChartApi,
  type ISeriesApi,
  type LineData,
  type Time,
} from 'lightweight-charts'
import type { ChartBar, ChartSeries, Timeframe } from '../../api/client'

import { zScoreBands } from './zScoreBands'
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

const UP_COLOR = '#ef5350'
const DOWN_COLOR = '#26a69a'
const UP_COLOR_SOFT = 'rgba(239, 83, 80, .72)'
const DOWN_COLOR_SOFT = 'rgba(38, 166, 154, .72)'
const lineColors = ['#FDE68A', '#60A5FA', '#C084FC', '#22d3ee', '#f472b6', '#84cc16']
const ZSCORE_COLORS = {
  histogram: '#c0c4cc',
  smooth: '#e2e8f0',
  regime: '#C084FC',
  high: '#fb923c',
  low: '#22d3ee',
}
const zscoreColor = (value: number) =>
  value >= 2 ? ZSCORE_COLORS.high : value <= -2 ? ZSCORE_COLORS.low : ZSCORE_COLORS.histogram
const sigmaFormat = {
  type: 'custom' as const,
  formatter: (value: number) => `${value.toFixed(2)}σ`,
  minMove: 0.01,
}
const zscoreAutoscale: AutoscaleInfoProvider = (original) => {
  const info = original()
  return {
    ...info,
    priceRange: {
      minValue: Math.min(info?.priceRange?.minValue ?? 0, -2),
      maxValue: Math.max(info?.priceRange?.maxValue ?? 0, 2),
    },
  }
}
const SMA_PERIODS = ['5', '20', '60']

const chartTime = (value: string): Time => value.slice(0, 10) as Time

interface LegendItem {
  /** React key：series key + component，同一指标的各分量唯一 */
  key: string
  /** 原始 series key 与分量，用于回查数值（同指标各分量共享 series key） */
  seriesKey: string
  component: string
  label: string
  color: string
  signed: boolean
  sigma?: boolean
}

interface LegendGroup {
  title: string
  paneIndex: number
  items: LegendItem[]
}

const componentLabels: Record<string, string> = {
  dif: 'DIF',
  dea: 'DEA',
  histogram: '柱',
  k: 'K',
  d: 'D',
  j: 'J',
  smooth: '平滑',
  regime: '长期',
}

const periodOf = (key: string): string => key.match(/\/p=(\d+)/)?.[1] ?? ''

function groupTitle(item: ChartSeries): string {
  if (item.kind === 'ZSCORE')
    return `Z-score ${periodOf(item.key)},${item.key.match(/\/sm=(\d+)/)?.[1] ?? ''},${item.key.match(/\/rg=(\d+)/)?.[1] ?? ''}`
  if (item.kind === 'MACD') {
    const fast = item.key.match(/\/f=(\d+)/)?.[1] ?? ''
    const slow = item.key.match(/\/s=(\d+)/)?.[1] ?? ''
    const signal = item.key.match(/\/sig=(\d+)/)?.[1] ?? ''
    return `MACD ${fast},${slow},${signal}`
  }
  return `${item.kind} ${periodOf(item.key)}`
}

/** 图例分组与 pane 归属：叠加均线归主图（pane 0），成交量固定 pane 1，副图指标按 kind+参数归 pane 2+ */
function buildLegendModel(series: ChartSeries[]): {
  colors: Map<string, string>
  paneIndexByKey: Map<string, number>
  paneKeys: string[]
  overlay: LegendGroup | null
  panes: LegendGroup[]
} {
  const colors = new Map<string, string>()
  const paneIndexByKey = new Map<string, number>()
  const overlayItems: LegendItem[] = []
  const paneGroups: LegendGroup[] = []
  const groupIndex = new Map<string, number>()
  const paneByKind = new Map<string, number>()
  let colorIndex = 0
  series.forEach((item) => {
    const period = item.key.match(/\/p=(\d+)/)?.[1]
    const color =
      item.kind === 'ZSCORE'
        ? ZSCORE_COLORS[item.component as 'histogram' | 'smooth' | 'regime']
        : item.kind === 'SMA' && SMA_PERIODS.includes(period ?? '')
          ? lineColors[SMA_PERIODS.indexOf(period!)]
          : lineColors[colorIndex++ % lineColors.length]
    colors.set(item.key, color)
    const isOverlay = item.kind === 'SMA' || item.kind === 'EMA'
    const groupKey = item.kind + item.key.split('/').slice(4).join('/')
    let paneIndex = 0
    if (!isOverlay) {
      paneIndex = paneByKind.get(groupKey) ?? paneByKind.size + 2
      paneByKind.set(groupKey, paneIndex)
    }
    paneIndexByKey.set(item.key, paneIndex)
    if (isOverlay) {
      overlayItems.push({
        key: item.key,
        seriesKey: item.key,
        component: item.component,
        label: `${item.kind} ${periodOf(item.key)}`,
        color,
        signed: false,
      })
      return
    }
    let paneGroupIndex = groupIndex.get(groupKey)
    if (paneGroupIndex === undefined) {
      paneGroups.push({ title: groupTitle(item), paneIndex, items: [] })
      paneGroupIndex = paneGroups.length - 1
      groupIndex.set(groupKey, paneGroupIndex)
    }
    paneGroups[paneGroupIndex].items.push({
      key: `${item.key}#${item.component}`,
      seriesKey: item.key,
      component: item.component,
      label: componentLabels[item.component] ?? item.component.toUpperCase(),
      color,
      signed: item.kind !== 'ZSCORE' && item.component === 'histogram',
      sigma: item.kind === 'ZSCORE',
    })
  })
  return {
    colors,
    paneIndexByKey,
    paneKeys: [...paneByKind.keys()],
    overlay: overlayItems.length > 0 ? { title: '', paneIndex: 0, items: overlayItems } : null,
    panes: paneGroups,
  }
}

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
  const [paneTops, setPaneTops] = useState<number[]>([0])
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
  const legendModel = useMemo(() => buildLegendModel(series), [series])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const chart = createChart(container, {
      autoSize: true,
      layout: {
        attributionLogo: false,
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
    // 计算各 pane 顶部相对图表容器的偏移，供图例定位到对应 pane 左上角
    const measurePaneTops = () => {
      const panesApi = chart.panes()
      if (panesApi.length === 0) return
      const paneEls = panesApi.map((pane) => pane.getHTMLElement())
      const tops: number[] = []
      if (paneEls.every((el): el is HTMLElement => el !== null)) {
        const baseTop = container.getBoundingClientRect().top
        panesApi.forEach((_, index) => {
          tops[index] = paneEls[index].getBoundingClientRect().top - baseTop
        })
      } else {
        const heights = panesApi.map((pane) => pane.getHeight())
        const separatorHeight =
          panesApi.length > 1
            ? Math.max(0, container.clientHeight - heights.reduce((sum, height) => sum + height, 0)) /
              (panesApi.length - 1)
            : 0
        let top = 0
        heights.forEach((height, index) => {
          tops[index] = top
          top += height + separatorHeight
        })
      }
      setPaneTops((prev) =>
        prev.length === tops.length && prev.every((top, index) => top === tops[index]) ? prev : tops,
      )
    }
    applyBarSpacingLimits()
    let resizeObserver: ResizeObserver | null = null
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        applyBarSpacingLimits()
        measurePaneTops()
      })
      resizeObserver.observe(container)
    }
    const candles = chart.addSeries(CandlestickSeries, {
      upColor: UP_COLOR,
      downColor: DOWN_COLOR,
      borderVisible: false,
      wickUpColor: UP_COLOR,
      wickDownColor: DOWN_COLOR,
      priceLineColor: UP_COLOR,
      // 纵轴高度按涨跌幅等比（candles 数据已是相对基准的百分比）；
      // 无同图叠加时轴标签按当前基准还原为价格，有叠加时两个品种基准不同、只能显示百分比
      priceFormat: {
        type: 'custom',
        formatter: (v: number) =>
          comparisonLabel || !displayBasisRef.current
            ? `${v.toFixed(2)}%`
            : (displayBasisRef.current.main * (1 + v / 100)).toFixed(2),
        minMove: 0.01,
      },
    })
    candlesRef.current = candles

    const volume = chart.addSeries(
      HistogramSeries,
      {
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
          priceLineVisible: false,
          lastValueVisible: false,
          priceFormat: { type: 'custom', formatter: (v: number) => `${v.toFixed(2)}%`, minMove: 0.01 },
        },
        0,
      )
    // 右轴只保留当前价与十字光标标签，指标值统一由图例展示，避免与 K 线冲突
    for (const item of series) {
      const isOverlay = item.kind === 'SMA' || item.kind === 'EMA'
      const paneIndex = legendModel.paneIndexByKey.get(item.key) ?? 0
      const color = legendModel.colors.get(item.key) ?? lineColors[0]
      const isZSCORE = item.kind === 'ZSCORE'
      if (item.component === 'histogram') {
        const indicator = chart.addSeries(
          HistogramSeries,
          {
            color,
            ...(isZSCORE ? { priceFormat: sigmaFormat, autoscaleInfoProvider: zscoreAutoscale } : {}),
            priceLineVisible: false,
            lastValueVisible: false,
          },
          paneIndex,
        )
        if (isZSCORE) {
          indicator.attachPrimitive(zScoreBands((value) => indicator.priceToCoordinate(value)))
          for (const price of [2, 1, 0, -1, -2])
            indicator.createPriceLine({
              price,
              color: price > 0 ? ZSCORE_COLORS.high : price < 0 ? ZSCORE_COLORS.low : ZSCORE_COLORS.histogram,
              lineWidth: 1,
              lineStyle: price === 0 ? 0 : 2,
              axisLabelVisible: true,
              title: '',
            })
        }
        indicatorUpdatersRef.current.set(item.key, (updated) =>
          indicator.setData(
            updated.points.map((point): HistogramData => ({
              time: chartTime(point.time),
              value: point.value,
              color: isZSCORE ? zscoreColor(point.value) : point.value >= 0 ? UP_COLOR_SOFT : DOWN_COLOR_SOFT,
            })),
          ),
        )
      } else {
        const indicator = chart.addSeries(
          LineSeries,
          {
            color,
            lineWidth: 1,
            ...(isZSCORE ? { priceFormat: sigmaFormat } : {}),
            priceLineVisible: false,
            lastValueVisible: false,
            ...(isOverlay
              ? {
                  priceFormat: {
                    type: 'custom' as const,
                    formatter: (v: number) => `${v.toFixed(2)}%`,
                    minMove: 0.01,
                  },
                }
              : {}),
          },
          paneIndex,
        )
        indicatorUpdatersRef.current.set(item.key, (updated) =>
          indicator.setData(
            updated.points.map((point): LineData => ({
              time: chartTime(point.time),
              ...(isZSCORE && item.component === 'smooth' ? { color: zscoreColor(point.value) } : {}),
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
    const paneKeys = ['price', 'volume', ...legendModel.paneKeys]
    panes.forEach((pane, index) =>
      pane.setStretchFactor(
        layoutRef.current.config?.paneWeights[paneKeys[index]] ?? (index === 0 ? 4 : 1.35),
      ),
    )
    measurePaneTops()
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
      if (beforeInteraction !== null) {
        const next = captureLayout()
        if (JSON.stringify(next) !== beforeInteraction)
          layoutRef.current.onLayoutChange?.(next)
        beforeInteraction = null
        // 交互（含拖动 pane 分隔条）结束后按最终布局校正图例位置
        measurePaneTops()
      }
    }
    // 拖动 pane 分隔条时 pane 高度持续变化；pane 元素是 <tr>，其上的 ResizeObserver
    // 在浏览器中不可靠（table-row 不派发 RO 通知），改为交互期间随指针移动实时重测
    const syncPaneTopsWhileDragging = () => {
      if (beforeInteraction !== null) measurePaneTops()
    }
    const wheel = () => {
      if (beforeInteraction === null) startInteraction()
      window.clearTimeout(layoutTimer)
      layoutTimer = window.setTimeout(rememberLayout, 150)
    }
    container.addEventListener('pointerdown', startInteraction)
    container.addEventListener('pointermove', syncPaneTopsWhileDragging)
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
      container.removeEventListener('pointermove', syncPaneTopsWhileDragging)
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

  // 图例取值：优先十字光标悬停的 K 线，光标离开时回退到最新一根
  const selectedIndex = hoverTime
    ? bars.findIndex((b) => b.close_time.slice(0, 10) === hoverTime)
    : bars.length - 1
  const selectedBar = selectedIndex >= 0 ? bars[selectedIndex] : undefined
  const prevClose = selectedIndex > 0 ? bars[selectedIndex - 1].close : undefined
  const hoverDate = selectedBar?.close_time.slice(0, 10)
  const barUp = selectedBar ? selectedBar.close >= selectedBar.open : false
  const change = selectedBar && prevClose !== undefined ? selectedBar.close - prevClose : null
  const changePct =
    selectedBar && prevClose !== undefined && prevClose > 0
      ? ((selectedBar.close - prevClose) / prevClose) * 100
      : null
  // 同一指标的各分量（DIF/DEA/柱…）共享 series key，需同时按 component 精确匹配
  const pointValue = (seriesKey: string, component: string) =>
    series
      .find((item) => item.key === seriesKey && item.component === component)
      ?.points.find((p) => p.time.slice(0, 10) === hoverDate)?.value
  const compactVolume = (value: number) =>
    value >= 1e8 ? `${(value / 1e8).toFixed(2)}亿` : value >= 1e4 ? `${(value / 1e4).toFixed(2)}万` : `${value}`
  const comparisonPoint = comparison?.find(
    (b) => periodKey(b.close_time, timeframe) === (selectedBar ? periodKey(selectedBar.close_time, timeframe) : ''),
  )
  const comparisonPct =
    comparisonPoint && basis?.comparison && comparisonPoint.close > 0
      ? percent(comparisonPoint.close, basis.comparison)
      : null
  const renderLegendItem = (item: LegendItem) => {
    const value = pointValue(item.seriesKey, item.component)
    const signClass =
      item.signed && value !== undefined ? (value >= 0 ? 'legend-up' : 'legend-down') : undefined
    return (
      <span key={item.key}>
        <em
          className={signClass}
          style={
            signClass
              ? undefined
              : {
                  color:
                    item.sigma && item.component === 'histogram' && value !== undefined
                      ? zscoreColor(value)
                      : item.color,
                }
          }
        >
          {item.label}
        </em>
        <b className={signClass}>
          {value !== undefined ? `${value.toFixed(2)}${item.sigma ? 'σ' : ''}` : '—'}
        </b>
      </span>
    )
  }
  return (
    <div className="chart-canvas-frame">
      {/* 主图图例：日期一行 + OHLC/涨跌/量一行 + 叠加均线一行 + 对比行 */}
      <div className="chart-legend chart-legend-main" style={{ top: (paneTops[0] ?? 0) + 6 }}>
        <div className="chart-legend-row chart-legend-date">
          <span>{hoverDate ?? '—'}</span>
          {basis && <em className="legend-muted">· 基准 {basis.date}</em>}
        </div>
        {selectedBar && (
          <div className="chart-legend-row">
            <span>
              <em>O</em> <b className={barUp ? 'legend-up' : 'legend-down'}>{selectedBar.open.toFixed(2)}</b>
            </span>
            <span>
              <em>H</em> <b className={barUp ? 'legend-up' : 'legend-down'}>{selectedBar.high.toFixed(2)}</b>
            </span>
            <span>
              <em>L</em> <b className={barUp ? 'legend-up' : 'legend-down'}>{selectedBar.low.toFixed(2)}</b>
            </span>
            <span>
              <em>C</em> <b className={barUp ? 'legend-up' : 'legend-down'}>{selectedBar.close.toFixed(2)}</b>
            </span>
            {change !== null && changePct !== null && (
              <span>
                <b className={change === 0 ? 'legend-flat' : change > 0 ? 'legend-up' : 'legend-down'}>
                  {change > 0 ? '+' : ''}
                  {change.toFixed(2)} ({change > 0 ? '+' : ''}
                  {changePct.toFixed(2)}%)
                </b>
              </span>
            )}
            <span>
              <em>V</em> <b>{compactVolume(selectedBar.volume)}</b>
            </span>
          </div>
        )}
        {legendModel.overlay && (
          <div className="chart-legend-row">
            {legendModel.overlay.items.map((item) => renderLegendItem(item))}
          </div>
        )}
        {comparisonLabel && (
          <div className="chart-legend-row">
            <span>
              <em className="legend-muted">{comparisonLabel}</em>{' '}
              <b>{comparisonPct !== null ? `${comparisonPct >= 0 ? '+' : ''}${comparisonPct.toFixed(2)}%` : '—'}</b>
            </span>
          </div>
        )}
        {comparison && !basisRef.current && <span role="alert">没有共同有效基准日，仅显示股票</span>}
      </div>
      {/* 副图图例：定位到对应 pane 左上角，展示指标原始值 */}
      {legendModel.panes.map((group) => (
        <div
          key={group.title}
          className="chart-legend chart-legend-pane"
          style={{ top: (paneTops[group.paneIndex] ?? 0) + 6 }}
        >
          <div className="chart-legend-row">
            <span className="legend-muted">{group.title}</span>
            {group.items.map((item) => renderLegendItem(item))}
          </div>
        </div>
      ))}
      <div
        className="financial-chart"
        ref={containerRef}
        role="img"
        aria-label="K 线、成交量与技术指标图表"
      />
    </div>
  )
}
