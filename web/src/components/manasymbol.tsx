/**
 * Mana symbols: the official art first, our own ink as the fallback.
 *
 * `OfficialSymbol` is ADR 33's client half — the real Scryfall-drawn symbol,
 * served from this app's own origin by `/api/symbols/{code}.svg`, which fills
 * a runtime cache on first ask and never puts a byte in git. `ManaGlyph` is
 * the hand-drawn five from PR #61, kept as the fallback so a cold cache with
 * no network still shows a sun rather than a broken-image square; the shapes
 * and their history live in `lib/managlyphs.ts`. Split from the data module
 * because a file exporting both a component and its data breaks fast
 * refresh, which `oxlint` enforces.
 */

import { useState, type ReactNode } from 'react'
import { GLYPH_PATH } from '../lib/managlyphs'

/**
 * Codes the symbol route has refused this session, remembered at module
 * level so one offline render of a 99 does not fire ~200 doomed requests
 * and re-fire them on every scroll. A symbol that comes back (the network
 * returns) is one reload away, which is what a browser already means by
 * "try again".
 */
const FAILED = new Set<string>()

/**
 * One official symbol as an image, with the caller's fallback behind it.
 *
 * The URL code is the braced symbol with its punctuation dropped — {W/U}
 * is WU — mirroring the server's own shape check, so nothing malformed is
 * ever asked for. The image is the *entire* symbol, coloured disc included;
 * callers keep their own disc behind it as the loading placeholder.
 */
export function OfficialSymbol({ symbol, size, fallback }: {
  symbol: string
  size: number
  fallback?: ReactNode
}) {
  const code = symbol.replace(/\//g, '').toUpperCase()
  const [failed, setFailed] = useState(false)
  if (failed || FAILED.has(code) || !/^[0-9A-Z]{1,10}$/.test(code)) {
    return <>{fallback ?? null}</>
  }
  return (
    <img
      src={`/api/symbols/${code}.svg`}
      // The wrapper pip owns the accessible name; the image is decoration
      // inside it, exactly as the drawn glyph was.
      alt=""
      width={size}
      height={size}
      draggable={false}
      loading="lazy"
      style={{ display: 'block', width: size, height: size }}
      onError={() => { FAILED.add(code); setFailed(true) }}
    />
  )
}

/**
 * One hand-drawn glyph, inked on whatever is behind it.
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
