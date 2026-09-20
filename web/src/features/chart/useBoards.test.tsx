import { act, renderHook } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { useBoards } from './useBoards'
import { BOARDS_KEY, readBoards } from './boards'

beforeEach(() => localStorage.clear())
it('edits remain drafts until save; naming, copying, switching, deleting restore independent boards', () => {
  const select = vi.fn()
  const { result, unmount } = renderHook(() => useBoards({ defaultSymbol: 'SSE:600938' }, select))
  act(() => {
    result.current.setConfig((c) => ({ ...c, comparison: 'INE:SC.MAIN' }))
  })
  expect(localStorage.getItem(BOARDS_KEY)).toBeNull()
  act(() => {
    expect(result.current.save()).toBe(true)
  })
  expect(result.current.dirty).toBe(false)
  act(() => {
    result.current.rename('油价看板')
  })
  act(() => {
    result.current.create('周线', true)
  })
  const copy = result.current.active.id
  act(() => result.current.setConfig((c) => ({ ...c, timeframe: 'WEEK' })))
  act(() => {
    result.current.select('default', true)
  })
  expect(select).toHaveBeenCalledWith('SSE:600938')
  expect(result.current.config.timeframe).toBe('DAY')
  act(() => {
    result.current.select(copy)
  })
  expect(result.current.config.timeframe).toBe('WEEK')
  act(() => result.current.setConfig((c) => ({ ...c, comparison: null })))
  act(() => result.current.restore())
  expect(result.current.config.comparison).toBe('INE:SC.MAIN')
  act(() => {
    result.current.remove()
  })
  expect(result.current.store.boards).toHaveLength(1)
  expect(result.current.remove()).toBe(false)
  unmount()
  const restored = renderHook(() => useBoards({}))
  expect(restored.result.current.active.name).toBe('油价看板')
})
it('failed saves keep dirty state and previous storage', () => {
  const { result } = renderHook(() => useBoards({ defaultSymbol: 'SSE:600938' }))
  act(() => {
    result.current.save()
  })
  act(() => result.current.setConfig((c) => ({ ...c, timeframe: 'WEEK' })))
  vi.spyOn(Storage.prototype, 'setItem').mockImplementationOnce(() => {
    throw new Error('存储已满')
  })
  act(() => {
    expect(result.current.save()).toBe(false)
  })
  expect(result.current.error).toBe('存储已满')
  expect(result.current.dirty).toBe(true)
  expect(readBoards().data.boards[0].config.timeframe).toBe('DAY')
  const leave = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(leave)
  expect(leave.defaultPrevented).toBe(true)
})

it('does not lose edits when creating or renaming is rejected; new starts from defaults', () => {
  const { result } = renderHook(() => useBoards({ defaultSymbol: 'SSE:600938' }))
  act(() => {
    result.current.save()
  })
  act(() => result.current.setConfig((c) => ({ ...c, comparison: 'INE:SC.MAIN' })))
  act(() => {
    expect(result.current.rename('')).toBe(false)
  })
  expect(result.current.dirty).toBe(true)
  act(() => {
    expect(result.current.create('', true)).toBe(false)
  })
  expect(result.current.config.comparison).toBe('INE:SC.MAIN')
  act(() => {
    result.current.create('新看板', false)
  })
  expect(result.current.config.comparison).toBeNull()
})
it('does not overwrite corrupt records and does not switch or remove on external conflict', () => {
  localStorage.setItem(BOARDS_KEY, 'corrupt')
  const bad = renderHook(() => useBoards({}))
  act(() => {
    expect(bad.result.current.save()).toBe(false)
  })
  expect(localStorage.getItem(BOARDS_KEY)).toBe('corrupt')
  bad.unmount()
  localStorage.clear()
  const { result } = renderHook(() => useBoards({}))
  act(() => {
    result.current.save()
  })
  act(() => {
    result.current.create('第二个', true)
  })
  localStorage.setItem(BOARDS_KEY, 'external')
  act(() => {
    expect(result.current.select('default')).toBe(false)
    expect(result.current.remove()).toBe(false)
  })
  expect(result.current.active.name).toBe('第二个')
})
