import { useEffect, useRef, useState } from 'react'
import { comparisonOptions, type BoardConfig } from './boards'
import type { BoardController } from './useBoards'

export function BoardToolbar({
  board,
  instrument,
  captureLayout,
}: {
  board: BoardController
  instrument: string
  captureLayout?: () => Partial<BoardConfig> | undefined
}) {
  const [pending, setPending] = useState<number | null>(null)
  const [action, setAction] = useState<'new' | 'copy' | 'rename' | 'delete' | null>(null)
  const [name, setName] = useState('')
  const dialogRef = useRef<HTMLElement>(null)
  useEffect(() => {
    if (!pending && !action) return
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const dialog = dialogRef.current
    const elements = () =>
      Array.from(
        dialog?.querySelectorAll<HTMLElement>(
          'button:not(:disabled),input:not(:disabled),select:not(:disabled)',
        ) ?? [],
      ).sort((a, b) =>
        a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1,
      )
    elements()[0]?.focus()
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        setPending(null)
        setAction(null)
      }
      if (event.key === 'Tab') {
        const items = elements(),
          first = items[0],
          last = items.at(-1)
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault()
          last?.focus()
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault()
          first?.focus()
        }
      }
    }
    dialog?.addEventListener('keydown', keydown)
    return () => {
      dialog?.removeEventListener('keydown', keydown)
      previous?.focus()
    }
  }, [pending, action])
  if (!board.active || !board.config) return null
  const requestName = (next: 'new' | 'copy' | 'rename') => {
    setName(next === 'rename' ? board.active!.name : '')
    setAction(next)
  }
  const submit = async () => {
    const ok =
      action === 'rename'
        ? await board.rename(name)
        : await board.create(name, action === 'copy', captureLayout?.())
    if (ok) setAction(null)
  }
  const confirmDelete = async () => {
    if (await board.remove()) setAction(null)
  }
  return (
    <>
      <label className="comparison-control">
        同图叠加
        <select
          aria-label="同图叠加"
          disabled={board.busy}
          value={board.config.comparison ?? ''}
          onChange={(e) => board.setConfig((c) => ({ ...c, comparison: e.target.value || null }))}
        >
          <option value="">不叠加</option>
          {comparisonOptions.map(([id, label]) => (
            <option key={id} value={id}>
              {label}
            </option>
          ))}
        </select>
      </label>
      <div className="board-toolbar">
        <select
          aria-label="当前看板"
          disabled={board.busy}
          value={board.active.id}
          onChange={(e) => {
            const id = Number(e.target.value)
            if (board.dirty) setPending(id)
            else void board.select(id)
          }}
        >
          {board.boards.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </select>
        <span role="status">{board.busy ? '保存中…' : board.dirty ? '未保存' : '已保存'}</span>
        <button
          className="primary-button"
          disabled={board.busy}
          onClick={() => void board.save(captureLayout?.())}
          type="button"
        >
          保存
        </button>
        <details className="board-menu">
          <summary aria-label="看板操作">更多</summary>
          <div>
            <button
              onClick={() => requestName('new')}
              disabled={board.busy || board.boards.length >= 20 || board.dirty}
              type="button"
            >
              新建看板
            </button>
            <button
              onClick={() => requestName('copy')}
              disabled={board.busy || board.boards.length >= 20}
              type="button"
            >
              另存为新看板
            </button>
            <button onClick={() => requestName('rename')} disabled={board.busy} type="button">
              重命名
            </button>
            <button
              onClick={() => board.setConfig((c) => ({ ...c, defaultSymbol: instrument }))}
              disabled={board.busy}
              type="button"
            >
              设为默认股票
            </button>
            <button onClick={board.restore} disabled={board.busy || !board.dirty} type="button">
              恢复已保存版本
            </button>
            <button
              onClick={() => setAction('delete')}
              disabled={board.busy || board.boards.length <= 1}
              type="button"
            >
              删除看板
            </button>
            <small>看板保存在服务端。新建前请保存或恢复当前修改。</small>
          </div>
        </details>
      </div>
      {board.error && (
        <p className="board-error" role="alert">
          {board.error}
        </p>
      )}
      {pending !== null && (
        <div className="board-dialog-backdrop">
          <section
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-label="未保存的看板"
            className="board-dialog"
          >
            <h3>当前看板有未保存修改</h3>
            <p>切换前如何处理这些修改？</p>
            <button
              disabled={board.busy}
              onClick={() => void board.select(pending, true).then((ok) => ok && setPending(null))}
              type="button"
            >
              保存并切换
            </button>
            <button
              disabled={board.busy}
              onClick={() => void board.select(pending).then((ok) => ok && setPending(null))}
              type="button"
            >
              放弃修改并切换
            </button>
            <button autoFocus onClick={() => setPending(null)} type="button">
              取消
            </button>
            {board.error && <p role="alert">{board.error}</p>}
          </section>
        </div>
      )}
      {action && (
        <div className="board-dialog-backdrop">
          <section
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-label={action === 'delete' ? '删除看板' : '看板名称'}
            className="board-dialog"
          >
            {action === 'delete' ? (
              <>
                <h3>删除「{board.active.name}」？</h3>
                <p>已保存配置和当前修改都会删除。</p>
                <button disabled={board.busy} onClick={() => void confirmDelete()} type="button">
                  确认删除
                </button>
              </>
            ) : (
              <form
                onSubmit={(e) => {
                  e.preventDefault()
                  void submit()
                }}
              >
                <label>
                  看板名称
                  <input
                    autoFocus
                    aria-label="看板名称"
                    maxLength={40}
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <button disabled={!name.trim() || board.busy} type="submit">
                  确认
                </button>
              </form>
            )}
            <button onClick={() => setAction(null)} type="button">
              取消
            </button>
            {board.error && <p role="alert">{board.error}</p>}
          </section>
        </div>
      )}
    </>
  )
}
