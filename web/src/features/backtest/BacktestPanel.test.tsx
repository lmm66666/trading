import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  createBacktestRun,
  fetchRunPage,
  getRun,
  listStrategies,
  searchInstruments,
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
    subscribeCrosshairMove: vi.fn(),
    unsubscribeCrosshairMove: vi.fn(),
  })),
}))

vi.mock('../../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/client')>()),
  listStrategies: vi.fn(),
  searchInstruments: vi.fn(),
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

const fmt = (date: Date) =>
  `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`

function today(): Date {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return d
}

function monthsAgo(base: Date, months: number): Date {
  const first = new Date(base.getFullYear(), base.getMonth(), 1)
  first.setMonth(first.getMonth() - months)
  const daysIn = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()
  first.setDate(Math.min(base.getDate(), daysIn))
  return first
}

/** 通过 RangePicker 预设选择时间窗口并确定 */
function pickPresetRange(label: string) {
  fireEvent.click(screen.getByRole('button', { name: '回测时间范围' }))
  fireEvent.click(screen.getByRole('button', { name: label }))
  fireEvent.click(screen.getByRole('button', { name: '确定' }))
}

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
  // 目录加载后 StrategyForm 自动选中第一个策略，等待选中后的元信息出现
  await screen.findByText(/主周期 日线/)
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
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
    fireEvent.click(screen.getByRole('button', { name: /成交与费用设置/ }))
    expect(screen.getByLabelText('每手股数')).toHaveValue(200)
    expect(screen.getByLabelText('持有期（根）')).toHaveAttribute('placeholder', '策略默认 10')

    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))

    await vi.waitFor(() => expect(onRunIdChange).toHaveBeenCalledWith('bt-1'))
    expect(createBacktestRun).toHaveBeenCalledWith({
      instrument: 'SSE:600000',
      strategy: 'daily_b1_buy',
      strategy_version: '1',
      idempotency_key: 'bt-uuid',
      start: `${fmt(monthsAgo(today(), 1))}T00:00:00Z`,
      end: `${fmt(today())}T00:00:00Z`,
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

  it('高级费用设置默认折叠，展开后可编辑并随提交上送', async () => {
    vi.mocked(createBacktestRun).mockResolvedValue({ run_id: 'bt-1', status: 'PENDING' })
    await renderWithCatalog()

    const toggle = screen.getByRole('button', { name: /成交与费用设置/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByLabelText('佣金（%）')).toBeNull()

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    fireEvent.change(screen.getByLabelText('佣金（%）'), { target: { value: '0.10' } })

    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    await vi.waitFor(() => expect(createBacktestRun).toHaveBeenCalled())
    expect(createBacktestRun).toHaveBeenCalledWith(
      expect.objectContaining({ config: expect.objectContaining({ commission_bps: 10 }) }),
    )
  })

  it('越界输入不发出请求', async () => {
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('初始资金（元）'), { target: { value: '500' } })
    pickPresetRange('近 1 月')
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
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    expect(screen.getByText('请先选择回测标的')).toBeVisible()
    expect(createBacktestRun).not.toHaveBeenCalled()
  })

  it('幂等冲突时显示可读提示', async () => {
    vi.mocked(createBacktestRun).mockRejectedValue(new Error('IDEMPOTENCY_CONFLICT'))
    await renderWithCatalog()
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    await screen.findByText('任务输入与已有任务冲突，请重新提交')
  })

  it('未发起任务时展示空态引导', async () => {
    await renderWithCatalog()
    expect(screen.getByText('尚未发起回测')).toBeVisible()
  })
})

describe('BacktestPanel 结果', () => {
  it('SUCCEEDED 后展示 summary 指标卡，空比例显示 —', async () => {
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    render(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SSE:600000" />)

    await screen.findByText('总收益率')
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


describe('回测工作台恢复与报告隔离', () => {
  it('恢复任务不闪现空态，明确不存在清理引用', async () => {
    let reject!: (reason: unknown) => void
    vi.mocked(getRun).mockReturnValue(new Promise((_, fail) => { reject = fail }))
    const changed = vi.fn()
    render(<BacktestPanel runId="gone" onRunIdChange={changed} selectedSymbol={null} />)
    expect(screen.queryByText('尚未发起回测')).toBeNull()
    expect(screen.getByText('正在加载上次回测…')).toBeVisible()
    await act(async () => reject(new ApiError(404, 'NOT_FOUND')))
    expect(changed).toHaveBeenCalledWith(null)
    expect(screen.getByText('上次回测记录已不可用，可重新发起回测')).toBeVisible()
  })

  it('新任务运行及失败保留报告，成功切换后不混用旧资源', async () => {
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    const { rerender } = render(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol={null} />)
    await screen.findByText('15.23%')
    vi.mocked(getRun).mockResolvedValue({ ...succeededStatus, run_id: 'r2', status: 'FAILED', summary: undefined })
    rerender(<BacktestPanel runId="r2" onRunIdChange={noop} selectedSymbol={null} />)
    expect(screen.getByText('15.23%')).toBeVisible()
    await screen.findByText('失败')
    expect(screen.getByText('15.23%')).toBeVisible()
    vi.mocked(getRun).mockResolvedValue({ ...succeededStatus, run_id: 'r3', summary: { ...succeededStatus.summary!, total_return: 0.3 } })
    rerender(<BacktestPanel runId="r3" onRunIdChange={noop} selectedSymbol={null} />)
    await screen.findByText('30.00%')
    expect(screen.queryByText('15.23%')).toBeNull()
    expect(fetchRunPage).toHaveBeenCalledWith('backtest', 'r3', 'equity', undefined, 1000)
    expect(screen.getByText(/原始条件未保存/)).toBeVisible()
  })

  it('编辑草稿后刷新恢复草稿，报告仍使用冻结提交条件', async () => {
    vi.mocked(createBacktestRun).mockResolvedValue({ run_id: 'r1', status: 'PENDING' })
    const first = render(<BacktestPanel runId={null} onRunIdChange={noop} selectedSymbol="SSE:600000" />)
    await screen.findByText(/主周期 日线/)
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
    await vi.waitFor(() => expect(createBacktestRun).toHaveBeenCalled())
    fireEvent.change(screen.getByLabelText('初始资金（元）'), { target: { value: '2000000' } })
    first.unmount()
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    render(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SZSE:000001" />)
    await screen.findByText('15.23%')
    expect(screen.getByLabelText('初始资金（元）')).toHaveValue(2000000)
    expect(screen.getByText(/初始资金 1,000,000 元/)).toBeInTheDocument()
    expect(screen.getByText(/本机提交记录/)).toBeInTheDocument()
  })
})

it('证券搜索选择保留名称、使用证券手数，日期和参数直接编辑并提交', async () => {
  vi.mocked(listStrategies).mockResolvedValue([{ ...definition, parameters: { volume_period: { default: 10, min: 1, max: 50, integer: true } } }])
  vi.mocked(searchInstruments).mockResolvedValue([{ instrument: 'SZSE:300750', name: '宁德时代', code: '300750', exchange: 'SZSE', board: 'CHINEXT', lot_size: 200 }])
  vi.mocked(createBacktestRun).mockResolvedValue({ run_id: 'r1', status: 'PENDING' })
  await renderWithCatalog()
  fireEvent.click(screen.getByRole('button', { name: /SSE:600000.*更换/ }))
  fireEvent.change(screen.getByRole('combobox', { name: '搜索股票' }), { target: { value: '300750' } })
  fireEvent.click(await screen.findByRole('option', { name: /宁德时代/ }))
  fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
  fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-02-01' } })
  fireEvent.click(screen.getByText('策略参数'))
  fireEvent.change(screen.getByLabelText('参数 volume_period'), { target: { value: '2.5' } })
  fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
  expect(createBacktestRun).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('参数 volume_period'), { target: { value: '20' } })
  fireEvent.change(screen.getByLabelText('资金使用比例（%）'), { target: { value: '29.99' } })
  fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
  await vi.waitFor(() => expect(createBacktestRun).toHaveBeenCalledWith(expect.objectContaining({ instrument: 'SZSE:300750', parameters: { volume_period: 20 }, config: expect.objectContaining({ cash_fraction_bps: 2999, lot_size: 200 }) })))
  expect(screen.getByText('宁德时代')).toBeVisible()
})
it('目录失败可恢复，切策略清空旧参数', async () => {
  vi.mocked(listStrategies).mockRejectedValueOnce(Error('目录断开')).mockResolvedValue([definition, { ...definition, strategy: 'weekly_b1_buy', version: '2' }])
  render(<BacktestPanel runId={null} onRunIdChange={noop} selectedSymbol={null} />)
  fireEvent.click(await screen.findByRole('button', { name: '重试加载策略' }))
  await screen.findByText(/主周期 日线/)
  fireEvent.change(screen.getByLabelText('策略'), { target: { value: 'weekly_b1_buy@2' } })
  expect(screen.getByLabelText('策略')).toHaveValue('weekly_b1_buy@2')
})
it('隐藏时停止轮询，返回保留编辑条件并恢复查询', async () => {
  vi.mocked(getRun).mockResolvedValue({ ...succeededStatus, status: 'RUNNING', summary: undefined })
  const { rerender } = render(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SSE:600000" />)
  await screen.findByText('运行中')
  fireEvent.change(screen.getByLabelText('初始资金（元）'), { target: { value: '2000000' } })
  rerender(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SSE:600000" active={false} />)
  expect(screen.queryByRole('region', { name: '策略回测' })).toBeNull()
  vi.mocked(getRun).mockResolvedValue(succeededStatus)
  rerender(<BacktestPanel runId="r1" onRunIdChange={noop} selectedSymbol="SSE:600000" active />)
  await screen.findByText('15.23%')
  expect(screen.getByLabelText('初始资金（元）')).toHaveValue(2000000)
  expect(getRun).toHaveBeenCalledTimes(2)
})
it('晚到的默认标的信息补全名称和手数，不覆盖手动编辑', async () => {
  const { rerender } = render(<BacktestPanel runId={null} onRunIdChange={noop} selectedSymbol="SSE:600000" />)
  await screen.findByText(/主周期 日线/)
  rerender(<BacktestPanel runId={null} onRunIdChange={noop} selectedSymbol="SSE:600000" selectedName="浦发银行" defaultLotSize={200} />)
  expect(screen.getByText('浦发银行')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: /成交与费用设置/ }))
  expect(screen.getByLabelText('每手股数')).toHaveValue(200)
  fireEvent.change(screen.getByLabelText('每手股数'), { target: { value: '300' } })
  rerender(<BacktestPanel runId={null} onRunIdChange={noop} selectedSymbol="SSE:600000" selectedName="浦发银行" defaultLotSize={100} />)
  expect(screen.getByLabelText('每手股数')).toHaveValue(300)
})
it('请求标识生成失败也释放提交状态，修复后可以重新提交', async () => {
  vi.mocked(crypto.randomUUID).mockImplementationOnce(() => { throw Error('UUID unavailable') })
  await renderWithCatalog()
  pickPresetRange('近 1 月')
  fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
  expect(await screen.findByText('UUID unavailable')).toBeVisible()
  expect(screen.getByRole('button', { name: '发起回测' })).toBeEnabled()
  vi.mocked(createBacktestRun).mockResolvedValue({ run_id: 'r', status: 'PENDING' })
  fireEvent.click(screen.getByRole('button', { name: '发起回测' }))
  await vi.waitFor(() => expect(createBacktestRun).toHaveBeenCalled())
})
