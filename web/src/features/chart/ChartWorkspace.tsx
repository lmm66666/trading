import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { queryChart, type ChartResult, type PriceView, type Timeframe } from '../../api/client'
import { IndicatorManager } from '../indicators/IndicatorManager'
import { mergeBars, mergeSeries } from './chartData'
import { FinancialChart } from './FinancialChart'
import type { BoardController } from './useBoards'
import { BoardToolbar } from './BoardToolbar'
import { useComparison } from './useComparison'
import { comparisonOptions, type BoardConfig } from './boards'

interface ChartWorkspaceProps {
  instrument: string
  /** 看板控制器（由 App 持有，保证非 chart 视图下看板状态仍加载与保活）。 */
  board: BoardController
  /** 当前证券是否已在自选清单 */
  watched: boolean
  onToggleWatch: () => void
  onStateChange: (timeframe: Timeframe, priceView: PriceView) => void
  query?: typeof queryChart
}

export function ChartWorkspace({
  instrument,
  board,
  watched,
  onToggleWatch,
  onStateChange,
  query = queryChart,
}: ChartWorkspaceProps) {
  const captureLayout = useRef<(() => Pick<BoardConfig, 'paneWeights' | 'visibleBars'>) | null>(null)
  const registerCapture = useCallback((capture: typeof captureLayout.current) => {
    captureLayout.current = capture
  }, [])
  const config = board.config
  const timeframe = config?.timeframe
  const priceView = config?.priceView
  const indicators = config?.indicators
  const [result, setResult] = useState<ChartResult | null>(null)
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const stateChangeRef = useRef(onStateChange)
  stateChangeRef.current = onStateChange
  useEffect(() => {
    if (!timeframe || !priceView) return
    stateChangeRef.current(timeframe, priceView)
  }, [timeframe, priceView, instrument])
  const generationRef = useRef(0)
  const pageControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    if (!timeframe || !priceView || !indicators) return
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
    board.setConfig((c) => ({ ...c, timeframe: value }))
  }
  const changePriceView = (value: PriceView) => {
    board.setConfig((c) => ({ ...c, priceView: value }))
  }

  const loadMore = useCallback(() => {
    if (!timeframe || !priceView || !indicators) return
    if (!result?.has_more || !result.next_before || loadingMore) return
    const generation = generationRef.current
    const controller = new AbortController()
    pageControllerRef.current?.abort()
    pageControllerRef.current = controller
    setLoadingMore(true)
    query(
      {
        instrument,
        timeframe,
        price_view: priceView,
        before: result.next_before,
        limit: 400,
        data_version: result.data_version,
        indicators,
      },
      controller.signal,
    )
      .then((older) => {
        if (controller.signal.aborted || generation !== generationRef.current) return
        setResult((current) => {
          if (
            !current ||
            older.instrument.instrument !== instrument ||
            older.timeframe !== timeframe ||
            older.price_view !== priceView ||
            older.data_version !== current.data_version
          )
            return current
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

  const comparison = useComparison(config?.comparison ?? null, result, timeframe ?? 'DAY')

  const quote = useMemo(() => {
    const latest = result?.bars.at(-1)
    if (!latest) return null
    const previousClose = result?.bars.at(-2)?.close
    const change = previousClose !== undefined && previousClose > 0 ? latest.close - previousClose : null
    const changePercent = change !== null && previousClose !== undefined ? (change / previousClose) * 100 : null
    return { latest, change, changePercent }
  }, [result])

  if (!config) return null

  return (
    <main className="chart-workspace">
      <header className="chart-header">
        <div className="security-title">
          <div className="security-heading">
            <div className="security-name">
              <h2>{result?.instrument.name ?? '正在读取证券信息'}</h2>
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
              <div className={`chart-quote ${quote.change === null || quote.change === 0 ? 'quote-flat' : quote.change > 0 ? 'quote-up' : 'quote-down'}`} aria-label="最新行情">
                <strong>{quote.latest.close.toFixed(2)}</strong>
                <span>
                  <small>{config.timeframe === 'DAY' ? '日涨跌' : '周涨跌'}</small>{' '}
                  {quote.change !== null && quote.changePercent !== null ? (
                    <>{quote.change > 0 ? '+' : ''}{quote.change.toFixed(2)} ({quote.change > 0 ? '+' : ''}{quote.changePercent.toFixed(2)}%)</>
                  ) : '—'}
                </span>
              </div>
            )}
          </div>
          <div className="security-meta">
            <p>
              {result?.instrument.code ?? instrument.split(':')[1]} · {result?.instrument.exchange ?? instrument.split(':')[0]} · {config.timeframe === 'DAY' ? '日线' : '周线'} ·{' '}
              {config.priceView === 'QFQ' ? '前复权' : '不复权'}
            </p>
            <span className="version-chip">快照 v{result?.data_version ?? '—'}</span>
          </div>
        </div>
      </header>
      <nav className="chart-toolbar" aria-label="图表工具栏">
        <div className="segmented-control" aria-label="周期">
          <button
            aria-label="日线"
            aria-pressed={config.timeframe === 'DAY'}
            onClick={() => changeTimeframe('DAY')}
            type="button"
          >
            日
          </button>
          <button
            aria-label="周线"
            aria-pressed={config.timeframe === 'WEEK'}
            onClick={() => changeTimeframe('WEEK')}
            type="button"
          >
            周
          </button>
        </div>
        <div className="toolbar-divider" />
        <div className="segmented-control price-view" aria-label="复权方式">
          <button
            aria-pressed={config.priceView === 'QFQ'}
            onClick={() => changePriceView('QFQ')}
            type="button"
          >
            前复权
          </button>
          <button
            aria-pressed={config.priceView === 'RAW'}
            onClick={() => changePriceView('RAW')}
            type="button"
          >
            不复权
          </button>
        </div>
        <div className="toolbar-divider" />
        <IndicatorManager
          indicators={config.indicators}
          onChange={(next) => board.setConfig((c) => ({ ...c, indicators: next }))}
        />
        <BoardToolbar board={board} instrument={instrument} captureLayout={() => captureLayout.current?.()} />
      </nav>
      <section className="chart-stage" aria-live="polite">
        {status === 'loading' && (
          <div className="chart-loading">
            <span className="loading-orbit" />
            <strong>正在构建行情图</strong>
            <span>读取固定版本行情与指标</span>
          </div>
        )}
        {status === 'error' && (
          <div className="chart-error">
            <strong>行情暂时无法加载</strong>
            <span>{error}</span>
          </div>
        )}
        {status === 'ready' && result && result.bars.length === 0 && (
          <div className="chart-error">
            <strong>暂无行情数据</strong>
            <span>该证券在当前周期没有可展示的 K 线。</span>
          </div>
        )}
        {status === 'ready' && result && result.bars.length > 0 && (
          <FinancialChart
            key={`${instrument}:${config.timeframe}:${config.priceView}:${config.comparison}`}
            bars={result.bars}
            onLoadMore={loadMore}
            series={result.series}
            comparison={comparison.bars.length ? comparison.bars : undefined}
            comparisonLabel={comparisonOptions.find(([id]) => id === config.comparison)?.[1]}
            timeframe={config.timeframe}
            config={config}
            onCaptureLayout={registerCapture}
            onLayoutChange={(layout) => board.setConfig((c) => ({ ...c, ...layout }))}
          />
        )}
        {config.comparison && (comparison.loading || comparison.error) && (
          <div className="comparison-notice" role="status">
            {comparison.loading ? '正在加载关联行情…' : comparison.error}
            {comparison.error && (
              <button onClick={comparison.retry} type="button">
                重试关联行情
              </button>
            )}
          </div>
        )}
        {error && status === 'ready' && <div className="chart-toast">{error}</div>}
      </section>
    </main>
  )
}
