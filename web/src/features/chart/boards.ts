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
  const i = value as IndicatorRequest
  if (i.kind === 'MACD')
    return integer(i.fast, 1, 499) && integer(i.slow, i.fast + 1, 500) && integer(i.signal, 1, 500)
  return ['SMA', 'EMA', 'KDJ', 'STD'].includes(i.kind) && 'period' in i && integer(i.period, 1, 500)
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
