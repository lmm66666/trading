import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RefreshMonitor } from './RefreshMonitor'
import * as api from '../../api/client'
vi.mock('../../api/client', () => ({
  getRefreshStatus: vi.fn(),
  listRefreshRuns: vi.fn(),
  getRefreshRun: vi.fn(),
  listRefreshFailures: vi.fn(),
  triggerMarketRefresh: vi.fn(),
}))
const run = {
  id: 1,
  run_id: 'run-1',
  kind: 'STOCK' as const,
  trigger: 'MANUAL',
  state: 'RUNNING',
  total: 10,
  succeeded: 3,
  failed: 1,
  started_at: new Date().toISOString(),
  finished_at: null,
  heartbeat_at: new Date().toISOString(),
  snapshot_at: new Date().toISOString(),
  last_progress_at: null,
  snapshot_revision: 2,
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(api.getRefreshStatus).mockResolvedValue({
    stock: null,
    futures: null,
    futures_enabled: false,
  })
  vi.mocked(api.listRefreshRuns).mockResolvedValue({ items: [] })
  vi.mocked(api.listRefreshFailures).mockResolvedValue({ items: [] })
  vi.mocked(api.getRefreshRun).mockResolvedValue(run)
  vi.mocked(api.triggerMarketRefresh).mockResolvedValue({
    status: 'ACCEPTED',
    run_id: 'run-1',
    progress_available: true,
  })
})
describe('数据更新面板', () => {
  it('明确区分股票操作和期货状态，关闭不取消更新', async () => {
    render(<RefreshMonitor onReload={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
    await screen.findByText('未启用')
    fireEvent.click(screen.getByRole('button', { name: '更新全部股票' }))
    await screen.findByText(/更新已受理/)
    expect(api.triggerMarketRefresh).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: '关闭数据更新' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('显示真实进度并阻止重复触发', async () => {
    vi.mocked(api.getRefreshStatus).mockResolvedValue({
      stock: run,
      futures: null,
      futures_enabled: true,
    })
    render(<RefreshMonitor onReload={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
    await screen.findByText('已处理 4 / 10')
    expect(screen.getByRole('button', { name: '更新全部股票' })).toBeDisabled()
    expect(screen.getByRole('progressbar')).toHaveAttribute('value', '4')
  })
  it('提交结果未知时只查询，不重发', async () => {
    vi.mocked(api.triggerMarketRefresh).mockRejectedValue(new Error('timeout'))
    render(<RefreshMonitor onReload={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
    await screen.findByText('未启用')
    fireEvent.click(screen.getByRole('button', { name: '更新全部股票' }))
    await screen.findByText(/提交结果未确认/)
    expect(api.triggerMarketRefresh).toHaveBeenCalledTimes(1)
  })
  it('历史结果可以查看失败明细，加载最新行情由用户决定', async () => {
    const done = {
      ...run,
      state: 'PARTIAL_SUCCEEDED',
      succeeded: 9,
      finished_at: new Date().toISOString(),
    }
    vi.mocked(api.getRefreshStatus).mockResolvedValue({
      stock: done,
      futures: null,
      futures_enabled: null,
    })
    vi.mocked(api.listRefreshRuns).mockResolvedValue({ items: [done] })
    vi.mocked(api.getRefreshRun).mockResolvedValue(done)
    vi.mocked(api.listRefreshFailures).mockResolvedValue({
      items: [
        {
          id: 1,
          run_id: 'run-1',
          exchange: 'SSE',
          code: '600000',
          name: '浦发银行',
          error_code: 'REFRESH_FAILED',
          completed_at: run.started_at,
        },
      ],
    })
    const reload = vi.fn()
    render(<RefreshMonitor onReload={reload} />)
    fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
    await screen.findByText('启用状态未知')
    fireEvent.click(screen.getByRole('button', { name: /查看任务 run-1/ }))
    await screen.findByText(/浦发银行/)
    expect(reload).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '加载最新行情' }))
    expect(reload).toHaveBeenCalledOnce()
  })
})

it('准备、空范围与中断各自展示，Escape 归还焦点', async () => {
  const interrupted = {
    ...run,
    state: 'INTERRUPTED',
    finished_at: new Date().toISOString(),
    total: 0,
    succeeded: 0,
    failed: 0,
  }
  vi.mocked(api.getRefreshStatus).mockResolvedValue({
    stock: interrupted,
    futures: { ...run, kind: 'FUTURES', state: 'PREPARING', total: null },
    futures_enabled: true,
  })
  render(<RefreshMonitor onReload={vi.fn()} />)
  const button = screen.getByRole('button', { name: /数据更新/ })
  fireEvent.click(button)
  await screen.findByText('无更新对象')
  expect(screen.getByText('正在准备证券范围')).toBeInTheDocument()
  expect(screen.getByText(/最后记录进度/)).toBeInTheDocument()
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
  expect(button).toHaveFocus()
})
it('过期心跳不变成失败，详情和历史故障可重试', async () => {
  vi.mocked(api.getRefreshStatus).mockResolvedValue({
    stock: { ...run, heartbeat_at: '2000-01-01T00:00:00Z' },
    futures: null,
    futures_enabled: true,
  })
  vi.mocked(api.listRefreshRuns).mockRejectedValue(new Error('offline'))
  vi.mocked(api.getRefreshRun).mockRejectedValueOnce(new Error('offline'))
  render(<RefreshMonitor onReload={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
  await screen.findByText(/进度记录已过期/)
  await screen.findByText('更新历史暂不可用')
  vi.mocked(api.listRefreshRuns).mockResolvedValue({ items: [] })
  fireEvent.click(screen.getByRole('button', { name: '重试历史' }))
  await screen.findByText('暂无更新记录')
  fireEvent.click(screen.getByRole('button', { name: '查看股票详情' }))
  await screen.findByText(/进度暂不可用/)
  fireEvent.click(screen.getByRole('button', { name: '重试查询' }))
  await waitFor(() => expect(api.getRefreshRun).toHaveBeenCalledTimes(2))
})
it('存储不可用的受理仍不重复提交，查询错误保留提示', async () => {
  vi.mocked(api.triggerMarketRefresh).mockResolvedValue({
    status: 'ACCEPTED',
    run_id: 'run-1',
    progress_available: false,
  })
  render(<RefreshMonitor onReload={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
  await screen.findByText('未启用')
  fireEvent.click(screen.getByRole('button', { name: '更新全部股票' }))
  await screen.findByText(/更新已受理，进度记录暂不可用/)
  expect(api.triggerMarketRefresh).toHaveBeenCalledTimes(1)
})
it('进度连接失败可以手动刷新', async () => {
  vi.mocked(api.getRefreshStatus).mockRejectedValue(new Error('offline'))
  render(<RefreshMonitor onReload={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
  await screen.findByRole('alert')
  vi.mocked(api.getRefreshStatus).mockResolvedValue({
    stock: null,
    futures: null,
    futures_enabled: false,
  })
  fireEvent.click(screen.getByRole('button', { name: '刷新进度' }))
  await screen.findByText('未启用')
})
it('历史和失败明细游标固定在当前任务', async () => {
  const done = { ...run, state: 'FAILED', finished_at: run.started_at }
  vi.mocked(api.listRefreshRuns).mockResolvedValue({
    items: Array.from({ length: 20 }, (_, i) => ({
      ...done,
      id: 20 - i,
      run_id: `history-${i}`,
    })),
  })
  vi.mocked(api.getRefreshRun).mockResolvedValue(done)
  vi.mocked(api.listRefreshFailures).mockResolvedValue({
    items: Array.from({ length: 50 }, (_, i) => ({
      id: i + 1,
      run_id: 'history-0',
      exchange: 'SSE',
      code: String(600000 + i),
      error_code: 'REFRESH_FAILED',
      completed_at: run.started_at,
    })),
  })
  render(<RefreshMonitor onReload={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
  await screen.findByRole('button', { name: '查看任务 history-0' })
  fireEvent.click(screen.getByRole('button', { name: '更早记录' }))
  await waitFor(() =>
    expect(api.listRefreshRuns).toHaveBeenCalledWith(
      1,
      expect.any(AbortSignal),
    ),
  )
  await screen.findByRole('button', { name: '查看任务 history-0' })
  fireEvent.click(screen.getByRole('button', { name: '最近记录' }))
  await screen.findByRole('button', { name: '查看任务 history-0' })
  fireEvent.click(screen.getByRole('button', { name: '查看任务 history-0' }))
  await screen.findByText(/SSE:600000/)
  fireEvent.click(screen.getByRole('button', { name: '下一页明细' }))
  await waitFor(() =>
    expect(api.listRefreshFailures).toHaveBeenCalledWith(
      'history-0',
      50,
      expect.any(AbortSignal),
    ),
  )
  await screen.findByText(/SSE:600000/)
  fireEvent.click(screen.getByRole('button', { name: '明细首页' }))
  await waitFor(() =>
    expect(api.listRefreshFailures).toHaveBeenLastCalledWith(
      'history-0',
      0,
      expect.any(AbortSignal),
    ),
  )
})
it('页面隐藏取消查询，恢复可见重新查询，卸载停止轮询', async () => {
  const { unmount } = render(<RefreshMonitor onReload={vi.fn()} />)
  await waitFor(() => expect(api.getRefreshStatus).toHaveBeenCalledTimes(1))
  const firstSignal = vi.mocked(api.getRefreshStatus).mock.calls[0][0]!
  Object.defineProperty(document, 'hidden', { configurable: true, value: true })
  fireEvent(document, new Event('visibilitychange'))
  expect(firstSignal.aborted).toBe(true)
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    value: false,
  })
  fireEvent(document, new Event('visibilitychange'))
  await waitFor(() => expect(api.getRefreshStatus).toHaveBeenCalledTimes(2))
  unmount()
  expect(vi.mocked(api.getRefreshStatus).mock.calls[1][0]!.aborted).toBe(true)
})
it('失败明细请求失败可重试并查看期货详情', async () => {
  vi.mocked(api.getRefreshStatus).mockResolvedValue({
    stock: null,
    futures: { ...run, kind: 'FUTURES' },
    futures_enabled: true,
  })
  vi.mocked(api.listRefreshFailures).mockRejectedValueOnce(new Error('offline'))
  render(<RefreshMonitor onReload={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
  await screen.findByRole('button', { name: '查看期货详情' })
  fireEvent.click(screen.getByRole('button', { name: '查看期货详情' }))
  await screen.findByText('失败明细暂不可用')
  fireEvent.click(screen.getByRole('button', { name: '重试明细' }))
  await screen.findByText('暂无已记录的失败明细')
})
it('过期的运行记录允许显式尝试更新，由服务端防重', async () => {
  vi.mocked(api.getRefreshStatus).mockResolvedValue({
    stock: { ...run, heartbeat_at: '2000-01-01T00:00:00Z' },
    futures: null,
    futures_enabled: true,
  })
  render(<RefreshMonitor onReload={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
  await screen.findByText(/进度记录已过期/)
  expect(screen.getByRole('button', { name: '更新全部股票' })).toBeEnabled()
})
it('长时间查询失败后仍刷新时钟并解除过期记录的按钮锁定', async () => {
  vi.useFakeTimers()
  try {
    const now = new Date()
    vi.setSystemTime(now)
    vi.mocked(api.getRefreshStatus)
      .mockResolvedValueOnce({
        stock: { ...run, heartbeat_at: now.toISOString() },
        futures: null,
        futures_enabled: true,
      })
      .mockRejectedValue(new Error('offline'))
    const { act } = await import('@testing-library/react')
    render(<RefreshMonitor onReload={vi.fn()} />)
    await act(async () => {
      await Promise.resolve()
    })
    fireEvent.click(screen.getByRole('button', { name: /数据更新/ }))
    await act(async () => {
      await Promise.resolve()
    })
    expect(screen.getByRole('button', { name: '更新全部股票' })).toBeDisabled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(90000)
    })
    expect(screen.getByRole('button', { name: '更新全部股票' })).toBeEnabled()
  } finally {
    vi.useRealTimers()
  }
})
