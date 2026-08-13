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
import { COLOR_VAR } from '../lib/mtg'
import { CardHover, ColorRing, ManaText } from '../components/ui'

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

  /**
   * The three most-built commanders in each combination, keyed by slot.
   *
   * The faces of a colour pair, and deliberately *not* a hand-written list of
   * guild characters: a name typed from memory is a name nobody checked, and
   * rule 1 applies to reference data the same way it applies to a deck. These
   * come out of the corpus with their real colour identity, so the three
   * legends under "Selesnya" are Selesnya because Scryfall says so.
   *
   * Cached per key because the carousel is arrow-driven and stepping through
   * ten guilds should not be ten repeat queries on the way back.
   */
  const [leaders, setLeaders] = useState<Record<string, Card[]>>({})

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

  // Who leads the combination currently on screen. Skipped entirely when the
  // slot is already cached, so arrowing back through the carousel is free.
  useEffect(() => {
    if (chosen || mode !== 'guided' || !current) return
    const key = current.key
    if (leaders[key]) return
    let live = true
    api.searchCards({
      identity: current.colors.join(''),
      identity_exact: 'true',
      commanders_only: 'true',
      sort: 'edhrec',
      limit: 3,
    })
      .then((r) => { if (live) setLeaders((c) => ({ ...c, [key]: r.cards })) })
      // A fresh clone has no corpus. An empty list renders as nothing, which
      // is the right amount of noise for a page whose whole point is that it
      // works before `data refresh` has ever run.
      .catch(() => { if (live) setLeaders((c) => ({ ...c, [key]: [] })) })
    return () => { live = false }
  }, [current, chosen, mode, leaders])

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

  const tierInfo = taxonomy.tiers.find((t) => t.key === tier) ?? null
  const era = current ? taxonomy.eras.find((e) => e.name === TIER_ERA[current.tier]) : null
  // Colourless and All five have exactly one member, so stepping is a no-op
  // and a counter reading "1 / 1" is a control that says nothing.
  const many = inTier.length > 1

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
                    <CardHover card={card} className="block">
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
                    </CardHover>
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

          {/* Tier-level context, above the carousel rather than inside it.
              Ravnica names all ten guilds; repeating that paragraph under
              every guild taught it ten times and said nothing about the guild
              you were actually looking at. It sits here once, and the card
              below is free to be about Azorius.

              Two paragraphs, because they answer different questions and only
              one of them always has an answer. The blurb is what this kind of
              combination *is* and every tier has one — which is the gap that
              used to push the definition of a shard into Bant's own
              description, where someone who arrowed straight to Naya never
              saw it. The era is the block of Magic that supplied the names,
              and only guilds, shards and wedges come from one. */}
          <div className="max-w-3xl border-l-2 pl-4"
               style={{ borderColor: 'var(--baseline)' }}>
            <p className="text-sm leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              {tierInfo?.blurb}
            </p>
            {era && (
              <p className="mt-2 text-sm leading-relaxed"
                 style={{ color: 'var(--text-muted)' }}>
                <strong style={{ color: 'var(--text-secondary)' }}>{era.name}</strong>
                {' '}— {era.setting}. {era.story}
              </p>
            )}
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
                {many && (
                  <span className="ml-auto tabular text-xs"
                        style={{ color: 'var(--text-muted)' }}>
                    {index + 1} / {inTier.length}
                  </span>
                )}
              </div>

              <p className="mt-4 text-lg" style={{ color: 'var(--text-primary)' }}>
                {current.tagline}
              </p>
              <p className="mt-3 max-w-3xl text-sm leading-relaxed"
                 style={{ color: 'var(--text-secondary)' }}>
                <ManaText>{current.history}</ManaText>
              </p>

              {/* Who actually leads it — the specific thing this slot has
                  that the era paragraph above cannot say. Read off the corpus
                  by exact colour identity rather than typed from memory, so
                  the three legends under a guild's name are that guild's
                  colours because Scryfall says they are. Hover for the card. */}
              {leaders[current.key]?.length > 0 && (
                <div className="mt-5">
                  <p className="text-xs uppercase tracking-wide"
                     style={{ color: 'var(--text-muted)' }}>
                    Most-built commanders in these colours
                  </p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {leaders[current.key].map((card) => (
                      <CardHover key={card.name} card={card}>
                        <button
                          onClick={() => {
                            // Straight to the name step: picking a face of the
                            // guild is picking the guild.
                            setChosen(current)
                            setCommanders(null)
                            pickCommander(card)
                          }}
                          className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition hover:opacity-90"
                          style={{ border: '1px solid var(--hairline)',
                                   background: 'var(--page)' }}
                        >
                          {card.art_crop && (
                            <img src={card.art_crop} alt="" loading="lazy"
                                 className="h-7 w-12 rounded object-cover" />
                          )}
                          <span className="font-medium">{card.name}</span>
                        </button>
                      </CardHover>
                    ))}
                  </div>
                </div>
              )}

              <div className="mt-6 flex flex-wrap items-center gap-2">
                {/* Not rendered at all on a tier of one. A disabled pair would
                    still be two controls explaining that they do nothing;
                    Colourless and All five simply have nowhere to step. */}
                {many && (
                  <>
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
                  </>
                )}
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

          {/* The tile shows the art crop; hovering shows the whole card,
              floating beside the cursor rather than inside the tile. Choosing
              a commander is choosing a card, and the crop does not say what it
              costs, what it does, or whether it is the one you meant. */}
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {commanders?.map((card) => (
              <CardHover key={card.name} card={card} className="block">
                <button onClick={() => pickCommander(card)}
                        className="card-surface block w-full overflow-hidden rounded-xl text-left transition hover:opacity-90">
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
              </CardHover>
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
