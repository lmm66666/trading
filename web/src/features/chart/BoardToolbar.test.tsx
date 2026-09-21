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
const trigger = () => screen.getByRole('button', { name: /^看板：/ })
const openBoards = async () => fireEvent.click(await screen.findByRole('button', { name: /^看板：/ }))
const boardOptions = () =>
  within(screen.getByRole('listbox', { name: '看板列表' })).getAllByRole('option')
const nameBoard = (value: string) => {
  fireEvent.change(screen.getByRole('textbox', { name: '看板名称' }), { target: { value } })
  click('确认')
}

it('manually saves and manages independent named boards', async () => {
  render(<Harness />)
  fireEvent.change(await screen.findByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'INE:SC.MAIN' },
  })
  await openBoards()
  expect(screen.getByRole('status')).toHaveTextContent('未保存')
  click('保存')
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已保存'))
  expect(server.boards[0].config.comparison).toBe('INE:SC.MAIN')

  click('重命名')
  nameBoard('油价看板')
  await waitFor(() => expect(trigger()).toHaveAttribute('aria-label', '看板：油价看板'))

  await openBoards()
  click('另存为新看板')
  nameBoard('油价副本')
  await waitFor(() => expect(server.boards).toHaveLength(2))
  expect(screen.getByRole('combobox', { name: '同图叠加' })).toHaveValue('INE:SC.MAIN')

  await openBoards()
  expect(boardOptions()).toHaveLength(2)
  click('设为默认股票')
  expect(screen.getByRole('status')).toHaveTextContent('未保存')
  click('恢复已保存版本')
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已保存'))

  click('新建看板')
  nameBoard('空白看板')
  await waitFor(() => expect(server.boards).toHaveLength(3))
  expect(screen.getByRole('combobox', { name: '同图叠加' })).toHaveValue('')

  await openBoards()
  click('删除看板')
  click('取消')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await openBoards()
  click('删除看板')
  click('确认删除')
  await waitFor(() => expect(server.boards).toHaveLength(2))
  await openBoards()
  expect(screen.getByRole('status')).toHaveTextContent('已保存')
})

it('allows cancel, discard, or save when switching a dirty board', async () => {
  render(<Harness />)
  await screen.findByRole('combobox', { name: '同图叠加' })
  await openBoards()
  click('另存为新看板')
  nameBoard('第二看板')
  await waitFor(() => expect(server.boards).toHaveLength(2))
  fireEvent.change(screen.getByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'INE:SC.MAIN' },
  })
  const selectFirst = async () => {
    await openBoards()
    fireEvent.click(screen.getByRole('option', { name: '默认看板' }))
  }
  await selectFirst()
  click('取消')
  expect(trigger()).toHaveAttribute('aria-label', '看板：第二看板')

  await selectFirst()
  click('保存并切换')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(server.boards[1].config.comparison).toBe('INE:SC.MAIN')

  fireEvent.change(screen.getByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'SHFE:AU.MAIN' },
  })
  await openBoards()
  fireEvent.click(screen.getByRole('option', { name: '第二看板' }))
  click('放弃修改并切换')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(server.boards[0].config.comparison).toBeNull()
  expect(screen.getByRole('combobox', { name: '同图叠加' })).toHaveValue('INE:SC.MAIN')

  await openBoards()
  fireEvent.click(screen.getByRole('option', { name: '默认看板' }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('keeps switching dialog open when saving fails', async () => {
  render(<Harness />)
  await screen.findByRole('combobox', { name: '同图叠加' })
  await openBoards()
  click('另存为新看板')
  nameBoard('第二看板')
  await waitFor(() => expect(server.boards).toHaveLength(2))
  fireEvent.change(screen.getByRole('combobox', { name: '同图叠加' }), {
    target: { value: 'INE:SC.MAIN' },
  })
  vi.mocked(updateChartBoard).mockRejectedValueOnce(new Error('服务维护中，请稍后重试'))
  await openBoards()
  fireEvent.click(screen.getByRole('option', { name: '默认看板' }))
  click('保存并切换')
  await waitFor(() =>
    expect(within(screen.getByRole('dialog')).getByRole('alert')).toHaveTextContent('服务维护中'),
  )
  expect(screen.getByRole('dialog')).toBeVisible()
})

it('keeps keyboard focus in the dialog and Escape cancels without a write', async () => {
  render(<Harness />)
  await screen.findByRole('combobox', { name: '同图叠加' })
  await openBoards()
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
