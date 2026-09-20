import { useEffect, useRef, useState, type ReactNode } from 'react'

const PRESETS: ReadonlyArray<{ label: string; months?: number; ytd?: boolean }> = [
  { label: '近 1 月', months: 1 },
  { label: '近 3 月', months: 3 },
  { label: '近 6 月', months: 6 },
  { label: '近 1 年', months: 12 },
  { label: '近 3 年', months: 36 },
  { label: '今年以来', ytd: true },
]

const DAY_LABELS: ReadonlyArray<string> = ['一', '二', '三', '四', '五', '六', '日']

function fmtDate(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

function parseDate(value: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
  const date = new Date(`${value}T00:00:00`)
  return Number.isNaN(date.getTime()) ? null : date
}

function startOfToday(): Date {
  const date = new Date()
  date.setHours(0, 0, 0, 0)
  return date
}

/** 今天减去 N 个自然月，月末截断（如 3 月 31 日减 1 月落到 2 月 28 日） */
function monthsAgo(base: Date, months: number): Date {
  const first = new Date(base.getFullYear(), base.getMonth() - months, 1)
  const daysInMonth = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()
  first.setDate(Math.min(base.getDate(), daysInMonth))
  return first
}

function monthLabel(date: Date): string {
  return `${date.getFullYear()} 年 ${date.getMonth() + 1} 月`
}

interface RangePickerProps {
  /** 触发按钮的可访问名称，同时区分扫描/回测两处实例 */
  ariaLabel?: string
  from: string
  to: string
  /** 仅在点击确定（完整区间）或清除（空区间）时回写 YYYY-MM-DD */
  onChange: (from: string, to: string) => void
}

/** 深色主题下的日期范围选择器：快捷预设 + 双月历区间点选 */
export function RangePicker({ ariaLabel = '时间范围', from, to, onChange }: RangePickerProps) {
  const [open, setOpen] = useState(false)
  const [start, setStart] = useState<Date | null>(null)
  const [end, setEnd] = useState<Date | null>(null)
  const [hover, setHover] = useState<Date | null>(null)
  const [preset, setPreset] = useState<string | null>(null)
  const [view, setView] = useState(() => {
    const today = startOfToday()
    return new Date(today.getFullYear(), today.getMonth() - 1, 1)
  })
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current && event.target instanceof Node && !rootRef.current.contains(event.target)) {
        setOpen(false)
      }
    }
    document.addEventListener('keydown', onKeyDown)
    document.addEventListener('pointerdown', onPointerDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('pointerdown', onPointerDown)
    }
  }, [open])

  const openPopover = () => {
    const fromDate = parseDate(from)
    const base = fromDate ?? startOfToday()
    setStart(fromDate)
    setEnd(parseDate(to))
    setPreset(null)
    setHover(null)
    setView(new Date(base.getFullYear(), base.getMonth() - (fromDate ? 0 : 1), 1))
    setOpen(true)
  }

  const pickDay = (date: Date) => {
    if (!start || (start && end)) {
      setStart(date)
      setEnd(null)
    } else if (date < start) {
      setEnd(start)
      setStart(date)
    } else {
      setEnd(date)
    }
    setPreset(null)
    setHover(null)
  }

  const applyPreset = (item: { label: string; months?: number; ytd?: boolean }) => {
    const today = startOfToday()
    const nextStart = item.ytd ? new Date(today.getFullYear(), 0, 1) : monthsAgo(today, item.months ?? 0)
    setStart(nextStart)
    setEnd(today)
    setPreset(item.label)
    setHover(null)
    setView(new Date(nextStart.getFullYear(), nextStart.getMonth(), 1))
  }

  const confirm = () => {
    if (!start || !end) return
    onChange(fmtDate(start), fmtDate(end))
    setOpen(false)
  }

  const clear = () => {
    setStart(null)
    setEnd(null)
    setPreset(null)
    setHover(null)
    onChange('', '')
  }

  const renderCalendar = (month: Date, nav: 'prev' | 'next') => {
    const year = month.getFullYear()
    const monthIndex = month.getMonth()
    const firstDow = (new Date(year, monthIndex, 1).getDay() + 6) % 7
    const daysInMonth = new Date(year, monthIndex + 1, 0).getDate()
    const today = startOfToday()
    const previewStart = start && !end && hover ? (hover < start ? hover : start) : null
    const previewEnd = start && !end && hover ? (hover < start ? start : hover) : null
    const cells: ReactNode[] = []
    for (let i = 0; i < firstDow; i += 1) {
      cells.push(<span key={`blank-${i}`} className="cal-day blank" aria-hidden="true" />)
    }
    for (let day = 1; day <= daysInMonth; day += 1) {
      const date = new Date(year, monthIndex, day)
      const isStart = start !== null && fmtDate(start) === fmtDate(date)
      const isEnd = end !== null && fmtDate(end) === fmtDate(date)
      const lo = end ? start : previewStart
      const hi = end ? end : previewEnd
      const inRange = lo !== null && hi !== null && date > lo && date < hi
      const classes = ['cal-day']
      if (inRange) classes.push('in-range')
      if (isStart) classes.push('range-start')
      if (isEnd) classes.push('range-end')
      if (fmtDate(date) === fmtDate(today)) classes.push('today')
      cells.push(
        <button
          key={day}
          type="button"
          className={classes.join(' ')}
          disabled={date > today}
          aria-pressed={isStart || isEnd ? 'true' : undefined}
          onClick={() => pickDay(date)}
          onMouseEnter={() => {
            if (start && !end) setHover(date)
          }}
        >
          {day}
        </button>,
      )
    }
    return (
      <div className="cal" role="group" aria-label={monthLabel(month)}>
        <div className="cal-head">
          {nav === 'prev' && (
            <button
              type="button"
              className="cal-nav"
              aria-label="上一月"
              onClick={() => setView(new Date(view.getFullYear(), view.getMonth() - 1, 1))}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="m15 18-6-6 6-6" />
              </svg>
            </button>
          )}
          <span className="cal-title">{monthLabel(month)}</span>
          {nav === 'next' && (
            <button
              type="button"
              className="cal-nav"
              aria-label="下一月"
              onClick={() => setView(new Date(view.getFullYear(), view.getMonth() + 1, 1))}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="m9 18 6-6-6-6" />
              </svg>
            </button>
          )}
        </div>
        <div className="cal-grid">
          {DAY_LABELS.map((label) => (
            <span key={label} className="cal-dow">{label}</span>
          ))}
          {cells}
        </div>
      </div>
    )
  }

  const leftMonth = view
  const rightMonth = new Date(view.getFullYear(), view.getMonth() + 1, 1)

  return (
    <div className="range-picker" ref={rootRef}>
      <button
        type="button"
        className={open ? 'range-trigger open' : 'range-trigger'}
        aria-label={ariaLabel}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => (open ? setOpen(false) : openPopover())}
      >
        <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <rect x="3" y="4" width="18" height="18" rx="2" />
          <path d="M16 2v4M8 2v4M3 10h18" />
        </svg>
        <span className={from && to ? 'range-text' : 'range-text placeholder'}>
          {from && to ? `${from} → ${to}` : '选择日期范围'}
        </span>
        <svg className="icon chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>
      {open && (
        <div className="range-popover" role="dialog">
          <div className="range-main">
            <div className="range-presets">
              {PRESETS.map((item) => (
                <button
                  key={item.label}
                  type="button"
                  className={preset === item.label ? 'active' : ''}
                  onClick={() => applyPreset(item)}
                >
                  {item.label}
                </button>
              ))}
            </div>
            <div className="range-calendars">
              {renderCalendar(leftMonth, 'prev')}
              {renderCalendar(rightMonth, 'next')}
            </div>
          </div>
          <div className="range-foot">
            <span className="range-selection">
              {start ? `${fmtDate(start)} → ${end ? fmtDate(end) : '…'}` : '点击选择起始日期'}
            </span>
            <div className="range-actions">
              <button type="button" className="range-btn" onClick={clear}>清除</button>
              <button type="button" className="range-btn primary" disabled={!start || !end} onClick={confirm}>确定</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
