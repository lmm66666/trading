import { useState } from 'react'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  cancelRun,
  createScanRun,
  fetchSnapshotPage,
  getRun,
  listStrategies,
  type RunStatus,
  type StrategyDefinition,
} from '../../api/client'
import { ScanPanel } from './ScanPanel'

vi.mock('../../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/client')>()),
  listStrategies: vi.fn(),
  createScanRun: vi.fn(),
  getRun: vi.fn(),
  cancelRun: vi.fn(),
  fetchSnapshotPage: vi.fn(),
}))

const definition: StrategyDefinition = {
  strategy: 'daily_b1_buy',
  version: '1',
  primary_timeframe: 'daily',
  warmup_bars: 60,
  default_hold_bars: 10,
  parameters: { lookback_days: { default: 20, min: 5, max: 120, integer: true } },
  features: ['close'],
  auxiliary: [],
}

const snapshotKey = {
  snapshot_id: 'snap-1',
  strategy_id: 'daily_b1_buy',
  strategy_version: '1',
  parameters_hash: 'hash-1',
  as_of: '2026-06-01T00:00:00Z',
}

const pageOf = (instruments: Array<string | { instrument: string; name?: string }>, nextSequence?: number) => ({
  snapshot_id: 'snap-1',
  run_id: 'r1',
  key: snapshotKey,
  data_version: 7,
  rows: instruments.map((item) => ({
    ...(typeof item === 'string' ? { instrument: item } : item),
    signal_time: '2026-06-01T00:00:00Z',
    reason: 'b1-buy',
    values: {},
  })),
  failures: [],
  ...(nextSequence === undefined ? {} : { next_sequence: nextSequence }),
})

const runningStatus: RunStatus = {
  run_id: 'r1', status: 'RUNNING', kind: 'scan', strategy: 'daily_b1_buy',
  strategy_version: '1', data_version: 7, engine_version: 'v1', attempts: 2,
  cancel_requested_at: null,
}

const succeededStatus: RunStatus = {
  ...runningStatus,
  status: 'SUCCEEDED',
  snapshot_id: 'snap-1',
}

const partialStatus: RunStatus = {
  ...runningStatus,
  status: 'PARTIAL_SUCCEEDED',
  snapshot_id: 'snap-1',
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
  fireEvent.click(screen.getByRole('button', { name: '扫描时间范围' }))
  fireEvent.click(screen.getByRole('button', { name: label }))
  fireEvent.click(screen.getByRole('button', { name: '确定' }))
}

async function renderWithCatalog(props?: Partial<Parameters<typeof ScanPanel>[0]>) {
  render(<ScanPanel runId={null} onRunIdChange={noop} onSelectInstrument={noop} {...props} />)
  // 目录加载后自动选中第一个策略，等待参数输入出现
  await screen.findByRole('combobox', { name: '策略' })
  await screen.findByLabelText('参数 lookback_days')
  fireEvent.click(screen.getByText('策略参数'))
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  vi.mocked(listStrategies).mockResolvedValue([definition])
  vi.spyOn(crypto, 'randomUUID').mockReturnValue('uuid-123' as ReturnType<typeof crypto.randomUUID>)
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('ScanPanel 表单', () => {
  it('按默认值与 UTC 转换提交扫描任务并记录 run_id', async () => {
    vi.mocked(createScanRun).mockResolvedValue({ run_id: 'r9', status: 'PENDING' })
    const onRunIdChange = vi.fn()
    await renderWithCatalog({ onRunIdChange })

    pickPresetRange('近 1 月')
    const exchange = screen.getByRole('button', { name: '上交所 SSE' })
    fireEvent.click(exchange)
    expect(exchange).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))

    await screen.findByText('提交中…')
    await vi.waitFor(() => expect(onRunIdChange).toHaveBeenCalledWith('r9'))
    expect(createScanRun).toHaveBeenCalledWith({
      strategy: 'daily_b1_buy',
      strategy_version: '1',
      idempotency_key: 'uuid-123',
      from: `${fmt(monthsAgo(today(), 1))}T00:00:00Z`,
      as_of: `${fmt(today())}T00:00:00Z`,
      parameters: undefined,
      scope: { exchanges: ['SSE'], active_only: true, limit: 5000 },
    })
  })

  it('已填参数随提交体上送', async () => {
    vi.mocked(createScanRun).mockResolvedValue({ run_id: 'r9', status: 'PENDING' })
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '30' } })
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))

    await vi.waitFor(() => expect(createScanRun).toHaveBeenCalled())
    expect(createScanRun).toHaveBeenCalledWith(expect.objectContaining({ parameters: { lookback_days: 30 } }))
  })

  it('未选日期时不发出请求', async () => {
    await renderWithCatalog()
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    expect(screen.getByText('请选择开始与截止日期')).toBeVisible()
    expect(createScanRun).not.toHaveBeenCalled()
  })

  it('参数越界时拦截提交', async () => {
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '999' } })
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    expect(screen.getByText('策略参数超出范围，请修正后重试')).toBeVisible()
    expect(createScanRun).not.toHaveBeenCalled()
  })

  it('幂等冲突时显示可读提示', async () => {
    vi.mocked(createScanRun).mockRejectedValue(new Error('IDEMPOTENCY_CONFLICT'))
    await renderWithCatalog()
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    await screen.findByText('任务输入与已有任务冲突，请重新提交')
  })

  it('其他错误原样展示且保留表单', async () => {
    vi.mocked(createScanRun).mockRejectedValue(new Error('NETWORK_DOWN'))
    await renderWithCatalog()
    pickPresetRange('近 1 月')
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    await screen.findByText('NETWORK_DOWN')
    const trigger = screen.getByRole('button', { name: '扫描时间范围' })
    expect(trigger).toHaveTextContent(fmt(monthsAgo(today(), 1)))
    expect(trigger).toHaveTextContent(fmt(today()))
  })

  it('未发起任务时展示空态引导', async () => {
    await renderWithCatalog()
    expect(screen.getByText('尚未发起扫描')).toBeVisible()
  })

  it('配置栏策略标签不重复', async () => {
    await renderWithCatalog()
    expect(screen.getAllByText('策略')).toHaveLength(1)
  })
})

describe('ScanPanel 任务跟踪', () => {
  it('以 2 秒间隔轮询并在终态停止', async () => {
    vi.useFakeTimers()
    vi.mocked(getRun).mockResolvedValueOnce(runningStatus).mockResolvedValueOnce(succeededStatus)
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
    render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={noop} />)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(getRun).toHaveBeenCalledTimes(1)
    expect(getRun).toHaveBeenCalledWith('scan', 'r1')
    expect(screen.getByText('运行中')).toBeVisible()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(getRun).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000)
    })
    expect(getRun).toHaveBeenCalledTimes(2)
    expect(screen.getByText('已完成')).toBeVisible()
  })

  it('卸载后停止轮询', async () => {
    vi.useFakeTimers()
    vi.mocked(getRun).mockResolvedValue(runningStatus)
    const { unmount } = render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={noop} />)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    unmount()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(getRun).toHaveBeenCalledTimes(1)
  })

  it('连续 3 次网络错误后进入错误态并停止轮询', async () => {
    vi.useFakeTimers()
    vi.mocked(getRun).mockRejectedValue(new Error('network down'))
    render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={noop} />)
    for (let round = 0; round < 3; round += 1) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(round === 0 ? 0 : 2000)
      })
    }
    expect(getRun).toHaveBeenCalledTimes(3)
    expect(screen.getByText('network down')).toBeVisible()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(getRun).toHaveBeenCalledTimes(3)
  })

  it('运行中可取消任务', async () => {
    vi.useFakeTimers()
    vi.mocked(getRun).mockResolvedValue(runningStatus)
    vi.mocked(cancelRun).mockResolvedValue(undefined)
    render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={noop} />)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    fireEvent.click(screen.getByRole('button', { name: '取消任务' }))
    await vi.waitFor(() => expect(cancelRun).toHaveBeenCalledWith('scan', 'r1'))
  })
})

describe('ScanPanel 结果', () => {
  async function renderSucceeded(status: RunStatus, onSelectInstrument = noop) {
    vi.mocked(getRun).mockResolvedValue(status)
    render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={onSelectInstrument} />)
    await vi.waitFor(() => expect(fetchSnapshotPage).toHaveBeenCalled())
    await screen.findByRole('button', { name: 'SSE:600000' })
  }

  it('按快照 key 固定续页直到没有 next_sequence', async () => {
    vi.mocked(fetchSnapshotPage)
      .mockResolvedValueOnce(pageOf(['SSE:600000'], 100))
      .mockResolvedValueOnce(pageOf(['SSE:600001']))

    await renderSucceeded(succeededStatus)

    expect(fetchSnapshotPage).toHaveBeenNthCalledWith(1, {
      strategy: 'daily_b1_buy',
      strategy_version: '1',
      snapshot_id: 'snap-1',
      limit: 100,
    })
    expect(screen.getByRole('button', { name: 'SSE:600000' })).toBeVisible()

    fireEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await vi.waitFor(() =>
      expect(fetchSnapshotPage).toHaveBeenNthCalledWith(2, {
        strategy: 'daily_b1_buy',
        strategy_version: '1',
        parameters_hash: 'hash-1',
        snapshot_id: 'snap-1',
        after_sequence: 100,
        limit: 100,
      }),
    )
    expect(await screen.findByRole('button', { name: 'SSE:600001' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '加载更多' })).toBeNull()
  })

  it('续页失败时保留已加载行并提示', async () => {
    vi.mocked(fetchSnapshotPage)
      .mockResolvedValueOnce(pageOf(['SSE:600000'], 100))
      .mockRejectedValueOnce(new Error('page broken'))
    await renderSucceeded(succeededStatus)
    fireEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findByText(/page broken/)
    expect(screen.getByRole('button', { name: 'SSE:600000' })).toBeVisible()
  })

  it('点击入选行回调完整证券身份', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
    const onSelectInstrument = vi.fn()
    await renderSucceeded(succeededStatus, onSelectInstrument)
    fireEvent.click(screen.getByRole('button', { name: 'SSE:600000' }))
    expect(onSelectInstrument).toHaveBeenCalledWith('SSE:600000')
  })

  it('信号时间转换为本地时区展示', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
    await renderSucceeded(succeededStatus)
    const expected = new Date('2026-06-01T00:00:00Z').toLocaleString('zh-CN', { hour12: false })
    expect(screen.getByText(expected)).toBeVisible()
  })

  it('终态结果合并为单张结果卡且状态条唯一', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
    await renderSucceeded(succeededStatus)
    expect(screen.getAllByLabelText('任务状态')).toHaveLength(1)
    const card = screen.getByLabelText('扫描结果')
    expect(within(card).getByText('daily_b1_buy v1')).toBeVisible()
  })

  it('结果表格展示代码、名称、信号时间与信号原因', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf([{ instrument: 'SSE:600000', name: '浦发银行' }]))
    await renderSucceeded(succeededStatus)
    expect(screen.getByRole('columnheader', { name: '代码' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '名称' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '信号时间' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '信号原因' })).toBeVisible()
    expect(screen.getByText('浦发银行')).toBeVisible()
    expect(screen.getByText('b1-buy')).toBeVisible()
  })

  it('名称缺失时名称列回退显示证券代码', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
    await renderSucceeded(succeededStatus)
    // 代码列按钮与名称列均显示证券身份
    expect(screen.getAllByText('SSE:600000')).toHaveLength(2)
  })

  it('入选 0 只时展示空态与调整建议而非空表头', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf([]))
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={noop} />)
    await screen.findByText('无入选证券')
    expect(screen.getByText(/调整时间范围、策略参数或交易所范围/)).toBeVisible()
    expect(screen.queryByRole('table')).toBeNull()
  })

  it('结果加载中展示骨架行', async () => {
    vi.mocked(fetchSnapshotPage).mockReturnValue(new Promise(() => {}))
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    render(<ScanPanel runId="r1" onRunIdChange={noop} onSelectInstrument={noop} />)
    const status = await screen.findByRole('status', { name: '结果加载中' })
    expect(status).toBeVisible()
    expect(status.querySelectorAll('.skeleton-bar').length).toBeGreaterThan(0)
  })

  it('部分成功时展示失败计数，failures 默认折叠', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue({
      ...pageOf(['SSE:600000']),
      failures: [
        { instrument: 'SZSE:002399', code: 'DATA_MISSING', message: '缺少行情', retryable: true, name: '广发证券' },
      ],
    })
    await renderSucceeded(partialStatus)

    expect(screen.getByText('1 只证券处理失败')).toBeVisible()
    const detail = screen.getByText('缺少行情')
    expect(detail).not.toBeVisible()
    fireEvent.click(screen.getByText('处理失败 1 只'))
    expect(detail).toBeVisible()
    expect(screen.getByText('广发证券')).toBeVisible()
    expect(screen.getByText('是')).toBeVisible()
  })
})


describe('扫描工作台恢复与结果保留', () => {
  it('读取已有任务时不展示未扫描空态', async () => {
    vi.mocked(getRun).mockReturnValue(new Promise(() => {}))
    await renderWithCatalog({ runId: 'r1' })
    expect(screen.queryByText('尚未发起扫描')).toBeNull()
    expect(screen.getByText('正在加载上次扫描…')).toBeVisible()
  })
  it('明确的任务不存在立即清理引用并保留表单', async () => {
    vi.mocked(getRun).mockRejectedValue(new ApiError(404, 'NOT_FOUND'))
    const change = vi.fn()
    await renderWithCatalog({ runId: 'gone', onRunIdChange: change })
    await screen.findByText('上次扫描记录已不可用，可重新发起扫描')
    expect(change).toHaveBeenCalledWith(null)
    expect(getRun).toHaveBeenCalledTimes(1)
  })
  it('保存直接输入的日期与参数，重挂载后恢复', async () => {
    const first = render(<ScanPanel runId={null} onRunIdChange={noop} onSelectInstrument={noop} />)
    await screen.findByRole('combobox', { name: '策略' })
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    expect(screen.getByLabelText('开始日期')).toHaveValue('2026-01-01')
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByText('策略参数'))
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '35' } })
    expect(JSON.parse(localStorage.getItem('wb.scan_preferences')!).draft.from).toBe('2026-01-01')
    first.unmount()
    await renderWithCatalog()
    expect(screen.getByLabelText('开始日期')).toHaveValue('2026-01-01')
    expect(screen.getByLabelText('参数 lookback_days')).toHaveValue(35)
  })
  it('分页未完成时只声明已加载数量', async () => {
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000'], 100))
    await renderWithCatalog({ runId: 'r1' })
    expect(await screen.findByText('已加载 1 条')).toBeVisible()
    expect(screen.queryByText('入选 1 只')).toBeNull()
  })
  it('新快照失败保留旧结果，重试得到零行后替换', async () => {
    vi.mocked(getRun).mockResolvedValue(succeededStatus)
    vi.mocked(fetchSnapshotPage).mockResolvedValueOnce(pageOf(['SSE:600000'])).mockRejectedValueOnce(new Error('new snapshot failed')).mockResolvedValueOnce(pageOf([]))
    const props = { onRunIdChange: noop, onSelectInstrument: noop }
    const { rerender } = render(<ScanPanel {...props} runId="r1" />)
    await screen.findByRole('button', { name: 'SSE:600000' })
    vi.mocked(getRun).mockResolvedValue({ ...succeededStatus, run_id: 'r2', snapshot_id: 'snap-2' })
    rerender(<ScanPanel {...props} runId="r2" />)
    await screen.findByText(/new snapshot failed/)
    expect(screen.getByRole('button', { name: 'SSE:600000' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '重试读取结果' }))
    await screen.findByText('无入选证券')
    expect(screen.queryByRole('button', { name: 'SSE:600000' })).toBeNull()
  })
})


it('submission freezes source conditions while the draft remains editable', async () => {
  let finish!: (value: { run_id: string; status: string }) => void
  vi.mocked(createScanRun).mockReturnValue(new Promise((resolve) => { finish = resolve }))
  vi.mocked(getRun).mockResolvedValue(succeededStatus)
  vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
  function Harness() {
    const [id, setId] = useState<string | null>(null)
    return <ScanPanel runId={id} onRunIdChange={setId} onSelectInstrument={noop} />
  }
  render(<Harness />)
  await screen.findByLabelText('参数 lookback_days')
  fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
  fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
  fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
  fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-02-01' } })
  await act(async () => finish({ run_id: 'r1', status: 'PENDING' }))
  await screen.findByRole('button', { name: 'SSE:600000' })
  expect(screen.getByText(/2026-01-01 → 2026-06-01/)).toBeVisible()
  expect(screen.getByText(/回看天数 20/)).toBeVisible()
  expect(screen.getByLabelText('开始日期')).toHaveValue('2026-02-01')
  expect(screen.getByText('条件已修改，以下仍为上次扫描结果')).toBeVisible()
  expect(JSON.parse(localStorage.getItem('wb.scan_preferences')!).draft.from).toBe('2026-02-01')
  expect(createScanRun).toHaveBeenCalledTimes(1)
})

it('stale pagination failure cannot replace the loading state of a new snapshot', async () => {
  let rejectPage!: (cause: Error) => void
  let finishSnapshot!: (page: ReturnType<typeof pageOf>) => void
  vi.mocked(getRun).mockResolvedValue(succeededStatus)
  vi.mocked(fetchSnapshotPage).mockResolvedValueOnce(pageOf(['SSE:600000'], 100))
    .mockReturnValueOnce(new Promise((_, reject) => { rejectPage = reject }))
    .mockReturnValueOnce(new Promise((resolve) => { finishSnapshot = resolve }))
  const props = { onRunIdChange: noop, onSelectInstrument: noop }
  const { rerender } = render(<ScanPanel {...props} runId="r1" />)
  await screen.findByRole('button', { name: 'SSE:600000' })
  fireEvent.click(screen.getByRole('button', { name: '加载更多' }))
  vi.mocked(getRun).mockResolvedValue({ ...succeededStatus, run_id: 'r2', snapshot_id: 'snap-2' })
  rerender(<ScanPanel {...props} runId="r2" />)
  await screen.findByText(/等待新扫描结果加载成功/)
  await act(async () => rejectPage(new Error('stale page error')))
  expect(screen.queryByText(/stale page error/)).toBeNull()
  expect(screen.getByRole('button', { name: '加载更多' })).toBeDisabled()
  await act(async () => finishSnapshot(pageOf(['SSE:600001'])))
  expect(screen.queryByRole('button', { name: 'SSE:600000' })).toBeNull()
  expect(screen.getByRole('button', { name: 'SSE:600001' })).toBeVisible()
})

it('old first-page success cannot overwrite a newer snapshot', async () => {
  let finishOld!: (page: ReturnType<typeof pageOf>) => void
  vi.mocked(getRun).mockResolvedValue(succeededStatus)
  vi.mocked(fetchSnapshotPage).mockReturnValueOnce(new Promise((resolve) => { finishOld = resolve })).mockResolvedValueOnce(pageOf(['SSE:600001']))
  const props = { onRunIdChange: noop, onSelectInstrument: noop }
  const { rerender } = render(<ScanPanel {...props} runId="r1" />)
  await screen.findByRole('status', { name: '结果加载中' })
  vi.mocked(getRun).mockResolvedValue({ ...succeededStatus, run_id: 'r2', snapshot_id: 'snap-2' })
  rerender(<ScanPanel {...props} runId="r2" />)
  await screen.findByRole('button', { name: 'SSE:600001' })
  await act(async () => finishOld(pageOf(['SSE:600000'])))
  expect(screen.queryByRole('button', { name: 'SSE:600000' })).toBeNull()
})

it('hiding the scan closes its calendar portal without clearing the draft', async () => {
  const props = { runId: null, onRunIdChange: noop, onSelectInstrument: noop }
  const { rerender } = render(<ScanPanel {...props} />)
  await screen.findByLabelText('参数 lookback_days')
  pickPresetRange('近 1 月')
  fireEvent.click(screen.getByRole('button', { name: '扫描时间范围' }))
  expect(screen.getByRole('dialog')).toBeVisible()
  rerender(<ScanPanel {...props} active={false} />)
  expect(screen.queryByRole('dialog')).toBeNull()
  rerender(<ScanPanel {...props} active />)
  expect(screen.getByLabelText('截止日期')).toHaveValue(fmt(today()))
})

it('can retry a failed strategy catalog without reloading the workbench', async () => {
  vi.mocked(listStrategies).mockRejectedValueOnce(new Error('catalog offline')).mockResolvedValueOnce([definition])
  render(<ScanPanel runId={null} onRunIdChange={noop} onSelectInstrument={noop} />)
  await screen.findByText('catalog offline')
  expect(screen.getByRole('button', { name: '发起扫描' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: '重试加载策略' }))
  await screen.findByLabelText('参数 lookback_days')
  expect(screen.getByRole('button', { name: '发起扫描' })).not.toBeDisabled()
})

it('shows reconnect errors even when preserving a completed result', async () => {
  vi.useFakeTimers()
  vi.mocked(getRun).mockResolvedValue(succeededStatus)
  vi.mocked(fetchSnapshotPage).mockResolvedValue(pageOf(['SSE:600000']))
  const props = { runId: 'r1', onRunIdChange: noop, onSelectInstrument: noop }
  const { rerender } = render(<ScanPanel {...props} />)
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  rerender(<ScanPanel {...props} active={false} />)
  vi.mocked(getRun).mockRejectedValue(new Error('connection lost'))
  rerender(<ScanPanel {...props} active />)
  await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
  expect(screen.getByText(/connection lost/)).toBeVisible()
  expect(screen.getByRole('button', { name: 'SSE:600000' })).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: '重新连接' }))
  vi.mocked(getRun).mockResolvedValue(succeededStatus)
  await act(async () => { await vi.advanceTimersByTimeAsync(2000) })
  expect(screen.queryByText(/connection lost/)).toBeNull()
})
