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
 *
 * **And the gates open from here** (Aaron's call, 2026-08-25). Tier 3 used to
 * be a fifth option on the Simulator, which made one screen answer two unlike
 * questions — "what do ten thousand shuffles say about my mana" is a number
 * you read, and "who wins" is a match you *watch*, for minutes. The Simulator
 * kept the arithmetic; the match moved whole to the room that was built to
 * watch one in. Two more decisions came with it.
 *
 * **This room narrates and the measuring surfaces do not.** `narrate: true`
 * crosses on the ask, Forge drops its `-q`, and about a hundred typed beats a
 * game come back on the job's stream. The flag is free in time and expensive
 * in volume (`events.go` measured both), which is exactly the trade a room
 * built for watching should take and a nightly sweep should refuse.
 *
 * **The beats are paced here, not by the server.** They arrive whole, one
 * game at a time, at the moment that game ends — Forge plays a game in
 * seconds and nobody can watch seconds of Commander, so the room holds a queue
 * and drains it at a rate a person can follow. When the next game's beats
 * arrive the current one is flushed rather than abandoned: every beat is
 * shown, and the room never falls a game behind what the pips already say.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import {
  api, errorMessage, followJob,
  type Coliseum, type ColiseumArena, type ColiseumFact,
  type DeckTile, type ForgeResult, type Job, type ValidationReport,
} from '../lib/api'
import { Badge, CardHover, Caveat, ErrorNote, NumberField, Select, StatTile }
  from '../components/ui'
import { DeckCaution } from '../components/closedform'
import { DataTable } from '../components/datatable'
import { MatchBeats, MatchTheater, type StagedBeat } from '../components/theater'
import {
  beatLine, playerTurns, shortName, theaterBeats, theaterRows, turnsTaken,
} from '../lib/theater'
import { HelpTip, Term } from '../components/term'

/** Every control and figure here, keyed to the served glossary — the same
 *  contract the Simulator's own controls use. */
const help = (key: string) => <HelpTip name={key} />

/** The seed a match gets unless somebody asks for another. Matches the
 *  server's own default, so the app and the CLI describe the same shuffle. */
const DEFAULT_SEED = 7

/** The beats of the newest game, in words, or nothing yet.
 *
 * The pacing belongs to [MatchBeats], which is keyed on the job — so a second
 * match starts an empty room by remounting rather than by an effect that
 * resets six pieces of state. This room's job is the translation, because
 * turning a slug into a name needs the shelf and the shelf is this room's. */
type Arriving = { game: number; beats: StagedBeat[]; truncated: boolean } | null

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
  const [params, setParams] = useSearchParams()
  const [data, setData] = useState<Coliseum | null>(null)
  const [failed, setFailed] = useState(false)
  const [chosen, setChosen] = useState(0)
  const [slide, setSlide] = useState(0)

  // The two seats. Each is an address — an owner and a slug (ADR 22) — and
  // each arrives whole in one query parameter, so a link into this room seats
  // both fighters: `/coliseum?a=aaron/gyome&b=aaron/arahbo`.
  const [a, setA] = useState(params.get('a') ?? '')
  const [b, setB] = useState(params.get('b') ?? '')
  const [decks, setDecks] = useState<DeckTile[]>([])
  // The gate (ADR 35). Where Forge is not installed the gates simply do not
  // open — no greyed-out button, no excuse. The room is still worth walking
  // through, which is why this page renders whole either way.
  const [forgeReady, setForgeReady] = useState(false)
  const [games, setGames] = useState(10)
  const [seed, setSeed] = useState(DEFAULT_SEED)
  const [job, setJob] = useState<Job | null>(null)
  const [forge, setForge] = useState<ForgeResult | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [checks, setChecks] = useState<Record<string, ValidationReport>>({})
  // Which of the two the right-hand column is showing. A place rather than an
  // action, so the control that switches it is a `.strip-tab`.
  const [pane, setPane] = useState<'game' | 'house'>('house')
  const cancelRef = useRef<null | (() => void)>(null)

  useEffect(() => {
    let alive = true
    api.coliseum()
      .then((d) => { if (alive) setData(d) })
      .catch(() => { if (alive) setFailed(true) })
    api.decks().then((d) => {
      if (!alive) return
      setDecks(d)
      // Both seats start occupied, because a form whose first state is
      // invalid scolds before anybody has touched it. A link that named the
      // fighters wins over the shelf's first two.
      const first = d[0]
      const second = d[1] ?? first
      if (first) setA((cur) => cur || `${first.owner}/${first.slug}`)
      if (second) setB((cur) => cur || `${second.owner}/${second.slug}`)
    }).catch((e) => { if (alive) setError(errorMessage(e)) })
    // Asked once, like `/api/claude`: a fact about the environment, and a
    // failed ask means the gates stay shut, which is the honest floor.
    api.forgeStatus()
      .then((st) => { if (alive) setForgeReady(st.available) })
      .catch(() => { if (alive) setForgeReady(false) })
    return () => { alive = false; cancelRef.current?.() }
  }, [])

  // The seats are in the URL, so a match somebody is watching is a link they
  // can send. Replaced rather than pushed: choosing an opponent is not a
  // navigation and should not need three Backs to undo.
  useEffect(() => {
    if (!a && !b) return
    setParams((cur) => {
      const next = new URLSearchParams(cur)
      if (a) next.set('a', a); else next.delete('a')
      if (b) next.set('b', b); else next.delete('b')
      return next
    }, { replace: true })
  }, [a, b, setParams])

  /** A deck the shelf knows, by its whole address. */
  const deckAt = useCallback((address: string) => {
    const cut = address.indexOf('/')
    const owner = cut < 0 ? '' : address.slice(0, cut)
    const slug = cut < 0 ? address : address.slice(cut + 1)
    return decks.find((d) => d.slug === slug && (!owner || d.owner === owner))
      ?? null
  }, [decks])

  const slugOf = (address: string) => {
    const cut = address.indexOf('/')
    return cut < 0 ? address : address.slice(cut + 1)
  }

  // What a match will leave out, before it is paid for — both seats, because
  // the bill here is minutes of real games. Asked only about a deck the shelf
  // has already said has errors, so the common case costs nothing.
  useEffect(() => {
    let live = true
    for (const address of [a, b]) {
      if (!address || checks[address]) continue
      const tile = deckAt(address)
      // `errors` is null when the pool never answered and the gate never ran,
      // which is not a pass and must not render as one — but it is also not
      // something this room can diagnose, so it asks nothing.
      if (!tile?.errors) continue
      api.validate({ owner: tile.owner, slug: tile.slug })
        .then((r) => { if (live) setChecks((c) => ({ ...c, [address]: r })) })
        .catch(() => undefined)   // a caution nobody can fetch is not an error
    }
    return () => { live = false }
  }, [a, b, decks, checks, deckAt])

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

  // ------------------------------------------------------------- the match

  const running = submitting
    || job?.status === 'queued' || job?.status === 'running'
  /** Pressed, and the job has not reported back yet — the seconds a match
   *  spends starting a JVM on another machine, and where "did that click
   *  land?" lives. */
  const lighting = submitting && !job

  async function sendThemIn() {
    if (!a || !b) return
    cancelRef.current?.()
    setSubmitting(true)
    setError(null)
    setForge(null)
    setJob(null)
    setPane('game')
    try {
      const submitted = await api.simForge({
        a_slug: slugOf(a), a_owner: a.slice(0, Math.max(0, a.indexOf('/'))),
        b_slug: slugOf(b), b_owner: b.slice(0, Math.max(0, b.indexOf('/'))),
        games, seed,
        // This room watches, so this room narrates. The measuring surfaces do
        // not ask, and Forge stays quiet for them.
        narrate: true,
      })
      setJob(submitted)
      const follower = followJob(submitted.id, setJob, undefined, submitted)
      cancelRef.current = follower.cancel
      const finished = await follower.promise
      setForge(finished.result as ForgeResult)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setSubmitting(false)
    }
  }

  const aDeck = deckAt(a)
  const bDeck = deckAt(b)
  const rows = forge ? forge.rows : theaterRows(job?.partial)

  /** Whatever the job is holding right now, in words.
   *
   *  Translated here rather than on the stage because turning a slug into a
   *  name needs the shelf, and the shelf is this room's. The stage renders
   *  what it is given. */
  const arriving = useMemo((): Arriving => {
    const heard = theaterBeats(job?.partial)
    if (!heard) return null
    const name = (slug: string) =>
      shortName(decks.find((d) => d.slug === slug)?.name ?? slug)
    // Forge counts every player-turn; a person counts their own. See
    // `playerTurns` for the measurement that made this necessary.
    const turns = playerTurns(heard.beats)
    return {
      game: heard.game,
      truncated: heard.truncated,
      beats: heard.beats.map((beat, i): StagedBeat => {
        const said = beatLine(beat, name)
        return {
          // The game and the beat's own place in it: a match replays row
          // numbers 1..n and a game replays beat 0..n, so neither alone is an
          // identity and both together are.
          key: `${heard.game}:${i}`,
          game: heard.game,
          turn: (beat.turn == null ? 0 : turns.get(beat.turn) ?? beat.turn),
          kind: beat.kind,
          who: said.who, text: said.text,
        }
      }),
    }
  }, [job?.partial, decks])

  // The play-by-play is offered from the moment a match starts and stays
  // offered after it ends, because the last game finishing is the moment
  // somebody most wants to read what just happened.
  const hasBeats = Boolean(job) && (running || Boolean(forge))


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
        Six houses, and what Rome did in each of them. Send two decks in and
        {' '}<Term name="tier-3">the Forge</Term> plays real games — whole
        ones, with an opponent, told blow by blow. It takes minutes; this is
        what those minutes are for.
      </p>

      {/* The gates. Only where the gate said Forge is installed — absent,
          never greyed out with an excuse, which is the rule the Ask Claude
          surfaces set and this inherits. The room itself is worth walking
          through either way, so nothing else on the page depends on it. */}
      {forgeReady && (
        <div className="card-surface mt-5 flex flex-wrap items-end gap-3
                        rounded-xl p-4">
          {/* Bounded, and measured on the deployed room rather than guessed:
              a `<select>` sizes to its widest option, and a deck named
              "Goreclaw, Terror of Qal Sisma — Mono-Green Stompy — gyome"
              made each of these 447px. Two of them ate a 992px bar and threw
              the dial, the shuffle and the button onto two more rows — five
              controls in three rows with half the bar empty. Capped, all five
              sit on one; the browser ellipsises the name, and the theater
              below carries it in full anyway. */}
          <Select label="Champion" value={a} onChange={setA}
                  className="min-w-0 max-w-[15rem] flex-1"
                  options={decks.map((d) => ({
                    value: `${d.owner}/${d.slug}`,
                    label: (d.writable ? d.name : `${d.name} — ${d.owner}`)
                      + (d.pilot ? ` (${d.pilot})` : ''),
                  }))} />
          <Select label="Challenger" value={b} onChange={setB}
                  className="min-w-0 max-w-[15rem] flex-1"
                  options={decks.map((d) => ({
                    value: `${d.owner}/${d.slug}`,
                    label: (d.writable ? d.name : `${d.name} — ${d.owner}`)
                      + (d.pilot ? ` (${d.pilot})` : ''),
                  }))} />
          <NumberField label="Games" value={games} onChange={setGames}
                       min={1} max={20} help={help('sim.forge_games')} />
          <NumberField label="Shuffle" value={seed} onChange={setSeed}
                       min={1} max={999999} help={help('sim.seed')} />
          {/* Pressed is a state this button has to be able to show: the job
              comes from the POST and the POST has a JVM starting behind it,
              so without its own word the slowest action in the app would be
              the one whose button sat unchanged for seconds after the click.
              The honest reading of that is that the click missed. */}
          <button type="button" onClick={() => void sendThemIn()}
                  disabled={running || !a || !b}
                  className="btn btn-primary btn-accent-1">
            {lighting ? 'Lighting the forge…'
              : running ? 'The match is on…' : 'Send them in'}
          </button>
        </div>
      )}

      {/* What a match will leave out, before it is paid for — both seats. */}
      <DeckCaution report={checks[a]} name={aDeck?.name ?? slugOf(a)} />
      {b !== a && (
        <DeckCaution report={checks[b]} name={bDeck?.name ?? slugOf(b)} />
      )}

      {error && <ErrorNote>The match failed: {error}</ErrorNote>}

      {/* The score. One element across both phases — fed from the job's
          `partial` while the games are landing and from the result's own rows
          once they have all landed — so the pips that lit one at a time are
          still lit when the match is over and the win has somewhere to arrive.

          Keyed on the job id, which is the only remount anybody wants: a
          second match between the same two decks reuses the row numbers 1..n,
          so without it React would match the new match's rows to the old
          one's elements and the second run would play out with no strikes at
          all. */}
      {job && (running || forge) && (
        <div className="mt-6">
          <MatchTheater
            key={job.id}
            a={aDeck} b={bDeck}
            aSlug={slugOf(a)} bSlug={slugOf(b)}
            games={forge ? forge.games : (job.total || games)}
            rows={rows}
            running={running}
          />
        </div>
      )}

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

            {/* The column beside the painting: the game while one is being
                played, the house otherwise. Both are offered once a match has
                started — the facts are the reason this room existed before it
                could run anything, and taking them away the moment a match
                begins would be the room forgetting itself. */}
            <section className="flex min-h-[14rem] flex-col">
              {hasBeats && (
                <div role="tablist" aria-label="What to watch"
                     className="mb-3 flex border-b"
                     style={{ borderColor: 'var(--hairline)' }}>
                  <button role="tab" type="button"
                          aria-selected={pane === 'game'}
                          onClick={() => setPane('game')}
                          className={`strip-tab -mb-px border-b-2 px-3 py-1.5
                                      text-sm font-medium${
                            pane === 'game' ? ' is-active' : ''}`}>
                    The game
                  </button>
                  <button role="tab" type="button"
                          aria-selected={pane === 'house'}
                          onClick={() => setPane('house')}
                          className={`strip-tab -mb-px border-b-2 px-3 py-1.5
                                      text-sm font-medium${
                            pane === 'house' ? ' is-active' : ''}`}>
                    The house
                  </button>
                </div>
              )}

              {hasBeats && pane === 'game' ? (
                // Keyed on the job, so a second match begins in an empty room:
                // every beat this stage is holding belongs to the match it was
                // mounted for, and a remount is a cheaper and more honest reset
                // than six setState calls in an effect.
                <MatchBeats key={job?.id} arriving={arriving} running={running} />
              ) : (
                <div className="flex flex-1 flex-col justify-between">
                  <div>
                    {facts[slide] && <Slide key={slide} fact={facts[slide]} />}
                  </div>
                  <div className="mt-4 flex items-center gap-2">
                    <button type="button" className="btn btn-sm"
                            onClick={() => step(-1)}>Back</button>
                    <button type="button" className="btn btn-sm"
                            onClick={() => step(1)}>Next</button>
                    <span className="ml-1 text-[0.75rem] text-[var(--muted)]">
                      {facts.length === 0 ? 'nothing to tell'
                        : `${slide + 1} of ${facts.length}`}
                    </span>
                  </div>
                </div>
              )}
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

          {/* The tale of the tape, once the last game has landed. Medians and
              tails, never a mean alone — and the clock kept apart from the
              draws, because a game called off at five minutes is the
              measurement giving up rather than a game that ended level. */}
          {forge && (
            <section aria-label="The result" className="mt-10 space-y-6">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {forge.decks.map((d) => (
                  <StatTile key={d.address} label={`${d.name} wins`}
                            value={d.wins.toString()}
                            hint={`of ${forge.played} played`}
                            help={help('stat.forge_wins')} />
                ))}
                <StatTile label="Draws" value={forge.draws.toString()}
                          hint="finished with no winner" />
                <StatTile label="Hit the clock" value={forge.timed_out.toString()}
                          hint={`called off at ${forge.clock}s`}
                          tone={forge.timed_out > 0 ? 'warning' : undefined}
                          help={help('stat.forge_timed_out')} />
                <StatTile label="Game length"
                          value={forge.median_seconds != null
                            ? `${forge.median_seconds}s` : '—'}
                          hint={forge.max_seconds != null
                            ? `median — longest ${forge.max_seconds}s` : undefined}
                          help={help('stat.forge_length')} />
              </div>

              <div className="card-surface space-y-2 rounded-xl p-5">
                <h3 className="text-sm font-semibold">Every game</h3>
                <DataTable
                  columns={[
                    { key: 'game', label: 'Game' },
                    { key: 'winner', label: 'Winner' },
                    { key: 'turns', label: 'Turns' },
                    { key: 'seconds', label: 'Seconds' },
                    { key: 'outcome', label: 'Outcome' },
                  ]}
                  rows={forge.rows.map((r) => ({
                    game: r.game,
                    winner: forge.decks.find((d) => d.slug === r.winner)?.name
                      ?? '—',
                    // A player's turns, not Forge's per-player-turn count.
                    turns: turnsTaken(r.turns) ?? '—',
                    seconds: r.seconds,
                    outcome: r.timed_out ? 'hit the clock'
                      : r.draw ? 'draw' : 'won',
                  }))}
                />
              </div>

              <div className="flex flex-wrap items-center gap-2 text-xs"
                   style={{ color: 'var(--text-secondary)' }}>
                <Badge>shuffle {forge.seed}</Badge>
                <span>
                  {forge.played} games in {Math.round(forge.wall_seconds)}s —
                  {' '}{Math.round(forge.startup_seconds)}s of that was lighting
                  the forge.
                </span>
              </div>
              <Caveat>{forge.caveat}</Caveat>
            </section>
          )}

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
