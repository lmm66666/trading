import { useState, type FormEvent } from 'react'
import { createScanRun } from '../../api/client'
import { RunMonitor } from '../strategy/RunMonitor'
import { StrategyForm, type StrategyFormValue } from '../strategy/StrategyForm'
import { describeTaskError, toRFC3339, validateDateRange } from '../strategy/taskUtils'
import { useRunPolling } from '../strategy/useRunPolling'
import { useStrategyCatalog } from '../strategy/useStrategyCatalog'
import { ScanResults } from './ScanResults'

const EXCHANGE_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'SSE', label: '上交所 SSE' },
  { value: 'SZSE', label: '深交所 SZSE' },
  { value: 'BSE', label: '北交所 BSE' },
]

interface ScanPanelProps {
  runId: string | null
  onRunIdChange: (runId: string | null) => void
  onSelectInstrument: (instrument: string) => void
}

/** 扫描视图：表单 → 任务状态条 → 结果表格 三段式 */
export function ScanPanel({ runId, onRunIdChange, onSelectInstrument }: ScanPanelProps) {
  const catalog = useStrategyCatalog()
  const [selection, setSelection] = useState<StrategyFormValue>({ strategy: '', version: '', parameters: {} })
  const [paramsValid, setParamsValid] = useState(true)
  const [from, setFrom] = useState('')
  const [asOf, setAsOf] = useState('')
  const [exchanges, setExchanges] = useState<string[]>([])
  const [activeOnly, setActiveOnly] = useState(true)
  const [limit, setLimit] = useState('5000')
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  const { status, error: pollingError } = useRunPolling('scan', runId)

  const toggleExchange = (exchange: string) => {
    setExchanges((previous) =>
      previous.includes(exchange)
        ? previous.filter((item) => item !== exchange)
        : [...previous, exchange],
    )
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const dateError = validateDateRange(from, asOf)
    if (dateError) {
      setFormError(dateError)
      return
    }
    if (!selection.strategy || !paramsValid) {
      setFormError('请选择策略并检查参数')
      return
    }
    const parsedLimit = Number(limit)
    if (!Number.isInteger(parsedLimit) || parsedLimit < 1 || parsedLimit > 5000) {
      setFormError('数量上限需在 1–5000 之间')
      return
    }
    setSubmitting(true)
    setFormError(null)
    try {
      const reference = await createScanRun({
        strategy: selection.strategy,
        strategy_version: selection.version,
        idempotency_key: crypto.randomUUID(),
        from: toRFC3339(from),
        as_of: toRFC3339(asOf),
        parameters: Object.keys(selection.parameters).length > 0 ? selection.parameters : undefined,
        scope: { exchanges, active_only: activeOnly, limit: parsedLimit },
      })
      onRunIdChange(reference.run_id)
    } catch (cause) {
      setFormError(describeTaskError(cause))
    } finally {
      setSubmitting(false)
    }
  }

  const terminalResult =
    status && (status.status === 'SUCCEEDED' || status.status === 'PARTIAL_SUCCEEDED') && status.snapshot_id
      ? status
      : null

  return (
    <main className="task-panel" aria-label="策略扫描">
      <section className="panel-card">
        <h2>策略扫描</h2>
        <form onSubmit={submit} noValidate>
          <StrategyForm
            definitions={catalog.definitions}
            loading={catalog.loading}
            error={catalog.error}
            value={selection}
            onChange={(next, valid) => {
              setSelection(next)
              setParamsValid(valid)
            }}
          />
          <div className="field-row">
            <label className="field">
              开始日期
              <input type="date" value={from} onChange={(event) => setFrom(event.target.value)} />
            </label>
            <label className="field">
              截止日期
              <input type="date" value={asOf} onChange={(event) => setAsOf(event.target.value)} />
            </label>
          </div>
          <fieldset className="scope-fieldset">
            <legend>范围</legend>
            <div className="checkbox-row">
              {EXCHANGE_OPTIONS.map((option) => (
                <label key={option.value}>
                  <input
                    type="checkbox"
                    checked={exchanges.includes(option.value)}
                    onChange={() => toggleExchange(option.value)}
                  />
                  {' '}{option.label}
                </label>
              ))}
            </div>
            <p className="scope-hint">全部不勾选表示覆盖所有支持的交易所</p>
            <div className="field-row">
              <label className="field">
                <input type="checkbox" checked={activeOnly} onChange={(event) => setActiveOnly(event.target.checked)} />
                {' '}仅活跃证券
              </label>
              <label className="field">
                数量上限
                <input
                  type="number"
                  min={1}
                  max={5000}
                  value={limit}
                  onChange={(event) => setLimit(event.target.value)}
                />
              </label>
            </div>
          </fieldset>
          {formError && <p className="form-error">{formError}</p>}
          <button className="submit-task" disabled={submitting} type="submit">
            {submitting ? '提交中…' : '发起扫描'}
          </button>
        </form>
      </section>
      <RunMonitor kind="scan" status={status} pollingError={pollingError} />
      {terminalResult && <ScanResults run={terminalResult} onSelectInstrument={onSelectInstrument} />}
    </main>
  )
}
