import { expect, it } from 'vitest'
import { comparisonBasis, percent, alignComparison, periodKey } from './comparison'
const bar = (day: string, close: number) => ({
  open_time: day,
  close_time: day,
  open: close,
  high: close,
  low: close,
  close,
  volume: 1,
  amount: 1,
  trading_status: 0,
})
it('normalizes each series against its price on a common date, never fills missing prices', () => {
  const main = [bar('2026-09-01', 10), bar('2026-09-02', 20), bar('2026-09-03', 30)]
  const oil = [bar('2026-09-02', 100), bar('2026-09-03', 110)]
  expect(comparisonBasis(main, oil, 'DAY')).toEqual({ date: '2026-09-02', main: 20, comparison: 100 })
  expect(percent(30, 20)).toBe(50)
  expect(percent(110, 100)).toBeCloseTo(10)
  expect(alignComparison(main, oil, 'DAY')).toEqual([
    { time: '2026-09-01' },
    { time: '2026-09-02', value: 100 },
    { time: '2026-09-03', value: 110 },
  ])
  expect(comparisonBasis(main, [bar('2020-01-01', 1)], 'DAY')).toBeNull()
  expect(comparisonBasis([bar('2026-09-01', 0)], undefined, 'DAY')).toBeNull()
})
it('aligns weekly bars by ISO week even when last sessions differ', () => {
  expect(periodKey('2026-09-10T07:00:00Z', 'WEEK')).toBe(periodKey('2026-09-11T07:00:00Z', 'WEEK'))
  expect(periodKey('2026-09-14', 'WEEK')).not.toBe(periodKey('2026-09-11', 'WEEK'))
})
