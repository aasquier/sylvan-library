/**
 * The Coliseum: the six houses a Forge match is watched in.
 *
 * A Tier 3 match takes minutes, and until now those minutes were a progress
 * bar. This is the room those minutes belong to — and it is a room rather
 * than a panel on the Simulator, because it is worth walking through when no
 * match is running at all.
 *
 * Three decisions worth keeping.
 *
 * **The painting is shown whole.** `art_crop` is 1.37:1 and a full-bleed band
 * keeps less than half of it — the lesson `PageMasthead` exists to enforce and
 * which this project has learned four times. So the arena is a framed stage at
 * its own ratio, and the weather happens *over* it.
 *
 * **The rotation is paced for reading, not for glancing.** Thirty seconds a
 * slide, because these are two or three sentences of real prose and a carousel
 * that moves at banner speed is a carousel nobody finishes a sentence in. A
 * slide replaces its predecessor rather than cross-fading over it, for the
 * same reason.
 *
 * **Nothing here is authored at runtime.** The arenas, the champions' roles
 * and every fact are checked-in prose (`reference/data/coliseum.json`); the
 * card facts and the art come from the pool, and a name the pool cannot
 * resolve is dropped and counted rather than guessed at. With no pool the
 * prose still answers whole — only the paintings go missing, which the stage
 * renders as the arena's palette alone.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'

import { api, type Coliseum, type ColiseumArena, type ColiseumFact } from '../lib/api'
import { CardHover } from '../components/ui'

/** How long a slide holds. Long, deliberately: see the note above. */
const SLIDE_MS = 24_000

/** How long the room stays in one arena before walking to the next.
 *
 *  Ninety seconds is roughly four slides, and six arenas is then a nine-minute
 *  circuit — longer than most matches, so a match rarely sees the same arena
 *  twice and never sits in one for its whole length. Both timers restart on a
 *  click: a person who has chosen where to stand should not be walked off. */
const ARENA_MS = 90_000

/** The pennants, laid out once rather than per render.
 *
 *  Every value here is a *different* number on purpose. Nine banners sharing
 *  one period and one phase read as a screensaver; nine with their own read as
 *  wind. The heraldry is five hues so no two neighbours match. */
/** The room's own painting, and it does not rotate.
 *
 *  The six arenas below take turns; this one does not, and the distinction is
 *  load-bearing rather than cosmetic. The banners and the crowd are placed
 *  against *this* painting's geometry — its rim, its gates — so pointing the
 *  hero at whichever arena happens to be selected hangs pennants in the middle
 *  of Valor's Reach's drawing room. An effect tuned to one painting belongs to
 *  that painting.
 *
 *  Grand Coliseum, Onslaught (ONS) #319, art by Carl Critchlow — the arena
 *  this room is named for, from the set that also printed its champion. */
const HERO = {
  url: 'https://cards.scryfall.io/art_crop/front/c/2/c2dc8061-a855-4a81-9eb7-350b355a9b3f.jpg?1783945028',
  printing: 'Onslaught',
  artist: 'Carl Critchlow',
  alt: 'A vast oval arena of pale stone standing alone on a bare plain, '
     + 'ringed with gatehouses and crowned by two tall statues, under a wide '
     + 'and hazy sky.',
}

/** The hero: the whole painting at page width, with what the painting implies
 *  but cannot do — banners flying from the wall, and a crowd filing in.
 *
 *  **Whole, not cropped.** Spanning the page and cropping the page are
 *  different asks; `art_crop` is 1.37:1 and this frame keeps that ratio, so
 *  none of Critchlow's arena is thrown away to make a letterbox. */
function Hero() {
  return (
    <div className="coliseum-hero">
      <img className="coliseum-hero-art" src={HERO.url} alt={HERO.alt} />
      <div className="coliseum-hero-sky" aria-hidden="true" />

      <h1 className="coliseum-title text-3xl font-semibold text-white sm:text-4xl">
        The Coliseum
      </h1>
      {/* The credit as a footnote on the painting rather than a caption under
          it — small, but never absent: it is somebody's work. */}
      <p className="coliseum-footnote">
        <em>Grand Coliseum</em>, {HERO.printing} — art by {HERO.artist}
      </p>
    </div>
  )
}

/** What each kind of slide is called on screen. The label is the promise the
 *  slide keeps — a reader who wants the Magic ones can find them. */
const KIND_LABEL: Record<ColiseumFact['kind'], string> = {
  roman: 'Rome',
  gladiator: 'The gladiators',
  coliseum: 'The Colosseum',
  magic: 'Magic',
  paired: 'Rome, and its echo',
}

function Slide({ fact }: { fact: ColiseumFact }) {
  return (
    <div className="arena-slide">
      <p className="text-[0.68rem] font-semibold uppercase tracking-[0.14em]
                    text-[var(--muted)]">
        {KIND_LABEL[fact.kind]}
      </p>
      {fact.rome && (
        <p className="mt-2 text-[0.95rem] leading-relaxed text-[var(--ink)]">
          {fact.rome}
        </p>
      )}
      {fact.magic && (
        <p className={`text-[0.95rem] leading-relaxed ${
          fact.rome
            ? 'mt-3 border-l-2 pl-3 text-[var(--muted)]'
            : 'mt-2 text-[var(--ink)]'
        }`}
           style={fact.rome ? { borderColor: 'var(--arena-glow)' } : undefined}>
          {fact.magic}
        </p>
      )}
      {fact.card && (
        <p className="mt-2 text-[0.75rem] italic text-[var(--muted)]">
          {fact.card}
        </p>
      )}
    </div>
  )
}

function Stage({ arena }: { arena: ColiseumArena }) {
  return (
    <div
      className="arena-stage"
      style={{
        // The palette belongs to the painting rather than to the theme, so it
        // arrives inline from the served prose and the stylesheet reads it.
        '--arena-ink': arena.palette.ink,
        '--arena-glow': arena.palette.glow,
      } as React.CSSProperties}
    >
      {/* The named printing, never the pool's default — see `ArenaArt`. The
          pool still answers for the card's *facts*; it just does not get to
          choose the painting. */}
      {arena.art.url
        ? <img className="arena-art" src={arena.art.url}
               alt={`${arena.name} — ${arena.art.printing} art by ${arena.art.artist}`}
               loading="lazy" />
        : <div className="arena-art-missing" role="img"
               aria-label={`${arena.name}, without its painting`} />}
      {/* Decoration, and never load-bearing: no pointer events, out of the
          accessibility tree, and removed outright under reduced motion. */}
      <div className="arena-motion" data-motion={arena.motion} aria-hidden="true" />
      <div className="arena-veil" aria-hidden="true" />
    </div>
  )
}

export default function ColiseumRoom() {
  const [data, setData] = useState<Coliseum | null>(null)
  const [failed, setFailed] = useState(false)
  const [chosen, setChosen] = useState(0)
  const [slide, setSlide] = useState(0)

  useEffect(() => {
    let alive = true
    api.coliseum()
      .then((d) => { if (alive) setData(d) })
      .catch(() => { if (alive) setFailed(true) })
    return () => { alive = false }
  }, [])

  const arena: ColiseumArena | undefined = data?.arenas[chosen]
  const facts = useMemo(() => arena?.facts ?? [], [arena])

  // `nudge` is bumped by every deliberate click, which restarts both clocks
  // below. Without it a click could be followed a heartbeat later by the timer
  // firing anyway — the room walking off just as somebody chose to stand
  // still, which is the single most irritating thing a carousel does.
  const [nudge, setNudge] = useState(0)

  // Walking into a house begins at its first fact rather than halfway through
  // the last one's — done by moving both values together in `enter`, never by
  // an effect that watches `chosen` and calls `setSlide`. That shape schedules
  // a second render for every arena change and oxlint refuses it by name
  // ("calling setState synchronously within an effect can trigger cascading
  // renders"); the two pieces of state change for one reason, so they change
  // in one place.
  const enter = useCallback((next: number) => {
    setNudge((n) => n + 1)
    setChosen(next)
    setSlide(0)
  }, [])

  useEffect(() => {
    if (facts.length < 2) return
    const id = window.setInterval(
      () => setSlide((n) => (n + 1) % facts.length), SLIDE_MS)
    return () => window.clearInterval(id)
  }, [facts.length, chosen, nudge])

  // And the room itself walks on, so six arenas are seen rather than one.
  const arenaCount = data?.arenas.length ?? 0
  useEffect(() => {
    if (arenaCount < 2) return
    const id = window.setInterval(() => {
      setChosen((n) => (n + 1) % arenaCount)
      setSlide(0)
    }, ARENA_MS)
    return () => window.clearInterval(id)
  }, [arenaCount, chosen, nudge])

  const step = useCallback((by: number) => {
    if (facts.length === 0) return
    setNudge((n) => n + 1)
    setSlide((n) => (n + by + facts.length) % facts.length)
  }, [facts.length])



  return (
    <div className="mx-auto max-w-5xl px-4 pb-16">
      {/* Not `PageMasthead`, and that is a deliberate departure rather than
          an oversight. That component is a *nameplate* — the painting beside
          the title — and its rule exists to stop a 1.37:1 crop being flattened
          into a band. This room's subject IS a place, so the place is the
          page's first fact; the ratio is kept whole, which is what that rule
          was actually protecting. The h1 moves here with it. */}
      <Hero />

      <p className="mt-6 max-w-2xl text-[0.95rem] leading-relaxed text-[var(--muted)]">
        Six houses, and what Rome did in each of them. A match played here takes
        minutes; this is what those minutes are for.
      </p>

      {failed && (
        <p className="mt-8 text-[var(--muted)]">
          The coliseum is dark tonight — its doors did not answer.
        </p>
      )}

      {data && arena && (
        <>
          {/* Places rather than actions, so `.strip-tab` rather than `.btn`
              (commandment 17). */}
          {/* Places rather than actions, so `.strip-tab` rather than `.btn`
              (commandment 17). The class carries colour and transition only —
              the border, padding and the `is-active` ink come from here, which
              is the shape `Library` and `DeckDetail` already use. */}
          <div role="tablist" aria-label="Arenas"
               className="mt-6 flex flex-wrap border-b"
               style={{ borderColor: 'var(--hairline)' }}>
            {data.arenas.map((a, i) => (
              <button
                key={a.key}
                role="tab"
                aria-selected={i === chosen}
                onClick={() => enter(i)}
                className={`strip-tab -mb-px border-b-2 px-3 py-2 text-sm font-medium${
                  i === chosen ? ' is-active' : ''}`}
              >
                {a.name}
              </button>
            ))}
          </div>

          <div className="mt-4 grid gap-6 lg:grid-cols-[1.15fr_1fr]">
            <div>
              <Stage arena={arena} />
              {/* Somebody painted this. Name them. */}
              <p className="mt-2 text-[0.75rem] text-[var(--muted)]">
                {arena.plane}
                {arena.backdrop && <> · <em>{arena.backdrop.name}</em></>}
                {' · '}{arena.art.printing}, art by {arena.art.artist}
              </p>
            </div>

            <section aria-label={`Facts about ${arena.name}`}
                     className="flex min-h-[14rem] flex-col justify-between">
              <div>
                {facts[slide] && <Slide key={slide} fact={facts[slide]} />}
              </div>
              <div className="mt-4 flex items-center gap-2">
                <button type="button" className="btn btn-sm"
                        onClick={() => step(-1)}>Back</button>
                <button type="button" className="btn btn-sm"
                        onClick={() => step(1)}>Next</button>
                <span className="ml-1 text-[0.75rem] text-[var(--muted)]">
                  {facts.length === 0 ? 'nothing to tell' : `${slide + 1} of ${facts.length}`}
                </span>
              </div>
            </section>
          </div>

          <section aria-label={`Champions of ${arena.name}`} className="mt-8">
            <h2 className="text-sm font-semibold uppercase tracking-[0.12em]
                           text-[var(--muted)]">
              Who fights here
            </h2>
            {arena.champions.length === 0 ? (
              <p className="mt-3 text-[0.9rem] text-[var(--muted)]">
                The stable is empty until the card pool answers.
              </p>
            ) : (
              <ul className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {arena.champions.map((c) => (
                  <li key={c.name}>
                    {/* The card itself on hover: a gladiator named and not
                        shown is a name, and the whole point is who fights. */}
                    <CardHover card={c} className="block">
                      <div className="flex gap-3">
                        {c.art_crop && (
                          <img src={c.art_crop} alt="" loading="lazy"
                               className="h-16 w-24 shrink-0 rounded object-cover" />
                        )}
                        <div className="min-w-0">
                          <p className="text-[0.85rem] font-semibold text-[var(--ink)]">
                            {c.name}
                          </p>
                          <p className="mt-0.5 text-[0.78rem] leading-snug text-[var(--muted)]">
                            {c.role}
                          </p>
                        </div>
                      </div>
                    </CardHover>
                  </li>
                ))}
              </ul>
            )}
          </section>

          {!data.pool && (
            <p className="mt-8 text-[0.8rem] text-[var(--muted)]">
              No card pool has answered, so the arenas are showing without their
              paintings. Everything written here still stands.
            </p>
          )}
        </>
      )}
    </div>
  )
}
