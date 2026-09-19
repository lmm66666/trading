import { useState } from 'react'
import { cancelRun, type RunKind, type RunStatus } from '../../api/client'

const STATUS_LABELS: Record<RunStatus['status'], string> = {
  PENDING: '排队中',
  RUNNING: '运行中',
  SUCCEEDED: '已完成',
  PARTIAL_SUCCEEDED: '部分成功',
  FAILED: '失败',
  CANCELLED: '已取消',
}

interface RunMonitorProps {
  kind: RunKind
  status: RunStatus | null
  /** 轮询进入错误态（连续网络错误）时的提示 */
  pollingError: string | null
}

/** 任务状态条：状态徽标、数据版本、尝试次数与取消按钮 */
export function RunMonitor({ kind, status, pollingError }: RunMonitorProps) {
  const [cancelling, setCancelling] = useState(false)
  const [cancelError, setCancelError] = useState<string | null>(null)

  if (!status && !pollingError) return null

  const cancellable = status?.status === 'PENDING' || status?.status === 'RUNNING'

  const requestCancel = async () => {
    if (!status || cancelling) return
    setCancelling(true)
    setCancelError(null)
    try {
      await cancelRun(kind, status.run_id)
    } catch (cause) {
      setCancelError(cause instanceof Error ? cause.message : '取消请求失败')
    } finally {
      setCancelling(false)
    }
  }

  return (
    <section className="run-monitor" aria-label="任务状态">
      <span className={`run-badge ${status ? status.status.toLowerCase() : 'error'}`}>
        {status ? STATUS_LABELS[status.status] : '轮询异常'}
      </span>
      {status && (
        <span className="run-meta">
          {status.run_id}
          {' · 数据版本 '}{status.data_version}
          {' · 尝试 '}{status.attempts} 次
          {status.cancel_requested_at ? ' · 已请求取消' : ''}
        </span>
      )}
      {cancellable && (
        <button className="cancel-task" onClick={requestCancel} disabled={cancelling} type="button">
          {cancelling ? '取消中…' : '取消任务'}
        </button>
      )}
      {(cancelError ?? pollingError) && <span className="run-error">{cancelError ?? pollingError}</span>}
    </section>
  )
}
