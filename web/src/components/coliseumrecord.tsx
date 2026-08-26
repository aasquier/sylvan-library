/** The Coliseum's record: what the room remembers of the bouts fought in it.
 *
 * **Why this draws a band and not a bar.** Every other board on this site is a
 * bar chart, and a bar chart was the obvious thing to reach for. It is also
 * the one visual that cannot tell this story: a bar of "100%" beside a bar of
 * "62%" says the first deck is better, and if the first deck has fought twice
 * that is a lie drawn to scale. The band is the fix — it shows the whole range
 * a deck's true form could sit in, so two bouts draw an enormous smear and
 * sixty-five draw a tight one, and the reader learns the difference without
 * anybody having to use the word for it. A newcomer sees "we don't really know
 * yet" at a glance, which is exactly commandment 2's ask.
 *
 * That is also why this file does not import the chart module: what it needs
 * is a forest plot, and pulling 113kB of charting library in to draw ten
 * horizontal rules would cost the room its load time for a worse picture.
 *
 * **Nothing here recomputes a rate.** The server withholds one below the floor
 * on purpose, and `wins / played` in this file would put the two-game win rate
 * straight back on the screen. When `rate` is null the counts are rendered and
 * the row says what it is short — that is the whole point of the null.
 *
 * Commandment 10: this room is a coliseum and nothing that computes it is ever
 * named. Bouts, not games; the house, not a server; a record, not a dataset.
 */

import { useMemo, useState } from 'react'
import type {
  ColiseumClassRecord,
  ColiseumDeckRecord,
  ColiseumMeeting,
  ColiseumRecord,
  ColiseumStandings,
} from '../lib/api'

/** The rate as a whole number of percent. Never called with a null rate. */
function pct(v: number): string {
  return `${Math.round(v * 100)}%`
}

/** The commander is a Commander deck's real name; the slug is how the library
 *  files it. Partners are two, and both are the deck. */
function titleOf(commander: string[], slug: string): string {
  return commander.length > 0 ? commander.join(' & ') : slug
}

/** What the row says instead of a rate, and it is deliberately a sentence
 *  rather than a dash: a dash reads as missing data, and this is not missing —
 *  it is the honest answer to a question that cannot be answered yet. */
function shortfall(record: ColiseumRecord, floor: number): string {
  const need = floor - record.played
  if (record.played === 0) return 'no bouts yet'
  return need === 1 ? 'one more bout to call it' : `${need} more bouts to call it`
}

/** The band: where a contestant's true form sits, given what it has shown.
 *
 * The span is the range; the mark is the record so far. An even break is drawn
 * behind both, because "better or worse than even" is the only question most
 * readers are actually asking, and a band that straddles it is a deck nobody
 * can call yet however good its record looks. */
function Band({ record, floor }: { record: ColiseumRecord; floor: number }) {
  const lo = record.lower * 100
  const hi = record.upper * 100

  // **The band draws itself in, and the drawing is CSS's alone.** It used to
  // mount at zero width and grow on the frame after, which is the ordinary
  // React way to get a transition out of a first render — and it puts the
  // *information* behind a callback. A tab that is not being looked at does
  // not run animation frames, so the band would sit at zero width until
  // somebody focused it, and any future path that suspended the effect would
  // leave an empty track that looks exactly like a deck with no record.
  //
  // A `scaleX` keyframe needs nothing from JavaScript and cannot know or care
  // what the width is, so the width is simply true from the first paint and
  // the motion is decoration over the top of it. It also lands the rule inside
  // the reduced-motion sweep (`cmd/mtglab/reducedmotion_test.go` reaches an
  // `animation:` declaration by the classes in its selector), which a
  // JS-driven transition could never be checked by.
  const label =
    record.rate === null
      ? `Somewhere between ${pct(record.lower)} and ${pct(record.upper)} of bouts` +
        ` — ${shortfall(record, floor)}`
      : `${pct(record.rate)} of bouts won, and the true share sits somewhere` +
        ` between ${pct(record.lower)} and ${pct(record.upper)}`

  return (
    <div className="record-band" role="img" aria-label={label}>
      <div className="record-band-track" aria-hidden="true" />
      <div className="record-band-even" aria-hidden="true" />
      <div
        className={`record-band-span${record.settled ? ' is-settled' : ''}`}
        aria-hidden="true"
        style={{ left: `${lo}%`, width: `${Math.max(hi - lo, 0.8)}%` }}
      />
      {record.rate !== null && (
        <div className="record-band-mark" aria-hidden="true"
             style={{ left: `${record.rate * 100}%` }} />
      )}
    </div>
  )
}

/** The counts, which are always true, in the room's own words. */
function Tally({ record }: { record: ColiseumRecord }) {
  return (
    <span className="record-tally">
      <span className="tabular">{record.wins}</span>
      <span className="record-tally-of">of</span>
      <span className="tabular">{record.played}</span>
      <span className="record-tally-of">
        {record.played === 1 ? 'bout' : 'bouts'}
      </span>
      {record.draws > 0 && (
        <span className="record-tally-aside">
          · {record.draws} drawn
        </span>
      )}
      {record.timed_out > 0 && (
        <span className="record-tally-aside"
              title="Bouts the hourglass ran out on. They count for nobody.">
          · {record.timed_out} unfinished
        </span>
      )}
    </span>
  )
}

/** The right-hand figure: a rate, or the reason there isn't one. */
function Figure({ record, floor }: { record: ColiseumRecord; floor: number }) {
  if (record.rate === null) {
    return <span className="record-figure is-shy">{shortfall(record, floor)}</span>
  }
  return (
    <span className={`record-figure${record.settled ? '' : ' is-young'}`}>
      <span className="tabular">{pct(record.rate)}</span>
      {!record.settled && <span className="record-provisional">still proving</span>}
    </span>
  )
}

function Row({
  title,
  under,
  record,
  floor,
}: {
  title: string
  under: React.ReactNode
  record: ColiseumRecord
  floor: number
}) {
  return (
    <li className="record-row">
      <div className="record-who">
        <span className="record-name">{title}</span>
        <span className="record-under">{under}</span>
      </div>
      <div className="record-figures">
        <Tally record={record} />
        <Figure record={record} floor={floor} />
      </div>
      <Band record={record} floor={floor} />
    </li>
  )
}

function Section({
  heading,
  blurb,
  children,
}: {
  heading: string
  blurb: string
  children: React.ReactNode
}) {
  return (
    <section className="mt-8">
      <h2 className="mb-1 border-b pb-2 text-sm font-semibold uppercase
                     tracking-[0.12em] text-[var(--text-muted)]"
          style={{ borderColor: 'var(--hairline)' }}>
        {heading}
      </h2>
      <p className="mb-3 mt-2 text-[0.8rem] leading-relaxed text-[var(--text-muted)]">
        {blurb}
      </p>
      {children}
    </section>
  )
}

function DeckBoard({ decks, floor }: { decks: ColiseumDeckRecord[]; floor: number }) {
  return (
    <ul className="record-board">
      {decks.map((d) => (
        <Row
          key={`${d.owner_id ?? 'house'}/${d.slug}`}
          title={titleOf(d.commander, d.slug)}
          under={
            <>
              {d.slug}
              {d.archetype && <> · {d.archetype}</>}
              {' · '}
              {d.matches === 1 ? 'one outing' : `${d.matches} outings`}
            </>
          }
          record={d.record}
          floor={floor}
        />
      ))}
    </ul>
  )
}

function ClassBoard({
  classes,
  floor,
}: {
  classes: ColiseumClassRecord[]
  floor: number
}) {
  return (
    <ul className="record-board">
      {classes.map((c) => (
        <Row
          key={c.archetype}
          title={c.archetype}
          under={c.decks === 1 ? 'one deck' : `${c.decks} decks`}
          record={c.record}
          floor={floor}
        />
      ))}
    </ul>
  )
}

function MeetingBoard({
  meetings,
  floor,
}: {
  meetings: ColiseumMeeting[]
  floor: number
}) {
  return (
    <ul className="record-board">
      {meetings.map((m) => (
        <Row
          key={`${m.a.owner_id ?? 'house'}/${m.a.slug}|${m.b.owner_id ?? 'house'}/${m.b.slug}`}
          title={`${titleOf(m.a.commander, m.a.slug)} vs ${titleOf(m.b.commander, m.b.slug)}`}
          under={
            <>
              {m.a_wins}–{m.b_wins}
              {m.draws > 0 && <>–{m.draws}</>}
              {' · '}
              {m.matches === 1 ? 'met once' : `met ${m.matches} times`}
            </>
          }
          record={m.record}
          floor={floor}
        />
      ))}
    </ul>
  )
}

/** The room before anybody has fought in it, which is what almost every
 *  visitor sees first and is therefore the state that matters most.
 *
 *  It is not an error and does not wear an error's clothes: no warning colour,
 *  no empty-box iconography. The room is simply new, and the copy says what to
 *  do about it in one sentence. */
function NothingYet() {
  return (
    <div className="record-empty">
      <p className="record-empty-lead">The sand has not been turned yet.</p>
      <p className="record-empty-body">
        No bouts have been fought here — so there is nothing to weigh, and the
        house would rather tell you that than invent a champion. Send two decks
        out onto the field above and the record begins keeping itself: how each
        deck fares, how each way of playing fares, and who has the measure of
        whom.
      </p>
    </div>
  )
}

/** The house's own tally across everything visible. */
function Header({ board }: { board: ColiseumStandings }) {
  const decided = board.games - board.timed_out
  return (
    <div className="record-header">
      <div className="record-stat">
        <span className="record-stat-figure tabular">{board.games}</span>
        <span className="record-stat-label">
          {board.games === 1 ? 'bout fought' : 'bouts fought'}
        </span>
      </div>
      <div className="record-stat">
        <span className="record-stat-figure tabular">{board.matches}</span>
        <span className="record-stat-label">
          {board.matches === 1 ? 'outing' : 'outings'}
        </span>
      </div>
      <div className="record-stat">
        <span className="record-stat-figure tabular">{board.decks.length}</span>
        <span className="record-stat-label">
          {board.decks.length === 1 ? 'deck on the sand' : 'decks on the sand'}
        </span>
      </div>
      {board.timed_out > 0 && (
        <div className="record-stat">
          <span className="record-stat-figure tabular">{board.timed_out}</span>
          <span className="record-stat-label">
            ran out of hourglass · {decided} decided
          </span>
        </div>
      )}
    </div>
  )
}

export function ColiseumRecord({ board }: { board: ColiseumStandings }) {
  // Something to say only when there is something in it. A board with matches
  // but no decks cannot happen, but a nil-safe read costs nothing.
  const empty = board.matches === 0 || board.decks.length === 0
  const floor = board.floor

  // How many are still short of a callable record. Named once here so the
  // note below and the sections agree, and so the copy can be specific
  // instead of hedging at everybody.
  const shy = useMemo(
    () => board.decks.filter((d) => d.record.rate === null).length,
    [board.decks],
  )

  // The legend is opened by the reader, not forced on them: most people read
  // the bands correctly without it, and the ones who want the rule stated can
  // ask for it.
  const [legend, setLegend] = useState(false)

  if (empty) return <NothingYet />

  return (
    <div className="record-sheet">
      <Header board={board} />

      <p className="record-caution">
        A record is only ever as good as the bouts behind it, so the house shows
        you both. The bar behind each line is the range a deck&apos;s true form
        could sit in — a wide one means the sand has not decided yet, however
        good the tally looks.{' '}
        {shy > 0 && (
          <>
            {shy === 1 ? 'One contestant is' : `${shy} contestants are`} still
            short of {floor} bouts, and {shy === 1 ? 'it is' : 'they are'} shown
            as a tally rather than a share on purpose.{' '}
          </>
        )}
        <button type="button" className="btn btn-quiet btn-sm align-baseline"
                aria-expanded={legend} onClick={() => setLegend((v) => !v)}>
          {legend ? 'Hide the reading' : 'How to read this'}
        </button>
      </p>

      {legend && (
        <div className="record-legend">
          <p>
            <strong>The tally</strong> is what happened: bouts won of bouts
            fought. It is never wrong and never rounded.
          </p>
          <p>
            <strong>The band</strong> is what the tally supports. It runs from
            the worst to the best the record honestly allows. The dot on it is
            the share won so far, and the faint line down the middle is an even
            break — a band straddling that line is a deck that has not yet shown
            it is better or worse than its opposition.
          </p>
          <p>
            <strong>The order</strong> is by the modest end of the band, not by
            the share won. That is why a deck that won both its bouts sits below
            one that won two thirds of sixty — winning twice is a good evening,
            not a reputation.
          </p>
          <p>
            <strong>Unfinished bouts</strong> are ones the hourglass ran out on
            before either deck could close it. They count for nobody and are
            kept apart from the tally rather than quietly folded in.
          </p>
        </div>
      )}

      <Section
        heading="The decks"
        blurb="Every deck that has taken the sand, and how it has fared across
               everything it has fought."
      >
        <DeckBoard decks={board.decks} floor={floor} />
      </Section>

      {board.archetypes.length > 0 && (
        <Section
          heading="The ways of playing"
          blurb="Grouped by the way each deck was playing at the time, not by
                 how it is labelled today — a deck rebuilt into something else
                 does not rewrite what it once was."
        >
          <ClassBoard classes={board.archetypes} floor={floor} />
        </Section>
      )}

      {board.meetings.length > 0 && (
        <Section
          heading="Head to head"
          blurb="Only bouts fought one against one. In a crowded field nobody
                 beat anybody in particular, so those bouts count on the boards
                 above and not here."
        >
          <MeetingBoard meetings={board.meetings} floor={floor} />
        </Section>
      )}
    </div>
  )
}
