import { useEffect, useRef, useState, type ReactNode } from 'react'
import { fetchRunPage, fromScaled, type BacktestFill, type BacktestOrder } from '../../api/client'

const PAGE_LIMIT = 100
const SIDE_LABELS: Record<number, string> = { 1: '买入', 2: '卖出' }
// 对应 internal/backtest/result.go 的稳定 OrderFinalReason 枚举。
const FINAL_REASON_LABELS: Record<number, string> = {
  0: '待执行', 1: '已成交', 2: '无下一根 K 线', 3: '执行配置无效', 4: '订单无效',
  5: '订单尚未生效', 6: '当前不可交易', 7: '涨停无法买入', 8: '跌停无法卖出', 9: '每手数量无效',
  10: '可用资金不足', 11: '可用持仓不足', 12: '重复成交', 13: '数值计算溢出', 14: '行情数据无效',
  15: '费用超过卖出所得', 16: '账户数值溢出', 17: '账户状态转换无效',
}
const money = (scaled: number) => fromScaled(scaled).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const time = (utc: string) => new Date(utc).toLocaleString('zh-CN', { hour12: false })
type Resource = 'orders' | 'trades'

function ResourceTable<T>({ runId, resource, active, head, rowsOf }: { runId: string; resource: Resource; active: boolean; head: string[]; rowsOf: (items: T[]) => ReactNode }) {
  const [items, setItems] = useState<T[]>([])
  const [next, setNext] = useState<number | undefined>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [request, setRequest] = useState<{ after?: number; attempt: number }>({ attempt: 0 })
  const completed = useRef(-1)
  useEffect(() => {
    if (!active || completed.current === request.attempt) return
    let current = true
    setLoading(true)
    setError(null)
    void fetchRunPage<T>('backtest', runId, resource, request.after, PAGE_LIMIT)
      .then((page) => {
        if (!current) return
        setItems((old) => request.after === undefined ? page.items : [...old, ...page.items])
        setNext(page.next_sequence)
      })
      .catch((cause: unknown) => { if (current) setError(cause instanceof Error ? cause.message : '记录加载失败') })
      .finally(() => { if (current) { completed.current = request.attempt; setLoading(false) } })
    return () => { current = false }
  }, [runId, resource, active, request])
  return <div role="tabpanel" aria-label={resource === 'trades' ? '成交记录' : '订单记录'} hidden={!active}>
    {items.length > 0 && <p className="results-hint">{next === undefined ? `共 ${items.length} 条` : `已加载 ${items.length} 条`}</p>}
    {loading && !items.length && <p className="results-hint" role="status">记录加载中…</p>}
    {error && <p className="results-error" role="alert">{error}{items.length > 0 ? '（已保留已加载记录）' : ''} <button type="button" className="table-load-more" onClick={() => setRequest((old) => ({ ...old, attempt: old.attempt + 1 }))}>重试加载记录</button></p>}
    {!loading && !error && !items.length && <p className="backtest-table-empty">本次回测没有{resource === 'trades' ? '成交' : '订单'}记录</p>}
    {items.length > 0 && <div className="backtest-table-scroll"><table><thead><tr>{head.map((label) => <th key={label}>{label}</th>)}</tr></thead><tbody>{rowsOf(items)}</tbody></table></div>}
    {next !== undefined && !error && <button type="button" className="table-load-more" disabled={loading} onClick={() => { if (!loading) setRequest((old) => ({ after: next, attempt: old.attempt + 1 })) }}>{loading ? '加载中…' : '加载更多'}</button>}
  </div>
}
interface OrdersTradesTablesProps { runId: string; active?: boolean }
export function OrdersTradesTables(props: OrdersTradesTablesProps) { return <TablesContent key={props.runId} {...props} /> }
function TablesContent({ runId, active = true }: OrdersTradesTablesProps) {
  const [tab, setTab] = useState<Resource>('trades')
  const [ordersVisited, setOrdersVisited] = useState(false)
  return <section className="orders-trades" aria-label="订单与成交">
    <div className="backtest-table-tabs" role="tablist" aria-label="回测记录">
      <button type="button" role="tab" aria-selected={tab === 'trades'} onClick={() => setTab('trades')}>成交记录</button>
      <button type="button" role="tab" aria-selected={tab === 'orders'} onClick={() => { setTab('orders'); setOrdersVisited(true) }}>订单记录</button>
    </div>
    <ResourceTable<BacktestFill> runId={runId} resource="trades" active={active && tab === 'trades'} head={['成交时间', '方向', '价格（元）', '数量', '成交额（元）', '详情']} rowsOf={(items) => items.map((fill) => <tr key={fill.id}>
      <td>{time(fill.time)}</td><td>{SIDE_LABELS[fill.side] ?? fill.side}</td><td>{fromScaled(fill.price).toLocaleString('zh-CN', { maximumFractionDigits: 4 })}</td><td>{fill.quantity}</td><td>{money(fill.gross)}</td>
      <td><details><summary>查看详情</summary><div className="backtest-row-details">证券 {fill.instrument}<br />成交 ID {fill.id}<br />订单 ID {fill.order_id}<br />佣金 {money(fill.commission)} 元<br />印花税 {money(fill.stamp_duty)} 元<br />过户费 {money(fill.transfer_fee)} 元</div></details></td>
    </tr>)} />
    {ordersVisited && <ResourceTable<BacktestOrder> runId={runId} resource="orders" active={active && tab === 'orders'} head={['信号时间', '方向', '数量', '执行结果', '原因', '详情']} rowsOf={(items) => items.map((order) => <tr key={order.id}>
      <td>{time(order.created_at)}</td><td>{SIDE_LABELS[order.side] ?? order.side}</td><td>{order.quantity}</td><td>{FINAL_REASON_LABELS[order.final_reason] ?? `未知结果（${order.final_reason}）`}</td><td>{order.reason}</td>
      <td><details><summary>查看详情</summary><div className="backtest-row-details">证券 {order.instrument}<br />订单 ID {order.id}<br />尝试时间 {time(order.attempted_at)}</div></details></td>
    </tr>)} />}
  </section>
}
