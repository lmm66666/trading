import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import App from './App'
import { fetchSnapshotPage, getRun, listChartBoards, listStrategies, listWatchlist } from './api/client'
import { defaultBoardConfig } from './features/chart/boards'
vi.mock('./api/client', async (original) => ({ ...await original<typeof import('./api/client')>(), listWatchlist: vi.fn(), listStrategies: vi.fn(), listChartBoards: vi.fn(), getRun: vi.fn(), fetchSnapshotPage: vi.fn() }))
vi.mock('./features/chart/ChartWorkspace', () => ({ ChartWorkspace: () => <p>行情图表</p> }))
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  localStorage.setItem('wb.scan_run_id', 'r1')
  window.history.replaceState(null, '', '/?tab=scan')
  vi.mocked(listWatchlist).mockResolvedValue([])
  vi.mocked(listChartBoards).mockResolvedValue({ boards: [{ id: 1, name: '默认看板', config: defaultBoardConfig() }], active_id: 1 })
  vi.mocked(listStrategies).mockResolvedValue([{ strategy: 's', version: '1', primary_timeframe: 'daily', warmup_bars: 1, default_hold_bars: 1, parameters: {}, features: [], auxiliary: [] }])
  vi.mocked(getRun).mockResolvedValue({ run_id: 'r1', kind: 'scan', status: 'SUCCEEDED', strategy: 's', strategy_version: '1', snapshot_id: 'snap1', data_version: 1, engine_version: '1', attempts: 1, cancel_requested_at: null })
  const page = { snapshot_id: 'snap1', run_id: 'r1', data_version: 1, key: { snapshot_id: 'snap1', strategy_id: 's', strategy_version: '1', parameters_hash: 'p', as_of: '2026-06-01' }, failures: [] }
  vi.mocked(fetchSnapshotPage).mockResolvedValueOnce({ ...page, rows: [{ instrument: 'SSE:600000', name: '浦发银行', signal_time: '2026-06-01T00:00:00Z', reason: 'buy' }], next_sequence: 1 }).mockResolvedValueOnce({ ...page, rows: [{ instrument: 'SSE:600001', signal_time: '2026-06-01T00:00:00Z', reason: 'buy' }] })
})
it('chart navigation retains the real scan draft, loaded pages and scroll container', async () => {
  render(<App />)
  await screen.findByRole('button', { name: 'SSE:600000' })
  fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
  fireEvent.click(screen.getByRole('button', { name: '加载更多' }))
  await screen.findByRole('button', { name: 'SSE:600001' })
  const workspace = screen.getByRole('region', { name: '策略扫描' })
  workspace.scrollTop = 300
  fireEvent.click(screen.getByRole('button', { name: 'SSE:600000' }))
  expect(await screen.findByText('行情图表')).toBeVisible()
  expect(workspace).not.toBeVisible()
  expect(screen.queryByRole('button', { name: 'SSE:600001' })).toBeNull()
  fireEvent.click(screen.getByRole('tab', { name: '扫描' }))
  expect(screen.getByLabelText('开始日期')).toHaveValue('2026-01-01')
  expect(screen.getByRole('button', { name: 'SSE:600001' })).toBeVisible()
  expect(workspace.scrollTop).toBe(300)
  expect(fetchSnapshotPage).toHaveBeenCalledTimes(2)
})
