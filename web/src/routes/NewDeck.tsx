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
 *
 * **A third door, and it opens somewhere else** (ADR 20). Both of the above
 * ask the same first question — which of the 32 do you want? — and somebody
 * who has never played cannot answer it. `theme` asks about *them* instead and
 * proposes colours at the end. Its output is deliberately the same state the
 * carousel produces, a `chosen` combination and a `commander`, so it lands on
 * step 3 and the button that makes the deck is the one that was already there.
 * It is the one mode that is **not** remembered: entering it starts a
 * conversation that costs money, and that should be a click every time.
 *
 * **And a fourth, which is the third wearing a costume** (ADR 21). `tarot`
 * runs the same interview under a different voice, with three cards dealt for
 * its first three slots — so a card is dealt *for* something you are about to
 * be asked, the readiness instrument is untouched, and this door lands on step
 * 3 by exactly the route the other three do. Not remembered either, and for
 * the same reason.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import {
  api,
  type Card,
  type Combination,
  type ColorTaxonomy,
  type ThemeCommander,
} from '../lib/api'
import { COLOR_VAR } from '../lib/mtg'
import { CardHover, ColorRing, ManaText } from '../components/ui'
import { ColorPentagram, TierGlyph } from '../components/pentagram'
import { TarotTable } from '../components/tarot'
import { ThemeInterview } from '../components/theme'

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

/** The four ways in, least-assuming first. */
const DOORS = [
  { key: 'theme', label: 'Help me decide' },
  { key: 'tarot', label: 'Read my cards' },
  { key: 'guided', label: 'Take me through the colours' },
  { key: 'direct', label: 'I know what I want' },
] as const

type Mode = (typeof DOORS)[number]['key']

const DOOR_BLURBS: Record<Mode, string> = {
  theme: 'A few questions about you — none of them about Magic — and then a '
       + 'suggestion you are free to ignore.',
  tarot: 'Pick a reader, turn over three cards, and answer what they ask. '
       + 'The cards colour the questions; the answers are still yours.',
  guided: 'Pick a colour combination, then a commander. There are 32 '
        + 'combinations in Magic, and building one of each is a challenge '
        + 'people spend years on.',
  direct: 'Pick a combination, or search for the commander you already have '
        + 'in mind.',
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
  const [params] = useSearchParams()
  const [taxonomy, setTaxonomy] = useState<ColorTaxonomy | null>(null)
  const [taxonomyError, setTaxonomyError] = useState<string | null>(null)

  // Remembered, so the tutorial is offered once rather than every visit —
  // except for the two Claude doors, which are not stored. Landing straight
  // back in a conversation because you tried one last week is a bill nobody
  // asked for, and that goes double for the one that opens with a shuffle.
  const [mode, setMode] = useState<Mode>(
    () => (localStorage.getItem('mtglab-new-deck-mode') === 'direct'
      ? 'direct' : 'guided'),
  )
  useEffect(() => {
    if (mode !== 'theme' && mode !== 'tarot') {
      localStorage.setItem('mtglab-new-deck-mode', mode)
    }
  }, [mode])

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

  /**
   * Show a combination in the carousel, wherever in the taxonomy it lives.
   *
   * The pentagram's vertices are mono and its ten lines are guilds, so a click
   * on the wheel can cross a tier boundary: picking Azorius from a diagram
   * drawn on the mono tier has to move the tier selector as well as the index.
   *
   * It stops at showing, deliberately. `choose` commits to a combination and
   * loads its commanders; this is "look at this one", which is what pointing
   * at a shape on a diagram means. The card below still carries the Build
   * button, so the act that starts a deck is the same one it was before the
   * wheel existed.
   */
  const goTo = useCallback((combo: Combination) => {
    const list = taxonomy?.combinations.filter((c) => c.tier === combo.tier) ?? []
    const at = list.findIndex((c) => c.key === combo.key)
    if (at < 0) return
    setTier(combo.tier)
    setIndex(at)
  }, [taxonomy])

  // Arriving from a Build button on the Learn page. The link carries a
  // combination rather than a commander, so it lands on the carousel showing
  // that slot — the reading and the choosing are still the same act, this one
  // just started on the other screen.
  useEffect(() => {
    const key = params.get('c')
    if (!key || !taxonomy) return
    const combo = taxonomy.combinations.find((c) => c.key === key)
    if (combo) {
      setMode('guided')
      goTo(combo)
    }
  }, [params, taxonomy, goTo])

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

  /**
   * The theme interview's output, landed as the create flow's own state.
   *
   * This is where ADR 20's "it proposes; you create" stops being a rule being
   * honoured and becomes how the screen is wired. The interview hands over a
   * combination key and a corpus-resolved card; both go into exactly the
   * variables the carousel would have set, so the next thing the user sees is
   * step 3 and the button that makes the deck is the existing one.
   */
  const takeProposal = (key: string, card: ThemeCommander) => {
    setChosen(taxonomy?.combinations.find((c) => c.key === key) ?? null)
    setCommanders(null)
    pickCommander({
      // The create route needs a name and takes the rest from the corpus. The
      // fields below the name are for the hover card on this page only.
      name: card.name, category: '', why: '', qty: 1, known: true,
      mana_cost: card.mana_cost, type_line: card.type_line ?? undefined,
      oracle_text: card.oracle_text ?? undefined,
      color_identity: card.color_identity,
      image: card.image, art_crop: card.art_crop,
    })
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
      <header className="space-y-3">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Start a deck</h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>
            {DOOR_BLURBS[mode]}
          </p>
        </div>
        {/* Four ways in, and the order is deliberate: the one that assumes
            least goes first. The last two open onto "which of the 32 do you
            want", which is a question somebody who has never played cannot
            answer (ADR 20). */}
        {!chosen && !commander && (
          <div className="flex flex-wrap gap-1">
            {DOORS.map((d) => (
              <button
                key={d.key}
                onClick={() => setMode(d.key)}
                className="rounded-md px-3 py-1.5 text-sm font-medium transition"
                style={{
                  color: mode === d.key ? 'var(--text-primary)' : 'var(--text-muted)',
                  background: mode === d.key ? 'var(--gridline)' : 'transparent',
                  border: '1px solid var(--hairline)',
                }}
              >
                {d.label}
              </button>
            ))}
          </div>
        )}
      </header>

      {/* ------------------------------------- step 1, theme: ask about them */}
      {!chosen && !commander && mode === 'theme' && (
        <ThemeInterview onPick={takeProposal} onLeave={() => setMode('guided')} />
      )}

      {/* ---------------------- step 1, tarot: the same interview, with a
          reader and three cards in front of it (ADR 21). The spread's three
          positions *are* the slot kinds, so this reaches step 3 by exactly the
          route the plain interview does — one door, dressed. */}
      {!chosen && !commander && mode === 'tarot' && (
        <TarotTable onPick={takeProposal} onLeave={() => setMode('guided')} />
      )}

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
          {/* Seven identical grey pills said nothing about what they were
              selecting, which on the one screen whose whole job is teaching
              the taxonomy was the wrong thing for the taxonomy's own control
              to be. Each carries the wheel with its own shape lit now, so the
              row reads as one figure picked out seven ways — and the two that
              people actually confuse, shard and wedge, are visibly an arc
              against a span before either label is read. */}
          <div className="flex flex-wrap gap-1.5">
            {taxonomy.tiers.map((t) => (
              <button
                key={t.key}
                onClick={() => pickTier(t.key)}
                aria-pressed={tier === t.key}
                className="flex items-center gap-2 rounded-lg py-1.5 pl-2 pr-3 text-sm font-medium transition"
                style={{
                  color: tier === t.key ? 'var(--text-primary)' : 'var(--text-muted)',
                  background: tier === t.key ? 'var(--gridline)' : 'transparent',
                  // The selected tier gets a real border rather than a hairline,
                  // because a filled background alone is nearly invisible on the
                  // dark theme where `--gridline` is two steps off the page.
                  border: `1px solid ${tier === t.key ? 'var(--baseline)' : 'var(--hairline)'}`,
                }}
              >
                <TierGlyph tier={t.key} />
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

          {/* The wheel, on the tier it explains.

              Mono is where it belongs because a vertex *is* a mono-colour
              deck, so the diagram is the tier's own chooser rather than an
              illustration beside it. That it also answers "why is Azorius
              white-blue and Orzhov white-black" is the geometry paying for
              itself: the ten lines were going to be drawn anyway, and leaving
              them dead would raise the question of what they were for. */}
          {tier === 'mono' && (
            <section className="card-surface rounded-xl px-6 py-6">
              <ColorPentagram
                combinations={taxonomy.combinations}
                onPick={goTo}
                selected={current?.key}
              />
            </section>
          )}

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

              {/* The short version of the depth, with the long one one click
                  away. This is a chooser and it stays one: the faces get
                  named here, and the story, the cards and the counts live on
                  Learn rather than growing this panel to twice its height on
                  the screen somebody is trying to get through. Only the twenty
                  slots that are a faction have champions at all. */}
              {current.champions.length > 0 && (
                <p className="mt-3 max-w-3xl text-sm"
                   style={{ color: 'var(--text-muted)' }}>
                  <span className="text-xs uppercase tracking-wide">
                    Who they are
                  </span>{' '}
                  <span style={{ color: 'var(--text-secondary)' }}>
                    {current.champions.map((c) => c.card).join(' · ')}
                  </span>
                </p>
              )}
              <p className="mt-2 text-xs">
                <Link to={`/learn?c=${current.key}`}
                      style={{ color: 'var(--series-1)' }}>
                  {current.lore
                    ? `Read what happened to ${current.name} →`
                    : `${current.name} on the Learn page →`}
                </Link>
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
