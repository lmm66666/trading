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
  it('搜索证券并支持键盘选择，选中后清空输入', async () => {
    const search = vi.fn().mockResolvedValue([result])
    const onSelect = vi.fn()
    render(<InstrumentSearch search={search} onSelect={onSelect} />)
    const input = screen.getByRole('combobox', { name: '搜索股票' })

    fireEvent.change(input, { target: { value: '海康' } })

    await waitFor(() => expect(search).toHaveBeenCalledWith('海康', expect.any(AbortSignal)))
    expect(await screen.findByText('海康威视')).toBeVisible()
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onSelect).toHaveBeenCalledWith(result)
    expect(input).toHaveValue('')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('无匹配时提示，清空输入后收起下拉', async () => {
    const search = vi.fn().mockResolvedValue([])
    render(<InstrumentSearch search={search} onSelect={vi.fn()} />)
    const input = screen.getByRole('combobox')
    fireEvent.change(input, { target: { value: '600000' } })

    expect(await screen.findByText('没有匹配的证券')).toBeVisible()
    fireEvent.change(input, { target: { value: '' } })
    expect(screen.queryByText('没有匹配的证券')).not.toBeInTheDocument()
  })

  it('处理搜索失败以及 Escape 收起下拉', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const search = vi.fn().mockRejectedValue(new Error('offline'))
    render(<InstrumentSearch search={search} onSelect={vi.fn()} />)
    const input = screen.getByRole('combobox')
    fireEvent.change(input, { target: { value: '600000' } })

    expect(await screen.findByText('搜索失败，请稍后重试')).toBeVisible()
    expect(consoleError).toHaveBeenCalled()
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(screen.queryByText('搜索失败，请稍后重试')).not.toBeInTheDocument()
    consoleError.mockRestore()
  })

  it('全局 “/” 快捷键在非输入态聚焦搜索框', () => {
    render(<InstrumentSearch search={vi.fn()} onSelect={vi.fn()} />)
    const input = screen.getByRole('combobox')
    expect(input).not.toHaveFocus()

    fireEvent.keyDown(window, { key: '/' })

    expect(input).toHaveFocus()
  })

  it('处于其他输入框时按 "/" 不劫持焦点', () => {
    render(
      <>
        <input aria-label="其他输入框" />
        <InstrumentSearch search={vi.fn()} onSelect={vi.fn()} />
      </>,
    )
    const other = screen.getByLabelText('其他输入框')

    fireEvent.keyDown(other, { key: '/' })

    expect(screen.getByRole('combobox')).not.toHaveFocus()
  })
})
