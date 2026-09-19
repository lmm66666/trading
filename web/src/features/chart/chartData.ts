import type {
  ChartBar,
  ChartPoint,
  ChartSeries,
  IndicatorRequest,
  PriceView,
  Timeframe,
} from '../../api/client'

export const defaultIndicators: IndicatorRequest[] = [
  { kind: 'SMA', period: 5 },
  { kind: 'SMA', period: 20 },
  { kind: 'SMA', period: 60 },
]

export interface WorkbenchState {
  symbol: string | null
  timeframe: Timeframe
  priceView: PriceView
  view: WorkbenchView
}

export type WorkbenchView = 'chart' | 'scan' | 'backtest'

const WORKBENCH_VIEWS: readonly WorkbenchView[] = ['chart', 'scan', 'backtest']

export function readWorkbenchState(search: string): WorkbenchState {
  const params = new URLSearchParams(search)
  const candidate = params.get('symbol')?.toUpperCase() ?? ''
  const symbol = /^(SSE|SZSE|BSE):[A-Z0-9]{1,32}$/.test(candidate) ? candidate : null
  const timeframe = params.get('timeframe') === 'WEEK' ? 'WEEK' : 'DAY'
  const priceView = params.get('view') === 'RAW' ? 'RAW' : 'QFQ'
  const tab = params.get('tab')
  const view = WORKBENCH_VIEWS.includes(tab as WorkbenchView) ? (tab as WorkbenchView) : 'chart'
  return { symbol, timeframe, priceView, view }
}

export function writeWorkbenchState(state: WorkbenchState): void {
  const params = new URLSearchParams()
  if (state.symbol) params.set('symbol', state.symbol)
  params.set('timeframe', state.timeframe)
  params.set('view', state.priceView)
  params.set('tab', state.view)
  window.history.replaceState(null, '', `${window.location.pathname}?${params}`)
}

export function mergeBars(existing: ChartBar[], older: ChartBar[]): ChartBar[] {
  const merged = new Map(older.map((item) => [item.close_time, item]))
  for (const item of existing) merged.set(item.close_time, item)
  return [...merged.values()].sort((left, right) => left.close_time.localeCompare(right.close_time))
}

function mergePoints(existing: ChartPoint[], older: ChartPoint[]): ChartPoint[] {
  const merged = new Map(older.map((item) => [item.time, item]))
  for (const item of existing) merged.set(item.time, item)
  return [...merged.values()].sort((left, right) => left.time.localeCompare(right.time))
}

export function mergeSeries(existing: ChartSeries[], older: ChartSeries[]): ChartSeries[] {
  const merged = new Map(existing.map((item) => [item.key, item]))
  for (const item of older) {
    const current = merged.get(item.key)
    merged.set(item.key, current ? { ...current, points: mergePoints(current.points, item.points) } : item)
  }
  return [...merged.values()]
}

export function indicatorLabel(indicator: IndicatorRequest): string {
  if (indicator.kind === 'SMA' || indicator.kind === 'EMA') return `${indicator.kind} ${indicator.period}`
  if (indicator.kind === 'KDJ') return `KDJ ${indicator.period}`
  return `MACD ${indicator.fast}, ${indicator.slow}, ${indicator.signal}`
}

export function indicatorIdentity(indicator: IndicatorRequest): string {
  if (indicator.kind === 'SMA' || indicator.kind === 'EMA' || indicator.kind === 'KDJ') {
    return `${indicator.kind}:${indicator.period}`
  }
  return `${indicator.kind}:${indicator.fast}:${indicator.slow}:${indicator.signal}`
}
