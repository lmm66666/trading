import { beforeEach, expect, it, vi } from 'vitest'
import { emptyBacktestDraft, readBacktestPreferences, saveBacktestPreferences, percentToBps, validateBacktestDraft } from './backtestPreferences'
const definition = { strategy: 's', version: '1', primary_timeframe: 'daily', warmup_bars: 0, default_hold_bars: 10, parameters: {}, features: [], auxiliary: [] }
beforeEach(() => localStorage.clear())
it('百分比精确换算整数bps，不截断非法精度和空值', () => {
  expect(percentToBps('0.03')).toBe(3)
  expect(percentToBps('29.99')).toBe(2999)
  expect(percentToBps('100')).toBe(10000)
  for (const text of ['', '0.001', '1e2', '-1', 'abc']) expect(percentToBps(text)).toBeNull()
})
it('恢复有界草稿和提交上下文，异常存储降级', () => {
  const draft = emptyBacktestDraft('SSE:600000', 100, '浦发银行')
  const submitted = { runId: 'r', draft, effectiveParameters: {}, effectiveHoldBars: 10 }
  expect(saveBacktestPreferences({ draft, submitted })).toBe(true)
  expect(readBacktestPreferences()).toEqual({ draft, submitted })
  for (const raw of ['{', '{}', JSON.stringify({ version: 1, draft: { ...draft, parameters: null } }), 'x'.repeat(40000)]) {
    localStorage.setItem('wb.backtest_preferences', raw)
    expect(readBacktestPreferences().submitted).toBeNull()
    expect(readBacktestPreferences().draft).toBeNull()
  }
  const mock = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw Error('full') })
  expect(saveBacktestPreferences({ draft, submitted })).toBe(false)
  mock.mockRestore()
})
it('边界校验阻止空值、资金超限、非法参数及百分比精度', () => {
  const draft = { ...emptyBacktestDraft('SSE:600000'), strategy: 's', version: '1', start: '2026-01-01', end: '2026-02-01' }
  expect(validateBacktestDraft(draft, definition)).toEqual({})
  expect(validateBacktestDraft({ ...draft, initialCash: '', minimumCommission: '', cashPercent: '0', commissionPercent: '0.001', lotSize: '0', holdBars: '-1' }, definition)).toMatchObject({ initialCash: expect.any(String), minimumCommission: expect.any(String), cashPercent: expect.any(String), commissionPercent: expect.any(String), lotSize: expect.any(String), holdBars: expect.any(String) })
  expect(validateBacktestDraft({ ...draft, parameters: { unknown: '1' } }, definition)).toHaveProperty('parameters')
})
