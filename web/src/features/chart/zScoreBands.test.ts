import { expect, it, vi } from 'vitest'
import { zScoreBands } from './zScoreBands'
it('draws sigma zones in current media coordinates and skips absent coordinates', () => {
  let scale = 10
  const coordinate = vi.fn((p: number) => 50 - p * scale)
  const bands = zScoreBands(coordinate)
  const view = bands.paneViews!()[0]
  expect(view.zOrder!()).toBe('bottom')
  const context = { fillStyle: '', fillRect: vi.fn() }
  const target = { useMediaCoordinateSpace: (fn: (scope: unknown) => void) => fn({ context, mediaSize: { width: 500 } }) }
  view.renderer()!.draw(target as never)
  expect(context.fillRect.mock.calls).toEqual([[0, 30, 500, 10], [0, 60, 500, 10]])
  scale = 5
  view.renderer()!.draw(target as never)
  expect(context.fillRect).toHaveBeenLastCalledWith(0, 55, 500, 5)
  coordinate.mockImplementation(() => null as never)
  context.fillRect.mockClear()
  view.renderer()!.draw(target as never)
  expect(context.fillRect).not.toHaveBeenCalled()
})
