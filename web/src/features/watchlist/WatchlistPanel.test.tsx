import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { WatchlistItem } from '../../api/client'
import { WatchlistPanel } from './WatchlistPanel'

const items: WatchlistItem[] = [
  {
    instrument: 'SSE:600000',
    code: '600000',
    name: '浦发银行',
    exchange: 'SSE',
    board: 'MAIN',
    lot_size: 100,
    close: 12.34,
    change: 0.15,
    change_pct: 1.23,
  },
  {
    instrument: 'SZSE:002415',
    code: '002415',
    name: '海康威视',
    exchange: 'SZSE',
    board: 'MAIN',
    lot_size: 100,
    close: null,
    change: null,
    change_pct: null,
  },
  {
    instrument: 'SZSE:000001',
    code: '000001',
    name: '平安银行',
    exchange: 'SZSE',
    board: 'MAIN',
    lot_size: 100,
    close: 10.5,
    change: -0.2,
    change_pct: -1.87,
  },
]

describe('WatchlistPanel', () => {
  it('渲染自选行、报价着色、当前证券高亮与数量徽标', () => {
    render(
      <WatchlistPanel
        actionError={null}
        currentInstrument="SSE:600000"
        items={items}
        onSelect={vi.fn()}
        onRefresh={vi.fn()}
        onToggle={vi.fn()}
        status="ready"
      />,
    )

    expect(screen.getByText('浦发银行')).toBeVisible()
    expect(screen.getByText('12.34')).toBeVisible()
    expect(screen.getByText('+1.23%')).toHaveClass('up')
    expect(screen.getByText('-1.87%')).toHaveClass('down')
    expect(screen.getAllByText('—')).toHaveLength(2)
    expect(screen.getByText('3')).toBeVisible()
    expect(screen.getByText('浦发银行').closest('li')).toHaveClass('current')
    expect(screen.getByText('平安银行').closest('li')).not.toHaveClass('current')
  })

  it('点击行回调选中证券，悬停移除与刷新各自回调', () => {
    const onSelect = vi.fn()
    const onToggle = vi.fn()
    const onRefresh = vi.fn()
    render(
      <WatchlistPanel
        actionError={null}
        currentInstrument={null}
        items={items}
        onSelect={onSelect}
        onRefresh={onRefresh}
        onToggle={onToggle}
        status="ready"
      />,
    )

    fireEvent.click(screen.getByText('浦发银行'))
    expect(onSelect).toHaveBeenCalledWith(items[0])
    fireEvent.click(screen.getByRole('button', { name: '移除自选 海康威视' }))
    expect(onToggle).toHaveBeenCalledWith('SZSE:002415')
    fireEvent.click(screen.getByRole('button', { name: '刷新自选' }))
    expect(onRefresh).toHaveBeenCalled()
  })

  it('加载中显示骨架行', () => {
    render(
      <WatchlistPanel
        actionError={null}
        currentInstrument={null}
        items={[]}
        onSelect={vi.fn()}
        onRefresh={vi.fn()}
        onToggle={vi.fn()}
        status="loading"
      />,
    )
    expect(screen.getByLabelText('正在加载自选')).toBeVisible()
  })

  it('加载失败显示错误态并可重试', () => {
    const onRefresh = vi.fn()
    render(
      <WatchlistPanel
        actionError={null}
        currentInstrument={null}
        items={[]}
        onSelect={vi.fn()}
        onRefresh={onRefresh}
        onToggle={vi.fn()}
        status="error"
      />,
    )
    expect(screen.getByText('自选加载失败，请确认后端服务可用。')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(onRefresh).toHaveBeenCalled()
  })

  it('空列表显示引导文案', () => {
    render(
      <WatchlistPanel
        actionError={null}
        currentInstrument={null}
        items={[]}
        onSelect={vi.fn()}
        onRefresh={vi.fn()}
        onToggle={vi.fn()}
        status="ready"
      />,
    )
    expect(screen.getByText('搜索证券后点击图表头部 ★ 添加自选。')).toBeVisible()
  })

  it('变更失败时显示提示', () => {
    render(
      <WatchlistPanel
        actionError="自选已满（上限 100 只）"
        currentInstrument={null}
        items={items}
        onSelect={vi.fn()}
        onRefresh={vi.fn()}
        onToggle={vi.fn()}
        status="ready"
      />,
    )
    expect(screen.getByText('自选已满（上限 100 只）')).toBeVisible()
  })
})

it('filters by name/code and sorts only on explicit action', () => {
  const props = {
    items,
    status: 'ready' as const,
    actionError: null,
    currentInstrument: null,
    onSelect: vi.fn(),
    onToggle: vi.fn(),
    onRefresh: vi.fn(),
  }
  const { rerender } = render(<WatchlistPanel {...props} />)
  fireEvent.change(screen.getByRole('textbox', { name: '筛选自选' }), { target: { value: '银行' } })
  expect(screen.queryByText('海康威视')).not.toBeInTheDocument()
  fireEvent.change(screen.getByRole('textbox', { name: '筛选自选' }), { target: { value: '' } })
  fireEvent.click(screen.getByRole('button', { name: '按涨跌幅排序' }))
  expect(screen.getAllByRole('listitem')[0]).toHaveTextContent('浦发银行')
  rerender(
    <WatchlistPanel
      {...props}
      items={items.map((i) => ({ ...i, change_pct: i.instrument === 'SZSE:000001' ? 50 : i.change_pct }))}
    />,
  )
  expect(screen.getAllByRole('listitem')[0]).toHaveTextContent('浦发银行')
})
