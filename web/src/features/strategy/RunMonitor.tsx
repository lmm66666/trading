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

/** 任务状态条：状态徽标 + 策略版本；run_id 等技术信息收进可展开的「任务详情」 */
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
      <div className="run-head">
        <span className={`run-badge ${status ? status.status.toLowerCase() : 'error'}`}>
          {status ? STATUS_LABELS[status.status] : '轮询异常'}
        </span>
        {status && (
          <span className="run-strategy">
            {status.strategy} v{status.strategy_version}
          </span>
        )}
        {cancellable && (
          <button className="cancel-task" onClick={requestCancel} disabled={cancelling} type="button">
            {cancelling ? '取消中…' : '取消任务'}
          </button>
        )}
        {(cancelError ?? pollingError) && <span className="run-error">{cancelError ?? pollingError}</span>}
      </div>
      {status && (
        <details className="run-details">
          <summary>任务详情</summary>
          <dl className="run-meta">
            <div>
              <dt>run_id</dt>
              <dd>{status.run_id}</dd>
            </div>
            <div>
              <dt>数据版本</dt>
              <dd>{status.data_version}</dd>
            </div>
            <div>
              <dt>尝试次数</dt>
              <dd>{status.attempts}</dd>
            </div>
            {status.cancel_requested_at && (
              <div>
                <dt>取消标记</dt>
                <dd>已请求取消</dd>
              </div>
            )}
          </dl>
        </details>
      )}
    </section>
  )
}
