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
 * Roman burial vault opening behind it; a permanent that is exiled gets it
 * with a road out of the city under it; and a permanent **nobody cast** gets
 * it with a battle already under way in the valley below.
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
import { type BoardCard, type Clash, halfNamed, pictureOf } from './board'
import { type HalfGlass } from './halves'
import { type Pip, poolPips } from './mana'
import { beatDelay, type Speed } from './reel'
import { legendName } from './theater'

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
 * - `put` — a real card arriving on the battlefield that **nothing cast**: an
 *   Atla Palani egg cracking into a Blightsteel Colossus, a reanimation, a
 *   blink, a Collected Company. `made` above answers the *token* half of
 *   Aaron's *"same thing for 'Enters the Battlefield'"*; this is the other
 *   half, and it is the half a newcomer most needs, because a creature
 *   appearing with nothing said about it is the one arrival they cannot
 *   account for. It was refused twice for want of the beat saying *nobody cast
 *   this* — see [mannerOf], which carries what changed and the counts behind
 *   it.
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
 * - **populate** — Forge's token-created event is a bare signal with no fields,
 *   so a populated copy is indistinguishable from any other token entering
 *   play. It needs a beat naming **the card being copied**, and saying that a
 *   copy is what this was. Given that, it is a `Manner` row, a `PLATE` row and
 *   a block of keyframes: the card, splitting into a mirrored second of itself.
 * - **eminence** — an eminence trigger reaches the stream as an `ability` beat
 *   with a zone and no words, so the room knows a commander did *something*
 *   from the command zone and cannot say what. **Which cat it did it to is
 *   now on the wire** and is drawn where the answer belongs, on the creature
 *   itself — `components/board.tsx`'s `Aimed`. What is still missing *here*
 *   is the ability's own **text**, and that is the part a plate would need:
 *   this stage says a sentence about a card, and "a commander did something"
 *   is not one.
 */
export type Manner = 'cast' | 'made' | 'put' | 'attach' | 'sacrificed'
  | 'dies' | 'exiled' | 'companion' | 'land'

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
 *   state, and there is nothing to infer. The **uncast real card** is the
 *   second exception and it is settled by the beat rather than the card — see
 *   the section below, which is the whole of this function's history.
 * - `sacrificed` fires for everything spent as a cost, and a *creature*
 *   sacrificed also dies — the scribe raises both, one beat apart, which is
 *   correct in Magic and would be two cards on this stage. So the death keeps
 *   creatures and planeswalkers (rule 700.4's own list) and this takes
 *   everything else: the Treasure, the Food, the Clue, the cracked fetchland.
 *   **A card whose type line the match never recorded is left alone**, because
 *   the room would be choosing between two drawings on no evidence, and
 *   silence is what it already does with everything it was not told.
 * ## The arrival that nobody cast
 *
 * Aaron, 2026-08-27, in the same breath as the crypt: *"Same thing for
 * 'Enters the Battlefield', we should be able to find something cool. A free
 * use painting or picture of a battle before us, like down in a valley...?"*
 * The token half of that ask is the `made` row above. This is the other half,
 * and it was refused twice before it was built — the refusals are kept here
 * because they are the argument for the shape it finally took.
 *
 * **How often it fires is a fact about the deck, not about the beat.** Gyome,
 * whose whole engine makes Food, raised 36 and 42 arrivals against 14 and 15
 * casts — and *clumped*, one turn raising fourteen of them and another
 * seventeen. Arahbo against Atla Palani, neither leaning on a token engine,
 * raised 5 and 9 against 5 and 11. So a scene on *every* arrival is a strobe
 * in some decks and unremarkable in others.
 *
 * What refuses *every* arrival is the rule directly above: almost all of them
 * were **cast one or two beats earlier and drawn then**, so a scene on the
 * arrival is the resolve, drawn twice.
 *
 * **Almost.** A first pass said that category was empty and was wrong: one
 * game turned up four arrivals nothing had cast — a Blightsteel Colossus, an
 * Emrakul, a Bonehoard Dracosaur and a Craterhoof Behemoth, all off Atla
 * Palani, Nest Tender, whose Eggs dying *"reveal cards from the top of your
 * library until you reveal a creature card. Put that card onto the
 * battlefield"*. A reanimation, a blink and a Collected Company are the same
 * shape. A newcomer watching an Emrakul appear out of nothing has exactly the
 * question this room exists to answer.
 *
 * **What was missing was the beat saying *nothing cast this*, and the beat
 * says it now.** `card.token` settled the token half because the scribe put
 * that flag on the wire; the other half needed the scribe to reach past the
 * view it is handed and ask the game model whether the card was cast, which it
 * now does. `entered` is that answer in Magic's own two words. Of fifty-nine
 * battlefield arrivals in the measured match, nineteen were cast; of the forty
 * that were not, twenty-six were lands (played, never cast) and thirteen were
 * tokens — leaving **one** real spell put onto the battlefield. That is the
 * rate this scene is drawn at, and it is why it can afford to be a scene.
 *
 * **An empty `entered` stays silent, and that is the case that matters.**
 * Every match already in the ledger was narrated before the scribe could
 * answer, and so is every match run by a worker on an older image. Reading
 * absence as "put" would redraw the whole archive as a room full of creatures
 * appearing from nowhere — every older game would read as cheating. So the
 * three states are three answers: `put` opens the field, `cast` says nothing
 * because the room already showed somebody paying for it, and *nobody said*
 * says nothing because nobody said.
 *
 * Two narrower cuts were considered along the way and stay rejected, because
 * `entered` made both unnecessary rather than merely wrong. *Only creatures*
 * is still mostly the resolve. *The first each turn* is a rule nobody could
 * learn by watching, which is commandment 2 backwards.
 */
export function mannerOf(kind: string, card?: ForgeBoardCard | null,
  entered?: string): Manner | null {
  switch (kind) {
    case 'cast': return 'cast'
    // **A land is the commonest beat in the room** — 11 a game, measured, and
    // in one real match more land drops than casts. It drew nothing for
    // exactly that reason, and that rule is lifted deliberately (Aaron,
    // 2026-08-28) rather than forgotten. What keeps it from being the arena
    // flashing at somebody every turn is length, not silence: see
    // `STAGE_LIFE.land` and `LAND_BEATS`.
    case 'land': return 'land'
    case 'dies': return 'dies'
    case 'exiled': return 'exiled'
    case 'attach': return 'attach'
    case 'companion': return 'companion'
    // The token first, because a token is *also* uncast and both flags are
    // true on one of them — Forge's own `wasCast` is false for every token, so
    // asking `entered` first would draw a Food Token on a battlefield instead
    // of drawing it being conjured, which is the wrong one of the two scenes.
    case 'enters': return card?.token ? 'made'
      : entered === 'put' ? 'put' : null
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
/**
 * Which scene the arena opens onto for this moment.
 *
 * **The manner says what happened; the scene says what kind of thing it
 * happened to** — and until now there was only one axis, so a creature that
 * somebody paid seven mana for got a glow and a plate while an identical
 * creature nobody cast got a whole battlefield (Aaron, 2026-08-28: *"I meant
 * all creatures should get that animation when they enter the battlefield"*).
 *
 * ## The rule, in one line each
 *
 * **A departure is a departure whatever the card was.** Exile takes the Appian
 * Way and death takes the columbaria, for every card alike — where a thing has
 * gone is the whole of what those two beats are about, and a Bolt and a Dragon
 * leave by the same door. Those two stay keyed on the manner and always will.
 *
 * **An arrival is about what arrived.** A creature walks into a fight already
 * under way, an artifact is struck out on an anvil, an enchantment is bound
 * over the world. Cast, put or conjured are three ways through the same door,
 * and the door is what the scene draws — which is why a cast creature, an
 * uncast one and a token creature all open the same valley.
 *
 * **A card that never lands gets an arcanum instead of a place** (Aaron's
 * call, 2026-08-28, and his own suggestion for the first of them: *"maybe we
 * could take the 'Tower' tarot card and use it for instants"*). An instant and
 * a sorcery are events rather than objects — there is nowhere for them to
 * arrive, so there is no place to draw — and the deck of the fortune-teller's
 * table is already here, already public domain, and already the room this
 * project keeps for things that are read rather than held. The Tower is
 * lightning through a crown and two figures falling, which is an instant; the
 * Magician is one hand raised and one lowered over a table of prepared tools,
 * which is a sorcery. It happens to you, or you do it.
 *
 * Null is a real answer and the commonest one after the six: a land, a
 * planeswalker, a battle, an Equipment strapped on, a sacrifice. Each of those
 * is a scene nobody has drawn, and a beat with no scene is what this room did
 * for all of them until today — the card, its light and its plate, which is
 * complete on its own.
 */
export type Scene = 'arva' | 'road' | 'crypt' | 'field' | 'forge' | 'temple'
  | 'veiling'
  | 'tower' | 'magician'

/** Whether a type line is an Aura, which is the one subtype this file reads.
 *
 *  **Subtypes are cut everywhere else and kept here**, and the exception earns
 *  itself: `castType` answers *Enchantment* for a Bear Umbra and for a Rhystic
 *  Study alike, and those are two different events. One is laid on a creature
 *  and rides it; the other settles over the table. They get different scenes,
 *  so the difference has to survive the read.
 *
 *  Forge writes `Enchantment - Aura` and Scryfall writes `Enchantment — Aura`;
 *  both are matched, because which one reaches a browser is not something a
 *  board should be sensitive to. */
function isAura(types: string | undefined): boolean {
  return !!types && /\bAura\b/.test(types)
}

export function sceneFor(manner: Manner, types: string | undefined):
Scene | null {
  switch (manner) {
    // Departures: the manner decides, and the card's kind is not consulted.
    case 'exiled': case 'companion': return 'road'
    case 'dies': return 'crypt'
    // **An Aura going onto a creature is an arrival like any other**, and it
    // is the one manner where that is not obvious: `attach` is drawn on the
    // beat the Aura reaches its host, which is the same beat it reaches the
    // battlefield. An Equipment being strapped on is a genuinely different
    // event — the sword already existed and has been picked up — so it falls
    // through to `castType` below and lands on the forge, which is where a
    // sword comes from.
    case 'attach': return isAura(types) ? 'veiling' : null
    // **A land opens the country it came from**, and it is the one manner that
    // answers before the kind is consulted — a land is a land, and there is no
    // sub-question to ask about what sort. The land row on the board is the
    // only other place this room says anything about them at all.
    case 'land': return 'arva'
    // Arrivals and castings: the kind decides.
    case 'cast': case 'put': case 'made': break
    default: return null
  }
  switch (castType(types)) {
    case 'Creature': return 'field'
    case 'Artifact': return 'forge'
    // An Aura cast from a hand opens the same rite as one attached, because it
    // is the same event: the difference between the two beats is which of them
    // Forge happened to report, and a room that drew two scenes for one thing
    // would be answering a question about the pipe.
    case 'Enchantment': return isAura(types) ? 'veiling' : 'temple'
    case 'Instant': return 'tower'
    case 'Sorcery': return 'magician'
    default: return null
  }
}

/** Which arcanum a scene draws, and null for the scenes that are places.
 *
 *  Served from `/tarot/`, which is where the reading room's own deck lives —
 *  the same 78 files, the same provenance, and no second copy. That is also
 *  why these are paths rather than bundler imports: the pictures are package
 *  data on the server, not part of anybody's JavaScript. */
export const ARCANA: Partial<Record<Scene, string>> = {
  tower: '/tarot/16-tower.webp',
  magician: '/tarot/01-magician.webp',
}

const STAGE_LIFE: Record<Manner, number> = {
  /* The commonest of them all by a wide margin — a game holds sixty or seventy
     casts and a dozen deaths — so it is the shortest, for the same reason
     `MARK_LIFE` keeps the attack lamp shortest of its three. */
  cast: 1150,
  /** **The shortest moment on this stage, and it has to be.** A cast is 1150
   *  and a land is two thirds of it. Nobody needs to *read* a land: they need
   *  to see the country it came from and get on with the turn, which is the
   *  same argument the mana flash makes at 760 for the same reason — a row of
   *  pips is counted, not read. Long enough to know a Forest from a Wastes;
   *  gone before the next thing happens. */
  land: 780,
  /* A cast plus a moment, and the moment is the number. A pile of tokens says
     one thing a single card does not — *how many* — and a count is read after
     the name rather than with it, so it needs the eye to come back. */
  made: 1300,
  /* The same, for the same reason from the other end: this plate carries a
     second line naming the host, and a line nobody finishes reading is a line
     that was not worth drawing. */
  attach: 1300,
  /* **The rarest thing a player does, so it gets the time.** One real spell in
     fifty-nine arrivals in the measured match, four in the one Atla Palani
     game this was built for — which puts it between the exile, which happens
     several times a game, and the companion, which happens once. It is also
     the only manner that is *both* watched and read: a card arriving has a
     journey the way an exile does, and a plate with a second line the way an
     attachment does, and neither half can have the other's time. Deliberately
     inside the four-beat cap below, so watching pace never clips it. */
  put: 1600,
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
/** How many beats a land may outlive its own, which is half what everything
 *  else gets. At watching pace this binds at 960ms and the land is already
 *  gone; at the fast end it cuts hard, which is the point — a beat that
 *  happens every turn must never be the thing still on screen when the turn
 *  after it starts. */
const LAND_BEATS = 2
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
  // A land keeps its own, tighter cap: see `LAND_BEATS`.
  if (manner === 'land' && beat !== 0) {
    return Math.max(STAGE_FLOOR, Math.min(STAGE_LIFE.land, beat * LAND_BEATS))
  }
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
 * - **"puts"** again for an uncast arrival, and the repeat is the right
 *   answer rather than a collision. It is one English verb doing one job in
 *   two places — a sword is *put on* a creature, a Colossus is *put onto the
 *   battlefield* — and the note line under each says which, exactly as it
 *   already does for the attachment. Magic uses the same word in both rules
 *   sentences, so a room that invented a second one would be teaching a
 *   distinction the game does not draw.
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
  // Magic's own verb. A land is *played*, never cast — it does not use the
  // stack, and a room that said "casts" here would be teaching a newcomer the
  // one thing about lands that most often has to be un-taught.
  land: { by: (who) => `${who} plays`, alone: 'Land' },
  made: { by: (who) => `${who} makes`, alone: 'Made' },
  /* The `alone` form carries the destination and the `by` form does not, which
     looks like an asymmetry and is one sentence read twice: "Gyome puts /
     Blightsteel Colossus" already stands on a picture of the battlefield, and
     a room hovering over the sand saying "onto the battlefield" is narrating
     what a person can see. Without a player there is no verb to hang it on, so
     the phrase has to be the whole plate. */
  put: { by: (who) => `${who} puts`, alone: 'Put onto the battlefield' },
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
 *
 * **An uncast arrival takes no type either, and it is the closest call of the
 * six.** The type is there for cards that never land: it says whether to
 * expect this one again. This one has landed — it is standing in a row on the
 * board a second later, where its own picture and its own lane say what it is
 * better than a word could. What the plate has room for instead is the fact
 * that nothing cast it, which nothing else on the screen says at all.
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
 * - **That nothing cast it.** The same failure as the companion and a beat
 *   earlier in a newcomer's evening: a creature standing on the battlefield
 *   that nobody was seen to pay for looks like somebody skipped a step. The
 *   plate above says a player put it there; this says the part that makes
 *   that worth a scene. Deliberately *"nothing cast it"* rather than "no mana
 *   was paid" — a mana cost may well have been paid somewhere, for the Egg or
 *   the reanimation spell, and the room does not know. What it knows is that
 *   this card was not cast, which is Forge's own boolean and not a reading.
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
  if (manner === 'put') return 'nothing cast it'
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
  /** **Which scene the arena opens onto**, or null for a beat that draws the
   *  card and nothing behind it. See [sceneFor], which decides it, and which
   *  carries the argument for why this is not simply the manner. */
  scene: Scene | null
  /** **The glass over the half of the card this beat is about**, or null for
   *  a card with one half — which is nearly every card.
   *
   *  All four two-named layouts wear one now. It was only ever an Adventure's
   *  second half, because that was the only box anybody had measured, and
   *  a split card and a flip card resolved to the right picture and got
   *  nothing (Aaron, 2026-08-28: *"split and flip cards find their picture now
   *  but get no glass"*). `lib/halves.ts` holds the measurements and the
   *  geometry; the numbers here are already in percentages of the frame, so
   *  the drawing does no arithmetic of its own. */
  half: HalfGlass | null
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
 * - **The spelling, and there are two ways it can differ.** Forge names a
 *   *face* and the board can carry Scryfall's combined `A // B` — a
 *   transforming card would otherwise be cast sixty times a match and found
 *   not once, silently, and only in the decks that play them. And Forge
 *   *renames* a card when its other half is cast, so a Bonecrusher Giant the
 *   board has been calling Bonecrusher Giant arrives here as Stomp. Both are
 *   `halfNamed`'s business rather than `===`'s.
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
    if (halfNamed(card, name) < 0) continue
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

/**
 * The bout: one attacker, and the wall that stopped it.
 *
 * **This is what the arrows became** (Aaron, 2026-08-28). An arrow across the
 * trench named a pair; it could not show them. The middle of the arena can, at
 * the size this stage already draws a spell — so a block stops being a line
 * between two small cards and becomes the one thing on the screen.
 *
 * **It keeps the board's own axis, which is why this shape was chosen over
 * three others.** Four layouts were drawn at the real stage size and looked at
 * (`the lists`, horizontal; `the ring`, an arc around a centred attacker; `the
 * gauntlet`, a file receding into the picture; and this one). Aaron chose the
 * charge: the attacker comes out of *its own seat's edge* and the wall ranks
 * across the defender's, so a person who has just read the board does not have
 * to re-learn which way the table faces. Far swings downward, near swings up.
 *
 * **Its cost is size and it is worth naming.** Two ranks in a 521-pixel arena
 * means nobody is at full stage size — the attacker is drawn at 0.62 of a
 * stage card and the wall at 0.52, which is smaller than anything else this
 * stage puts up. That is the price of showing both halves of a fight at once,
 * and it was paid deliberately.
 *
 * **The wall stacks** (Aaron: *"stacks of tokens can stay stacked with their x
 * number"*), so twelve Saprolings are one card with a twelve on it. That is
 * what a player sees across a table, and it is also what keeps the rank on the
 * stage: see `boutPitch` for what happens when even the stacks run out of room.
 */
export interface BoutFighter {
  /** The card's own board id.
   *
   *  **This is the identity that makes the wall assemble.** Forge announces a
   *  block per blocker, so a gang of three arrives as three beats and this
   *  moment is raised three times — each time with one more card in it. Keyed
   *  on the id, the cards already standing keep their DOM nodes and only the
   *  new one animates in, which is a wall being built rather than a wall being
   *  redrawn three times. */
  id: number
  name: string
  image: string | null
  /** How many identical cards this one stands for; 1 for almost everything,
   *  since Commander is singleton and only tokens repeat. */
  count: number
}

/** How a fight ended, or `null` while it is only being declared.
 *
 *  **Keyed on the attacker** (Aaron, 2026-08-28), which is what makes a mixed
 *  result answerable at all: a wall of three where two die and one lives has
 *  no single verdict, but the creature that swung into it either walked
 *  through or it did not. The backdrop answers for the attacker; the skulls on
 *  the individual cards answer for everyone else.
 *
 *  `null` for the declaration, which is most of them, and for a fight where
 *  **nobody died** — that one is not a quiet verdict, it is no verdict, and a
 *  triumph over a wall that is still standing would be the room claiming
 *  something the game did not say. */
export type Outcome = 'fell' | 'held'

export interface StagedBout {
  /** The beat's own identity, so each block in a gang resets the clock and the
   *  fight is still up when the last of them lands. */
  key: string
  attacker: BoutFighter
  blockers: BoutFighter[]
  /** Which seat's edge the attacker swings out of. */
  facing: 'far' | 'near'
  /** What the plate under the fight says. */
  word: string
  note: string | null
  /** How it ended, when this is the fight settling rather than being declared.
   *  Picks the scene: the ossuary if the attacker fell, the arch if it held. */
  outcome: Outcome | null
  /** The board id of the fighter that just died, so the bout can lay the stone
   *  on it.
   *
   *  **Both losers have to look like losers** (Aaron, 2026-08-28: *"when two
   *  things died in a clash, they both were losers, but only one showed the
   *  graveyard animation and the skull icon"*). The verdict took the beat away
   *  from the single-card death moment — which stopped three scenes stacking
   *  and quietly stopped the creature that fell from being buried at all. It
   *  is buried here instead, on its own card inside the fight, which is also
   *  the only place that can show a mixed wall: two of three blockers dead is
   *  two stones and one card standing. */
  dying: number | null
  life: number
}

/** How long a fight is watched for, before any pace applies.
 *
 *  Longer than any single card, and the reason is arithmetic rather than
 *  drama: there are `N + 1` cards to read here instead of one, and the plate
 *  under them names all of them. It is also the rarest of these moments — a
 *  game has sixty casts and a handful of blocks — so the cost of holding it is
 *  paid a few times a game. */
const BOUT_LIFE = 2000

/** How long a fight is watched for, at this pace. `stageLife`'s pair of caps,
 *  and its argument: a fast pace must not cut this to a strobe, and a slow one
 *  must not leave a fight standing over four later beats. */
export function boutLife(speed: Speed): number {
  const beat = beatDelay(speed)
  if (beat === 0) return BOUT_LIFE
  return Math.max(STAGE_FLOOR, Math.min(BOUT_LIFE, beat * STAGE_BEATS))
}

/** A blocker's width, and the stage's, **as fractions of the stage** rather
 *  than pixels.
 *
 *  A constant in pixels is a constant measured against one screen, which this
 *  room has already paid for once: the arena is drawn at 940 on a laptop and
 *  at whatever a phone gives it, and a rank laid out in pixels would be right
 *  on exactly one of them. These are the same numbers the layout was drawn and
 *  measured at, expressed against the box instead of against a monitor. */
export const BOUT_BLOCKER_W = 112.84 / 940
const BOUT_GAP = 14 / 940
/** How much of the stage the rank may span before it must overlap. Short of
 *  the full width on purpose: the mask fades the picture at the rim, and a
 *  card standing out there is a card standing in the fade. */
const BOUT_SPAN = 880 / 940

/**
 * How far apart the wall stands, as a fraction of the stage.
 *
 * **Overlap rather than shrink, and that was a choice.** Seven blockers at
 * their natural pitch span 874 of 940 pixels and an eighth does not fit. The
 * two ways out are to make every card smaller or to let them overlap, and
 * shrinking is the worse one: it makes a rare board illegible to punish it for
 * being rare, and it changes the size of a card for reasons that have nothing
 * to do with the card. Overlapped, a big wall reads as exactly what it is —
 * a rank closed up shoulder to shoulder — and every card in it stays the size
 * it was.
 *
 * Stacking makes this rare on its own: the boards that field eight blockers
 * are token boards, and twelve identical Saprolings arrive here as one card.
 */
export function boutPitch(n: number): number {
  const natural = BOUT_BLOCKER_W + BOUT_GAP
  if (n <= 1) return natural
  return Math.min(natural, (BOUT_SPAN - BOUT_BLOCKER_W) / (n - 1))
}

/** Where the `i`th of `n` blockers stands: the **left edge** as a fraction of
 *  the stage, so the rank is centred however wide it turns out to be. */
export function boutAt(i: number, n: number): number {
  return 0.5 + (i - (n - 1) / 2) * boutPitch(n) - BOUT_BLOCKER_W / 2
}

/** How many names the plate will read out before it stops. Three, because the
 *  fourth is where a label stops being a label: at nine blockers the untrimmed
 *  sentence ran the width of the arena and wrapped onto a second line under
 *  the cards it was describing. */
const WALL_NAMED = 3

/**
 * The wall, said the way a person says it.
 *
 * Three things, and each of them was a real fault in the first cut of this
 * sentence:
 *
 * - **Short names.** A comma-joined list of legends is unreadable, because
 *   half of them contain a comma: *"Brimaz, King of Oreskos, Arahbo, Roar of
 *   the World"* is four names to a reader and two to the game. `legendName` is
 *   the room's own cut, and it is deliberately **not** `shortName` beside it:
 *   these are card names, where the comma is Wizards' own separator between a
 *   legend and its title, and cutting on it is always right. `shortName` is
 *   for a *deck's* name, which is prose somebody wrote and whose commas are
 *   only a separator when the title turns out to be the general's.
 * - **A count rather than a plural.** Only tokens ever repeat, and pluralising
 *   a card's name is a guess about English that this project does not make
 *   about card text anywhere else — *Zombie Army* does not take an `s`. The
 *   board's own multiplication sign says the same thing and says it exactly.
 * - **A stop.** Past three names the plate is a paragraph. What is left is
 *   counted in *creatures* rather than in cards, because that is the question
 *   somebody is asking: a stack of twelve Saprolings and one Bear is thirteen
 *   creatures, not two.
 *
 * House style throughout: no Oxford comma, which is what the board's own
 * `listed` uses.
 */
function walled(blockers: BoutFighter[]): string {
  // **The cards stay apart and the sentence does not**, which is the one place
  // this deliberately disagrees with the rank standing above it.
  //
  // A real gang on Blightsteel Colossus put three Cat Tokens in the wall and
  // `stackRow` was right to draw all three: one was a 6/4 carrying Hammer of
  // Nazahn, one a 3/3 with a Basilisk Collar, one a bare 3/3. Three separate
  // cards, because a player can see all of that across a table. But the plate
  // read *"by Cat Token, Cat Token, Elephant Token and 1 more"*, which looks
  // like a fault however true it is. Nobody says a name twice; they say *three
  // Cats and an Elephant*. So the sentence counts by name, and the pictures
  // keep the distinction the sentence has no room for.
  const byName = new Map<string, number>()
  for (const b of blockers) {
    byName.set(b.name, (byName.get(b.name) ?? 0) + b.count)
  }
  const said = [...byName].map(([name, n]) =>
    n > 1 ? `${legendName(name)} ×${n}` : legendName(name))
  if (said.length > WALL_NAMED) {
    const rest = [...byName.values()].slice(WALL_NAMED)
      .reduce((n, c) => n + c, 0)
    return `${said.slice(0, WALL_NAMED).join(', ')} and ${rest} more`
  }
  if (said.length <= 1) return said[0] ?? ''
  return `${said.slice(0, -1).join(', ')} and ${said[said.length - 1]}`
}

/**
 * The fight the board is showing, if it is showing one.
 *
 * A pure reading of `clashOf`'s answer plus the card dictionary, which is the
 * only place a painting lives. Null for every beat that is not a block, which
 * is nearly all of them.
 *
 * **The plate says "Blocked" and not "blocks", and that is for a newcomer.**
 * Every other plate on this stage is a sentence about a player doing something
 * — *Gyome casts*, *Gyome sacrifices*. A block is the one beat where the
 * interesting party is the creature being stopped rather than the person doing
 * the stopping, and *"Blocked"* over the attacker's name says in one word what
 * the picture is showing. The wall is named underneath, where the note goes.
 */
export function stagedBout(clash: Clash | null, board: ForgeBoard | null,
  key: string, speed: Speed, outcome: Outcome | null = null,
  dying: number | null = null): StagedBout | null {
  if (!clash) return null
  // **A fighter fights in its own face's picture.** The dictionary paints a
  // card once, under the face it was filed by, and a permanent standing on the
  // other one — a modal double-faced card played as its land, a creature that
  // turned over — has a painting of its own that is not that. `card.name` is
  // the face the fold settled on, so asking `pictureOf` for the half that name
  // is gets the picture the board is already showing rather than the front of
  // a card whose back is in the fight.
  const fighter = (card: BoardCard, count: number): BoutFighter => {
    const face = faceFor(board, card.name, null)
    return {
      id: card.id,
      name: card.name,
      image: (face && pictureOf(face, halfNamed(face, card.name))) || null,
      count,
    }
  }
  const attacker = fighter(clash.attacker, 1)
  const blockers = clash.blockers.map((s) => fighter(s.card, s.count))
  return {
    key,
    attacker,
    blockers,
    facing: clash.swinging,
    // **Three words for three moments**, and the tense is the whole of it: a
    // fight is *Blocked* while it is being declared, and once the damage has
    // landed it is something that already happened.
    word: outcome === 'fell' ? 'Cut down'
      : outcome === 'held' ? 'Broke through' : 'Blocked',
    note: blockers.length ? `by ${walled(blockers)}` : null,
    outcome,
    dying,
    life: boutLife(speed),
  }
}

/**
 * Hold one fight for its own life.
 *
 * `useStagedMana`'s shape and its two rules — it can never strand a fight, and
 * it never carries one across a game — with one difference that is the whole
 * reason a gang assembles rather than flickering: **a new fight replaces the
 * slot's value, and the component keyed on the attacker keeps its element.**
 * Three cats on a Ghalta re-raise this three times with a bigger wall each
 * time; because every one of them is the same attacker, React reconciles onto
 * the same `StageBout` and only the arriving card is new.
 */
export function useStagedBout(next: StagedBout | null, key: string,
  game: number): StagedBout | null {
  const [showing, setShowing] = useState<StagedBout | null>(null)
  const [wasGame, setWasGame] = useState(game)
  const [wasKey, setWasKey] = useState<string | null>(null)
  if (game !== wasGame) {
    setWasGame(game)
    setWasKey(key)
    setShowing(next)
  } else if (key !== wasKey) {
    setWasKey(key)
    // A beat that is not a block leaves the fight up to finish its own life,
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
