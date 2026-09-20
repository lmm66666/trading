import { useEffect, useState } from 'react'
import { strategyParamList, type StrategyDefinition, type StrategyParam } from '../../api/client'

export interface StrategyFormValue {
  strategy: string
  version: string
  /** 仅包含用户填写的参数；留空项不提交，由服务端取默认值 */
  parameters: Record<string, number>
}

interface StrategyFormProps {
  definitions: StrategyDefinition[]
  loading?: boolean
  error?: string | null
  value: StrategyFormValue
  /** valid=false 表示存在超出 min/max 或非法的参数，调用方应拦截提交 */
  onChange: (value: StrategyFormValue, valid: boolean) => void
  disabled?: boolean
}

export const TIMEFRAME_LABELS: Record<string, string> = { daily: '日线', weekly: '周线' }

function parseEntries(
  params: StrategyParam[],
  raw: Record<string, string>,
): { entries: Record<string, number>; invalidNames: Set<string> } {
  const entries: Record<string, number> = {}
  const invalidNames = new Set<string>()
  for (const param of params) {
    const text = (raw[param.name] ?? '').trim()
    if (!text) continue
    const parsed = Number(text)
    if (!Number.isFinite(parsed)) {
      invalidNames.add(param.name)
      continue
    }
    const normalized = param.integer ? Math.round(parsed) : parsed
    if (normalized < param.min || normalized > param.max) {
      invalidNames.add(param.name)
      continue
    }
    entries[param.name] = normalized
  }
  return { entries, invalidNames }
}

/** 策略与参数选择表单：目录由调用方加载，参数校验结果随 onChange 上报 */
export function StrategyForm({ definitions, loading, error, value, onChange, disabled }: StrategyFormProps) {
  const [raw, setRaw] = useState<Record<string, string>>({})
  const selected = definitions.find(
    (item) => item.strategy === value.strategy && item.version === value.version,
  )
  const params = selected ? strategyParamList(selected) : []
  const { entries, invalidNames } = parseEntries(params, raw)

  useEffect(() => {
    setRaw({})
  }, [value.strategy, value.version])

  const selectStrategy = (strategyId: string) => {
    const definition = definitions.find((item) => item.strategy === strategyId)
    if (!definition) return
    setRaw({})
    onChange({ strategy: definition.strategy, version: definition.version, parameters: {} }, true)
  }

  const updateParam = (name: string, text: string) => {
    const nextRaw = { ...raw, [name]: text }
    setRaw(nextRaw)
    const parsed = parseEntries(params, nextRaw)
    onChange({ ...value, parameters: parsed.entries }, parsed.invalidNames.size === 0)
  }

  if (loading) {
    return <p className="form-hint">策略目录加载中…</p>
  }
  if (error) {
    return <p className="form-error">{error}</p>
  }
  if (definitions.length === 0) {
    return <p className="form-hint">服务端暂无可用策略</p>
  }

  return (
    <div className="strategy-form">
      <label className="field">
        策略
        <select
          value={value.strategy}
          disabled={disabled}
          onChange={(event) => selectStrategy(event.target.value)}
        >
          {definitions.map((definition) => (
            <option key={`${definition.strategy}@${definition.version}`} value={definition.strategy}>
              {definition.strategy} v{definition.version}
            </option>
          ))}
        </select>
      </label>
      {selected && (
        <p className="strategy-meta">
          主周期 {TIMEFRAME_LABELS[selected.primary_timeframe] ?? selected.primary_timeframe}
          {' · '}预热 {selected.warmup_bars} 根
          {' · '}默认持有 {selected.default_hold_bars} 根
        </p>
      )}
      {params.length > 0 && (
        <div className="param-grid">
          {params.map((param) => {
            const invalid = invalidNames.has(param.name)
            return (
              <label className="field" key={param.name}>
                {param.name}
                <input
                  type="number"
                  aria-label={`参数 ${param.name}`}
                  aria-invalid={invalid}
                  min={param.min}
                  max={param.max}
                  step={param.integer ? 1 : 'any'}
                  placeholder={`默认 ${param.default}`}
                  value={raw[param.name] ?? ''}
                  disabled={disabled}
                  onChange={(event) => updateParam(param.name, event.target.value)}
                />
                {invalid && (
                  <span className="field-error">
                    范围 {param.min}–{param.max}
                    {param.integer ? ' 的整数' : ''}
                  </span>
                )}
              </label>
            )
          })}
        </div>
      )}
    </div>
  )
}
