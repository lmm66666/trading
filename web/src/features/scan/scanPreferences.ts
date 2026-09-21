import { PARAMETER_NAMES } from '../strategy/strategyLabels'
import type { RunStatus } from '../../api/client'

export interface ScanDraft {
  strategy: string
  version: string
  parameters: Record<string, number>
  from: string
  asOf: string
  exchanges: string[]
  activeOnly: boolean
  limit: string
}
export interface ScanContext extends ScanDraft { effectiveParameters?: Record<string, number> }
export interface SubmittedScan { runId: string; draft: ScanDraft; effectiveParameters?: Record<string, number> }
export interface ScanPreferences { draft: ScanDraft; submitted: SubmittedScan | null }
const KEY = 'wb.scan_preferences'
export const emptyScanDraft = (): ScanDraft => ({ strategy: '', version: '', parameters: {}, from: '', asOf: '', exchanges: [], activeOnly: true, limit: '5000' })
const object = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value)
const validDate = (value: unknown) => typeof value === 'string' && (value === '' || (/^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(Date.parse(value)) && new Date(value).toISOString().slice(0, 10) === value))
const validParameters = (value: unknown): value is Record<string, number> => object(value) && Object.keys(value).length <= 100 && Object.values(value).every((v) => typeof v === 'number' && Number.isFinite(v))
function validDraft(value: unknown): value is ScanDraft {
  if (!object(value)) return false
  return typeof value.strategy === 'string' && value.strategy.length <= 128 && typeof value.version === 'string' && value.version.length <= 32 &&
    validParameters(value.parameters) &&
    validDate(value.from) && validDate(value.asOf) && Array.isArray(value.exchanges) && value.exchanges.length <= 3 && value.exchanges.every((v) => ['SSE', 'SZSE', 'BSE'].includes(v)) &&
    typeof value.activeOnly === 'boolean' && typeof value.limit === 'string' && Number.isInteger(Number(value.limit)) && Number(value.limit) >= 1 && Number(value.limit) <= 5000
}
export function readScanPreferences(): ScanPreferences {
  const fallback = { draft: emptyScanDraft(), submitted: null }
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw || raw.length > 32768) return fallback
    const data: unknown = JSON.parse(raw)
    if (!object(data) || data.version !== 1 || !validDraft(data.draft)) return fallback
    const submitted = data.submitted
    return { draft: data.draft, submitted: object(submitted) && typeof submitted.runId === 'string' && submitted.runId.length <= 256 && validDraft(submitted.draft)
      ? { runId: submitted.runId, draft: submitted.draft, ...(validParameters(submitted.effectiveParameters) ? { effectiveParameters: submitted.effectiveParameters } : {}) } : null }
  } catch { return fallback }
}
export function saveScanPreferences(preferences: ScanPreferences): boolean {
  try { localStorage.setItem(KEY, JSON.stringify({ version: 1, ...preferences })); return true } catch { return false }
}
export function contextForRun(submitted: SubmittedScan | null, run: RunStatus): ScanContext | null {
  return submitted?.runId === run.run_id && submitted.draft.strategy === run.strategy && submitted.draft.version === run.strategy_version ? { ...submitted.draft, effectiveParameters: submitted.effectiveParameters } : null
}
export function sameDraft(a: ScanDraft, b: ScanDraft): boolean {
  const normalize = (d: ScanDraft) => JSON.stringify({ strategy: d.strategy, version: d.version, from: d.from, asOf: d.asOf, activeOnly: d.activeOnly, limit: d.limit, exchanges: [...d.exchanges].sort(), parameters: Object.entries(d.parameters).sort(([a], [b]) => a.localeCompare(b)) })
  return normalize(a) === normalize(b)
}
export function describeScan(draft: ScanContext): string {
  const scope = draft.exchanges.length ? draft.exchanges.join(' / ') : '全部交易所'
  const params = Object.entries(draft.effectiveParameters ?? draft.parameters).map(([key, value]) => `${PARAMETER_NAMES[key] ?? key} ${value}`).join(' · ')
  return `${draft.from} → ${draft.asOf} · ${scope} · ${draft.activeOnly ? '仅活跃证券' : '含非活跃证券'} · 上限 ${draft.limit}${params ? ` · ${params}` : ' · 默认参数'}`
}
