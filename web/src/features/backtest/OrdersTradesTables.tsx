import { useEffect, useRef, useState } from 'react'
import {
  fetchRunPage,
  fromScaled,
  type BacktestFill,
  type BacktestOrder,
  type RunResource,
} from '../../api/client'

const PAGE_LIMIT = 100

const SIDE_LABELS: Record<number, string> = { 1: '买入', 2: '卖出' }
const FINAL_REASON_LABELS: Record<number, string> = { 0: '待执行', 1: '已成交', 2: '无下一根Bar' }

function formatMoney(scaled: number): string {
  return fromScaled(scaled).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

function formatLocalTime(utc: string): string {
  return new Date(utc).toLocaleString('zh-CN', { hour12: false })
}

/** 同一 run 结果页的通用分页读取：首屏自动加载一页，之后按游标续读 */
function useRunResource<T>(runId: string, resource: RunResource) {
  const [items, setItems] = useState<T[]>([])
  const [nextSequence, setNextSequence] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const runIdRef = useRef(runId)
  runIdRef.current = runId

  useEffect(() => {
    let cancelled = false
    setItems([])
    setNextSequence(null)
    setError(null)
    setLoading(true)
    fetchRunPage<T>('backtest', runId, resource, undefined, PAGE_LIMIT)
      .then((page) => {
        if (cancelled) return
        setItems(page.items)
        setNextSequence(page.next_sequence ?? null)
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : '结果加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [runId, resource])

  const loadMore = async () => {
    if (nextSequence === null || loading) return
    setLoading(true)
    setError(null)
    try {
      const page = await fetchRunPage<T>('backtest', runId, resource, nextSequence, PAGE_LIMIT)
      // 续页响应返回时若任务已切换，丢弃以免拼入新任务结果
      if (runIdRef.current !== runId) return
      setItems((previous) => [...previous, ...page.items])
      setNextSequence(page.next_sequence ?? null)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '结果加载失败')
    } finally {
      setLoading(false)
    }
  }

  return { items, nextSequence, loading, error, loadMore }
}

interface ResourceTableProps<T> {
  title: string
  resource: RunResource
  runId: string
  head: string[]
  rowsOf: (items: T[]) => React.ReactNode
}

function ResourceTable<T>({ title, resource, runId, head, rowsOf }: ResourceTableProps<T>) {
  const { items, nextSequence, loading, error, loadMore } = useRunResource<T>(runId, resource)
  return (
    <details className="resource-table" open>
      <summary>{title} {items.length > 0 ? `（${items.length} 条）` : ''}</summary>
      {loading && items.length === 0 && <p className="results-hint">加载中…</p>}
      {error && <p className="results-error">{error}{items.length > 0 ? '（已保留已加载记录）' : ''}</p>}
      {items.length > 0 && (
        <table>
          <thead>
            <tr>{head.map((label) => <th key={label}>{label}</th>)}</tr>
          </thead>
          <tbody>{rowsOf(items)}</tbody>
        </table>
      )}
      {nextSequence !== null && (
        <button className="table-load-more" onClick={loadMore} disabled={loading} type="button">
          {loading ? '加载中…' : '加载更多'}
        </button>
      )}
    </details>
  )
}

interface OrdersTradesTablesProps {
  runId: string
}

/** 回测订单与成交列表：两个可折叠表格，各自按游标分页 */
export function OrdersTradesTables({ runId }: OrdersTradesTablesProps) {
  return (
    <section className="orders-trades" aria-label="订单与成交">
      <ResourceTable<BacktestOrder>
        title="订单"
        resource="orders"
        runId={runId}
        head={['ID', '证券', '方向', '数量', '创建时间', '原因', '尝试时间', '结果']}
        rowsOf={(items) => items.map((order) => (
          <tr key={order.id}>
            <td>{order.id}</td>
            <td>{order.instrument}</td>
            <td>{SIDE_LABELS[order.side] ?? order.side}</td>
            <td>{order.quantity}</td>
            <td>{formatLocalTime(order.created_at)}</td>
            <td>{order.reason}</td>
            <td>{formatLocalTime(order.attempted_at)}</td>
            <td>{FINAL_REASON_LABELS[order.final_reason] ?? order.final_reason}</td>
          </tr>
        ))}
      />
      <ResourceTable<BacktestFill>
        title="成交"
        resource="trades"
        runId={runId}
        head={['ID', '订单', '证券', '方向', '时间', '价格(元)', '数量', '成交额(元)', '佣金(元)', '印花税(元)', '过户费(元)']}
        rowsOf={(items) => items.map((fill) => (
          <tr key={fill.id}>
            <td>{fill.id}</td>
            <td>{fill.order_id}</td>
            <td>{fill.instrument}</td>
            <td>{SIDE_LABELS[fill.side] ?? fill.side}</td>
            <td>{formatLocalTime(fill.time)}</td>
            <td>{formatMoney(fill.price)}</td>
            <td>{fill.quantity}</td>
            <td>{formatMoney(fill.gross)}</td>
            <td>{formatMoney(fill.commission)}</td>
            <td>{formatMoney(fill.stamp_duty)}</td>
            <td>{formatMoney(fill.transfer_fee)}</td>
          </tr>
        ))}
      />
    </section>
  )
}
