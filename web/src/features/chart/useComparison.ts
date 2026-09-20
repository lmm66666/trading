import { useEffect, useState } from 'react'
import { queryComparison, type ChartBar, type ChartResult, type Timeframe } from '../../api/client'
import { periodKey } from './comparison'

export function useComparison(instrument: string | null, result: ChartResult | null, timeframe: Timeframe) {
  const [state, setState] = useState<{ key: string; bars: ChartBar[]; error: string; loading: boolean }>({
    key: '',
    bars: [],
    error: '',
    loading: false,
  })
  const [retry, setRetry] = useState(0)
  const from = result?.bars[0] ? periodKey(result.bars[0].close_time, timeframe) : ''
  let to = result?.bars.at(-1)?.close_time.slice(0, 10) ?? ''
  if (to && timeframe === 'WEEK') {
    const end = new Date(periodKey(to, timeframe) + 'T00:00:00Z')
    end.setUTCDate(end.getUTCDate() + 6)
    to = end.toISOString().slice(0, 10)
  }
  const version = result?.data_version ?? 0
  const key = JSON.stringify([instrument, timeframe, version, from, to])
  useEffect(() => {
    if (!instrument || !from || !to || !version) return
    const controller = new AbortController()
    setState({ key, bars: [], error: '', loading: true })
    queryComparison({ instrument, timeframe, version, from, to }, controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return
        if (data.data_version !== version || data.instrument !== instrument)
          throw new Error('关联行情版本或标的不匹配')
        if (!data.bars.length) throw new Error('所选范围内没有关联行情')
        setState({
          key,
          bars: data.bars,
          error: data.bars.length >= 5000 ? '关联行情达到5000根上限，较早区间可能没有显示。' : '',
          loading: false,
        })
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted)
          setState({
            key,
            bars: [],
            error: reason instanceof Error ? reason.message : '关联行情加载失败',
            loading: false,
          })
      })
    return () => controller.abort()
  }, [instrument, timeframe, version, from, to, key, retry])
  return {
    bars: state.key === key ? state.bars : [],
    error: state.key === key ? state.error : '',
    loading: !!instrument && !!from && (state.key !== key || state.loading),
    retry: () => setRetry((n) => n + 1),
  }
}
