import { describe, expect, it } from 'vitest'
import { defaultBoardConfig, validConfig, validIndicator } from './boards'

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
