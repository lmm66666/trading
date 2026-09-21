import { useCallback, useEffect, useState } from 'react'
import { listStrategies, type StrategyDefinition } from '../../api/client'

export interface StrategyCatalogState {
  definitions: StrategyDefinition[]
  loading: boolean
  error: string | null
  retry: () => void
}

/** 加载服务端策略目录，供扫描与回测面板共用 */
export function useStrategyCatalog(): StrategyCatalogState {
  const [state, setState] = useState<Omit<StrategyCatalogState, 'retry'>>({ definitions: [], loading: true, error: null })

  const [attempt, setAttempt] = useState(0)
  const retry = useCallback(() => setAttempt((value) => value + 1), [])

  useEffect(() => {
    let cancelled = false
    setState({ definitions: [], loading: true, error: null })
    listStrategies()
      .then((definitions) => {
        if (!cancelled) setState({ definitions, loading: false, error: null })
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setState({
            definitions: [],
            loading: false,
            error: error instanceof Error ? error.message : '策略目录加载失败',
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [attempt])

  return { ...state, retry }
}
