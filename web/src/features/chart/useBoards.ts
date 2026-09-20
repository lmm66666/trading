import { useCallback, useEffect, useRef, useState } from 'react'
import {
  activateChartBoard,
  createChartBoard,
  deleteChartBoard,
  listChartBoards,
  updateChartBoard,
  type BoardConfig,
  type ChartBoard,
  type ChartBoardState,
} from '../../api/client'
import { defaultBoardConfig, validConfig } from './boards'

export type BoardStatus = 'loading' | 'ready' | 'error'

/** 键排序序列化：服务端与本地对象的键序可能不同，dirty 比较需先规范化。 */
function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`
  if (value !== null && typeof value === 'object') {
    return `{${Object.entries(value as Record<string, unknown>)
      .filter(([, v]) => v !== undefined)
      .sort(([a], [b]) => (a < b ? -1 : 1))
      .map(([k, v]) => `${JSON.stringify(k)}:${canonical(v)}`)
      .join(',')}}`
  }
  return JSON.stringify(value) ?? 'null'
}

export function useBoards(initial: Partial<BoardConfig>, onSelectSymbol?: (symbol: string) => void) {
  const [status, setStatus] = useState<BoardStatus>('loading')
  const [boards, setBoards] = useState<ChartBoard[]>([])
  const [activeId, setActiveId] = useState(0)
  const [config, setConfigState] = useState<BoardConfig | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [reloadTick, setReloadTick] = useState(0)
  /** React setState 异步批处理下防双击双发的同步守卫。 */
  const busyRef = useRef(false)
  /** initial 仅在首次加载（空表建默认看板 / 初始股票回调判断）时读取，此后 URL 参数不再种子草稿。 */
  const initialRef = useRef(initial)
  const selectRef = useRef(onSelectSymbol)
  selectRef.current = onSelectSymbol
  const active = boards.find((b) => b.id === activeId) ?? null
  const dirty = config !== null && active !== null && canonical(config) !== canonical(active.config)

  useEffect(() => {
    if (!dirty) return
    const leave = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', leave)
    return () => window.removeEventListener('beforeunload', leave)
  }, [dirty])

  /** 仅替换看板列表与激活 id，保留当前草稿（重命名等不触碰 config 的操作）。 */
  const applyBoards = useCallback((state: ChartBoardState) => {
    setBoards(state.boards)
    setActiveId(state.active_id)
  }, [])

  /** 以服务端全量状态替换本地并同步草稿为激活看板的已保存配置。 */
  const syncDraft = useCallback((state: ChartBoardState) => {
    setBoards(state.boards)
    setActiveId(state.active_id)
    const next = state.boards.find((b) => b.id === state.active_id)
    setConfigState(next ? structuredClone(next.config) : null)
  }, [])

  useEffect(() => {
    let alive = true
    setStatus('loading')
    setError('')
    void (async () => {
      let state = await listChartBoards()
      if (!alive) return
      if (state.boards.length === 0) {
        state = await createChartBoard('默认看板', {
          ...defaultBoardConfig(initialRef.current.defaultSymbol ?? null),
          ...initialRef.current,
        })
        if (!alive) return
      }
      syncDraft(state)
      setStatus('ready')
      // URL 已提供股票时不触发回调，保证 URL 股票优先。
      if (initialRef.current.defaultSymbol == null) {
        const board = state.boards.find((b) => b.id === state.active_id)
        if (board?.config.defaultSymbol) selectRef.current?.(board.config.defaultSymbol)
      }
    })().catch((reason: unknown) => {
      if (!alive) return
      setError(reason instanceof Error ? reason.message : '看板加载失败')
      setStatus('error')
    })
    return () => {
      alive = false
    }
  }, [reloadTick, syncDraft])

  const setConfig = useCallback((update: (c: BoardConfig) => BoardConfig) => {
    setConfigState((c) => (c ? update(c) : c))
  }, [])

  /** busy 互斥 + 统一错误捕获；返回 null 表示被拒绝（并发或失败），否则为服务端全量状态。 */
  const execute = useCallback(
    async (action: () => Promise<ChartBoardState>): Promise<ChartBoardState | null> => {
      if (busyRef.current) return null
      busyRef.current = true
      setBusy(true)
      setError('')
      try {
        return await action()
      } catch (reason: unknown) {
        setError(reason instanceof Error ? reason.message : '操作失败，请重试')
        return null
      } finally {
        busyRef.current = false
        setBusy(false)
      }
    },
    [],
  )

  const invalidConfigError = '看板名称、参数或数量不符合要求'

  const save = useCallback(
    async (layout?: Partial<BoardConfig>): Promise<boolean> => {
      if (!config || !active) return false
      const next = { ...config, ...layout }
      if (!validConfig(next)) {
        setError(invalidConfigError)
        return false
      }
      const state = await execute(() => updateChartBoard(active.id, { config: structuredClone(next) }))
      if (!state) return false
      syncDraft(state)
      return true
    },
    [config, active, execute, syncDraft],
  )

  /** 切换激活看板；saveFirst 为真时先把当前草稿保存到服务端再激活。 */
  const select = useCallback(
    async (id: number, saveFirst = false): Promise<boolean> => {
      if (!config || !active || busyRef.current) return false
      if (saveFirst && !validConfig(config)) {
        setError(invalidConfigError)
        return false
      }
      busyRef.current = true
      setBusy(true)
      setError('')
      try {
        if (saveFirst) await updateChartBoard(active.id, { config: structuredClone(config) })
        const state = await activateChartBoard(id)
        syncDraft(state)
        const selected = state.boards.find((b) => b.id === id)
        if (selected?.config.defaultSymbol) selectRef.current?.(selected.config.defaultSymbol)
        return true
      } catch (reason: unknown) {
        setError(reason instanceof Error ? reason.message : '切换看板失败')
        return false
      } finally {
        busyRef.current = false
        setBusy(false)
      }
    },
    [config, active, syncDraft],
  )

  const create = useCallback(
    async (name: string, copy: boolean, layout?: Partial<BoardConfig>): Promise<boolean> => {
      if (!config) return false
      const trimmed = name.trim()
      if (!trimmed) {
        setError('看板名称不能为空')
        return false
      }
      const next = copy ? { ...config, ...layout } : defaultBoardConfig(config.defaultSymbol)
      if (!validConfig(next)) {
        setError(invalidConfigError)
        return false
      }
      const state = await execute(() => createChartBoard(trimmed, structuredClone(next)))
      if (!state) return false
      syncDraft(state)
      return true
    },
    [config, execute, syncDraft],
  )

  const rename = useCallback(
    async (name: string): Promise<boolean> => {
      if (!active) return false
      const trimmed = name.trim()
      if (!trimmed) {
        setError('看板名称不能为空')
        return false
      }
      const state = await execute(() => updateChartBoard(active.id, { name: trimmed }))
      if (!state) return false
      applyBoards(state)
      return true
    },
    [active, execute, applyBoards],
  )

  const remove = useCallback(async (): Promise<boolean> => {
    if (!active || boards.length <= 1 || busyRef.current) return false
    const state = await execute(() => deleteChartBoard(active.id))
    if (!state) return false
    syncDraft(state)
    const next = state.boards.find((b) => b.id === state.active_id)
    if (next?.config.defaultSymbol) selectRef.current?.(next.config.defaultSymbol)
    return true
  }, [active, boards.length, execute, syncDraft])

  return {
    status,
    boards,
    activeId,
    active,
    config,
    busy,
    dirty,
    error,
    reload: useCallback(() => setReloadTick((t) => t + 1), []),
    setConfig,
    save,
    select,
    create,
    rename,
    remove,
    restore: useCallback(() => {
      if (active) setConfigState(structuredClone(active.config))
    }, [active]),
  }
}
export type BoardController = ReturnType<typeof useBoards>
