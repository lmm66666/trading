import type { IndicatorRequest, PriceView, Timeframe } from '../../api/client'
import { defaultIndicators, indicatorIdentity } from './chartData'

export const BOARDS_KEY = 'wb.boards.v1'
export const comparisonOptions = [
  ['INE:SC.MAIN', '国内原油主力连续'],
  ['SHFE:FU.MAIN', '燃料油主力连续'],
  ['INE:LU.MAIN', '低硫燃料油主力连续'],
  ['SHFE:AU.MAIN', '黄金主力连续'],
  ['SHFE:AG.MAIN', '白银主力连续'],
  ['DCE:J.MAIN', '焦炭主力连续'],
  ['DCE:JM.MAIN', '焦煤主力连续'],
  ['CZCE:ZC.MAIN', '动力煤（历史数据）'],
] as const
export interface BoardConfig {
  defaultSymbol: string | null
  timeframe: Timeframe
  priceView: PriceView
  indicators: IndicatorRequest[]
  comparison: string | null
  paneWeights: Record<string, number>
  visibleBars: number
}
export interface Board {
  id: string
  name: string
  config: BoardConfig
}
export interface BoardStore {
  schemaVersion: 1
  activeId: string
  boards: Board[]
}
export function defaultBoardConfig(symbol: string | null = null): BoardConfig {
  return {
    defaultSymbol: symbol,
    timeframe: 'DAY',
    priceView: 'QFQ',
    indicators: structuredClone(defaultIndicators),
    comparison: null,
    paneWeights: {},
    visibleBars: 120,
  }
}
const integer = (n: unknown, min: number, max: number): n is number =>
  Number.isInteger(n) && Number(n) >= min && Number(n) <= max
export function validIndicator(value: unknown): value is IndicatorRequest {
  if (!value || typeof value !== 'object') return false
  const i = value as IndicatorRequest
  if (i.kind === 'MACD')
    return integer(i.fast, 1, 499) && integer(i.slow, i.fast + 1, 500) && integer(i.signal, 1, 500)
  return ['SMA', 'EMA', 'KDJ', 'STD'].includes(i.kind) && 'period' in i && integer(i.period, 1, 500)
}
function validConfig(c: BoardConfig): boolean {
  return (
    !!c &&
    (c.defaultSymbol === null || /^(SSE|SZSE|BSE):[A-Z0-9]{1,32}$/.test(c.defaultSymbol)) &&
    ['DAY', 'WEEK'].includes(c.timeframe) &&
    ['RAW', 'QFQ'].includes(c.priceView) &&
    Array.isArray(c.indicators) &&
    c.indicators.length <= 16 &&
    c.indicators.every(validIndicator) &&
    new Set(c.indicators.map(indicatorIdentity)).size === c.indicators.length &&
    (c.comparison === null || comparisonOptions.some(([id]) => id === c.comparison)) &&
    integer(c.visibleBars, 10, 400) &&
    !!c.paneWeights &&
    typeof c.paneWeights === 'object' &&
    !Array.isArray(c.paneWeights) &&
    Object.keys(c.paneWeights).length <= 18 &&
    Object.values(c.paneWeights).every(
      (v) => typeof v === 'number' && Number.isFinite(v) && v > 0 && v <= 10000,
    )
  )
}
export function validStore(s: BoardStore): boolean {
  return (
    !!s &&
    s.schemaVersion === 1 &&
    Array.isArray(s.boards) &&
    s.boards.length > 0 &&
    s.boards.length <= 20 &&
    s.boards.every(
      (b) =>
        !!b &&
        typeof b.id === 'string' &&
        b.id.length > 0 &&
        b.id.length <= 64 &&
        typeof b.name === 'string' &&
        b.name.trim().length > 0 &&
        b.name.length <= 40 &&
        validConfig(b.config),
    ) &&
    new Set(s.boards.map((b) => b.id)).size === s.boards.length &&
    s.boards.some((b) => b.id === s.activeId)
  )
}
export function readBoards(): { data: BoardStore; raw: string | null; error: string } {
  const fallback: BoardStore = {
    schemaVersion: 1,
    activeId: 'default',
    boards: [{ id: 'default', name: '默认看板', config: defaultBoardConfig() }],
  }
  let raw: string | null = null
  try {
    raw = window.localStorage.getItem(BOARDS_KEY)
    if (!raw) return { data: fallback, raw, error: '' }
    const data = JSON.parse(raw) as BoardStore
    if (!validStore(data)) throw new Error('invalid')
    return { data, raw, error: '' }
  } catch {
    return { data: fallback, raw, error: '看板存储不可用或格式损坏，请备份站点数据后处理；原记录未覆盖。' }
  }
}
export function saveBoards(data: BoardStore, previousRaw: string | null): string {
  if (!validStore(data)) throw new Error('看板名称、参数或数量不符合要求')
  if (window.localStorage.getItem(BOARDS_KEY) !== previousRaw)
    throw new Error('其他页面修改了看板，请重新载入后再保存')
  const raw = JSON.stringify(data)
  window.localStorage.setItem(BOARDS_KEY, raw)
  return raw
}
