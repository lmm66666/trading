import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import { useComparison } from './useComparison'
import type { ChartResult } from '../../api/client'
const bar = {
  open_time: '2026-09-01T00:00:00Z',
  close_time: '2026-09-01T07:00:00Z',
  open: 9,
  close: 10,
  low: 8,
  high: 12,
  volume: 100,
  amount: 100,
  trading_status: 0,
}
const result: ChartResult = {
  instrument: {
    instrument: 'SSE:600938',
    code: '600938',
    name: '中国海油',
    exchange: 'SSE',
    board: 'MAIN',
    lot_size: 100,
  },
  timeframe: 'DAY',
  price_view: 'QFQ',
  data_version: 8,
  bars: [bar],
  series: [],
  has_more: false,
  next_before: null,
}
const response = (data: unknown) => ({ ok: true, json: async () => ({ code: 0, data }) })
beforeEach(() => vi.stubGlobal('fetch', vi.fn()))
afterEach(() => vi.unstubAllGlobals())
it('queries same version and calendar window, handles unavailable and retries', async () => {
  vi.mocked(fetch).mockResolvedValueOnce(
    response({ instrument: 'INE:SC.MAIN', data_version: 8, bars: [bar] }) as Response,
  )
  const { result: state, rerender } = renderHook(({ id }) => useComparison(id, result, 'DAY'), {
    initialProps: { id: 'INE:SC.MAIN' as string | null },
  })
  await waitFor(() => expect(state.current.bars).toHaveLength(1))
  const url = new URL(String(vi.mocked(fetch).mock.calls[0][0]), 'http://localhost')
  expect(url.searchParams.get('version')).toBe('8')
  expect(url.searchParams.get('from')).toBe('2026-09-01')
  vi.mocked(fetch).mockResolvedValueOnce(
    response({ instrument: 'SHFE:AU.MAIN', data_version: 8, bars: [] }) as Response,
  )
  rerender({ id: 'SHFE:AU.MAIN' })
  await waitFor(() => expect(state.current.error).toContain('没有关联行情'))
  vi.mocked(fetch).mockResolvedValueOnce(
    response({ instrument: 'SHFE:AU.MAIN', data_version: 8, bars: [bar] }) as Response,
  )
  act(() => state.current.retry())
  await waitFor(() => expect(state.current.bars).toHaveLength(1))
  rerender({ id: null })
  expect(state.current.bars).toEqual([])
})
it('rejects version mismatch and ignores late aborted responses', async () => {
  let resolve: (value: Response) => void = () => {}
  vi.mocked(fetch).mockImplementationOnce(
    () =>
      new Promise((r) => {
        resolve = r
      }),
  )
  const { result: state, rerender } = renderHook(({ id }) => useComparison(id, result, 'WEEK'), {
    initialProps: { id: 'INE:SC.MAIN' },
  })
  vi.mocked(fetch).mockResolvedValueOnce(
    response({ instrument: 'SHFE:AU.MAIN', data_version: 99, bars: [bar] }) as Response,
  )
  rerender({ id: 'SHFE:AU.MAIN' })
  await waitFor(() => expect(state.current.error).toContain('不匹配'))
  await act(async () =>
    resolve(response({ instrument: 'INE:SC.MAIN', data_version: 8, bars: [bar] }) as Response),
  )
  expect(state.current.bars).toEqual([])
  expect(vi.mocked(fetch).mock.calls[0][1]?.signal?.aborted).toBe(true)
})
it('reports connection failures and bounded truncation', async () => {
  vi.mocked(fetch).mockRejectedValueOnce(new Error('offline'))
  const { result: state } = renderHook(() => useComparison('INE:SC.MAIN', result, 'DAY'))
  await waitFor(() => expect(state.current.error).toContain('无法连接'))
  vi.mocked(fetch).mockResolvedValueOnce(
    response({ instrument: 'INE:SC.MAIN', data_version: 8, bars: Array(5000).fill(bar) }) as Response,
  )
  act(() => state.current.retry())
  await waitFor(() => expect(state.current.error).toContain('5000'))
})
