import { useEffect, useRef, useState } from 'react'
import {
  getRefreshStatus,
  triggerMarketRefresh,
  listRefreshRuns,
  getRefreshRun,
  listRefreshFailures,
  type RefreshRun,
  type RefreshStatus,
  type RefreshFailure,
} from '../../api/client'
import './updates.css'

const active = (run: RefreshRun | null) =>
  run?.state === 'RUNNING' || run?.state === 'PREPARING'
const stale = (run: RefreshRun | null) =>
  !!run && active(run) && Date.now() - Date.parse(run.heartbeat_at) > 60000
const labels: Record<string, string> = {
  PREPARING: '正在准备',
  RUNNING: '更新中',
  SUCCEEDED: '全部成功',
  PARTIAL_SUCCEEDED: '部分成功',
  FAILED: '失败',
  INTERRUPTED: '已中断',
}
const date = (value: string | null) =>
  value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const percent = (run: RefreshRun) =>
  run.total
    ? `${Math.floor(((run.succeeded + run.failed) * 100) / run.total)}%`
    : '正在准备'

function RunSummary({ run }: { run: RefreshRun }) {
  const minutes = Math.max(
    0,
    Math.floor(
      ((run.finished_at ? Date.parse(run.finished_at) : Date.now()) -
        Date.parse(run.started_at)) /
        60000,
    ),
  )
  const stale = active(run) && Date.now() - Date.parse(run.heartbeat_at) > 60000
  return (
    <div className="update-summary">
      <p>
        <strong>{labels[run.state] ?? '状态未知'}</strong> ·{' '}
        {run.trigger === 'MANUAL' ? '手动触发' : '定时触发'}
      </p>
      {run.total === null ? (
        <p>正在准备证券范围</p>
      ) : run.total === 0 ? (
        <p>无更新对象</p>
      ) : (
        <>
          <progress
            aria-label={`${run.kind === 'STOCK' ? '股票' : '期货'}更新进度`}
            max={run.total}
            value={run.succeeded + run.failed}
          />
          <p>
            已处理 {run.succeeded + run.failed} / {run.total}
          </p>
          <p>
            成功 {run.succeeded} · 失败 {run.failed} · 待完成{' '}
            {run.total - run.succeeded - run.failed}
          </p>
        </>
      )}
      <dl>
        <dt>开始时间</dt>
        <dd>{date(run.started_at)}</dd>
        <dt>已用时间</dt>
        <dd>
          {Math.floor(minutes / 60)} 小时 {minutes % 60} 分
        </dd>
        <dt>最近进展</dt>
        <dd>{date(run.last_progress_at)}</dd>
        <dt>记录时间</dt>
        <dd>{date(run.snapshot_at)}</dd>
      </dl>
      {run.state === 'INTERRUPTED' && (
        <p className="update-warning">
          最后记录进度。已保存行情保留，重新更新不会按此记录跳过证券。
        </p>
      )}
      {stale && (
        <p className="update-warning" role="status">
          进度记录已过期，运行状态待确认。
        </p>
      )}
    </div>
  )
}

function RefreshDetails({ id }: { id: string }) {
  const [run, setRun] = useState<RefreshRun | null>(null)
  const [error, setError] = useState('')
  const [, advanceClock] = useState(0)
  const [retry, setRetry] = useState(0)
  const [after, setAfter] = useState(0)
  const [failures, setFailures] = useState<RefreshFailure[]>([])
  const [failureError, setFailureError] = useState('')
  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout>
    let request: AbortController | null = null
    const load = async () => {
      if (document.hidden || stopped) return
      request?.abort()
      const controller = new AbortController()
      request = controller
      let delay = 30000
      try {
        const next = await getRefreshRun(id, controller.signal)
        if (stopped || controller.signal.aborted) return
        setRun(next)
        setError('')
        if (active(next)) delay = 5000
        else return
      } catch {
        if (stopped || controller.signal.aborted) return
        setError('进度暂不可用，任务是否仍在运行待确认。')
      }
      advanceClock((v) => v + 1)
      timer = setTimeout(load, delay)
    }
    const visibility = () => {
      clearTimeout(timer)
      request?.abort()
      if (!document.hidden) void load()
    }
    void load()
    document.addEventListener('visibilitychange', visibility)
    return () => {
      stopped = true
      clearTimeout(timer)
      request?.abort()
      document.removeEventListener('visibilitychange', visibility)
    }
  }, [id, retry])
  useEffect(() => {
    const controller = new AbortController()
    setFailureError('')
    setFailures([])
    if (!run?.failed) return () => controller.abort()
    listRefreshFailures(id, after, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setFailures(result.items)
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailureError('失败明细暂不可用')
      })
    return () => controller.abort()
  }, [id, after, run?.failed, retry])
  return (
    <section className="update-details" aria-label="任务详情">
      <h3>任务详情</h3>
      <small>{id}</small>
      {error && (
        <p role="alert">
          {error}{' '}
          <button type="button" onClick={() => setRetry((v) => v + 1)}>
            重试查询
          </button>
        </p>
      )}
      {run && <RunSummary run={run} />}
      <h4>失败明细</h4>
      {failureError ? (
        <p role="alert">
          {failureError}{' '}
          <button type="button" onClick={() => setRetry((v) => v + 1)}>
            重试明细
          </button>
        </p>
      ) : failures.length ? (
        <ul>
          {failures.map((f) => (
            <li key={f.id}>
              {f.name || f.code} · {f.exchange}:{f.code} · 刷新失败 ·{' '}
              {date(f.completed_at)}
            </li>
          ))}
        </ul>
      ) : (
        <p>暂无已记录的失败明细</p>
      )}
      <div className="update-actions">
        <button
          type="button"
          disabled={after === 0}
          onClick={() => setAfter(0)}
        >
          明细首页
        </button>
        <button
          type="button"
          disabled={failures.length < 50}
          onClick={() => setAfter(failures[failures.length - 1].id)}
        >
          下一页明细
        </button>
      </div>
    </section>
  )
}

function RefreshHistory({
  onSelect,
  pulse,
}: {
  onSelect: (id: string) => void
  pulse: string
}) {
  const [before, setBefore] = useState(0)
  const [runs, setRuns] = useState<RefreshRun[]>([])
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  const [loading, setLoading] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    listRefreshRuns(before, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setRuns(result.items)
      })
      .catch(() => {
        if (!controller.signal.aborted) setError('更新历史暂不可用')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [before, retry, pulse])
  return (
    <section className="update-history">
      <h3>更新历史</h3>
      {error && (
        <p role="alert">
          {error}{' '}
          <button type="button" onClick={() => setRetry((v) => v + 1)}>
            重试历史
          </button>
        </p>
      )}
      {loading ? (
        <p>正在读取历史…</p>
      ) : runs.length ? (
        <ul>
          {runs.map((run) => (
            <li key={run.run_id}>
              <button
                type="button"
                aria-label={`查看任务 ${run.run_id}`}
                onClick={() => onSelect(run.run_id)}
              >
                {run.kind === 'STOCK' ? '股票' : '期货'} ·{' '}
                {date(run.started_at)} · {labels[run.state] ?? '状态未知'}
              </button>
            </li>
          ))}
        </ul>
      ) : (
        !error && <p>暂无更新记录</p>
      )}
      <div className="update-actions">
        <button
          type="button"
          disabled={loading || before === 0}
          onClick={() => setBefore(0)}
        >
          最近记录
        </button>
        <button
          type="button"
          disabled={loading || runs.length < 20}
          onClick={() => setBefore(runs[runs.length - 1].id)}
        >
          更早记录
        </button>
      </div>
    </section>
  )
}

export function RefreshMonitor({ onReload }: { onReload: () => void }) {
  const [open, setOpen] = useState(false)
  const [status, setStatus] = useState<RefreshStatus | null>(null)
  const [error, setError] = useState('')
  const [, advanceClock] = useState(0)
  const [pulse, setPulse] = useState(0)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const close = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (open) close.current?.focus()
  }, [open])
  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout>
    let request: AbortController | null = null
    const load = async () => {
      if (document.hidden || stopped) return
      request?.abort()
      const controller = new AbortController()
      request = controller
      let delay = 30000
      try {
        const next = await getRefreshStatus(controller.signal)
        if (stopped || controller.signal.aborted) return
        setStatus(next)
        setError('')
        if (open && (active(next.stock) || active(next.futures))) delay = 5000
      } catch {
        if (stopped || controller.signal.aborted) return
        setError('无法获取最新进度，以下为最后记录，运行状态待确认。')
      }
      advanceClock((v) => v + 1)
      timer = setTimeout(load, delay)
    }
    const visibility = () => {
      clearTimeout(timer)
      request?.abort()
      if (!document.hidden) void load()
    }
    void load()
    document.addEventListener('visibilitychange', visibility)
    return () => {
      stopped = true
      clearTimeout(timer)
      request?.abort()
      document.removeEventListener('visibilitychange', visibility)
    }
  }, [open, pulse])
  const dismiss = () => {
    setOpen(false)
    trigger.current?.focus()
  }
  const submit = async () => {
    setBusy(true)
    setNotice('')
    try {
      const receipt = await triggerMarketRefresh()
      setSelected(receipt.run_id)
      setNotice(
        receipt.progress_available
          ? '更新已受理，可关闭页面，NAS 会继续执行。'
          : '更新已受理，进度记录暂不可用，请勿重复提交。',
      )
    } catch {
      setNotice(
        '提交结果未确认，正在查询当前任务。不会自动重新提交；发现的运行中任务也可能来自定时更新。',
      )
    } finally {
      setBusy(false)
      setPulse((v) => v + 1)
    }
  }
  const running = active(status?.stock ?? null)
    ? status?.stock
    : active(status?.futures ?? null)
      ? status?.futures
      : null
  return (
    <>
      <button
        ref={trigger}
        type="button"
        className="update-entry"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        数据更新
        {error
          ? ' · 状态待确认'
          : running
            ? ` · ${running.kind === 'STOCK' ? '股票' : '期货'} ${percent(running)}`
            : ''}
      </button>
      {open && (
        <section
          className="update-panel"
          role="dialog"
          aria-label="数据更新"
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              event.stopPropagation()
              dismiss()
            }
          }}
        >
          <div className="update-heading">
            <h2>数据更新</h2>
            <button
              ref={close}
              type="button"
              onClick={dismiss}
              aria-label="关闭数据更新"
            >
              关闭
            </button>
          </div>
          <p className="update-hint">
            任务在 NAS 执行，关闭页面或电脑不会停止更新。
          </p>
          {error && (
            <p role="alert" className="update-warning">
              {error}{' '}
              <button type="button" onClick={() => setPulse((v) => v + 1)}>
                刷新进度
              </button>
            </p>
          )}
          {notice && (
            <p role="status" className="update-notice">
              {notice}
            </p>
          )}
          <div className="update-grid">
            <section className="update-card">
              <h3>股票行情</h3>
              <button
                type="button"
                className="update-primary"
                disabled={
                  busy ||
                  (!status && !error) ||
                  (active(status?.stock ?? null) &&
                    !stale(status?.stock ?? null))
                }
                onClick={() => void submit()}
              >
                {busy ? '正在提交…' : '更新全部股票'}
              </button>
              {status?.stock ? (
                <>
                  <RunSummary run={status.stock} />
                  <button
                    type="button"
                    onClick={() => setSelected(status.stock!.run_id)}
                  >
                    查看股票详情
                  </button>
                </>
              ) : (
                <p>{status ? '暂无更新记录' : '正在读取进度…'}</p>
              )}
            </section>
            <section className="update-card">
              <h3>期货行情</h3>
              <p>
                {status?.futures_enabled === false
                  ? '未启用'
                  : status?.futures_enabled === true
                    ? '按计划定时更新'
                    : '启用状态未知'}
              </p>
              {status?.futures && (
                <>
                  <RunSummary run={status.futures} />
                  <button
                    type="button"
                    onClick={() => setSelected(status.futures!.run_id)}
                  >
                    查看期货详情
                  </button>
                </>
              )}
            </section>
          </div>
          {status &&
            [status.stock, status.futures].some(
              (run) => run && !active(run),
            ) && (
              <p className="update-notice">
                更新记录已结束，当前图表保持原有数据版本。
                <button type="button" onClick={onReload}>
                  加载最新行情
                </button>
              </p>
            )}
          {selected && <RefreshDetails key={selected} id={selected} />}
          <RefreshHistory
            onSelect={setSelected}
            pulse={`${pulse}:${status?.stock?.run_id}:${status?.stock?.state}:${status?.futures?.run_id}:${status?.futures?.state}`}
          />
        </section>
      )}
    </>
  )
}
