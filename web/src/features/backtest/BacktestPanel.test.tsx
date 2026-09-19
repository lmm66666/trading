import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createBacktestRun,
  fetchRunPage,
  getRun,
  listStrategies,
  type RunStatus,
  type StrategyDefinition,
} from '../../api/client'
import { BacktestPanel } from './BacktestPanel'

vi.mock('lightweight-charts', () => ({
  ColorType: { Solid: 'solid' },
  LineSeries: 'line',
  createChart: vi.fn(() => ({
    addSeries: vi.fn(() => ({ setData: vi.fn() })),
    timeScale: () => ({ fitContent: vi.fn() }),
    remove: vi.fn(),
  })),
}))

vi.mock('../../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/client')>()),
  listStrategies: vi.fn(),
  createBacktestRun: vi.fn(),
  getRun: vi.fn(),
  cancelRun: vi.fn(),
  fetchRunPage: vi.fn(),
}))

const definition: StrategyDefinition = {
  strategy: 'daily_b1_buy',
  version: '1',
  primary_timeframe: 'daily',
  warmup_bars: 60,
  default_hold_bars: 10,
  parameters: {},
  features: ['close'],
  auxiliary: [],
}

const succeededStatus: RunStatus = {
  run_id: 'r1', status: 'SUCCEEDED', kind: 'backtest', strategy: 'daily_b1_buy',
  strategy_version: '1', data_version: 7, engine_version: 'v1', attempts: 1,
  cancel_requested_at: null,
  summary: {
    total_return: 0.1523, annualized_return: null, maximum_drawdown: -0.08,
    closed_trades: 12, win_rate: null, profit_factor: 1.8,
    average_holding_bars: 10, has_open_position: false,
  },
}

const noop = () => {}

async function renderWithCatalog(props?: Partial<Parameters<typeof BacktestPanel>[0]>) {
  render(
    <BacktestPanel
      runId={null}
      onRunIdChange={noop}
      selectedSymbol="SSE:600000"
      defaultLotSize={200}
      {...props}
    />,
  )
  fireEvent.change(await screen.findByRole('combobox'), { target: { value: 'daily_b1_buy' } })
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(listStrategies).mockResolvedValue([definition])
  vi.mocked(fetchRunPage).mockResolvedValue({ items: [] })
  vi.spyOn(crypto, 'randomUUID').mockReturnValue('bt-uuid' as ReturnType<typeof crypto.randomUUID>)
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('BacktestPanel 表单', () => {
  it('默认带入当前证券与换算后的执行假设', async () => {
    vi.mocked(createBacktestRun).mockResolvedValue({ run_id: 'bt-1', status: 'PENDING' })
    const onRunIdChange = vi.fn()
    await renderWithCatalog({ onRunIdChange })

    expect(screen.getByText('SSE:600000')).toBeVisible()
    expect(screen.getByLabelText('每手股数')).toHaveValue(200)
    expect(screen.getByLabelText('持有期（根）')).toHaveAttribute('placeholder', '策略默认 10')

    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))

    await vi.waitFor(() => expect(onRunIdChange).toHaveBeenCalledWith('bt-1'))
    expect(createBacktestRun).toHaveBeenCalledWith({
      instrument: 'SSE:600000',
      strategy: 'daily_b1_buy',
      strategy_version: '1',
      idempotency_key: 'bt-uuid',
      start: '2026-01-01T00:00:00Z',
      end: '2026-06-01T00:00:00Z',
      parameters: undefined,
      config: {
        initial_cash: 10_000_000_000,      // 100 万元 → 缩放 10000
        cash_fraction_bps: 10000,
        commission_bps: 3,
        minimum_commission: 50_000,        // 5 元 → 缩放 10000
        stamp_duty_bps: 5,
        transfer_fee_bps: 0,
        slippage_bps: 5,
        lot_size: 200,
        hold_bars: 0,                      // 留空 = 策略默认
      },
    })
  })

  it('越界输入不发出请求', async () => {
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('初始资金（元）'), { target: { value: '500' } })
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    expect(screen.getByText('初始资金需在 1 千–10 亿元之间')).toBeVisible()
    expect(createBacktestRun).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('初始资金（元）'), { target: { value: '2000000000' } })
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    expect(screen.getByText('初始资金需在 1 千–10 亿元之间')).toBeVisible()
    expect(createBacktestRun).not.toHaveBeenCalled()
  })

  it('未选择证券时拦截提交', async () => {
    await renderWithCatalog({ selectedSymbol: null })
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    expect(screen.getByText('请选择证券与策略并检查参数')).toBeVisible()
    expect(createBacktestRun).not.toHaveBeenCalled()
  })

  it('幂等冲突时显示可读提示', async () => {
    vi.mocked(createBacktestRun).mockRejectedValue(new Error('IDEMPOTENCY_CONFLICT'))
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    await screen.findByText('任务输入与已有任务冲突，请重新提交')
  })
})

describe('BacktestPanel 结果', () => {
  it('SUCCEEDED 后展示 summary 指标卡，空比例显示 —', async () => {
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    render(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SSE:600000" />)

    await screen.findByText('总收益')
    expect(screen.getByText('15.23%')).toBeVisible()
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(2) // 年化收益、胜率
    expect(screen.getByText('-8.00%')).toBeVisible()
    expect(screen.getByText('1.8')).toBeVisible()
    expect(screen.getByText('10 根')).toBeVisible()
    expect(screen.getByText('12')).toBeVisible()
    expect(screen.getByText('空仓')).toBeVisible()
    expect(fetchRunPage).toHaveBeenCalledWith('backtest', 'r1', 'equity', undefined, 1000)
  })

  it('PENDING 状态下可取消任务', async () => {
    vi.useFakeTimers()
    vi.mocked(getRun).mockResolvedValue({
      run_id: 'r1', status: 'PENDING', kind: 'backtest', strategy: 'daily_b1_buy',
      strategy_version: '1', data_version: 7, engine_version: 'v1', attempts: 0,
      cancel_requested_at: null,
    })
    const { cancelRun } = await import('../../api/client')
    vi.mocked(cancelRun).mockResolvedValue(undefined)
    render(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SSE:600000" />)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    fireEvent.click(screen.getByRole('button', { name: '取消任务' }))
    await vi.waitFor(() => expect(cancelRun).toHaveBeenCalledWith('backtest', 'r1'))
  })
})
