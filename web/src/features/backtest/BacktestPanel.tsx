import { useEffect, useRef, useState } from 'react'
import { createBacktestRun, type RunStatus } from '../../api/client'
import { RunMonitor } from '../strategy/RunMonitor'
import { describeTaskError } from '../strategy/taskUtils'
import { useRunPolling } from '../strategy/useRunPolling'
import { useStrategyCatalog } from '../strategy/useStrategyCatalog'
import { BacktestForm } from './BacktestForm'
import { BacktestReport } from './BacktestReport'
import { backtestContext, backtestInput, emptyBacktestDraft, readBacktestPreferences, saveBacktestPreferences, type SubmittedBacktest } from './backtestPreferences'

interface BacktestPanelProps {
  runId: string | null
  onRunIdChange: (runId: string | null) => void
  selectedSymbol: string | null
  selectedName?: string
  defaultLotSize?: number
  active?: boolean
}
/** 草稿、提交快照和已展示报告各自持有身份，切页仅暂停副作用。 */
export function BacktestPanel({ runId, onRunIdChange, selectedSymbol, selectedName, defaultLotSize, active = true }: BacktestPanelProps) {
  const catalog = useStrategyCatalog()
  const [preferences] = useState(readBacktestPreferences)
  const [draft, setDraft] = useState(() => preferences.draft ?? emptyBacktestDraft(selectedSymbol ?? '', defaultLotSize, selectedName))
  const [submitted, setSubmitted] = useState<SubmittedBacktest | null>(preferences.submitted)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [storageError, setStorageError] = useState(false)
  const [report, setReport] = useState<{ run: RunStatus; context: SubmittedBacktest | null } | null>(null)
  const awaitingMetadata = useRef(!preferences.draft && defaultLotSize === undefined)
  const submittingRef = useRef(false)
  const clearedRef = useRef<string | null>(null)
  const polling = useRunPolling('backtest', runId, { active, classifyErrors: true })
  const { status } = polling
  useEffect(() => {
    if (!selectedName || !selectedSymbol) return
    const fillLotSize = awaitingMetadata.current && defaultLotSize !== undefined
    setDraft((current) => current.instrument === selectedSymbol ? {
      ...current, instrumentName: current.instrumentName || selectedName,
      lotSize: fillLotSize ? String(defaultLotSize) : current.lotSize,
    } : current)
    if (defaultLotSize !== undefined) awaitingMetadata.current = false
  }, [selectedSymbol, selectedName, defaultLotSize])
  useEffect(() => {
    const first = catalog.definitions[0]
    if (first) setDraft((current) => current.strategy ? current : { ...current, strategy: first.strategy, version: first.version })
  }, [catalog.definitions])
  useEffect(() => { setStorageError(!saveBacktestPreferences({ draft, submitted })) }, [draft, submitted])
  useEffect(() => {
    if (polling.missing && runId && clearedRef.current !== runId) {
      clearedRef.current = runId
      setNotice('上次回测记录已不可用，可重新发起回测')
      onRunIdChange(null)
    }
  }, [polling.missing, runId, onRunIdChange])
  useEffect(() => {
    if (status?.status === 'SUCCEEDED' && status.summary) {
      setReport((previous) => previous?.run.run_id === status.run_id ? previous : { run: status, context: backtestContext(submitted, status) })
    }
  }, [status, submitted])
  const busy = Boolean(runId && !polling.missing && (!status || status.status === 'PENDING' || status.status === 'RUNNING'))
  const submit = async () => {
    if (busy || submittingRef.current) return
    submittingRef.current = true
    setSubmitting(true)
    setFormError(null)
    try {
      const frozen = structuredClone(draft)
      const definition = catalog.definitions.find((d) => d.strategy === frozen.strategy && d.version === frozen.version)!
      const input = backtestInput(frozen)
      const effectiveParameters = Object.fromEntries(Object.entries(definition.parameters).map(([key, spec]) => [key, input.parameters?.[key] ?? spec.default]))
      const reference = await createBacktestRun(input)
      const next = { runId: reference.run_id, draft: frozen, effectiveParameters, effectiveHoldBars: input.config.hold_bars || definition.default_hold_bars }
      setSubmitted(next)
      setNotice(null)
      onRunIdChange(reference.run_id)
    } catch (cause) { setFormError(describeTaskError(cause)) }
    finally { submittingRef.current = false; setSubmitting(false) }
  }
  const showingCurrent = report?.run.run_id === runId
  return <section className="backtest-workspace" aria-label="策略回测" hidden={!active}>
    <header className="backtest-page-heading"><div><h1>策略回测</h1><p>调整条件，检验单只证券的历史表现</p></div><span className="backtest-mode">单标的 · 手动回测</span></header>
    <BacktestForm draft={draft} onChange={(next) => { if (next.instrument !== draft.instrument || next.lotSize !== draft.lotSize) awaitingMetadata.current = false; setDraft(next) }} definitions={catalog.definitions} loading={catalog.loading} error={catalog.error} retryCatalog={catalog.retry}
      active={active} busy={busy} submitting={submitting} submitError={formError} onSubmit={() => void submit()} />
    {storageError && <p className="backtest-notice" role="alert">浏览器无法保存设置，刷新后可能无法恢复条件。</p>}
    {notice && <p className="backtest-notice" role="status">{notice}</p>}
    {polling.loading && <p className="backtest-notice" role="status">{report ? '正在加载本次回测状态…' : '正在加载上次回测…'}</p>}
    {!showingCurrent && !polling.missing && <RunMonitor key={runId} kind="backtest" status={status} pollingError={polling.error} />}
    {polling.error && !polling.missing && <div className="backtest-notice" role="alert">
      {showingCurrent && <span>任务状态连接中断：{polling.error}（已保留报告） </span>}
      <button type="button" className="table-load-more" onClick={polling.retry}>重新连接</button>
    </div>}
    {report && !showingCurrent && <p className="backtest-notice">{busy ? '新回测进行中，下方仍为上次报告。' : '下方保留上次成功报告。'}</p>}
    {!runId && !report && <div className="backtest-idle">
      <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M3 3v18h18M7 14l4-4 4 3 5-6" /></svg>
      <strong>尚未发起回测</strong><span>设置上方证券、策略与日期后，点击「发起回测」。</span>
    </div>}
    {report && <BacktestReport key={report.run.run_id} run={report.run} context={report.context} active={active} />}
  </section>
}
