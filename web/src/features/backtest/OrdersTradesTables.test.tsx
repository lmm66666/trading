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
  it('渲染订单与成交并映射枚举与金额', async () => {
    vi.mocked(fetchRunPage)
      .mockResolvedValueOnce({ items: [orderOf('o-1')] })
      .mockResolvedValueOnce({ items: [fillOf('f-1')] })

    render(<OrdersTradesTables runId="r1" />)

    // 'o-1' 同时出现在订单表 ID 列与成交表订单列，需用 findAllByText
    expect((await screen.findAllByText('o-1'))[0]).toBeVisible()
    expect(screen.getByText('买入')).toBeVisible()
    expect(screen.getByText('已成交')).toBeVisible()
    expect(await screen.findByText('f-1')).toBeVisible()
    expect(screen.getByText('25')).toBeVisible()          // 250000 缩放 → 25 元
    expect(screen.getByText('250')).toBeVisible()         // gross → 250 元
    expect(screen.getByText('卖出')).toBeVisible()
    expect(fetchRunPage).toHaveBeenCalledWith('backtest', 'r1', 'orders', undefined, 100)
    expect(fetchRunPage).toHaveBeenCalledWith('backtest', 'r1', 'trades', undefined, 100)
  })

  it('按游标加载更多订单', async () => {
    vi.mocked(fetchRunPage)
      .mockResolvedValueOnce({ items: [orderOf('o-1')], next_sequence: 100 })
      .mockResolvedValueOnce({ items: [orderOf('o-2')] })
      .mockResolvedValue({ items: [] })

    render(<OrdersTradesTables runId="r1" />)
    expect(await screen.findByText('o-1')).toBeVisible()

    fireEvent.click(screen.getAllByRole('button', { name: '加载更多' })[0])
    expect(await screen.findByText('o-2')).toBeVisible()
    expect(fetchRunPage).toHaveBeenCalledWith('backtest', 'r1', 'orders', 100, 100)

    await waitFor(() => expect(screen.queryByRole('button', { name: '加载更多' })).toBeNull())
  })

  it('分页失败时保留已加载行并提示', async () => {
    vi.mocked(fetchRunPage)
      .mockResolvedValueOnce({ items: [orderOf('o-1')], next_sequence: 100 })
      .mockRejectedValueOnce(new Error('orders broken'))
      .mockResolvedValue({ items: [] })

    render(<OrdersTradesTables runId="r1" />)
    expect(await screen.findByText('o-1')).toBeVisible()

    fireEvent.click(screen.getAllByRole('button', { name: '加载更多' })[0])
    expect(await screen.findByText(/orders broken/)).toBeVisible()
    expect(screen.getByText('o-1')).toBeVisible()
  })
})
