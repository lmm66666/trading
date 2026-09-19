import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
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

const pageOf = (instruments: string[], nextSequence?: number) => ({
  snapshot_id: 'snap-1',
  run_id: 'r1',
  key: snapshotKey,
  data_version: 7,
  rows: instruments.map((instrument) => ({
    instrument,
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

async function renderWithCatalog(props?: Partial<Parameters<typeof ScanPanel>[0]>) {
  render(<ScanPanel runId={null} onRunIdChange={noop} onSelectInstrument={noop} {...props} />)
  // 目录加载后表单默认未选策略，先选中再等待参数输入出现
  fireEvent.change(await screen.findByRole('combobox'), { target: { value: 'daily_b1_buy' } })
  await screen.findByLabelText('参数 lookback_days')
}

beforeEach(() => {
  vi.clearAllMocks()
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

    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByLabelText('上交所 SSE'))
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))

    await screen.findByText('提交中…')
    await vi.waitFor(() => expect(onRunIdChange).toHaveBeenCalledWith('r9'))
    expect(createScanRun).toHaveBeenCalledWith({
      strategy: 'daily_b1_buy',
      strategy_version: '1',
      idempotency_key: 'uuid-123',
      from: '2026-01-01T00:00:00Z',
      as_of: '2026-06-01T00:00:00Z',
      parameters: undefined,
      scope: { exchanges: ['SSE'], active_only: true, limit: 5000 },
    })
  })

  it('已填参数随提交体上送', async () => {
    vi.mocked(createScanRun).mockResolvedValue({ run_id: 'r9', status: 'PENDING' })
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '30' } })
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))

    await vi.waitFor(() => expect(createScanRun).toHaveBeenCalled())
    expect(createScanRun).toHaveBeenCalledWith(expect.objectContaining({ parameters: { lookback_days: 30 } }))
  })

  it('日期非法时不发出请求', async () => {
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-06-01' } })
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-01-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    expect(screen.getByText('开始日期不能晚于截止日期')).toBeVisible()
    expect(createScanRun).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2000-01-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    expect(screen.getByText('时间跨度不能超过 20 年')).toBeVisible()
    expect(createScanRun).not.toHaveBeenCalled()
  })

  it('参数越界时拦截提交', async () => {
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('参数 lookback_days'), { target: { value: '999' } })
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    expect(screen.getByText('请选择策略并检查参数')).toBeVisible()
    expect(createScanRun).not.toHaveBeenCalled()
  })

  it('幂等冲突时显示可读提示', async () => {
    vi.mocked(createScanRun).mockRejectedValue(new Error('IDEMPOTENCY_CONFLICT'))
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    await screen.findByText('任务输入与已有任务冲突，请重新提交')
  })

  it('其他错误原样展示且保留表单', async () => {
    vi.mocked(createScanRun).mockRejectedValue(new Error('NETWORK_DOWN'))
    await renderWithCatalog()
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    fireEvent.change(screen.getByLabelText('截止日期'), { target: { value: '2026-06-01' } })
    fireEvent.click(screen.getByRole('button', { name: '发起扫描' }))
    await screen.findByText('NETWORK_DOWN')
    expect(screen.getByLabelText('开始日期')).toHaveValue('2026-01-01')
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

  it('部分成功时展示失败计数，failures 默认折叠', async () => {
    vi.mocked(fetchSnapshotPage).mockResolvedValue({
      ...pageOf(['SSE:600000']),
      failures: [
        { instrument: 'SZSE:002399', code: 'DATA_MISSING', message: '缺少行情', retryable: true },
      ],
    })
    await renderSucceeded(partialStatus)

    expect(screen.getByText('1 只证券处理失败')).toBeVisible()
    const detail = screen.getByText('缺少行情')
    expect(detail).not.toBeVisible()
    fireEvent.click(screen.getByText('处理失败 1 只'))
    expect(detail).toBeVisible()
    expect(screen.getByText('是')).toBeVisible()
  })
})
