/**
 * A control that starts work stops accepting the press.
 *
 * Commandment 17's least visible clause. A button that fires a request and
 * goes on listening answers a second press by doing the thing twice — two
 * recorded edits on a write, a swallowed refusal on a read — and in both cases
 * the person pressing sees *nothing happen*, which on a slow connection is
 * indistinguishable from a click that missed. So they press again.
 *
 * **This is a guard because the count has been taken by hand four times and
 * moved every time**: 19 async controls / 6 undisabled on 2026-08-23, 21/7 on
 * 08-24, a re-measure on 09-05, and — this file, run against the tree rather
 * than read off it — **46 async controls of 217 buttons, 9 undisabled**. Four
 * hand counts of the same invariant is the signal that it wants a test rather
 * than another sweep, and the hand counts were low every time because they
 * were keyed on the click's own words: half the offenders never say `async`
 * at the call site. The two writes that actually cost something (#436) were
 * found by the first sweep and fixed by the third, which is a long time for a
 * double-click to be two recorded edits. With `signOut` fixed the nine are
 * eight, and all eight are named below with their reasons.
 *
 * **The offenders are discovered; the exemptions are declared.** Every
 * `<button>` in the tree is in scope automatically, so a control written
 * tomorrow is measured without anybody remembering to add it. What cannot be
 * derived is *why* a particular control is fine without `disabled` — a
 * clipboard copy is idempotent, an unmounting control cannot be pressed twice
 * — so those are written below with their reasons, and a third test fails if
 * one of them stops matching anything. A permission that has outlived its
 * control is how an allowlist turns into a hole.
 *
 * **What it deliberately does not do.** It reads `disabled` in the tag and
 * nothing about the label beside it, so it cannot tell the house's both-halves
 * pattern (a `disabled` *and* a spoken pending state) from the disabling half
 * alone. That distinction lives in the per-control tests — `App.test.tsx`'s
 * sign-out, `DeckDetail.test.tsx`'s two writes — because it is about what a
 * particular control says, which is a sentence and not a shape.
 */

import { describe, expect, it } from 'vitest'
import { openTags, type FoundTag } from './tagscan'

/** Every component in the app, as source text. Tests are skipped by
 *  `openTags`; the glob's keys are relative to this file. */
const sources = import.meta.glob('./**/*.tsx', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>

/** Async work started inline in the handler itself. `void f()` is the house
 *  idiom for "fire this promise and do not await it", which is exactly the
 *  shape that leaves the button live. */
const INLINE_ASYNC = /void\s+[A-Za-z_$]|async\s*\(|\.then\s*\(|await\s/

/**
 * The names declared `async` in one file.
 *
 * Half the offenders never say `async` at the call site — `onClick={() =>
 * choose(c)}` looks synchronous and starts a card search — so a scan keyed on
 * the tag's own words misses them. That is not a hypothetical either: the
 * 08-24 census was taken that way and walked past both of `NewDeck`'s.
 * Cross-file handlers are still missed, which this can live with: a guard may
 * miss, it may not invent.
 */
function asyncNames(src: string): Set<string> {
  const names = new Set<string>()
  for (const re of [
    /async\s+function\s+([A-Za-z_$][\w$]*)/g,
    /(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]*)?=\s*(?:useCallback\(\s*)?async\b/g,
    /([A-Za-z_$][\w$]*)\s*:\s*async\b/g,
  ]) {
    for (const m of src.matchAll(re)) if (m[1]) names.add(m[1])
  }
  return names
}

/** The `onClick={…}` expression, or '' when the tag has no click handler. */
function clickExpr(tag: string): string {
  const at = tag.indexOf('onClick={')
  if (at < 0) return ''
  let depth = 0
  for (let i = at + 'onClick='.length; i < tag.length; i++) {
    if (tag[i] === '{') depth++
    else if (tag[i] === '}' && --depth === 0) {
      return tag.slice(at + 'onClick={'.length, i)
    }
  }
  return ''
}

/** Every button whose click starts something that finishes later. */
function asyncControls(): FoundTag[] {
  return openTags(sources, 'button').filter((b) => {
    const expr = clickExpr(b.tag)
    if (!expr) return false
    if (INLINE_ASYNC.test(expr) || /\bapi\.[A-Za-z_$]/.test(expr)) return true
    const named = asyncNames(b.source)
    return [...expr.matchAll(/\b([A-Za-z_$][\w$]*)\b/g)]
      .some((m) => m[1] !== undefined && named.has(m[1]))
  })
}

/**
 * The controls that start work and are right not to disable, and why.
 *
 * `call` is matched against the tag, so an entry survives the file moving and
 * fails when the handler it names is renamed or removed — which is what makes
 * a permission expire with the thing it was granted for.
 */
const ALLOWED: { where: string; call: string; why: string }[] = [
  {
    where: 'src/components/artifacts.tsx', call: 'copy()',
    why: 'A clipboard copy: idempotent, local, and it answers the press with '
       + 'a `Copied` label. Disabling a copy button is the convention nowhere.',
  },
  {
    where: 'src/components/tokens.tsx', call: 'take()',
    why: 'The token shopping list, same shape as the copy above — and it '
       + 'speaks its own `role="status"` sentence besides.',
  },
  {
    where: 'src/routes/Coliseum.tsx', call: 'clip.writeText',
    why: 'Copies this page’s address. Same category again; the confirmation '
       + 'is deliberately temporary so the button goes back to saying what '
       + 'it does.',
  },
  {
    where: 'src/routes/Admin.tsx', call: 'gather()',
    // Re-derived 2026-09-05 rather than copied: the reason carried in the
    // 09-05 Red ledger entry was that "a second POST follows the first
    // instead of starting another", and the handler says otherwise —
    // `refreshLibrary` in `internal/api/upkeep.go` answers 409 with the
    // running job's id, which the client turns into an error message. The
    // control is still fine, for a different reason, and the difference
    // matters: an exemption resting on a server behaviour would have to be
    // re-checked whenever that handler moves.
    why: 'Unmounts on the press. `gather()` sets `asking` false as its first '
       + 'statement, before it awaits anything, and the button renders only '
       + 'in the `asking` branch — so the second click has nothing under it. '
       + 'Same category as NewDeck’s two below.',
  },
  {
    where: 'src/components/dossier.tsx', call: 'load()',
    why: 'Latches on `fetched`, so the second press has nothing to fetch. '
       + 'The button is also a disclosure, whose job is to keep answering.',
  },
  {
    where: 'src/routes/Research.tsx', call: 'ask(example)',
    why: 'The example slips are rendered only while nothing is being asked '
       + '(`!report && !busy`), so a press removes the control that made it. '
       + '`ask` self-guards on `busy` as well.',
  },
  {
    where: 'src/routes/NewDeck.tsx', call: 'choose(c)',
    why: 'Picking a colour combination sets `chosen`, and the whole step it '
       + 'lives in renders on `!chosen` — the grid is gone before a second '
       + 'press could land.',
  },
  {
    where: 'src/routes/NewDeck.tsx', call: 'choose(current)',
    why: 'The guided door’s “Build …”, same unmount as the grid above.',
  },
]

function isAllowed(control: FoundTag): boolean {
  return ALLOWED.some((a) => control.where.startsWith(`${a.where}:`)
    && control.tag.includes(a.call))
}

describe('a control that starts async work', () => {
  it('is found at all, so this guard cannot pass by matching nothing', () => {
    // Forty-six at the time of writing, across 217 buttons. The floor is
    // deliberately far below that: this asserts the scanner still reads the
    // tree, not that the count is frozen. It is not idle — a lazy tag matcher
    // takes it to 11, which is how the mutation run found that the guard
    // would otherwise have reported a clean sweep of half a tree.
    expect(asyncControls().length,
      'the scanner found almost no async controls — suspect the tag matcher '
      + 'or the glob before believing the app has none')
      .toBeGreaterThan(15)
  })

  it('stops accepting the press while the work is out', () => {
    const listening = asyncControls()
      .filter((b) => !b.tag.includes('disabled'))
      .filter((b) => !isAllowed(b))
      .map((b) => b.where)

    expect(listening,
      'commandment 17: a control answers the hand that reaches for it, and a '
      + 'button that fires a request and keeps listening answers a second '
      + 'press by doing the thing twice. The house pattern is a busy flag '
      + 'driving `disabled` *and* a visible pending state — both halves or '
      + 'neither. `App.tsx`’s sign-out and `DeckDetail.tsx`’s two writes are '
      + 'the shapes to copy. If this control genuinely does not need it, say '
      + 'why in `ALLOWED` above rather than here.')
      .toEqual([])
  })

  it('has no permission left over from a control that has gone', () => {
    const controls = asyncControls()
    const stale = ALLOWED
      .filter((a) => !controls.some((b) => b.where.startsWith(`${a.where}:`)
        && b.tag.includes(a.call)))
      .map((a) => `${a.where} — ${a.call}`)

    expect(stale,
      'an exemption that matches nothing is a hole waiting for the next '
      + 'control to fall into it: the handler was renamed, moved or deleted, '
      + 'and the permission outlived the reason it was granted. Delete the '
      + 'entry, or point it at where the control went.')
      .toEqual([])
  })
})
