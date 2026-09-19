import { useState } from 'react'
import type { InstrumentSummary, PriceView, Timeframe } from './api/client'
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
  const [scanRunId, setScanRunId] = useState<string | null>(() => readStoredRunId('scan'))
  const [backtestRunId, setBacktestRunId] = useState<string | null>(() => readStoredRunId('backtest'))
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false)

  const syncState = (
    nextSymbol: string | null,
    nextTimeframe: Timeframe,
    nextPriceView: PriceView,
    nextView: WorkbenchView,
  ) => {
    writeWorkbenchState({ symbol: nextSymbol, timeframe: nextTimeframe, priceView: nextPriceView, view: nextView })
  }

  const selectInstrument = (instrument: InstrumentSummary) => {
    selectSymbol(instrument.instrument, view)
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

  /** 从扫描结果或证券搜索选中证券；扫描结果行点击时切回图表视图 */
  const selectSymbol = (instrument: string, nextView: WorkbenchView) => {
    setSymbol(instrument)
    setMobileSearchOpen(false)
    setView(nextView)
    writeWorkbenchState({ symbol: instrument, timeframe, priceView, view: nextView })
  }

  const changeScanRunId = (runId: string | null) => {
    setScanRunId(runId)
    storeRunId('scan', runId)
  }

  return (
    <div className="app-shell">
      <aside className={mobileSearchOpen ? 'sidebar open' : 'sidebar'}>
        <button className="mobile-close" aria-label="关闭股票搜索" onClick={() => setMobileSearchOpen(false)} type="button">×</button>
        <InstrumentSearch onSelect={selectInstrument} />
      </aside>
      <div className="workspace-column">
        <button className="mobile-search-trigger" onClick={() => setMobileSearchOpen(true)} type="button">
          <span aria-hidden="true">⌕</span> 搜索股票
        </button>
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
            <main className="welcome-stage">
              <div className="welcome-grid" aria-hidden="true" />
              <div className="welcome-card">
                <div className="welcome-symbol"><span /><span /><span /><span /><span /></div>
                <span className="eyebrow">TRADING WORKBENCH</span>
                <h2>选择一只股票<br />开始观察市场</h2>
                <p>搜索证券后，这里将展示 K 线、成交量、均线与独立副图指标。</p>
                <button onClick={() => setMobileSearchOpen(true)} type="button">搜索股票 <span>→</span></button>
              </div>
              <footer className="welcome-footer">
                <span><i className="legend-red" />上涨</span>
                <span><i className="legend-green" />下跌</span>
                <span>日线 / 周线</span>
                <span>RAW / QFQ</span>
              </footer>
            </main>
          )
        ) : view === 'scan' ? (
          <ScanPanel runId={scanRunId} onRunIdChange={changeScanRunId} onSelectInstrument={(instrument) => selectSymbol(instrument, 'chart')} />
        ) : (
          <main className="panel-placeholder">
            回测功能建设中
            {backtestRunId ? <span className="placeholder-run-id">{backtestRunId}</span> : null}
          </main>
        )}
      </div>
      {mobileSearchOpen && <button className="sidebar-scrim" aria-label="关闭股票搜索" onClick={() => setMobileSearchOpen(false)} type="button" />}
    </div>
  )
}
