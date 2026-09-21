import type { RunStatus } from '../../api/client'
import { PARAMETER_NAMES, strategyName } from '../strategy/strategyLabels'
import { EquityChart } from './EquityChart'
import { OrdersTradesTables } from './OrdersTradesTables'
import type { SubmittedBacktest } from './backtestPreferences'

const percent = (value: number | null) => value === null ? '—' : `${(value * 100).toFixed(2)}%`
export function BacktestReport({ run, context, active }: { run: RunStatus; context: SubmittedBacktest | null; active: boolean }) {
  const summary = run.summary!
  const d = context?.draft
  return <section className="backtest-report" aria-label="回测报告">
    <header className="backtest-report-heading"><div><span className="backtest-eyebrow">回测报告</span>
      <h2>{d ? `${d.instrumentName || d.instrument} · ` : ''}{strategyName(run.strategy)} <small>v{run.strategy_version}</small></h2>
      <p>{d ? `${d.instrument} · ${d.start} → ${d.end}` : '原始条件未保存，无法确认本报告的证券和日期范围。'}</p>
    </div><span className="backtest-completed">已完成</span></header>
    <details className="backtest-run-context"><summary>查看运行条件</summary>
      {d && context && <>
        <p>本机提交记录 · 初始资金 {Number(d.initialCash).toLocaleString('zh-CN')} 元 · 资金使用 {d.cashPercent}% · 持有期 {context.effectiveHoldBars} 根 · 每手 {d.lotSize} 股</p>
        <p>佣金 {d.commissionPercent}% · 最低 {d.minimumCommission} 元 · 印花税 {d.stampDutyPercent}% · 过户费 {d.transferFeePercent}% · 滑点 {d.slippagePercent}%</p>
        <p>策略参数：{Object.entries(context.effectiveParameters).map(([key, value]) => `${PARAMETER_NAMES[key] ?? key} ${value}`).join(' · ') || '无可调参数'}</p>
      </>}
      <p>任务 {run.run_id} · 数据版本 {run.data_version} · 引擎 {run.engine_version}</p>
    </details>
    <div className="backtest-metrics" aria-label="回测摘要">
      <div className="backtest-metric"><span>总收益率</span><strong className={summary.total_return === null || summary.total_return === 0 ? '' : summary.total_return > 0 ? 'positive' : 'negative'}>{percent(summary.total_return)}</strong></div>
      <div className="backtest-metric"><span>最大回撤</span><strong>{percent(summary.maximum_drawdown)}</strong></div>
      <div className="backtest-metric"><span>年化收益率</span><strong>{percent(summary.annualized_return)}</strong>{summary.annualized_return === null && <small>当前区间无法计算年化</small>}</div>
      <div className="backtest-metric"><span>平仓笔数</span><strong>{summary.closed_trades}</strong></div>
    </div>
    <EquityChart runId={run.run_id} active={active} />
    <dl className="backtest-secondary-metrics">
      <div><dt>胜率</dt><dd>{percent(summary.win_rate)}</dd>{summary.win_rate === null && <small>尚无可统计的平仓交易</small>}</div>
      <div><dt>盈利因子</dt><dd>{summary.profit_factor ?? '—'}</dd>{summary.profit_factor === null && <small>无亏损交易或无平仓交易，无法计算</small>}</div>
      <div><dt>平均持有</dt><dd>{summary.closed_trades ? `${summary.average_holding_bars} 根` : '—'}</dd></div>
      <div><dt>期末持仓</dt><dd>{summary.has_open_position ? '有持仓' : '空仓'}</dd>{summary.has_open_position && <small>期末持仓按最后收盘价计入权益</small>}</div>
    </dl>
    {summary.closed_trades === 0 && <p className="backtest-notice">本次没有已平仓交易；是否产生买入成交可查看下方成交记录。</p>}
    <OrdersTradesTables runId={run.run_id} active={active} />
  </section>
}
