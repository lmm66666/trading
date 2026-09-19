import { useEffect, useState } from 'react'
import { listStrategies, type StrategyDefinition } from '../../api/client'

export interface StrategyCatalogState {
  definitions: StrategyDefinition[]
  loading: boolean
  error: string | null
}

/** 加载服务端策略目录，供扫描与回测面板共用 */
export function useStrategyCatalog(): StrategyCatalogState {
  const [state, setState] = useState<StrategyCatalogState>({ definitions: [], loading: true, error: null })

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
  }, [])

  return state
}
