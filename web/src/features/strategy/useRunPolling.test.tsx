import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ApiError, getRun, type RunStatus } from '../../api/client'
import { useRunPolling } from './useRunPolling'
vi.mock('../../api/client', async (original) => ({ ...await original<typeof import('../../api/client')>(), getRun: vi.fn() }))
const running: RunStatus = { run_id: 'r1', kind: 'scan', status: 'RUNNING', strategy: 's', strategy_version: '1', data_version: 1, engine_version: '1', attempts: 1, cancel_requested_at: null }
beforeEach(() => { vi.useFakeTimers(); vi.clearAllMocks() })
afterEach(() => vi.useRealTimers())
const tick = async (ms = 0) => act(async () => { await vi.advanceTimersByTimeAsync(ms) })
it('restoring a saved task is loading, not idle', () => {
  vi.mocked(getRun).mockReturnValue(new Promise(() => {}))
  const { result } = renderHook(() => useRunPolling('scan', 'r1', { classifyErrors: true }))
  expect(result.current.loading).toBe(true)
})
it('stops immediately on typed missing task and exposes missing separately', async () => {
  vi.mocked(getRun).mockRejectedValue(new ApiError(404, 'NOT_FOUND'))
  const { result } = renderHook(() => useRunPolling('scan', 'r1', { classifyErrors: true }))
  await tick(10000)
  expect(result.current.missing).toBe(true)
  expect(getRun).toHaveBeenCalledTimes(1)
})
it('retains the task on network failure and allows explicit retry', async () => {
  vi.mocked(getRun).mockRejectedValue(new Error('offline'))
  const { result } = renderHook(() => useRunPolling('scan', 'r1', { classifyErrors: true }))
  await tick(4000)
  expect(result.current.error).toBe('offline')
  expect(result.current.missing).toBe(false)
  vi.mocked(getRun).mockResolvedValue(running)
  act(() => result.current.retry())
  await tick()
  expect(result.current.status?.status).toBe('RUNNING')
  expect(result.current.error).toBeNull()
})
it('ignores stale failure after changing task, pauses when hidden and resumes', async () => {
  let reject!: (error: Error) => void
  vi.mocked(getRun).mockReturnValueOnce(new Promise((_, no) => { reject = no })).mockResolvedValue(running)
  const { result, rerender } = renderHook(({ id, active }) => useRunPolling('scan', id, { active, classifyErrors: true }), { initialProps: { id: 'old', active: true } })
  rerender({ id: 'r1', active: true })
  await tick()
  await act(async () => reject(new ApiError(404, 'NOT_FOUND')))
  expect(result.current.missing).toBe(false)
  expect(result.current.status?.run_id).toBe('r1')
  rerender({ id: 'r1', active: false })
  const calls = vi.mocked(getRun).mock.calls.length
  await tick(10000)
  expect(getRun).toHaveBeenCalledTimes(calls)
  rerender({ id: 'r1', active: true })
  await tick()
  expect(getRun).toHaveBeenCalledTimes(calls + 1)
})
