import { useEffect, useState } from 'react'
import { defaultBoardConfig, readBoards, saveBoards, type BoardConfig, type BoardStore } from './boards'

export function useBoards(initial: Partial<BoardConfig>, onSelectSymbol?: (symbol: string) => void) {
  const [loaded] = useState(readBoards)
  const [store, setStore] = useState(loaded.data)
  const [raw, setRaw] = useState(loaded.raw)
  const [error, setError] = useState(loaded.error)
  const active = store.boards.find((b) => b.id === store.activeId)!
  const [config, setConfig] = useState<BoardConfig>(() =>
    loaded.raw ? active.config : { ...active.config, ...initial },
  )
  const dirty = JSON.stringify(config) !== JSON.stringify(active.config) || raw === null
  useEffect(() => {
    if (!dirty) return
    const leave = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', leave)
    return () => window.removeEventListener('beforeunload', leave)
  }, [dirty])
  const persist = (next: BoardStore) => {
    try {
      if (loaded.error) throw new Error(loaded.error)
      const nextRaw = saveBoards(next, raw)
      setRaw(nextRaw)
      setStore(next)
      setError('')
      return true
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败，请重试')
      return false
    }
  }
  const save = (layout?: Partial<BoardConfig>) => {
    const next = { ...config, ...layout }
    if (
      !persist({
        ...store,
        boards: store.boards.map((b) => (b.id === active.id ? { ...b, config: structuredClone(next) } : b)),
      })
    )
      return false
    setConfig(next)
    return true
  }
  const select = (id: string, saveFirst = false) => {
    const next = {
      ...store,
      activeId: id,
      boards: store.boards.map((b) =>
        saveFirst && b.id === active.id ? { ...b, config: structuredClone(config) } : b,
      ),
    }
    if (!persist(next)) return false
    const selected = next.boards.find((b) => b.id === id)!
    setConfig(structuredClone(selected.config))
    if (selected.config.defaultSymbol) onSelectSymbol?.(selected.config.defaultSymbol)
    return true
  }
  const create = (name: string, copy: boolean, layout?: Partial<BoardConfig>) => {
    const nextConfig = copy
      ? structuredClone({ ...config, ...layout })
      : defaultBoardConfig(config.defaultSymbol)
    const id = crypto.randomUUID()
    if (
      !persist({
        ...store,
        activeId: id,
        boards: [...store.boards, { id, name: name.trim(), config: nextConfig }],
      })
    )
      return false
    setConfig(nextConfig)
    return true
  }
  const rename = (name: string) =>
    persist({
      ...store,
      boards: store.boards.map((b) => (b.id === active.id ? { ...b, name: name.trim() } : b)),
    })
  const remove = () => {
    if (store.boards.length <= 1) return false
    const boards = store.boards.filter((b) => b.id !== active.id)
    if (!persist({ ...store, boards, activeId: boards[0].id })) return false
    setConfig(structuredClone(boards[0].config))
    if (boards[0].config.defaultSymbol) onSelectSymbol?.(boards[0].config.defaultSymbol)
    return true
  }
  return {
    store,
    active,
    config,
    setConfig,
    dirty,
    error,
    save,
    select,
    create,
    rename,
    remove,
    restore: () => setConfig(structuredClone(active.config)),
  }
}
export type BoardController = ReturnType<typeof useBoards>
