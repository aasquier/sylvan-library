/**
 * The 32 combinations as *addresses*, and the taxonomy they are read out of.
 *
 * Each of the 32 has a page of its own at `/colors/:slug`, so each of them
 * needs a name a person can type, a link can carry and a bookmark can keep.
 * Two questions, and they have different answers.
 *
 * ## What the canonical slug is
 *
 * **Twenty-five of the 32 are places, and a place is called by its name.**
 * The ten guilds, the five shards, the five clans and the five colours are
 * all things Magic has a proper noun for, and `/colors/golgari` is the
 * address a Commander player would guess on the first try. `Mono-White`
 * drops its prefix, because the page is about White and the "mono" is a fact
 * about the *deck* rather than about the colour.
 *
 * **The other seven are arithmetic, and arithmetic gets letters.** The five
 * four-colour sets and WUBRG are not places — nobody lives in Artifice — and
 * their names are product labels that two sources disagree about: Wizards'
 * Commander 2016 deck is *Artifice* and EDHREC's Nephilim is *Yore-Tiller*,
 * for one identical set of colours. Putting either in the URL picks a winner
 * in the one place a link can never be corrected afterwards. `wubr` picks
 * nobody, and `wubrg` is not a compromise at all — it is what every player
 * already calls five-colour out loud. Colourless is the seventh and takes
 * `colourless`, because `c` alone is not a word and reads as a typo.
 *
 * ## What a slug will *answer to*
 *
 * Everything, deliberately, because a reference page's job is to be found:
 * the canonical form, every combination's colour key (`/colors/bg`), its
 * name, its aliases, and the American spelling of anything with a *colour*
 * in it. All of them are generated from the served taxonomy by the rules
 * below — **there is no table of 32 slugs in this file**, for the reason
 * `components/pentagram.tsx` states about itself: the served colors table is
 * the one authority on what a slot is called, and a second copy drifts in
 * silence. Rename a guild in the data and its address changes with it.
 *
 * Anything that is not the canonical form redirects to it, so `/colors/bg`
 * and the `?c=BG` links already shared out of the old Learn page both land on
 * `/colors/golgari` and stay there.
 *
 * The taxonomy cache at the bottom is `lib/glossary.ts`'s, for the same
 * reason: 32 rows of fixed prose that cannot go stale inside a session, asked
 * for by every colour page and by the index they link back to.
 */

import { useEffect, useState } from 'react'
import { api, type ColorTaxonomy, type Combination } from './api'

/**
 * The tiers whose members Magic gave a proper name to. Read as a question
 * about the *tier* rather than about the name, because every one of the 32
 * has a name and only these have one that names a place.
 */
const NAMED_TIERS = new Set(['mono', 'guild', 'shard', 'wedge'])

/**
 * Any string as a URL segment: lower case, punctuation to hyphens, no
 * repeats and no hyphen at either end.
 *
 * `Mono-White` would come through as `mono-white` and the prefix is dropped
 * before this ever sees it — see [comboSlug]. Nothing else in the table has
 * punctuation in it today, but a name that gained an apostrophe would come
 * out of here as a working URL rather than as a broken one.
 */
export function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/** `Mono-White` is a page about White. */
function bareName(name: string): string {
  return slugify(name.replace(/^mono[\s-]*/i, ''))
}

/**
 * The address one combination lives at.
 *
 * Colourless is checked before the tier rule rather than after it: its tier
 * key is `colorless` in the data (the code spelling) and its name is
 * `Colourless` (the prose spelling), and the page should be at the one the
 * site says out loud. The American spelling still resolves — see
 * [slugsFor].
 */
export function comboSlug(combo: Combination): string {
  if (combo.tier === 'colorless') return 'colourless'
  if (NAMED_TIERS.has(combo.tier)) return bareName(combo.name)
  return combo.key.toLowerCase()
}

/** `colour` -> `color`, applied to every spelling so nobody has to guess
 *  which side of the Atlantic this site was written on. Returns nothing when
 *  there is no *colour* in the word, so the caller adds no duplicates. */
function americanised(slug: string): string | null {
  const swapped = slug.replace(/colour/g, 'color')
  return swapped === slug ? null : swapped
}

/**
 * Every spelling one combination answers to, canonical first.
 *
 * Order is the contract: the first entry is what a redirect sends you to, and
 * everything after it is a way of arriving.
 */
export function slugsFor(combo: Combination): string[] {
  const out: string[] = [comboSlug(combo)]
  const add = (s: string) => {
    if (s && !out.includes(s)) out.push(s)
  }
  add(combo.key.toLowerCase())
  add(bareName(combo.name))
  add(slugify(combo.name))
  for (const alias of combo.aliases) add(slugify(alias))
  // Copied rather than iterated in place: `add` is appending to `out` as we
  // go, and the American spellings are about the spellings we already have.
  for (const spelling of [...out]) {
    const other = americanised(spelling)
    if (other) add(other)
  }
  return out
}

/**
 * Every spelling of every combination, pointing at the combination itself.
 *
 * **Canonical forms are registered in a first pass and are never overwritten**,
 * so an alias of one row can never take an address that is another row's own
 * page — the failure that would be invisible until somebody's bookmark opened
 * the wrong colour. Nothing in the served table collides today and
 * `colors.test.ts` is what keeps that true rather than this sentence.
 */
export function slugIndex(combinations: Combination[]): Map<string, Combination> {
  const index = new Map<string, Combination>()
  for (const combo of combinations) index.set(comboSlug(combo), combo)
  for (const combo of combinations) {
    for (const slug of slugsFor(combo)) {
      if (!index.has(slug)) index.set(slug, combo)
    }
  }
  return index
}

/** What a URL segment resolved to, and whether it was already the address
 *  this page should be living at. `null` is a segment naming nothing. */
export interface Resolved {
  combo: Combination
  /** The canonical slug — equal to what was asked for, or the place a
   *  redirect should send it. */
  slug: string
  canonical: boolean
}

/** Resolve one URL segment against the served table. Case-insensitive,
 *  because a link pasted out of a chat window arrives however it arrives. */
export function resolveSlug(combinations: Combination[], asked: string): Resolved | null {
  const combo = slugIndex(combinations).get(slugify(asked))
  if (!combo) return null
  const slug = comboSlug(combo)
  return { combo, slug, canonical: slug === slugify(asked) }
}

/** The address of a combination named by its colour key — what the old
 *  `?c=BG` links carried. `null` when no row has that key. */
export function slugForKey(combinations: Combination[], key: string): string | null {
  const combo = combinations.find((c) => c.key === key.toUpperCase())
  return combo ? comboSlug(combo) : null
}

/** A combination's page. One place builds this string. */
export function colorPath(combo: Combination): string {
  return `/colors/${comboSlug(combo)}`
}

/* ------------------------------------------------- the taxonomy, fetched once */

let pending: Promise<ColorTaxonomy> | null = null
let loaded: ColorTaxonomy | null = null

/** What went wrong, for the one screen that has to say so. `undefined` while
 *  a fetch is in flight or has not been asked for. */
export interface TaxonomyState {
  taxonomy: ColorTaxonomy | null
  failed: boolean
}

/**
 * The 32, the five colours, the seven tiers and the three eras — fetched once
 * per page load and shared by every screen that reads them.
 *
 * Unlike the glossary this one *does* report failure, because a colour page
 * with no taxonomy has nothing to render at all: it cannot even work out
 * which combination the address in the bar refers to. The glossary's silence
 * is right for a tooltip and wrong for a room.
 */
export function useColorTaxonomy(): TaxonomyState {
  const [taxonomy, setTaxonomy] = useState<ColorTaxonomy | null>(loaded)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    if (loaded) return
    let live = true
    pending ??= api.colors()
    pending
      .then((t) => {
        loaded = t
        if (live) setTaxonomy(t)
      })
      .catch(() => {
        pending = null
        if (live) setFailed(true)
      })
    return () => { live = false }
  }, [])
  return { taxonomy, failed }
}

/** Test seam: drop the module-level cache between cases. */
export function resetColorTaxonomyCache(): void {
  pending = null
  loaded = null
}
