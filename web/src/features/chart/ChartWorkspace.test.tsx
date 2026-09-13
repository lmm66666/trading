import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ChartResult } from '../../api/client'
import { ChartWorkspace } from './ChartWorkspace'

vi.mock('./FinancialChart', () => ({
  FinancialChart: ({ bars }: { bars: unknown[] }) => <div data-testid="financial-chart">{bars.length} bars</div>,
}))

const response: ChartResult = {
  instrument: {
    instrument: 'SZSE:002415', code: '002415', name: '海康威视', exchange: 'SZSE', board: 'MAIN', lot_size: 100,
  },
  timeframe: 'DAY',
  price_view: 'QFQ',
  data_version: 17,
  bars: [
    { open_time: '2026-09-11T00:00:00Z', close_time: '2026-09-11T07:00:00Z', open: 33, high: 34, low: 32.8, close: 33.25, volume: 43390000, amount: 1440000000, trading_status: 0 },
  ],
  series: [],
  has_more: false,
  next_before: null,
}

describe('ChartWorkspace', () => {
  it('加载默认均线并允许切换周期', async () => {
    const query = vi.fn().mockResolvedValue(response)
    render(
      <ChartWorkspace
        instrument="SZSE:002415"
        initialPriceView="QFQ"
        initialTimeframe="DAY"
        onStateChange={vi.fn()}
        query={query}
      />,
    )

    await waitFor(() => expect(query).toHaveBeenCalledWith(expect.objectContaining({
      instrument: 'SZSE:002415', timeframe: 'DAY', price_view: 'QFQ',
      indicators: [{ kind: 'SMA', period: 5 }, { kind: 'SMA', period: 20 }, { kind: 'SMA', period: 60 }],
    }), expect.any(AbortSignal)))
    expect(await screen.findByText('海康威视')).toBeVisible()
    expect(screen.getByTestId('financial-chart')).toHaveTextContent('1 bars')

    fireEvent.click(screen.getByRole('button', { name: '周线' }))
    await waitFor(() => expect(query).toHaveBeenLastCalledWith(expect.objectContaining({ timeframe: 'WEEK' }), expect.any(AbortSignal)))
  })

  it('按固定数据版本向前翻页并合并行情', async () => {
    const first = { ...response, has_more: true, next_before: '2026-09-11T07:00:00Z' }
    const older = {
      ...response,
      bars: [{ ...response.bars[0], open_time: '2026-09-10T00:00:00Z', close_time: '2026-09-10T07:00:00Z' }],
      has_more: false,
      next_before: null,
    }
    const query = vi.fn().mockResolvedValueOnce(first).mockResolvedValueOnce(older)
    render(<ChartWorkspace instrument="SZSE:002415" initialPriceView="QFQ" initialTimeframe="DAY" onStateChange={vi.fn()} query={query} />)

    fireEvent.click(await screen.findByRole('button', { name: '加载更早行情' }))
    await waitFor(() => expect(query).toHaveBeenLastCalledWith(expect.objectContaining({
      before: first.next_before, data_version: 17,
    }), expect.any(AbortSignal)))
    expect(screen.getByTestId('financial-chart')).toHaveTextContent('2 bars')
  })

  it('显示服务错误并同步复权方式', async () => {
    const onStateChange = vi.fn()
    const query = vi.fn().mockRejectedValue(new Error('行情版本不存在'))
    render(<ChartWorkspace instrument="SZSE:002415" initialPriceView="QFQ" initialTimeframe="DAY" onStateChange={onStateChange} query={query} />)

    expect(await screen.findByText('行情版本不存在')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '不复权' }))
    expect(onStateChange).toHaveBeenCalledWith('DAY', 'RAW')
  })

  it('切换周期后忽略尚未完成的旧分页响应', async () => {
    let resolveOlder!: (value: ChartResult) => void
    const olderPromise = new Promise<ChartResult>((resolve) => { resolveOlder = resolve })
    const first = { ...response, has_more: true, next_before: response.bars[0].close_time }
    const weekly = { ...response, timeframe: 'WEEK' as const, bars: [{ ...response.bars[0], close: 40 }] }
    const query = vi.fn((input: { before?: string; timeframe: string }) => {
      if (input.before) return olderPromise
      return Promise.resolve(input.timeframe === 'WEEK' ? weekly : first)
    })
    render(<ChartWorkspace instrument="SZSE:002415" initialPriceView="QFQ" initialTimeframe="DAY" onStateChange={vi.fn()} query={query} />)

    fireEvent.click(await screen.findByRole('button', { name: '加载更早行情' }))
    fireEvent.click(screen.getByRole('button', { name: '周线' }))
    await waitFor(() => expect(query).toHaveBeenCalledWith(expect.objectContaining({ timeframe: 'WEEK' }), expect.any(AbortSignal)))
    await act(async () => resolveOlder({ ...response, bars: [{ ...response.bars[0], close_time: '2026-09-10T07:00:00Z' }] }))

    expect(screen.getByTestId('financial-chart')).toHaveTextContent('1 bars')
  })
})
