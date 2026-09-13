import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { InstrumentSearch } from './InstrumentSearch'

const result = {
  instrument: 'SZSE:002415',
  code: '002415',
  name: '海康威视',
  exchange: 'SZSE' as const,
  board: 'MAIN',
  lot_size: 100,
}

describe('InstrumentSearch', () => {
  it('搜索证券并支持键盘选择', async () => {
    const search = vi.fn().mockResolvedValue([result])
    const onSelect = vi.fn()
    render(<InstrumentSearch search={search} onSelect={onSelect} />)

    fireEvent.change(screen.getByRole('combobox', { name: '搜索股票' }), {
      target: { value: '海康' },
    })

    await waitFor(() => expect(search).toHaveBeenCalledWith('海康', expect.any(AbortSignal)))
    expect(await screen.findByText('海康威视')).toBeVisible()
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' })

    expect(onSelect).toHaveBeenCalledWith(result)
  })

  it('清空查询时显示引导文案', () => {
    render(<InstrumentSearch search={vi.fn()} onSelect={vi.fn()} />)
    expect(screen.getByText('输入代码或名称开始搜索')).toBeVisible()
  })

  it('处理搜索失败以及结果列表的关闭', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const search = vi.fn().mockRejectedValue(new Error('offline'))
    render(<InstrumentSearch search={search} onSelect={vi.fn()} />)
    const input = screen.getByRole('combobox')
    fireEvent.change(input, { target: { value: '600000' } })

    expect(await screen.findByText('搜索失败，请稍后重试')).toBeVisible()
    expect(consoleError).toHaveBeenCalled()
    fireEvent.keyDown(input, { key: 'Escape' })
    fireEvent.change(input, { target: { value: '' } })
    expect(screen.getByText('输入代码或名称开始搜索')).toBeVisible()
    consoleError.mockRestore()
  })
})
