import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { addWatchlistItem, listWatchlist, removeWatchlistItem, type WatchlistItem } from './api/client'

vi.mock('./api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/client')>()
  return {
    ...actual,
    addWatchlistItem: vi.fn(),
    listWatchlist: vi.fn(),
    removeWatchlistItem: vi.fn(),
  }
})
vi.mock('./features/search/InstrumentSearch', () => ({
  InstrumentSearch: ({ onSelect }: { onSelect: (item: unknown) => void }) => (
    <button onClick={() => onSelect({ instrument: 'SZSE:002415' })} type="button">选择海康威视</button>
  ),
}))
vi.mock('./features/chart/ChartWorkspace', () => ({
  ChartWorkspace: ({ instrument, onStateChange, onToggleWatch, watched }: {
    instrument: string
    onStateChange: (timeframe: string, view: string) => void
    onToggleWatch: () => void
    watched: boolean
  }) => (
    <div>
      <span>图表 {instrument}</span>
      <button aria-pressed={watched} onClick={onToggleWatch} type="button">星标</button>
      <button onClick={() => onStateChange('WEEK', 'RAW')} type="button">更新图表状态</button>
    </div>
  ),
}))
vi.mock('./features/watchlist/WatchlistPanel', () => ({
  WatchlistPanel: ({ items, status, actionError, currentInstrument, onSelect, onToggle, onRefresh }: {
    items: WatchlistItem[]
    status: string
    actionError: string | null
    currentInstrument: string | null
    onSelect: (item: WatchlistItem) => void
    onToggle: (instrument: string) => void
    onRefresh: () => void
  }) => (
    <div>
      <span>自选面板 {status} {items.length} {currentInstrument ?? '无'}</span>
      <span>自选错误 {actionError ?? '无'}</span>
      {items.map((item) => <span key={item.instrument}>自选行 {item.instrument}</span>)}
      <button onClick={onRefresh} type="button">刷新自选</button>
      <button onClick={() => items.length > 0 && onToggle(items[0].instrument)} type="button">面板移除</button>
      <button onClick={() => onSelect(items[0])} type="button">打开自选证券</button>
    </div>
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
vi.mock('./features/backtest/BacktestPanel', () => ({
  BacktestPanel: ({ runId, selectedSymbol, onRunIdChange }: {
    runId: string | null
    selectedSymbol: string | null
    onRunIdChange: (runId: string | null) => void
  }) => (
    <div>
      <span>回测面板 {selectedSymbol ?? '未选证券'} {runId ?? '无任务'}</span>
      <button onClick={() => onRunIdChange('bt-new')} type="button">记录回测任务</button>
    </div>
  ),
}))

const watched: WatchlistItem = {
  instrument: 'SSE:600000', code: '600000', name: '浦发银行', exchange: 'SSE', board: 'MAIN', lot_size: 100,
  close: 12.34, change: 0.15, change_pct: 1.23,
}

describe('App', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/')
    window.localStorage.clear()
    vi.mocked(listWatchlist).mockReset().mockResolvedValue([])
    vi.mocked(addWatchlistItem).mockReset()
    vi.mocked(removeWatchlistItem).mockReset()
  })

  it('从空状态选股并同步 URL', () => {
    render(<App />)
    expect(screen.getByText(/选择一只证券/)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '选择海康威视' }))
    expect(screen.getByText('图表 SZSE:002415')).toBeVisible()
    expect(window.location.search).toContain('symbol=SZSE%3A002415')
  })

  it('顶栏浮层搜索与自选抽屉开关可交互', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '打开股票搜索' }))
    expect(screen.getByRole('button', { name: '关闭股票搜索' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '关闭股票搜索' }))

    fireEvent.click(screen.getByRole('button', { name: '打开自选清单' }))
    expect(screen.getByRole('button', { name: '关闭自选清单' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '关闭自选清单' }))
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
    expect(screen.getByText(/选择一只证券/)).toBeVisible()
  })

  it('切换视图时保持选中证券并同步 tab', () => {
    window.history.replaceState(null, '', '/?symbol=SSE%3A600000')
    render(<App />)
    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: '回测' }))
    expect(screen.getByText(/回测面板 SSE:600000/)).toBeVisible()
    expect(window.location.search).toContain('symbol=SSE%3A600000')
    expect(window.location.search).toContain('tab=backtest')
    fireEvent.click(screen.getByRole('tab', { name: '图表' }))
    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
  })

  it('回测任务 run_id 写入 localStorage', () => {
    window.localStorage.clear()
    window.history.replaceState(null, '', '/?tab=backtest')
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '记录回测任务' }))
    expect(window.localStorage.getItem('wb.backtest_run_id')).toBe('bt-new')
    expect(screen.getByText(/bt-new/)).toBeVisible()
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

  it('挂载时拉取自选并渲染到面板', async () => {
    vi.mocked(listWatchlist).mockResolvedValue([watched])
    render(<App />)
    await waitFor(() => expect(screen.getByText('自选行 SSE:600000')).toBeVisible())
    expect(screen.getByText(/自选面板 ready 1 无/)).toBeVisible()
  })

  it('自选加载失败呈现错误态', async () => {
    vi.mocked(listWatchlist).mockRejectedValue(new Error('offline'))
    render(<App />)
    await waitFor(() => expect(screen.getByText(/自选面板 error/)).toBeVisible())
  })

  it('图表 ★ 添加自选后列表即时更新', async () => {
    window.history.replaceState(null, '', '/?symbol=SSE%3A600000')
    vi.mocked(addWatchlistItem).mockResolvedValue([watched])
    render(<App />)
    await waitFor(() => expect(screen.getByText(/自选面板 ready 0/)).toBeVisible())
    expect(screen.getByRole('button', { name: '星标' })).toHaveAttribute('aria-pressed', 'false')

    fireEvent.click(screen.getByRole('button', { name: '星标' }))

    await waitFor(() => expect(addWatchlistItem).toHaveBeenCalledWith('SSE:600000'))
    await waitFor(() => expect(screen.getByText('自选行 SSE:600000')).toBeVisible())
    expect(screen.getByRole('button', { name: '星标' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('已在自选时 ★ 与面板移除均调用移除接口', async () => {
    window.history.replaceState(null, '', '/?symbol=SSE:600000')
    const other: WatchlistItem = {
      instrument: 'SZSE:002415', code: '002415', name: '海康威视', exchange: 'SZSE', board: 'MAIN', lot_size: 100,
      close: null, change: null, change_pct: null,
    }
    vi.mocked(listWatchlist).mockResolvedValue([watched, other])
    vi.mocked(removeWatchlistItem).mockResolvedValue([other])
    render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: '星标' })).toHaveAttribute('aria-pressed', 'true'))

    fireEvent.click(screen.getByRole('button', { name: '星标' }))

    await waitFor(() => expect(removeWatchlistItem).toHaveBeenCalledWith('SSE:600000'))
    await waitFor(() => expect(screen.getByRole('button', { name: '星标' })).toHaveAttribute('aria-pressed', 'false'))

    fireEvent.click(screen.getByRole('button', { name: '面板移除' }))
    await waitFor(() => expect(removeWatchlistItem).toHaveBeenCalledWith('SZSE:002415'))
  })

  it('更新自选失败时保留原列表并提示', async () => {
    window.history.replaceState(null, '', '/?symbol=SSE:600000')
    vi.mocked(listWatchlist).mockResolvedValue([watched])
    vi.mocked(removeWatchlistItem).mockRejectedValue(new Error('自选服务不可用'))
    render(<App />)
    await waitFor(() => expect(screen.getByText('自选行 SSE:600000')).toBeVisible())

    fireEvent.click(screen.getByRole('button', { name: '面板移除' }))

    await waitFor(() => expect(screen.getByText('自选错误 自选服务不可用')).toBeVisible())
    expect(screen.getByText('自选行 SSE:600000')).toBeVisible()
    expect(screen.getByText(/自选面板 ready 1/)).toBeVisible()
  })

  it('点击自选行切回图表视图', async () => {
    vi.mocked(listWatchlist).mockResolvedValue([watched])
    window.history.replaceState(null, '', '/?tab=scan')
    render(<App />)
    await waitFor(() => expect(screen.getByText('自选行 SSE:600000')).toBeVisible())

    fireEvent.click(screen.getByRole('button', { name: '打开自选证券' }))

    expect(screen.getByText('图表 SSE:600000')).toBeVisible()
    expect(window.location.search).toContain('symbol=SSE%3A600000')
    expect(window.location.search).toContain('tab=chart')
  })

  it('刷新自选重新拉取列表', async () => {
    vi.mocked(listWatchlist).mockResolvedValue([])
    render(<App />)
    await waitFor(() => expect(screen.getByText(/自选面板 ready 0/)).toBeVisible())

    vi.mocked(listWatchlist).mockResolvedValue([watched])
    fireEvent.click(screen.getByRole('button', { name: '刷新自选' }))

    await waitFor(() => expect(screen.getByText('自选行 SSE:600000')).toBeVisible())
    expect(listWatchlist).toHaveBeenCalledTimes(2)
  })
})
