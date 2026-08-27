/**
 * The reel: one clock for the room, and one running order.
 *
 * Forge plays a game of Commander in about twenty seconds and nobody can watch
 * twenty seconds of Commander, so the beats arrive whole — one game at a time,
 * at the moment that game ends — and the room drains them at a rate a person
 * can follow. That draining used to live inside the play-by-play component,
 * which was fine while the play-by-play was the only thing watching.
 *
 * It is not any more. The board is folded to exactly the count this hook has
 * told: the server builds `BoardStep` and `GameEvent` one-for-one, so the board
 * after *n* steps is the board at beat *n*. That property is only worth having
 * if one clock drives it — a component pacing itself beside another would drift
 * within a turn, and the picture would be describing a sentence nobody had read
 * yet.
 *
 * So the pacing is here, the room owns it, and the field is handed the count.
 *
 * **And the running order is here too.** A match is a series, not a game: the
 * second bout lands while the first is still being told, and it used to *clip*
 * it — the arriving game replaced the reel wholesale and whatever was left of
 * the last one was simply never seen (Aaron, 2026-08-26: "the next game in the
 * series clips the previous, can we pin the first one until it ends and queue
 * the remaining?"). It does not any more. A bout is told to its end, the room
 * takes a breath, and the next one begins. Every bout the match has raised is
 * reachable at any time, so nobody is held behind a slow retelling of game one
 * while game four is being fought.
 *
 * **And it counts the repeats**, which is the running order's other half. Four
 * tokens made at once reach the browser as four identical beats in a row,
 * because the only thing the wire can say is that a card moved; [countRuns]
 * turns that back into one moment with a four on it, without removing a beat
 * or moving one. Nothing is folded away — the board still steps once per beat
 * — it is only said once.
 *
 * A hook rather than a component, in `lib/` for the reason `lib/motion.ts`
 * gives at the top of its own file: oxlint's fast-refresh rule is right, this
 * is not a component, and more than one file needs it.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'

import type { ForgeBoard } from './api'

/** One beat, already turned into words, and given an identity by the room. */
export interface StagedBeat {
  key: string
  game: number
  turn: number
  kind: string
  who: string | null
  text: string
  /** The card this beat is about, in Forge's own spelling, carried past the
   *  sentence so the *board* can find it too.
   *
   *  A mark on the battlefield needs to know which permanent the sentence was
   *  about, and re-parsing English to get back a name it already had would be
   *  a fine way to introduce a bug. */
  card?: string
  /** The card on the other end — the attacker a blocker stepped in front of,
   *  or the permanent an Aura or Equipment was put on. */
  target?: string
  /** How an arrival reached the battlefield — `'cast'`, `'put'`, or nothing
   *  said. Carried past the sentence for `card`'s reason: the arena draws a
   *  scene for the permanent nobody cast, and only the beat knows which
   *  those are. See `ForgeBeat.entered`, which argues why the missing case is
   *  a third answer and not a `'put'`. */
  entered?: string
  /**
   * How many identical beats in a row this one **begins**.
   *
   * `1` for a beat standing alone, `N` for the first of a run of N, and **`0`
   * for every beat that follows the first** — see [countRuns], which is the
   * only thing that sets it and carries the whole argument.
   *
   * Optional because a `StagedBeat` built anywhere but the reel has not been
   * counted; read a missing value as `1`, which is what a beat that has not
   * been asked the question really is.
   */
  run?: number
}

/**
 * Count identical beats that arrive back to back, so the room can tell one
 * moment once.
 *
 * **Four tokens made at once is one thing that happened, and the wire has no
 * word for it.** Forge announces a token by moving a card into a zone, one
 * card at a time with one id each, and the scribe raises one `enters` beat per
 * card — so a Trostani populate or an Academy Manufactor trigger reaches the
 * browser as three or six or nine separate beats saying exactly the same
 * sentence in a row. There is no `amount` anywhere on that path and inventing
 * one would be the room claiming a fact nothing told it (ADR 44).
 *
 * **So it is counted rather than guessed.** Three consecutive beats reading
 * "Gyome makes Clue Token" *are* three Clue Tokens; that is not an inference
 * about Magic, it is arithmetic on what the account already said. Measured on
 * the recorded match rather than hoped for: a real Gyome game raises runs of
 * three Clues, three Foods and three Treasures back to back, with only
 * beat-less `stats` lines between them.
 *
 * **Identity is the sentence, not a list of fields.** Two beats are the same
 * moment repeated when their kind, their player and their words all agree —
 * and `text` already carries everything the beat said, including the numbers a
 * field-by-field key would have to remember to include (`damage` says "deals 3
 * to Gyome" and two different damages are two different sentences). A beat
 * that names no card is never folded: a player taking two turns in a row is
 * two turns, not one turn twice, and Time Warp is a real card.
 *
 * **Nothing is removed.** Every beat still ticks, and the board still folds one
 * step for each of them, so the tokens still arrive on the sand one by one and
 * scrubbing still lands where it always did. This only says *which* beat of a
 * run is the one worth drawing in the middle of the arena, and the followers
 * are marked `0` so the stage can leave the card that is already up alone
 * rather than replaying it under a new key.
 *
 * **The first of the run carries the count, not the last.** The moment a pile
 * of tokens is made is the moment the first one lands; announcing it on the
 * last would tell somebody about a thing that finished happening two beats
 * ago.
 */
export function countRuns(beats: StagedBeat[]): StagedBeat[] {
  // What makes two beats the same moment: the kind, the player and the words.
  // A beat that names no card is folded with nothing, so it is given an
  // identity — its own place in the bout — that nothing else can equal.
  const same = (b: StagedBeat, at: number) => b.card
    ? `${b.kind} | ${b.who ?? ''} | ${b.text}`
    : `alone ${at}`
  const out = beats.map((b): StagedBeat => ({ ...b, run: 1 }))
  let head: StagedBeat | null = null
  let key = ''
  out.forEach((beat, at) => {
    if (head && same(beat, at) === key) {
      beat.run = 0
      head.run = (head.run ?? 1) + 1
      return
    }
    head = beat
    key = same(beat, at)
  })
  return out
}

/** How fast the room is telling it, or whether it is telling it at all.
 *
 * **The Forge is never waited for and never slowed down.** It plays its games
 * at whatever speed it plays them and the results land when they land; this
 * governs only how fast the *room* reads them out. That separation is the
 * whole point — a match is a measurement and watching it is a performance, and
 * making the measurement slow to suit the performance would be the wrong trade
 * in every direction.
 *
 * `study` rather than `slow`, because the pace it now names is not a slower
 * version of watching — it is a different intention. See `SPEEDS`.
 */
export type Speed = 'paused' | 'study' | 'play' | 'fast'

/** What the room is holding: the beats already told, the ones still to tell,
 *  and which game they belong to. */
interface Reel {
  shown: StagedBeat[]
  queue: StagedBeat[]
  game: number
  truncated: boolean
  /** How many beats of this game have been told — which is also how many
   *  board steps have happened, because the server builds one per beat. */
  told: number
  /** The board of **the game being drained**, which is not always the newest
   *  one the server has.
   *
   * It is held here rather than read from the partial for exactly that
   * reason: a game that finishes while the previous one is still being told
   * replaces the partial immediately, and a board taken straight from the
   * partial would jump to game three while the column was still reading out
   * game two. Carried together, the picture and the sentences cannot be about
   * different games. */
  board: ForgeBoard | null
  /** Which match these beats belong to.
   *
   * The room outlives a match — the hook is not remounted between them — so
   * without this a second match would show the first one's board for as long
   * as it took its own opening bout to arrive. The mark carries the match it
   * was made in, so a mark belonging to a finished match simply does not
   * apply to this one and the field opens empty. Nothing has to be raced out
   * of a stale frame, because the stale frame is never derived. */
  match: string
}

/**
 * How long a beat is held, in milliseconds, at each speed.
 *
 * **Absolute, and that is the first correction.** The first version multiplied
 * a *derived* pace — one that measured how long games were taking and spread
 * the beats across that, and which collapsed to a 20ms catch-up floor the
 * moment a match finished. So "Slow" on a finished match meant 50ms a beat,
 * which is not slow, and Aaron was right that it was unwatchable at every
 * setting.
 *
 * **And the slow one is a study pace, which is the second.** 1200ms was still
 * too fast to follow — twice, in Aaron's words: "it moves quicker than the mind
 * can keep up with", and then "not slow enough, lets slow it down by another
 * 20-30%". Doubling and then taking the deeper end of that range lands on
 * three seconds a beat. That is not a slower way of watching; it is reading a
 * game, one line at a time, which is why the control no longer calls itself
 * Slow. A newcomer meeting their first Commander game needs the beat to still
 * be on screen while they work out what it meant (commandment 2), and three
 * seconds is that.
 *
 * The Forge does not wait for these numbers and is not slowed by them: it
 * plays flat out, its results land when they land, and every game of a
 * finished match can be watched back at leisure. So the room has no reason to
 * chase the engine, and a speed control that means a fixed thing is worth more
 * than one that is clever about a race it does not need to win.
 *
 * A game is about a hundred and thirty beats, so: Study is about six and a half
 * minutes, Watch is about a minute, Fast is twenty seconds.
 */
const SPEEDS: Record<Exclude<Speed, 'paused'>, number> = {
  study: 3000,
  play: 480,
  fast: 150,
}

/** The breath between two bouts.
 *
 * A game's last beat is its outcome — "has lost due to accumulation of 21
 * damage from generals" — and advancing the moment the queue empties would put
 * the next bout's opening hand on screen before anybody had read how the last
 * one ended. Long enough to land that sentence, short enough that a series
 * still feels like one thing rather than a slideshow. */
export const BETWEEN_BOUTS = 2500

/** How long a beat is held at a speed. Zero when paused, which is not a
 *  delay — it is the absence of one, and the caller schedules nothing. */
export function beatDelay(speed: Speed): number {
  return speed === 'paused' ? 0 : SPEEDS[speed]
}

/** What a game hands over: its number, its beats, and whether it was cut. */
export interface Arriving {
  game: number
  beats: StagedBeat[]
  truncated: boolean
  board: ForgeBoard | null
}

/** The running order, as the transport needs to see it. */
export interface Series {
  /** How many bouts are still ahead of the one on the field. What the room
   *  owes the watcher, and the reason it is said out loud rather than left to
   *  be inferred from a row of numbered chips. */
  waiting: number
  /** Watch this one instead, now. Nobody is held behind a slow retelling of
   *  the opening bout while the fourth is being fought. */
  pick: (game: number) => void
}

/**
 * Tell a match, one bout at a time, at reading speed.
 *
 * `match` identifies the match itself — beats from a previous one are not this
 * one's, and the room is not remounted between them. `games` is every bout the
 * match has raised so far, in order, and `stage` turns one of those numbers
 * into its beats. Handing over a *resolver* rather than the beats themselves is
 * what keeps this bounded: only the bout being told is ever held as staged
 * beats, however far behind the room has fallen.
 *
 * **The room never chases the Forge.** It used to try — measuring how long
 * games took and spreading each game's beats across that window — and the
 * cleverness cost more than it bought: the pace collapsed to a catch-up floor
 * the moment a match ended, so every speed setting meant roughly the same
 * unwatchable thing. It does not need to keep up. A finished match carries
 * every game with it, so falling behind is not falling behind; it is simply
 * being somewhere else in a recording that is not going anywhere.
 *
 * **And it never skips.** The room walks forward through the bouts: when one
 * ends and the match has another, that one begins. Pausing holds it there, and
 * picking a bout by hand starts the walk from that bout instead.
 */
export function useReel(match: string, games: number[],
  stage: (game: number) => Arriving | null,
  speed: Speed): [Reel, (to: number) => void, Series] {
  // **One piece of state, and it is a position rather than a copy.** The reel
  // used to hold the beats twice over — a `shown` list and a `queue` — and
  // move them across a mark, which meant every arrival had to be *loaded* into
  // it by an effect. That shape is why this file kept needing a ref to
  // remember what it had already loaded, and why React rightly complained
  // about a `setState` running straight out of an effect.
  //
  // Where the mark is says everything. The two lists are a slice of one array
  // at that number, taken during render, and a bout that has not been reached
  // yet needs no loading because nothing was ever copied out of it.
  const [mark, setMark] = useState<{ match: string; game: number; told: number }>(
    { match: '', game: 0, told: 0 })
  // The bout somebody asked for, and the match they asked for it in. Carrying
  // the match with it is what makes a pick expire on its own: a choice made
  // during the last match is not a choice about this one, and a bare number
  // would send the room hunting for a bout that this match may never raise.
  const [asked, setAsked] = useState<{ match: string; game: number }>(
    { match: '', game: 0 })

  // Which bout the room should be telling. What was asked for while this match
  // is the one asking, and otherwise simply the first bout there is.
  const wanted = asked.match === match ? asked.game : 0
  const target = wanted || games[0] || 0

  // The bout in words, and its repeats counted. Resolved once per *bout*
  // rather than once per beat: `stage` walks a game's worth of beats, and
  // doing that on every tick of the clock would be a hundred and thirty
  // translations a second.
  //
  // **The counting belongs to the running order**, which is why it happens
  // here rather than wherever the beats were made. Whether four identical
  // beats are four moments or one moment told once is a question about how the
  // room tells a bout, and this hook is what tells a bout — see [countRuns].
  const bout = useMemo(() => {
    if (target === 0) return null
    const heard = stage(target)
    return heard && { ...heard, beats: countRuns(heard.beats) }
  }, [stage, target])

  // How far into *this* bout of *this* match the mark has got. A mark left
  // behind by another bout — or by a match that is over — is not this one's,
  // and the room opens at the first beat rather than somewhere in the middle
  // of a fight it has never shown. This is the whole of what used to need a
  // ref, an `EMPTY` sentinel and a loading effect to arrange.
  const told = (mark.match === match && mark.game === target)
    ? Math.min(mark.told, bout?.beats.length ?? 0)
    : 0

  const beats = bout?.beats
  const queued = (beats?.length ?? 0) - told

  const reel: Reel = {
    shown: beats ? beats.slice(0, told) : [],
    queue: beats ? beats.slice(told) : [],
    game: bout?.game ?? 0,
    truncated: bout?.truncated ?? false,
    told,
    board: bout?.board ?? null,
    match,
  }

  // One beat leaves the queue per tick, and the next tick is scheduled from
  // what is left — so the pace is re-read after every beat rather than once a
  // game. A queue that empties simply stops scheduling.
  useEffect(() => {
    if (speed === 'paused' || queued <= 0) return
    const id = window.setTimeout(
      () => setMark({ match, game: target, told: told + 1 }), SPEEDS[speed])
    return () => window.clearTimeout(id)
  }, [speed, queued, match, target, told])

  /** The next bout after the one being told, or zero. A number rather than the
   *  list, so the breath below is not restarted every time a bout lands. */
  const nextUp = useMemo(
    () => games.find((g) => g > reel.game) ?? 0, [games, reel.game])

  // The walk forward. A bout whose queue has run dry hands over to the next
  // one the match has raised — after a breath, so its last sentence is read
  // rather than glimpsed. Paused holds it: a room somebody has stopped does
  // not wander on to the next fight without them.
  useEffect(() => {
    if (speed === 'paused' || !bout || nextUp === 0 || queued > 0) return
    const id = window.setTimeout(
      () => setAsked({ match, game: nextUp }), BETWEEN_BOUTS)
    return () => window.clearTimeout(id)
  }, [speed, bout, nextUp, match, queued])

  // Moving the mark by hand. It sets the reel's own position rather than
  // overlaying it, so pressing play afterwards carries on from where the hand
  // left off instead of snapping back.
  //
  // Scrubbing is possible at all because the board is a **pure fold over a
  // count** — `foldBoard(board, n)` is the board after n beats, with no state
  // carried between calls — so going backwards is the same operation as going
  // forwards and costs the same. The controls are a second way to set the
  // number, not a second engine.
  const total = beats?.length ?? 0
  const seek = useCallback((to: number) => {
    setMark({ match, game: target, told: Math.max(0, Math.min(to, total)) })
  }, [match, target, total])

  const pick = useCallback(
    (game: number) => setAsked({ match, game }), [match])

  const series = useMemo((): Series => ({
    // The bouts still ahead of the one on the field. Not simply "the others":
    // a match watched back from the middle has bouts behind it too, and those
    // are not waiting for anybody — they have already been told.
    waiting: games.filter((g) => g > reel.game).length,
    pick,
  }), [games, reel.game, pick])

  return [reel, seek, series]
}
