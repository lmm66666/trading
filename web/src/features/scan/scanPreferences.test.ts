import { beforeEach, expect, it, vi } from 'vitest'
import { readScanPreferences, saveScanPreferences } from './scanPreferences'
beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })
const draft = { strategy: 'daily_b1_buy', version: '1', parameters: { lookback_days: 30 }, from: '2026-01-01', asOf: '2026-06-01', exchanges: ['SSE'], activeOnly: true, limit: '5000' }
it('round trips one draft and matching submitted context without result data', () => {
  expect(saveScanPreferences({ draft, submitted: { runId: 'r1', draft } })).toBe(true)
  expect(readScanPreferences()).toEqual({ draft, submitted: { runId: 'r1', draft } })
})
it.each(['broken', 'null', '{"version":99}', JSON.stringify({ version: 1, draft: { ...draft, from: '2026-02-30' } }), JSON.stringify({ version: 1, draft: { ...draft, limit: '9000' } }), JSON.stringify({ version: 1, draft: { ...draft, exchanges: ['UNKNOWN'] } })])('rejects unsafe or corrupt preferences: %s', (raw) => {
  localStorage.setItem('wb.scan_preferences', raw)
  expect(readScanPreferences().draft.strategy).toBe('')
})
it('storage failure does not throw or report persistence success', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('denied') })
  expect(saveScanPreferences({ draft, submitted: null })).toBe(false)
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('denied') })
  expect(readScanPreferences().submitted).toBeNull()
})
