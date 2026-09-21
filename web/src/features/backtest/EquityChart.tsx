import { useEffect, useRef, useState } from 'react'
import { ColorType, createChart, LineSeries, type MouseEventParams, type Time } from 'lightweight-charts'
import { fetchRunPage, fromScaled, type EquityPoint } from '../../api/client'

const PAGE_LIMIT = 1000
const MAX_PAGES = 10
interface EquityChartProps { runId: string; active?: boolean }
export function EquityChart(props: EquityChartProps) { return <EquityChartContent key={props.runId} {...props} /> }
function EquityChartContent({ runId, active = true }: EquityChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [points, setPoints] = useState<EquityPoint[]>([])
  const [ready, setReady] = useState(false)
  const [incomplete, setIncomplete] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [hover, setHover] = useState<EquityPoint | null>(null)
  useEffect(() => {
    if (!active || ready) return
    let current = true
    setError(null)
    const load = async () => {
      const collected: EquityPoint[] = []
      let after: number | undefined
      try {
        for (let page = 0; page < MAX_PAGES; page += 1) {
          const result = await fetchRunPage<EquityPoint>('backtest', runId, 'equity', after, PAGE_LIMIT)
          if (!current) return
          collected.push(...result.items)
          after = result.next_sequence
          if (after === undefined) break
        }
        setPoints(collected)
        setIncomplete(after !== undefined)
        setReady(true)
      } catch (cause) { if (current) setError(cause instanceof Error ? cause.message : '权益曲线加载失败') }
    }
    void load()
    return () => { current = false }
  }, [runId, active, ready, attempt])
  useEffect(() => {
    if (!containerRef.current) return
    const chart = createChart(containerRef.current, {
      autoSize: true,
      layout: { background: { type: ColorType.Solid, color: '#0b0e14' }, textColor: '#a0abc0' },
      grid: { vertLines: { color: '#161c28' }, horzLines: { color: '#161c28' } },
      rightPriceScale: { borderColor: '#1d2432' }, timeScale: { borderColor: '#1d2432', timeVisible: false },
      localization: { locale: 'zh-CN', priceFormatter: (value: number) => value.toLocaleString('zh-CN', { maximumFractionDigits: 2 }) },
    })
    const series = chart.addSeries(LineSeries, { color: '#6188ff', lineWidth: 2, priceLineVisible: false, lastValueVisible: true })
    if (points.length) {
      series.setData(points.map((p) => ({ time: p.time.slice(0, 10) as Time, value: fromScaled(p.equity) })))
      chart.timeScale().fitContent()
    }
    const byDate = new Map(points.map((point) => [point.time.slice(0, 10), point]))
    const crosshair = (event: MouseEventParams) => {
      const time = event.time
      const date = typeof time === 'string' ? time : typeof time === 'object' ? `${time.year}-${String(time.month).padStart(2, '0')}-${String(time.day).padStart(2, '0')}` : ''
      setHover(byDate.get(date) ?? null)
    }
    chart.subscribeCrosshairMove(crosshair)
    return () => { chart.unsubscribeCrosshairMove(crosshair); chart.remove() }
  }, [points])
  const selected = hover ?? points.at(-1)
  const money = (value: number) => fromScaled(value).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return <section className="equity-section" aria-label="权益曲线">
    <header className="results-heading"><h3>账户权益</h3><span>单位：元</span></header>
    <div className="backtest-equity-legend">{selected && <><span>{selected.time.slice(0, 10)}</span><span>权益 {money(selected.equity)} 元</span><span>现金 {money(selected.cash)} 元</span><span>持仓市值 {money(selected.position_value)} 元</span></>}</div>
    <div className="equity-chart" ref={containerRef} role="img" aria-label="回测权益曲线" />
    {!ready && !error && <p className="results-hint" role="status">权益曲线加载中…</p>}
    {error && <p className="results-error" role="alert">{error} <button type="button" className="table-load-more" onClick={() => setAttempt((n) => n + 1)}>重试权益曲线</button></p>}
    {ready && !points.length && <p className="results-hint">本次回测没有权益记录</p>}
    {incomplete && <p className="backtest-notice" role="status">已达到权益读取上限（10 页），曲线未完整；收益指标仍来自服务端完整报告。</p>}
  </section>
}
