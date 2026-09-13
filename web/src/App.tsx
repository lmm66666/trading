import { useState } from 'react'
import type { InstrumentSummary, PriceView, Timeframe } from './api/client'
import { ChartWorkspace } from './features/chart/ChartWorkspace'
import { readWorkbenchState, writeWorkbenchState } from './features/chart/chartData'
import { InstrumentSearch } from './features/search/InstrumentSearch'

export default function App() {
  const initial = readWorkbenchState(window.location.search)
  const [symbol, setSymbol] = useState(initial.symbol)
  const [timeframe, setTimeframe] = useState<Timeframe>(initial.timeframe)
  const [priceView, setPriceView] = useState<PriceView>(initial.priceView)
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false)

  const syncState = (nextSymbol: string | null, nextTimeframe: Timeframe, nextPriceView: PriceView) => {
    writeWorkbenchState({ symbol: nextSymbol, timeframe: nextTimeframe, priceView: nextPriceView })
  }

  const selectInstrument = (instrument: InstrumentSummary) => {
    setSymbol(instrument.instrument)
    setMobileSearchOpen(false)
    syncState(instrument.instrument, timeframe, priceView)
  }

  const changeChartState = (nextTimeframe: Timeframe, nextPriceView: PriceView) => {
    setTimeframe(nextTimeframe)
    setPriceView(nextPriceView)
    syncState(symbol, nextTimeframe, nextPriceView)
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
        {symbol ? (
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
        )}
      </div>
      {mobileSearchOpen && <button className="sidebar-scrim" aria-label="关闭股票搜索" onClick={() => setMobileSearchOpen(false)} type="button" />}
    </div>
  )
}
