import { strategyName } from '../strategy/strategyLabels'
import { useEffect, useRef, useState } from 'react'
import {
  fetchSnapshotPage,
  type SnapshotPage,
} from '../../api/client'
import type { RunStatus } from '../../api/client'
import { RunMonitor } from '../strategy/RunMonitor'
import { describeScan, sameDraft, type ScanDraft, type ScanContext } from './scanPreferences'

const PAGE_LIMIT = 100

interface ScanResultsProps {
  /** 终态且带 snapshot_id 的扫描任务 */
  run: RunStatus
  onSelectInstrument: (instrument: string) => void
  context?: ScanContext | null
  draft?: ScanDraft
  active?: boolean
}

function formatLocalTime(utc: string): string {
  return new Date(utc).toLocaleString('zh-CN', { hour12: false })
}

/** 扫描结果卡：卡头为任务状态条，卡体按快照 key 分页读取入选行，failures 折叠展示 */
export function ScanResults({ run, onSelectInstrument, context = null, draft, active = true }: ScanResultsProps) {
  const [displayed, setDisplayed] = useState<{ run: RunStatus; context: ScanContext | null; page: SnapshotPage } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [retry, setRetry] = useState(0)
  const [pageLoading, setPageLoading] = useState(false)
  const [pageError, setPageError] = useState<string | null>(null)
  const generation = useRef(0)
  const pagePending = useRef(false)
  const contextRef = useRef(context)
  contextRef.current = context

  useEffect(() => {
    const token = ++generation.current
    pagePending.current = false
    setPageLoading(false)
    if (!active) return
    if (displayed?.run.run_id === run.run_id) { setLoading(false); return }
    let current = true
    setLoading(true)
    setError(null)
    const submittedContext = contextRef.current
    fetchSnapshotPage({ strategy: run.strategy, strategy_version: run.strategy_version, snapshot_id: run.snapshot_id, limit: PAGE_LIMIT })
      .then((page) => {
        if (!current || token !== generation.current) return
        setDisplayed({ run, context: submittedContext, page })
        setPageError(null)
      })
      .catch((cause: unknown) => {
        if (current && token === generation.current) setError(cause instanceof Error ? cause.message : '扫描结果加载失败')
      })
      .finally(() => { if (current && token === generation.current) setLoading(false) })
    return () => { current = false; generation.current += 1 }
  }, [run, retry, active, displayed?.run.run_id])

  const loadMore = async () => {
    if (!displayed || pagePending.current || !active || loading || displayed.page.next_sequence == null) return
    const token = generation.current
    const { key, next_sequence } = displayed.page
    pagePending.current = true
    setPageLoading(true)
    setPageError(null)
    try {
      const page = await fetchSnapshotPage({ strategy: key.strategy_id, strategy_version: key.strategy_version,
        parameters_hash: key.parameters_hash, snapshot_id: key.snapshot_id, after_sequence: next_sequence, limit: PAGE_LIMIT })
      if (token !== generation.current) return
      setDisplayed((previous) => previous ? { ...previous, page: { ...page, rows: [...previous.page.rows, ...page.rows] } } : previous)
    } catch (cause) {
      if (token === generation.current) setPageError(cause instanceof Error ? cause.message : '扫描结果加载失败')
    } finally {
      if (token === generation.current) { pagePending.current = false; setPageLoading(false) }
    }
  }
  const rows = displayed?.page.rows ?? []
  const failures = displayed?.page.failures ?? []
  const nextSequence = displayed?.page.next_sequence ?? null
  const shownRun = displayed?.run ?? run
  const partial = shownRun.status === 'PARTIAL_SUCCEEDED'

  return (
    <section className="scan-results" aria-label="扫描结果">
      <RunMonitor kind={shownRun.kind} status={shownRun} pollingError={null} />
      <p className="scan-result-source">{strategyName(shownRun.strategy)} v{shownRun.strategy_version} · {displayed?.context ? describeScan(displayed.context) : '原始条件未保存'}</p>
      {displayed?.context && <span className="scan-source-note">条件来自本机提交记录</span>}
      {displayed?.context && draft && !sameDraft(displayed.context, draft) && <p className="scan-notice">条件已修改，以下仍为上次扫描结果</p>}
      {displayed && displayed.run.run_id !== run.run_id && <p className="scan-notice">保留上次结果，等待新扫描结果加载成功</p>}
      <div className="results-body">
        <header className="results-heading">
          <h3>{!displayed ? '结果加载中…' : nextSequence !== null ? `已加载 ${rows.length} 条` : `入选 ${rows.length} 只`}</h3>
          {partial && failures.length > 0 && (
            <span className="failures-badge">{failures.length} 只证券处理失败</span>
          )}
        </header>
        {(error || pageError) && (
          <p className="results-error">
            {error || pageError}
            {displayed ? '（已保留已加载结果）' : ''}
            {error && <button type="button" onClick={() => setRetry((value) => value + 1)}>重试读取结果</button>}
          </p>
        )}
        {loading && !displayed && (
          <div className="results-skeleton" role="status" aria-label="结果加载中">
            {[0, 1, 2].map((index) => (
              <div className="skeleton-row" key={index} aria-hidden="true">
                <span className="skeleton-bar" />
                <span className="skeleton-bar" />
                <span className="skeleton-bar" />
                <span className="skeleton-bar" />
              </div>
            ))}
          </div>
        )}
        {displayed && rows.length === 0 && (
          <div className="results-empty">
            <strong>无入选证券</strong>
            <span>可调整时间范围、策略参数或交易所范围后重新扫描</span>
          </div>
        )}
        {rows.length > 0 && (
          <div className="scan-table-scroll"><table className="results-table">
            <thead>
              <tr>
                <th>代码</th>
                <th>名称</th>
                <th>信号时间</th>
                <th>信号原因</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.instrument}>
                  <td>
                    <button
                      className="instrument-link"
                      onClick={() => onSelectInstrument(row.instrument)}
                      type="button"
                    >
                      {row.instrument}
                    </button>
                  </td>
                  <td className="col-name">{row.name ?? row.instrument}</td>
                  <td>{formatLocalTime(row.signal_time)}</td>
                  <td>{row.reason}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
        {nextSequence !== null && (
          <button className="table-load-more" onClick={loadMore} disabled={loading || pageLoading || !active} type="button">
            {pageLoading ? '加载中…' : '加载更多'}
          </button>
        )}
        {failures.length > 0 && (
          <details className="failures">
            <summary>处理失败 {failures.length} 只</summary>
            <table>
              <thead>
                <tr>
                  <th>证券</th>
                  <th>名称</th>
                  <th>失败代码</th>
                  <th>说明</th>
                  <th>可重试</th>
                </tr>
              </thead>
              <tbody>
                {failures.map((failure) => (
                  <tr key={failure.instrument}>
                    <td>{failure.instrument}</td>
                    <td>{failure.name ?? failure.instrument}</td>
                    <td>{failure.code}</td>
                    <td>{failure.message}</td>
                    <td>{failure.retryable ? '是' : '否'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </details>
        )}
      </div>
    </section>
  )
}
