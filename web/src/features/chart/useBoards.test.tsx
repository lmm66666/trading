import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  activateChartBoard,
  createChartBoard,
  deleteChartBoard,
  listChartBoards,
  updateChartBoard,
  type BoardConfig,
  type ChartBoardState,
} from '../../api/client'
import { useBoards } from './useBoards'
import { defaultBoardConfig } from './boards'

vi.mock('../../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/client')>()
  return {
    ...actual,
    activateChartBoard: vi.fn(),
    createChartBoard: vi.fn(),
    deleteChartBoard: vi.fn(),
    listChartBoards: vi.fn(),
    updateChartBoard: vi.fn(),
  }
})

/** 有状态 fake 服务端：模拟 t_chart_boards 的整表行为（创建即激活、删除后激活剩余首块）。 */
let server: { boards: { id: number; name: string; config: BoardConfig }[]; activeId: number }
let nextId: number

function stateOf(): ChartBoardState {
  return { boards: structuredClone(server.boards), active_id: server.activeId }
}

function seedBoard(name = '默认看板', config = defaultBoardConfig()): number {
  const id = nextId++
  server.boards.push({ id, name, config })
  server.activeId = id
  return id
}

beforeEach(() => {
  server = { boards: [], activeId: 0 }
  nextId = 1
  vi.mocked(listChartBoards).mockReset().mockImplementation(async () => stateOf())
  vi.mocked(createChartBoard).mockReset().mockImplementation(async (name, config) => {
    const board = { id: nextId++, name, config: structuredClone(config) }
    server.boards.push(board)
    server.activeId = board.id
    return stateOf()
  })
  vi.mocked(updateChartBoard).mockReset().mockImplementation(async (id, changes) => {
    const board = server.boards.find((b) => b.id === id)
    if (!board) throw new Error('看板不存在')
    if (changes.name !== undefined) board.name = changes.name
    if (changes.config !== undefined) board.config = structuredClone(changes.config)
    return stateOf()
  })
  vi.mocked(activateChartBoard).mockReset().mockImplementation(async (id) => {
    if (!server.boards.some((b) => b.id === id)) throw new Error('看板不存在')
    server.activeId = id
    return stateOf()
  })
  vi.mocked(deleteChartBoard).mockReset().mockImplementation(async (id) => {
    const index = server.boards.findIndex((b) => b.id === id)
    if (index < 0) throw new Error('看板不存在')
    server.boards.splice(index, 1)
    if (server.activeId === id) server.activeId = server.boards[0]?.id ?? 0
    return stateOf()
  })
})

it('creates a default board on empty table seeded with URL parameters', async () => {
  const { result } = renderHook(() =>
    useBoards({ defaultSymbol: 'SSE:600000', timeframe: 'WEEK', priceView: 'RAW' }),
  )
  await waitFor(() => expect(result.current.status).toBe('ready'))
  expect(createChartBoard).toHaveBeenCalledWith(
    '默认看板',
    expect.objectContaining({ defaultSymbol: 'SSE:600000', timeframe: 'WEEK', priceView: 'RAW' }),
  )
  expect(result.current.boards).toHaveLength(1)
  expect(result.current.config?.timeframe).toBe('WEEK')
  expect(result.current.dirty).toBe(false)
})

it('restores the active board symbol only when the URL provided none', async () => {
  const select = vi.fn()
  seedBoard('自选看板', defaultBoardConfig('SSE:600938'))
  const { result } = renderHook(() => useBoards({}, select))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  expect(select).toHaveBeenCalledWith('SSE:600938')
  expect(result.current.active?.name).toBe('自选看板')

  select.mockClear()
  seedBoard()
  const url = renderHook(() => useBoards({ defaultSymbol: 'SZSE:002415' }, select))
  await waitFor(() => expect(url.result.current.status).toBe('ready'))
  expect(select).not.toHaveBeenCalled()
})

it('edits remain drafts until save; boards stay independent across switch, copy, rename, delete', async () => {
  const select = vi.fn()
  const { result } = renderHook(() => useBoards({ defaultSymbol: 'SSE:600938' }, select))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  expect(createChartBoard).toHaveBeenCalledOnce()
  const firstId = result.current.activeId

  act(() => result.current.setConfig((c) => ({ ...c, comparison: 'INE:SC.MAIN' })))
  expect(result.current.dirty).toBe(true)
  await act(async () => {
    expect(await result.current.save()).toBe(true)
  })
  expect(result.current.dirty).toBe(false)

  await act(async () => {
    expect(await result.current.rename('油价看板')).toBe(true)
  })
  expect(result.current.active?.name).toBe('油价看板')

  await act(async () => {
    expect(await result.current.create('周线', true)).toBe(true)
  })
  const copyId = result.current.activeId
  expect(result.current.boards).toHaveLength(2)
  act(() => result.current.setConfig((c) => ({ ...c, timeframe: 'WEEK' })))

  await act(async () => {
    expect(await result.current.select(firstId, true)).toBe(true)
  })
  expect(updateChartBoard).toHaveBeenCalledWith(copyId, {
    config: expect.objectContaining({ timeframe: 'WEEK' }),
  })
  expect(result.current.config?.timeframe).toBe('DAY')
  expect(select).toHaveBeenCalledWith('SSE:600938')

  await act(async () => {
    expect(await result.current.select(copyId)).toBe(true)
  })
  expect(result.current.config?.timeframe).toBe('WEEK')

  act(() => result.current.setConfig((c) => ({ ...c, comparison: null })))
  act(() => result.current.restore())
  expect(result.current.config?.comparison).toBe('INE:SC.MAIN')

  await act(async () => {
    expect(await result.current.remove()).toBe(true)
  })
  expect(result.current.boards).toHaveLength(1)
  expect(result.current.active?.name).toBe('油价看板')
})

it('creating a blank board starts from defaults; rename keeps the unsaved draft', async () => {
  const { result } = renderHook(() => useBoards({ defaultSymbol: 'SSE:600938' }))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  act(() => result.current.setConfig((c) => ({ ...c, comparison: 'INE:SC.MAIN' })))
  await act(async () => {
    expect(await result.current.rename('改名')).toBe(true)
  })
  expect(result.current.active?.name).toBe('改名')
  expect(result.current.dirty).toBe(true)
  expect(result.current.config?.comparison).toBe('INE:SC.MAIN')
  await act(async () => {
    expect(await result.current.create('新看板', false)).toBe(true)
  })
  expect(result.current.config?.comparison).toBeNull()
  expect(result.current.config?.defaultSymbol).toBe('SSE:600938')
})

it('rejects empty names and invalid config locally without hitting the server', async () => {
  const { result } = renderHook(() => useBoards({}))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  vi.mocked(updateChartBoard).mockClear()
  vi.mocked(createChartBoard).mockClear()
  await act(async () => {
    expect(await result.current.rename('')).toBe(false)
  })
  await act(async () => {
    expect(await result.current.create('  ', false)).toBe(false)
  })
  expect(updateChartBoard).not.toHaveBeenCalled()
  expect(createChartBoard).not.toHaveBeenCalled() // 首建发生在 mockClear 之前，此后不再调用
  act(() => result.current.setConfig((c) => ({ ...c, visibleBars: 999 })))
  await act(async () => {
    expect(await result.current.save()).toBe(false)
  })
  // 文案只描述配置参数问题：名称校验不走 validConfig（空名单独提示，长度由服务端把关）。
  expect(result.current.error).toBe('看板参数不符合要求，请检查标的、指标、对比或布局设置')
  expect(updateChartBoard).not.toHaveBeenCalled()
  expect(result.current.dirty).toBe(true)
})

it('failed operations keep the draft and local state, including save-and-switch', async () => {
  const firstId = seedBoard()
  const secondId = seedBoard('第二看板')
  const { result } = renderHook(() => useBoards({}))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  expect(result.current.activeId).toBe(secondId)
  act(() => result.current.setConfig((c) => ({ ...c, comparison: 'INE:SC.MAIN' })))

  vi.mocked(updateChartBoard).mockRejectedValueOnce(new Error('服务不可用'))
  await act(async () => {
    expect(await result.current.save()).toBe(false)
  })
  expect(result.current.error).toBe('服务不可用')
  expect(result.current.dirty).toBe(true)
  expect(result.current.config?.comparison).toBe('INE:SC.MAIN')

  vi.mocked(updateChartBoard).mockRejectedValueOnce(new Error('保存失败'))
  await act(async () => {
    expect(await result.current.select(firstId, true)).toBe(false)
  })
  expect(activateChartBoard).not.toHaveBeenCalled()
  expect(result.current.activeId).toBe(secondId)
  expect(result.current.dirty).toBe(true)

  const leave = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(leave)
  expect(leave.defaultPrevented).toBe(true)
})

it('reports load failure and can retry', async () => {
  vi.mocked(listChartBoards).mockRejectedValueOnce(new Error('网络中断'))
  const { result } = renderHook(() => useBoards({}))
  await waitFor(() => expect(result.current.status).toBe('error'))
  expect(result.current.error).toBe('网络中断')
  act(() => result.current.reload())
  await waitFor(() => expect(result.current.status).toBe('ready'))
  expect(result.current.boards).toHaveLength(1)
  expect(result.current.active?.name).toBe('默认看板')
})

it('ignores concurrent operations while busy', async () => {
  seedBoard()
  let resolveSave!: (value: ChartBoardState) => void
  vi.mocked(updateChartBoard).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveSave = resolve
      }),
  )
  const { result } = renderHook(() => useBoards({}))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  act(() => result.current.setConfig((c) => ({ ...c, comparison: 'INE:SC.MAIN' })))
  let first!: Promise<boolean>
  let second!: Promise<boolean>
  act(() => {
    first = result.current.save()
    second = result.current.save()
  })
  expect(result.current.busy).toBe(true)
  await act(async () => {
    expect(await second).toBe(false)
  })
  await act(async () => resolveSave(stateOf()))
  expect(await first).toBe(true)
  expect(result.current.busy).toBe(false)
})

it('refuses to remove the last board without calling the server', async () => {
  seedBoard()
  const { result } = renderHook(() => useBoards({}))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  await act(async () => {
    expect(await result.current.remove()).toBe(false)
  })
  expect(deleteChartBoard).not.toHaveBeenCalled()
})

it('ZSCORE remains a draft until saved and restores its full parameters after remount', async () => {
  const initial = defaultBoardConfig()
  seedBoard('涨幅偏差', initial)
  const hook = renderHook(() => useBoards({}))
  await waitFor(() => expect(hook.result.current.status).toBe('ready'))
  const zscore = { kind: 'ZSCORE' as const, period: 126, smooth: 5, regime: 252 }
  act(() => hook.result.current.setConfig((c) => ({ ...c, indicators: [zscore] })))
  expect(server.boards[0].config.indicators).toEqual(initial.indicators)
  expect(hook.result.current.dirty).toBe(true)
  await act(async () => {
    expect(await hook.result.current.save()).toBe(true)
  })
  hook.unmount()
  const restored = renderHook(() => useBoards({}))
  await waitFor(() => expect(restored.result.current.status).toBe('ready'))
  expect(restored.result.current.config?.indicators).toEqual([zscore])
  expect(restored.result.current.dirty).toBe(false)
})

it('migrates legacy board drafts, keeps dirty on restore, and persists only on explicit save', async () => {
  const config = defaultBoardConfig('SSE:600938')
  config.comparison = 'SHFE:AU.MAIN'
  config.indicators = [{kind:'STD',period:20},{kind:'RETZ',period:126,smooth:5,regime:252}] as never
  seedBoard('旧看板', config)
  const {result} = renderHook(() => useBoards({}))
  await waitFor(() => expect(result.current.status).toBe('ready'))
  expect(result.current.config?.indicators).toEqual([{kind:'ZSCORE',period:126,smooth:5,regime:252,lag:0}])
  expect(result.current.config?.comparison).toBe('SHFE:AU.MAIN')
  expect(result.current.dirty).toBe(true)
  expect(result.current.migrationNotice).toContain('保存')
  expect(updateChartBoard).not.toHaveBeenCalled()
  act(() => result.current.restore())
  expect(result.current.dirty).toBe(true)
  await act(async () => {await result.current.save()})
  expect(result.current.dirty).toBe(false)
  expect(result.current.migrationNotice).toBe('')
  expect(server.boards[0].config.indicators[0].kind).toBe('ZSCORE')
})
