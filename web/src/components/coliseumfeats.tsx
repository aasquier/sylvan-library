/** The Coliseum's laurels: the three feats the room crowns.
 *
 * The record next door answers *how a deck has fared*, which is a question
 * about averages and needs a band drawn round it before it can be believed.
 * These are the opposite kind of fact and want the opposite treatment: one
 * blow, one creature, one pile, each of them a single thing that happened once
 * on a particular turn of a particular game. Nothing here is a rate, nothing
 * here needs an interval, and a sample size of one is not a weakness — it is
 * the whole point. A hall of fame is a list of moments.
 *
 * **So the top of each board is drawn and the rest is listed.** Ten equal rows
 * would say these are ten comparable measurements; they are not, and the one
 * at the top is the one anybody came to see. The champion gets its painting at
 * a size you can actually look at, and the nine behind it get a thumbnail and
 * a line — which is also what keeps thirty card images off the critical path.
 *
 * **Commandment 19 lives in this file more than anywhere else on the site**,
 * because this is thirty pieces of Wizards' art on one page. Two rules, and
 * neither is negotiable:
 *
 *   1. **No `filter` ever touches a card.** The champion's glint and the
 *      hover lift are both *layers over* the picture — a pseudo-element and an
 *      `.art-lift` — never a filter that re-renders the painting on its way to
 *      the screen. `go/cmd/mtglab/cardimagery_test.go` reads the built bundle
 *      and will fail the build over it, which is exactly why it exists.
 *   2. **Every painting is credited where it hangs.** Artist and printing, in
 *      words, in the same room as the picture — on the champion and on all nine
 *      rows behind it. The server will not hand over an image without them (see
 *      `pool.Art`), so there is no path here that could forget.
 *
 * Commandment 10: the room's own words throughout. Bouts, tables and turns —
 * never a match id, never a row, never anything that computes any of it.
 */

import type { ColiseumArt, ColiseumStandings } from '../lib/api'
import { Section } from './coliseumrecord'

/** The rank, in the room's own numerals. Ten of them, and the board holds
 *  exactly ten (`ledger.TopBlows`), so this is a table rather than an
 *  algorithm — and it answers with the plain figure if the board ever grows,
 *  which is the failure a reader can still read. */
const NUMERALS = ['I', 'II', 'III', 'IV', 'V', 'VI', 'VII', 'VIII', 'IX', 'X']

function numeral(rank: number): string {
  return NUMERALS[rank - 1] ?? String(rank)
}

/** How big the table was, in the words the room uses for it everywhere else. */
function table(seats: number): string {
  return seats > 2 ? `at a table of ${seats}` : 'in a duel'
}

/** When it happened, as the turn a player at that table would call it.
 *
 * **A feat's turn is a count of *player* turns and has to be divided by the
 * number of people playing.** Forge keeps one counter that ticks on everybody's
 * turn, so the seventh turn of a duel arrives as 14 and the thirteenth turn of
 * a pod arrives as 49. Printing that raw is how the board would tell somebody
 * their four-player game ran sixty-four turns.
 *
 * **This deliberately disagrees with the length the room prints on a bout, and
 * the disagreement is Forge's rather than ours.** `GameResult.Turns` halves —
 * always by two, even at four seats — because Forge's own log formatter does,
 * and `scribe.go` argues at length for copying that exactly: the ledger records
 * two paths, and a column meaning rounds on one and player-turns on the other
 * poisons every comparison across a deploy. That argument is about a stored
 * number staying comparable to itself. It says nothing about what a sentence
 * should say to a reader, and "turn 49" is not a true answer to *when* at a
 * table of four. So the stored number is left alone and the sentence divides by
 * the seats that were actually at the table.
 *
 * Rounded up, because the turn somebody is on is not finished yet. */
function turnAt(turn: number, seats: number): string {
  if (turn <= 0) return ''
  return `on turn ${Math.ceil(turn / Math.max(seats, 2))}`
}

/** A deck by the name the library files it under.
 *
 *  **Empty is a real answer here and it is not an error.** A bout recorded
 *  before the room kept seat rosters cannot say whose creature it was, and
 *  saying nothing is the honest version of that — an empty name rendered as
 *  "unknown deck" invents a certainty about the gap. */
function deckLine(slug: string): string {
  return slug === '' ? '' : slug
}

/** One feat, reduced to the four things every board draws the same way: the
 *  card that did it, the figure it is ranked on, the words under the figure,
 *  and the sentence that puts it in a game. */
interface Feat {
  card: string
  /** The ranked number itself — damage, power, a count of tokens. */
  figure: string
  /** What the figure is, in three or four words. */
  unit: string
  /** Where and when, as one sentence with no full stop. */
  where: string
}

/** The painting, with its credit under it — the two halves of one object, and
 *  they are one component precisely so that no caller can draw the first half
 *  without the second (commandment 19).
 *
 *  `eager` is the champion of each board, which is above the fold on every
 *  screen this page has; everything else waits until it is scrolled to. Thirty
 *  card faces fetched at once would be the slowest room on the site. */
function Painting({ card, art, eager }: {
  card: string
  art: ColiseumArt | undefined
  eager?: boolean
}) {
  if (!art) {
    // **A plate rather than nothing.** The room has no painting for this card
    // — an unrefreshed pool, or a token nobody has printed under that name —
    // and a missing picture must not collapse the row it was sitting in, or
    // the list stops lining up exactly where a reader is comparing it.
    return (
      <span className="laurel-art is-bare" aria-hidden="true">
        <span className="laurel-art-bare-mark" />
      </span>
    )
  }
  return (
    <span className="laurel-art">
      <img src={art.image} alt={card} className="laurel-art-face"
           loading={eager ? 'eager' : 'lazy'} decoding="async" />
      {/* The glint, and it is a **layer over** the painting rather than
          anything applied to it. A `filter` here would be the exact fault
          commandment 19 is written about, and the bundle check would fail the
          build over it. */}
      <span className="art-lift laurel-art-glint" aria-hidden="true" />
    </span>
  )
}

/** The credit line. Small, quiet, and never optional. */
function Credit({ art }: { art: ColiseumArt | undefined }) {
  if (!art) return null
  return (
    <span className="laurel-credit">
      Art by {art.artist} · {art.printing}
    </span>
  )
}

/** The one at the top of a board, drawn rather than listed. */
function Champion({ feat, art }: { feat: Feat; art: ColiseumArt | undefined }) {
  return (
    <div className="laurel-champion">
      <Painting card={feat.card} art={art} eager />
      <div className="laurel-champion-body">
        <p className="laurel-champion-card">{feat.card}</p>
        <p className="laurel-champion-where">{feat.where}</p>
        <Credit art={art} />
      </div>
      {/* **The figure is on the right, where every figure on this board is.**
          It began beside the card's name and left two thirds of the plate
          empty, which read as a panel somebody had not finished — and it broke
          the one column a reader actually scans: the nine rows underneath all
          carry their number at the right edge, and the champion's belongs in
          the same place or the eye has to start over at the top. */}
      <p className="laurel-figure">
        <span className="laurel-figure-n tabular">{feat.figure}</span>
        <span className="laurel-figure-unit">{feat.unit}</span>
      </p>
    </div>
  )
}

/** The nine behind the champion. */
function Rest({ feats, art }: {
  feats: Feat[]
  art: Record<string, ColiseumArt>
}) {
  if (feats.length === 0) return null
  return (
    <ol className="laurel-list">
      {feats.map((feat, i) => (
        // The rank is the position, so the key is too: two identical Food
        // stacks of eleven are two real rows and neither is a duplicate of
        // the other.
        <li key={`${feat.card}-${i}`} className="laurel-row">
          <span className="laurel-rank" aria-hidden="true">
            {numeral(i + 2)}
          </span>
          <Painting card={feat.card} art={art[feat.card]} />
          <span className="laurel-row-body">
            <span className="laurel-row-card">{feat.card}</span>
            <span className="laurel-row-where">{feat.where}</span>
            <Credit art={art[feat.card]} />
          </span>
          {/* **The unit is spoken and not drawn.** Written out, "damage at
              once" ran down all nine rows in the same grey — nine copies of a
              phrase the board's own heading and its champion have both already
              said. A screen reader has neither of those in earshot by the time
              it reaches row seven, so it still gets the words. */}
          <span className="laurel-row-figure">
            <span className="tabular">{feat.figure}</span>
            <span className="sr-only"> {feat.unit}</span>
          </span>
        </li>
      ))}
    </ol>
  )
}

/** One whole board: its name, its argument, its champion and its list. */
function Board({ heading, blurb, feats, art }: {
  heading: string
  blurb: string
  feats: Feat[]
  art: Record<string, ColiseumArt>
}) {
  const [champion, ...rest] = feats
  // A board nobody has done anything on is not drawn at all — an empty
  // "killing blow" heading over nothing reads as a fault rather than as a
  // room where nobody has been killed yet.
  if (!champion) return null
  return (
    <Section heading={heading} blurb={blurb}>
      <Champion feat={champion} art={art[champion.card]} />
      <Rest feats={rest} art={art} />
    </Section>
  )
}

/** The room before anybody has done anything worth writing down.
 *
 *  Not an error and not dressed as one — it is the true state of a new room,
 *  and it says what to do about it in one sentence (the record's own rule for
 *  the same moment). */
function NothingYet() {
  return (
    <div className="record-empty">
      <p className="record-empty-lead">No laurels have been awarded yet.</p>
      <p className="record-empty-body">
        Nothing has happened on this sand that anybody would tell a story
        about — no blow that ended a game, no creature big enough to be
        remembered, no pile of tokens tall enough to count out loud. Send a
        table out onto the field above and the room starts keeping the three
        of them: the hardest hit, the largest creature, and the deepest stack.
      </p>
    </div>
  )
}

export function ColiseumFeats({ board }: { board: ColiseumStandings }) {
  // **Every list is defaulted, and that is not defensive noise.** These three
  // and the paintings beside them arrive from a server that may be older than
  // this bundle by a deploy — the two halves go out separately and the browser
  // is the half that keeps the old one — so a key that is not there yet must
  // read as an empty board rather than a crash.
  const blows = board.blows ?? []
  const giants = board.giants ?? []
  const stacks = board.stacks ?? []
  const art = board.art ?? {}

  if (blows.length === 0 && giants.length === 0 && stacks.length === 0) {
    return <NothingYet />
  }

  const blowFeats: Feat[] = blows.map((b) => ({
    card: b.card,
    figure: String(b.amount),
    unit: 'damage at once',
    where: [
      // **The whole swing, not the last damage line.** Forge announces combat
      // damage one source at a time, so a ten-creature alpha strike arrives as
      // ten separate lines and the last of them is usually a 2/2. The board
      // ranks the swing; this sentence says how many cards were in it, because
      // "57 damage from Blightsteel Colossus" alone reads as one enormous
      // creature rather than the team that actually swung.
      b.sources > 1 ? `with ${b.sources} cards in the swing` : 'on its own',
      b.combat ? 'in combat' : 'out of combat',
      deckLine(b.victim) && `${b.victim} fell`,
      table(b.seats),
      turnAt(b.turn, b.seats),
    ].filter(Boolean).join(' · '),
  }))

  const giantFeats: Feat[] = giants.map((g) => ({
    card: g.card,
    // Power and toughness the way a player says them out loud, and ranked on
    // the first number for the same reason: "a 15/15" leads with the power.
    figure: `${g.power}/${g.toughness}`,
    unit: 'on the battlefield',
    where: [deckLine(g.deck), table(g.seats), turnAt(g.turn, g.seats)]
      .filter(Boolean).join(' · '),
  }))

  const stackFeats: Feat[] = stacks.map((s) => ({
    card: s.card,
    figure: String(s.count),
    unit: 'held at once',
    where: [deckLine(s.deck), table(s.seats), turnAt(s.turn, s.seats)]
      .filter(Boolean).join(' · '),
  }))

  return (
    <div className="laurel-sheet">
      <p className="record-caution">
        Three things the room writes down whoever does them. These are not
        records of how well a deck plays — the record next door is where that
        question is answered, carefully. These are the moments: the hardest a
        table has ever been hit, the largest creature to stand on the sand, and
        the deepest pile of one token anybody has held at once.
      </p>

      <Board
        heading="The killing blow"
        blurb="The hardest a player has ever been hit in one go — the whole
               swing, not the last card in it, which is why a wall of small
               creatures can out-hit a single enormous one."
        feats={blowFeats}
        art={art}
      />

      <Board
        heading="The largest creature"
        blurb="Ranked on power, because that is the number a player says out
               loud. It had to be standing on the battlefield: a creature that
               was only ever huge on the stack was never really there."
        feats={giantFeats}
        art={art}
      />

      <Board
        heading="The deepest stack"
        blurb="Tokens of one kind, held all at once. A deck that makes forty
               Food across a long game and eats each one never had a stack —
               this is the pile that was actually on the table."
        feats={stackFeats}
        art={art}
      />
    </div>
  )
}
