/**
 * What the centre of the arena shows, and for how long.
 *
 * **Half of Commander never touches the battlefield.** A Lightning Bolt is
 * cast, resolves, and is in a graveyard before the sentence describing it has
 * finished being read — and until now the Coliseum drew nothing at all for it,
 * because the board draws permanents and a board is a place that holds what
 * stays (Aaron, 2026-08-26: *"there is nothing to mark their existence in the
 * coliseum"*). So every cast gets a moment in the middle of the sand, and a
 * creature that dies gets the same moment with the skull played over it.
 *
 * The drawing is `components/stage.tsx`. Everything that is not drawing is
 * here, in `lib/` for the reason `lib/reel.ts` gives at the top of its own
 * file: oxlint's fast-refresh rule is right, a table of durations and a hook
 * are not components, and putting them in the component file would also make a
 * cycle out of `board.tsx` ⇄ `stage.tsx`.
 *
 * ## The one mechanism this shares with the marks
 *
 * `components/board.tsx` learned the hard way that **a visual timed by the
 * beat is a visual cut off before it is half told** — at watching pace a beat
 * is 480ms, so a second of animation simply never finished, and lengthening
 * the stylesheet did nothing at all because the stylesheet was never what
 * ended it. Its answer was that a mark **names its own lifetime**, and that
 * one number is both the CSS `animation-duration` and the element's life in
 * the tree, handed across as a custom property so the two cannot drift.
 *
 * This uses that same mechanism and does not invent a parallel one: `STAGE_LIFE`
 * and `stageLife` are `MARK_LIFE` and `markLife` wearing the same shape, and
 * the *death* takes its length from the mark's own number rather than choosing
 * a second answer to one question.
 *
 * ## What this is not
 *
 * **It is not the board.** The board is a fold over a count: at beat *n* it
 * shows the state after *n* beats, which is why scrubbing works at all. This
 * shows **events**, and an event is not a state — it happened, it is over.
 * That is the one place it deliberately parts company with the marks. A mark
 * is given back to its beat while the transport is paused, because a mark is
 * part of the board that beat describes; a cast is not part of any board. So a
 * stage item always expires on its own timer, paused or playing, and the arena
 * is never left with a card sitting over it while somebody reads the timeline.
 */

import { useEffect, useState } from 'react'

import type { ForgeBoard, ForgeBoardCard } from './api'
import { sameCard } from './board'
import { type Pip, poolPips } from './mana'
import { beatDelay, type Speed } from './reel'

/**
 * What is happening to the card on the stage.
 *
 * Two of these are drawn today and two are named in this comment without being
 * drawn, which is deliberate rather than unfinished: **the shape of the data
 * they need is the thing that is missing, and guessing at it in a browser
 * would be the room claiming a mechanic happened when nothing told it so.**
 *
 * - `cast` — every spell, from Sol Ring to the sorcery that wins the game.
 *   Forge's `GameEventSpellAbilityCast`, filtered to spells, already crosses as
 *   a `cast` beat carrying the card's name.
 * - `dies` — a creature leaving the battlefield for a graveyard. Already on the
 *   wire, and already drawn twice elsewhere: the skull on the card in its row,
 *   the ghost rising off the graveyard pile. This is the middle of that.
 *
 * Not yet, and what each would need:
 *
 * - **populate** — Forge's token-created event is a bare signal with no fields,
 *   so a populated copy is indistinguishable from any other token entering
 *   play. It needs a beat naming **the card being copied**, and saying that a
 *   copy is what this was. Given that, it is a `Manner` row, a `PLATE` row and
 *   a block of keyframes: the card, splitting into a mirrored second of itself.
 * - **eminence** — activated and triggered abilities never reach the stream at
 *   all, so an eminence trigger is invisible. It needs a beat naming **the card
 *   whose ability fired** and **the ability's own words**, because the whole
 *   reason to draw eminence is that a newcomer cannot otherwise see why the
 *   board just changed. The words are the part that matters more than the
 *   glow.
 */
export type Manner = 'cast' | 'dies'

/** Which beats get the middle of the arena.
 *
 * **A land is played, not cast**, and this is the one judgement in the file
 * that is a Magic call rather than a rendering one. Aaron asked for every card
 * *as it is being cast*; a land does not use the stack, is not cast, and is the
 * single most routine thing that happens in a game — eight or ten a game, one
 * almost every turn. Filling the arena with a Forest would spend the effect on
 * the beat that needs it least, and the board already draws the land arriving
 * in its own row.
 *
 * `resolve` is left alone for the same reason from the other end: a spell that
 * is cast and then resolves is *one* spell, and drawing it twice a beat apart
 * would read as two. The cast is the moment somebody committed to it, so the
 * cast is the moment that is drawn.
 */
export function mannerOf(kind: string): Manner | null {
  return kind === 'cast' ? 'cast' : kind === 'dies' ? 'dies' : null
}

/**
 * How long a card is on the stage, in milliseconds, before any pace applies.
 *
 * Aaron's brief was three things and all three are constraints: **BIG**, fade
 * in and out, and *"it shouldn't linger for too long"*. So a cast is a beat and
 * a half at watching pace — long enough that the eye lands on the card, reads
 * the name and the art, and is finished before it starts waiting. Under about
 * eight hundred milliseconds is a flash somebody has to ask about; over about a
 * second and a half is a room that has stopped to admire itself while the game
 * goes on behind it.
 */
const STAGE_LIFE: Record<Manner, number> = {
  /* The commonest of the two by a wide margin — a game holds sixty or seventy
     casts and a dozen deaths — so it is the shorter one, for the same reason
     `MARK_LIFE` keeps the attack lamp shortest of its three. */
  cast: 1150,
  /* Normally never read: a death takes the mark's own length, because the skull
     on the card in its row, the ghost off the grave and the card in the middle
     are **one event drawn in three places** and one event gets one clock. It is
     written down anyway so that a stage rendered without that number still gets
     the right length rather than a plausible-looking default. */
  dies: 2000,
}

/** The most beats a stage item may outlive its own.
 *
 *  The same cap, and the same argument, as `MARK_BEATS`: the pace is a
 *  *reading speed*, and holding a card for six seconds because somebody chose
 *  to read slowly is not what they asked for. At watching pace and slower this
 *  never binds. It exists for the fast end, where 150ms a beat would leave a
 *  Lightning Bolt filling the arena while eight later beats went past behind
 *  it — which is the exact failure Aaron named, in reverse. */
const STAGE_BEATS = 4
/** ...and the floor under it, so no pace can cut a reveal down to a strobe. A
 *  card is either shown to somebody or it is not; half a fade is worse than
 *  nothing, because it draws the eye to a thing that is already gone. */
const STAGE_FLOOR = 620
/** A breath past the animation, so the card leaves the tree *after* it has
 *  dissolved rather than popping from under its own last frame. */
export const STAGE_TAIL = 90
/** How long the card being shoved off the stage takes to get out of the way.
 *
 *  Short, and deliberately not scaled by pace: this is not something anybody is
 *  meant to watch, it is the absence of a hard cut. Two spells cast a beat
 *  apart is an ordinary thing — a ritual and the spell it paid for, a cascade,
 *  a storm count — and the first one vanishing mid-hold to be replaced by the
 *  second reads as a fault. Given a fifth of a second to leave, it reads as one
 *  spell following another. */
export const STAGE_PART = 200

/**
 * How long this manner is watched for, at this pace.
 *
 * **A death is the marks' number and nothing else** — not the marks' number
 * capped again here, which is what this did first and which a test caught
 * immediately. The two caps are a beat apart (five against four), so at
 * watching pace the mark's 2000ms met a stage life of 1920ms: the stylesheet
 * would still have been running `--mark-life-dies` when the element was pulled
 * out from under it, and a card would have popped off eighty milliseconds
 * before its own last frame. That is precisely the drift the whole
 * one-number-for-both design exists to prevent, reintroduced by being careful
 * twice. One event, one clock, and the clock belongs to the mark.
 *
 * The cap below is therefore only ever for a cast, which has no mark to
 * inherit a length from and so needs its own answer about pace.
 */
export function stageLife(manner: Manner, speed: Speed,
  dies: number | null): number {
  if (manner === 'dies') return dies ?? STAGE_LIFE.dies
  const beat = beatDelay(speed)
  // Paused is not a slow pace, it is the absence of one: nothing is draining,
  // so there is nothing for the card to outlive and nothing to cap it against.
  if (beat === 0) return STAGE_LIFE[manner]
  return Math.max(STAGE_FLOOR,
    Math.min(STAGE_LIFE[manner], beat * STAGE_BEATS))
}

/** What the plate under the card says, in Magic's own words.
 *
 *  A museum plate rather than a caption: the arena is a building full of
 *  exhibits and this is the one moment a card is held up to be looked at. The
 *  words are the game's own — a creature *dies*, which is the rules term and
 *  also the plainer of the two for somebody at their first game. */
export const PLATE: Record<Manner, string> = { cast: 'Cast', dies: 'Dies' }

/** One card, on the stage, for one moment. */
export interface Staged {
  /** The beat's own identity, which is what makes the second Lightning Bolt of
   *  a game play again rather than reconciling onto an element whose animation
   *  has already run. */
  key: string
  manner: Manner
  name: string
  /** The whole card face, or null when the match never painted one — see
   *  `faceFor`, and the plate that stands in for it. */
  image: string | null
  /** How long this one is watched for, which is both the CSS duration and the
   *  element's life. One number, handed over rather than written twice. */
  life: number
}

/**
 * The card face for a name, out of the match's own card list.
 *
 * **The beat carries a name and no id**, so this is a lookup rather than a
 * dereference: Forge's cast event names the card, and the id it had on the
 * stack is not an id anything else in the board refers to. The board's `cards`
 * is the right place to ask, and the reason it works is a fact worth writing
 * down — the scribe names a card on **any zone line at all, ahead of every
 * filter**, so a sorcery that only ever went hand → stack → graveyard is in
 * that list, painted, exactly as a creature on the battlefield is. What is
 * *not* in it is a card the game never touched; the library is not enumerated.
 *
 * Two things it has to get right:
 *
 * - **The spelling.** Forge names a *face* and the board can carry Scryfall's
 *   combined `A // B`, which is why this uses `sameCard` rather than `===`. A
 *   transforming card would otherwise be cast sixty times a match and found
 *   not once — silently, and only in the decks that play them.
 * - **Whose copy.** Two seats can each run a Swords to Plowshares, and in a
 *   singleton format that is the only way one name is two cards. Same printing,
 *   same picture, so this is very nearly cosmetic — but preferring the caster's
 *   own copy costs one comparison and means the card shown is the card cast.
 *
 * A miss is a real answer and not a failure: **null means draw the plate**,
 * which is a legible card set in type and not a broken image.
 */
export function faceFor(board: ForgeBoard | null, name: string,
  seat: number | null): ForgeBoardCard | null {
  let best: ForgeBoardCard | null = null
  for (const card of board?.cards ?? []) {
    if (!sameCard(card.name, name)) continue
    // A card with a painting beats one without, and this seat's copy beats the
    // other seat's — in that order, because a picture is what this is for.
    if (!best || (card.image && !best.image)
      || (!!card.image === !!best.image && seat != null && card.seat === seat
        && best.seat !== seat)) {
      best = card
    }
  }
  return best
}

/**
 * Hold one card on the stage, and one on its way off.
 *
 * Three things this must not do, and the whole hook is those three:
 *
 * - **It must never leave a card stuck.** A card stranded over the arena is
 *   worse than no card at all — the board is the thing a person came to watch,
 *   and this is drawn on top of it. Every timeout is cleared by the effect that
 *   set it: on the next card, on a pace change, on unmount. There is no path
 *   where a card is mounted without a timer that takes it away.
 * - **It must never pile up.** Two slots, both single values, so it cannot hold
 *   three cards however fast the beats arrive. A burst of casts walks through
 *   the same two boxes.
 * - **It must not carry a card across a game.** Choosing another game of the
 *   same match keeps this mounted, and a Bolt held over from game one appearing
 *   over game two's opening hand is the room lying about what it is showing.
 *   Dropped at the boundary rather than left to time out across it.
 *
 * **The card is chosen during the render, and only the taking-away is a
 * timer** — the same call `useHeldMark` makes, for the same reason. Raising it
 * from an effect would draw it one commit *after* the sentence that raised it,
 * and half a frame between the words and the picture is the thing this room has
 * been careful about since the beats and the board were first paced from one
 * clock. React re-runs a render that sets its own state before committing
 * anything, so the beat and its card still reach the screen together.
 */
export function useStaged(next: Staged | null, key: string, game: number):
  { showing: Staged | null; parting: Staged | null } {
  const [showing, setShowing] = useState<Staged | null>(null)
  const [parting, setParting] = useState<Staged | null>(null)
  const [wasGame, setWasGame] = useState(game)
  // Null rather than `key`, and never the empty string: a stage that mounts
  // with a beat already in hand has to raise that beat's card rather than wait
  // for the next one, and `''` is a key `beat?.key ?? ''` can really produce —
  // so the sentinel has to be something a key cannot be.
  const [wasKey, setWasKey] = useState<string | null>(null)
  if (game !== wasGame) {
    setWasGame(game)
    setWasKey(key)
    setShowing(next)
    setParting(null)
  } else if (key !== wasKey) {
    setWasKey(key)
    // A beat with nothing to show leaves whatever is up alone — it finishes its
    // own life. Only another card takes the stage from it.
    if (next) {
      // And whatever was up is shoved off rather than cut, if it is a
      // *different* card: a beat re-rendered under its own key is the same
      // moment, not a new one.
      if (showing && showing.key !== next.key) setParting(showing)
      setShowing(next)
    }
  }
  useEffect(() => {
    if (!showing) return
    const done = window.setTimeout(() => setShowing(null),
      showing.life + STAGE_TAIL)
    return () => { window.clearTimeout(done) }
  }, [showing])
  useEffect(() => {
    if (!parting) return
    const gone = window.setTimeout(() => setParting(null), STAGE_PART)
    return () => { window.clearTimeout(gone) }
  }, [parting])
  return { showing, parting }
}

/* ------------------------------------------------ mana, in the same middle */

/**
 * **The mana that arrived**, flashed in the middle of the arena.
 *
 * This is the extension the top of this file left room for, arriving from the
 * one direction that comment did not predict: not another *card*, but another
 * kind of event entirely. Aaron, 2026-08-26, having watched the first attempt
 * put a bead on the tapped card instead: *"I wanted the mana symbol to maybe
 * just show in the middle like the cast cards do now."*
 *
 * Three things make it a different object from a staged card rather than a
 * variant of one, and all three are the reason it has its own hook:
 *
 * - **It is small and it is short.** A game holds fifty-odd pool movements
 *   against sixty-odd casts, and a card-sized reveal for every tapped Forest
 *   would be exhausting long before turn six. A row of pips at a fifth of a
 *   card's height, for two thirds of a cast's time, is a glance rather than a
 *   halt.
 * - **It shares the beat with the thing it paid for.** Mana arriving and the
 *   spell it was spent on land on *the same beat* — Forge raises no beat for a
 *   mana ability at all (it does not use the stack), so the whole fill and
 *   drain reaches the browser attached to the cast itself. If this took the
 *   card's slot the commonest mana in the game would evict the commonest card
 *   in the game, and the two would flicker over each other all match. It gets
 *   its own slot and its own place on the sand, and the two read as one
 *   sentence: this much mana, and what it bought.
 * - **It belongs to a player.** A cast card is centred because a spell happens
 *   to the table; mana happens to a *person*, so this is drawn in the half of
 *   the arena belonging to the seat that gained it. That costs nothing and
 *   says something the centre could not.
 *
 * **It never claims a source.** Pips, and no card behind them — Forge's mana
 * event carries a seat and a pool and nothing else, and ADR 44 is why nothing
 * here draws a line from a turned permanent to a mana.
 */
export interface StagedMana {
  /** The beat's own identity, so the same mana arriving twice plays twice
   *  rather than reconciling onto a finished animation. */
  key: string
  /** Whose mana, as the edge of the table they sit at — which is where this is
   *  drawn. */
  facing: 'far' | 'near'
  pips: Pip[]
  /** Both the CSS duration and the element's life, handed over as one number
   *  for `STAGE_LIFE`'s reason: two places writing down one length is two
   *  places for them to drift apart. */
  life: number
}

/** How long a mana flash is watched for, before any pace applies.
 *
 *  Two thirds of a cast, which is the ratio that came out of asking what each
 *  one is for: a card is *read* — a name, an art, a plate — and a row of pips
 *  is *counted*, and counting three of anything is quicker than reading a
 *  title. Long enough to see how many and what colour; gone before it is in
 *  the way of the spell it paid for. */
const MANA_LIFE = 760
/** The most beats a flash may outlive its own, and the floor under it. The
 *  same pair, and the same argument, as `STAGE_BEATS` and `STAGE_FLOOR`:
 *  tighter at the top because mana is the commoner event, and floored because
 *  a fifth of a fade is worse than no fade at all. */
const MANA_BEATS = 2
const MANA_FLOOR = 420

/** How long this flash is watched for, at this pace. */
export function manaLife(speed: Speed): number {
  const beat = beatDelay(speed)
  if (beat === 0) return MANA_LIFE
  return Math.max(MANA_FLOOR, Math.min(MANA_LIFE, beat * MANA_BEATS))
}

/**
 * What arrived in the middle, if anything did.
 *
 * A pure reading of the fold's own answer — `gained` is settled in
 * `lib/board.ts`, which is the only place that knows what each pool held going
 * *into* the beat. Null for a beat where no mana arrived, which is most of
 * them.
 */
export function stagedMana(gained: string, facing: 'far' | 'near', key: string,
  speed: Speed): StagedMana | null {
  const pips = poolPips(gained)
  if (!pips.length) return null
  return { key: `${key}:${facing}`, facing, pips, life: manaLife(speed) }
}

/**
 * Hold one seat's mana flash for its own life.
 *
 * A slimmer `useStaged`: one slot and no parting, because two mana flashes in a
 * row are two counts of the same thing and shoving the first one aside would
 * be ceremony for something that is over. It keeps the two rules that matter —
 * it can never strand a flash (every timeout is cleared by the effect that set
 * it) and it never carries one across a game — and it raises the flash during
 * the render for `useStaged`'s reason: mana that appeared a commit after the
 * beat that spent it would read as the wrong order of events.
 */
export function useStagedMana(next: StagedMana | null, key: string,
  game: number): StagedMana | null {
  const [showing, setShowing] = useState<StagedMana | null>(null)
  const [wasGame, setWasGame] = useState(game)
  const [wasKey, setWasKey] = useState<string | null>(null)
  if (game !== wasGame) {
    setWasGame(game)
    setWasKey(key)
    setShowing(next)
  } else if (key !== wasKey) {
    setWasKey(key)
    // A beat with no mana leaves the last flash alone to finish its own life,
    // exactly as a beat with no card leaves the stage alone.
    if (next) setShowing(next)
  }
  useEffect(() => {
    if (!showing) return
    const done = window.setTimeout(() => setShowing(null),
      showing.life + STAGE_TAIL)
    return () => { window.clearTimeout(done) }
  }, [showing])
  return showing
}
