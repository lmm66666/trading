import { useCallback, useEffect, useState } from 'react'
import { ApiError, getRun, type RunKind, type RunStatus } from '../../api/client'

const TERMINAL = new Set(['SUCCEEDED', 'PARTIAL_SUCCEEDED', 'FAILED', 'CANCELLED'])
interface PollState {
  key: string
  status: RunStatus | null
  error: string | null
  missing: boolean
}
interface PollOptions {
  active?: boolean
  /** 扫描启用错误分类；默认保持回测的既有重试语义。 */
  classifyErrors?: boolean
}

export function useRunPolling(kind: RunKind, runId: string | null, { active = true, classifyErrors = false }: PollOptions = {}) {
  const key = `${kind}:${runId ?? ''}`
  const [state, setState] = useState<PollState | null>(null)
  const [attempt, setAttempt] = useState(0)
  const retry = useCallback(() => setAttempt((value) => value + 1), [])

  useEffect(() => {
    if (!runId || !active) return
    let current = true
    let timer: number | undefined
    let errors = 0
    setState((previous) => ({ key, status: previous?.key === key ? previous.status : null, error: null, missing: false }))
    const poll = async () => {
      try {
        const status = await getRun(kind, runId)
        if (!current) return
        errors = 0
        setState({ key, status, error: null, missing: false })
        if (TERMINAL.has(status.status)) return
      } catch (cause) {
        if (!current) return
        errors += 1
        const businessError = classifyErrors && cause instanceof ApiError && cause.status < 500 && cause.status !== 429
        if (businessError || errors >= 3) {
          const missing = businessError && cause.status === 404 && cause.code === 'NOT_FOUND'
          setState((previous) => ({ key, status: previous?.key === key ? previous.status : null,
            error: missing ? '上次扫描记录已不可用' : cause instanceof Error ? cause.message : '任务状态读取失败', missing }))
          return
        }
      }
      timer = window.setTimeout(poll, 2000)
    }
    void poll()
    return () => {
      current = false
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [kind, runId, key, active, attempt, classifyErrors])

  const current = runId && state?.key === key ? state : null
  return { status: current?.status ?? null, error: current?.error ?? null, missing: current?.missing ?? false,
    loading: Boolean(runId && !current?.status && !current?.error), retry }
}
