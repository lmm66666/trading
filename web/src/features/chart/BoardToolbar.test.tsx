import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import {
  activateChartBoard,
  createChartBoard,
  deleteChartBoard,
  listChartBoards,
  updateChartBoard,
  type BoardConfig,
  type ChartBoardState,
} from '../../api/client'
import { BoardToolbar } from './BoardToolbar'
import { useBoards } from './useBoards'
import { defaultBoardConfig } from './boards'

vi.mock('../../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/client')>()
  return {
    ...actual,
    activateChartBoard: vi.fn(),
    createChartBoard: vi.fn(),
    deleteChartBoard: vi.fn(),
    listChartBoards: vi.fn(),
    updateChartBoard: vi.fn(),
  }
})

/** 有状态 fake 服务端，与 useBoards.test 同口径。 */
let server: { boards: { id: number; name: string; config: BoardConfig }[]; activeId: number }
let nextId: number

function stateOf(): ChartBoardState {
  return { boards: structuredClone(server.boards), active_id: server.activeId }
}

beforeEach(() => {
  server = { boards: [], activeId: 0 }
  nextId = 1
  vi.mocked(listChartBoards).mockReset().mockImplementation(async () => stateOf())
  vi.mocked(createChartBoard).mockReset().mockImplementation(async (name, config) => {
    const board = { id: nextId++, name, config: structuredClone(config) }
    server.boards.push(board)
    server.activeId = board.id
    return stateOf()
  })
  vi.mocked(updateChartBoard).mockReset().mockImplementation(async (id, changes) => {
    const board = server.boards.find((b) => b.id === id)
    if (!board) throw new Error('看板不存在')
    if (changes.name !== undefined) board.name = changes.name
    if (changes.config !== undefined) board.config = structuredClone(changes.config)
    return stateOf()
  })
  vi.mocked(activateChartBoard).mockReset().mockImplementation(async (id) => {
    server.activeId = id
    return stateOf()
  })
  vi.mocked(deleteChartBoard).mockReset().mockImplementation(async (id) => {
    server.boards = server.boards.filter((b) => b.id !== id)
    if (server.activeId === id) server.activeId = server.boards[0]?.id ?? 0
    return stateOf()
  })
})

function Harness() {
  const board = useBoards({ defaultSymbol: 'SSE:600938' })
  return <BoardToolbar board={board} instrument="SSE:601857" />
}

const click = (name: string) => fireEvent.click(screen.getByRole('button', { name }))
const nameBoard = (value: string) => {
  fireEvent.change(screen.getByRole('textbox', { name: '看板名称' }), { target: { value } })
  click('确认')
}
const boardCount = () =>
  (screen.getByRole('combobox', { name: '当前看板' }) as HTMLSelectElement).options.length

it('manually saves and manages independent named boards', async () => {
  render(<Harness />)
  fireEvent.change(await screen.findByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'INE:SC.MAIN' },
  })
  expect(screen.getByRole('status')).toHaveTextContent('未保存')
  click('保存')
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已保存'))
  expect(server.boards[0].config.comparison).toBe('INE:SC.MAIN')

  fireEvent.click(screen.getByText('更多'))
  click('重命名')
  nameBoard('油价看板')
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: '当前看板' })).toHaveDisplayValue('油价看板'),
  )

  fireEvent.click(screen.getByText('更多'))
  click('另存为新看板')
  nameBoard('油价副本')
  await waitFor(() => expect(boardCount()).toBe(2))
  expect(screen.getByRole('combobox', { name: '同图叠加' })).toHaveValue('INE:SC.MAIN')

  fireEvent.click(screen.getByText('更多'))
  click('设为默认股票')
  expect(screen.getByRole('status')).toHaveTextContent('未保存')
  click('恢复已保存版本')
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已保存'))

  fireEvent.click(screen.getByText('更多'))
  click('新建看板')
  nameBoard('空白看板')
  await waitFor(() => expect(boardCount()).toBe(3))
  expect(screen.getByRole('combobox', { name: '同图叠加' })).toHaveValue('')

  click('删除看板')
  click('取消')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  click('删除看板')
  click('确认删除')
  await waitFor(() => expect(boardCount()).toBe(2))
  expect(screen.getByRole('status')).toHaveTextContent('已保存')
})

it('allows cancel, discard, or save when switching a dirty board', async () => {
  render(<Harness />)
  await screen.findByRole('status')
  fireEvent.click(screen.getByText('更多'))
  click('另存为新看板')
  nameBoard('第二看板')
  await waitFor(() => expect(boardCount()).toBe(2))
  const second = server.boards[1].id
  fireEvent.change(screen.getByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'INE:SC.MAIN' },
  })
  const select = () =>
    fireEvent.change(screen.getByRole('combobox', { name: '当前看板' }), {
      target: { value: String(server.boards[0].id) },
    })
  select()
  click('取消')
  expect(screen.getByRole('combobox', { name: '当前看板' })).toHaveValue(String(second))

  select()
  click('保存并切换')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(server.boards[1].config.comparison).toBe('INE:SC.MAIN')

  fireEvent.change(screen.getByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'SHFE:AU.MAIN' },
  })
  fireEvent.change(screen.getByRole('combobox', { name: '当前看板' }), {
    target: { value: String(second) },
  })
  click('放弃修改并切换')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(server.boards[0].config.comparison).toBeNull()
  expect(screen.getByRole('combobox', { name: '同图叠加' })).toHaveValue('INE:SC.MAIN')

  fireEvent.change(screen.getByRole('combobox', { name: '当前看板' }), {
    target: { value: String(server.boards[0].id) },
  })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('keeps switching dialog open when saving fails', async () => {
  render(<Harness />)
  await screen.findByRole('status')
  fireEvent.click(screen.getByText('更多'))
  click('另存为新看板')
  nameBoard('第二看板')
  await waitFor(() => expect(boardCount()).toBe(2))
  fireEvent.change(screen.getByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'INE:SC.MAIN' },
  })
  vi.mocked(updateChartBoard).mockRejectedValueOnce(new Error('服务维护中，请稍后重试'))
  fireEvent.change(screen.getByRole('combobox', { name: '当前看板' }), {
    target: { value: String(server.boards[0].id) },
  })
  click('保存并切换')
  await waitFor(() =>
    expect(within(screen.getByRole('dialog')).getByRole('alert')).toHaveTextContent('服务维护中'),
  )
  expect(screen.getByRole('dialog')).toBeVisible()
})

it('keeps keyboard focus in the dialog and Escape cancels without a write', async () => {
  render(<Harness />)
  await screen.findByRole('status')
  fireEvent.click(screen.getByText('更多'))
  click('另存为新看板')
  const dialog = screen.getByRole('dialog')
  const cancel = within(dialog).getByRole('button', { name: '取消' })
  cancel.focus()
  fireEvent.keyDown(cancel, { key: 'Tab' })
  expect(screen.getByRole('textbox', { name: '看板名称' })).toHaveFocus()
  fireEvent.keyDown(document.activeElement!, { key: 'Tab', shiftKey: true })
  expect(cancel).toHaveFocus()
  fireEvent.keyDown(cancel, { key: 'Escape' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(server.boards).toHaveLength(1)
})
