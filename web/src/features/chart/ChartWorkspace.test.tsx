import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import {
  activateChartBoard,
  createChartBoard,
  deleteChartBoard,
  listChartBoards,
  updateChartBoard,
  type BoardConfig,
  type ChartResult,
} from '../../api/client'
import { ChartWorkspace } from './ChartWorkspace'
import { useBoards } from './useBoards'

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

let server: { boards: { id: number; name: string; config: BoardConfig }[]; activeId: number }
let nextId: number

function stateOf() {
  return { boards: structuredClone(server.boards), active_id: server.activeId }
}

beforeEach(() => {
  server = { boards: [], activeId: 0 }
  nextId = 1
  vi.mocked(listChartBoards).mockReset().mockImplementation(async () => stateOf())
  vi.mocked(createChartBoard).mockReset().mockImplementation(async (name: string, config: BoardConfig) => {
    const board = { id: nextId++, name, config: structuredClone(config) }
    server.boards.push(board)
    server.activeId = board.id
    return stateOf()
  })
  vi.mocked(updateChartBoard).mockReset().mockImplementation(
    async (id: number, changes: { name?: string; config?: BoardConfig }) => {
      const board = server.boards.find((b) => b.id === id)
      if (!board) throw new Error('看板不存在')
      if (changes.name !== undefined) board.name = changes.name
      if (changes.config !== undefined) board.config = structuredClone(changes.config)
      return stateOf()
    },
  )
  vi.mocked(activateChartBoard).mockReset().mockImplementation(async (id: number) => {
    server.activeId = id
    return stateOf()
  })
  vi.mocked(deleteChartBoard).mockReset().mockImplementation(async (id: number) => {
    server.boards = server.boards.filter((b) => b.id !== id)
    if (server.activeId === id) server.activeId = server.boards[0]?.id ?? 0
    return stateOf()
  })
})

vi.mock('./FinancialChart', () => ({
  FinancialChart: ({ bars, onLoadMore }: { bars: unknown[]; onLoadMore: () => void }) => (
    <div>
      <div data-testid="financial-chart">{bars.length} bars</div>
      <button onClick={onLoadMore} type="button">
        触发自动加载
      </button>
    </div>
  ),
}))

type HarnessProps = Omit<ComponentProps<typeof ChartWorkspace>, 'board'>

function Harness(props: HarnessProps) {
  const board = useBoards({ defaultSymbol: 'SZSE:002415' })
  return <ChartWorkspace {...props} board={board} />
}

const response: ChartResult = {
  instrument: {
    instrument: 'SZSE:002415',
    code: '002415',
    name: '海康威视',
    exchange: 'SZSE',
    board: 'MAIN',
    lot_size: 100,
  },
  timeframe: 'DAY',
  price_view: 'QFQ',
  data_version: 17,
  bars: [
    {
      open_time: '2026-09-11T00:00:00Z',
      close_time: '2026-09-11T07:00:00Z',
      open: 33,
      high: 34,
      low: 32.8,
      close: 33.25,
      volume: 43390000,
      amount: 1440000000,
      trading_status: 0,
    },
  ],
  series: [],
  has_more: false,
  next_before: null,
}

const baseProps = {
  instrument: 'SZSE:002415',
  onStateChange: vi.fn(),
  onToggleWatch: vi.fn(),
  watched: false,
}

describe('ChartWorkspace', () => {
  it('加载默认均线并允许切换周期', async () => {
    const query = vi.fn().mockResolvedValue(response)
    render(<Harness {...baseProps} query={query} />)

    await waitFor(() =>
      expect(query).toHaveBeenCalledWith(
        expect.objectContaining({
          instrument: 'SZSE:002415',
          timeframe: 'DAY',
          price_view: 'QFQ',
          indicators: [
            { kind: 'SMA', period: 5 },
            { kind: 'SMA', period: 20 },
            { kind: 'SMA', period: 60 },
          ],
        }),
        expect.any(AbortSignal),
      ),
    )
    expect(await screen.findByText('海康威视')).toBeVisible()
    expect(screen.getByTestId('financial-chart')).toHaveTextContent('1 bars')

    fireEvent.click(screen.getByRole('button', { name: '周线' }))
    await waitFor(() =>
      expect(query).toHaveBeenLastCalledWith(
        expect.objectContaining({ timeframe: 'WEEK' }),
        expect.any(AbortSignal),
      ),
    )
  })

  it('图表头部 ★ 反映自选状态并回调切换', async () => {
    const onToggleWatch = vi.fn()
    const query = vi.fn().mockResolvedValue(response)
    const props: HarnessProps = { ...baseProps, onToggleWatch, query }
    const { rerender } = render(<Harness {...props} />)

    const star = await screen.findByRole('button', { name: '添加自选' })
    expect(star).toHaveAttribute('aria-pressed', 'false')
    expect(star).toHaveTextContent('☆')
    fireEvent.click(star)
    expect(onToggleWatch).toHaveBeenCalledTimes(1)

    rerender(<Harness {...props} watched />)
    const filled = screen.getByRole('button', { name: '移除自选' })
    expect(filled).toHaveAttribute('aria-pressed', 'true')
    expect(filled).toHaveTextContent('★')
    fireEvent.click(filled)
    expect(onToggleWatch).toHaveBeenCalledTimes(2)
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
    render(<Harness {...baseProps} query={query} />)

    fireEvent.click(await screen.findByRole('button', { name: '触发自动加载' }))
    await waitFor(() =>
      expect(query).toHaveBeenLastCalledWith(
        expect.objectContaining({
          before: first.next_before,
          data_version: 17,
        }),
        expect.any(AbortSignal),
      ),
    )
    expect(screen.getByTestId('financial-chart')).toHaveTextContent('2 bars')
  })

  it('自动加载进行中不重复触发请求', async () => {
    let resolveOlder!: (value: ChartResult) => void
    const first = { ...response, has_more: true, next_before: response.bars[0].close_time }
    const query = vi
      .fn()
      .mockResolvedValueOnce(first)
      .mockReturnValueOnce(
        new Promise<ChartResult>((resolve) => {
          resolveOlder = resolve
        }),
      )
    render(<Harness {...baseProps} query={query} />)

    const trigger = await screen.findByRole('button', { name: '触发自动加载' })
    fireEvent.click(trigger)
    fireEvent.click(trigger)
    expect(query).toHaveBeenCalledTimes(2)

    await act(async () =>
      resolveOlder({
        ...response,
        bars: [{ ...response.bars[0], close_time: '2026-09-10T07:00:00Z' }],
        has_more: false,
        next_before: null,
      }),
    )
    expect(screen.getByTestId('financial-chart')).toHaveTextContent('2 bars')
  })

  it('没有更多历史数据时不触发自动加载', async () => {
    const query = vi.fn().mockResolvedValue(response)
    render(<Harness {...baseProps} query={query} />)

    fireEvent.click(await screen.findByRole('button', { name: '触发自动加载' }))
    expect(query).toHaveBeenCalledTimes(1)
  })

  it('显示服务错误并同步复权方式', async () => {
    const onStateChange = vi.fn()
    const query = vi.fn().mockRejectedValue(new Error('行情版本不存在'))
    render(<Harness {...baseProps} onStateChange={onStateChange} query={query} />)

    expect(await screen.findByText('行情版本不存在')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '不复权' }))
    expect(onStateChange).toHaveBeenCalledWith('DAY', 'RAW')
  })

  it('切换周期后忽略尚未完成的旧分页响应', async () => {
    let resolveOlder!: (value: ChartResult) => void
    const olderPromise = new Promise<ChartResult>((resolve) => {
      resolveOlder = resolve
    })
    const first = { ...response, has_more: true, next_before: response.bars[0].close_time }
    const weekly = { ...response, timeframe: 'WEEK' as const, bars: [{ ...response.bars[0], close: 40 }] }
    const query = vi.fn((input: { before?: string; timeframe: string }) => {
      if (input.before) return olderPromise
      return Promise.resolve(input.timeframe === 'WEEK' ? weekly : first)
    })
    render(<Harness {...baseProps} query={query} />)

    fireEvent.click(await screen.findByRole('button', { name: '触发自动加载' }))
    fireEvent.click(screen.getByRole('button', { name: '周线' }))
    await waitFor(() =>
      expect(query).toHaveBeenCalledWith(
        expect.objectContaining({ timeframe: 'WEEK' }),
        expect.any(AbortSignal),
      ),
    )
    await act(async () =>
      resolveOlder({ ...response, bars: [{ ...response.bars[0], close_time: '2026-09-10T07:00:00Z' }] }),
    )

    expect(screen.getByTestId('financial-chart')).toHaveTextContent('1 bars')
  })
})

it('keeps configured indicators on stock switch and restores only manual saves', async () => {
  const query = vi.fn().mockResolvedValue(response)
  const props: HarnessProps = { ...baseProps, query }
  const view = render(<Harness {...props} />)
  await screen.findByTestId('financial-chart')
  fireEvent.click(screen.getByRole('button', { name: /指标/ }))
  fireEvent.click(screen.getByRole('button', { name: '添加 MACD 12, 26, 9' }))
  await waitFor(() =>
    expect(query).toHaveBeenLastCalledWith(
      expect.objectContaining({
        indicators: expect.arrayContaining([expect.objectContaining({ kind: 'MACD' })]),
      }),
      expect.anything(),
    ),
  )
  fireEvent.click(screen.getByRole('button', { name: '保存' }))
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已保存'))
  expect(updateChartBoard).toHaveBeenCalledWith(
    expect.any(Number),
    expect.objectContaining({
      config: expect.objectContaining({
        indicators: expect.arrayContaining([expect.objectContaining({ kind: 'MACD' })]),
      }),
    }),
  )
  view.rerender(<Harness {...props} instrument="SSE:600938" />)
  await waitFor(() =>
    expect(query).toHaveBeenLastCalledWith(
      expect.objectContaining({
        instrument: 'SSE:600938',
        indicators: expect.arrayContaining([expect.objectContaining({ kind: 'MACD' })]),
      }),
      expect.anything(),
    ),
  )
  expect(screen.getByText('已保存')).toBeVisible()
  view.unmount()
  render(<Harness {...props} />)
  await waitFor(() =>
    expect(query).toHaveBeenLastCalledWith(
      expect.objectContaining({
        indicators: expect.arrayContaining([expect.objectContaining({ kind: 'MACD' })]),
      }),
      expect.anything(),
    ),
  )
})


it.each([
  [100, 110, 'quote-up', '+10.00 (+10.00%)'],
  [100, 90, 'quote-down', '-10.00 (-10.00%)'],
  [100, 100, 'quote-flat', '0.00 (0.00%)'],
  [0, 100, 'quote-flat', '—'],
  [null, 100, 'quote-flat', '—'],
])('顶部报价以前收盘 %s 计算最新收盘 %s 的涨跌', async (previous, close, tone, text) => {
  const latest = { ...response.bars[0], open: 120, close }
  const bars = previous === null ? [latest] : [
    { ...latest, close_time: '2026-09-10T07:00:00Z', close: previous }, latest,
  ]
  render(<Harness {...baseProps} query={vi.fn().mockResolvedValue({ ...response, bars })} />)
  const quote = await screen.findByLabelText('最新行情')
  expect(quote).toHaveClass(tone)
  expect(quote).toHaveTextContent(text)
  expect(quote.closest('.security-heading')).toContainElement(screen.getByText('海康威视'))
  expect(quote).toHaveTextContent('日涨跌')
})


it('周线报价按前一周收盘计算并标注周涨跌', async () => {
  const bars = [
    { ...response.bars[0], close_time: '2026-09-04T07:00:00Z', close: 100 },
    { ...response.bars[0], open: 120, close: 110 },
  ]
  render(<Harness {...baseProps} query={vi.fn().mockResolvedValue({ ...response, bars })} />)
  await screen.findByLabelText('最新行情')
  fireEvent.click(screen.getByRole('button', { name: '周线' }))
  const quote = await screen.findByLabelText('最新行情')
  expect(quote).toHaveTextContent('周涨跌 +10.00 (+10.00%)')
})
