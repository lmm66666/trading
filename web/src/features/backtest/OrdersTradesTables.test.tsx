import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchRunPage, type BacktestFill, type BacktestOrder } from '../../api/client'
import { OrdersTradesTables } from './OrdersTradesTables'

vi.mock('../../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/client')>()),
  fetchRunPage: vi.fn(),
}))

const orderOf = (id: string): BacktestOrder => ({
  id, instrument: 'SSE:600000', side: 1, quantity: 100,
  created_at: '2026-03-01T01:00:00Z', reason: 'b1-buy',
  attempted_at: '2026-03-02T01:00:00Z', final_reason: 1,
})

const fillOf = (id: string): BacktestFill => ({
  id, order_id: 'o-1', instrument: 'SSE:600000', side: 2, time: '2026-03-03T01:00:00Z',
  price: 250000, quantity: 100, gross: 2_500_000, commission: 7500, stamp_duty: 12500, transfer_fee: 0,
})

beforeEach(() => {
  vi.clearAllMocks()
})


describe('OrdersTradesTables', () => {
  it('默认成交、订单按需加载，保留价格精度和分页状态', async () => {
    vi.mocked(fetchRunPage).mockImplementation(async (_kind, _run, resource) => resource === 'trades'
      ? { items: [{ ...fillOf('f-1'), price: 251234 }], next_sequence: 100 }
      : { items: [orderOf('o-1')] })
    render(<OrdersTradesTables runId="r1" />)
    expect(await screen.findByText('25.1234')).toBeVisible()
    expect(screen.getByText('250.00')).toBeVisible()
    expect(screen.getByText('已加载 1 条')).toBeVisible()
    expect(fetchRunPage).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('tab', { name: '订单记录' }))
    expect(await screen.findByText('已成交')).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: '成交记录' }))
    expect(screen.getByText('25.1234')).toBeVisible()
    expect(fetchRunPage).toHaveBeenCalledTimes(2)
  })
  it('分页错误保留行，重试同一游标', async () => {
    vi.mocked(fetchRunPage).mockResolvedValueOnce({ items: [fillOf('f-1')], next_sequence: 100 }).mockRejectedValueOnce(Error('temporary')).mockResolvedValueOnce({ items: [fillOf('f-2')] })
    render(<OrdersTradesTables runId="r1" />)
    fireEvent.click(await screen.findByRole('button', { name: '加载更多' }))
    fireEvent.click(await screen.findByRole('button', { name: '重试加载记录' }))
    expect(await screen.findByText('共 2 条')).toBeVisible()
    expect(fetchRunPage).toHaveBeenLastCalledWith('backtest', 'r1', 'trades', 100, 100)
  })
  it('首屏失败可重试，零成交明确说明', async () => {
    vi.mocked(fetchRunPage).mockRejectedValueOnce(Error('temporary')).mockResolvedValue({ items: [] })
    render(<OrdersTradesTables runId="r1" />)
    fireEvent.click(await screen.findByRole('button', { name: '重试加载记录' }))
    expect(await screen.findByText('本次回测没有成交记录')).toBeVisible()
  })
  it('旧任务续页错误不得污染新任务，隐藏时暂停首屏请求', async () => {
    let reject!: (error: unknown) => void
    vi.mocked(fetchRunPage).mockResolvedValueOnce({ items: [fillOf('f-1')], next_sequence: 100 }).mockReturnValueOnce(new Promise((_, fail) => { reject = fail })).mockResolvedValue({ items: [] })
    const { rerender } = render(<OrdersTradesTables runId="r1" />)
    fireEvent.click(await screen.findByRole('button', { name: '加载更多' }))
    rerender(<OrdersTradesTables runId="r2" active={false} />)
    await waitFor(() => expect(fetchRunPage).toHaveBeenCalledTimes(2))
    reject(Error('old error'))
    rerender(<OrdersTradesTables runId="r2" active />)
    expect(await screen.findByText('本次回测没有成交记录')).toBeVisible()
    expect(screen.queryByText(/old error/)).toBeNull()
  })
})
it('订单拒绝原因以中文解释，未知枚举保留编号', async () => {
  vi.mocked(fetchRunPage).mockImplementation(async (_kind, _run, resource) => ({ items: resource === 'orders' ? [{ ...orderOf('invalid'), final_reason: 14 }, { ...orderOf('future'), final_reason: 99 }] : [] }))
  render(<OrdersTradesTables runId="r" />)
  fireEvent.click(screen.getByRole('tab', { name: '订单记录' }))
  expect(await screen.findByText('行情数据无效')).toBeVisible()
  expect(screen.getByText('未知结果（99）')).toBeVisible()
})
