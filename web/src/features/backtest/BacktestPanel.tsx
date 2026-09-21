import { useState, type FormEvent } from 'react'
import { createBacktestRun, toScaled, type RunStatus } from '../../api/client'
import { EquityChart } from './EquityChart'
import { OrdersTradesTables } from './OrdersTradesTables'
import { InstrumentSearch } from '../search/InstrumentSearch'
import { RunMonitor } from '../strategy/RunMonitor'
import { RangePicker } from '../strategy/RangePicker'
import { StrategyForm, type StrategyFormValue } from '../strategy/StrategyForm'
import { describeTaskError, toRFC3339, validateDateRange } from '../strategy/taskUtils'
import { useRunPolling } from '../strategy/useRunPolling'
import { useStrategyCatalog } from '../strategy/useStrategyCatalog'

interface BacktestPanelProps {
  runId: string | null
  onRunIdChange: (runId: string | null) => void
  /** 当前选中的证券身份，作为回测标的默认值 */
  selectedSymbol: string | null
  /** 当前证券的手数默认值；无身份信息时回退为 100 */
  defaultLotSize?: number
}

function percent(value: number | null): string {
  return value === null ? '—' : `${(value * 100).toFixed(2)}%`
}

/** 回测视图：左配置栏（标的/策略/时间/执行假设）→ 右侧状态条 + 摘要与订单成交 */
export function BacktestPanel({ runId, onRunIdChange, selectedSymbol, defaultLotSize }: BacktestPanelProps) {
  const catalog = useStrategyCatalog()
  const [instrument, setInstrument] = useState<string | null>(selectedSymbol)
  const [searchOpen, setSearchOpen] = useState(false)
  const [selection, setSelection] = useState<StrategyFormValue>({ strategy: '', version: '', parameters: {} })
  const [paramsValid, setParamsValid] = useState(true)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [initialCash, setInitialCash] = useState('1000000')
  const [cashFractionBps, setCashFractionBps] = useState('10000')
  const [commissionBps, setCommissionBps] = useState('3')
  const [minimumCommission, setMinimumCommission] = useState('5')
  const [stampDutyBps, setStampDutyBps] = useState('5')
  const [transferFeeBps, setTransferFeeBps] = useState('0')
  const [slippageBps, setSlippageBps] = useState('5')
  const [lotSize, setLotSize] = useState(String(defaultLotSize ?? 100))
  const [holdBars, setHoldBars] = useState('')
  const [feesOpen, setFeesOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  const { status, error: pollingError } = useRunPolling('backtest', runId)

  const selectedDefinition = catalog.definitions.find(
    (item) => item.strategy === selection.strategy && item.version === selection.version,
  )

  const chooseInstrument = (item: { instrument: string; lot_size: number }) => {
    setInstrument(item.instrument)
    setLotSize(String(item.lot_size))
    setSearchOpen(false)
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const dateError = validateDateRange(start, end)
    if (dateError) {
      setFormError(dateError)
      return
    }
    if (!instrument) {
      setFormError('请先选择回测标的')
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
    const initialCashYuan = Number(initialCash)
    if (!Number.isFinite(initialCashYuan) || initialCashYuan < 1000 || initialCashYuan > 1e9) {
      setFormError('初始资金需在 1 千–10 亿元之间')
      return
    }
    const cashFraction = Number(cashFractionBps)
    if (!Number.isInteger(cashFraction) || cashFraction < 1 || cashFraction > 10000) {
      setFormError('现金使用比例需在 1–10000 bps 之间')
      return
    }
    const feeValues = [
      { name: '佣金比例', value: commissionBps },
      { name: '印花税', value: stampDutyBps },
      { name: '过户费', value: transferFeeBps },
      { name: '滑点', value: slippageBps },
    ]
    for (const fee of feeValues) {
      const parsed = Number(fee.value)
      if (!Number.isInteger(parsed) || parsed < 0 || parsed > 10000) {
        setFormError(`${fee.name}需在 0–10000 bps 之间`)
        return
      }
    }
    const minCommissionYuan = Number(minimumCommission)
    if (!Number.isFinite(minCommissionYuan) || minCommissionYuan < 0 || minCommissionYuan > 1e9) {
      setFormError('最低佣金需在 0–10 亿元之间')
      return
    }
    const parsedLotSize = Number(lotSize)
    if (!Number.isInteger(parsedLotSize) || parsedLotSize <= 0) {
      setFormError('每手股数必须是正整数')
      return
    }
    let parsedHoldBars = 0
    if (holdBars.trim() !== '') {
      parsedHoldBars = Number(holdBars)
      if (!Number.isInteger(parsedHoldBars) || parsedHoldBars < 0) {
        setFormError('持有期必须是非负整数')
        return
      }
    }
    setSubmitting(true)
    setFormError(null)
    try {
      const reference = await createBacktestRun({
        instrument,
        strategy: selection.strategy,
        strategy_version: selection.version,
        idempotency_key: crypto.randomUUID(),
        start: toRFC3339(start),
        end: toRFC3339(end),
        parameters: Object.keys(selection.parameters).length > 0 ? selection.parameters : undefined,
        config: {
          initial_cash: toScaled(initialCashYuan),
          cash_fraction_bps: cashFraction,
          commission_bps: Number(commissionBps),
          minimum_commission: toScaled(minCommissionYuan),
          stamp_duty_bps: Number(stampDutyBps),
          transfer_fee_bps: Number(transferFeeBps),
          slippage_bps: Number(slippageBps),
          lot_size: parsedLotSize,
          hold_bars: parsedHoldBars,
        },
      })
      onRunIdChange(reference.run_id)
    } catch (cause) {
      setFormError(describeTaskError(cause))
    } finally {
      setSubmitting(false)
    }
  }

  const succeeded = status && status.status === 'SUCCEEDED' && status.summary
    ? { ...status, summary: status.summary }
    : null

  return (
    <main className="task-workspace" aria-label="策略回测">
      <aside className="config-column">
        <section className="panel-card config-card">
          <div className="config-head">
            <h2>策略回测</h2>
            <p>对单一标的回放策略信号并模拟成交</p>
          </div>
          <form onSubmit={submit} noValidate>
            <div className="form-section">
              <div className="section-label">标的</div>
              <div className="instrument-field">
                <span className="instrument-current" aria-label="回测标的">{instrument ?? '未选择证券'}</span>
                <button className="change-instrument" onClick={() => setSearchOpen((open) => !open)} type="button">
                  {instrument ? '更换' : '选择证券'}
                </button>
                {searchOpen && (
                  <div className="instrument-popover">
                    <InstrumentSearch onSelect={chooseInstrument} />
                  </div>
                )}
              </div>
            </div>
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
                ariaLabel="回测时间范围"
                from={start}
                to={end}
                onChange={(nextStart, nextEnd) => {
                  setStart(nextStart)
                  setEnd(nextEnd)
                }}
              />
            </div>
            <div className="form-section">
              <div className="section-label">执行假设</div>
              <div className="field-grid">
                <label className="field">
                  初始资金（元）
                  <input
                    type="number"
                    min={1000}
                    max={1e9}
                    value={initialCash}
                    onChange={(event) => setInitialCash(event.target.value)}
                  />
                </label>
                <label className="field">
                  现金使用比例（bps）
                  <input
                    type="number"
                    min={1}
                    max={10000}
                    value={cashFractionBps}
                    onChange={(event) => setCashFractionBps(event.target.value)}
                  />
                </label>
                <label className="field">
                  每手股数
                  <input
                    type="number"
                    min={1}
                    value={lotSize}
                    onChange={(event) => setLotSize(event.target.value)}
                  />
                </label>
                <label className="field">
                  持有期（根）
                  <input
                    type="number"
                    min={0}
                    placeholder={selectedDefinition ? `策略默认 ${selectedDefinition.default_hold_bars}` : '策略默认'}
                    value={holdBars}
                    onChange={(event) => setHoldBars(event.target.value)}
                  />
                </label>
              </div>
              <button
                type="button"
                className="advanced-toggle"
                aria-expanded={feesOpen}
                onClick={() => setFeesOpen((open) => !open)}
              >
                <svg className="icon chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <path d="m9 18 6-6-6-6" />
                </svg>
                高级费用设置
                <span className="advanced-summary">
                  佣金 {commissionBps}bps · 印花税 {stampDutyBps}bps · 滑点 {slippageBps}bps
                </span>
              </button>
              {feesOpen && (
                <div className="advanced-body">
                  <div className="field-grid">
                    <label className="field">
                      佣金（bps）
                      <input
                        type="number"
                        min={0}
                        max={10000}
                        value={commissionBps}
                        onChange={(event) => setCommissionBps(event.target.value)}
                      />
                    </label>
                    <label className="field">
                      最低佣金（元）
                      <input
                        type="number"
                        min={0}
                        max={1e9}
                        value={minimumCommission}
                        onChange={(event) => setMinimumCommission(event.target.value)}
                      />
                    </label>
                    <label className="field">
                      印花税（bps）
                      <input
                        type="number"
                        min={0}
                        max={10000}
                        value={stampDutyBps}
                        onChange={(event) => setStampDutyBps(event.target.value)}
                      />
                    </label>
                    <label className="field">
                      过户费（bps）
                      <input
                        type="number"
                        min={0}
                        max={10000}
                        value={transferFeeBps}
                        onChange={(event) => setTransferFeeBps(event.target.value)}
                      />
                    </label>
                    <label className="field">
                      滑点（bps）
                      <input
                        type="number"
                        min={0}
                        max={10000}
                        value={slippageBps}
                        onChange={(event) => setSlippageBps(event.target.value)}
                      />
                    </label>
                  </div>
                  <p className="scope-hint">费用为演示值，非费率建议</p>
                </div>
              )}
            </div>
            {formError && <p className="form-error">{formError}</p>}
            <button className="submit-task" disabled={submitting} type="submit">
              {submitting ? '提交中…' : '发起回测'}
            </button>
          </form>
        </section>
      </aside>
      <div className="result-column">
        <RunMonitor kind="backtest" status={status} pollingError={pollingError} />
        {!status && !pollingError && (
          <div className="result-empty">
            <svg className="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M3 3v18h18" />
              <path d="m7 14 4-4 4 3 5-6" />
            </svg>
            <strong>尚未发起回测</strong>
            <span>在左侧选择标的、策略与时间范围，点击「发起回测」后结果将在此展示</span>
          </div>
        )}
        {succeeded && (
          <>
            <section className="summary-cards" aria-label="回测摘要">
              <div className="summary-card"><span>总收益</span><strong>{percent(succeeded.summary.total_return)}</strong></div>
              <div className="summary-card"><span>年化收益</span><strong>{percent(succeeded.summary.annualized_return)}</strong></div>
              <div className="summary-card"><span>最大回撤</span><strong>{percent(succeeded.summary.maximum_drawdown)}</strong></div>
              <div className="summary-card"><span>胜率</span><strong>{percent(succeeded.summary.win_rate)}</strong></div>
              <div className="summary-card"><span>盈利因子</span><strong>{succeeded.summary.profit_factor === null ? '—' : succeeded.summary.profit_factor}</strong></div>
              <div className="summary-card"><span>平均持有</span><strong>{succeeded.summary.average_holding_bars} 根</strong></div>
              <div className="summary-card"><span>平仓笔数</span><strong>{succeeded.summary.closed_trades}</strong></div>
              <div className="summary-card"><span>期末持仓</span><strong>{succeeded.summary.has_open_position ? '有持仓' : '空仓'}</strong></div>
            </section>
            <EquityChart runId={succeeded.run_id} />
            <OrdersTradesTables runId={succeeded.run_id} />
          </>
        )}
      </div>
    </main>
  )
}
