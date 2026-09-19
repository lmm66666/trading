import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cancelRun,
  createBacktestRun,
  createScanRun,
  fetchRunPage,
  fetchSnapshotPage,
  fromScaled,
  getRun,
  listStrategies,
  queryChart,
  searchInstruments,
  strategyParamList,
  toScaled,
  type BacktestRunInput,
  type ScanRunInput,
  type StrategyDefinition,
} from './client'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

function okResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ code: 0, message: 'success', data }), { status })
}

describe('API client', () => {
  it('搜索时编码关键词并返回列表', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 0, message: 'ok', data: { items: [{ instrument: 'SZSE:002415' }] },
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()

    await expect(searchInstruments(' 海康 ', controller.signal)).resolves.toEqual([{ instrument: 'SZSE:002415' }])
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/instruments?q=%E6%B5%B7%E5%BA%B7&limit=20', { signal: controller.signal })
  })

  it('发送图表查询 JSON', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 0, message: 'ok', data: { bars: [] },
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const input = { instrument: 'SZSE:002415', timeframe: 'DAY' as const, price_view: 'QFQ' as const, indicators: [] }

    await queryChart(input)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/chart-queries', expect.objectContaining({
      method: 'POST', body: JSON.stringify(input),
    }))
  })

  it('将业务错误与非 JSON 响应转换为可读错误', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      code: 40001, message: '参数无效', data: null,
    }), { status: 400 })).mockResolvedValueOnce(new Response('gateway down', { status: 502 })))

    await expect(searchInstruments('x')).rejects.toThrow('参数无效')
    await expect(searchInstruments('x')).rejects.toThrow('无法解析')
  })
})

describe('开发模式连接失败回退', () => {
  it('后端不可达时返回演示数据', async () => {
    vi.stubEnv('MODE', 'development')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    vi.resetModules()
    const { queryChart, searchInstruments } = await import('./client')

    await expect(searchInstruments('600519')).resolves.toEqual([
      expect.objectContaining({ code: '600519', name: '贵州茅台' }),
    ])
    const chart = await queryChart({
      instrument: 'SSE:600519', timeframe: 'DAY', price_view: 'QFQ', indicators: [],
    })
    expect(chart.bars.length).toBeGreaterThan(0)
    expect(chart.instrument.name).toBe('贵州茅台')
  })

  it('业务错误不走演示数据回退', async () => {
    vi.stubEnv('MODE', 'development')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 40001, message: '参数无效', data: null,
    }), { status: 400 })))
    vi.resetModules()
    const { searchInstruments } = await import('./client')

    await expect(searchInstruments('x')).rejects.toThrow('参数无效')
  })

  it('非开发模式不启用回退', async () => {
    vi.stubEnv('MODE', 'test')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    vi.resetModules()
    const { searchInstruments } = await import('./client')

    await expect(searchInstruments('x')).rejects.toThrow('无法连接服务')
  })
})

describe('策略任务客户端', () => {
  it('拉取策略目录并返回裸数组', async () => {
    const definition: StrategyDefinition = {
      strategy: 'daily_b1_buy', version: '1', primary_timeframe: 'daily',
      warmup_bars: 60, default_hold_bars: 10,
      parameters: { lookback: { default: 20, min: 5, max: 120, integer: true } },
      features: ['close'], auxiliary: [],
    }
    const fetchMock = vi.fn().mockResolvedValue(okResponse([definition]))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listStrategies()).resolves.toEqual([definition])
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/strategies', undefined)
  })

  it('把参数对象映射转换为按名称排序的数组', () => {
    const definition: StrategyDefinition = {
      strategy: 's', version: '1', primary_timeframe: 'daily', warmup_bars: 1,
      default_hold_bars: 1,
      parameters: {
        zeta: { default: 2, min: 1, max: 3, integer: false },
        alpha: { default: 10, min: 5, max: 20, integer: true },
      },
      features: [], auxiliary: [],
    }
    expect(strategyParamList(definition)).toEqual([
      { name: 'alpha', default: 10, min: 5, max: 20, integer: true },
      { name: 'zeta', default: 2, min: 1, max: 3, integer: false },
    ])
  })

  it('创建扫描任务时发送 JSON 体', async () => {
    const fetchMock = vi.fn().mockResolvedValue(okResponse({ run_id: 'r1', status: 'PENDING' }, 202))
    vi.stubGlobal('fetch', fetchMock)
    const input: ScanRunInput = {
      strategy: 'daily_b1_buy', strategy_version: '1', idempotency_key: 'uuid-1',
      from: '2026-01-01T00:00:00Z', as_of: '2026-06-01T00:00:00Z',
      scope: { exchanges: ['SSE'], active_only: true, limit: 5000 },
    }

    await expect(createScanRun(input)).resolves.toEqual({ run_id: 'r1', status: 'PENDING' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/scan-runs', expect.objectContaining({
      method: 'POST', body: JSON.stringify(input),
    }))
  })

  it('创建回测任务时发送 JSON 体', async () => {
    const fetchMock = vi.fn().mockResolvedValue(okResponse({ run_id: 'r2', status: 'PENDING' }, 202))
    vi.stubGlobal('fetch', fetchMock)
    const input: BacktestRunInput = {
      instrument: 'SSE:600000', strategy: 'daily_b1_buy', strategy_version: '1',
      idempotency_key: 'uuid-2', start: '2026-01-01T00:00:00Z', end: '2026-06-01T00:00:00Z',
      config: {
        initial_cash: 10000000000, cash_fraction_bps: 10000, commission_bps: 3,
        minimum_commission: 50000, stamp_duty_bps: 5, transfer_fee_bps: 0,
        slippage_bps: 5, lot_size: 100, hold_bars: 10,
      },
    }

    await expect(createBacktestRun(input)).resolves.toEqual({ run_id: 'r2', status: 'PENDING' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/backtest-runs', expect.objectContaining({
      method: 'POST', body: JSON.stringify(input),
    }))
  })

  it('按任务类型构造状态与取消 URL，取消发送空对象体', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(okResponse({ run_id: 'r1', status: 'RUNNING' }))
      .mockResolvedValueOnce(okResponse({ run_id: 'r1', cancel_requested: true }, 202))
      .mockResolvedValueOnce(okResponse({ run_id: 'r2', status: 'RUNNING' }))
      .mockResolvedValueOnce(okResponse({ run_id: 'r2', cancel_requested: true }, 202))
    vi.stubGlobal('fetch', fetchMock)

    await getRun('scan', 'r1')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/scan-runs/r1', undefined)
    await cancelRun('scan', 'r1')
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/scan-runs/r1/cancel', expect.objectContaining({
      method: 'POST', body: '{}',
    }))
    await getRun('backtest', 'r2')
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/backtest-runs/r2', undefined)
    await cancelRun('backtest', 'r2')
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/v1/backtest-runs/r2/cancel', expect.objectContaining({
      method: 'POST', body: '{}',
    }))
  })

  it('快照首页只带策略参数，续页固定 key 并追加游标', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(okResponse({ snapshot_id: 'snap', rows: [], failures: [] }))
      .mockResolvedValueOnce(okResponse({ snapshot_id: 'snap', rows: [], failures: [], next_sequence: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await fetchSnapshotPage({ strategy: 'daily_b1_buy', strategy_version: '1', snapshot_id: 'snap', limit: 100 })
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&strategy_version=1&snapshot_id=snap&limit=100',
      undefined,
    )
    await fetchSnapshotPage({
      strategy: 'daily_b1_buy', strategy_version: '1', parameters_hash: 'hash',
      snapshot_id: 'snap', after_sequence: 100, limit: 100,
    })
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/signal-snapshots/latest?strategy=daily_b1_buy&strategy_version=1&parameters_hash=hash&snapshot_id=snap&after_sequence=100&limit=100',
      undefined,
    )
  })

  it('回测结果页按资源与游标构造 URL', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(okResponse({ items: [] }))
      .mockResolvedValueOnce(okResponse({ items: [], next_sequence: 300 }))
    vi.stubGlobal('fetch', fetchMock)

    await fetchRunPage('backtest', 'r1', 'equity')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/backtest-runs/r1/equity?limit=100', undefined)
    await fetchRunPage('backtest', 'r1', 'orders', 200, 500)
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/backtest-runs/r1/orders?limit=500&after_sequence=200', undefined)
  })
})

describe('金额缩放换算', () => {
  it('元与缩放整数双向换算', () => {
    expect(toScaled(1_000_000)).toBe(10_000_000_000)
    expect(toScaled(5)).toBe(50_000)
    expect(fromScaled(10_000_000_000)).toBe(1_000_000)
    expect(fromScaled(50_000)).toBe(5)
  })

  it('处理小数与上边界精度', () => {
    expect(toScaled(1000.5555)).toBe(10_005_555)
    expect(fromScaled(10_005_555)).toBeCloseTo(1000.5555)
    expect(Number.isSafeInteger(toScaled(1e9))).toBe(true)
    expect(toScaled(1e9)).toBe(1e13)
  })
})
