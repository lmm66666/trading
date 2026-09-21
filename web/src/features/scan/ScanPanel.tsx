import { useState, type FormEvent } from 'react'
import { createScanRun } from '../../api/client'
import { RunMonitor } from '../strategy/RunMonitor'
import { RangePicker } from '../strategy/RangePicker'
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

/** 扫描视图：左配置栏（策略/时间/范围）→ 右侧任务状态条，终态合并为单张结果卡 */
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
    if (!selection.strategy) {
      setFormError('请选择策略')
      return
    }
    if (!paramsValid) {
      setFormError('策略参数超出范围，请修正后重试')
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
    <main className="task-workspace" aria-label="策略扫描">
      <aside className="config-column">
        <section className="panel-card config-card">
          <div className="config-head">
            <h2>策略扫描</h2>
            <p>对全市场或指定交易所运行策略信号扫描</p>
          </div>
          <form onSubmit={submit} noValidate>
            <div className="form-section">
              <div className="section-label">策略</div>
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
            </div>
            <div className="form-section">
              <div className="section-label">时间范围</div>
              <RangePicker
                ariaLabel="扫描时间范围"
                from={from}
                to={asOf}
                onChange={(nextFrom, nextTo) => {
                  setFrom(nextFrom)
                  setAsOf(nextTo)
                }}
              />
            </div>
            <div className="form-section">
              <div className="section-label">范围</div>
              <div className="scope-block">
                <div className="chip-row" role="group" aria-label="交易所范围">
                  {EXCHANGE_OPTIONS.map((option) => (
                    <button
                      key={option.value}
                      type="button"
                      className="chip"
                      aria-pressed={exchanges.includes(option.value)}
                      onClick={() => toggleExchange(option.value)}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <p className="scope-hint">全部不勾选表示覆盖所有支持的交易所</p>
                <div className="scope-inline">
                  <label className="switch-field">
                    <span className="switch">
                      <input
                        type="checkbox"
                        checked={activeOnly}
                        onChange={(event) => setActiveOnly(event.target.checked)}
                      />
                      <i aria-hidden="true" />
                    </span>
                    仅活跃证券
                  </label>
                  <label className="inline-input">
                    数量上限
                    <input
                      className="control"
                      type="number"
                      min={1}
                      max={5000}
                      value={limit}
                      onChange={(event) => setLimit(event.target.value)}
                    />
                  </label>
                </div>
              </div>
            </div>
            {formError && <p className="form-error">{formError}</p>}
            <button className="submit-task" disabled={submitting} type="submit">
              {submitting ? '提交中…' : '发起扫描'}
            </button>
          </form>
        </section>
      </aside>
      <div className="result-column">
        {!terminalResult && <RunMonitor kind="scan" status={status} pollingError={pollingError} />}
        {!status && !pollingError && (
          <div className="result-empty">
            <svg className="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <circle cx="11" cy="11" r="7" />
              <path d="m21 21-4.3-4.3" />
            </svg>
            <strong>尚未发起扫描</strong>
            <span>在左侧配置策略与时间范围，点击「发起扫描」后结果将在此展示</span>
          </div>
        )}
        {terminalResult && <ScanResults run={terminalResult} onSelectInstrument={onSelectInstrument} />}
      </div>
    </main>
  )
}
