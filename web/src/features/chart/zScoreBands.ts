import type { IPrimitivePaneRenderer, ISeriesPrimitive } from 'lightweight-charts'

/** Coordinates are resolved on every redraw, so bands follow pane scaling and resizing. */
export function zScoreBands(coordinate: (price: number) => number | null): ISeriesPrimitive {
  const renderer: IPrimitivePaneRenderer = {
    draw(target) {
      target.useMediaCoordinateSpace(({ context, mediaSize }) => {
        for (const [from, to, color] of [[1, 2, '#fb923c18'], [-1, -2, '#22d3ee18']] as const) {
          const a = coordinate(from), b = coordinate(to)
          if (a === null || b === null) continue
          context.fillStyle = color
          context.fillRect(0, Math.min(a, b), mediaSize.width, Math.abs(a - b))
        }
      })
    },
  }
  const views = [{ zOrder: () => 'bottom' as const, renderer: () => renderer }]
  return { paneViews: () => views }
}
