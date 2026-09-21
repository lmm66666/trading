import type { ZScoreDiagnostic } from '../../api/client'
import { comparisonOptions } from './boards'
const value = (n: number | null, suffix = '') => n === null ? '—' : `${n.toFixed(2)}${suffix}`

export function ZScoreSummary({ item }: { item: ZScoreDiagnostic }) {
  return <section className="zscore-summary" aria-label={`Z-score ${item.period}/${item.regime} 诊断`}>
    <strong>Z-score · {comparisonOptions.find(([id]) => id === item.comparison)?.[1] ?? '未选择关联期货'}</strong>
    {item.warning ? <span role="status">{item.warning}</span> : <>
      <span>{item.period}期 Z {value(item.z, 'σ')} · {item.regime}期 Z {value(item.long_z, 'σ')}</span>
      <span>63期相对表现 {value(item.relative_performance, '%')}</span>
      <span>5期收益相关（{item.period}期窗口）{value(item.correlation)}</span>
      <strong>{item.state}</strong>
      <small>最近已知期货收盘价：{item.commodity_date || '—'} · 滞后 {item.lag} 根期货日线 · 期货 RAW 收盘</small>
      <small className="zscore-thresholds"><span>+1～+2σ 偏强观察</span> · <span>−1～−2σ 偏弱观察</span></small>
    </>}
  </section>
}
