import { mockChartQuery, mockSearchInstruments } from './mock'

export type Timeframe = 'DAY' | 'WEEK'
export type PriceView = 'RAW' | 'QFQ'

export interface InstrumentSummary {
  instrument: string
  code: string
  name: string
  exchange: 'SSE' | 'SZSE' | 'BSE'
  board: string
  lot_size: number
}

export interface ChartBar {
  open_time: string
  close_time: string
  open: number
  high: number
  low: number
  close: number
  volume: number
  amount: number
  trading_status: number
}

export type IndicatorRequest =
  | { kind: 'SMA'; period: number }
  | { kind: 'EMA'; period: number }
  | { kind: 'MACD'; fast: number; slow: number; signal: number }
  | { kind: 'KDJ'; period: number }
  | { kind: 'STD'; period: number }

export interface ChartPoint {
  time: string
  value: number
}

export interface ChartSeries {
  key: string
  kind: IndicatorRequest['kind']
  component: string
  points: ChartPoint[]
}

export interface ChartResult {
  instrument: InstrumentSummary
  timeframe: Timeframe
  price_view: PriceView
  data_version: number
  bars: ChartBar[]
  series: ChartSeries[]
  has_more: boolean
  next_before: string | null
}

interface Envelope<T> {
  code: number
  message: string
  data: T
}

export interface ChartQueryInput {
  instrument: string
  timeframe: Timeframe
  price_view: PriceView
  before?: string
  limit?: number
  data_version?: number
  indicators: IndicatorRequest[]
}

/** 连接层失败（无法访问后端或响应不可解析），区别于后端返回的业务错误。 */
export class ConnectivityError extends Error {}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(url, init)
  } catch (error) {
    if (init?.signal?.aborted) throw error
    throw new ConnectivityError('无法连接服务，请确认后端是否运行')
  }
  let envelope: Envelope<T> | null = null
  try {
    envelope = (await response.json()) as Envelope<T>
  } catch {
    if (init?.signal?.aborted) throw new DOMException('请求已中止', 'AbortError')
    throw new ConnectivityError(`服务返回了无法解析的响应（HTTP ${response.status}）`)
  }
  if (!response.ok || envelope.code !== 0) {
    throw new Error(envelope.message || `请求失败（HTTP ${response.status}）`)
  }
  return envelope.data
}

// 仅 vite dev 模式下，后端不可达时回退到演示数据，便于单独预览前端 UI。
const mockFallbackEnabled = import.meta.env.MODE === 'development'

function withMockFallback<T>(promise: Promise<T>, fallback: () => T): Promise<T> {
  if (!mockFallbackEnabled) return promise
  return promise.catch((reason: unknown) => {
    if (reason instanceof ConnectivityError) {
      console.warn('[dev] 后端不可达，使用演示数据代替真实接口')
      return fallback()
    }
    throw reason
  })
}

// ---- 金额缩放换算：后端 Price/Money 均为缩放 10000 的整数 ----

const MONEY_SCALE = 10000

/** 元 → 缩放整数（四舍五入到分位边界，避免浮点误差累积） */
export function toScaled(yuan: number): number {
  return Math.round(yuan * MONEY_SCALE)
}

/** 缩放整数 → 元 */
export function fromScaled(scaled: number): number {
  return scaled / MONEY_SCALE
}

// ---- 策略目录 ----

export interface StrategyParamSpec {
  default: number
  min: number
  max: number
  integer: boolean
}

export interface StrategyDefinition {
  strategy: string
  version: string
  primary_timeframe: string
  warmup_bars: number
  default_hold_bars: number
  /** 服务端返回参数对象映射：{ 参数名: 规格 } */
  parameters: Record<string, StrategyParamSpec>
  features: string[]
  auxiliary: string[]
}

export interface StrategyParam {
  name: string
  default: number
  min: number
  max: number
  integer: boolean
}

/** 把服务端的参数映射转换为按名称排序的参数数组，供表单渲染 */
export function strategyParamList(definition: StrategyDefinition): StrategyParam[] {
  return Object.entries(definition.parameters)
    .map(([name, spec]) => ({ name, ...spec }))
    .sort((left, right) => left.name.localeCompare(right.name))
}

// ---- 持久化策略任务（扫描与回测）----

export type RunKind = 'scan' | 'backtest'
export type RunResource = 'orders' | 'trades' | 'equity'

export type RunStatusValue =
  'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'PARTIAL_SUCCEEDED' | 'FAILED' | 'CANCELLED'

export interface RunReference {
  run_id: string
  status: string
}

export interface BacktestSummary {
  total_return: number | null
  annualized_return: number | null
  maximum_drawdown: number | null
  closed_trades: number
  win_rate: number | null
  profit_factor: number | null
  average_holding_bars: number
  has_open_position: boolean
}

export interface RunStatus {
  run_id: string
  status: RunStatusValue
  kind: RunKind
  strategy: string
  strategy_version: string
  data_version: number
  engine_version: string
  attempts: number
  cancel_requested_at: string | null
  /** 扫描终态 SUCCEEDED/PARTIAL_SUCCEEDED 时返回，用于精确定位快照 */
  snapshot_id?: string
  /** 回测 SUCCEEDED 时返回 */
  summary?: BacktestSummary
}

export interface ScanRunInput {
  strategy: string
  strategy_version: string
  idempotency_key: string
  from: string
  as_of: string
  parameters?: Record<string, number>
  scope: { exchanges: string[]; active_only: boolean; limit: number }
}

export interface BacktestConfigInput {
  initial_cash: number
  cash_fraction_bps: number
  commission_bps: number
  minimum_commission: number
  stamp_duty_bps: number
  transfer_fee_bps: number
  slippage_bps: number
  lot_size: number
  hold_bars: number
}

export interface BacktestRunInput {
  instrument: string
  strategy: string
  strategy_version: string
  idempotency_key: string
  start: string
  end: string
  parameters?: Record<string, number>
  config: BacktestConfigInput
}

export function listStrategies(): Promise<StrategyDefinition[]> {
  return request<StrategyDefinition[]>('/api/v1/strategies')
}

export function getRun(kind: RunKind, runId: string): Promise<RunStatus> {
  return request<RunStatus>(`/api/v1/${kind}-runs/${encodeURIComponent(runId)}`)
}

export function cancelRun(kind: RunKind, runId: string): Promise<void> {
  return request<void>(`/api/v1/${kind}-runs/${encodeURIComponent(runId)}/cancel`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  })
}

function postTask<T>(url: string, input: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function createScanRun(input: ScanRunInput): Promise<RunReference> {
  return postTask<RunReference>('/api/v1/scan-runs', input)
}

export function createBacktestRun(input: BacktestRunInput): Promise<RunReference> {
  return postTask<RunReference>('/api/v1/backtest-runs', input)
}

// ---- 快照与结果分页 ----

export interface Page<T> {
  items: T[]
  next_sequence?: number
}

export interface SnapshotPageQuery {
  strategy: string
  strategy_version?: string
  parameters_hash?: string
  snapshot_id?: string
  after_sequence?: number
  limit?: number
}

export interface SnapshotKey {
  snapshot_id: string
  strategy_id: string
  strategy_version: string
  parameters_hash: string
  as_of: string
}

export interface SnapshotRow {
  instrument: string
  signal_time: string
  reason: string
  values?: Record<string, number>
}

export interface SnapshotFailure {
  instrument: string
  code: string
  message: string
  retryable: boolean
}

export interface SnapshotPage {
  snapshot_id: string
  run_id: string
  key: SnapshotKey
  data_version: number
  rows: SnapshotRow[]
  failures: SnapshotFailure[]
  next_sequence?: number
}

export function fetchSnapshotPage(query: SnapshotPageQuery): Promise<SnapshotPage> {
  const params = new URLSearchParams({ strategy: query.strategy })
  if (query.strategy_version) params.set('strategy_version', query.strategy_version)
  if (query.parameters_hash) params.set('parameters_hash', query.parameters_hash)
  if (query.snapshot_id) params.set('snapshot_id', query.snapshot_id)
  if (query.after_sequence !== undefined && query.after_sequence > 0) {
    params.set('after_sequence', String(query.after_sequence))
  }
  params.set('limit', String(query.limit ?? 100))
  return request<SnapshotPage>(`/api/v1/signal-snapshots/latest?${params}`)
}

export function fetchRunPage<T>(
  kind: RunKind,
  runId: string,
  resource: RunResource,
  after?: number,
  limit?: number,
): Promise<Page<T>> {
  const params = new URLSearchParams({ limit: String(limit ?? 100) })
  if (after !== undefined && after > 0) params.set('after_sequence', String(after))
  return request<Page<T>>(`/api/v1/${kind}-runs/${encodeURIComponent(runId)}/${resource}?${params}`)
}

// ---- 回测结果行 ----

export interface BacktestOrder {
  id: string
  instrument: string
  side: number
  quantity: number
  created_at: string
  reason: string
  attempted_at: string
  final_reason: number
}

export interface BacktestFill {
  id: string
  order_id: string
  instrument: string
  side: number
  time: string
  price: number
  quantity: number
  gross: number
  commission: number
  stamp_duty: number
  transfer_fee: number
}

export interface EquityPoint {
  time: string
  equity: number
  cash: number
  position_value: number
}

export function searchInstruments(query: string, signal?: AbortSignal): Promise<InstrumentSummary[]> {
  const params = new URLSearchParams({ q: query.trim(), limit: '20' })
  return withMockFallback(
    request<{ items: InstrumentSummary[] }>(`/api/v1/instruments?${params}`, { signal }).then(
      (result) => result.items,
    ),
    () => mockSearchInstruments(query),
  )
}

export function queryChart(input: ChartQueryInput, signal?: AbortSignal): Promise<ChartResult> {
  return withMockFallback(
    request<ChartResult>('/api/v1/chart-queries', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
      signal,
    }),
    () => mockChartQuery(input),
  )
}

// ---- 自选清单（无 mock 回退：后端不可用时呈现错误态）----

export interface WatchlistItem extends InstrumentSummary {
  close: number | null
  change: number | null
  change_pct: number | null
}

function unwrapItems(result: { items: WatchlistItem[] }): WatchlistItem[] {
  return result.items
}

export function listWatchlist(): Promise<WatchlistItem[]> {
  return request<{ items: WatchlistItem[] }>('/api/v1/watchlist').then(unwrapItems)
}

export function addWatchlistItem(instrument: string): Promise<WatchlistItem[]> {
  return request<{ items: WatchlistItem[] }>('/api/v1/watchlist', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instrument }),
  }).then(unwrapItems)
}

export function removeWatchlistItem(instrument: string): Promise<WatchlistItem[]> {
  return request<{ items: WatchlistItem[] }>(`/api/v1/watchlist/${encodeURIComponent(instrument)}`, {
    method: 'DELETE',
  }).then(unwrapItems)
}

// ---- 行情看板（服务端持久化，无 mock 回退：后端不可用时呈现错误态）----

export interface BoardConfig {
  defaultSymbol: string | null
  timeframe: Timeframe
  priceView: PriceView
  indicators: IndicatorRequest[]
  comparison: string | null
  paneWeights: Record<string, number>
  visibleBars: number
}

export interface ChartBoard {
  id: number
  name: string
  config: BoardConfig
}

export interface ChartBoardState {
  boards: ChartBoard[]
  active_id: number
}

export function listChartBoards(): Promise<ChartBoardState> {
  return request<ChartBoardState>('/api/v1/chart-boards')
}

export function createChartBoard(name: string, config: BoardConfig): Promise<ChartBoardState> {
  return request<ChartBoardState>('/api/v1/chart-boards', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, config }),
  })
}

export function updateChartBoard(
  id: number,
  changes: { name?: string; config?: BoardConfig },
): Promise<ChartBoardState> {
  return request<ChartBoardState>(`/api/v1/chart-boards/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(changes),
  })
}

export function activateChartBoard(id: number): Promise<ChartBoardState> {
  return request<ChartBoardState>(`/api/v1/chart-boards/${id}/activate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  })
}

export function deleteChartBoard(id: number): Promise<ChartBoardState> {
  return request<ChartBoardState>(`/api/v1/chart-boards/${id}`, { method: 'DELETE' })
}

export interface ComparisonQuery {
  instrument: string
  timeframe: Timeframe
  version: number
  from: string
  to: string
}
export interface ComparisonResult {
  instrument: string
  data_version: number
  bars: ChartBar[]
}
/** Fixed-version comparison data uses the existing futures-capable market endpoint. */
export function queryComparison(input: ComparisonQuery, signal?: AbortSignal): Promise<ComparisonResult> {
  const params = new URLSearchParams({
    instrument: input.instrument,
    timeframe: input.timeframe === 'DAY' ? 'daily' : 'weekly',
    view: 'raw',
    version: String(input.version),
    from: input.from,
    to: input.to,
    limit: '5000',
  })
  return request<ComparisonResult>(`/api/v1/market/bars?${params}`, { signal })
}
