import type { ChartBar, Timeframe } from '../../api/client'
export interface ComparisonBasis {
  date: string
  main: number
  comparison?: number
}
export function periodKey(time: string, timeframe: Timeframe): string {
  const date = time.slice(0, 10)
  if (timeframe === 'DAY') return date
  const d = new Date(date + 'T00:00:00Z')
  d.setUTCDate(d.getUTCDate() - ((d.getUTCDay() + 6) % 7))
  return d.toISOString().slice(0, 10)
}
export const percent = (value: number, base: number) => (value / base - 1) * 100
export function comparisonBasis(
  main: ChartBar[],
  comparison: ChartBar[] | undefined,
  timeframe: Timeframe,
): ComparisonBasis | null {
  const other = new Map(comparison?.map((b) => [periodKey(b.close_time, timeframe), b.close]))
  for (const b of main) {
    if (!Number.isFinite(b.close) || b.close <= 0) continue
    const value = other.get(periodKey(b.close_time, timeframe))
    if (!comparison) return { date: b.close_time.slice(0, 10), main: b.close }
    if (value !== undefined && Number.isFinite(value) && value > 0)
      return { date: b.close_time.slice(0, 10), main: b.close, comparison: value }
  }
  return null
}
export function alignComparison(
  main: ChartBar[],
  comparison: ChartBar[],
  timeframe: Timeframe,
): Array<{ time: string; value?: number }> {
  const other = new Map(comparison.map((b) => [periodKey(b.close_time, timeframe), b.close]))
  return main.map((b) => {
    const value = other.get(periodKey(b.close_time, timeframe))
    return value === undefined
      ? { time: b.close_time.slice(0, 10) }
      : { time: b.close_time.slice(0, 10), value }
  })
}
