import { useEffect, useId, useRef, useState } from 'react'
import {
  searchInstruments,
  type Exchange,
  type InstrumentSummary,
} from '../../api/client'

interface InstrumentSearchProps {
  onSelect: (instrument: InstrumentSummary) => void
  search?: typeof searchInstruments
}

// 徽标单字：股票按市场，期货统一“期”（具体交易所以 exchange-tag 与 title 呈现）。
const EXCHANGE_MARKS: Record<Exchange, string> = {
  SSE: '沪',
  SZSE: '深',
  BSE: '北',
  SHFE: '期',
  INE: '期',
  DCE: '期',
  CZCE: '期',
}

export function InstrumentSearch({ onSelect, search = searchInstruments }: InstrumentSearchProps) {
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<InstrumentSummary[]>([])
  const [activeIndex, setActiveIndex] = useState(-1)
  const [status, setStatus] = useState<'idle' | 'loading' | 'error'>('idle')
  const [collapsed, setCollapsed] = useState(false)
  const requestRef = useRef(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listID = useId()

  useEffect(() => {
    const value = query.trim()
    setCollapsed(false)
    if (!value) {
      setItems([])
      setActiveIndex(-1)
      setStatus('idle')
      return
    }
    const request = ++requestRef.current
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setStatus('loading')
      search(value, controller.signal)
        .then((results) => {
          if (request !== requestRef.current) return
          setItems(results)
          setActiveIndex(-1)
          setStatus('idle')
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted || request !== requestRef.current) return
          setItems([])
          setStatus('error')
          console.error('search instruments', error)
        })
    }, 220)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [query, search])

  // 全局 “/” 快捷键：不在输入态时聚焦搜索框
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey) return
      const target = event.target instanceof Element ? event.target : null
      if (target?.closest('input, textarea, select, [contenteditable="true"]')) return
      event.preventDefault()
      inputRef.current?.focus()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const select = (item: InstrumentSummary) => {
    setQuery('')
    setItems([])
    setActiveIndex(-1)
    onSelect(item)
  }

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown' && items.length > 0) {
      event.preventDefault()
      setActiveIndex((current) => (current + 1) % items.length)
    } else if (event.key === 'ArrowUp' && items.length > 0) {
      event.preventDefault()
      setActiveIndex((current) => (current <= 0 ? items.length - 1 : current - 1))
    } else if (event.key === 'Enter' && activeIndex >= 0) {
      event.preventDefault()
      select(items[activeIndex])
    } else if (event.key === 'Escape') {
      setItems([])
      setActiveIndex(-1)
      setCollapsed(true)
    }
  }

  const dropdownOpen = query.trim() !== '' && !collapsed

  return (
    <section className="instrument-search" aria-label="股票搜索区">
      <div className="search-box">
        <span className="search-icon" aria-hidden="true">⌕</span>
        <input
          ref={inputRef}
          aria-label="搜索股票"
          aria-autocomplete="list"
          aria-controls={listID}
          aria-expanded={dropdownOpen}
          aria-activedescendant={activeIndex >= 0 ? `${listID}-${activeIndex}` : undefined}
          autoComplete="off"
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={onKeyDown}
          placeholder="代码 / 名称"
          role="combobox"
          value={query}
        />
        {status === 'loading' && <span className="search-spinner" aria-label="正在搜索" />}
        <kbd className="search-kbd" aria-hidden="true">/</kbd>
      </div>
      {dropdownOpen && (
        <div className="search-results" id={listID} role="listbox" aria-label="搜索结果">
          {status === 'idle' && items.length === 0 && (
            <p className="search-empty">没有匹配的证券</p>
          )}
          {status === 'error' && <p className="search-error">搜索失败，请稍后重试</p>}
          {items.map((item, index) => (
            <button
              aria-selected={activeIndex === index}
              className={activeIndex === index ? 'search-result active' : 'search-result'}
              id={`${listID}-${index}`}
              key={item.instrument}
              onClick={() => select(item)}
              onMouseEnter={() => setActiveIndex(index)}
              role="option"
              type="button"
            >
              <span className="symbol-mark" title={item.exchange}>{EXCHANGE_MARKS[item.exchange]}</span>
              <span className="result-name"><strong>{item.name}</strong><small>{item.code}</small></span>
              <span className="exchange-tag">{item.exchange}</span>
            </button>
          ))}
        </div>
      )}
    </section>
  )
}
