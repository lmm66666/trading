import { useEffect, useState, type FormEvent } from 'react'
import { strategyParamList, type StrategyDefinition } from '../../api/client'
import { InstrumentSearch } from '../search/InstrumentSearch'
import { RangePicker } from '../strategy/RangePicker'
import { TIMEFRAME_LABELS } from '../strategy/StrategyForm'
import { PARAMETER_NAMES, strategyName } from '../strategy/strategyLabels'
import { FEE_FIELDS, validateBacktestDraft, type BacktestDraft } from './backtestPreferences'

interface Props {
  draft: BacktestDraft
  onChange: (draft: BacktestDraft) => void
  definitions: StrategyDefinition[]
  loading: boolean
  error: string | null
  retryCatalog: () => void
  active: boolean
  busy: boolean
  submitting: boolean
  submitError: string | null
  onSubmit: () => void
}
export function BacktestForm({ draft, onChange, definitions, loading, error, retryCatalog, active, busy, submitting, submitError, onSubmit }: Props) {
  const [searchOpen, setSearchOpen] = useState(false)
  const [feesOpen, setFeesOpen] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const selected = definitions.find((d) => d.strategy === draft.strategy && d.version === draft.version)
  useEffect(() => { if (!active) setSearchOpen(false) }, [active])
  const patch = (next: Partial<BacktestDraft>) => { onChange({ ...draft, ...next }); setErrors({}) }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (busy || submitting || loading || error) return
    const next = validateBacktestDraft(draft, selected)
    setErrors(next)
    if (Object.keys(next).some((key) => ['lotSize', 'minimumCommission', ...FEE_FIELDS.map(([k]) => k)].includes(key))) setFeesOpen(true)
    if (!Object.keys(next).length) onSubmit()
  }
  const field = (key: Exclude<keyof BacktestDraft, 'parameters'>, label: string, min: number, max?: number, step: number | 'any' = 1, placeholder?: string) => <label className="field" key={key}>
    {label}<input type="number" aria-label={label} aria-invalid={Boolean(errors[key])} min={min} max={max} step={step} value={draft[key]} placeholder={placeholder} onChange={(e) => patch({ [key]: e.target.value })} />
    {errors[key] && <span className="field-error">{errors[key]}</span>}
  </label>
  return <form className="backtest-form" onSubmit={submit} noValidate>
    <div className="backtest-primary-fields">
      <div className="backtest-symbol"><span>回测标的</span><button type="button" className="backtest-symbol-button" aria-expanded={searchOpen} onClick={() => setSearchOpen(!searchOpen)}>
        <strong>{draft.instrumentName || draft.instrument || '选择证券'}</strong>{draft.instrumentName && <small>{draft.instrument}</small>}<span>{draft.instrument ? '更换' : '搜索'}</span>
      </button>
        {searchOpen && active && <div className="backtest-search"><InstrumentSearch onSelect={(item) => { patch({ instrument: item.instrument, instrumentName: item.name, lotSize: String(item.lot_size) }); setSearchOpen(false) }} /></div>}
        {errors.instrument && <span className="field-error">{errors.instrument}</span>}
      </div>
      <label className="field">策略<select aria-label="策略" value={`${draft.strategy}@${draft.version}`} disabled={loading || Boolean(error)} onChange={(e) => {
        const d = definitions.find((item) => `${item.strategy}@${item.version}` === e.target.value)
        if (d) patch({ strategy: d.strategy, version: d.version, parameters: {} })
      }}>
        {!selected && <option value={`${draft.strategy}@${draft.version}`}>{draft.strategy ? '原策略已不可用，请重新选择' : '选择策略'}</option>}
        {definitions.map((d) => <option key={`${d.strategy}@${d.version}`} value={`${d.strategy}@${d.version}`}>{strategyName(d.strategy)} v{d.version}</option>)}
      </select>{errors.strategy && <span className="field-error">{errors.strategy}</span>}</label>
      <div className="backtest-dates">
        <label className="field">开始日期<input type="date" value={draft.start} onChange={(e) => patch({ start: e.target.value })} /></label>
        <label className="field">截止日期<input type="date" value={draft.end} onChange={(e) => patch({ end: e.target.value })} /></label>
        {active && <RangePicker ariaLabel="回测时间范围" from={draft.start} to={draft.end} onChange={(start, end) => patch({ start, end })} />}
        {errors.dates && <span className="field-error">{errors.dates}</span>}
      </div>
      <button className="submit-task" type="submit" disabled={busy || submitting || loading || Boolean(error)}>{submitting ? '提交中…' : busy ? '回测进行中' : '发起回测'}</button>
    </div>
    {loading && <p role="status">策略目录加载中…</p>}
    {error && <p className="form-error" role="alert">{error} <button type="button" className="table-load-more" onClick={retryCatalog}>重试加载策略</button></p>}
    {!loading && !error && !definitions.length && <p>服务端暂无可用策略</p>}
    <div className="backtest-assumptions">
      {field('initialCash', '初始资金（元）', 1000, 1e9, 'any')}
      {field('cashPercent', '资金使用比例（%）', 0.01, 100, 0.01)}
      {field('holdBars', '持有期（根）', 0, undefined, 1, selected ? `策略默认 ${selected.default_hold_bars}` : '策略默认')}
      {selected && <p className="form-hint">主周期 {TIMEFRAME_LABELS[selected.primary_timeframe] ?? selected.primary_timeframe} · 预热 {selected.warmup_bars} 根<br />持有期留空或 0 使用策略默认 {selected.default_hold_bars} 根</p>}
    </div>
    <details className="backtest-parameters"><summary>策略参数{errors.parameters ? ' · 请修正参数' : ''}</summary>
      <div className="param-grid">{selected && strategyParamList(selected).map((p) => <label className="field" key={p.name}>{PARAMETER_NAMES[p.name] ?? p.name}
        <input type="number" aria-label={`参数 ${p.name}`} value={draft.parameters[p.name] ?? ''} min={p.min} max={p.max} step={p.integer ? 1 : 'any'} placeholder={`默认 ${p.default}`} onChange={(e) => patch({ parameters: { ...draft.parameters, [p.name]: e.target.value } })} />
        <span className="form-hint">默认 {p.default} · 范围 {p.min}–{p.max}{p.integer ? '，整数' : ''}</span>
      </label>)}</div>
      {selected && !Object.keys(selected.parameters).length && <p className="form-hint">此策略无可调参数</p>}
    </details>
    {errors.parameters && <p className="form-error" role="alert">{errors.parameters}</p>}
    <button type="button" className="advanced-toggle" aria-expanded={feesOpen} onClick={() => setFeesOpen(!feesOpen)}>成交与费用设置 <span aria-hidden="true">{feesOpen ? '−' : '+'}</span></button>
    <p className="backtest-fee-summary">佣金 {draft.commissionPercent}% · 最低 {draft.minimumCommission} 元 · 印花税 {draft.stampDutyPercent}% · 过户费 {draft.transferFeePercent}% · 滑点 {draft.slippagePercent}%</p>
    {feesOpen && <div className="backtest-fees">
      {field('lotSize', '每手股数', 1)}
      {FEE_FIELDS.map(([key, label]) => field(key, label, 0, 100, 0.01))}
      {field('minimumCommission', '最低佣金（元）', 0, 1e9, 'any')}
      <p className="form-hint">费用为演示值，非费率建议</p>
    </div>}
    {submitError && <p className="form-error" role="alert">{submitError}</p>}
  </form>
}
