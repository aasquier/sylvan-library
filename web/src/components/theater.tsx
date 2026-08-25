/**
 * The match theater: two commanders across an anvil, and the games landing
 * between them one blow at a time.
 *
 * The half of a match a person can see. The wire half already carries each
 * game as it ends — the job's `partial` grows a row every time a game finishes
 * — and once carried that stream to a two-pixel bar and nothing else. A match
 * is the most *theatrical* thing this tool does: real games of Commander,
 * played out, taking minutes. Watching a number climb from 3 to 4 is not
 * watching a match.
 *
 * Both stages here belong to the Coliseum (`routes/Coliseum.tsx`), which is
 * the only room that starts a match. [MatchTheater] is the score — two
 * commanders across an anvil, games landing between them — and [MatchBeats] is
 * the inside of the game currently being played.
 *
 * Three decisions worth keeping.
 *
 * **The stage does not strike itself when the match ends.** It is seeded from
 * `partial.rows` while the job runs and from `result.rows` once it is done,
 * and those are the same rows built by the same `forgeruns._row` — so the
 * pips that lit one at a time are the pips the tale of the tape agrees with,
 * and the moment of victory has somewhere to land. What it is *not* is the
 * record: it shows the last few games, and the full table below it is still
 * the complete one.
 *
 * **Wins are counted here, from the rows, in both phases.** The finished
 * result carries a `wins` tally of its own and reading it when done would
 * mean two derivations that can disagree — the exact drift `_row` and
 * `wire.game_to_wire` were built to make impossible one and two layers down.
 * A clocked-out game lights nobody's pip, which is the ledger's own rule
 * (`_shape` counts it for neither seat) rather than a second opinion about it.
 *
 * **Nothing here mirrors a painting.** The two panels are mirror images of
 * each other — each name sits against its own outer edge, so the two
 * paintings meet in the middle and the commanders converge on the anvil
 * rather than staring off the ends of the screen. That is the whole trick,
 * and it is done with two gradients: no card art is ever flipped to make the
 * subjects face each other. It is somebody's painting, and the layout does
 * the facing.
 *
 * Motion is CSS and gated by `prefers-reduced-motion` in `index.css` beside
 * the rules it turns off, rather than by a `reducedMotion()` read here —
 * `lib/motion.ts` is for motion JavaScript drives (a video, a canvas), and a
 * keyframe that a media query can switch off should be switched off there.
 */

import { useEffect, useRef } from 'react'

import type { DeckSummary, ForgeGameRow } from '../lib/api'
import type { StagedBeat } from '../lib/reel'
import { shortName, turnsTaken } from '../lib/theater'

/** How many games the stage keeps in view. The feed is the last few blows,
 *  not the record — `forge.rows` renders in full below it. */
const FEED = 6

/** The anvil the two of them are arguing across. Drawn rather than fetched,
 *  like the mana symbols' fallback glyphs: four paths, both themes, and no
 *  file to license. */
function Anvil({ size = 30 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 30 30" aria-hidden="true"
         fill="none">
      {/* the horn and body */}
      <path d="M3 11 h18 l4.5 2.5 -4.5 1 v1.5 h-3 l-2 3.5 H10 l-2-3.5 H5 z"
            fill="var(--anvil-iron)" stroke="var(--anvil-brass)"
            strokeWidth="0.9" strokeLinejoin="round" />
      {/* the waist and the block it stands on */}
      <path d="M12 19.5 h6 v3 h-6 z" fill="var(--anvil-iron)"
            stroke="var(--anvil-brass)" strokeWidth="0.9" />
      <path d="M8.5 22.5 h13 v3 h-13 z" fill="var(--anvil-iron)"
            stroke="var(--anvil-brass)" strokeWidth="0.9"
            strokeLinejoin="round" />
      {/* the struck face, catching the light */}
      <path d="M4 11.6 h16.5" stroke="var(--anvil-glint)" strokeWidth="1.1"
            strokeLinecap="round" />
    </svg>
  )
}

/** One commander's side of the table: their painting, their name, and a
 *  track of pips that fills as they take games. */
function Champion({ deck, slug, wins, games, side }: {
  deck: DeckSummary | null
  slug: string
  wins: number
  games: number
  side: 'left' | 'right'
}) {
  // A deck the shelf has not handed us yet still gets a seat and a name —
  // the slug is the name it was submitted under, and an empty panel at the
  // moment the match starts would read as a bug rather than as a wait.
  const name = deck?.name ?? slug
  const commander = deck?.commander?.length
    ? deck.commander.join(' + ') : ''
  const commanderLine = commander && !name.includes(commander) ? commander : ''
  return (
    <div className={`theater-champion theater-champion-${side}`}>
      {deck?.art_crop && (
        <img className="theater-art" src={deck.art_crop} alt="" loading="lazy" />
      )}
      <div className="theater-champion-body">
        <div className="theater-name">{name}</div>
        {/* Only when it is not already the deck's name. A deck is usually
            named for its commander, and printing "Arahbo, Roar of the World"
            under "Arahbo, Roar of the World — Cats" is a caption that repeats
            its picture. A partner pair, or a deck named for what it does,
            still earns the line. */}
        {commanderLine && (
          <div className="theater-commander">{commanderLine}</div>
        )}
        {/* The score is stated in words as well as pips: a count of lit studs
            is a picture, and a picture is not a number anybody can read
            aloud. The pips are decoration over it, and marked as such. */}
        <div className="theater-score tabular">
          {wins} <span className="theater-score-of">
            {wins === 1 ? 'game' : 'games'}
          </span>
        </div>
        <div className="theater-pips" aria-hidden="true">
          {Array.from({ length: games }, (_, i) => (
            <span key={i}
                  className={`theater-pip${i < wins ? ' theater-pip-lit' : ''}`} />
          ))}
        </div>
      </div>
    </div>
  )
}

/**
 * The stage.
 *
 * `rows` is the live feed while the match runs and the finished record
 * afterwards; the caller picks the source, because only the caller knows
 * which phase it is in. Everything else on the stage is derived from those
 * rows, so the two phases cannot describe the same match differently.
 */
export function MatchTheater({ a, b, aSlug, bSlug, games, rows, running }: {
  a: DeckSummary | null
  b: DeckSummary | null
  aSlug: string
  bSlug: string
  games: number
  rows: ForgeGameRow[]
  running: boolean
}) {
  const winsA = rows.filter((r) => r.winner === aSlug).length
  const winsB = rows.filter((r) => r.winner === bSlug).length
  const played = rows.length
  // The gauge reads off the rows rather than off `job.percent`, so it agrees
  // with the pips beside it even on the tick where one has arrived and the
  // other has not. Clamped because a bar past its own end is never the truth
  // anybody wanted to show.
  const heat = games > 0 ? Math.min(100, (played / games) * 100) : 0
  const feed = rows.slice(-FEED).reverse()

  // Short names in the feed: a row is one line and a deck's full name is
  // most of it. The panels above still carry the whole name, so nothing is
  // lost — the feed is just not where a subtitle belongs.
  const name = (slug: string | null) =>
    slug === aSlug ? shortName(a?.name ?? aSlug)
      : slug === bSlug ? shortName(b?.name ?? bSlug)
        : null

  return (
    <section className="theater card-surface rounded-xl p-5">
      <div className="theater-stage">
        <Champion deck={a} slug={aSlug} wins={winsA} games={games} side="left" />
        <div className="theater-anvil">
          <Anvil size={36} />
          <span className="theater-anvil-label tabular">
            {played} / {games}
          </span>
        </div>
        <Champion deck={b} slug={bSlug} wins={winsB} games={games} side="right" />
      </div>

      {/* The forge heat: iron at rest, ember through orange to white at the
          working edge. It is a progress bar and says so to a screen reader —
          the heat is how it looks, not a second meaning. */}
      <div className={`theater-gauge${running ? ' theater-gauge-lit' : ''}`}
           role="progressbar" aria-valuemin={0} aria-valuemax={games}
           aria-valuenow={played}
           aria-label={`${played} of ${games} games played`}>
        <div className="theater-heat" style={{ width: `${heat}%` }} />
      </div>

      <div className="theater-feed">
        {feed.length === 0 ? (
          <p className="theater-quiet">
            {running
              // **Not "half a minute."** That was true of a forge already
              // burning and false of the deployed one, which lights its coals
              // from cold before the first game — measured watching a real
              // match on the instance, where the promise expired long before
              // anything happened. A wait that overruns its own estimate
              // reads as a broken page.
              ? 'The forge is being lit. The first game can take a minute or '
                + 'two, and longer when the coals have gone cold.'
              : 'No games yet.'}
          </p>
        ) : feed.map((r) => (
          // Keyed by the game's own number, which is what makes the strike
          // animation land on arrivals only: a row already on the stage keeps
          // its element across the tick that adds the next one, and React
          // mounts — and so animates — exactly the new one.
          <div key={r.game} className="theater-row">
            <span className="theater-row-n tabular">{r.game}</span>
            <span className="theater-row-who">
              {r.timed_out ? 'called off at the clock'
                : r.draw ? 'a draw'
                  : name(r.winner) ?? 'nobody'}
            </span>
            {/* The player's turns rather than Forge's, which counts each
                seat's turn separately and so reads about double — see
                `turnsTaken`. Titled, because "T8" is a number somebody may
                want to know the units of. */}
            <span className="theater-row-turns tabular"
                  title={r.turns != null
                    ? `${turnsTaken(r.turns)} turns each` : undefined}>
              {r.turns != null ? `T${turnsTaken(r.turns)}` : '—'}
            </span>
            <span className="theater-row-secs tabular">{r.seconds}s</span>
          </div>
        ))}
      </div>

      {/* What a person waiting is owed: how long this is going to take, in
          the units it actually varies in. Kept from the bar this stage
          replaced — a match is minutes, and a screen that goes quiet without
          saying so reads as a screen that has broken. */}
      {running && feed.length > 0 && (
        <p className="theater-quiet" style={{ marginTop: '8px' }}>
          Whole games of Commander, one at a time — a typical game takes a few
          seconds, and a wide board can take two minutes while the pilot
          thinks.
        </p>
      )}
    </section>
  )
}

/**
 * The play-by-play: what happened, in the order it happened.
 *
 * The theater above this counts games. This is the inside of one — the beats
 * Forge raises while it plays, which until now reached a terminal and nowhere
 * else. Three decisions worth keeping.
 *
 * **It is an account, not a board.** Forge's log carries no counters, no token
 * creation and effectively no tapped state (`events.go` counted: five mentions
 * in seven hundred lines), so a reconstructed battlefield would be quietly
 * wrong on exactly the token and counter decks that most want one. A record of
 * what *happened* is something the log can honestly support; a picture of what
 * *is* is not, and the difference matters more than the picture would have
 * been worth.
 *
 * **Newest last, and the list scrolls itself.** A feed that grows upward reads
 * like a chat log and makes a turn's beats run backwards; a game reads down
 * the page. The scroll is pinned to the bottom while the match is live, which
 * is why this takes `running` at all.
 *
 * **A turn is a rule across the column**, not another line. Forge announces
 * turns constantly and "turn 6" is not a beat anybody is watching for — but it
 * is the only structure the log has, and without it a hundred lines of casting
 * and blocking is one undifferentiated wall.
 */
export function MatchBeats({ beats, game, truncated, running }: {
  /** The beats already told, paced by the room's own clock (`lib/reel.ts`).
   *  This component owned that pacing until the board arrived; it does not
   *  any more, because the board watches the same clock and two of them
   *  would drift apart inside a turn. */
  beats: StagedBeat[]
  game: number
  truncated: boolean
  running: boolean
}) {
  // Pinned to the bottom while the match is live: a game reads downward, so
  // the newest beat is the one at the bottom and it is the one to be looking
  // at. Released when the match ends, so a finished game can be scrolled back
  // through without the page fighting for the scrollbar.
  const scroller = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!running) return
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
  }, [beats.length, running])

  return (
    <section className="beats" aria-label="What is happening in the game">
      <header className="beats-head">
        <span className="beats-title">
          {game > 0 ? `Game ${game}` : 'The game'}
        </span>
        {running && (
          <span className="beats-live" aria-hidden="true">
            <span className="beats-live-dot" />
            live
          </span>
        )}
      </header>

      {/* `aria-live` polite rather than assertive, and on the list rather than
          on each line: a screen reader should be able to follow the game
          without every land drop interrupting whatever it was saying. */}
      <div className="beats-scroll" ref={scroller}
           aria-live="polite" aria-relevant="additions">
        {beats.length === 0 ? (
          <p className="theater-quiet">
            {running
              ? 'The first game is being played. There is a wait before the '
                + 'first blow, and then it comes all at once.'
              : 'Nothing has happened yet.'}
          </p>
        ) : beats.map((b) => (
          b.kind === 'turn' ? (
            <div key={b.key} className="beat-turn">
              <span className="beat-turn-n">Turn {b.turn}</span>
              {b.who && <span className="beat-turn-who">{b.who}</span>}
            </div>
          ) : (
            <div key={b.key} className="beat" data-kind={b.kind}>
              {/* `title` for the long ones: the column ellipsises rather than
                  wrapping, because a name on two lines breaks the ledger the
                  whole column reads as. */}
              {b.who && (
                <span className="beat-who" title={b.who}>{b.who}</span>
              )}
              <span className="beat-text">{b.text}</span>
            </div>
          )
        ))}
      </div>

      {truncated && (
        <p className="theater-quiet" style={{ marginTop: '8px' }}>
          This one ran long — the opening is shown and the rest was left out.
        </p>
      )}
    </section>
  )
}
