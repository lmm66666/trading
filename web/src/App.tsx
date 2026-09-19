import { useState } from 'react'
import type { InstrumentSummary, PriceView, Timeframe } from './api/client'
import { BacktestPanel } from './features/backtest/BacktestPanel'
import { ChartWorkspace } from './features/chart/ChartWorkspace'
import { readWorkbenchState, writeWorkbenchState, type WorkbenchView } from './features/chart/chartData'
import { ScanPanel } from './features/scan/ScanPanel'
import { InstrumentSearch } from './features/search/InstrumentSearch'

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
  const [symbol, setSymbol] = useState(initial.symbol)
  const [timeframe, setTimeframe] = useState<Timeframe>(initial.timeframe)
  const [priceView, setPriceView] = useState<PriceView>(initial.priceView)
  const [view, setView] = useState<WorkbenchView>(initial.view)
  const [instrumentInfo, setInstrumentInfo] = useState<InstrumentSummary | null>(null)
  const [scanRunId, setScanRunId] = useState<string | null>(() => readStoredRunId('scan'))
  const [backtestRunId, setBacktestRunId] = useState<string | null>(() => readStoredRunId('backtest'))
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false)
  const [watchlistOpen, setWatchlistOpen] = useState(false)

  const syncState = (
    nextSymbol: string | null,
    nextTimeframe: Timeframe,
    nextPriceView: PriceView,
    nextView: WorkbenchView,
  ) => {
    writeWorkbenchState({ symbol: nextSymbol, timeframe: nextTimeframe, priceView: nextPriceView, view: nextView })
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

  return (
    <div className="app-shell">
      <header className="topbar">
        <button className="mobile-menu-trigger" aria-label="打开自选清单" onClick={() => setWatchlistOpen(true)} type="button">☰</button>
        <div className="brand"><span aria-hidden="true">◆</span>Trading</div>
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
          <button className="mobile-close" aria-label="关闭股票搜索" onClick={() => setMobileSearchOpen(false)} type="button">×</button>
          <InstrumentSearch onSelect={selectInstrument} />
        </div>
        <button className="mobile-search-trigger" aria-label="打开股票搜索" onClick={() => setMobileSearchOpen(true)} type="button">⌕</button>
      </header>
      {watchlistOpen && <button className="sidebar-scrim" aria-label="关闭自选清单" onClick={() => setWatchlistOpen(false)} type="button" />}
      <aside className={watchlistOpen ? 'watchlist-aside open' : 'watchlist-aside'} />
      <main className="workspace">
        {view === 'chart' ? (
          symbol ? (
            <ChartWorkspace
              instrument={symbol}
              initialPriceView={priceView}
              initialTimeframe={timeframe}
              key={symbol}
              onStateChange={changeChartState}
            />
          ) : (
            <section className="welcome-stage">
              <div className="welcome-grid" aria-hidden="true" />
              <div className="welcome-card">
                <span className="eyebrow">TRADING WORKBENCH</span>
                <h2>选择一只证券<br />开始观察市场</h2>
                <p>按 <kbd>/</kbd> 或点击右上角搜索，输入代码或名称打开图表。</p>
                <ul className="welcome-views">
                  <li><strong>图表</strong><span>K 线、成交量与技术指标</span></li>
                  <li><strong>扫描</strong><span>按策略筛选当前满足条件的证券</span></li>
                  <li><strong>回测</strong><span>用历史行情模拟策略表现</span></li>
                </ul>
              </div>
            </section>
          )
        ) : view === 'scan' ? (
          <ScanPanel runId={scanRunId} onRunIdChange={changeScanRunId} onSelectInstrument={(instrument) => selectSymbol(instrument, 'chart')} />
        ) : (
          <BacktestPanel
            runId={backtestRunId}
            onRunIdChange={changeBacktestRunId}
            selectedSymbol={symbol}
            defaultLotSize={instrumentInfo?.lot_size}
          />
        )}
      </main>
    </div>
  )
}
