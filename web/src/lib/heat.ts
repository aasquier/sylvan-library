/**
 * Colours for the castability heatmap.
 *
 * In `lib/` rather than beside the component that uses them, because oxlint's
 * fast-refresh rule runs with `--deny-warnings` and a non-component export
 * from a `.tsx` fails the lint. Same reason `lib/motion.ts` and
 * `lib/theater.ts` exist; see the note in either.
 *
 * Two rules govern what these return, and both are accessibility rather than
 * taste. **The wash is never the only signal** — every cell in the grid prints
 * its own number on top, so the colour is a way to find the shape at a glance
 * and never the way to read a value. And **the ink is chosen against the wash
 * it sits on**, not against the page, so the digits stay legible at both ends
 * of the ramp and in either theme.
 */

/** Clamp to the unit interval. A rounded probability can arrive at 1.0001. */
function unit(value: number): number {
  return Math.max(0, Math.min(1, value))
}

/**
 * The cell background for a probability.
 *
 * Ramps through the surface's own accent rather than red-to-green: a
 * red/green ramp is the single most common way to build a chart that a
 * red-green colourblind reader cannot use, and the deck's own numbers are not
 * good-versus-bad anyway — a 40% on turn 2 for an eight-drop is neither.
 * Low values stay near the page so the grid reads as mostly empty where the
 * deck is mostly unable, which is the honest shape.
 */
export function heatWash(odds: number): string {
  const value = unit(odds)
  if (value < 0.005) return 'transparent'
  // Perceptual-ish easing: the interesting band is 40-95%, and a linear ramp
  // spends most of its range on differences nobody is deciding anything on.
  const eased = Math.pow(value, 0.75)
  return `color-mix(in srgb, var(--heat-ink) ${(eased * 62).toFixed(1)}%, transparent)`
}

/** Ink for a cell, dark on a pale wash and pale once the wash is strong. */
export function heatInk(odds: number): string {
  return unit(odds) > 0.62 ? 'var(--heat-on)' : 'var(--text-primary)'
}

/**
 * Colour for a lag figure.
 *
 * `null` means the card never becomes reliable inside the horizon, which is
 * the loudest thing the table can say; zero or one turn behind is ordinary
 * and deliberately unmarked, because colouring every row teaches nothing.
 */
export function lagTone(lag: number | null): string {
  if (lag === null) return 'var(--status-critical)'
  if (lag >= 3) return 'var(--status-critical)'
  if (lag >= 2) return 'var(--status-warning)'
  return 'var(--text-secondary)'
}

/**
 * The integer a heatmap cell prints.
 *
 * Floored, not rounded, and that is a correctness choice rather than a
 * rounding preference. The lag column calls a card reliable at 90%, so a cell
 * holding 0.8996 must not print "90" beside a lag of "never" — a reader
 * comparing the two would be looking at a screen contradicting itself.
 * Flooring makes the display and the verdict the same statement: a cell reads
 * 90 exactly when its odds have actually reached the bar.
 */
export function heatPercent(odds: number): string {
  const value = unit(odds)
  return value < 0.005 ? '·' : String(Math.floor(value * 100))
}
