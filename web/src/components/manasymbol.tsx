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
 *
 * `ManaPip` is the third: **one mana that exists**, drawn once per mana, which
 * is what a *pool* is made of. Its argument is at the bottom of the file.
 *
 * **A fourth used to live here and does not any more.** `ManaProduced` drew
 * one mark for what a permanent *taps for* — the official pip for a colour,
 * the hybrid for a pair, a cut-coin prism for three or more — and its only
 * caller was the bead on a turned permanent, which #341 took off the card. The
 * reason it went rather than being left exported and unused is written where
 * the bead was, in `components/board.tsx`; the short version is that a bead
 * beside a live mana pool starts implying the one thing ADR 44 refuses to say.
 * If it is ever wanted again it is one revert away and its argument is intact
 * in the history — a dead exported component is a worse thing to leave behind
 * than a deleted one.
 */

import { useState, type ReactNode } from 'react'
import { GLYPH_PATH, hasGlyph } from '../lib/managlyphs'
import { COLOR_VAR } from '../lib/mtg'

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

/* ------------------------------------------------ one mana that exists */

/**
 * **One mana in a pool**, as a pip.
 *
 * The disc is painted first and the official symbol lands on top of it, and
 * that ordering is doing real work: the pips arrive on a beat and are gone inside a second, so a symbol
 * that popped in a frame late would read as a stutter rather than as mana. The
 * disc is right the instant it mounts and never changes shape when the picture
 * catches up.
 *
 * `size` is the pip's diameter; everything else about how it moves belongs to
 * whoever is drawing the row, because a pip filling a pool and a pip flashing
 * in the middle of the arena are the same object doing two different things.
 */
export function ManaPip({ symbol, size = 14 }: {
  symbol: string
  size?: number
}) {
  const code = symbol.toUpperCase()
  return (
    <span
      className="mana-pip"
      style={{ width: size, height: size,
        // A letter nothing recognises still gets a disc, so a pipe that
        // changed under us shows something odd rather than showing nothing —
        // an empty pool and a mis-read one otherwise look identical, which is
        // the exact bug this wire has already had once.
        background: COLOR_VAR[code] ?? 'var(--mtg-c)' }}
    >
      <OfficialSymbol
        symbol={code}
        size={size}
        fallback={hasGlyph(code)
          ? <ManaGlyph symbol={code} size={size * 0.74} />
          : null}
      />
    </span>
  )
}
