import type { BoardConfig, ChartBoard, IndicatorRequest } from '../../api/client'
import { defaultIndicators, indicatorIdentity } from './chartData'

export type { BoardConfig } from '../../api/client'
export type Board = ChartBoard

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
  const fields = value as Record<string, unknown>
  if (
    fields.kind !== 'ZSCORE' &&
    ((fields.smooth !== undefined && fields.smooth !== 0) ||
      (fields.regime !== undefined && fields.regime !== 0) || (fields.lag !== undefined && fields.lag !== 0))
  )
    return false
  const i = value as IndicatorRequest
  if (i.kind === 'MACD')
    return integer(i.fast, 1, 499) && integer(i.slow, i.fast + 1, 500) && integer(i.signal, 1, 500)
  if (i.kind === 'ZSCORE')
    return (
      ['fast', 'slow', 'signal'].every((field) => fields[field] === undefined || fields[field] === 0) &&
      integer(i.period, 2, 500) &&
      integer(i.smooth, 1, 500) &&
      integer(i.regime, Number(i.period) + 1, 500) && integer(i.lag ?? 0, 0, 5)
    )
  return ['SMA', 'EMA', 'KDJ'].includes(i.kind) && 'period' in i && integer(i.period, 1, 500)
}

/** 提交前预检（服务端为权威校验，口径与其保持一致）。 */
export function validConfig(c: BoardConfig): boolean {
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

export function hasLegacyIndicators(config: BoardConfig): boolean {
  return config.indicators.some((item) => ['STD', 'RETZ'].includes(item.kind))
}

/** Only migrate the draft: persisted boards remain untouched until the user saves. */
export function migrateBoardConfig(saved: BoardConfig): BoardConfig {
  const config = structuredClone(saved)
  if (!hasLegacyIndicators(config)) return config
  let replaced = config.indicators.some((item) => item.kind === 'ZSCORE')
  config.indicators = config.indicators.flatMap((item): IndicatorRequest[] => {
    if (!['STD', 'RETZ'].includes(item.kind)) return [item]
    if (replaced) return []
    replaced = true
    return [{ kind: 'ZSCORE', period: 126, smooth: 5, regime: 252, lag: 0 }]
  })
  config.paneWeights = Object.fromEntries(Object.entries(config.paneWeights).filter(([key]) => !/^(STD|RETZ)/i.test(key)))
  return config
}
