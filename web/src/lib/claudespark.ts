/**
 * The spark, serialized — Claude's mark as an image URL.
 *
 * `ClaudeMark` in `components/tarot.tsx` draws this same spark as live SVG
 * for the persona grid: a warm light on the night the card backs are printed
 * on, radiating the concentric rings a voice makes. The About Claude page
 * wears it as its masthead, and `PageMasthead` (and the `SceneBackdrop` it
 * owns) take an `img src` rather than a component — so this file is that
 * drawing written out as a data URI. Same night, same rings, same rays; the
 * geometry constants below are the tile's, copied by hand and cross-checked
 * against `tarot.tsx` in review rather than imported, because the component
 * builds JSX and this builds a string, and a shared abstraction over those
 * two would be bigger than both.
 *
 * It is the one image in the app that owes nobody a credit line: drawn for
 * this library, not borrowed from a card, so rule 5 has nothing to gate and
 * no artist goes unnamed. Fixed gradient ids are safe here where the tile
 * needed `useId` — a data URI is its own document, and nothing else lives
 * in it.
 */

/** Two decimals is plenty at masthead sizes, and keeps the URI short. */
const f = (n: number) => String(Number(n.toFixed(2)))

const STARS: Array<[number, number, number]> = [
  [62, 70, 1.6], [140, 330, 1.2], [210, 96, 1.1], [318, 40, 1.4],
  [430, 88, 1.2], [538, 150, 1.6], [566, 330, 1.1], [468, 396, 1.3],
  [96, 210, 1.0], [246, 402, 1.2], [388, 372, 1.0], [520, 244, 1.0],
]

const stars = STARS
  .map(([x, y, r]) => `<circle cx="${x}" cy="${y}" r="${r}" opacity="0.55"/>`)
  .join('')

const rings = [74, 118, 166, 220]
  .map((r, i) =>
    `<circle cx="313" cy="222" r="${r}" fill="none" stroke="#f0a67a"`
    + ` stroke-width="${f(1.6 - i * 0.3)}" opacity="${f(0.34 - i * 0.07)}"/>`)
  .join('')

const rays = Array.from({ length: 12 }, (_, i) => {
  const a = (i * Math.PI * 2) / 12 - Math.PI / 2
  const inner = 26
  const outer = i % 2 === 0 ? 74 : 50
  return `<line x1="${f(Math.cos(a) * inner)}" y1="${f(Math.sin(a) * inner)}"`
    + ` x2="${f(Math.cos(a) * outer)}" y2="${f(Math.sin(a) * outer)}"`
    + ` stroke="${i % 2 === 0 ? '#f7d9b8' : '#e8956d'}"`
    + ` stroke-width="${i % 2 === 0 ? 7 : 4.5}"`
    + ` stroke-linecap="round" opacity="0.92"/>`
}).join('')

/* Explicit width/height give the image an intrinsic size, so the masthead
 * lays it out exactly as it does a Scryfall art crop (626x457). */
const SPARK_SVG =
  '<svg xmlns="http://www.w3.org/2000/svg" width="626" height="457"'
  + ' viewBox="0 0 626 457">'
  + '<defs>'
  + '<radialGradient id="night" cx="50%" cy="42%" r="75%">'
  + '<stop offset="0%" stop-color="#2c3f6b"/>'
  + '<stop offset="60%" stop-color="#1b2647"/>'
  + '<stop offset="100%" stop-color="#101830"/>'
  + '</radialGradient>'
  + '<radialGradient id="halo" cx="50%" cy="50%" r="50%">'
  + '<stop offset="0%" stop-color="rgba(255,236,210,0.95)"/>'
  + '<stop offset="30%" stop-color="rgba(240,166,122,0.55)"/>'
  + '<stop offset="70%" stop-color="rgba(218,119,86,0.18)"/>'
  + '<stop offset="100%" stop-color="rgba(218,119,86,0)"/>'
  + '</radialGradient>'
  + '</defs>'
  + '<rect width="626" height="457" fill="url(#night)"/>'
  + `<g fill="#f0e4c2">${stars}</g>`
  + rings
  + '<circle cx="313" cy="222" r="150" fill="url(#halo)"/>'
  + `<g transform="translate(313 222)">${rays}`
  + '<circle r="19" fill="#fbe8d0"/>'
  + '<circle r="9" fill="#fff6ea"/>'
  + '</g>'
  + '</svg>'

export const CLAUDE_SPARK_URI =
  `data:image/svg+xml,${encodeURIComponent(SPARK_SVG)}`
