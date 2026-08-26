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
 * A hook rather than a component, in `lib/` for the reason `lib/motion.ts`
 * gives at the top of its own file: oxlint's fast-refresh rule is right, this
 * is not a component, and more than one file needs it.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

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
  /** The card on the other end — the attacker a blocker stepped in front of. */
  target?: string
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
   * as it took its own opening bout to arrive. Carrying the match's identity
   * in the reel makes "these beats are not this match's" a thing the room can
   * *see*, rather than a stale frame it has to be raced out of. */
  match: string
}

const EMPTY: Reel = {
  shown: [], queue: [], game: 0, truncated: false, told: 0, board: null,
  match: '',
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
  // One piece of state rather than five, because every change moves two of
  // them together: a beat leaving the queue is a beat entering the shown list,
  // and a bout beginning does both at once.
  const [reel, setReel] = useState<Reel>(EMPTY)
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

  // The bout already loaded, as match-and-number. A ref rather than state
  // because it exists only to stop the effect below reloading what it just
  // loaded: `stage` is rebuilt whenever a new game lands, and without this
  // every arrival would restart the bout being told from its first beat.
  const staged = useRef('')
  const at = `${match}:${target}`

  useEffect(() => {
    if (target === 0 || staged.current === at) return
    const next = stage(target)
    // A bout the room cannot resolve yet is not an error: the match has said
    // it exists before its beats crossed. The next render tries again.
    if (!next) return
    staged.current = at
    setReel({
      shown: [],
      queue: next.beats,
      game: next.game,
      truncated: next.truncated,
      // A new bout is a new board, so both start over together.
      told: 0,
      board: next.board,
      match,
    })
  }, [at, target, match, stage])

  // One beat leaves the queue per tick, and the next tick is scheduled from
  // what is left — so the pace is re-read after every beat rather than once a
  // game. A queue that empties simply stops scheduling.
  useEffect(() => {
    if (speed === 'paused') return
    const next = reel.queue[0]
    if (!next) return
    const id = window.setTimeout(() => setReel((r) => ({
      ...r,
      shown: [...r.shown, next],
      queue: r.queue.slice(1),
      told: r.told + 1,
    })), SPEEDS[speed])
    return () => window.clearTimeout(id)
  }, [reel.queue, speed])

  // Beats from a match that is over are not this match's. Until this match's
  // opening bout is staged the field is empty, which is the honest picture and
  // costs nobody a frame of the last match's board.
  const showing = reel.match === match ? reel : EMPTY

  /** The next bout after the one being told, or zero. A number rather than the
   *  list, so the breath below is not restarted every time a bout lands. */
  const nextUp = useMemo(
    () => games.find((g) => g > showing.game) ?? 0, [games, showing.game])

  // The walk forward. A bout whose queue has run dry hands over to the next
  // one the match has raised — after a breath, so its last sentence is read
  // rather than glimpsed. Paused holds it: a room somebody has stopped does
  // not wander on to the next fight without them.
  useEffect(() => {
    if (speed === 'paused' || showing.game === 0 || nextUp === 0) return
    if (showing.queue.length > 0) return
    const id = window.setTimeout(
      () => setAsked({ match, game: nextUp }), BETWEEN_BOUTS)
    return () => window.clearTimeout(id)
  }, [speed, nextUp, match, showing.game, showing.queue.length])

  // Moving the mark by hand. It sets the reel's own position rather than
  // overlaying it, so pressing play afterwards carries on from where the hand
  // left off instead of snapping back.
  const seek = useCallback((to: number) => {
    setReel((r) => seekReel(r, to))
  }, [])

  const pick = useCallback(
    (game: number) => setAsked({ match, game }), [match])

  const series = useMemo((): Series => ({
    // The bouts still ahead of the one on the field. Not simply "the others":
    // a match watched back from the middle has bouts behind it too, and those
    // are not waiting for anybody — they have already been told.
    waiting: games.filter((g) => g > showing.game).length,
    pick,
  }), [games, showing.game, pick])

  return [showing, seek, series]
}

/**
 * Move the room to a beat by hand.
 *
 * Scrubbing is possible at all because the board is a **pure fold over a
 * count** — `foldBoard(board, n)` is the board after n beats, with no state
 * carried between calls — so going backwards is the same operation as going
 * forwards and costs the same. That was true before anybody asked for it; the
 * controls are just a second way to set the number.
 *
 * The whole game's beats are held either side of the mark, so a step back is a
 * beat moving from `shown` to `queue` and a step forward is the reverse.
 */
export function seekReel(reel: Reel, to: number): Reel {
  const all = [...reel.shown, ...reel.queue]
  const at = Math.max(0, Math.min(to, all.length))
  return {
    ...reel,
    shown: all.slice(0, at),
    queue: all.slice(at),
    told: at,
  }
}
