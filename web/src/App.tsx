import { useCallback, useEffect, useState, type CSSProperties } from 'react'
import {
  addWatchlistItem,
  listWatchlist,
  removeWatchlistItem,
  type InstrumentSummary,
  type PriceView,
  type Timeframe,
  type WatchlistItem,
} from './api/client'
import { BacktestPanel } from './features/backtest/BacktestPanel'
import { ChartWorkspace } from './features/chart/ChartWorkspace'
import { readWorkbenchState, writeWorkbenchState, type WorkbenchView } from './features/chart/chartData'
import { ScanPanel } from './features/scan/ScanPanel'
import { InstrumentSearch } from './features/search/InstrumentSearch'
import { useBoards } from './features/chart/useBoards'
import { WatchlistPanel } from './features/watchlist/WatchlistPanel'

export type RunKindStore = 'scan' | 'backtest'

const RUN_ID_KEYS: Record<RunKindStore, string> = {
  scan: 'wb.scan_run_id',
  backtest: 'wb.backtest_run_id',
}

function readStoredRunId(kind: RunKindStore): string | null {
  try {
    return window.localStorage.getItem(RUN_ID_KEYS[kind])
  } catch {
    return null
  }
}

function storeRunId(kind: RunKindStore, runId: string | null): void {
  try {
    if (runId) window.localStorage.setItem(RUN_ID_KEYS[kind], runId)
    else window.localStorage.removeItem(RUN_ID_KEYS[kind])
  } catch {
    // 存储不可用时静默忽略，仅影响刷新恢复
  }
}

const VIEW_TABS: ReadonlyArray<{ key: WorkbenchView; label: string }> = [
  { key: 'chart', label: '图表' },
  { key: 'scan', label: '扫描' },
  { key: 'backtest', label: '回测' },
]

export default function App() {
  const initial = readWorkbenchState(window.location.search)
  const [symbol, setSymbol] = useState<string | null>(initial.symbol)
  const [timeframe, setTimeframe] = useState<Timeframe>(initial.timeframe)
  const [priceView, setPriceView] = useState<PriceView>(initial.priceView)
  const [view, setView] = useState<WorkbenchView>(initial.view)
  const [scanVisited, setScanVisited] = useState(initial.view === 'scan')
  useEffect(() => { if (view === 'scan') setScanVisited(true) }, [view])
  const [instrumentInfo, setInstrumentInfo] = useState<InstrumentSummary | null>(null)
  const [scanRunId, setScanRunId] = useState<string | null>(() => readStoredRunId('scan'))
  const [backtestRunId, setBacktestRunId] = useState<string | null>(() => readStoredRunId('backtest'))
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(260)
  const [watchlistOpen, setWatchlistOpen] = useState(false)
  const [watchlist, setWatchlist] = useState<{
    items: WatchlistItem[]
    status: 'loading' | 'ready' | 'error'
  }>({
    items: [],
    status: 'loading',
  })
  const [watchActionError, setWatchActionError] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    listWatchlist()
      .then((items) => {
        if (active) setWatchlist({ items, status: 'ready' })
      })
      .catch(() => {
        if (active) setWatchlist((current) => ({ items: current.items, status: 'error' }))
      })
    return () => {
      active = false
    }
  }, [])

  const refreshWatchlist = useCallback(() => {
    listWatchlist()
      .then((items) => {
        setWatchlist({ items, status: 'ready' })
        setWatchActionError(null)
      })
      .catch((reason: unknown) => {
        setWatchActionError(reason instanceof Error ? reason.message : '刷新自选失败')
        setWatchlist((current) => (current.items.length > 0 ? current : { items: [], status: 'error' }))
      })
  }, [])

  /** 在列表中则移除、不在则添加；成功以服务端返回的完整列表替换本地状态，失败保留原列表 */
  const toggleWatch = useCallback(
    (instrument: string) => {
      const exists = watchlist.items.some((item) => item.instrument === instrument)
      const request = exists ? removeWatchlistItem(instrument) : addWatchlistItem(instrument)
      request
        .then((items) => {
          setWatchlist({ items, status: 'ready' })
          setWatchActionError(null)
        })
        .catch((reason: unknown) => {
          setWatchActionError(reason instanceof Error ? reason.message : '更新自选失败')
        })
    },
    [watchlist.items],
  )

  const syncState = (
    nextSymbol: string | null,
    nextTimeframe: Timeframe,
    nextPriceView: PriceView,
    nextView: WorkbenchView,
  ) => {
    writeWorkbenchState({
      symbol: nextSymbol,
      timeframe: nextTimeframe,
      priceView: nextPriceView,
      view: nextView,
    })
  }

  const selectInstrument = (instrument: InstrumentSummary) => {
    selectSymbol(instrument.instrument, 'chart', instrument)
  }

  const changeBacktestRunId = (runId: string | null) => {
    setBacktestRunId(runId)
    storeRunId('backtest', runId)
  }

  const changeChartState = (nextTimeframe: Timeframe, nextPriceView: PriceView) => {
    setTimeframe(nextTimeframe)
    setPriceView(nextPriceView)
    syncState(symbol, nextTimeframe, nextPriceView, view)
  }

  const switchView = (nextView: WorkbenchView) => {
    setView(nextView)
    syncState(symbol, timeframe, priceView, nextView)
  }

  /** 从搜索、自选或扫描结果选中证券；始终切回指定视图并收起浮层 */
  const selectSymbol = (instrument: string, nextView: WorkbenchView, summary?: InstrumentSummary) => {
    setSymbol(instrument)
    setInstrumentInfo(summary ?? null)
    setMobileSearchOpen(false)
    setWatchlistOpen(false)
    setView(nextView)
    writeWorkbenchState({ symbol: instrument, timeframe, priceView, view: nextView })
  }

  const changeScanRunId = (runId: string | null) => {
    setScanRunId(runId)
    storeRunId('scan', runId)
  }

  // 看板控制器提升到 App：ChartWorkspace 仅在选中证券后挂载，若在内部加载看板，
  // URL 无股票时 GET 永不触发、看板默认股票无法恢复。
  const board = useBoards(
    { defaultSymbol: initial.symbol, timeframe: initial.timeframe, priceView: initial.priceView },
    (next) => selectSymbol(next, 'chart'),
  )

  return (
    <div
      className="app-shell"
      style={{ '--sidebar-width': `${sidebarCollapsed ? 0 : sidebarWidth}px` } as CSSProperties}
    >
      <header className="topbar">
        <button
          className="mobile-menu-trigger"
          aria-label="打开自选清单"
          onClick={() => setWatchlistOpen(true)}
          type="button"
        >
          ☰
        </button>
        <button
          className="sidebar-toggle"
          aria-label={sidebarCollapsed ? '展开自选' : '收起自选'}
          onClick={() => setSidebarCollapsed((v) => !v)}
          type="button"
        >
          ☰
        </button>
        <div className="brand">
          <span aria-hidden="true">◆</span>Trading
        </div>
        <nav className="view-tabs" aria-label="工作台视图">
          {VIEW_TABS.map((tab) => (
            <button
              key={tab.key}
              className={view === tab.key ? 'view-tab active' : 'view-tab'}
              aria-selected={view === tab.key}
              role="tab"
              onClick={() => switchView(tab.key)}
              type="button"
            >
              {tab.label}
            </button>
          ))}
        </nav>
        <div className={mobileSearchOpen ? 'search-overlay open' : 'search-overlay'}>
          <button
            className="mobile-close"
            aria-label="关闭股票搜索"
            onClick={() => setMobileSearchOpen(false)}
            type="button"
          >
            ×
          </button>
          <InstrumentSearch onSelect={selectInstrument} />
        </div>
        <button
          className="mobile-search-trigger"
          aria-label="打开股票搜索"
          onClick={() => setMobileSearchOpen(true)}
          type="button"
        >
          ⌕
        </button>
      </header>
      {watchlistOpen && (
        <button
          className="sidebar-scrim"
          aria-label="关闭自选清单"
          onClick={() => setWatchlistOpen(false)}
          type="button"
        />
      )}
      <aside
        className={`watchlist-aside${watchlistOpen ? ' open' : ''}${sidebarCollapsed ? ' collapsed' : ''}`}
      >
        <WatchlistPanel
          actionError={watchActionError}
          currentInstrument={symbol}
          items={watchlist.items}
          onAdd={() => {
            setMobileSearchOpen(true)
            document.querySelector<HTMLInputElement>('.topbar input[aria-label="搜索股票"]')?.focus()
          }}
          onRefresh={refreshWatchlist}
          onSelect={(item) => selectSymbol(item.instrument, 'chart', item)}
          onToggle={toggleWatch}
          status={watchlist.status}
        />
        <div
          className="sidebar-resizer"
          role="separator"
          aria-label="调整自选宽度"
          aria-orientation="vertical"
          tabIndex={0}
          aria-valuenow={sidebarWidth}
          aria-valuemin={220}
          aria-valuemax={360}
          onPointerDown={(e) => e.currentTarget.setPointerCapture(e.pointerId)}
          onPointerMove={(e) => {
            if (e.currentTarget.hasPointerCapture(e.pointerId))
              setSidebarWidth(Math.max(220, Math.min(360, e.clientX)))
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
              e.preventDefault()
              setSidebarWidth((w) => Math.max(220, Math.min(360, w + (e.key === 'ArrowRight' ? 10 : -10))))
            }
          }}
        />
      </aside>
      <main className="workspace">
        {board.status === 'error' && (
          <div className="board-error" role="alert">
            <span>看板加载失败：{board.error || '无法连接服务'}</span>
            <button onClick={board.reload} type="button">
              重试
            </button>
          </div>
        )}
        <div className="chart-view" hidden={view !== 'chart'}>
          {symbol && board.config ? (
            <ChartWorkspace
              board={board}
              instrument={symbol}
              onStateChange={changeChartState}
              onToggleWatch={() => toggleWatch(symbol)}
              watched={watchlist.items.some((item) => item.instrument === symbol)}
            />
          ) : (
            <section className="welcome-stage">
              <div className="welcome-grid" aria-hidden="true" />
              <div className="welcome-card">
                <span className="eyebrow">TRADING WORKBENCH</span>
                <h2>
                  选择一只证券
                  <br />
                  开始观察市场
                </h2>
                <p>
                  按 <kbd>/</kbd> 或点击右上角搜索，输入代码或名称打开图表。
                </p>
                <ul className="welcome-views">
                  <li>
                    <strong>图表</strong>
                    <span>K 线、成交量与技术指标</span>
                  </li>
                  <li>
                    <strong>扫描</strong>
                    <span>按策略筛选当前满足条件的证券</span>
                  </li>
                  <li>
                    <strong>回测</strong>
                    <span>用历史行情模拟策略表现</span>
                  </li>
                </ul>
              </div>
            </section>
          )}
        </div>
        {(scanVisited || view === 'scan') && (
          <ScanPanel
            active={view === 'scan'}
            runId={scanRunId}
            onRunIdChange={changeScanRunId}
            onSelectInstrument={(instrument) => selectSymbol(instrument, 'chart')}
          />
        )}
        {view === 'backtest' ? (
          <BacktestPanel
            runId={backtestRunId}
            onRunIdChange={changeBacktestRunId}
            selectedSymbol={symbol}
            defaultLotSize={instrumentInfo?.lot_size}
          />
        ) : null}
      </main>
    </div>
  )
}
