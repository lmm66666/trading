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
export interface SubmittedScan { runId: string; draft: ScanDraft }
export interface ScanPreferences { draft: ScanDraft; submitted: SubmittedScan | null }
const KEY = 'wb.scan_preferences'
export const emptyScanDraft = (): ScanDraft => ({ strategy: '', version: '', parameters: {}, from: '', asOf: '', exchanges: [], activeOnly: true, limit: '5000' })
const object = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value)
const validDate = (value: unknown) => typeof value === 'string' && (value === '' || (/^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(Date.parse(value)) && new Date(value).toISOString().slice(0, 10) === value))
function validDraft(value: unknown): value is ScanDraft {
  if (!object(value)) return false
  return typeof value.strategy === 'string' && value.strategy.length <= 128 && typeof value.version === 'string' && value.version.length <= 32 &&
    object(value.parameters) && Object.keys(value.parameters).length <= 100 && Object.values(value.parameters).every((v) => typeof v === 'number' && Number.isFinite(v)) &&
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
      ? { runId: submitted.runId, draft: submitted.draft } : null }
  } catch { return fallback }
}
export function saveScanPreferences(preferences: ScanPreferences): boolean {
  try { localStorage.setItem(KEY, JSON.stringify({ version: 1, ...preferences })); return true } catch { return false }
}
export function contextForRun(submitted: SubmittedScan | null, run: RunStatus): ScanDraft | null {
  return submitted?.runId === run.run_id && submitted.draft.strategy === run.strategy && submitted.draft.version === run.strategy_version ? submitted.draft : null
}
export const STRATEGY_NAMES: Record<string, string> = { daily_b1_buy: '日线 B1', weekly_b1_buy: '周线 B1', bottom_surge_pullback: '底部倍量回撤' }
export const strategyName = (id: string) => STRATEGY_NAMES[id] ?? id
export const PARAMETER_NAMES: Record<string, string> = { lookback_days: '回看天数', pullback_bars: '回撤根数', volume_ratio: '成交量倍数', volume_period: '均量周期', rally_pct: '上涨幅度（%）', pullback_pct: '回撤幅度（%）', kdj_threshold: 'KDJ 阈值', ma_trend_lookback: '均线趋势回看', low_band_pct: '底部区间（%）', single_volume_ratio: '单日倍量', single_rally_pct: '单日涨幅（%）', gradual_days: '温和上涨天数', gradual_volume_ratio: '温和上涨量比', gradual_rally_pct: '温和涨幅（%）', surge_gap: '倍量间隔', j_min: 'J 值下限', j_max: 'J 值上限' }
export function sameDraft(a: ScanDraft, b: ScanDraft): boolean {
  const normalize = (d: ScanDraft) => JSON.stringify({ ...d, exchanges: [...d.exchanges].sort(), parameters: Object.entries(d.parameters).sort(([a], [b]) => a.localeCompare(b)) })
  return normalize(a) === normalize(b)
}
export function describeScan(draft: ScanDraft): string {
  const scope = draft.exchanges.length ? draft.exchanges.join(' / ') : '全部交易所'
  const params = Object.entries(draft.parameters).map(([key, value]) => `${PARAMETER_NAMES[key] ?? key} ${value}`).join(' · ')
  return `${draft.from} → ${draft.asOf} · ${scope} · ${draft.activeOnly ? '仅活跃证券' : '含非活跃证券'} · 上限 ${draft.limit}${params ? ` · ${params}` : ' · 默认参数'}`
}
