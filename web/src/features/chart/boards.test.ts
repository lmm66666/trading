import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defaultBoardConfig, readBoards, saveBoards, BOARDS_KEY } from './boards'

describe('manual board storage', () => {
  beforeEach(() => localStorage.clear())
  it('does not write on read; restores only explicitly saved configuration', () => {
    const loaded = readBoards()
    expect(localStorage.getItem(BOARDS_KEY)).toBeNull()
    const config = defaultBoardConfig('SSE:600938')
    config.indicators.push({ kind: 'MACD', fast: 12, slow: 26, signal: 9 })
    config.comparison = 'INE:SC.MAIN'
    const next = { ...loaded.data, boards: [{ id: 'oil', name: '油价看板', config }], activeId: 'oil' }
    saveBoards(next, loaded.raw)
    expect(readBoards().data).toEqual(next)
    config.indicators = []
    expect(readBoards().data.boards[0].config.indicators).toHaveLength(4)
  })
  it('rejects corruption, unsupported version and another tab overwrite', () => {
    localStorage.setItem(BOARDS_KEY, 'bad json')
    expect(readBoards().error).toBeTruthy()
    localStorage.setItem(BOARDS_KEY, JSON.stringify({ schemaVersion: 99 }))
    expect(readBoards().error).toBeTruthy()
    localStorage.clear()
    const loaded = readBoards()
    localStorage.setItem(BOARDS_KEY, 'changed')
    expect(() => saveBoards(loaded.data, loaded.raw)).toThrow(/其他页面/)
  })
  it('reports storage failures instead of success', () => {
    const data = readBoards().data
    vi.spyOn(Storage.prototype, 'setItem').mockImplementationOnce(() => {
      throw new Error('quota')
    })
    expect(() => saveBoards(data, null)).toThrow()
  })
})

it('rejects malformed config, duplicated indicators, invalid names and unsupported instruments', () => {
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
    localStorage.clear()
    const data = readBoards().data
    mutate(data.boards[0].config)
    expect(() => saveBoards(data, null)).toThrow()
  }
  localStorage.clear()
  const data = readBoards().data
  data.boards[0].name = ' '
  expect(() => saveBoards(data, null)).toThrow()
})
