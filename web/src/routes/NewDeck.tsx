/**
 * Start a new deck — and learn what the colours mean on the way.
 *
 * The last gap in the deck lifecycle was a create form. This is that form,
 * wrapped around the thing a new player actually needs first: a name for what
 * they want to build. Three steps, and the first one is a history lesson.
 *
 * The carousel is the chooser rather than an aside. Slide through the ten
 * guilds and you are reading Ravnica's factions *and* picking a colour pair;
 * there is no separate "now select your colours" control, because the reading
 * and the choosing are the same act.
 *
 * Colour identity is never sent to the server. The commander decides it
 * (rule 2), and the taxonomy exists to help someone *find* a commander — so
 * the last step searches for legends whose identity is exactly the slot you
 * landed on, and the deck records whatever the corpus says about the one you
 * pick.
 *
 * **The lesson is skippable, and skipping is remembered.** A player who has
 * known what Golgari means for fifteen years should not be walked through
 * Ravnica to reach a text field, and should not have to decline it twice. Two
 * modes share every step after the first: `guided` is the carousel, `direct`
 * is a compact grid of all 32 plus a commander search that ignores colour
 * entirely — for someone who arrived already knowing the card they want. The
 * choice persists in localStorage, so the tutorial is offered once rather
 * than every visit.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type Card, type Combination, type ColorTaxonomy } from '../lib/api'
import { COLOR_NAMES, COLOR_VAR } from '../lib/mtg'
import { ManaText } from '../components/ui'

/** The era whose story named a tier, so the lesson has its setting attached. */
const TIER_ERA: Record<string, string> = {
  guild: 'Ravnica',
  shard: 'Alara',
  wedge: 'Tarkir',
}

/**
 * Canonical WUBRG key for a colour identity — the mirror of `colors.key_for`
 * in Python, and it must stay the mirror: the server's 32 slots are keyed this
 * way, so a different ordering here would fail to match any of them.
 */
function keyFor(identity: string[] | undefined): string {
  const order = 'WUBRG'
  const key = order.split('').filter((c) => (identity ?? []).includes(c)).join('')
  return key || 'C'
}

function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
}

/* ------------------------------------------------------------------ pips */

function ColorRing({ colors, size = 34 }: { colors: string[]; size?: number }) {
  if (!colors.length) {
    return (
      <span
        className="inline-flex items-center justify-center rounded-full font-semibold"
        style={{
          width: size, height: size, fontSize: size * 0.4,
          background: 'var(--mtg-c)', color: '#141414',
          boxShadow: '0 0 0 1px var(--hairline)',
        }}
      >C</span>
    )
  }
  return (
    <span className="inline-flex items-center" style={{ gap: 2 }}>
      {colors.map((c) => (
        <span
          key={c}
          title={COLOR_NAMES[c]}
          className="inline-flex items-center justify-center rounded-full font-semibold"
          style={{
            width: size, height: size, fontSize: size * 0.45,
            background: COLOR_VAR[c], color: '#141414',
            boxShadow: '0 0 0 1px var(--hairline)',
          }}
        >{c}</span>
      ))}
    </span>
  )
}

/* -------------------------------------------------------------- the page */

export default function NewDeck() {
  const navigate = useNavigate()
  const [taxonomy, setTaxonomy] = useState<ColorTaxonomy | null>(null)
  const [taxonomyError, setTaxonomyError] = useState<string | null>(null)

  // Remembered, so the tutorial is offered once rather than every visit.
  const [mode, setMode] = useState<'guided' | 'direct'>(
    () => (localStorage.getItem('mtglab-new-deck-mode') === 'direct'
      ? 'direct' : 'guided'),
  )
  useEffect(() => { localStorage.setItem('mtglab-new-deck-mode', mode) }, [mode])

  const [tier, setTier] = useState('guild')
  const [index, setIndex] = useState(0)

  // The fastest path of all: someone who already knows the commander and does
  // not care what the colours are called.
  const [nameQuery, setNameQuery] = useState('')
  const [nameHits, setNameHits] = useState<Card[] | null>(null)

  const [chosen, setChosen] = useState<Combination | null>(null)
  const [commanders, setCommanders] = useState<Card[] | null>(null)
  const [commander, setCommander] = useState<Card | null>(null)

  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [name, setName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.colors().then(setTaxonomy).catch((e) => setTaxonomyError(String(e)))
  }, [])

  const inTier = useMemo(
    () => taxonomy?.combinations.filter((c) => c.tier === tier) ?? [],
    [taxonomy, tier],
  )
  const current = inTier[index] ?? null

  // Changing tier restarts the carousel; leaving the index where it was would
  // land on a different combination than the one on screen a moment ago.
  const pickTier = useCallback((next: string) => {
    setTier(next)
    setIndex(0)
  }, [])

  const step = (delta: number) => {
    if (!inTier.length) return
    setIndex((i) => (i + delta + inTier.length) % inTier.length)
  }

  // Arrow keys drive the carousel while it is the active step. Only bound
  // before a combination is chosen, so it cannot fight the commander list.
  useEffect(() => {
    if (chosen || mode !== 'guided') return
    const onKey = (e: KeyboardEvent) => {
      // Not while someone is typing in the commander search.
      const el = document.activeElement
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return
      if (e.key === 'ArrowLeft') step(-1)
      if (e.key === 'ArrowRight') step(1)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // Debounced so a name search does not fire a query per keystroke.
  useEffect(() => {
    const q = nameQuery.trim()
    if (q.length < 2) { setNameHits(null); return }
    const timer = setTimeout(() => {
      api.searchCards({ q, commanders_only: 'true', sort: 'edhrec', limit: 12 })
        .then((r) => setNameHits(r.cards))
        .catch(() => setNameHits([]))
    }, 250)
    return () => clearTimeout(timer)
  }, [nameQuery])

  const choose = async (combo: Combination) => {
    setChosen(combo)
    setCommanders(null)
    setCommander(null)
    setError(null)
    try {
      const res = await api.searchCards({
        identity: combo.colors.join(''),
        identity_exact: 'true',
        commanders_only: 'true',
        sort: 'edhrec',
        limit: 24,
      })
      setCommanders(res.cards)
    } catch (e) {
      setError(String(e))
      setCommanders([])
    }
  }

  const pickCommander = (card: Card) => {
    setCommander(card)
    setName(card.name)
    if (!slugTouched) setSlug(slugify(card.name))
  }

  const create = async () => {
    if (!commander) return
    setCreating(true)
    setError(null)
    try {
      const made = await api.createDeck({
        slug, commander: [commander.name], name: name || commander.name,
      })
      navigate(`/decks/${made.slug}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setCreating(false)
    }
  }

  if (taxonomyError) {
    return (
      <div className="card-surface rounded-xl px-6 py-10 text-center">
        <p className="text-sm" style={{ color: 'var(--status-critical)' }}>
          Could not load the colour guide: {taxonomyError}
        </p>
      </div>
    )
  }
  if (!taxonomy) {
    return <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
  }

  const era = current ? taxonomy.eras.find((e) => e.name === TIER_ERA[current.tier]) : null

  return (
    <div className="space-y-8">
      <header className="flex flex-wrap items-start gap-4">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Start a deck</h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>
            {mode === 'guided'
              ? 'Pick a colour combination, then a commander. There are 32 combinations in Magic, and building one of each is a challenge people spend years on.'
              : 'Pick a combination, or search for the commander you already have in mind.'}
          </p>
        </div>
        {!chosen && !commander && (
          <button
            onClick={() => setMode(mode === 'guided' ? 'direct' : 'guided')}
            className="ml-auto rounded-md px-3 py-1.5 text-sm"
            style={{ border: '1px solid var(--hairline)', color: 'var(--text-secondary)' }}
          >
            {mode === 'guided' ? 'Skip the guide →' : '← Show the guide'}
          </button>
        )}
      </header>

      {/* ------------------------------- step 1, direct: no lesson, just 32 */}
      {!chosen && mode === 'direct' && (
        <section className="space-y-6">
          <div>
            <label className="block max-w-lg">
              <span className="text-xs uppercase tracking-wide"
                    style={{ color: 'var(--text-muted)' }}>
                Know your commander already?
              </span>
              <input
                value={nameQuery}
                onChange={(e) => setNameQuery(e.target.value)}
                placeholder="Search any legend by name…"
                className="mt-1 w-full rounded-md px-3 py-2 text-sm"
                style={{ background: 'var(--page)', color: 'var(--text-primary)',
                         border: '1px solid var(--hairline)' }}
              />
            </label>
            {nameHits && nameHits.length > 0 && (
              <ul className="mt-2 max-w-lg space-y-1">
                {nameHits.map((card) => (
                  <li key={card.name}>
                    <button
                      onClick={() => {
                        // Straight past the colour step entirely: the
                        // commander already determines the identity, so the
                        // slot is looked up rather than chosen.
                        const key = keyFor(card.color_identity)
                        setChosen(taxonomy.combinations.find(
                          (c) => c.key === key) ?? null)
                        pickCommander(card)
                      }}
                      className="w-full rounded-md px-3 py-2 text-left text-sm"
                      style={{ border: '1px solid var(--hairline)' }}
                    >
                      <span className="font-medium">{card.name}</span>{' '}
                      <span style={{ color: 'var(--text-muted)' }}>
                        — {card.type_line}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {nameHits?.length === 0 && nameQuery.trim().length >= 2 && (
              <p className="mt-2 text-sm" style={{ color: 'var(--text-muted)' }}>
                No legend by that name.
              </p>
            )}
          </div>

          <div>
            <p className="mb-2 text-xs uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              …or pick a combination
            </p>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {taxonomy.combinations.map((c) => (
                <button key={c.key} onClick={() => choose(c)}
                        className="card-surface flex items-center gap-3 rounded-lg px-3 py-2 text-left transition hover:opacity-90">
                  <ColorRing colors={c.colors} size={20} />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">{c.name}</span>
                    <span className="block truncate text-xs"
                          style={{ color: 'var(--text-muted)' }}>
                      {c.aliases[0] ?? c.key}
                    </span>
                  </span>
                </button>
              ))}
            </div>
          </div>
        </section>
      )}

      {/* -------------------------------- step 1, guided: the history lesson */}
      {!chosen && mode === 'guided' && (
        <section className="space-y-4">
          <div className="flex flex-wrap gap-1">
            {taxonomy.tiers.map((t) => (
              <button
                key={t.key}
                onClick={() => pickTier(t.key)}
                className="rounded-md px-3 py-1.5 text-sm font-medium transition"
                style={{
                  color: tier === t.key ? 'var(--text-primary)' : 'var(--text-muted)',
                  background: tier === t.key ? 'var(--gridline)' : 'transparent',
                  border: '1px solid var(--hairline)',
                }}
              >
                {t.label}
              </button>
            ))}
          </div>

          {current && (
            <article
              className="card-surface rounded-xl px-6 py-6"
              // A wash of the combination's own colours, so the panel *is* the
              // identity rather than describing one. Very low alpha: this sits
              // behind body prose and has to stay readable in both themes.
              style={{
                backgroundImage: current.colors.length
                  ? `linear-gradient(135deg, ${current.colors
                      .map((c, i) => `color-mix(in srgb, ${COLOR_VAR[c]} 22%, transparent) ${
                        (i / Math.max(current.colors.length - 1, 1)) * 100}%`)
                      .join(', ')})`
                  : 'none',
              }}
            >
              <div className="flex flex-wrap items-center gap-4">
                <ColorRing colors={current.colors} />
                <div>
                  <h2 className="text-2xl font-semibold tracking-tight">{current.name}</h2>
                  {current.aliases.length > 0 && (
                    <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
                      also called {current.aliases.join(', ')} — EDHREC’s name for
                      the same colours
                    </p>
                  )}
                </div>
                <span className="ml-auto tabular text-xs" style={{ color: 'var(--text-muted)' }}>
                  {index + 1} / {inTier.length}
                </span>
              </div>

              <p className="mt-4 text-lg" style={{ color: 'var(--text-primary)' }}>
                {current.tagline}
              </p>
              <p className="mt-3 max-w-3xl text-sm leading-relaxed"
                 style={{ color: 'var(--text-secondary)' }}>
                <ManaText>{current.history}</ManaText>
              </p>

              {era && (
                <p className="mt-4 max-w-3xl border-l-2 pl-4 text-sm leading-relaxed"
                   style={{ borderColor: 'var(--baseline)', color: 'var(--text-muted)' }}>
                  <strong style={{ color: 'var(--text-secondary)' }}>
                    {era.name}
                  </strong>{' '}
                  — {era.setting}. {era.story}
                </p>
              )}

              <div className="mt-6 flex flex-wrap items-center gap-2">
                <button onClick={() => step(-1)}
                        className="rounded-md px-3 py-2 text-sm"
                        style={{ border: '1px solid var(--hairline)',
                                 color: 'var(--text-secondary)' }}>
                  ← Previous
                </button>
                <button onClick={() => step(1)}
                        className="rounded-md px-3 py-2 text-sm"
                        style={{ border: '1px solid var(--hairline)',
                                 color: 'var(--text-secondary)' }}>
                  Next →
                </button>
                <button onClick={() => choose(current)}
                        className="ml-auto rounded-md px-4 py-2 text-sm font-medium"
                        style={{ background: 'var(--series-1)', color: '#fff' }}>
                  Build {current.name}
                </button>
              </div>

              <p className="mt-3 text-xs" style={{ color: 'var(--text-muted)' }}>
                Colour identity here is Scryfall’s, checked against{' '}
                <em>{current.verified_by}</em>.
              </p>
            </article>
          )}
        </section>
      )}

      {/* -------------------------------------------- step 2: the commander */}
      {chosen && !commander && (
        <section className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <ColorRing colors={chosen.colors} size={26} />
            <h2 className="text-xl font-semibold tracking-tight">
              {chosen.name} commanders
            </h2>
            <button onClick={() => { setChosen(null); setCommanders(null) }}
                    className="ml-auto rounded-md px-3 py-1.5 text-sm"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              ← Choose different colours
            </button>
          </div>
          <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            Legends whose colour identity is exactly {chosen.name}, most-built
            first. A commander from a smaller identity would be legal, but the
            deck it leads fills a different one of the 32.
          </p>

          {commanders === null && (
            <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Searching…</p>
          )}
          {commanders?.length === 0 && (
            <div className="card-surface rounded-xl px-6 py-8 text-center">
              <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
                No commanders found. This needs the card corpus — run{' '}
                <code>mtglab data refresh</code>.
              </p>
            </div>
          )}

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {commanders?.map((card) => (
              <button key={card.name} onClick={() => pickCommander(card)}
                      className="card-surface overflow-hidden rounded-xl text-left transition hover:opacity-90">
                {card.art_crop && (
                  <img src={card.art_crop} alt="" loading="lazy"
                       className="h-24 w-full object-cover" />
                )}
                <div className="px-4 py-3">
                  <p className="font-medium">{card.name}</p>
                  <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
                    {card.type_line}
                  </p>
                </div>
              </button>
            ))}
          </div>
        </section>
      )}

      {/* ------------------------------------------------ step 3: the name */}
      {commander && (
        <section className="card-surface space-y-4 rounded-xl px-6 py-6">
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-xl font-semibold tracking-tight">
              {commander.name}
            </h2>
            <button onClick={() => setCommander(null)}
                    className="ml-auto rounded-md px-3 py-1.5 text-sm"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              ← Pick another commander
            </button>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block">
              <span className="text-xs uppercase tracking-wide"
                    style={{ color: 'var(--text-muted)' }}>Deck name</span>
              <input value={name} onChange={(e) => setName(e.target.value)}
                     className="mt-1 w-full rounded-md px-3 py-2 text-sm"
                     style={{ background: 'var(--page)', color: 'var(--text-primary)',
                              border: '1px solid var(--hairline)' }} />
            </label>
            <label className="block">
              <span className="text-xs uppercase tracking-wide"
                    style={{ color: 'var(--text-muted)' }}>Slug</span>
              <input value={slug}
                     onChange={(e) => { setSlug(e.target.value); setSlugTouched(true) }}
                     className="mt-1 w-full rounded-md px-3 py-2 font-mono text-sm"
                     style={{ background: 'var(--page)', color: 'var(--text-primary)',
                              border: '1px solid var(--hairline)' }} />
              <span className="mt-1 block text-xs" style={{ color: 'var(--text-muted)' }}>
                The folder under <code>decks/</code>. Lowercase, hyphens.
              </span>
            </label>
          </div>

          {error && (
            <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
          )}

          <div className="flex flex-wrap items-center gap-3">
            <button onClick={create} disabled={!slug || creating}
                    className="rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                    style={{ background: 'var(--series-1)', color: '#fff' }}>
              {creating ? 'Creating…' : 'Create the deck'}
            </button>
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              It starts empty, as a <strong>draft</strong> — add the 99 next, and
              every card will want a reason for its slot.
            </p>
          </div>
        </section>
      )}
    </div>
  )
}
