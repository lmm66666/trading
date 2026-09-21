import { useState, type FormEvent } from 'react'
import { strategyParamList, type StrategyDefinition } from '../../api/client'
import { RangePicker } from '../strategy/RangePicker'
import { validateDateRange } from '../strategy/taskUtils'
import { PARAMETER_NAMES, strategyName, type ScanDraft } from './scanPreferences'

interface Props {
  active: boolean
  draft: ScanDraft
  onChange: (draft: ScanDraft) => void
  definitions: StrategyDefinition[]
  loading: boolean
  error: string | null
  busy: boolean
  submitting: boolean
  onSubmit: () => void
  submitError: string | null
}
const EXCHANGES = [['SSE', '上交所 SSE'], ['SZSE', '深交所 SZSE'], ['BSE', '北交所 BSE']]

export function ScanForm({ active, draft, onChange, definitions, loading, error, busy, submitting, onSubmit, submitError }: Props) {
  const [validation, setValidation] = useState<string | null>(null)
  const [invalid, setInvalid] = useState<Record<string, string>>({})
  const selected = definitions.find((d) => d.strategy === draft.strategy && d.version === draft.version)
  const params = selected ? strategyParamList(selected) : []
  const patch = (next: Partial<ScanDraft>) => onChange({ ...draft, ...next })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (busy || submitting) return
    const dateError = validateDateRange(draft.from, draft.asOf)
    const invalidParameters = Object.keys(invalid).length > 0 || params.some((p) => {
      const value = draft.parameters[p.name]
      return value !== undefined && (value < p.min || value > p.max || (p.integer && !Number.isInteger(value)))
    }) || Object.keys(draft.parameters).some((key) => !params.some((p) => p.name === key))
    const message = dateError || (!selected ? '请选择可用策略' : invalidParameters ? '策略参数超出范围，请修正后重试' :
      !Number.isInteger(Number(draft.limit)) || Number(draft.limit) < 1 || Number(draft.limit) > 5000 ? '数量上限需在 1–5000 之间' : null)
    setValidation(message)
    if (!message) onSubmit()
  }
  return <form className="scan-form" onSubmit={submit} noValidate>
    <div className="scan-primary-fields">
      <label className="scan-strategy-field">策略
        <select aria-label="策略" value={`${draft.strategy}@${draft.version}`} disabled={loading || Boolean(error)} onChange={(event) => {
          const definition = definitions.find((d) => `${d.strategy}@${d.version}` === event.target.value)
          if (definition) { setInvalid({}); patch({ strategy: definition.strategy, version: definition.version, parameters: {} }) }
        }}>
          {!selected && <option value={`${draft.strategy}@${draft.version}`}>{draft.strategy ? '原策略已不可用，请重新选择' : '选择策略'}</option>}
          {definitions.map((d) => <option key={`${d.strategy}@${d.version}`} value={`${d.strategy}@${d.version}`}>{strategyName(d.strategy)} v{d.version}</option>)}
        </select>
      </label>
      <div className="scan-date-fields">
        <label>开始日期<input aria-label="开始日期" type="date" value={draft.from} onChange={(e) => patch({ from: e.target.value })} /></label>
        <span className="scan-date-arrow" aria-hidden="true">→</span>
        <label>截止日期<input aria-label="截止日期" type="date" value={draft.asOf} onChange={(e) => patch({ asOf: e.target.value })} /></label>
        <div className="scan-calendar">{active && <RangePicker ariaLabel="扫描时间范围" from={draft.from} to={draft.asOf} onChange={(from, asOf) => patch({ from, asOf })} />}</div>
      </div>
      <button className="submit-task" type="submit" disabled={submitting || busy || loading || Boolean(error)}>{submitting ? '提交中…' : busy ? '扫描进行中' : '发起扫描'}</button>
    </div>
    {loading && <p role="status">策略目录加载中…</p>}
    {error && <p className="form-error" role="alert">{error}</p>}
    {!loading && !error && !definitions.length && <p>服务端暂无可用策略</p>}
    <div className="scan-secondary-fields">
      <div className="chip-row" role="group" aria-label="交易所范围">
        <span>交易所</span>
        <button type="button" className="chip" aria-pressed={!draft.exchanges.length} onClick={() => patch({ exchanges: [] })}>全部</button>
        {EXCHANGES.map(([id, name]) => <button key={id} type="button" className="chip" aria-pressed={draft.exchanges.includes(id)} onClick={() => patch({ exchanges: draft.exchanges.includes(id) ? draft.exchanges.filter((e) => e !== id) : [...draft.exchanges, id] })}>{name}</button>)}
      </div>
      <details className="scan-parameters"><summary>策略参数</summary>
        {!params.length && <p className="form-hint">此策略无可调参数</p>}
        <div className="param-grid">{params.map((p) => <label className="field" key={p.name}>
          {PARAMETER_NAMES[p.name] ?? p.name}
          <input type="number" aria-label={`参数 ${p.name}`} aria-invalid={p.name in invalid} min={p.min} max={p.max} step={p.integer ? 1 : 'any'}
            value={invalid[p.name] ?? draft.parameters[p.name] ?? ''} placeholder={`默认 ${p.default}`} onChange={(e) => {
              const text = e.target.value
              const value = Number(text)
              const nextInvalid = { ...invalid }
              const parameters = { ...draft.parameters }
              if (text && (!Number.isFinite(value) || value < p.min || value > p.max || (p.integer && !Number.isInteger(value)))) nextInvalid[p.name] = text
              else { delete nextInvalid[p.name]; if (text) parameters[p.name] = value; else delete parameters[p.name] }
              setInvalid(nextInvalid); patch({ parameters })
            }} />
          <span className={p.name in invalid ? 'field-error' : 'form-hint'}>默认 {p.default} · 范围 {p.min}–{p.max}{p.integer ? '，整数' : ''}</span>
        </label>)}</div>
      </details>
      <details className="scan-advanced"><summary>高级设置</summary><div className="scope-inline">
        <label><input type="checkbox" checked={draft.activeOnly} onChange={(e) => patch({ activeOnly: e.target.checked })} />仅活跃证券</label>
        <label>数量上限<input type="number" min="1" max="5000" value={draft.limit} onChange={(e) => patch({ limit: e.target.value })} /></label>
      </div></details>
    </div>
    <p className="scan-explanation">取窗口内最后一根可用 K 线的信号，并非区间内全部历史信号。</p>
    {(validation || submitError) && <p className="form-error" role="alert">{validation || submitError}</p>}
  </form>
}
