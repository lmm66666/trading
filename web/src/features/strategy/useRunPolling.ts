import { useEffect, useRef, useState } from 'react'
import { getRun, type RunKind, type RunStatus, type RunStatusValue } from '../../api/client'

const POLL_INTERVAL_MS = 2000
const MAX_CONSECUTIVE_ERRORS = 3

const TERMINAL_STATUSES: readonly RunStatusValue[] = [
  'SUCCEEDED',
  'PARTIAL_SUCCEEDED',
  'FAILED',
  'CANCELLED',
]

export interface RunPollingResult {
  status: RunStatus | null
  /** 连续网络错误达到上限后进入错误态，轮询停止 */
  error: string | null
}

/**
 * 以 2 秒固定间隔轮询任务状态：终态停止、卸载停止、连续 3 次网络错误停止。
 * 用世代 ref 防止旧任务的响应污染新任务的状态。
 */
export function useRunPolling(kind: RunKind, runId: string | null): RunPollingResult {
  const [status, setStatus] = useState<RunStatus | null>(null)
  const [error, setError] = useState<string | null>(null)
  const generationRef = useRef(0)

  useEffect(() => {
    if (!runId) {
      setStatus(null)
      setError(null)
      return
    }
    const generation = ++generationRef.current
    setStatus(null)
    setError(null)
    let timer: number | undefined
    let active = true
    let errorCount = 0

    const poll = async () => {
      if (!active || generationRef.current !== generation) return
      try {
        const result = await getRun(kind, runId)
        if (!active || generationRef.current !== generation) return
        errorCount = 0
        setStatus(result)
        if (TERMINAL_STATUSES.includes(result.status)) return
        timer = window.setTimeout(poll, POLL_INTERVAL_MS)
      } catch (cause) {
        if (!active || generationRef.current !== generation) return
        errorCount += 1
        if (errorCount >= MAX_CONSECUTIVE_ERRORS) {
          setError(cause instanceof Error ? cause.message : '任务状态轮询失败')
          return
        }
        timer = window.setTimeout(poll, POLL_INTERVAL_MS)
      }
    }

    void poll()
    return () => {
      active = false
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [kind, runId])

  return { status, error }
}
