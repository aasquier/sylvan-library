/**
 * The site never tells a visitor the cards are on their own machine.
 *
 * This app began as a tool on one laptop and became a hosted library, and the
 * copy did not all make the journey. Four strings went on describing the card
 * shelves as the reader's — the deck library's masthead rendered *"7 decks ·
 * 35,393 cards in the local pool"* to anybody who visited an instance that was
 * emphatically not their computer — and a fifth said the printed history was
 * queried locally. Nothing failed; it was simply a small untruth told to every
 * visitor, which makes it commandment 2's problem rather than a bug.
 *
 * **The guard is absolute rather than clever, and that is the whole design.**
 * It holds those phrases to zero across every source under `src/`, comments
 * included, so there is no tokenizer to be wrong about where a string ends and
 * a comment begins — a scanner that has to decide that question is a scanner
 * that can quietly stop matching and then report safety it never measured. The
 * price is that a comment discussing this rule has to paraphrase it, which the
 * comments at the two fixed sites do. This file is exempt because it is a
 * test, and the exemption is the same one that lets a test quote the server's
 * message or the string it is correcting.
 *
 * **What it does not cover, said plainly.** It is a phrase guard: it stops
 * *these* words coming back, not the next sentence that relocates the shelves
 * in some other wording. That sentence is found by sweeping for the claim, the
 * way `CardSearch`'s was — the four-string list this file is derived from
 * walked straight past it, which is why the count moved from four to five on
 * the night it was fixed.
 *
 * The server's own gate message is out of scope here and still says it
 * (`internal/gate`): three frozen `.report.json` goldens pin that string, and
 * regenerating a frozen golden is a decision rather than a tidy-up.
 */

import { describe, expect, it } from 'vitest'

/** Every source in the app, read through Vite: `tsconfig.app.json` pins
 *  `types` to `vite/client`, so `node:fs` does not typecheck here and this is
 *  the reader that does. The same shape `loadingstates.test.ts` and
 *  `components/controlstate.test.tsx` use. */
const plain = import.meta.glob('./**/*.ts', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>
const jsx = import.meta.glob('./**/*.tsx', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>

/** The wordings that put the card shelves on the reader's machine. */
const FORBIDDEN = ['local pool', 'queried locally']

/** Everything the guard reads: the app's own sources, tests excluded. A test
 *  may legitimately quote the server's message or an old string it is
 *  correcting, and neither is copy anybody reads. */
function appSources(from: Record<string, string> = { ...plain, ...jsx }): [string, string][] {
  return Object.entries(from).filter(([path]) => !path.includes('.test.'))
}

describe('the copy a visitor reads', () => {
  // Each glob separately, because a single floor over both is not the
  // assertion it looks like: the tree holds 39 non-test `.ts` and 70 `.tsx`,
  // so either half alone clears any floor low enough to be safe — and a
  // broken `.ts` glob would then leave the whole `lib/` copy unswept while
  // this test went on reporting a real sweep. Measured, not assumed: blanking
  // one glob passed the combined form.
  it.each([['.ts', plain, 20], ['.tsx', jsx, 40]] as const)(
    'is really reading the app’s %s files, so a moved glob cannot read as a clean sweep',
    (kind, from, floor) => {
      expect(appSources(from).length,
        `the ${kind} glob found almost no sources — suspect the pattern before `
        + 'believing the app has none')
        .toBeGreaterThan(floor)
    })

  it('never places the card shelves on the reader’s own machine', () => {
    const offenders: string[] = []
    for (const [path, body] of appSources()) {
      body.split('\n').forEach((line, n) => {
        const said = line.toLowerCase()
        if (FORBIDDEN.some((phrase) => said.includes(phrase))) {
          offenders.push(`${path.replace(/^\.\//, 'src/')}:${n + 1}  ${line.trim()}`)
        }
      })
    }
    expect(offenders,
      'this instance is somebody else’s library, not the reader’s laptop. '
      + 'The house word for these cards is "the shelves" — see the masthead '
      + 'in `routes/Library.tsx` — and a name is resolved "against the '
      + 'library’s own cards".')
      .toEqual([])
  })
})
