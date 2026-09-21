import { useEffect, useRef, useState } from 'react'
import {
  fetchSnapshotPage,
  type SnapshotFailure,
  type SnapshotKey,
  type SnapshotRow,
} from '../../api/client'
import type { RunStatus } from '../../api/client'
import { RunMonitor } from '../strategy/RunMonitor'

const PAGE_LIMIT = 100

interface ScanResultsProps {
  /** 终态且带 snapshot_id 的扫描任务 */
  run: RunStatus
  onSelectInstrument: (instrument: string) => void
}

function formatLocalTime(utc: string): string {
  return new Date(utc).toLocaleString('zh-CN', { hour12: false })
}

/** 扫描结果卡：卡头为任务状态条，卡体按快照 key 分页读取入选行，failures 折叠展示 */
export function ScanResults({ run, onSelectInstrument }: ScanResultsProps) {
  const [rows, setRows] = useState<SnapshotRow[]>([])
  const [failures, setFailures] = useState<SnapshotFailure[]>([])
  const [key, setKey] = useState<SnapshotKey | null>(null)
  const [nextSequence, setNextSequence] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const runIdRef = useRef(run.run_id)
  runIdRef.current = run.run_id

  useEffect(() => {
    let cancelled = false
    setRows([])
    setFailures([])
    setKey(null)
    setNextSequence(null)
    setError(null)
    setLoading(true)
    // 用终态响应中的 snapshot_id 精确定位快照，策略与版本取任务回显值
    fetchSnapshotPage({
      strategy: run.strategy,
      strategy_version: run.strategy_version,
      snapshot_id: run.snapshot_id,
      limit: PAGE_LIMIT,
    })
      .then((page) => {
        if (cancelled) return
        setRows(page.rows)
        setFailures(page.failures)
        setKey(page.key)
        setNextSequence(page.next_sequence ?? null)
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : '扫描结果加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [run.run_id, run.strategy, run.strategy_version, run.snapshot_id])

  const loadMore = async () => {
    if (!key || nextSequence === null || loading) return
    setLoading(true)
    setError(null)
    try {
      const page = await fetchSnapshotPage({
        strategy: key.strategy_id,
        strategy_version: key.strategy_version,
        parameters_hash: key.parameters_hash,
        snapshot_id: key.snapshot_id,
        after_sequence: nextSequence,
        limit: PAGE_LIMIT,
      })
      // 续页响应返回时若任务已切换，丢弃以免拼入新任务结果
      if (runIdRef.current !== run.run_id) return
      setRows((previous) => [...previous, ...page.rows])
      setFailures(page.failures)
      setNextSequence(page.next_sequence ?? null)
    } catch (cause) {
      // 分页失败保留已加载行，仅提示错误
      setError(cause instanceof Error ? cause.message : '扫描结果加载失败')
    } finally {
      setLoading(false)
    }
  }

  const partial = run.status === 'PARTIAL_SUCCEEDED'

  return (
    <section className="scan-results" aria-label="扫描结果">
      <RunMonitor kind={run.kind} status={run} pollingError={null} />
      <div className="results-body">
        <header className="results-heading">
          <h3>{loading && rows.length === 0 ? '结果加载中…' : `入选 ${rows.length} 只`}</h3>
          {partial && failures.length > 0 && (
            <span className="failures-badge">{failures.length} 只证券处理失败</span>
          )}
        </header>
        {error && (
          <p className="results-error">
            {error}
            {rows.length > 0 ? '（已保留已加载结果）' : ''}
          </p>
        )}
        {loading && rows.length === 0 && (
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
        {!loading && rows.length === 0 && !error && (
          <div className="results-empty">
            <strong>无入选证券</strong>
            <span>可调整时间范围、策略参数或交易所范围后重新扫描</span>
          </div>
        )}
        {rows.length > 0 && (
          <table className="results-table">
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
          </table>
        )}
        {nextSequence !== null && (
          <button className="table-load-more" onClick={loadMore} disabled={loading} type="button">
            {loading ? '加载中…' : '加载更多'}
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
