/**
 * The reel: one clock for the room.
 *
 * Forge plays a game of Commander in about twenty seconds and nobody can watch
 * twenty seconds of Commander, so the beats arrive whole — one game at a time,
 * at the moment that game ends — and the room drains them at a rate a person
 * can follow. That draining used to live inside the play-by-play component,
 * which was fine while the play-by-play was the only thing watching.
 *
 * It is not any more. The board and the account are the same game seen twice,
 * and the server builds them to be paced together: `BoardStep` and `GameEvent`
 * are one-for-one, so the board after *n* steps is the board at beat *n*. That
 * property is only worth having if one clock drives both — two components each
 * pacing themselves would drift apart within a turn, and the picture would be
 * describing a sentence nobody had read yet.
 *
 * So the pacing is here, the room owns it, and both views are handed the same
 * count.
 *
 * A hook rather than a component, in `lib/` for the reason `lib/motion.ts`
 * gives at the top of its own file: oxlint's fast-refresh rule is right, this
 * is not a component, and more than one file needs it.
 */

import { useEffect, useRef, useState } from 'react'

import type { ForgeBoard } from './api'

/** One beat, already turned into words, and given an identity by the room. */
export interface StagedBeat {
  key: string
  game: number
  turn: number
  kind: string
  who: string | null
  text: string
}

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
}

const EMPTY: Reel = {
  shown: [], queue: [], game: 0, truncated: false, told: 0, board: null,
}

/** How many beats stay in the DOM.
 *
 * The play-by-play is a feed, not a transcript: a twenty-game match raises two
 * thousand beats and nobody scrolls back through them. The **board** is not
 * capped by this — it is folded from `told`, which keeps counting after a beat
 * has scrolled out of the column, because a creature does not leave the
 * battlefield when the sentence about it leaves the screen. */
export const BEATS_KEPT = 80

/** The slowest and the fastest a beat may be told, in milliseconds.
 *
 * The floor is a real limit rather than a round number: below about twenty
 * milliseconds a beat is a flicker, the board's arrival animation cannot
 * finish, and nothing is being *watched* any more. The ceiling is reading
 * speed. */
const SLOWEST = 420
const FASTEST = 20

/**
 * How long to wait before telling the next beat.
 *
 * **Paced to finish**, which is the correction that made the board usable.
 * The first version knew only its own backlog — fast when deep, slow when
 * shallow — and that was fine while the beats were the only thing watching,
 * because a column of sentences falling behind just scrolls. A *board* that
 * falls behind is worse than useless: it is reset to an empty field every
 * time the next game lands, so a ten-game match showed turn two of game one,
 * then turn two of game two, and never a game.
 *
 * Measured on the fixture decks, which play a game in about five seconds: the
 * old curve took roughly sixteen seconds to drain one game's hundred and
 * thirty beats, so it never finished one.
 *
 * So the room measures how long games actually take — the gap between one
 * game's beats arriving and the next's — and spreads this game's beats across
 * that. Slow decks get a leisurely board; the fixtures get a brisk one; and
 * either way a game finishes before the next begins. `budget` is null until a
 * second game has arrived (the first gap is JVM boot and means nothing), and
 * until then the old backlog curve stands in.
 */
export function pace(queued: number, budget: number | null): number {
  if (budget != null && queued > 0) {
    return Math.max(FASTEST, Math.min(SLOWEST, budget / queued))
  }
  if (queued > 80) return 45
  if (queued > 40) return 90
  if (queued > 12) return 200
  return SLOWEST
}

/** What a game hands over: its number, its beats, and whether it was cut. */
export interface Arriving {
  game: number
  beats: StagedBeat[]
  truncated: boolean
  board: ForgeBoard | null
}

/**
 * Drain a game's beats at reading speed, and say how many have been told.
 *
 * `arriving` is handed over again on every poll, which is why the game number
 * is the identity: the same log arriving twenty times is one game, and only a
 * new number is news.
 */
export function useReel(arriving: Arriving | null): Reel {
  // One piece of state rather than five, because every change moves two of
  // them together: a beat leaving the queue is a beat entering the shown list,
  // and a game arriving does both at once.
  const [reel, setReel] = useState<Reel>(EMPTY)
  // The newest game already taken in. A ref rather than state so the guard
  // below can read it without putting it in the effect's dependencies.
  const heard = useRef(0)
  // When the last game's beats arrived, and how long the gap before them was.
  // Refs because they steer the *next* timeout rather than the render — a
  // measurement of the match, not a fact about the picture.
  const arrivedAt = useRef(0)
  const budget = useRef<number | null>(null)

  useEffect(() => {
    if (!arriving || arriving.game <= heard.current) return
    heard.current = arriving.game
    // How long that game took, measured rather than assumed. The **first**
    // gap is skipped deliberately: it holds the JVM's boot and the card
    // database, which is fifteen seconds of no game at all and would set a
    // budget three times too generous for every game after it.
    const now = Date.now()
    if (arrivedAt.current > 0) {
      budget.current = now - arrivedAt.current
    }
    arrivedAt.current = now
    // The game before this one is flushed rather than abandoned: every beat is
    // shown, and the room never falls behind what the pips already say. It is
    // a flurry at the moment a game ends, which is what a game ending is.
    setReel((r) => ({
      shown: [...r.shown, ...r.queue].slice(-BEATS_KEPT),
      queue: arriving.beats,
      game: arriving.game,
      truncated: arriving.truncated,
      // A new game is a new board, so both start over together.
      told: 0,
      board: arriving.board,
    }))
  }, [arriving])

  // One beat leaves the queue per tick, and the next tick is scheduled from
  // what is left — so the pace is re-read after every beat rather than once a
  // game. A queue that empties simply stops scheduling.
  useEffect(() => {
    const next = reel.queue[0]
    if (!next) return
    const id = window.setTimeout(() => setReel((r) => ({
      ...r,
      shown: [...r.shown, next].slice(-BEATS_KEPT),
      queue: r.queue.slice(1),
      told: r.told + 1,
    })), pace(reel.queue.length, budget.current))
    return () => window.clearTimeout(id)
  }, [reel.queue])

  return reel
}
