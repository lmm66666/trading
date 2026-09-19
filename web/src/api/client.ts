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
