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

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init)
  let envelope: Envelope<T> | null = null
  try {
    envelope = (await response.json()) as Envelope<T>
  } catch {
    throw new Error(`服务返回了无法解析的响应（HTTP ${response.status}）`)
  }
  if (!response.ok || envelope.code !== 0) {
    throw new Error(envelope.message || `请求失败（HTTP ${response.status}）`)
  }
  return envelope.data
}

export function searchInstruments(query: string, signal?: AbortSignal): Promise<InstrumentSummary[]> {
  const params = new URLSearchParams({ q: query.trim(), limit: '20' })
  return request<{ items: InstrumentSummary[] }>(`/api/v1/instruments?${params}`, { signal }).then(
    (result) => result.items,
  )
}

export function queryChart(input: ChartQueryInput, signal?: AbortSignal): Promise<ChartResult> {
  return request<ChartResult>('/api/v1/chart-queries', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  })
}
