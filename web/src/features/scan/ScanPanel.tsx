import { useEffect, useRef, useState } from 'react'
import { createScanRun, type RunStatus } from '../../api/client'
import { RunMonitor } from '../strategy/RunMonitor'
import { describeTaskError, toRFC3339 } from '../strategy/taskUtils'
import { useRunPolling } from '../strategy/useRunPolling'
import { useStrategyCatalog } from '../strategy/useStrategyCatalog'
import { ScanForm } from './ScanForm'
import { ScanResults } from './ScanResults'
import { contextForRun, readScanPreferences, saveScanPreferences, type ScanDraft, type SubmittedScan } from './scanPreferences'

interface ScanPanelProps {
  runId: string | null
  onRunIdChange: (runId: string | null) => void
  onSelectInstrument: (instrument: string) => void
  active?: boolean
}

/** 草稿、当前任务、已完成快照分离；切换视图仅暂停副作用。 */
export function ScanPanel({ runId, onRunIdChange, onSelectInstrument, active = true }: ScanPanelProps) {
  const catalog = useStrategyCatalog()
  const [preferences] = useState(readScanPreferences)
  const [draft, setDraft] = useState(preferences.draft)
  const [submitted, setSubmitted] = useState<SubmittedScan | null>(preferences.submitted)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [storageError, setStorageError] = useState(false)
  const [candidate, setCandidate] = useState<{ run: RunStatus; context: ScanDraft | null } | null>(null)
  const submittingRef = useRef(false)
  const clearedRef = useRef<string | null>(null)
  const polling = useRunPolling('scan', runId, { active, classifyErrors: true })
  const { status, error: pollingError } = polling

  useEffect(() => {
    const first = catalog.definitions[0]
    if (first) setDraft((current) => current.strategy ? current : { ...current, strategy: first.strategy, version: first.version })
  }, [catalog.definitions])
  useEffect(() => { setStorageError(!saveScanPreferences({ draft, submitted })) }, [draft, submitted])
  useEffect(() => {
    if (polling.missing && runId && clearedRef.current !== runId) {
      clearedRef.current = runId
      setNotice('上次扫描记录已不可用，可重新发起扫描')
      onRunIdChange(null)
    }
  }, [polling.missing, runId, onRunIdChange])
  useEffect(() => {
    if (status?.snapshot_id && (status.status === 'SUCCEEDED' || status.status === 'PARTIAL_SUCCEEDED')) {
      setCandidate((previous) => previous?.run.run_id === status.run_id ? previous : { run: status, context: contextForRun(submitted, status) })
    }
  }, [status, submitted])

  const busy = Boolean(runId && !polling.missing && (!status || status.status === 'PENDING' || status.status === 'RUNNING'))
  const submit = async () => {
    if (submittingRef.current || busy) return
    submittingRef.current = true
    setSubmitting(true)
    setFormError(null)
    const frozen = structuredClone(draft)
    try {
      const reference = await createScanRun({
        strategy: frozen.strategy, strategy_version: frozen.version, idempotency_key: crypto.randomUUID(),
        from: toRFC3339(frozen.from), as_of: toRFC3339(frozen.asOf),
        parameters: Object.keys(frozen.parameters).length ? frozen.parameters : undefined,
        scope: { exchanges: frozen.exchanges, active_only: frozen.activeOnly, limit: Number(frozen.limit) },
      })
      const next = { runId: reference.run_id, draft: frozen }
      // 上下文先于任务 ID 落盘；两者不同步时恢复路径拒绝绑定。
      setStorageError(!saveScanPreferences({ draft, submitted: next }))
      setSubmitted(next)
      setNotice(null)
      onRunIdChange(reference.run_id)
    } catch (cause) { setFormError(describeTaskError(cause)) }
    finally { submittingRef.current = false; setSubmitting(false) }
  }
  const terminalShown = Boolean(candidate && status && candidate.run.run_id === status.run_id)
  return <section className="scan-workspace" aria-label="策略扫描" hidden={!active}>
    <header className="scan-page-heading"><div><h1>策略扫描</h1><p>调整条件，发现值得进一步观察的证券</p></div><span className="scan-mode">手动扫描</span></header>
    <ScanForm active={active} draft={draft} onChange={setDraft} definitions={catalog.definitions} loading={catalog.loading} error={catalog.error}
      busy={busy} submitting={submitting} onSubmit={() => void submit()} submitError={formError} />
    {storageError && <p className="scan-notice" role="alert">浏览器无法保存设置，刷新后可能无法恢复条件。</p>}
    {notice && <p className="scan-notice" role="status">{notice}</p>}
    <div className="scan-result-area">
      {polling.loading && <p className="scan-notice" role="status">正在加载上次扫描…</p>}
      {!terminalShown && !polling.missing && <RunMonitor key={runId} kind="scan" status={status} pollingError={pollingError} />}
      {pollingError && !polling.missing && <button type="button" className="table-load-more" onClick={polling.retry}>重新连接</button>}
      {!runId && !candidate && <div className="scan-idle">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><circle cx="10" cy="10" r="6" /><path d="m15 15 6 6" /></svg>
        <strong>尚未发起扫描</strong><span>设置上方策略和日期后，点击「发起扫描」。</span>
      </div>}
      {candidate && <ScanResults run={candidate.run} context={candidate.context} draft={draft} active={active} onSelectInstrument={onSelectInstrument} />}
    </div>
  </section>
}
