import { fireEvent, render, screen, within } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { RangePicker } from './RangePicker'

function Harness({ onChange }: { onChange: (from: string, to: string) => void }) {
  const [range, setRange] = useState({ from: '', to: '' })
  return (
    <div>
      <RangePicker
        ariaLabel="时间范围"
        from={range.from}
        to={range.to}
        onChange={(from, to) => {
          setRange({ from, to })
          onChange(from, to)
        }}
      />
      <button type="button">外部按钮</button>
    </div>
  )
}

const fmt = (date: Date) =>
  `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`

function today(): Date {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return d
}

function monthsAgo(base: Date, months: number): Date {
  const first = new Date(base.getFullYear(), base.getMonth(), 1)
  first.setMonth(first.getMonth() - months)
  const daysIn = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()
  first.setDate(Math.min(base.getDate(), daysIn))
  return first
}

/** 组件默认视图：左 = 上一月，右 = 当月 */
function defaultMonths() {
  const base = today()
  const left = new Date(base.getFullYear(), base.getMonth() - 1, 1)
  const right = new Date(base.getFullYear(), base.getMonth(), 1)
  return { left, right }
}

const monthLabel = (d: Date) => `${d.getFullYear()} 年 ${d.getMonth() + 1} 月`

function monthGroup(d: Date) {
  return screen.getByRole('group', { name: monthLabel(d) })
}

function openPicker() {
  fireEvent.click(screen.getByRole('button', { name: '时间范围' }))
}

function dayCell(month: Date, day: number) {
  return within(monthGroup(month)).getByRole('button', { name: String(day) })
}

describe('RangePicker', () => {
  it('点击触发框打开弹层，展示预设与双月历', () => {
    render(<Harness onChange={vi.fn()} />)
    const trigger = screen.getByRole('button', { name: '时间范围' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).toHaveTextContent('选择日期范围')

    fireEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(screen.getByRole('button', { name: '近 1 月' })).toBeVisible()
    expect(screen.getByRole('button', { name: '今年以来' })).toBeVisible()
    const { left, right } = defaultMonths()
    expect(monthGroup(left)).toBeVisible()
    expect(monthGroup(right)).toBeVisible()
    // 未选完整区间前确定不可用
    expect(screen.getByRole('button', { name: '确定' })).toBeDisabled()
  })

  it('预设"近 1 月"确定后回写区间并关闭弹层', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    openPicker()
    fireEvent.click(screen.getByRole('button', { name: '近 1 月' }))
    fireEvent.click(screen.getByRole('button', { name: '确定' }))

    expect(onChange).toHaveBeenCalledWith(fmt(monthsAgo(today(), 1)), fmt(today()))
    expect(screen.queryByRole('dialog')).toBeNull()
    const trigger = screen.getByRole('button', { name: '时间范围' })
    expect(trigger).toHaveTextContent(`${fmt(monthsAgo(today(), 1))} → ${fmt(today())}`)
  })

  it('预设"今年以来"起点为当年 1 月 1 日', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    openPicker()
    fireEvent.click(screen.getByRole('button', { name: '今年以来' }))
    fireEvent.click(screen.getByRole('button', { name: '确定' }))
    const ytd = new Date(today().getFullYear(), 0, 1)
    expect(onChange).toHaveBeenCalledWith(fmt(ytd), fmt(today()))
  })

  it('日历依次点选起点与终点完成选择', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    openPicker()
    const { left } = defaultMonths()
    fireEvent.click(dayCell(left, 5))
    expect(screen.getByRole('button', { name: '确定' })).toBeDisabled()
    fireEvent.click(dayCell(left, 10))
    fireEvent.click(screen.getByRole('button', { name: '确定' }))

    const start = new Date(left.getFullYear(), left.getMonth(), 5)
    const end = new Date(left.getFullYear(), left.getMonth(), 10)
    expect(onChange).toHaveBeenCalledWith(fmt(start), fmt(end))
  })

  it('反向点选自动交换起点与终点', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    openPicker()
    const { left } = defaultMonths()
    fireEvent.click(dayCell(left, 10))
    fireEvent.click(dayCell(left, 5))
    fireEvent.click(screen.getByRole('button', { name: '确定' }))

    const start = new Date(left.getFullYear(), left.getMonth(), 5)
    const end = new Date(left.getFullYear(), left.getMonth(), 10)
    expect(onChange).toHaveBeenCalledWith(fmt(start), fmt(end))
  })

  it('起点终点单元格暴露 aria-pressed，悬停预览区间', () => {
    render(<Harness onChange={vi.fn()} />)
    openPicker()
    const { left } = defaultMonths()
    fireEvent.click(dayCell(left, 10))
    expect(dayCell(left, 10)).toHaveAttribute('aria-pressed', 'true')

    fireEvent.mouseEnter(dayCell(left, 15))
    expect(dayCell(left, 12).className).toContain('in-range')

    fireEvent.click(dayCell(left, 15))
    expect(dayCell(left, 15)).toHaveAttribute('aria-pressed', 'true')
  })

  it('未来日期禁用', () => {
    render(<Harness onChange={vi.fn()} />)
    openPicker()
    // 导航到下一月后右月历为下个月，全部晚于今日
    fireEvent.click(screen.getByRole('button', { name: '下一月' }))
    const next = new Date(today().getFullYear(), today().getMonth() + 1, 1)
    expect(dayCell(next, 1)).toBeDisabled()
  })

  it('清除回写空区间并禁用确定', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    openPicker()
    fireEvent.click(screen.getByRole('button', { name: '近 3 月' }))
    fireEvent.click(screen.getByRole('button', { name: '清除' }))

    expect(onChange).toHaveBeenLastCalledWith('', '')
    expect(screen.getByRole('button', { name: '确定' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '时间范围' })).toHaveTextContent('选择日期范围')
  })

  it('Escape 关闭弹层且不回写未确认的选择', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    openPicker()
    const { left } = defaultMonths()
    fireEvent.click(dayCell(left, 5))
    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('点击组件外部关闭弹层', () => {
    render(<Harness onChange={vi.fn()} />)
    openPicker()
    expect(screen.getByRole('dialog')).toBeVisible()
    fireEvent.pointerDown(screen.getByRole('button', { name: '外部按钮' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
