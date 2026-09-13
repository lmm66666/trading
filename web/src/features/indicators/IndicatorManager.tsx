import { useState } from 'react'
import type { IndicatorRequest } from '../../api/client'
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
]

export function IndicatorManager({ indicators, onChange }: IndicatorManagerProps) {
  const [open, setOpen] = useState(false)
  const identities = new Set(indicators.map(indicatorIdentity))

  const toggle = (indicator: IndicatorRequest) => {
    const identity = indicatorIdentity(indicator)
    onChange(
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
              <span>{label}</span><span aria-hidden="true">{selected ? '✓' : '+'}</span>
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
            <div><span className="eyebrow">INDICATORS</span><h2>指标管理</h2></div>
            <button aria-label="关闭指标管理" className="icon-button" onClick={() => setOpen(false)} type="button">×</button>
          </div>
          {renderGroup('叠加指标', '直接显示在主图价格区域', overlayPresets)}
          {renderGroup('副图指标', '在价格图下方独立分栏', panePresets)}
          <p className="indicator-hint">参数模板由服务端统一计算，图表只负责展现。</p>
        </div>
      )}
    </div>
  )
}
