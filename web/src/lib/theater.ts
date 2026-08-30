/**
 * The match theater's pure functions, in `lib/` for the reason `lib/motion.ts`
 * gives at the top of its own file: oxlint's fast-refresh rule is right, these
 * are not components, and each of them is needed by more than one file.
 * `theaterRows` and `theaterBeats` are called by the Coliseum (which owns the
 * job) and `shortName`, `legendName` and `beatLine` by the stage (which owns
 * the row and the beat); putting any of them in `components/theater.tsx` costs
 * that file its fast refresh to save an import.
 *
 * The two `theater*` readers are narrowings of `Job.partial`, which is
 * `unknown` because the shape belongs to the job's kind. Somebody has to do
 * that once, in a way that is total over every phase the job passes through —
 * so both of them answer "nothing yet" for a job that has not ticked, a worker
 * too old to send that half, and a finished job whose partial the server has
 * cleared.
 */

import type { ForgeBeat, ForgeBeats, ForgeBoard, ForgeGameRow }
  from './api'

/** The Forge's `partial` payload, narrowed.
 *
 * `Job.partial` is `unknown` because the shape belongs to the job's kind, so
 * somebody has to do this once. It is deliberately total: a job that has not
 * ticked yet, a pre-theater worker streaming counts with no rows at all (the
 * skew `worker.run_match` tolerates on purpose), and a finished job whose
 * partial the server has cleared all arrive here and all mean "no rows yet".
 */
export function theaterRows(partial: unknown): ForgeGameRow[] {
  if (!partial || typeof partial !== 'object') return []
  const rows = (partial as { rows?: unknown }).rows
  return Array.isArray(rows) ? (rows as ForgeGameRow[]) : []
}

/** What to call a deck in a line that has to stay one line.
 *
 * A deck is *usually* named for its commander and then for what it does —
 * "Arahbo, Roar of the World — Cats" — and a feed row wants the first part of
 * that: the general's name, which is how anybody actually refers to the deck
 * out loud. The dash is a real separator in that title and everything after it
 * goes.
 *
 * **The comma is not, and that was this function's bug.** It used to cut at
 * the first comma unconditionally, on the assumption that a deck's name is
 * always "Commander, Epithet — Theme". That is *card-name* grammar — a legend
 * really does carry a comma between name and title, which is what
 * [legendName] below is for — applied to prose somebody wrote freely. Aaron's
 * Atla Palani deck is called **"Life, Uh, Finds a Way"**, and every surface in
 * the Coliseum called it *"Life"*: the dropdown he picks it with, the tape,
 * the feed. The commas in that title are commas in a sentence, and a separator
 * that occurs inside the data is not a separator — the house learned this once
 * already, when the importer split a commander field on its commas.
 *
 * So the cut is decided by lookup rather than by punctuation: the comma is a
 * separator **only when the text before it is the deck's own general**, which
 * is the case the shortening was written for and the only case it is right
 * for. Anything else is a title that happens to contain a comma and comes back
 * whole. Matched case-insensitively and against the general's own pre-comma
 * head, so "Atla Palani, Nest Tender — Dinos" shortens under a commander
 * recorded as "Atla Palani, Nest Tender".
 *
 * `commander` takes the deck row's whole `commander` list because that is the
 * shape every caller holds, and a partner pair is two chances to be named for
 * your general rather than one. **Absent or unmatched, the whole pre-dash
 * title is the answer** — the safe direction: a name that is longer than it
 * needed to be is untidy, and a name cut down to somebody else's word is
 * wrong.
 */
export function shortName(
  name: string, commander?: string | readonly string[] | null,
): string {
  const beforeDash = (name.split('—')[0] ?? name).trim() || name
  const comma = beforeDash.indexOf(',')
  if (comma < 0) return beforeDash
  const head = beforeDash.slice(0, comma).trim()
  if (!head) return beforeDash
  const generals = typeof commander === 'string' ? [commander] : commander ?? []
  const namedForIt = generals.some((general) =>
    (general.split(',')[0] ?? general).trim().toLowerCase()
      === head.toLowerCase())
  return namedForIt ? head : beforeDash
}

/** What to call a *card* whose name carries a title.
 *
 * The unconditional comma cut [shortName] used to be, kept for the job it was
 * always correct at: Magic prints a legend as "Brimaz, King of Oreskos", the
 * comma is the printed separator between the name and the title, and a board
 * that lists two of them reads *"Brimaz, King of Oreskos, Arahbo, Roar of the
 * World"* — four names to a reader and two to the game.
 *
 * The distinction against [shortName] is the whole point of there being two
 * functions: **a card's name is a form and a deck's name is prose.** One may
 * be split on its punctuation because Wizards put the punctuation there; the
 * other may not, because a person did.
 */
export function legendName(name: string): string {
  const beforeDash = name.split('—')[0] ?? name
  return (beforeDash.split(',')[0] ?? beforeDash).trim() || name
}

/** The Forge's `partial.beats`, narrowed — the newest game's narration, or
 *  null.
 *
 * Total for `theaterRows`'s reasons and two more of its own: a match nobody
 * asked to narrate carries `beats: null` for its whole length, and a shim
 * deployed a few minutes behind the app never sends the line at all. Both
 * arrive here and both mean "no beats", which is a quiet room rather than a
 * broken one.
 */
export function theaterBeats(partial: unknown): ForgeBeats | null {
  if (!partial || typeof partial !== 'object') return null
  const beats = (partial as { beats?: unknown }).beats
  if (!beats || typeof beats !== 'object') return null
  const { game, beats: list } = beats as { game?: unknown; beats?: unknown }
  if (typeof game !== 'number' || !Array.isArray(list)) return null
  const board = (beats as { board?: unknown }).board
  return {
    game,
    beats: list as ForgeBeat[],
    truncated: (beats as { truncated?: unknown }).truncated === true,
    // Null for a match played by a worker without the scribe, which is a room
    // that draws the account alone — the same degrade every hop of this wire
    // makes, and the reason this reader is total rather than trusting.
    board: (board && typeof board === 'object') ? (board as ForgeBoard) : null,
  }
}

/** One beat as a line of English.
 *
 * The vocabulary began as the CLI's (`narrateGame` in `cmd/mtglab/sim.go`), and
 * the two saying one thing is the point rather than a coincidence: the terminal
 * account exists so a person can read exactly what the browser will be handed,
 * and a beat that reads differently in the two places makes that check a lie.
 * **The browser has since outrun it** — `enters`, `attach`, `exiled`,
 * `sacrificed`, `ability` and `companion` all have a sentence here and no case
 * in the terminal, which quietly says nothing at all for six of the kinds the
 * scribe raises. That is a gap in the CLI rather than a licence to drift here;
 * a sentence added below should be added there too when Go is next open.
 *
 * `name` turns a deck's slug into whatever the room calls it. It is passed in
 * because only the room knows — the beat carries a slug, and the shelf carries
 * the name that slug belongs to.
 *
 * **An unrecognised kind is rendered, never dropped.** Forge is not an API and
 * a release that adds a beat should read as a plain sentence here rather than
 * as a gap in the game.
 *
 * **Every new beat kind needs a case here.** The default is a safety net, not
 * a destination: it prints the kind's own name, which is the wire showing
 * through to a user (commandment 10). `enters` and `attach` sat in it for as
 * long as the scribe has existed, `ability` sat in it for a release after it
 * started firing forty-odd times a game, and `exiled` was written into this
 * file in the same change that raised it — which is the habit worth keeping.
 * Anything the scribe learns to report next — mana, populate — arrives in
 * exactly the same state and needs a sentence added below before it is on
 * screen.
 *
 * **`who` is read now; `text` still is not.** This comment used to say that
 * nothing rendered any of it and that whether it came back was Aaron's call.
 * He made the call — *"It would be nice if we added the players name too,
 * Gyome CASTS Creature"* — and half of it came back: `who` and `target` now
 * stand on the plate under the card in the middle of the arena
 * (`lib/stage.ts`, `components/stage.tsx`). The **sentence** did not, and the
 * reason is that the plate is not a line of prose. It is a museum label read in
 * about a second — a name, a deed, a card — so it builds its own short phrase
 * out of the manner and the card's type line, and `text`'s "casts Lightning
 * Bolt" would print the card's name a second time under a picture of it.
 *
 * So `text` is still correct and unread, and it is still worth keeping right:
 * the play-by-play #328 removed is expected back, this is what it will read,
 * and a reader that is wrong while nobody is looking is a reader that is wrong
 * on the day somebody looks.
 *
 * **`who` is the player the sentence is *about*, and a death has none.** That
 * is the one thing to hold on to when adding a case: `dies`, `exiled` and
 * `damage` deliberately return `who: null`, because a creature dying is not
 * something its controller did and a plate reading "Gyome dies" would be the
 * room saying the wrong thing about the wrong player. The seat is still on the
 * wire for anything that needs it; it simply is not the subject of this
 * sentence.
 */
export function beatLine(beat: ForgeBeat, name: (slug: string) => string):
  { who: string | null; text: string } {
  const who = beat.who ? name(beat.who) : null
  const them = beat.against ? name(beat.against) : null
  const card = beat.card || 'something'
  switch (beat.kind) {
    case 'turn':
      return { who, text: 'takes the turn' }
    case 'mulligan':
      return { who, text: `keeps ${beat.amount ?? 0}` }
    case 'land':
      return { who, text: `plays ${card}` }
    case 'cast':
      return { who, text: `casts ${card}` }
    case 'resolve':
      return { who, text: `${card} resolves` }
    // The two the scribe reports and this reader never had a case for. Both
    // fire in every modern match, so without these every one of them fell to
    // the default and read as its own kind followed by a card name — the wire
    // showing through, which is exactly what commandment 10 forbids and what
    // `lib/claudecopy.ts` exists to prevent one surface over.
    //
    // Cast and enters are two different moments and Magic says so: a spell is
    // cast, and the permanent it becomes *enters*. A token or a land never had
    // a cast at all, which is why folding this into `cast` would be wrong
    // rather than merely redundant.
    case 'enters':
      return { who, text: `${card} enters the battlefield` }
    // Aura or Equipment, and the board draws the same fact as gear on the
    // permanent carrying it. "goes on" rather than "is attached to": attached
    // is the rulebook's word, and this line is read by somebody watching their
    // first game.
    case 'attach':
      return { who, text: `puts ${card} on ${beat.target || 'a permanent'}` }
    // **A companion is bought in, and it was never dealt.** Aaron watched a
    // match, saw Kaheera land in a hand and thought the engine was cheating —
    // *"you don't shuffle your companion in with normal cards to be dealt,
    // they come from outside the game like the commander does"*. He was right
    // about the rules and Forge was right about the game: the card starts in a
    // command zone, and its controller pays {3} to put it in their hand. The
    // room drew that arrival and said nothing at all about it, which is a
    // beginner being shown a game that cheats (commandment 2).
    //
    // **"outside the game" rather than "the command zone"**, which is the same
    // choice `attach` makes two cases down. Both are true; one of them is what
    // a person watching needs, and the other is a zone name that means nothing
    // until you already know the answer.
    //
    // **The three is stated, not read.** Rule 702.139b fixes the cost at {3}
    // for every companion there has ever been, and the scribe deliberately
    // attributes no mana to the ability — so this is the rulebook speaking,
    // never a number inferred from whichever lands happened to tap.
    case 'companion':
      return { who, text: `calls in ${card} from outside the game` }
    // A cost paid rather than something that happened to the card, which is
    // why it is not `dies` — rule 700.4 gives that word to a creature or
    // planeswalker put into a graveyard, and a Treasure cracked for mana does
    // neither. The player is the subject here for exactly that reason: a
    // sacrifice is a thing somebody chose.
    //
    // **And the wire does not say which player yet.** Measured on a live match
    // (2026-08-27): the scribe's `sacrificed` line carries no seat, so `who` is
    // null on every one of these and the sentence arrives with its subject
    // missing. That is a hole in Go rather than a reason to write a different
    // sentence here — the day the seat crosses, this reads correctly with no
    // change, and until then the plate on the centre stage falls back to its
    // nameless form rather than naming the wrong person.
    case 'sacrificed':
      return { who, text: `sacrifices ${card}` }
    // **Activated and triggered are two different sentences**, and the wire
    // already knows which — `trigger` is the scribe reading Forge's own flag.
    // An ability somebody *activated* is a thing they did, so the player is
    // the subject; one the game *triggered* happened by itself, so the card
    // is, exactly as it is for a death. Composing a trigger as "<player>
    // <card> triggers" would put two subjects in one sentence.
    //
    // Forty-odd of these a game — measured on a real match, against fifteen
    // casts — so this is the commonest kind in the whole stream, and it spent
    // a release falling to the default and printing its own wire name.
    case 'ability':
      return beat.trigger
        ? { who: null, text: `${card} triggers` }
        : { who, text: `uses ${card}` }
    case 'attack':
      return { who, text: `attacks ${them ?? 'across the table'} with ${card}` }
    case 'block':
      return { who, text: `blocks ${beat.target || 'the attacker'} with ${card}` }
    case 'unblocked':
      return { who, text: `lets ${card} through` }
    case 'damage':
      return {
        who: null,
        text: `${card} deals ${beat.amount ?? 0} to ${beat.target || them || 'somebody'}`,
      }
    case 'life':
      return { who, text: `is at ${beat.life ?? 0}` }
    case 'dies':
      return { who: null, text: `${card} is destroyed` }
    // Exile's own word, and it is worth not reaching for a synonym: "exiled"
    // is what the card says, what a player says, and — the part that matters
    // for commandment 2 — the thing a newcomer will need to look up exactly
    // once. "Removed" or "banished" would be kinder for one sentence and would
    // leave them unable to read the card that did it.
    case 'exiled':
      return { who: null, text: `${card} is exiled` }
    case 'outcome':
      // Forge writes all nine of its outcome sentences to follow "<player> has
      // won/lost", so the note is already a sentence's tail and is rendered
      // whole rather than re-grammared. "has lost due to accumulation of 21
      // damage from generals" is the loss condition this format is named for,
      // and it only reads correctly if nothing tries to improve it.
      return { who, text: beat.note || 'is finished' }
    default:
      // A beat this browser has never heard of still names its card, which is
      // more useful than nothing and honest about being less than a sentence.
      return { who, text: beat.card ? `${beat.kind}: ${beat.card}` : beat.kind }
  }
}

/**
 * Forge's turn number is not the turn number anybody says out loud.
 *
 * Measured rather than assumed (2026-08-25, `gyome-food` vs
 * `atla-palani-dinos`): Forge increments once per *player-turn* and alternates
 * strictly, so a heads-up game reads
 *
 *     turn 1  seat 1      turn 3  seat 1      turn 15  seat 1
 *     turn 2  seat 2      turn 4  seat 2
 *
 * and its "turn 15" was the first player's **eighth** turn. Every game length
 * this app reported was therefore about double what a person would count —
 * "T15" beside a Commander game that took eight turns, which reads as a
 * marathon and was not one. Magic's rules agree with Forge (each player's turn
 * is its own turn); Magic *players* do not, and the room is for players.
 *
 * **Counted per seat rather than divided by two**, which matters exactly when
 * somebody takes an extra turn: Time Warp gives one player turns 7 and 8 back
 * to back, and halving would credit the opponent with a turn they never took.
 * Counting the turn beats each seat actually had is right in both cases.
 *
 * The wire is left alone — `ForgeGameRow.turns` and `ForgeBeat.turn` carry
 * Forge's own number, because the match ledger records it and the frozen
 * corpus pins its bytes. The translation belongs here, at the last moment,
 * for the same reason `errorMessage` strips a class name here.
 */
export function playerTurns(beats: ForgeBeat[]): Map<number, number> {
  const seen = new Map<string, number>()
  const out = new Map<number, number>()
  for (const beat of beats) {
    if (beat.kind !== 'turn' || beat.turn == null) continue
    // A turn beat with no player is a turn nobody can be credited with; it
    // keeps Forge's number rather than borrowing the last player's count.
    const who = beat.who ?? ''
    const next = (seen.get(who) ?? 0) + 1
    seen.set(who, next)
    out.set(beat.turn, next)
  }
  return out
}

/** How many turns a finished game took, as a player would count them.
 *
 * **Which is what the row already says**, and this function used to halve it a
 * second time. The correction, measured 2026-08-25 and proven three ways:
 *
 * - a recorded narration holds seventeen `Turn: Turn N` lines and one
 *   `Game Outcome: Turn 9`;
 * - Forge's own `GameLogFormatter` renders that line as
 *   `Math.ceil(ev.lastTurnNumber() / 2.0)`, read out of the bytecode;
 * - a live match reported 23 through the log against 46 raw on the bus.
 *
 * So Forge halves it itself. `ForgeGameRow.turns` is read off that line, has
 * always been **rounds**, and halving it here rendered a nine-turn game as
 * "T5" — the same mistake as the one this was written to fix, in the other
 * direction. The row is now passed through.
 *
 * `ForgeBeat.turn` is different and still needs converting: a beat carries the
 * `Turn: Turn N` number, which really is a player-turn, and `playerTurns`
 * above counts those per seat. Two numbers, two meanings, one of them already
 * done for us.
 */
export function turnsTaken(turns: number | null): number | null {
  return turns
}

/** Where each of a bout's turns begins, as a count of beats **told**.
 *
 * `i + 1` rather than `i`, and that is the whole subtlety: `seek` takes a
 * told-count, so landing on the marker's own index would stop the instant
 * *before* the turn is announced. Landing one past it puts the announcement on
 * the board as the last thing said, which is what "step to the turn" means to
 * somebody watching.
 *
 * **These are player-turns.** Forge prints a turn line per seat and alternates
 * them, so consecutive marks are one player's turn apart — the unit Aaron
 * asked for ("a player's turn at a time, not a full two player turn") and the
 * one [playerTurns] above already counts. Nothing is halved here; halving is
 * the mistake `turnsTaken`'s comment records this project making once already.
 *
 * Structurally typed rather than taking `StagedBeat`, so `lib/reel.ts` does not
 * have to be imported to ask a question about a list of beats. */
export function turnMarks(beats: readonly { kind: string }[]): number[] {
  const marks: number[] = []
  beats.forEach((b, i) => { if (b.kind === 'turn') marks.push(i + 1) })
  return marks
}

/**
 * Where a turn step lands, given where the transport is now.
 *
 * One rule in each direction, and both are chosen so the control always does
 * something rather than sometimes doing nothing:
 *
 *   - **Back** goes to the largest mark strictly before `at`. From the middle
 *     of a turn that is the start of the turn you are in; pressed again it is
 *     the one before. That is the media convention, it is a single rule, and
 *     it means the first press always moves. Before the first turn there is
 *     nowhere to go but the opening, which is a real place to stand.
 *   - **Forward** goes to the smallest mark after `at`, and off the end it
 *     goes to `of` — this bout's last beat. It does **not** run on into the
 *     next bout: the room queues bouts and tells each to its end, and a turn
 *     step that crossed a bout boundary would skip a whole fight.
 */
export function stepToTurn(
  marks: readonly number[], at: number, of: number, dir: 'back' | 'on',
): number {
  if (dir === 'back') {
    const before = marks.filter((m) => m < at)
    return before[before.length - 1] ?? 0
  }
  return marks.find((m) => m > at) ?? of
}
