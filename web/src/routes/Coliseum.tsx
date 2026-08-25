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
  type DeckTile, type ForgeBeats, type ForgeResult, type Job,
  type ValidationReport,
} from '../lib/api'
import { Badge, CardHover, Caveat, ErrorNote, NumberField, Select, StatTile }
  from '../components/ui'
import { DeckCaution } from '../components/closedform'
import { DataTable } from '../components/datatable'
import { MatchBoard } from '../components/board'
import { MatchBeats, MatchTheater } from '../components/theater'
import { type Arriving, type Speed, type StagedBeat, useReel }
  from '../lib/reel'
import {
  beatLine, playerTurns, shortName, theaterBeats, theaterRows, turnsTaken,
} from '../lib/theater'
import { CrossedSwordsGlyph } from '../components/glyphs'
import { HelpTip, Term } from '../components/term'
import secutorArt from '../assets/coliseum/secutor.webp'

/** Every control and figure here, keyed to the served glossary — the same
 *  contract the Simulator's own controls use. */
const help = (key: string) => <HelpTip name={key} />

/** The seed a match gets unless somebody asks for another. Matches the
 *  server's own default, so the app and the CLI describe the same shuffle. */
const DEFAULT_SEED = 7

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

      {/* One bar, not two absolutes. Both used to be anchored to the bottom
          of the frame and they closed to twelve pixels on a laptop and
          overlapped outright on a phone; a flex row cannot overlap itself,
          and below the width where both fit the credit takes its own line.
          The scrim under them is in `.coliseum-plate` and is what makes the
          credit legible over pale stone in the light theme. */}
      <div className="coliseum-plate">
        <h1 className="coliseum-title text-3xl font-semibold text-white
                       sm:text-4xl">
          The Coliseum
        </h1>
        {/* The credit as a footnote on the painting rather than a caption
            under it — small, but never absent: it is somebody's work. */}
        <p className="coliseum-footnote">
          <em>Grand Coliseum</em>, {HERO.printing} — art by {HERO.artist}
        </p>
      </div>
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
  /** Every game the room can show: the live one while the match runs, and all
   *  of them once it is over.
   *
   * The partial carries one game — the newest — and is cleared the moment the
   * job finishes; the result carries the whole match. So a match is watched
   * live one game at a time, and watched *back* a game at a time, which is
   * when somebody actually has the time for it. */
  const played = useMemo((): ForgeBeats[] => {
    if (forge?.beats?.length) return forge.beats
    const live = theaterBeats(job?.partial)
    return live ? [live] : []
  }, [forge, job?.partial])

  // Which game is on the field. Null follows the match; a number is a choice,
  // and it survives the match ending so a pick does not get yanked away by the
  // last game landing.
  const [pickedGame, pickGame] = useState<number | null>(null)
  const watching = useMemo(() => {
    if (pickedGame != null) {
      const picked = played.find((g) => g.game === pickedGame)
      if (picked) return picked
    }
    return played[played.length - 1]
  }, [played, pickedGame])

  const arriving = useMemo((): Arriving | null => {
    const heard = watching ?? null
    if (!heard) return null
    const name = (slug: string) =>
      shortName(decks.find((d) => d.slug === slug)?.name ?? slug)
    // Forge counts every player-turn; a person counts their own. See
    // `playerTurns` for the measurement that made this necessary.
    const turns = playerTurns(heard.beats)
    return {
      game: heard.game,
      truncated: heard.truncated,
      // The board rides with the beats it belongs to, so the picture and the
      // sentences can never be about different games — see `lib/reel.ts`.
      board: heard.board,
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
  }, [watching, decks])

  // **One clock for the room.** The reel drains the beats at reading speed and
  // says how many it has told; the account renders those beats and the board
  // is folded to exactly that many steps. The server builds one board step per
  // beat, so a single count keeps the picture and the sentences describing the
  // same moment — two components pacing themselves would drift within a turn.
  // **How fast the room reads, which is not how fast the Forge plays.** The
  // match runs flat out and lands its results when it lands them; this only
  // governs the retelling. `Watch` is the natural pace — a game spread across
  // the time the next one takes to arrive.
  const [speed, setSpeed] = useState<Speed>('play')
  // `seek` moves the reel's own mark, so pressing play after a scrub carries
  // on from where the hand left off rather than snapping back.
  const [reel, seek] = useReel(arriving, speed)

  /** What the field calls a seat. The board carries slugs and Forge's own deck
   *  titles; only the room has the shelf that turns either into a name. */
  const seatName = useCallback((slug: string | null, fallback: string) =>
    shortName(slug
      ? (decks.find((d) => d.slug === slug)?.name ?? slug)
      : fallback), [decks])

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
        <div data-open={running ? 'true' : 'false'}
             className="card-surface gatehouse mt-5 flex flex-wrap items-end
                        gap-3 rounded-xl p-4">
          {/* **The selects carry a floor, and that is the whole fix.**
              Three rounds of this bug, and the first two treated a
              *breakpoint* as if it knew how wide the room was.

              `flex-1` is `flex: 1 1 0%`, and with `min-w-0` these two were
              the only items on the row that could give: the number fields are
              a fixed `w-28` and the button holds its label. So they absorbed
              every deficit by shrinking toward nothing, and `flex-wrap` never
              saved them — an item with `min-width: 0` always fits, so the row
              has no reason to break. Measured on the deployed room with the
              `sm:` branch live and the container at phone widths: 32px at
              390, 19px at 520, 64px at 610, and one single row at every one
              of them.

              Round two hung the fix on `basis-full` below `sm`, which is
              correct only while the breakpoint agrees with the device. Aaron's
              phone reports a layout width at or above 640 — Safari's desktop
              mode does exactly that, and it is sticky per site — so it took
              the `sm:` branch at phone width and crushed, and the second fix
              never applied to the person who reported it.

              A minimum width does not care what the device claims to be. At
              `11rem` the pair cannot shrink into a smear; when the row runs
              out of space they *wrap*, which is what `flex-wrap` was there to
              do all along. `grow` rather than `flex-1` because grow leaves
              the basis alone, and the basis is what makes them take their own
              line on a real phone. The ellipsis on a long deck name survives:
              a `<select>` truncates its own option text at any width. */}
          <Select label="Champion" value={a} onChange={setA}
                  className="min-w-[11rem] grow basis-full
                             sm:basis-[13rem] sm:max-w-[15rem]"
                  options={decks.map((d) => ({
                    value: `${d.owner}/${d.slug}`,
                    label: (d.writable ? d.name : `${d.name} — ${d.owner}`)
                      + (d.pilot ? ` (${d.pilot})` : ''),
                  }))} />
          <Select label="Challenger" value={b} onChange={setB}
                  className="min-w-[11rem] grow basis-full
                             sm:basis-[13rem] sm:max-w-[15rem]"
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
          {/* `.btn-arena`, not the chart accent it used to wear: this is the
              one control in the app that starts a fight, and `--series-1` is
              the colour of *series one of a line chart*. Blood, sand and the
              smithy's brass, with the pair squaring up under the hand
              (Aaron, 2026-08-25 — "could be red with some swords"). */}
          <button type="button" onClick={() => void sendThemIn()}
                  disabled={running || !a || !b}
                  className="btn btn-arena w-full sm:w-auto">
            <CrossedSwordsGlyph />
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

      {/* **The field, at full width, and that is the layout decision.** Two
          Commander battlefields, two hands, two land rows and two graveyards
          do not fit in a column beside a painting — a board squeezed into one
          is a board of thumbnails, and a graveyard reduced to a number is the
          thing this was built to stop being. So the arena's painting and the
          house's facts move below it while a match is on, and the thing you
          came to watch takes the room.

          Keyed on the job for the same reason the theater is: a second match
          begins on an empty field rather than on the last one's board. */}
      {hasBeats && (
        <div className="mt-6">
          <MatchBoard key={`board-${job?.id}`} board={reel.board}
                      shown={reel.told} game={reel.game}
                      name={seatName} running={running}
                      speed={speed} setSpeed={setSpeed}
                      of={reel.shown.length + reel.queue.length}
                      seek={seek}
                      games={played.map((g) => g.game)}
                      playing={reel.game} chooseGame={pickGame} />
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
                    The account
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
                <MatchBeats beats={reel.shown} game={reel.game}
                            truncated={reel.truncated} running={running} />
              ) : (
                <div className="flex min-h-0 flex-1 flex-col">
                  <div>
                    {facts[slide] && <Slide key={slide} fact={facts[slide]} />}
                  </div>
                  {/* **The slack, given a tenant.** The controls are pinned
                      to the bottom of this column on purpose — the painting
                      beside it is tall, the thirteen slides are not the same
                      length, and controls that move as the prose changes are
                      controls you have to find again every time. Measured on
                      the deployed room, that leaves between 162 and 274
                      pixels of nothing above them, which is a hole rather
                      than breathing room.

                      So the hole is where he stands, and it is *why* he can
                      stand: he takes exactly the space the slide did not, up
                      to a ceiling, and never less than his floor. On a phone
                      the two columns stack and there is little slack left —
                      the floor is what keeps him in the room there, and it
                      is the reason he is not simply sized to the gap.

                      Decorative, so `aria-hidden`: the facts beside him
                      already say what a secutor was, and a screen reader
                      reading a caption of the illustration of the thing it
                      just described is noise. */}
                  <div className="coliseum-yard">
                    <img className="coliseum-secutor" src={secutorArt}
                         alt="" aria-hidden="true" />
                  </div>
                  <div className="mt-4 flex items-center gap-2">
                    {/* `.btn` alone is a border-box with a transparent
                        border and no voice at all: no hover, no press,
                        nothing. These two carry the whole thirteen-slide
                        walk through an arena's lore and they answered the
                        hand with silence. `.btn-quiet` is the house's
                        patient voice and it was one word away. */}
                    <button type="button" className="btn btn-quiet btn-sm"
                            onClick={() => step(-1)}>Back</button>
                    <button type="button" className="btn btn-quiet btn-sm"
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
