import { describe, expect, it } from 'vitest'
import { mockChartQuery, mockSearchInstruments } from './mock'

describe('mockSearchInstruments', () => {
  it('按代码与名称过滤', () => {
    expect(mockSearchInstruments('600519')).toEqual([
      expect.objectContaining({ code: '600519', name: '贵州茅台' }),
    ])
    expect(mockSearchInstruments('茅台')[0]?.name).toBe('贵州茅台')
    expect(mockSearchInstruments('SZSE:002415')[0]?.instrument).toBe('SZSE:002415')
    expect(mockSearchInstruments('不存在')).toEqual([])
    expect(mockSearchInstruments('  ')).toEqual([])
  })
})

describe('mockChartQuery', () => {
  const base = {
    instrument: 'SSE:600519',
    timeframe: 'DAY' as const,
    price_view: 'QFQ' as const,
    limit: 400,
    indicators: [{ kind: 'SMA' as const, period: 5 }],
  }

  it('生成确定的 K 线与指标窗口', () => {
    const first = mockChartQuery(base)
    const again = mockChartQuery(base)
    expect(first.bars).toHaveLength(400)
    expect(first.bars.map((bar) => bar.close_time)).toEqual(
      [...first.bars].map((bar) => bar.close_time).sort(),
    )
    expect(first.has_more).toBe(true)
    expect(first.next_before).toBe(first.bars[0].close_time)
    expect(first.series[0]?.points).toHaveLength(400)
    expect(first.series[0]?.kind).toBe('SMA')
    expect(again).toEqual(first)
  })

  it('QFQ 与 RAW 价格不同且最新一根一致', () => {
    const qfq = mockChartQuery(base)
    const raw = mockChartQuery({ ...base, price_view: 'RAW' })
    expect(qfq.bars[0].close).not.toBe(raw.bars[0].close)
    expect(qfq.bars.at(-1)?.close).toBe(raw.bars.at(-1)?.close)
  })

  it('分页返回更早数据并最终收敛 has_more=false', () => {
    const first = mockChartQuery(base)
    const older = mockChartQuery({ ...base, before: first.next_before!, data_version: first.data_version })
    expect(older.bars.at(-1)!.close_time < first.bars[0].close_time).toBe(true)
    expect(older.has_more).toBe(true)
    const oldest = mockChartQuery({ ...base, before: older.next_before!, data_version: first.data_version })
    expect(oldest.has_more).toBe(false)
    expect(oldest.next_before).toBeNull()
  })

  it('MACD/KDJ 返回对应分量序列', () => {
    const result = mockChartQuery({
      ...base,
      indicators: [
        { kind: 'EMA', period: 10 },
        { kind: 'MACD', fast: 12, slow: 26, signal: 9 },
        { kind: 'KDJ', period: 9 },
      ],
    })
    expect(result.series.map((item) => item.component)).toEqual([
      'line',
      'dif',
      'dea',
      'histogram',
      'k',
      'd',
      'j',
    ])
    for (const item of result.series) expect(item.points.length).toBeGreaterThan(0)
  })

  it('未知证券回退为演示证券信息', () => {
    const result = mockChartQuery({ ...base, instrument: 'SZSE:999999' })
    expect(result.instrument).toEqual({
      instrument: 'SZSE:999999',
      code: '999999',
      name: '演示证券',
      exchange: 'SZSE',
      board: '主板',
      lot_size: 100,
    })
  })
})

it('ZSCORE mock standardizes the two-price log ratio', () => {
  const result = mockChartQuery({
    instrument: 'SSE:600519',
    comparison: 'SHFE:AU.MAIN',
    timeframe: 'DAY',
    price_view: 'RAW',
    limit: 1000,
    indicators: [{ kind: 'ZSCORE', period: 5, smooth: 2, regime: 10 }],
  })
  expect(result.series.map((item) => item.key)).toEqual(
    ['histogram', 'smooth', 'regime'].map((field) => `zscore/day/raw/${field}/p=5/sm=2/rg=10/lag=0/SSE:600519/SHFE:AU.MAIN`),
  )
  for (const [index, window] of [
    [0, 5],
    [2, 10],
  ]) {
    expect(result.series[index].points[0].time).toBe(result.bars[window - 1].close_time)
    const commodity = mockChartQuery({ instrument: 'SHFE:AU.MAIN', timeframe: 'DAY', price_view: 'RAW', limit: 1000, indicators: [] })
    const returns = result.bars.slice(0, window).map((bar, i) => Math.log(bar.close) - Math.log(commodity.bars[i].close))
    const mean = returns.reduce((a, b) => a + b, 0) / window
    const sd = Math.sqrt(returns.reduce((a, b) => a + (b - mean) ** 2, 0) / window)
    expect(result.series[index].points[0].value).toBeCloseTo((returns.at(-1)! - mean) / sd, 6)
  }
})
