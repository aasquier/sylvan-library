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
  /** The card this beat is about, in Forge's own spelling, carried past the
   *  sentence so the *board* can find it too.
   *
   *  The account only ever needed `text`; a mark on the battlefield needs to
   *  know which permanent the sentence was about, and re-parsing English to
   *  get back a name it already had would be a fine way to introduce a bug. */
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
 */
export type Speed = 'paused' | 'slow' | 'play' | 'fast'

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

/**
 * How long a beat is held, in milliseconds, at each speed.
 *
 * **Absolute, and that is the correction.** The first version multiplied a
 * *derived* pace — one that measured how long games were taking and spread the
 * beats across that, and which collapsed to a 20ms catch-up floor the moment a
 * match finished. So "Slow" on a finished match meant 50ms a beat, which is
 * not slow, and Aaron was right that it was unwatchable at every setting.
 *
 * The Forge does not wait for these numbers and is not slowed by them: it
 * plays flat out, its results land when they land, and every game of a
 * finished match can be watched back at leisure. So the room has no reason to
 * chase the engine, and a speed control that means a fixed thing is worth more
 * than one that is clever about a race it does not need to win.
 *
 * A game is about a hundred and thirty beats, so: Slow is a shade over two
 * and a half minutes, Watch is about a minute, Fast is twenty seconds.
 */
const SPEEDS: Record<Exclude<Speed, 'paused'>, number> = {
  slow: 1200,
  play: 480,
  fast: 150,
}

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

/**
 * Drain a game's beats at reading speed, and say how many have been told.
 *
 * `arriving` is handed over again on every poll, which is why the game number
 * is the identity: the same log arriving twenty times is one game, and only a
 * new number is news.
 *
 * **The room never chases the Forge.** It used to try — measuring how long
 * games took and spreading each game's beats across that window — and the
 * cleverness cost more than it bought: the pace collapsed to a catch-up floor
 * the moment a match ended, so every speed setting meant roughly the same
 * unwatchable thing. It does not need to keep up. A finished match carries
 * every game with it, so falling behind is not falling behind; it is simply
 * being somewhere else in a recording that is not going anywhere.
 */
export function useReel(arriving: Arriving | null,
  speed: Speed): [Reel, (to: number) => void] {
  // One piece of state rather than five, because every change moves two of
  // them together: a beat leaving the queue is a beat entering the shown list,
  // and a game arriving does both at once.
  const [reel, setReel] = useState<Reel>(EMPTY)
  // The game currently loaded. A ref rather than state so the guard below can
  // read it without putting it in the effect's dependencies.
  //
  // **Which game, not how far along.** It used to hold the highest game number
  // seen and refuse anything lower, which was right while the only source was
  // a match marching forwards — and wrong the moment somebody could pick a
  // game to watch back. Loading game two after game five is not going
  // backwards; it is choosing.
  const heard = useRef(0)


  useEffect(() => {
    if (!arriving || arriving.game === heard.current) return
    heard.current = arriving.game
    // A new game starts clean. The previous one is not flushed into the column
    // any more: every game of a finished match can be watched back on its own,
    // so running two of them together in one feed reads as a mistake rather
    // than as continuity.
    setReel(() => ({
      shown: [],
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
    })), SPEEDS[speed])
    return () => window.clearTimeout(id)
  }, [reel.queue, speed])

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
