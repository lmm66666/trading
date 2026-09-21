import { toScaled, type BacktestRunInput, type RunStatus, type StrategyDefinition } from '../../api/client'
import { toRFC3339, validateDateRange } from '../strategy/taskUtils'

export interface BacktestDraft {
  instrument: string
  instrumentName: string
  strategy: string
  version: string
  parameters: Record<string, string>
  start: string
  end: string
  initialCash: string
  cashPercent: string
  commissionPercent: string
  minimumCommission: string
  stampDutyPercent: string
  transferFeePercent: string
  slippagePercent: string
  lotSize: string
  holdBars: string
}
export interface SubmittedBacktest { runId: string; draft: BacktestDraft; effectiveParameters: Record<string, number>; effectiveHoldBars: number }
export interface BacktestPreferences { draft: BacktestDraft | null; submitted: SubmittedBacktest | null }
const KEY = 'wb.backtest_preferences'
export const emptyBacktestDraft = (instrument = '', lotSize = 100, instrumentName = ''): BacktestDraft => ({
  instrument, instrumentName, strategy: '', version: '', parameters: {}, start: '', end: '',
  initialCash: '1000000', cashPercent: '100', commissionPercent: '0.03', minimumCommission: '5',
  stampDutyPercent: '0.05', transferFeePercent: '0', slippagePercent: '0.05', lotSize: String(lotSize), holdBars: '',
})
/** 按十进制位换算，不用浮点乘法判断是否为整数。 */
export function percentToBps(text: string): number | null {
  if (!/^\d+(\.\d{1,2})?$/.test(text)) return null
  const [whole, fraction = ''] = text.split('.')
  return Number(whole) * 100 + Number(fraction.padEnd(2, '0'))
}
export const FEE_FIELDS = [
  ['commissionPercent', '佣金（%）'], ['stampDutyPercent', '印花税（%）'],
  ['transferFeePercent', '过户费（%）'], ['slippagePercent', '滑点（%）'],
] as const
export function validateBacktestDraft(d: BacktestDraft, definition?: StrategyDefinition): Record<string, string> {
  const errors: Record<string, string> = {}
  const dateError = validateDateRange(d.start, d.end)
  if (dateError) errors.dates = dateError
  if (!d.instrument) errors.instrument = '请先选择回测标的'
  if (!definition) errors.strategy = '请选择可用策略'
  const cash = Number(d.initialCash)
  if (!d.initialCash.trim() || !Number.isFinite(cash) || cash < 1000 || cash > 1e9) errors.initialCash = '初始资金需在 1 千–10 亿元之间'
  for (const [key, label] of [['cashPercent', '资金使用比例'], ...FEE_FIELDS] as const) {
    const bps = percentToBps(d[key])
    if (bps === null || bps < (key === 'cashPercent' ? 1 : 0) || bps > 10000) errors[key] = `${label}需在 ${key === 'cashPercent' ? '0.01' : '0'}–100% 之间，最多两位小数`
  }
  const minimum = Number(d.minimumCommission)
  if (!d.minimumCommission.trim() || !Number.isFinite(minimum) || minimum < 0 || minimum > 1e9) errors.minimumCommission = '最低佣金需在 0–10 亿元之间'
  if (!Number.isInteger(Number(d.lotSize)) || Number(d.lotSize) <= 0) errors.lotSize = '每手股数必须是正整数'
  if (!Number.isInteger(Number(d.holdBars)) || Number(d.holdBars) < 0) errors.holdBars = '持有期必须是非负整数'
  for (const [key, text] of Object.entries(d.parameters)) {
    const spec = definition?.parameters[key]
    const value = Number(text)
    if (!spec || (text.trim() && (!Number.isFinite(value) || value < spec.min || value > spec.max || (spec.integer && !Number.isInteger(value))))) errors.parameters = '策略参数超出范围，请修正后重试'
  }
  return errors
}
export function backtestInput(d: BacktestDraft): BacktestRunInput {
  const parameters = Object.fromEntries(Object.entries(d.parameters).filter(([, text]) => text.trim()).map(([key, text]) => [key, Number(text)]))
  return {
    instrument: d.instrument, strategy: d.strategy, strategy_version: d.version, idempotency_key: crypto.randomUUID(),
    start: toRFC3339(d.start), end: toRFC3339(d.end), parameters: Object.keys(parameters).length ? parameters : undefined,
    config: {
      initial_cash: toScaled(Number(d.initialCash)), cash_fraction_bps: percentToBps(d.cashPercent)!,
      commission_bps: percentToBps(d.commissionPercent)!, minimum_commission: toScaled(Number(d.minimumCommission)),
      stamp_duty_bps: percentToBps(d.stampDutyPercent)!, transfer_fee_bps: percentToBps(d.transferFeePercent)!,
      slippage_bps: percentToBps(d.slippagePercent)!, lot_size: Number(d.lotSize), hold_bars: Number(d.holdBars),
    },
  }
}
const object = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value)
function validDraft(value: unknown): value is BacktestDraft {
  if (!object(value)) return false
  return Object.keys(emptyBacktestDraft()).filter((key) => key !== 'parameters').every((key) => typeof value[key] === 'string' && (value[key] as string).length <= 256) &&
    object(value.parameters) && Object.keys(value.parameters).length <= 100 && Object.values(value.parameters).every((v) => typeof v === 'string' && v.length <= 128)
}
export function readBacktestPreferences(): BacktestPreferences {
  const fallback = { draft: null, submitted: null }
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw || raw.length > 32768) return fallback
    const data: unknown = JSON.parse(raw)
    if (!object(data) || data.version !== 1 || !validDraft(data.draft)) return fallback
    const s = data.submitted
    const valid = object(s) && typeof s.runId === 'string' && s.runId.length <= 256 && validDraft(s.draft) &&
      object(s.effectiveParameters) && Object.keys(s.effectiveParameters).length <= 100 && Object.values(s.effectiveParameters).every((v) => typeof v === 'number' && Number.isFinite(v)) &&
      typeof s.effectiveHoldBars === 'number' && Number.isInteger(s.effectiveHoldBars) && s.effectiveHoldBars >= 0
    return { draft: data.draft, submitted: valid ? s as unknown as SubmittedBacktest : null }
  } catch { return fallback }
}
export function saveBacktestPreferences(preferences: BacktestPreferences): boolean {
  try { localStorage.setItem(KEY, JSON.stringify({ version: 1, ...preferences })); return true } catch { return false }
}
export function backtestContext(submitted: SubmittedBacktest | null, run: RunStatus): SubmittedBacktest | null {
  return submitted?.runId === run.run_id && submitted.draft.strategy === run.strategy && submitted.draft.version === run.strategy_version ? submitted : null
}
