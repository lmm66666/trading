import { useEffect, useRef, useState } from 'react'
import {
  ColorType,
  createChart,
  LineSeries,
  type LineData,
  type Time,
} from 'lightweight-charts'
import { fetchRunPage, fromScaled, type EquityPoint } from '../../api/client'

const PAGE_LIMIT = 1000
const MAX_PAGES = 10

interface EquityChartProps {
  runId: string
}

/** 权益曲线：挂载后连续分页拉取全部权益点，金额换算为元后绘制 */
export function EquityChart({ runId }: EquityChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [pointCount, setPointCount] = useState(0)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const chart = createChart(container, {
      autoSize: true,
      layout: {
        background: { type: ColorType.Solid, color: '#0c1017' },
        textColor: '#8d98aa',
      },
      grid: { vertLines: { color: '#18202b' }, horzLines: { color: '#18202b' } },
      rightPriceScale: { borderColor: '#273142' },
      timeScale: { borderColor: '#273142', timeVisible: false },
      localization: { locale: 'zh-CN' },
    })
    const equity = chart.addSeries(LineSeries, {
      color: '#f2b84b', lineWidth: 2, priceLineVisible: false, lastValueVisible: true,
    })

    let active = true
    const loadAll = async () => {
      const points: LineData[] = []
      let after: number | undefined
      try {
        for (let page = 0; page < MAX_PAGES; page += 1) {
          const result = await fetchRunPage<EquityPoint>('backtest', runId, 'equity', after, PAGE_LIMIT)
          if (!active) return
          for (const point of result.items) {
            points.push({ time: point.time.slice(0, 10) as Time, value: fromScaled(point.equity) })
          }
          if (result.next_sequence === undefined) break
          after = result.next_sequence
        }
        if (!active) return
        equity.setData(points)
        chart.timeScale().fitContent()
        setPointCount(points.length)
        setLoading(false)
      } catch (cause) {
        if (!active) return
        setError(cause instanceof Error ? cause.message : '权益曲线加载失败')
        setLoading(false)
      }
    }
    void loadAll()
    return () => {
      active = false
      chart.remove()
    }
  }, [runId])

  return (
    <section className="equity-section" aria-label="权益曲线">
      <header className="results-heading">
        <h3>权益曲线{pointCount > 0 ? `（${pointCount} 点）` : ''}</h3>
      </header>
      <div className="equity-chart" ref={containerRef} role="img" aria-label="回测权益曲线" />
      {loading && <p className="results-hint">权益曲线加载中…</p>}
      {error && <p className="results-error">{error}</p>}
    </section>
  )
}
