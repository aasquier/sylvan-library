/**
 * No technology backing this app may reach a user's eye.
 *
 * Commandment 10 names what may never render — "not languages, not databases
 * or frameworks, not seeds, not model ids, not wire tokens" — and it was
 * sharpened *after* a seed rendered on the Wheel. That one was removed, and
 * the identical thing was left standing two screens away: the simulator's
 * provenance badge read `seed 7`, its control was labelled `Seed`, `/learn`'s
 * search box offered "mulligan, ramp, seed…" as example words, and the
 * glossary entry behind the tooltip taught the word four more times. A hand
 * sweep found the first two and walked past the other two. A hand sweep is a
 * measurement, not a guard; this file is the guard — the Go crossing kept the
 * seam (`lib/claudecopy.ts`) and dropped the tripwire, and the Wheel and the
 * Simulator had each re-grown a rendered seed once already by then.
 *
 * **Scope is what renders, and only that.** `seed` is the wire field, the
 * `sim.seed` glossary key, `?seed=` on two reading URLs and half the comments
 * in `components/tarot.tsx` — all correct, all invisible, none of this file's
 * business. ADR 18's reproducibility is built on that field keeping its name.
 * So the sweep reads the two places a person actually reads in a `.tsx`
 * file: JSX text nodes, and the five attributes whose value is a sentence
 * (`label`, `title`, `placeholder`, `aria-label`, `alt`). The glossary's own
 * prose is the third rendered place, and it is swept where it lives
 * (`internal/reference`'s `TestNoGlossaryEntryTeachesATechnologyWord`).
 *
 * **What is banned is a short table, and a word earns a row by having
 * drifted, not by sounding technical.** Banning every technology-shaped word
 * would fail on `deck.yaml`, which renders on purpose — a person edits that
 * file and needs its name. Two names are allowed by ruling and no row here
 * may ever match them: *Claude*, by name and never by model id, and *Forge*,
 * the in-world name of the thing that plays the games (Aaron, 2026-08-28).
 * The control case below holds both.
 *
 * **Comments are skipped outright rather than filtered afterwards.** A JSX
 * comment is `{/* … *\/}`, so it disappears with every other brace group;
 * a line comment inside a tag is blanked first. `web/src` explains the seed
 * field at length and correctly, and a guard that made those comments
 * unwriteable would be deleted within the week.
 *
 * **The honest cost.** A text node holding `=`, `;` or a parenthesis is
 * skipped as code — TypeScript generics and arrow functions both put a `>`
 * where JSX puts one, and `useState<Mode>('mana')` is not a screen — so a
 * rendered sentence with a bracket in it goes unread. That makes this a floor
 * rather than a proof, and it is the right trade: a missed sentence is a word
 * that keeps rendering, while a false positive is a guard somebody silences.
 * The one legitimate rendered "seeds" in the tree — a dandelion, in a
 * painting's caption — lives in `lib/claudefavorites.ts` as data, outside
 * this sweep by construction rather than by exception.
 */

import { describe, expect, it } from 'vitest'
import { tagEnd } from './tagscan'

/** Every component's source, read through Vite rather than the filesystem —
 *  `tsconfig.app.json` pins `types` to `vite/client`, so `node:fs` does not
 *  typecheck here. The shape `loadingstates.test.ts` and `hostedcopy.test.ts`
 *  use. */
const sources = import.meta.glob('./**/*.tsx', {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>

/** A banned shape and what the game says instead. The message is the fix. */
interface Banned { shape: RegExp; instead: string }

const FORBIDDEN: Banned[] = [
  {
    shape: /\bseeds?\b/i,
    instead: 'Magic shuffles: the Simulator says "shuffle {n}" and its control is "Shuffle"',
  },
  {
    shape: /\b(?:DuckDB|SQLite|Postgres(?:QL)?)\b/i,
    instead: 'the cards are "the library’s own" (see hostedcopy.test.ts); no database has a name here',
  },
  {
    shape: /\bclaude-[a-z0-9]+(?:-[a-z0-9]+)*\b|\b(?:Opus|Sonnet|Haiku|Fable)\b/,
    instead: 'Claude, by name only — never a model id or a model family',
  },
  {
    shape: /\b(?:TypeScript|JavaScript|Golang)\b/,
    instead: 'no language is named to a player; say what the thing does',
  },
]

/** Attributes whose value is read by a person rather than by a machine.
 *  `seed=` is pointedly *not* here: `tarot.tsx` sets it on an SVG
 *  `feTurbulence`, a filter parameter no eye ever meets. */
const RENDERED_ATTRS = ['label', 'title', 'placeholder', 'aria-label', 'alt']

/** Anything with one of these in it is code, not copy. */
const CODE_PUNCTUATION = /[=;()]/

/** One run of characters a person will read, and where it starts. */
interface Fragment { text: string; at: number }

/** Every balanced `{…}` group collapsed to `{}`, so an interpolated value is
 *  invisible (it is named by the code and read by nobody) while the brace
 *  itself survives — `seed {}` is still the badge's shape. Backticks are
 *  tracked so a template literal's braces do not unbalance the count, which is
 *  `tagEnd`'s rule too. */
function collapseBraces(text: string): string {
  let out = ''
  let depth = 0
  let inTick = false
  for (const c of text) {
    if (depth === 0) { out += c; if (c === '{') { depth = 1; out += '}' } ; continue }
    if (c === '`') inTick = !inTick
    if (inTick) continue
    if (c === '{') depth++
    else if (c === '}') depth--
  }
  return out
}

/** Every rendered fragment in one `.tsx` source: attribute values, then JSX
 *  text nodes. Two shapes because the failures were one of each — the badge
 *  was a text node (`<Badge>seed {seed}</Badge>`) and the control an attribute
 *  (`label="Seed"`). */
export function renderedFragments(source: string): Fragment[] {
  // Line and block comments blanked to spaces, so every offset still points
  // at the line it came from.
  const stripped = source.replace(/\/\/[^\n]*|\/\*[\s\S]*?\*\//g, (m) => ' '.repeat(m.length))
  const found: Fragment[] = []

  const attrs = RENDERED_ATTRS.join('|')
  const attr = new RegExp(`\\b(?:${attrs})=(?:"([^"]*)"|\\{'([^']*)'\\}|\\{"([^"]*)"\\})`, 'g')
  for (const m of stripped.matchAll(attr)) {
    const value = m[1] ?? m[2] ?? m[3] ?? ''
    found.push({ text: value, at: (m.index ?? 0) + m[0].indexOf(value) })
  }

  // A JSX text node: whatever follows an opening tag's `>` up to the next `<`.
  // `tagEnd` finds the `>` that really closes the tag — a lazy regex stops at
  // the one inside `onClick={() => …}`, which is the bug `controlstate`'s
  // first draft shipped with.
  const open = /<[A-Za-z][\w.]*(?=[\s/>])/g
  for (const m of stripped.matchAll(open)) {
    const end = tagEnd(stripped, m.index ?? 0)
    if (end < 0) continue
    const raw = stripped.slice(end + 1).split('<', 1)[0] ?? ''
    const text = collapseBraces(raw).replace(/\{\}/g, ' ')
    if (text.trim() && !CODE_PUNCTUATION.test(text)) found.push({ text, at: end + 1 })
  }
  return found
}

/** `path:line: 'word' renders. what to say instead.` for every banned match. */
function offencesIn(path: string, source: string): string[] {
  const out: string[] = []
  for (const { text, at } of renderedFragments(source)) {
    for (const { shape, instead } of FORBIDDEN) {
      const m = shape.exec(text)
      if (!m) continue
      const line = source.slice(0, at).split('\n').length
      out.push(`${path.replace(/^\.\//, 'src/')}:${line}: '${m[0]}' renders. ${instead}.`
        + `\n    …${text.trim().slice(0, 90)}…`)
    }
  }
  return out
}

describe('the copy a visitor reads', () => {
  it('never names a technology', () => {
    const offences: string[] = []
    for (const [path, body] of Object.entries(sources)) {
      if (path.includes('.test.')) continue
      offences.push(...offencesIn(path, body))
    }
    expect(offences, 'commandment 10: no seed, database, model id or language reaches a player')
      .toEqual([])
  })

  // Anti-vacuity, in three parts. A glob that resolved to nothing, a reader
  // that stopped finding fragments, and a table that stopped matching would
  // each report a clean tree, which is indistinguishable from one.
  it('is really reading every component', () => {
    expect(Object.keys(sources).filter((p) => !p.includes('.test.')).length).toBeGreaterThan(40)
  })

  // The four shapes that rendered before the guard existed, re-injected. A
  // guard that cannot catch the thing it was built for is decoration; this
  // is derived from the offences (PR #191's diff), not from the rule.
  it.each([
    ['the badge', '<Badge>seed {seed}</Badge>', 'seed'],
    ['the control', '<NumberField label="Seed" value={seed} onChange={setSeed} />', 'Seed'],
    ['the example words', '<input placeholder="mulligan, ramp, seed…" />', 'seed'],
    ['a badge with a real number', '<Badge>seed 7</Badge>', 'seed'],
    ['a database', '<p>Cards are looked up in DuckDB.</p>', 'DuckDB'],
    ['a model id', '<p>Read by claude-opus-4-1.</p>', 'claude-opus-4-1'],
    ['a model family', '<span title="Answered by Sonnet" />', 'Sonnet'],
  ])('catches %s', (_what, control, word) => {
    const found = offencesIn('./x.tsx', control)
    expect(found).toHaveLength(1)
    expect(found[0]).toContain(`'${word}' renders`)
  })

  // And the shapes it must *not* flag, or the guard would ban the cure, the
  // two allowed names, and the comments that explain the field.
  it.each([
    ['the cure', '<Badge>shuffle {seed}</Badge>'],
    ['the relabelled control', '<NumberField label="Shuffle" value={seed} onChange={setSeed} />'],
    ['the wire field in an expression', '<span key={`coin-${spin.seed}`} className="x" />'],
    ['a JSX comment', '<div>{/* the stashed seed re-deals the same cards */}</div>'],
    ['a line comment inside a tag', '<Foo // seed is the wire field\n  label="Shuffle" />'],
    ['a filter parameter', '<feTurbulence seed={7} baseFrequency="0.02" />'],
    ['a generic, which is not a tag', "const [mode] = useState<Mode>('seed')"],
    ['Claude, by name', '<p>Claude read the table.</p>'],
    ['Forge, by ruling', '<p>Forge plays the games.</p>'],
    ['a card name that contains the letters', '<p>Seedborn Muse untaps.</p>'],
  ])('lets %s through', (_what, control) => {
    expect(offencesIn('./x.tsx', control)).toEqual([])
  })
})
