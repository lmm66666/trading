import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

vi.mock('./features/search/InstrumentSearch', () => ({
  InstrumentSearch: ({ onSelect }: { onSelect: (item: unknown) => void }) => (
    <button onClick={() => onSelect({ instrument: 'SZSE:002415' })} type="button">选择海康威视</button>
  ),
}))
vi.mock('./features/chart/ChartWorkspace', () => ({
  ChartWorkspace: ({ instrument, onStateChange }: { instrument: string; onStateChange: (timeframe: string, view: string) => void }) => (
    <div><span>图表 {instrument}</span><button onClick={() => onStateChange('WEEK', 'RAW')} type="button">更新图表状态</button></div>
  ),
}))
vi.mock('./features/scan/ScanPanel', () => ({
  ScanPanel: ({ runId, onRunIdChange, onSelectInstrument }: {
    runId: string | null
    onRunIdChange: (runId: string | null) => void
    onSelectInstrument: (instrument: string) => void
  }) => (
    <div>
      <span>扫描面板 {runId ?? '无任务'}</span>
      <button onClick={() => onRunIdChange('r-new')} type="button">记录新任务</button>
      <button onClick={() => onSelectInstrument('SSE:600000')} type="button">打开入选证券</button>
    </div>
  ),
}))

describe('App', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/')
    window.localStorage.clear()
  })

  it('从空状态选股并同步 URL', () => {
    render(<App />)
    expect(screen.getByText(/选择一只股票/)).toBeVisible()
    const searchButtons = screen.getAllByRole('button', { name: '搜索股票' })
    fireEvent.click(searchButtons.at(-1)!)
    const closeButtons = screen.getAllByRole('button', { name: '关闭股票搜索' })
    fireEvent.click(closeButtons.at(-1)!)
    fireEvent.click(screen.getByRole('button', { name: '选择海康威视' }))
    expect(screen.getByText('图表 SZSE:002415')).toBeVisible()
    expect(window.location.search).toContain('symbol=SZSE%3A002415')
  })

  it('恢复 URL 状态并接收图表状态变更', () => {
    window.history.replaceState(null, '', '/?symbol=SSE%3A600000&timeframe=DAY&view=QFQ')
    render(<App />)
    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '更新图表状态' }))
    expect(window.location.search).toContain('timeframe=WEEK')
    expect(window.location.search).toContain('view=RAW')
  })

  it('按 tab 参数切换视图，非法值回落图表', () => {
    window.history.replaceState(null, '', '/?tab=scan')
    const { unmount } = render(<App />)
    expect(screen.getByText('扫描面板 无任务')).toBeVisible()
    unmount()

    window.history.replaceState(null, '', '/?tab=OPTIMIZER')
    render(<App />)
    expect(screen.getByText(/选择一只股票/)).toBeVisible()
  })

  it('切换视图时保持选中证券并同步 tab', () => {
    window.history.replaceState(null, '', '/?symbol=SSE%3A600000')
    render(<App />)
    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: '回测' }))
    expect(screen.getByText('回测功能建设中')).toBeVisible()
    expect(window.location.search).toContain('symbol=SSE%3A600000')
    expect(window.location.search).toContain('tab=backtest')
    fireEvent.click(screen.getByRole('tab', { name: '图表' }))
    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
  })

  it('从 localStorage 恢复最近任务标识', () => {
    window.localStorage.setItem('wb.scan_run_id', 'scan-run-1')
    window.history.replaceState(null, '', '/?tab=scan')
    render(<App />)
    expect(screen.getByText('扫描面板 scan-run-1')).toBeVisible()
  })

  it('新任务 run_id 写入 localStorage，点击入选证券切回图表', () => {
    window.history.replaceState(null, '', '/?tab=scan')
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '记录新任务' }))
    expect(window.localStorage.getItem('wb.scan_run_id')).toBe('r-new')
    expect(screen.getByText('扫描面板 r-new')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '打开入选证券' }))
    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
    expect(window.location.search).toContain('symbol=SSE%3A600000')
    expect(window.location.search).not.toContain('tab=scan')
  })
})
