import { afterEach, describe, expect, it, vi } from 'vitest'
import { queryChart, searchInstruments } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

describe('API client', () => {
  it('搜索时编码关键词并返回列表', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 0, message: 'ok', data: { items: [{ instrument: 'SZSE:002415' }] },
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()

    await expect(searchInstruments(' 海康 ', controller.signal)).resolves.toEqual([{ instrument: 'SZSE:002415' }])
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/instruments?q=%E6%B5%B7%E5%BA%B7&limit=20', { signal: controller.signal })
  })

  it('发送图表查询 JSON', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 0, message: 'ok', data: { bars: [] },
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const input = { instrument: 'SZSE:002415', timeframe: 'DAY' as const, price_view: 'QFQ' as const, indicators: [] }

    await queryChart(input)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/chart-queries', expect.objectContaining({
      method: 'POST', body: JSON.stringify(input),
    }))
  })

  it('将业务错误与非 JSON 响应转换为可读错误', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      code: 40001, message: '参数无效', data: null,
    }), { status: 400 })).mockResolvedValueOnce(new Response('gateway down', { status: 502 })))

    await expect(searchInstruments('x')).rejects.toThrow('参数无效')
    await expect(searchInstruments('x')).rejects.toThrow('无法解析')
  })
})

describe('开发模式连接失败回退', () => {
  it('后端不可达时返回演示数据', async () => {
    vi.stubEnv('MODE', 'development')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    vi.resetModules()
    const { queryChart, searchInstruments } = await import('./client')

    await expect(searchInstruments('600519')).resolves.toEqual([
      expect.objectContaining({ code: '600519', name: '贵州茅台' }),
    ])
    const chart = await queryChart({
      instrument: 'SSE:600519', timeframe: 'DAY', price_view: 'QFQ', indicators: [],
    })
    expect(chart.bars.length).toBeGreaterThan(0)
    expect(chart.instrument.name).toBe('贵州茅台')
  })

  it('业务错误不走演示数据回退', async () => {
    vi.stubEnv('MODE', 'development')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 40001, message: '参数无效', data: null,
    }), { status: 400 })))
    vi.resetModules()
    const { searchInstruments } = await import('./client')

    await expect(searchInstruments('x')).rejects.toThrow('参数无效')
  })

  it('非开发模式不启用回退', async () => {
    vi.stubEnv('MODE', 'test')
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    vi.resetModules()
    const { searchInstruments } = await import('./client')

    await expect(searchInstruments('x')).rejects.toThrow('无法连接服务')
  })
})
