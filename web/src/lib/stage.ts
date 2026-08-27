/**
 * What the centre of the arena shows, and for how long.
 *
 * **Half of Commander never touches the battlefield.** A Lightning Bolt is
 * cast, resolves, and is in a graveyard before the sentence describing it has
 * finished being read — and until now the Coliseum drew nothing at all for it,
 * because the board draws permanents and a board is a place that holds what
 * stays (Aaron, 2026-08-26: *"there is nothing to mark their existence in the
 * coliseum"*). So every cast gets a moment in the middle of the sand; a
 * creature that dies gets the same moment with the stone played over it and a
 * Roman burial vault opening behind it; and a permanent that is exiled gets it
 * with a road out of the city under it.
 *
 * **And the plate under it is a sentence about a player**, which is the second
 * ask and the one that decided the shape of everything below (Aaron,
 * 2026-08-27: *"It would be nice if we added the players name too, Gyome CASTS
 * Creature, etc"*, and *"for an aura or something that targets something if
 * the text box called out the target too"*). A moment on this stage is
 * therefore **who**, **what they did**, **which card**, and **to what** — four
 * pieces, three of which are usually empty, and a plate that only ever draws
 * what it was actually told.
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
 * Every one of these is drawn today, and three more are named in this comment
 * without being drawn, which is deliberate rather than unfinished: **the shape
 * of the data they need is the thing that is missing, and guessing at it in a
 * browser would be the room claiming a mechanic happened when nothing told it
 * so.**
 *
 * - `cast` — every spell, from Sol Ring to the sorcery that wins the game.
 *   Forge's `GameEventSpellAbilityCast`, filtered to spells, already crosses as
 *   a `cast` beat carrying the card's name.
 * - `dies` — a creature leaving the battlefield for a graveyard. Already on the
 *   wire, and already drawn twice elsewhere: the skull on the card in its row,
 *   the ghost rising off the graveyard pile. This is the middle of that, and
 *   since 2026-08-27 it is the *place* as well as the event — the vault the
 *   card goes down into, which `components/stage.tsx`'s `StageCrypt` argues
 *   alongside the question of whether the stone should have given way to it.
 * - `exiled` — a permanent leaving the battlefield for exile. `dies`'s twin,
 *   and it arrived for the same reason: a great deal of Commander's removal
 *   exiles rather than destroys, and a Path to Exile used to take a creature
 *   off the sand with nothing said about it at all (Aaron, 2026-08-27). Raised
 *   in Go by the scribe, which also argues why it is every permanent rather
 *   than only creatures and why only the battlefield raises it.
 * - `made` — a **token** arriving. The one thing on this stage that was never
 *   cast, never drawn and never in anybody's deck: it is conjured, so nothing
 *   before it announced it and the board's own row is the only place it has
 *   ever appeared (Aaron, 2026-08-27: *"Token creation should show in the
 *   center stage, with how many tokens were created"*). The count comes from
 *   [countRuns] in `lib/reel.ts`, which argues at length why it is a count and
 *   not a guess.
 * - `sacrificed` — a permanent its controller spent. Aaron's word for the hole
 *   was *"before they go to the ether"*, and the ether is where a Food, a Clue
 *   and a Treasure go: they are artifacts, so rule 700.4 does not let them
 *   *die*, and `dies` was correctly silent about the commonest disappearance
 *   in a Gyome game. **Only what cannot die takes this**, which is what keeps
 *   the two from doubling — a sacrificed creature raises both beats one after
 *   the other, and drawing both would shove one card off the stage to put the
 *   same card straight back.
 * - `attach` — an Aura or an Equipment finding a host, and the only beat on
 *   this wire that carries a **target**. A cast does not: Forge announces the
 *   spell and the attachment as two separate moments, so *"for an aura or
 *   something that targets something if the text box called out the target
 *   too"* (Aaron, 2026-08-27) is answered here rather than on the cast.
 * - `companion` — a companion bought in from outside the game. See `PLATE`.
 *
 * Not yet, and what each would need:
 *
 * - **an arrival nobody cast** — a reanimation, a blink, a Collected Company,
 *   or an Atla Palani Egg dying and putting a Blightsteel Colossus onto the
 *   battlefield. `made` above answers the *token* half of Aaron's *"same thing
 *   for Enters the Battlefield"*; this is the other half, and it is the half a
 *   newcomer most needs, because a creature appearing with nothing said about
 *   it is the one arrival they cannot account for. The `enters` beat already
 *   exists and already names the card. What is missing is the bit that
 *   separates it from the resolve: **the beat saying that nothing cast this**.
 *   `card.token` settles the token half only because the scribe put that flag
 *   on the wire; nothing on the wire settles this one. The scribe knows — it
 *   saw, or did not see, the cast for that same id — and the browser cannot
 *   work it out, because a stage item is built from one beat and this file is
 *   deliberately not handed the history. Without it the only honest choice is
 *   every non-token arrival or none, and [mannerOf] carries the counts that
 *   make "every" the wrong one.
 * - **populate** — Forge's token-created event is a bare signal with no fields,
 *   so a populated copy is indistinguishable from any other token entering
 *   play. It needs a beat naming **the card being copied**, and saying that a
 *   copy is what this was. Given that, it is a `Manner` row, a `PLATE` row and
 *   a block of keyframes: the card, splitting into a mirrored second of itself.
 * - **eminence** — an eminence trigger reaches the stream as an `ability` beat
 *   with a zone and no words, so the room knows a commander did *something*
 *   from the command zone and cannot say what. It needs the ability's own
 *   **text**, because the whole reason to draw eminence is that a newcomer
 *   cannot otherwise see why the board just changed. The words are the part
 *   that matters more than the glow.
 */
export type Manner = 'cast' | 'made' | 'attach' | 'sacrificed' | 'dies'
  | 'exiled' | 'companion'

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
 *
 * **Two kinds cannot answer from the kind alone**, which is why the card is
 * passed in, and both of them are the same rule: *nothing is drawn twice*.
 *
 * - `enters` fires for every permanent arriving, and almost all of them were
 *   cast one beat earlier and drawn then. A **token** is the exception that
 *   makes the beat worth having: it was never cast, so this is the only moment
 *   it has. `ForgeBoardCard.token` is the scribe's own flag, off Forge's card
 *   state, and there is nothing to infer.
 * - `sacrificed` fires for everything spent as a cost, and a *creature*
 *   sacrificed also dies — the scribe raises both, one beat apart, which is
 *   correct in Magic and would be two cards on this stage. So the death keeps
 *   creatures and planeswalkers (rule 700.4's own list) and this takes
 *   everything else: the Treasure, the Food, the Clue, the cracked fetchland.
 *   **A card whose type line the match never recorded is left alone**, because
 *   the room would be choosing between two drawings on no evidence, and
 *   silence is what it already does with everything it was not told.
 * ## The arrival that nobody cast, and why it is still missing
 *
 * Aaron, 2026-08-27, in the same breath as the crypt: *"Same thing for
 * 'Enters the Battlefield', we should be able to find something cool. A free
 * use painting or picture of a battle before us, like down in a valley...?"*
 * The token half of that ask is the `made` row above. The other half is not
 * built, and the reason was measured rather than guessed at — five games
 * across three matches, counting what `enters` actually carries.
 *
 * **How often it fires is a fact about the deck, not about the beat.** Gyome,
 * whose whole engine makes Food, raised 36 and 42 arrivals against 14 and 15
 * casts — and *clumped*, one turn raising fourteen of them and another
 * seventeen. Arahbo against Atla Palani, neither leaning on a token engine,
 * raised 5 and 9 against 5 and 11. So a scene on *every* arrival is a strobe
 * in some decks and unremarkable in others, which is a reason to be careful
 * rather than a reason to refuse.
 *
 * What refuses it is the rule directly above: almost every arrival that is not
 * a token was **cast one or two beats earlier and drawn then**, so a scene on
 * it is the resolve, drawn twice.
 *
 * **Almost.** A first pass of this paragraph said that category was empty and
 * was wrong: a third match turned up four arrivals in a single game that
 * nothing had cast — a Blightsteel Colossus, an Emrakul, a Bonehoard
 * Dracosaur and a Craterhoof Behemoth, all off Atla Palani, Nest Tender, whose
 * Eggs dying *"reveal cards from the top of your library until you reveal a
 * creature card. Put that card onto the battlefield"*. A reanimation, a blink
 * and a Collected Company are the same shape. A newcomer watching an Emrakul
 * appear out of nothing has exactly the question this room exists to answer,
 * and that is the beat the valley belongs to.
 *
 * **It is unbuildable from here rather than refused**, which puts it with
 * `populate` and `eminence` above and not with `land` and `resolve` below.
 * `card.token` settles the token half because the scribe put that flag on the
 * wire; nothing on the wire settles the other half. The beat carries a name, a
 * seat and a turn, and [Staged] is built from one beat with no history to
 * scan. What it needs is the beat itself saying **nothing cast this** — which
 * the scribe knows, having seen or not seen the cast for that same id. Given
 * that, it is one row in [Manner], one in [PLATE], one in `STAGE_LIFE` and a
 * scene: a field seen from above.
 *
 * Two narrower cuts were considered and stay rejected. *Only creatures* is
 * still mostly the resolve. *The first each turn* is a rule nobody could learn
 * by watching, which is commandment 2 backwards.
 */
export function mannerOf(kind: string, card?: ForgeBoardCard | null):
  Manner | null {
  switch (kind) {
    case 'cast': return 'cast'
    case 'dies': return 'dies'
    case 'exiled': return 'exiled'
    case 'attach': return 'attach'
    case 'companion': return 'companion'
    case 'enters': return card?.token ? 'made' : null
    case 'sacrificed': return canDie(card?.types) === false
      ? 'sacrificed' : null
    default: return null
  }
}

/** Whether rule 700.4 would call this card leaving the battlefield a death —
 *  and `null` for a card whose type line nobody recorded, which is a third
 *  answer rather than a no. */
function canDie(types: string | undefined): boolean | null {
  const kind = castType(types)
  if (kind === null) return null
  return kind === 'Creature' || kind === 'Planeswalker'
}

/**
 * The card's own kind, in the one word a player would use for it.
 *
 * Aaron, 2026-08-27: *"we say 'CAST' and the card name, I would also like to
 * display the type, like 'CAST CREATURE' or 'CAST SORCERY'."* Which is a
 * better ask than it looks, and the reason is commandment 2. Half of what
 * crosses this stage never reaches the battlefield, so the picture is all
 * anybody gets — and a newcomer watching a Lightning Bolt appear and vanish
 * has no way to know whether that was a permanent they should expect to see
 * again. The type is the difference between *a card happened* and *a spell
 * resolved and is gone*.
 *
 * **Reading the line rather than the card**, which is `rowFor`'s precedent one
 * file over: the type line is on the wire already, and a browser that had to
 * look a card up to draw a word would be a second place for Magic facts to
 * rot.
 *
 * **The order is a priority and not a search**, because most cards have more
 * than one type and only one of them is the answer. An Artifact Creature is a
 * *creature* — that is what it does, what it attacks with and what a player
 * calls it — and a Legendary Enchantment Artifact — Equipment is an artifact.
 * So the list runs from the type that most decides how a card behaves to the
 * type that least does.
 *
 * Supertypes never appear: Legendary, Basic, Snow and World are adjectives on
 * a card, not what it is. Kindred is left out for a different reason — it
 * never occurs alone, so it can only ever hide the type sitting beside it.
 * Subtypes are cut with the line, because "Cast Creature" is the useful word
 * and "Cast Cat Warrior" is the card's own name said twice.
 */
const CAST_TYPES = ['Creature', 'Planeswalker', 'Battle', 'Instant', 'Sorcery',
  'Enchantment', 'Artifact', 'Land'] as const

export function castType(types: string | undefined): string | null {
  if (!types) return null
  // Forge writes `Legendary Creature - Cat Warrior`; Scryfall's long dash also
  // turns up on this wire, so both separators are cut. Everything after either
  // one is a subtype and none of it is the answer.
  const head = types.split(/\s[-—]\s/)[0] ?? types
  return CAST_TYPES.find((t) => head.includes(t)) ?? null
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
  /* The commonest of them all by a wide margin — a game holds sixty or seventy
     casts and a dozen deaths — so it is the shortest, for the same reason
     `MARK_LIFE` keeps the attack lamp shortest of its three. */
  cast: 1150,
  /* A cast plus a moment, and the moment is the number. A pile of tokens says
     one thing a single card does not — *how many* — and a count is read after
     the name rather than with it, so it needs the eye to come back. */
  made: 1300,
  /* The same, for the same reason from the other end: this plate carries a
     second line naming the host, and a line nobody finishes reading is a line
     that was not worth drawing. */
  attach: 1300,
  /* A Food going is a smaller event than a Bolt resolving and it happens more
     often — a dozen a game in a deck built to do it — so it takes the cast's
     length and not the death's. */
  sacrificed: 1150,
  /* Normally never read: a death takes the mark's own length, because the skull
     on the card in its row, the ghost off the grave and the card in the middle
     are **one event drawn in three places** and one event gets one clock. It is
     written down anyway so that a stage rendered without that number still gets
     the right length rather than a plausible-looking default. */
  dies: 2000,
  /* The longest thing that is not a death, because it is the only plate in the
     room carrying a **rule** rather than a name. It happens once in a game, at
     most twice, so the time is spent on something a person sees once — and the
     sentence it has to land is the one Aaron did not believe when he watched
     it happen. Deliberately just inside the four-beat cap below, so watching
     pace never clips it and only the fast end shortens it. */
  companion: 1800,
  /* The longest of the ones a player does, and the only one where the length
     is doing work rather than being spent. A cast is *read* — a name, an art,
     a plate — and it can end the moment the eye has them. An exile is
     **watched**: the card goes down a road and the whole point is that it gets
     smaller and further away, which is a thing that takes time or does not
     happen. Under about a second and a quarter the card jumps rather than
     leaves. This is still inside the cap below, so a fast pace shortens it
     like anything else. */
  exiled: 1500,
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

/**
 * What the plate under the card says, in Magic's own words.
 *
 * A museum plate rather than a caption: the arena is a building full of
 * exhibits and this is the one moment a card is held up to be looked at. The
 * words are the game's own — a creature *dies*, which is the rules term and
 * also the plainer of the two for somebody at their first game.
 *
 * **Two forms, and which one is used is a fact about the beat rather than a
 * fallback.** Aaron asked for the player: *"It would be nice if we added the
 * players name too, Gyome CASTS Creature."* So a deed somebody *did* is
 * written as they did it — `by`, in the third person, with their name in front
 * of it. A thing that merely *happened to a card* has no doer, and `alone` is
 * the whole plate for it. That is not a degraded version of the first: a
 * creature dying is not something its controller chose, `beatLine` returns no
 * player for exactly that reason, and a plate reading "Gyome dies" would name
 * the wrong subject entirely. `by` is `null` for those two, so it cannot
 * happen even if a player arrives on the beat later.
 *
 * `alone` still stands for every other manner, and it is a real state rather
 * than dead code — two ways over. The room turns a deck's slug into a name off
 * the shelf, so a finished match reopened before the shelf answers has beats
 * whose player nobody can name yet. And **a sacrifice has no seat on the wire
 * at all** (measured on a live match, 2026-08-27), so today every one of them
 * reads "Sacrificed" rather than "Gyome sacrifices". That is a hole in Go, and
 * this is the right thing to draw while it is open: the room saying less than
 * it would like, rather than naming a player nothing told it about.
 *
 * A word on three of them:
 *
 * - **"puts"** for an attachment, rather than "attaches" or "equips". The
 *   rulebook's word is *attached*, and `beatLine` already argues why the
 *   plainer one wins here: this is read by somebody watching their first game,
 *   and the host is named on the line under it, so "Gyome puts / Bloodforged
 *   Battle-Axe / on Syr Gwyn" is a whole sentence in three short pieces.
 * - **"makes"** for a token, which is the word a player says out loud, and it
 *   is doing more than naming the event — it says the card was *conjured*
 *   rather than drawn or cast, which is the one thing about tokens a newcomer
 *   has to be told once.
 * - **"'s companion"** rather than a verb, because the word `companion` is the
 *   whole point of the plate. Aaron watched Kaheera arrive in a hand and
 *   thought the game had cheated; the fix is not a smoother sentence, it is
 *   the mechanic's own name on screen where it can be looked up, with the rule
 *   under it. Commandment 2, and the same argument `beatLine` makes for
 *   keeping the word "exiled".
 */
export const PLATE: Record<Manner, { by: ((who: string) => string) | null;
  alone: string }> = {
  cast: { by: (who) => `${who} casts`, alone: 'Cast' },
  made: { by: (who) => `${who} makes`, alone: 'Made' },
  attach: { by: (who) => `${who} puts`, alone: 'Put on' },
  sacrificed: { by: (who) => `${who} sacrifices`, alone: 'Sacrificed' },
  companion: { by: (who) => `${who}'s companion`, alone: 'A companion' },
  dies: { by: null, alone: 'Dies' },
  exiled: { by: null, alone: 'Exiled' },
}

/**
 * What the plate says, in full.
 *
 * The deed, and then the card's kind when there is one and it adds something.
 * **Only a cast and a token get the type**, and the asymmetry is the point
 * rather than an omission.
 *
 * "Gyome casts Creature" answers a question somebody watching actually has,
 * because half of what is cast never lands and the type is the only clue about
 * which half this was. A token gets it for a different reason with the same
 * shape: a Servo Token is a creature and a Food Token is not, they are drawn
 * in different rows for that reason, and the name alone does not say which.
 *
 * "Dies Creature" answers nothing — rule 700.4 gives that word to creatures
 * and planeswalkers, so a thing that dies is already one of two things, and
 * the picture on the stage is the other half of the answer. Exile sits with it
 * for the same reason from the other side: it is drawn on the way *out*, and
 * where the card has gone matters more than what it was. An attachment and a
 * companion are both already named by their own word.
 */
export function plateWord(manner: Manner, types: string | undefined,
  who?: string | null): string {
  const said = PLATE[manner]
  const head = (who && said.by) ? said.by(who) : said.alone
  const kind = (manner === 'cast' || manner === 'made') ? castType(types) : null
  return kind ? `${head} ${kind}` : head
}

/**
 * The rest of the sentence, under the card's name — and null for the four
 * manners that are a whole sentence already.
 *
 * **One line, and it is the half the card cannot show.** A plate is read in
 * about a second while a picture the size of the arena is competing with it,
 * so the test for anything on it is not *is this true* but *would somebody who
 * missed it have misread what they saw*. Two things pass:
 *
 * - **The host.** An Aura on the stack is a card; an Aura on a creature is a
 *   different creature. Without the name the picture is a card being held up
 *   for no visible reason, which is exactly what Aaron asked about — *"for an
 *   aura or something that targets something if the text box called out the
 *   target too"*.
 * - **Where a companion came from.** This is the whole reason that beat
 *   exists: a card appearing in a hand nobody dealt it to looks like cheating,
 *   and it looks like cheating *because the room said nothing*. "Outside the
 *   game" is the fact, and the three is the rule — 702.139b fixes it for every
 *   companion there has ever been, which is why it can be stated here while
 *   nothing on the wire attributes a single mana to it (ADR 44). A number read
 *   off whichever lands happened to tap would be the room guessing.
 *
 * Everything else is left off. "Gyome sacrifices Food Token, into the
 * graveyard" spends a line on a fact the graveyard pile is already drawing.
 */
export function plateNote(manner: Manner,
  target: string | undefined): string | null {
  if (manner === 'attach') return `on ${target || 'a permanent'}`
  if (manner === 'companion') {
    return 'from outside the game — three mana paid'
  }
  return null
}

/** One card, on the stage, for one moment. */
export interface Staged {
  /** What the plate under the card reads, already assembled — see
   *  [plateWord]. Settled here rather than in the drawing, so the one place
   *  that knows the card's type line is the one place that reads it. */
  word: string
  /** The rest of the sentence, or null — see [plateNote]. */
  note: string | null
  /** How many identical cards this one moment is about, from [countRuns] in
   *  `lib/reel.ts`. `1` for almost everything; four when four tokens were
   *  conjured at once, and the stage says so the way the board does. */
  count: number
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
