import { useState } from 'react'
import type { WatchlistItem } from '../../api/client'

interface WatchlistPanelProps {
  items: WatchlistItem[]
  status: 'loading' | 'ready' | 'error'
  /** 变更（添加/移除/刷新）失败的提示；加载失败由 status 表达 */
  actionError: string | null
  currentInstrument: string | null
  onSelect: (item: WatchlistItem) => void
  /** 在列表中则移除；与图表头部 ★ 共用同一入口 */
  onToggle: (instrument: string) => void
  onRefresh: () => void
  onAdd?: () => void
}

/** 左侧自选清单：报价红涨绿跌、当前证券高亮、悬停移除 */
export function WatchlistPanel({
  items,
  status,
  actionError,
  currentInstrument,
  onSelect,
  onToggle,
  onRefresh,
  onAdd,
}: WatchlistPanelProps) {
  const [filter, setFilter] = useState('')
  const [order, setOrder] = useState<string[]>([])
  const [descending, setDescending] = useState(true)
  const visible = items
    .filter((i) => `${i.name} ${i.code}`.includes(filter.trim()))
    .slice()
    .sort((a, b) => {
      if (!order.length) return 0
      const left = order.indexOf(a.instrument),
        right = order.indexOf(b.instrument)
      return (left < 0 ? Infinity : left) - (right < 0 ? Infinity : right)
    })
  const sort = () => {
    setOrder(
      [...items]
        .sort((a, b) => {
          if (a.change_pct === null) return b.change_pct === null ? 0 : 1
          if (b.change_pct === null) return -1
          return descending ? b.change_pct - a.change_pct : a.change_pct - b.change_pct
        })
        .map((i) => i.instrument),
    )
    setDescending(!descending)
  }
  return (
    <section className="watchlist-panel" aria-label="自选清单">
      <header className="watchlist-header">
        <h2>自选</h2>
        <span className="watchlist-count">{items.length}</span>
        {onAdd && (
          <button className="icon-button" aria-label="添加自选证券" onClick={onAdd} type="button">
            ＋
          </button>
        )}
        <button
          aria-label="刷新报价"
          className="watchlist-refresh"
          disabled={status === 'loading'}
          onClick={onRefresh}
          type="button"
        >
          ⟳
        </button>
      </header>
      <div className="watchlist-filter">
        <input
          aria-label="筛选自选"
          placeholder="筛选名称 / 代码"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>
      <div className="watchlist-columns">
        <button onClick={() => setOrder([])} type="button">
          名称 / 默认顺序
        </button>
        <button aria-label="按涨跌幅排序" onClick={sort} type="button">
          涨跌幅 {order.length ? (descending ? '↑' : '↓') : '↕'}
        </button>
      </div>
      {status === 'ready' && items.length > 0 && visible.length === 0 && (
        <p className="watchlist-empty">没有匹配的自选证券</p>
      )}
      {actionError && status === 'ready' && (
        <p className="watchlist-notice" role="alert">
          {actionError}
        </p>
      )}
      {status === 'loading' && (
        <div aria-label="正在加载自选" aria-live="polite" className="watchlist-skeleton">
          <span />
          <span />
          <span />
          <span />
        </div>
      )}
      {status === 'error' && (
        <div className="watchlist-error">
          <p>自选加载失败，请确认后端服务可用。</p>
          <button className="watchlist-retry" onClick={onRefresh} type="button">
            重试
          </button>
        </div>
      )}
      {status === 'ready' && items.length === 0 && (
        <p className="watchlist-empty">搜索证券后点击图表头部 ★ 添加自选。</p>
      )}
      {status === 'ready' && items.length > 0 && (
        <ul className="watchlist-list">
          {visible.map((item) => {
            const pct = item.change_pct
            return (
              <li
                className={item.instrument === currentInstrument ? 'watchlist-row current' : 'watchlist-row'}
                key={item.instrument}
              >
                <button
                  aria-current={item.instrument === currentInstrument ? true : undefined}
                  className="watchlist-row-main"
                  onClick={() => onSelect(item)}
                  type="button"
                >
                  <span className="watchlist-names">
                    <strong>{item.name}</strong>
                    <small>
                      {item.code} · {item.exchange}
                    </small>
                  </span>
                  <span className="watchlist-quote">
                    <span className="watchlist-price">
                      {item.close === null ? '—' : item.close.toFixed(2)}
                    </span>
                    <span
                      className={
                        pct === null
                          ? 'watchlist-change'
                          : pct >= 0
                            ? 'watchlist-change up'
                            : 'watchlist-change down'
                      }
                    >
                      {pct === null ? '—' : `${pct >= 0 ? '+' : ''}${pct.toFixed(2)}%`}
                    </span>
                  </span>
                </button>
                <button
                  aria-label={`移除自选 ${item.name}`}
                  className="watchlist-remove"
                  onClick={() => onToggle(item.instrument)}
                  type="button"
                >
                  ×
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
