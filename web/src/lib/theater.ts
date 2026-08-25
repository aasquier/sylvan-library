/**
 * The match theater's pure functions, in `lib/` for the reason `lib/motion.ts`
 * gives at the top of its own file: oxlint's fast-refresh rule is right, these
 * are not components, and each of them is needed by more than one file.
 * `theaterRows` and `theaterBeats` are called by the Coliseum (which owns the
 * job) and `shortName` and `beatLine` by the stage (which owns the row and the
 * beat); putting any of them in `components/theater.tsx` costs that file its
 * fast refresh to save an import.
 *
 * The two `theater*` readers are narrowings of `Job.partial`, which is
 * `unknown` because the shape belongs to the job's kind. Somebody has to do
 * that once, in a way that is total over every phase the job passes through —
 * so both of them answer "nothing yet" for a job that has not ticked, a worker
 * too old to send that half, and a finished job whose partial the server has
 * cleared.
 */

import type { ForgeBeat, ForgeBeats, ForgeGameRow } from './api'

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
 * A deck is named for its commander and then for what it does — "Arahbo, Roar
 * of the World — Cats" — and a feed row wants the first part of that: the
 * general's name, which is how anybody actually refers to the deck out loud.
 * So take everything before the dash, then everything before the title's
 * comma. A name with neither is already short and comes back untouched.
 */
export function shortName(name: string): string {
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
  return {
    game,
    beats: list as ForgeBeat[],
    truncated: (beats as { truncated?: unknown }).truncated === true,
  }
}

/** One beat as a line of English.
 *
 * The vocabulary is the CLI's (`narrateGame` in `cmd/mtglab/sim.go`), and
 * keeping the two in step is the point rather than a coincidence: the terminal
 * account exists so a person can read exactly what the browser will be handed,
 * and a beat that reads differently in the two places makes that check a lie.
 *
 * `name` turns a deck's slug into whatever the room calls it. It is passed in
 * because only the room knows — the beat carries a slug, and the shelf carries
 * the name that slug belongs to.
 *
 * **An unrecognised kind is rendered, never dropped.** Forge is not an API and
 * a release that adds a beat should read as a plain sentence here rather than
 * as a gap in the game.
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
 * The row carries Forge's per-player-turn count and no seats, so this is the
 * one place the halving cannot be avoided — `playerTurns` needs the beats and
 * a row has none. Exact for every game without an extra turn, and one high
 * for a game with one, which is the honest limit of what a row can say.
 *
 * `seats` is 2 for every match this app runs; it is a parameter rather than a
 * literal because the CLI plays pods, and a four-player game's turn 15 is
 * round four rather than round eight.
 */
export function turnsTaken(turns: number | null, seats = 2): number | null {
  if (turns == null || seats < 1) return turns
  return Math.ceil(turns / seats)
}
