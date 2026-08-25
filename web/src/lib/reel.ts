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

import { useCallback, useEffect, useRef, useState } from 'react'

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

/** How fast the room is telling it, or whether it is telling it at all.
 *
 * **The Forge is never waited for and never slowed down.** It plays its games
 * at whatever speed it plays them and the results land when they land; this
 * governs only how fast the *room* reads them out. That separation is the
 * whole point — a match is a measurement and watching it is a performance, and
 * making the measurement slow to suit the performance would be the wrong trade
 * in every direction.
 */
export type Speed = 'paused' | 'slow' | 'play' | 'fast'

/** The multiplier each speed applies to the natural pace. Larger is slower. */
const SPEEDS: Record<Exclude<Speed, 'paused'>, number> = {
  slow: 2.5,
  play: 1,
  fast: 0.25,
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

/** How many beats the play-by-play renders.
 *
 * The account is a feed, not a transcript: a twenty-game match raises two
 * thousand beats and nobody scrolls back through them. **This is a rendering
 * limit and not a memory one** — the reel keeps every beat it has told, so the
 * scrubber can walk back through a whole game, and the column shows the tail.
 * Cutting the model here is what would make going backwards impossible. */
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
 * either way a game finishes before the next begins.
 *
 * **`left` is the time remaining, not the whole budget**, and the difference
 * is the whole correctness of this. The first version divided the *game's*
 * budget by the *shrinking* queue, so each beat was slower than the last: at
 * 126 beats over 50 seconds it starts at 396ms and, by the time sixty are
 * left, asks for 793 — pinned at the ceiling, and a game that should have
 * taken fifty seconds took several minutes. Aaron watched a board crawl one
 * turn a minute. Time left over beats left is constant when it is keeping up
 * and self-correcting when it is not, which is what was wanted both times.
 *
 * `left` is null until a second game has arrived (the first gap is JVM boot
 * and means nothing), and until then the old backlog curve stands in.
 */
export function pace(queued: number, left: number | null): number {
  if (left != null && queued > 0) {
    return Math.max(FASTEST, Math.min(SLOWEST, left / queued))
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
 *
 * **`running` is the other half of the pacing, and it is the one that matters
 * most for how this feels.** Reading speed is only worth anything while there
 * is something still to wait for. Once the match is over there is no next game
 * to stay ahead of, and holding the rest back means a viewer watches a board
 * inch through a game that finished minutes ago — the single worst state this
 * room can be in, and the one Aaron found. A finished match empties its queue
 * at the floor.
 */
export function useReel(arriving: Arriving | null, running: boolean,
  speed: Speed): [Reel, (to: number) => void] {
  // One piece of state rather than five, because every change moves two of
  // them together: a beat leaving the queue is a beat entering the shown list,
  // and a game arriving does both at once.
  const [reel, setReel] = useState<Reel>(EMPTY)
  // The newest game already taken in. A ref rather than state so the guard
  // below can read it without putting it in the effect's dependencies.
  const heard = useRef(0)
  // When the last game's beats arrived, and when this game's should be told
  // by. Refs because they steer the *next* timeout rather than the render — a
  // measurement of the match, not a fact about the picture.
  const arrivedAt = useRef(0)
  const budget = useRef<number | null>(null)
  const deadline = useRef<number | null>(null)

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
    // This game should be told by the time the next one is expected. A game
    // that runs long simply gets flushed, which is what already happens.
    deadline.current = budget.current == null ? null : now + budget.current
    // The game before this one is flushed rather than abandoned: every beat is
    // shown, and the room never falls behind what the pips already say. It is
    // a flurry at the moment a game ends, which is what a game ending is.
    setReel((r) => ({
      shown: [...r.shown, ...r.queue],
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
    if (speed === 'paused') return
    const next = reel.queue[0]
    if (!next) return
    const id = window.setTimeout(() => setReel((r) => ({
      ...r,
      shown: [...r.shown, next],
      queue: r.queue.slice(1),
      told: r.told + 1,
    })), SPEEDS[speed] * (running
      ? pace(reel.queue.length,
          deadline.current == null ? null : deadline.current - Date.now())
      // Nothing left to stay ahead of: catch up to the result the room is
      // already showing above.
      : FASTEST))
    return () => window.clearTimeout(id)
  }, [reel.queue, running, speed])

  // Moving the mark by hand. It sets the reel's own position rather than
  // overlaying it, so pressing play afterwards carries on from where the hand
  // left off instead of snapping back.
  const seek = useCallback((to: number) => {
    setReel((r) => seekReel(r, to))
  }, [])

  return [reel, seek]
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
