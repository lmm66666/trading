import type {
  ChartBar,
  ChartPoint,
  ChartQueryInput,
  ChartResult,
  ChartSeries,
  IndicatorRequest,
  InstrumentSummary,
} from './client'

// 仅供 vite dev 使用的演示数据：后端不可达时由 client.ts 回退到这里。
// 所有数据由种子随机数生成，同参数输出完全确定，分页与合并行为与真实接口一致。

const MOCK_INSTRUMENTS: readonly InstrumentSummary[] = [
  { instrument: 'SSE:600519', code: '600519', name: '贵州茅台', exchange: 'SSE', board: '主板', lot_size: 100 },
  { instrument: 'SSE:601318', code: '601318', name: '中国平安', exchange: 'SSE', board: '主板', lot_size: 100 },
  { instrument: 'SSE:688981', code: '688981', name: '中芯国际', exchange: 'SSE', board: '科创板', lot_size: 200 },
  { instrument: 'SZSE:000001', code: '000001', name: '平安银行', exchange: 'SZSE', board: '主板', lot_size: 100 },
  { instrument: 'SZSE:000858', code: '000858', name: '五粮液', exchange: 'SZSE', board: '主板', lot_size: 100 },
  { instrument: 'SZSE:002415', code: '002415', name: '海康威视', exchange: 'SZSE', board: '主板', lot_size: 100 },
  { instrument: 'SZSE:300750', code: '300750', name: '宁德时代', exchange: 'SZSE', board: '创业板', lot_size: 100 },
  { instrument: 'BSE:920002', code: '920002', name: '示例股份', exchange: 'BSE', board: '北交所', lot_size: 100 },
]

export function mockSearchInstruments(query: string): InstrumentSummary[] {
  const keyword = query.trim().toLowerCase()
  if (!keyword) return []
  return MOCK_INSTRUMENTS.filter(
    (item) =>
      item.code.includes(keyword) ||
      item.name.toLowerCase().includes(keyword) ||
      item.instrument.toLowerCase().includes(keyword),
  ).slice(0, 20)
}

function hashSeed(text: string): number {
  let hash = 2166136261
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return hash >>> 0
}

function mulberry32(seed: number): () => number {
  let state = seed
  return () => {
    state = (state + 0x6d2b79f5) | 0
    let t = Math.imul(state ^ (state >>> 15), 1 | state)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function formatDate(date: Date): string {
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

function tradingDates(count: number, weekly: boolean): string[] {
  const cursor = new Date()
  const dates: string[] = []
  if (weekly) {
    while (cursor.getDay() !== 5) cursor.setDate(cursor.getDate() - 1)
    for (let i = 0; i < count; i++) {
      dates.push(formatDate(cursor))
      cursor.setDate(cursor.getDate() - 7)
    }
  } else {
    while (dates.length < count) {
      const day = cursor.getDay()
      if (day !== 0 && day !== 6) dates.push(formatDate(cursor))
      cursor.setDate(cursor.getDate() - 1)
    }
  }
  return dates.reverse()
}

function round2(value: number): number {
  return Math.round(value * 100) / 100
}

function buildBars(instrument: string, timeframe: 'DAY' | 'WEEK', priceView: 'RAW' | 'QFQ'): ChartBar[] {
  const seed = hashSeed(`${instrument}|${timeframe}`)
  const random = mulberry32(seed)
  const weekly = timeframe === 'WEEK'
  const count = weekly ? 620 : 1000
  const volatility = weekly ? 0.05 : 0.02
  const startPrice = 6 + (seed % 4000) / 100
  const baseVolume = 8_000_000 + (seed % 60) * 2_000_000

  const candles: Array<{ open: number; high: number; low: number; close: number }> = []
  let previousClose = startPrice
  for (let i = 0; i < count; i++) {
    const open = previousClose * (1 + (random() - 0.5) * 0.006)
    const close = Math.max(1, open * (1 + (random() - 0.485) * volatility * 2))
    const high = Math.max(open, close) * (1 + random() * volatility * 0.6)
    const low = Math.min(open, close) * (1 - random() * volatility * 0.6)
    candles.push({ open, high, low, close })
    previousClose = close
  }

  const dates = tradingDates(count, weekly)
  return candles.map((candle, index) => {
    // 前复权：越早的 K 线价格越低，最新一根与不复权一致
    const adjust = priceView === 'QFQ' ? 1 - 0.0002 * (count - 1 - index) : 1
    const open = round2(candle.open * adjust)
    const high = round2(candle.high * adjust)
    const low = round2(candle.low * adjust)
    const close = round2(candle.close * adjust)
    const spike = random() > 0.97 ? 2.5 : 1
    const volume = Math.round(baseVolume * (0.55 + random() * 0.9) * spike)
    return {
      open_time: `${dates[index]}T09:31:00+08:00`,
      close_time: `${dates[index]}T15:00:00+08:00`,
      open,
      high,
      low,
      close,
      volume,
      amount: Math.round(volume * close),
      trading_status: 0,
    }
  })
}

const barsCache = new Map<string, ChartBar[]>()

function masterBars(instrument: string, timeframe: 'DAY' | 'WEEK', priceView: 'RAW' | 'QFQ'): ChartBar[] {
  const key = `${instrument}|${timeframe}|${priceView}`
  let bars = barsCache.get(key)
  if (!bars) {
    bars = buildBars(instrument, timeframe, priceView)
    barsCache.set(key, bars)
  }
  return bars
}

interface ValueWindow {
  start: number
  end: number
}

function pointsFrom(values: Array<number | null>, bars: ChartBar[], window: ValueWindow): ChartPoint[] {
  const points: ChartPoint[] = []
  for (let i = window.start; i < window.end; i++) {
    const value = values[i]
    if (value !== null) points.push({ time: bars[i].close_time, value })
  }
  return points
}

function smaSeries(values: number[], period: number): Array<number | null> {
  const out: Array<number | null> = new Array(values.length).fill(null)
  let sum = 0
  for (let i = 0; i < values.length; i++) {
    sum += values[i]
    if (i >= period) sum -= values[i - period]
    if (i >= period - 1) out[i] = sum / period
  }
  return out
}

function emaSeries(values: number[], period: number, startIndex: number): Array<number | null> {
  const out: Array<number | null> = new Array(values.length).fill(null)
  if (startIndex >= values.length) return out
  let ema = values[startIndex]
  out[startIndex] = ema
  const k = 2 / (period + 1)
  for (let i = startIndex + 1; i < values.length; i++) {
    ema = values[i] * k + ema * (1 - k)
    out[i] = ema
  }
  return out
}

function macdSeries(
  closes: number[],
  fast: number,
  slow: number,
  signal: number,
): { dif: Array<number | null>; dea: Array<number | null>; hist: Array<number | null> } {
  const emaFast = emaSeries(closes, fast, 0)
  const emaSlow = emaSeries(closes, slow, 0)
  const dif: Array<number | null> = closes.map((_, i) => (i < slow - 1 ? null : emaFast[i]! - emaSlow[i]!))
  const dea: Array<number | null> = new Array(closes.length).fill(null)
  const hist: Array<number | null> = new Array(closes.length).fill(null)
  const first = dif.findIndex((value) => value !== null)
  if (first >= 0) {
    let emaSignal = dif[first]!
    dea[first] = emaSignal
    const k = 2 / (signal + 1)
    for (let i = first + 1; i < closes.length; i++) {
      emaSignal = dif[i]! * k + emaSignal * (1 - k)
      dea[i] = emaSignal
      hist[i] = (dif[i]! - emaSignal) * 2
    }
  }
  return { dif, dea, hist }
}

function kdjSeries(
  closes: number[],
  highs: number[],
  lows: number[],
  period: number,
): { k: Array<number | null>; d: Array<number | null>; j: Array<number | null> } {
  const k: Array<number | null> = new Array(closes.length).fill(null)
  const d: Array<number | null> = new Array(closes.length).fill(null)
  const j: Array<number | null> = new Array(closes.length).fill(null)
  let prevK = 50
  let prevD = 50
  for (let i = period - 1; i < closes.length; i++) {
    let highest = -Infinity
    let lowest = Infinity
    for (let n = i - period + 1; n <= i; n++) {
      highest = Math.max(highest, highs[n])
      lowest = Math.min(lowest, lows[n])
    }
    const rsv = highest === lowest ? 50 : ((closes[i] - lowest) / (highest - lowest)) * 100
    const currentK = (2 / 3) * prevK + (1 / 3) * rsv
    const currentD = (2 / 3) * prevD + (1 / 3) * currentK
    k[i] = currentK
    d[i] = currentD
    j[i] = 3 * currentK - 2 * currentD
    prevK = currentK
    prevD = currentD
  }
  return { k, d, j }
}

function indicatorSeries(
  indicator: IndicatorRequest,
  master: { bars: ChartBar[]; closes: number[]; highs: number[]; lows: number[] },
  window: ValueWindow,
): ChartSeries[] {
  const build = (key: string, component: string, values: Array<number | null>): ChartSeries => ({
    key,
    kind: indicator.kind,
    component,
    points: pointsFrom(values, master.bars, window),
  })
  switch (indicator.kind) {
    case 'SMA':
      return [build(`SMA/p=${indicator.period}`, 'line', smaSeries(master.closes, indicator.period))]
    case 'EMA':
      return [build(`EMA/p=${indicator.period}`, 'line', emaSeries(master.closes, indicator.period, 0))]
    case 'MACD': {
      const key = `MACD/f=${indicator.fast}/s=${indicator.slow}/g=${indicator.signal}`
      const { dif, dea, hist } = macdSeries(master.closes, indicator.fast, indicator.slow, indicator.signal)
      return [
        build(`${key}:dif`, 'dif', dif),
        build(`${key}:dea`, 'dea', dea),
        build(`${key}:hist`, 'histogram', hist),
      ]
    }
    case 'KDJ': {
      const key = `KDJ/p=${indicator.period}`
      const { k, d, j } = kdjSeries(master.closes, master.highs, master.lows, indicator.period)
      return [build(`${key}:k`, 'k', k), build(`${key}:d`, 'd', d), build(`${key}:j`, 'j', j)]
    }
  }
}

function instrumentSummary(instrument: string): InstrumentSummary {
  const known = MOCK_INSTRUMENTS.find((item) => item.instrument === instrument)
  if (known) return { ...known }
  const separator = instrument.indexOf(':')
  const exchange = separator > 0 ? instrument.slice(0, separator) : ''
  const code = separator > 0 ? instrument.slice(separator + 1) : instrument
  return {
    instrument,
    code,
    name: '演示证券',
    exchange: exchange === 'SZSE' || exchange === 'BSE' ? exchange : 'SSE',
    board: '主板',
    lot_size: 100,
  }
}

export function mockChartQuery(input: ChartQueryInput): ChartResult {
  const bars = masterBars(input.instrument, input.timeframe, input.price_view)
  const limit = Math.min(Math.max(input.limit ?? 400, 1), bars.length)
  let end = bars.length
  if (input.before) {
    const index = bars.findIndex((bar) => bar.close_time >= input.before!)
    if (index >= 0) end = index
  }
  const start = Math.max(0, end - limit)
  const window = { start, end }
  const closes = bars.map((bar) => bar.close)
  const highs = bars.map((bar) => bar.high)
  const lows = bars.map((bar) => bar.low)
  return {
    instrument: instrumentSummary(input.instrument),
    timeframe: input.timeframe,
    price_view: input.price_view,
    data_version: 1,
    bars: bars.slice(start, end),
    series: input.indicators.flatMap((indicator) => indicatorSeries(indicator, { bars, closes, highs, lows }, window)),
    has_more: start > 0,
    next_before: start > 0 ? bars[start].close_time : null,
  }
}
