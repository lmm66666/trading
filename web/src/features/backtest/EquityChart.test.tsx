import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchRunPage, type EquityPoint } from '../../api/client'
import { EquityChart } from './EquityChart'

const mocks = vi.hoisted(() => {
  const setData = vi.fn()
  const fitContent = vi.fn()
  const remove = vi.fn()
  const addSeries = vi.fn(() => ({ setData }))
  const subscribeCrosshairMove = vi.fn()
  const createChart = vi.fn(() => ({ addSeries, timeScale: () => ({ fitContent }), remove, subscribeCrosshairMove, unsubscribeCrosshairMove: vi.fn() }))
  return { subscribeCrosshairMove, addSeries, createChart, fitContent, remove, setData }
})

vi.mock('lightweight-charts', () => ({
  ColorType: { Solid: 'solid' },
  LineSeries: 'line',
  createChart: mocks.createChart,
}))

vi.mock('../../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/client')>()),
  fetchRunPage: vi.fn(),
}))

const equityOf = (items: Array<[string, number]>, nextSequence?: number) => ({
  items: items.map(([time, equity]) => ({ time, equity, cash: equity, position_value: 0 }) satisfies EquityPoint),
  ...(nextSequence === undefined ? {} : { next_sequence: nextSequence }),
})

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(fetchRunPage).mockResolvedValue(equityOf([['2026-01-01T00:00:00Z', 1_000_000_000]]))
})

describe('EquityChart', () => {
  it('连续分页拉取权益点直到没有 next_sequence，金额换算为元后入图', async () => {
    vi.mocked(fetchRunPage)
      .mockResolvedValueOnce(equityOf([['2026-01-01T00:00:00Z', 1_000_000_000]], 2))
      .mockResolvedValueOnce(equityOf([['2026-01-02T00:00:00Z', 1_010_000_000]]))

    render(<EquityChart runId="r1" />)

    await waitFor(() => expect(fetchRunPage).toHaveBeenCalledTimes(2))
    expect(fetchRunPage).toHaveBeenNthCalledWith(1, 'backtest', 'r1', 'equity', undefined, 1000)
    expect(fetchRunPage).toHaveBeenNthCalledWith(2, 'backtest', 'r1', 'equity', 2, 1000)
    await waitFor(() => expect(mocks.setData).toHaveBeenCalled())
    expect(mocks.setData).toHaveBeenCalledWith([
      { time: '2026-01-01', value: 100000 },
      { time: '2026-01-02', value: 101000 },
    ])
    expect(mocks.fitContent).toHaveBeenCalled()
  })

  it('加载失败时显示错误', async () => {
    vi.mocked(fetchRunPage).mockRejectedValue(new Error('equity broken'))
    const { findByText } = render(<EquityChart runId="r1" />)
    expect(await findByText('equity broken')).toBeVisible()
  })

  it('卸载时销毁图表并停止状态更新', async () => {
    let resolvePage: ((page: ReturnType<typeof equityOf>) => void) | undefined
    vi.mocked(fetchRunPage).mockReturnValue(new Promise<ReturnType<typeof equityOf>>((resolve) => { resolvePage = resolve }))
    const { unmount } = render(<EquityChart runId="r1" />)
    unmount()
    resolvePage?.(equityOf([['2026-01-01T00:00:00Z', 1_000_000_000]]))
    await waitFor(() => expect(fetchRunPage).toHaveBeenCalled())
    expect(mocks.setData).not.toHaveBeenCalled()
    expect(mocks.remove).toHaveBeenCalled()
  })
})

it('达到读取上限明确显示不完整，不能冒充全部结果', async () => {
  let page = 0
  vi.mocked(fetchRunPage).mockImplementation(async () => equityOf([[`2026-01-${String(++page).padStart(2, '0')}T00:00:00Z`, 10000]], page))
  render(<EquityChart runId="r" />)
  expect(await screen.findByText(/曲线未完整/)).toBeVisible()
  expect(fetchRunPage).toHaveBeenCalledTimes(10)
})
it('加载失败可重试，十字线展示现金与持仓市值', async () => {
  vi.mocked(fetchRunPage).mockRejectedValueOnce(Error('temporary')).mockResolvedValue({ items: [{ time: '2026-01-01T00:00:00Z', equity: 1200000, cash: 500000, position_value: 700000 }] })
  render(<EquityChart runId="r" />)
  fireEvent.click(await screen.findByRole('button', { name: '重试权益曲线' }))
  await waitFor(() => expect(mocks.setData).toHaveBeenCalledWith([{ time: '2026-01-01', value: 120 }]))
  act(() => mocks.subscribeCrosshairMove.mock.lastCall![0]({ time: '2026-01-01' }))
  expect(screen.getByText(/现金 50.00 元/)).toBeVisible()
  expect(screen.getByText(/持仓市值 70.00 元/)).toBeVisible()
})
