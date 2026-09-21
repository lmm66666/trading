import { describe, expect, it } from 'vitest'
import { migrateBoardConfig, defaultBoardConfig, validConfig, validIndicator } from './boards'

describe('board config pre-check', () => {
  it('accepts default and explicit valid configurations', () => {
    expect(validConfig(defaultBoardConfig())).toBe(true)
    expect(validConfig(defaultBoardConfig('SSE:600938'))).toBe(true)
    const config = defaultBoardConfig('SSE:600938')
    config.indicators.push({ kind: 'MACD', fast: 12, slow: 26, signal: 9 })
    config.comparison = 'INE:SC.MAIN'
    config.paneWeights = { price: 3, volume: 1 }
    expect(validConfig(config)).toBe(true)
  })
  it('rejects malformed config', () => {
    for (const mutate of [
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.defaultSymbol = 'evil'
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.visibleBars = 0
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.comparison = 'WTI'
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.timeframe = 'MONTH' as never
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.indicators = [{ kind: 'SMA', period: 0 }]
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.indicators = [{ kind: 'MACD', fast: 26, slow: 12, signal: 9 }]
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.indicators.push(c.indicators[0])
      },
      (c: ReturnType<typeof defaultBoardConfig>) => {
        c.paneWeights = { price: NaN }
      },
    ]) {
      const config = defaultBoardConfig('SSE:600938')
      mutate(config)
      expect(validConfig(config)).toBe(false)
    }
  })
  it('validates individual indicators', () => {
    expect(validIndicator({ kind: 'SMA', period: 20 })).toBe(true)
    expect(validIndicator({ kind: 'EMA', period: 0 })).toBe(false)
    expect(validIndicator({ kind: 'MACD', fast: 12, slow: 26, signal: 9 })).toBe(true)
    expect(validIndicator({ kind: 'MACD', fast: 500, slow: 501, signal: 9 })).toBe(false)
    expect(validIndicator(null)).toBe(false)
  })
})

it('validates ZSCORE bounds, unrelated fields and distinct saved configurations', () => {
  const zscore = { kind: 'ZSCORE' as const, period: 126, smooth: 5, regime: 252 }
  expect(validIndicator(zscore)).toBe(true)
  for (const patch of [
    { period: 1 },
    { period: 501 },
    { smooth: 0 },
    { smooth: 501 },
    { regime: 126 },
    { regime: 501 },
    { smooth: 1.5 },
    { fast: 12 },
    { slow: 26 },
    { signal: 9 },
  ])
    expect(validIndicator({ ...zscore, ...patch })).toBe(false)
  for (const kind of ['SMA', 'EMA', 'STD', 'KDJ'])
    expect(validIndicator({ kind, period: 20, smooth: 5 })).toBe(false)
  expect(validIndicator({ kind: 'MACD', fast: 12, slow: 26, signal: 9, regime: 252 })).toBe(false)
  const config = defaultBoardConfig()
  config.indicators = [zscore, { ...zscore, smooth: 10 }, { ...zscore, regime: 300 }]
  expect(validConfig(JSON.parse(JSON.stringify(config)))).toBe(true)
  config.indicators.push(zscore)
  expect(validConfig(config)).toBe(false)
})

it('replaces legacy indicators once without mutating the saved board or guessing a commodity', () => {
  const saved = defaultBoardConfig()
  saved.indicators = [{ kind: 'STD', period: 20 }, { kind: 'RETZ', period: 126, smooth: 5, regime: 252 }, { kind: 'SMA', period: 5 }] as never
  const draft = migrateBoardConfig(saved)
  expect(draft.indicators).toEqual([{ kind: 'ZSCORE', period: 126, smooth: 5, regime: 252, lag: 0 }, { kind: 'SMA', period: 5 }])
  expect(saved.indicators[0].kind).toBe('STD')
  expect(draft.comparison).toBeNull()
  expect(migrateBoardConfig(draft)).toEqual(draft)
})
