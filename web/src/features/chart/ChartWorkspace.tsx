import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  queryChart,
  type ChartResult,
  type IndicatorRequest,
  type PriceView,
  type Timeframe,
} from '../../api/client'
import { IndicatorManager } from '../indicators/IndicatorManager'
import { defaultIndicators, mergeBars, mergeSeries } from './chartData'
import { FinancialChart } from './FinancialChart'

interface ChartWorkspaceProps {
  instrument: string
  initialTimeframe: Timeframe
  initialPriceView: PriceView
  /** 当前证券是否已在自选清单 */
  watched: boolean
  onToggleWatch: () => void
  onStateChange: (timeframe: Timeframe, priceView: PriceView) => void
  query?: typeof queryChart
}

export function ChartWorkspace({
  instrument,
  initialTimeframe,
  initialPriceView,
  watched,
  onToggleWatch,
  onStateChange,
  query = queryChart,
}: ChartWorkspaceProps) {
  const [timeframe, setTimeframe] = useState(initialTimeframe)
  const [priceView, setPriceView] = useState(initialPriceView)
  const [indicators, setIndicators] = useState<IndicatorRequest[]>(defaultIndicators)
  const [result, setResult] = useState<ChartResult | null>(null)
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const generationRef = useRef(0)
  const pageControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    setTimeframe(initialTimeframe)
    setPriceView(initialPriceView)
  }, [initialPriceView, initialTimeframe])

  useEffect(() => {
    const generation = ++generationRef.current
    pageControllerRef.current?.abort()
    const controller = new AbortController()
    setStatus('loading')
    setLoadingMore(false)
    setError('')
    setResult(null)
    query({ instrument, timeframe, price_view: priceView, limit: 400, indicators }, controller.signal)
      .then((data) => {
        if (controller.signal.aborted || generation !== generationRef.current) return
        setResult(data)
        setStatus('ready')
      })
      .catch((reason: unknown) => {
        if (controller.signal.aborted || generation !== generationRef.current) return
        setError(reason instanceof Error ? reason.message : '加载行情失败')
        setStatus('error')
      })
    return () => {
      controller.abort()
      pageControllerRef.current?.abort()
    }
  }, [indicators, instrument, priceView, query, timeframe])

  const changeTimeframe = (value: Timeframe) => {
    setTimeframe(value)
    onStateChange(value, priceView)
  }
  const changePriceView = (value: PriceView) => {
    setPriceView(value)
    onStateChange(timeframe, value)
  }

  const loadMore = useCallback(() => {
    if (!result?.has_more || !result.next_before || loadingMore) return
    const generation = generationRef.current
    const controller = new AbortController()
    pageControllerRef.current?.abort()
    pageControllerRef.current = controller
    setLoadingMore(true)
    query({
      instrument,
      timeframe,
      price_view: priceView,
      before: result.next_before,
      limit: 400,
      data_version: result.data_version,
      indicators,
    }, controller.signal)
      .then((older) => {
        if (controller.signal.aborted || generation !== generationRef.current) return
        setResult((current) => {
          if (!current || older.instrument.instrument !== instrument || older.timeframe !== timeframe || older.price_view !== priceView || older.data_version !== current.data_version) return current
          return {
            ...current,
            bars: mergeBars(current.bars, older.bars),
            series: mergeSeries(current.series, older.series),
            has_more: older.has_more,
            next_before: older.next_before,
          }
        })
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted && generation === generationRef.current) {
          setError(reason instanceof Error ? reason.message : '加载更早行情失败')
        }
      })
      .finally(() => {
        if (generation === generationRef.current) setLoadingMore(false)
      })
  }, [indicators, instrument, loadingMore, priceView, query, result, timeframe])

  const quote = useMemo(() => {
    const latest = result?.bars.at(-1)
    if (!latest) return null
    const change = latest.close - latest.open
    const changePercent = latest.open === 0 ? 0 : change / latest.open * 100
    return { latest, change, changePercent }
  }, [result])

  return (
    <main className="chart-workspace">
      <header className="chart-header">
        <div className="security-title">
          <span className="security-code">{result?.instrument.code ?? instrument.split(':')[1]}</span>
          <div>
            <h2>{result?.instrument.name ?? '正在读取证券信息'}</h2>
            <p>{instrument} · {timeframe === 'DAY' ? '日线' : '周线'} · {priceView === 'QFQ' ? '前复权' : '不复权'}</p>
          </div>
          <button
            aria-label={watched ? '移除自选' : '添加自选'}
            aria-pressed={watched}
            className={watched ? 'watch-toggle active' : 'watch-toggle'}
            onClick={onToggleWatch}
            title={watched ? '移除自选' : '添加自选'}
            type="button"
          >
            {watched ? '★' : '☆'}
          </button>
        </div>
        {quote && (
          <div className={quote.change >= 0 ? 'quote-up' : 'quote-down'} aria-label="最新行情">
            <strong>{quote.latest.close.toFixed(2)}</strong>
            <span>{quote.change >= 0 ? '+' : ''}{quote.change.toFixed(2)} · {quote.changePercent >= 0 ? '+' : ''}{quote.changePercent.toFixed(2)}%</span>
          </div>
        )}
      </header>
      <nav className="chart-toolbar" aria-label="图表工具栏">
        <div className="segmented-control" aria-label="周期">
          <button aria-label="日线" aria-pressed={timeframe === 'DAY'} onClick={() => changeTimeframe('DAY')} type="button">日</button>
          <button aria-label="周线" aria-pressed={timeframe === 'WEEK'} onClick={() => changeTimeframe('WEEK')} type="button">周</button>
        </div>
        <div className="toolbar-divider" />
        <div className="segmented-control price-view" aria-label="复权方式">
          <button aria-pressed={priceView === 'QFQ'} onClick={() => changePriceView('QFQ')} type="button">前复权</button>
          <button aria-pressed={priceView === 'RAW'} onClick={() => changePriceView('RAW')} type="button">不复权</button>
        </div>
        <div className="toolbar-divider" />
        <IndicatorManager indicators={indicators} onChange={setIndicators} />
        <span className="version-chip">快照 v{result?.data_version ?? '—'}</span>
      </nav>
      <section className="chart-stage" aria-live="polite">
        {status === 'loading' && (
          <div className="chart-loading"><span className="loading-orbit" /><strong>正在构建行情图</strong><span>读取固定版本行情与指标</span></div>
        )}
        {status === 'error' && (
          <div className="chart-error"><strong>行情暂时无法加载</strong><span>{error}</span></div>
        )}
        {status === 'ready' && result && result.bars.length === 0 && (
          <div className="chart-error"><strong>暂无行情数据</strong><span>该证券在当前周期没有可展示的 K 线。</span></div>
        )}
        {status === 'ready' && result && result.bars.length > 0 && (
          <>
            <FinancialChart bars={result.bars} onLoadMore={loadMore} series={result.series} />
            {result.has_more && (
              <button className="load-more" disabled={loadingMore} onClick={loadMore} type="button">
                {loadingMore ? '正在加载…' : '加载更早行情'}
              </button>
            )}
          </>
        )}
        {error && status === 'ready' && <div className="chart-toast">{error}</div>}
      </section>
    </main>
  )
}
