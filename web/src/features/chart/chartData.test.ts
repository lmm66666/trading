import { describe, expect, it } from 'vitest'
import {
  defaultIndicators,
  indicatorIdentity,
  indicatorLabel,
  mergeBars,
  mergeSeries,
  readWorkbenchState,
  writeWorkbenchState,
} from './chartData'
import type { ChartBar } from '../../api/client'

const bar = (closeTime: string, close: number): ChartBar => ({
  open_time: closeTime,
  close_time: closeTime,
  open: close,
  high: close,
  low: close,
  close,
  volume: 1,
  amount: close,
  trading_status: 0,
})

describe('chart data state', () => {
  it('prepends older bars without duplicates and keeps time order', () => {
    const got = mergeBars(
      [bar('2026-01-02T00:00:00Z', 2), bar('2026-01-03T00:00:00Z', 3)],
      [bar('2026-01-01T00:00:00Z', 1), bar('2026-01-02T00:00:00Z', 20)],
    )

    expect(got.map((item) => [item.close_time, item.close])).toEqual([
      ['2026-01-01T00:00:00Z', 1],
      ['2026-01-02T00:00:00Z', 2],
      ['2026-01-03T00:00:00Z', 3],
    ])
  })

  it('defines the three requested moving averages', () => {
    expect(defaultIndicators).toEqual([
      { kind: 'SMA', period: 5 },
      { kind: 'SMA', period: 20 },
      { kind: 'SMA', period: 60 },
    ])
  })

  it('normalizes URL state and rejects incomplete symbols', () => {
    expect(readWorkbenchState('?symbol=szse%3A002415&timeframe=WEEK&view=RAW')).toEqual({
      symbol: 'SZSE:002415',
      timeframe: 'WEEK',
      priceView: 'RAW',
      view: 'chart',
    })
    expect(readWorkbenchState('?symbol=002415&timeframe=MONTH&view=HFQ')).toEqual({
      symbol: null,
      timeframe: 'DAY',
      priceView: 'QFQ',
      view: 'chart',
    })
  })

  it('whitelists the tab parameter and falls back to chart', () => {
    expect(readWorkbenchState('?tab=scan').view).toBe('scan')
    expect(readWorkbenchState('?tab=backtest').view).toBe('backtest')
    expect(readWorkbenchState('?tab=chart').view).toBe('chart')
    expect(readWorkbenchState('?tab=OPTIMIZER').view).toBe('chart')
    expect(readWorkbenchState('?').view).toBe('chart')
  })

  it('writes canonical state to the URL', () => {
    writeWorkbenchState({ symbol: 'SSE:600000', timeframe: 'WEEK', priceView: 'RAW', view: 'chart' })
    expect(window.location.search).toBe('?symbol=SSE%3A600000&timeframe=WEEK&view=RAW&tab=chart')
    writeWorkbenchState({ symbol: null, timeframe: 'DAY', priceView: 'QFQ', view: 'scan' })
    expect(window.location.search).toBe('?timeframe=DAY&view=QFQ&tab=scan')
  })

  it('merges paginated indicator points while preserving current values', () => {
    const current = [
      { key: 'sma', kind: 'SMA' as const, component: 'value', points: [{ time: '2026-01-02', value: 2 }] },
    ]
    const older = [
      {
        key: 'sma',
        kind: 'SMA' as const,
        component: 'value',
        points: [
          { time: '2026-01-01', value: 1 },
          { time: '2026-01-02', value: 20 },
        ],
      },
    ]
    expect(mergeSeries(current, older)[0].points).toEqual([
      { time: '2026-01-01', value: 1 },
      { time: '2026-01-02', value: 2 },
    ])
    expect(mergeSeries([], older)).toEqual(older)
  })

  it('formats every supported indicator identity', () => {
    expect(indicatorLabel({ kind: 'EMA', period: 10 })).toBe('EMA 10')
    expect(indicatorLabel({ kind: 'KDJ', period: 9 })).toBe('KDJ 9')
    expect(indicatorLabel({ kind: 'MACD', fast: 12, slow: 26, signal: 9 })).toBe('MACD 12, 26, 9')
    expect(indicatorIdentity({ kind: 'SMA', period: 20 })).toBe('SMA:20')
    expect(indicatorIdentity({ kind: 'MACD', fast: 12, slow: 26, signal: 9 })).toBe('MACD:12:26:9')
  })
})

it('keeps RETZ identities and all components stable across pagination', () => {
  const indicator = { kind: 'RETZ' as const, period: 126, smooth: 5, regime: 252 }
  expect(indicatorLabel(indicator)).toBe('涨幅Z 126,5,252')
  expect(indicatorIdentity(indicator)).toBe('RETZ:126:5:252')
  expect(indicatorIdentity({ ...indicator, smooth: 10 })).not.toBe(indicatorIdentity(indicator))
  expect(indicatorIdentity({ ...indicator, regime: 300 })).not.toBe(indicatorIdentity(indicator))
  const components = ['histogram', 'smooth', 'regime']
  const page = (time: string, value: number) =>
    components.map((component) => ({
      key: `retz/day/raw/${component}/p=126/sm=5/rg=252`,
      kind: 'RETZ' as const,
      component,
      points: [{ time, value }],
    }))
  const merged = mergeSeries(
    page('2026-09-21', 2),
    mergeSeries(page('2026-09-20', 1), page('2026-09-21', 99)),
  )
  expect(merged.map((item) => item.component)).toEqual(components)
  for (const item of merged)
    expect(item.points).toEqual([
      { time: '2026-09-20', value: 1 },
      { time: '2026-09-21', value: 2 },
    ])
})
