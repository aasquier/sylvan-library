/**
 * One mana symbol, drawn.
 *
 * The shapes and the argument for drawing rather than fetching them live in
 * `lib/managlyphs.ts`; this is only the element. Split because a module that
 * exports both a component and its data breaks fast refresh, which `oxlint`
 * enforces.
 */

import { GLYPH_PATH } from '../lib/managlyphs'

/**
 * One glyph, inked on whatever is behind it.
 *
 * No disc of its own: `Pip` owns the coloured circle, and drawing a second one
 * here would put two rounded shapes a pixel apart. `ink` is fixed dark rather
 * than themed, for the reason the pentagram's letters were — the five discs
 * are the same pale colours in both themes, so ink that followed the theme
 * would turn white on cream.
 */
export function ManaGlyph({ symbol, size, ink = '#141414' }: {
  symbol: string
  size: number
  ink?: string
}) {
  const glyph = GLYPH_PATH[symbol]
  if (!glyph) return null
  return (
    <svg
      viewBox="0 0 100 100"
      width={size}
      height={size}
      aria-hidden
      focusable="false"
      style={{ display: 'block' }}
    >
      <path d={glyph.d} fill={ink} fillRule={glyph.evenOdd ? 'evenodd' : 'nonzero'} />
    </svg>
  )
}
