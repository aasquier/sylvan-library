/**
 * Reading JSX opening tags out of source text.
 *
 * Two guards scan the tree for controls — `components/controlstate.test.tsx`
 * asks whether a control says which state it is in, `asynccontrols.test.ts`
 * asks whether one that starts work stops listening — and both need the same
 * unglamorous thing first: where does this tag end?
 *
 * **It is written rather than regexed, and that is not fastidiousness.** A
 * lazy `<button[\s\S]*?>` stops at the `>` inside `onClick={() => setTab(t)}`,
 * which is not a hypothetical: it is the bug `controlstate.test.tsx`'s first
 * draft shipped with, and it silently reported four of six real offenders as
 * clean. A guard that reads half a tag is worse than no guard, because it
 * reports safety it never measured.
 *
 * Nothing the app renders imports this. It lives under `src/` so the guards
 * can reach it and so a `.ts` file gets typechecked by the same `npm run
 * check` everything else does; Vite bundles from `main.tsx` outward, so it
 * reaches no browser.
 */

/**
 * The index of the `>` that closes the opening tag starting at `from`.
 *
 * Braces are counted so an expression's own angle brackets do not end the tag,
 * backticks are tracked so a template literal's braces do not unbalance the
 * count, and `=>` is stepped over because an arrow's `>` is not a tag's.
 * Returns -1 when the tag never closes, which a caller should skip rather than
 * guess at.
 */
export function tagEnd(src: string, from: number): number {
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

/** One opening tag, with enough about it to report a finding a person can go
 *  and look at. */
export interface FoundTag {
  /** `src/routes/Admin.tsx:739` — the path as a reader would type it. */
  where: string
  /** The whole opening tag, whitespace as written. */
  tag: string
  /** The file's full source, for a scan that has to look outside the tag. */
  source: string
}

/**
 * Every `<name …>` opening tag across a set of sources.
 *
 * `sources` is what `import.meta.glob(…, { query: '?raw' })` hands back: keys
 * relative to the importing file. Test files are skipped — a fixture is not a
 * surface — and the keys are rewritten to repository-shaped paths so a failure
 * message names something a person can open.
 *
 * The `../` in the anchor is the point: glob keys are relative to the file
 * that asked, so only a *leading* `../` is the prefix being rewritten. A bare
 * `.replace('../', …)` rewrites the first occurrence wherever it falls, which
 * is a different rule that happens to agree on today's paths.
 */
export function openTags(sources: Record<string, string>, name: string): FoundTag[] {
  const found: FoundTag[] = []
  for (const [path, src] of Object.entries(sources)) {
    if (path.includes('.test.')) continue
    const re = new RegExp(`<${name}\\b`, 'g')
    let m: RegExpExecArray | null
    while ((m = re.exec(src))) {
      const end = tagEnd(src, m.index + name.length + 1)
      if (end < 0) continue
      const line = src.slice(0, m.index).split('\n').length
      found.push({
        where: `${path.replace(/^(\.\.\/|\.\/)/, 'src/')}:${line}`,
        tag: src.slice(m.index, end + 1),
        source: src,
      })
    }
  }
  return found
}
