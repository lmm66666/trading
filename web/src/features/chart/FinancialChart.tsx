import { useEffect, useRef } from 'react'
import {
  CandlestickSeries,
  ColorType,
  createChart,
  HistogramSeries,
  LineSeries,
  type HistogramData,
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

export function FinancialChart({ bars, series, onLoadMore }: FinancialChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const loadMoreRef = useRef(onLoadMore)
  loadMoreRef.current = onLoadMore

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const chart = createChart(container, {
      autoSize: true,
      layout: {
        background: { type: ColorType.Solid, color: '#0c1017' },
        textColor: '#8d98aa',
        panes: { separatorColor: '#242c39', separatorHoverColor: '#344054', enableResize: true },
      },
      grid: { vertLines: { color: '#18202b' }, horzLines: { color: '#18202b' } },
      crosshair: { vertLine: { color: '#7c8ba1', labelBackgroundColor: '#273142' }, horzLine: { color: '#7c8ba1', labelBackgroundColor: '#273142' } },
      rightPriceScale: { borderColor: '#273142', scaleMargins: { top: 0.08, bottom: 0.23 } },
      timeScale: { borderColor: '#273142', timeVisible: false, rightOffset: 3, barSpacing: 8, minBarSpacing: 3 },
      localization: { locale: 'zh-CN' },
    })
    const candles = chart.addSeries(CandlestickSeries, {
      upColor: '#ef4444', downColor: '#10b981', borderVisible: false,
      wickUpColor: '#ef4444', wickDownColor: '#10b981', priceLineColor: '#ef4444',
    })
    candles.setData(bars.map((bar) => ({
      time: chartTime(bar.close_time), open: bar.open, high: bar.high, low: bar.low, close: bar.close,
    })))

    const volume = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' }, priceScaleId: 'volume', lastValueVisible: false, priceLineVisible: false,
    })
    volume.priceScale().applyOptions({ scaleMargins: { top: 0.8, bottom: 0 } })
    volume.setData(bars.map((bar) => ({
      time: chartTime(bar.close_time), value: bar.volume,
      color: bar.close >= bar.open ? 'rgba(239, 68, 68, .55)' : 'rgba(16, 185, 129, .55)',
    })))

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
        indicator.setData(item.points.map((point): HistogramData => ({
          time: chartTime(point.time), value: point.value,
          color: point.value >= 0 ? 'rgba(239, 68, 68, .72)' : 'rgba(16, 185, 129, .72)',
        })))
      } else {
        const indicator = chart.addSeries(LineSeries, {
          color, lineWidth: isOverlay ? 2 : 1, priceLineVisible: false,
          lastValueVisible: true, title: isOverlay ? `${item.kind} ${item.key.match(/period=(\d+)/)?.[1] ?? ''}` : item.component.toUpperCase(),
        }, paneIndex)
        indicator.setData(item.points.map((point): LineData => ({ time: chartTime(point.time), value: point.value })))
      }
    }
    const panes = chart.panes()
    if (panes[0]) panes[0].setStretchFactor(4)
    for (let index = 1; index < panes.length; index++) panes[index].setStretchFactor(1.35)
    chart.timeScale().fitContent()
    const rangeHandler = (range: { from: number; to: number } | null) => {
      if (range && range.from < 12) loadMoreRef.current()
    }
    chart.timeScale().subscribeVisibleLogicalRangeChange(rangeHandler)
    return () => {
      chart.timeScale().unsubscribeVisibleLogicalRangeChange(rangeHandler)
      chart.remove()
    }
  }, [bars, series])

  return <div className="financial-chart" ref={containerRef} role="img" aria-label="K 线、成交量与技术指标图表" />
}
