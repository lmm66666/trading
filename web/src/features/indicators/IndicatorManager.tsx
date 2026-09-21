import { useState } from 'react'
import type { IndicatorRequest } from '../../api/client'
import { validIndicator } from '../chart/boards'
import { indicatorIdentity, indicatorLabel } from '../chart/chartData'

interface IndicatorManagerProps {
  indicators: IndicatorRequest[]
  onChange: (indicators: IndicatorRequest[]) => void
}

const overlayPresets: IndicatorRequest[] = [
  { kind: 'SMA', period: 5 },
  { kind: 'SMA', period: 20 },
  { kind: 'SMA', period: 60 },
  { kind: 'EMA', period: 10 },
]

const panePresets: IndicatorRequest[] = [
  { kind: 'MACD', fast: 12, slow: 26, signal: 9 },
  { kind: 'KDJ', period: 9 },
  { kind: 'STD', period: 20 },
  { kind: 'RETZ', period: 126, smooth: 5, regime: 252 },
]

export function IndicatorManager({ indicators, onChange }: IndicatorManagerProps) {
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  const apply = (next: IndicatorRequest[]) => {
    const cost = next.reduce(
      (sum, i) =>
        sum +
        (i.kind === 'KDJ'
          ? i.period * 3
          : i.kind === 'STD'
            ? i.period
            : i.kind === 'RETZ'
              ? i.period + i.regime
              : i.kind === 'MACD'
                ? 3
                : 1),
      0,
    )
    if (
      next.length > 16 ||
      !next.every(validIndicator) ||
      new Set(next.map(indicatorIdentity)).size !== next.length ||
      cost > 2000
    ) {
      setError('参数无效、重复或超出指标预算（最多16个，周期1–500）')
      return
    }
    setError('')
    onChange(next)
  }
  const identities = new Set(indicators.map(indicatorIdentity))

  const toggle = (indicator: IndicatorRequest) => {
    const identity = indicatorIdentity(indicator)
    apply(
      identities.has(identity)
        ? indicators.filter((item) => indicatorIdentity(item) !== identity)
        : [...indicators, indicator],
    )
  }

  const renderGroup = (title: string, description: string, presets: IndicatorRequest[]) => (
    <section className="indicator-group">
      <div className="indicator-group-heading">
        <strong>{title}</strong>
        <span>{description}</span>
      </div>
      <div className="indicator-options">
        {presets.map((indicator) => {
          const label = indicatorLabel(indicator)
          const selected = identities.has(indicatorIdentity(indicator))
          return (
            <button
              aria-label={`${selected ? '移除' : '添加'} ${label}`}
              aria-pressed={selected}
              className={selected ? 'indicator-option selected' : 'indicator-option'}
              key={indicatorIdentity(indicator)}
              onClick={() => toggle(indicator)}
              type="button"
            >
              <span>{label}</span>
              <span aria-hidden="true">{selected ? '✓' : '+'}</span>
            </button>
          )
        })}
      </div>
    </section>
  )

  return (
    <div className="indicator-manager">
      <button
        aria-expanded={open}
        className={open ? 'toolbar-button active' : 'toolbar-button'}
        onClick={() => setOpen((value) => !value)}
        type="button"
      >
        <span className="button-glyph">⌁</span>
        指标
        <span className="indicator-count">{indicators.length}</span>
      </button>
      {open && (
        <div className="indicator-popover" role="dialog" aria-label="指标管理">
          <div className="popover-heading">
            <div>
              <span className="eyebrow">INDICATORS</span>
              <h2>指标管理</h2>
            </div>
            <button
              aria-label="关闭指标管理"
              className="icon-button"
              onClick={() => setOpen(false)}
              type="button"
            >
              ×
            </button>
          </div>
          {renderGroup('叠加指标', '直接显示在主图价格区域', overlayPresets)}
          {renderGroup('副图指标', '在价格图下方独立分栏', panePresets)}
          <section className="indicator-editors" aria-label="已添加指标参数">
            {indicators.map((indicator, index) => (
              <IndicatorEditor
                key={indicatorIdentity(indicator)}
                indicator={indicator}
                onApply={(next) => apply(indicators.map((item, i) => (i === index ? next : item)))}
                onRemove={() => apply(indicators.filter((_, i) => i !== index))}
                onMove={(direction) => {
                  const next = [...indicators]
                  const target = index + direction
                  if (target < 0 || target >= next.length) return
                  ;[next[index], next[target]] = [next[target], next[index]]
                  apply(next)
                }}
              />
            ))}
          </section>
          {error && <p role="alert">{error}</p>}
          <p className="indicator-hint">
            STD 为收盘价总体标准差；涨幅Z 为对数日涨幅的滚动 Z-Score（band,smooth,regime，要求 regime &gt;
            band）。指标跟随当前股票。参数模板由服务端统一计算，图表只负责展现。
          </p>
        </div>
      )}
    </div>
  )
}

function IndicatorEditor({
  indicator,
  onApply,
  onRemove,
  onMove,
}: {
  indicator: IndicatorRequest
  onApply: (next: IndicatorRequest) => void
  onRemove: () => void
  onMove: (direction: number) => void
}) {
  const [draft, setDraft] = useState(indicator)
  const label = indicatorLabel(indicator)
  const fields =
    indicator.kind === 'MACD'
      ? ['fast', 'slow', 'signal']
      : indicator.kind === 'RETZ'
        ? ['period', 'smooth', 'regime']
        : ['period']
  return (
    <form
      className="indicator-editor"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(draft)
      }}
    >
      <strong>{label}</strong>
      {fields.map((field) => (
        <label key={field}>
          {field}
          <input
            aria-label={`${label} ${field}`}
            type="number"
            min={1}
            max={500}
            required
            value={Number((draft as unknown as Record<string, unknown>)[field])}
            onChange={(e) => setDraft({ ...draft, [field]: Number(e.target.value) })}
          />
        </label>
      ))}
      <button aria-label={`应用 ${label} 参数`} type="submit">
        应用
      </button>
      <button aria-label={`上移 ${label}`} onClick={() => onMove(-1)} type="button">
        ↑
      </button>
      <button aria-label={`下移 ${label}`} onClick={() => onMove(1)} type="button">
        ↓
      </button>
      <button aria-label={`删除 ${label}`} onClick={onRemove} type="button">
        删除
      </button>
    </form>
  )
}
