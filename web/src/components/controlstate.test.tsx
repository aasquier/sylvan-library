/** Every control that is a place or a setting says which one it is on.
 *
 * Commandment 20's second half. The first half — "a toggle is a button, not a
 * link" — is the one that got written down and enforced; this is the clause
 * right after it, *"it says which state it is in (`aria-expanded` for a
 * disclosure, `aria-pressed` for a setting)"*, and nothing checked it.
 *
 * Found by walking the deployed deck page on 2026-08-30: all six tabs came
 * back `role: null, aria-selected: null`, so the strip drew a highlight that
 * only a sighted person could read and named no current tab at all. The
 * `.disclosure-toggle` family had it right on the same page (five of five),
 * which is what made the gap legible — one house pattern honoured in one
 * place and forgotten in another is exactly the kind of drift a guard is
 * for, and `Coliseum` had already written the correct shape down.
 *
 * **The list is discovered, not written here.** `import.meta.glob` walks every
 * source in `src/`, so a control added tomorrow is in this test's scope
 * without anybody remembering to add it — the property that makes the door's
 * route sweeps trustworthy, applied to markup. A hand-kept list would go
 * stale the first week and then read as coverage.
 *
 * It deliberately fails when it finds *nothing* to check, too. A scanner
 * whose regex has quietly stopped matching passes silently forever, which is
 * worse than no guard: it reports safety it never measured.
 */

import { describe, expect, it } from 'vitest'

/** Every `.tsx` in the app, as source text. Tests excluded by the glob. */
const sources = import.meta.glob('../**/*.tsx', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>

/** The classes that mark a control as a place or a setting rather than an
 *  action. `.btn` is deliberately absent: an action's label says what it does
 *  and it has no state to announce. */
const STATEFUL = /chip-toggle|strip-tab/

/** What counts as saying so. `aria-current` is the odd one and is allowed
 *  because `Admin`'s ward strip uses it and it does announce the current
 *  item; the point of this guard is that *something* speaks, not that every
 *  strip picks the same word. */
const SAYS_STATE = /aria-(selected|pressed|expanded|current)/

/**
 * Finds the end of a JSX opening tag.
 *
 * Written rather than regexed because `onClick={() => setTab(t.id)}` contains
 * a `>` and a lazy `[\s\S]*?>` stops on the arrow — which is not a hypothetical
 * failure mode, it is the one this file's first draft had, and it silently
 * reported four of the six real sites as clean.
 */
function tagEnd(src: string, from: number): number {
  let depth = 0
  let inTick = false
  for (let i = from; i < src.length; i++) {
    const c = src[i]
    if (c === '`') inTick = !inTick
    if (inTick) continue
    if (c === '{') depth++
    else if (c === '}') depth--
    else if (c === '>' && depth === 0 && src[i - 1] !== '=') return i
  }
  return -1
}

/** Every `<button>` in the tree whose class marks it a place or a setting. */
function statefulButtons() {
  const found: { where: string; tag: string }[] = []
  for (const [path, src] of Object.entries(sources)) {
    if (path.includes('.test.')) continue
    const re = /<button\b/g
    let m: RegExpExecArray | null
    while ((m = re.exec(src))) {
      const end = tagEnd(src, m.index + '<button'.length)
      if (end < 0) continue
      const tag = src.slice(m.index, end + 1)
      if (!STATEFUL.test(tag)) continue
      const line = src.slice(0, m.index).split('\n').length
      found.push({ where: `${path.replace('../', 'src/')}:${line}`, tag })
    }
  }
  return found
}

describe('a control that is a place or a setting', () => {
  it('is found at all, so this guard cannot pass by matching nothing', () => {
    const all = statefulButtons()
    // Eighteen at the time of writing. The floor is deliberately well under
    // that: this asserts the scanner still works, not that the count is
    // frozen — a real number here would fail every time somebody adds a tab.
    expect(all.length, 'the scanner found no stateful controls at all — '
      + 'suspect the tag matcher before believing the app has none')
      .toBeGreaterThan(8)
  })

  it('says which state it is in', () => {
    const silent = statefulButtons()
      .filter((b) => !SAYS_STATE.test(b.tag))
      .map((b) => b.where)

    expect(silent, 'commandment 20: a control that changes what is in front of '
      + 'you says which state it is in. `aria-selected` for a tab, '
      + '`aria-pressed` for a setting, `aria-expanded` for a disclosure. '
      + '`Coliseum.tsx` and `Library.tsx` are the shapes to copy.')
      .toEqual([])
  })
})
