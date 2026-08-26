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
 * `ManaProduced` is the third: **one** mark for what a permanent taps for,
 * which is a different question from a cost and has to be drawn differently.
 * Its argument is at the bottom of the file.
 */

import { useState, type ReactNode } from 'react'
import { GLYPH_PATH, hasGlyph } from '../lib/managlyphs'
import { COLOR_VAR, producedSymbol } from '../lib/mtg'

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

/* ------------------------------------------ what a permanent taps for */

/**
 * One wedge of the prism, as an SVG path in a 0–100 box.
 *
 * Starts at twelve o'clock and sweeps clockwise, so a five-wedge disc puts
 * white at the top — the same place the eye starts on a colour identity
 * anywhere else in this app.
 */
function wedge(i: number, n: number): string {
  const r = 50
  const at = (k: number) => {
    const a = (k / n) * 2 * Math.PI - Math.PI / 2
    return `${(50 + r * Math.cos(a)).toFixed(3)} `
      + `${(50 + r * Math.sin(a)).toFixed(3)}`
  }
  // A single colour never reaches here — it has an official symbol of its own
  // — so no wedge is ever the whole circle and every arc has two distinct ends.
  return `M50 50 L${at(i)} A${r} ${r} 0 ${1 / n > 0.5 ? 1 : 0} 1 ${at(i + 1)} Z`
}

/**
 * **What this permanent taps for**, as one mark.
 *
 * A cost is a row of pips because a cost *is* a row of things you pay. What a
 * land or a mana creature taps for is one mana with a choice attached, so it
 * is one mark: the official pip for a single colour, the official **hybrid**
 * symbol for a pair — Magic's own glyph for "or" — and for three or more a
 * prism, a coin cut into a wedge per colour. `lib/mtg`'s `producedSymbol`
 * argues which, and why a Temple Garden must never be drawn `{G}{W}`.
 *
 * The prism is inked here rather than fetched because there is no official
 * symbol for it: nothing in the set means "any of these five". It is the same
 * five fixed colours every disc in this app uses, cut the way a gem is.
 *
 * **It says what the printing does, never what this game's pool received** —
 * see `forgeBoardCard.Makes` in Go for why nothing on that pipe can say the
 * second thing.
 */
export function ManaProduced({ colors, size = 15 }: {
  colors: string[]
  size?: number
}) {
  const sym = producedSymbol(colors)
  if (!colors.length) return null

  if (sym === null) {
    return (
      <span
        className="inline-flex items-center justify-center overflow-hidden rounded-full"
        style={{ width: size, height: size,
          boxShadow: '0 0 0 1px var(--hairline)' }}
      >
        <svg viewBox="0 0 100 100" width={size} height={size}
             aria-hidden focusable="false" style={{ display: 'block' }}>
          {colors.map((c, i) => (
            <path key={c} d={wedge(i, colors.length)}
                  fill={COLOR_VAR[c] ?? 'var(--mtg-c)'}
                  // **The cut, and it is what makes this read.** Magic's five
                  // are all pale, so five of them meeting edge to edge at
                  // fifteen pixels is one light smudge — measured on a real
                  // board before it was believed. A dark line down each seam
                  // turns the smudge into a cut coin, which is a shape the eye
                  // resolves at any size.
                  stroke="rgba(10, 8, 5, 0.55)" strokeWidth={5} />
          ))}
        </svg>
      </span>
    )
  }

  // The hybrid's two halves, split on the same diagonal the official symbol
  // is. Only ever a loading placeholder and a fallback: when the real glyph
  // arrives it covers this entirely, disc and all.
  const [left, right] = sym.split('/')
  const disc = right !== undefined
    ? `linear-gradient(135deg, ${COLOR_VAR[left ?? ''] ?? 'var(--mtg-c)'} 50%,`
      + ` ${COLOR_VAR[right] ?? 'var(--mtg-c)'} 50%)`
    : COLOR_VAR[sym] ?? 'var(--mtg-c)'
  return (
    <span
      className="inline-flex items-center justify-center overflow-hidden rounded-full"
      style={{ width: size, height: size, background: disc,
        boxShadow: '0 0 0 1px var(--hairline)' }}
    >
      <OfficialSymbol
        symbol={sym}
        size={size}
        // A pair has no drawn glyph of its own, and the split disc underneath
        // is already saying "one of these two", which is the whole claim.
        fallback={hasGlyph(sym)
          ? <ManaGlyph symbol={sym} size={size * 0.74} />
          : null}
      />
    </span>
  )
}
