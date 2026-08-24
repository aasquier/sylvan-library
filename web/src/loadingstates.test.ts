/**
 * A page that is waiting says so by moving.
 *
 * Commandment 6 asks for a site that is alive rather than one that sits there,
 * and the place that promise is quietly broken is the null-guard at the top of
 * a component: `if (!thing) return <p>Loading…</p>` is one line, reads as
 * finished, and renders a page holding perfectly still. Five of them had
 * accumulated by 2026-08-24 — the fortune-teller's table waiting on its
 * readers, the theme interview, the new-deck colour guide and the glossary —
 * and every one of them was a surface a newcomer meets first.
 *
 * The measurement is why this is a guard rather than a preference: those four
 * endpoints answer in **0.6–0.7ms of server work**. There is nothing to make
 * faster. The entire wait a person experiences is the round trip plus the page
 * choosing to sit still through it, so the only thing left to improve is what
 * the page does while it waits — which is `Spinner`'s whole job, and `Spinner`
 * is the one named place where that motion is defined.
 *
 * **This checks JSX text, not source text.** The pattern is a `>` and a `<`
 * with the word between them and no tag, brace or attribute in the way, so a
 * comment that says "still loading" and a prop named `loading` are both
 * invisible to it, and `<Spinner label="Loading…" />` cannot match by
 * construction — the label lives inside the tag. That is the conservatism
 * that keeps a guard from crying wolf, and it costs only the case nobody
 * writes: a wait announced through an interpolated variable.
 */

import { describe, expect, it } from 'vitest'

/** Every component's source, read through Vite rather than the filesystem:
 *  `tsconfig.app.json` pins `types` to `vite/client`, so `node:fs` does not
 *  typecheck here and this is the reader that does. */
const sources = import.meta.glob('./**/*.tsx', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

/** A JSX text node announcing a wait: a tag's `>`, the word, and the next
 *  `<`, with nothing between them that only code or a comment would contain.
 *  The excluded set is what makes it a *text node* rather than any two angle
 *  brackets in the file — `/` and `*` end a comment's reach, `(){}=;` end an
 *  expression's — and the price is the case nobody writes: a wait announced
 *  with a slash or a bracket in the sentence. It can miss; it cannot invent. */
const stillWaiting =
  /> *[^<>{}()/*=;]*\b(loading|please wait|one moment)\b[^<>{}()/*=;]* *</gi

describe('a page that is waiting', () => {
  it('never says so by standing still', () => {
    const offenders: string[] = []
    for (const [path, body] of Object.entries(sources)) {
      if (path.includes('.test.')) continue
      for (const m of body.matchAll(stillWaiting)) {
        offenders.push(`${path}: ${m[0].trim()}`)
      }
    }
    // The message is the fix, because the fix is always the same one.
    expect(offenders, 'a wait rendered as static text; use <Spinner label="…" />'
      + ' from components/ui, which is the one place the motion is defined')
      .toEqual([])
  })

  // Anti-vacuity, both halves. A glob that resolved to nothing and a pattern
  // that stopped matching would each produce an empty offender list, which is
  // indistinguishable from a clean tree.
  it('is checked against every component, by a pattern that still matches', () => {
    expect(Object.keys(sources).length).toBeGreaterThan(40)
    const control = `<p className="text-sm" style={{ color: 'x' }}>Loading…</p>`
    expect([...control.matchAll(stillWaiting)]).toHaveLength(1)
    // And the shape it must *not* flag, or the guard would ban the cure.
    expect([...`<Spinner label="Loading…" />`.matchAll(stillWaiting)]).toHaveLength(0)
  })
})
