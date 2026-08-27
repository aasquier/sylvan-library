/**
 * The centre of the arena: the card that is only there for a moment.
 *
 * **Everything here is mechanism, and that is a decision rather than a
 * shortcut.** The suite runs on jsdom, which has no layout engine and resolves
 * `index.css` to nothing at all — it cannot see how big the card is, where it
 * sits, whether it covers the board or whether a single frame of it ever
 * animated. So none of that is asserted here, because an assertion that cannot
 * fail is worse than no assertion: it reads like coverage. Size, position and
 * motion were checked in a browser, in both themes, at 375 and at desktop.
 *
 * What jsdom *can* hold honestly is the half that has actually gone wrong in
 * this room before:
 *
 * - whether the right beat puts a card up at all, and finds it a picture;
 * - whether it goes away again, ever, at the right time;
 * - whether a burst of them piles up;
 * - whether one leaks across a game, or past an unmount.
 *
 * That last group is why the durations are read as *values* rather than
 * trusted: the number in the stylesheet and the number the element lives for
 * are one number, and the only way they stay one is if the test reads what a
 * browser would read.
 */

import { act, cleanup, render } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import type { ForgeBoard } from '../lib/api'
import type { Speed, StagedBeat } from '../lib/reel'
import { castType, faceFor, mannerOf, plateNote, plateWord, stageLife }
  from '../lib/stage'
import { MatchBoard } from './board'

afterEach(cleanup)

/**
 * A match with the thing the board could never draw in it.
 *
 * `Lightning Bolt` is the whole point: it is cast, it resolves, it is in a
 * graveyard, and it is on no battlefield at any moment this board is ever
 * folded to. It is in the card list anyway — the scribe names a card on any
 * zone line at all, before any filter, so an instant that only ever went
 * hand-to-stack-to-graveyard is named and painted exactly as a creature is.
 * That fact is the foundation the whole feature stands on, so the fixture is
 * built to fail loudly if it stops being true.
 *
 * `Ancestral Vision` is the other half: a real card the pool could not paint,
 * standing in for every name the art lookup misses. It must draw a plate and
 * never a hole.
 *
 * The rest are the cards that make a *sentence* possible: a token nothing ever
 * cast, an Equipment with a host to name, a companion that was never dealt,
 * and a Food that cannot die because rule 700.4 does not let an artifact.
 */
const MATCH: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 10, name: 'Fleecemane Lion', types: 'Creature - Cat', seat: 1,
      image: 'https://example.test/lion.jpg' },
    { id: 11, name: 'Forest', types: 'Basic Land - Forest', seat: 1,
      image: 'https://example.test/forest.jpg' },
    // Cast, resolved, and in a graveyard. Never a permanent, never in a row.
    { id: 12, name: 'Lightning Bolt', types: 'Instant', seat: 1,
      image: 'https://example.test/bolt.jpg' },
    // Named by the match, and never painted by the pool.
    { id: 13, name: 'Ancestral Vision', types: 'Sorcery', seat: 1 },
    // Conjured. Nothing cast it, so the beat it enters on is the only moment
    // it will ever have.
    { id: 14, name: 'Clue Token', types: 'Artifact - Clue', seat: 1,
      token: true, image: 'https://example.test/clue.jpg' },
    { id: 15, name: 'Food Token', types: 'Artifact - Food', seat: 1,
      token: true, image: 'https://example.test/food.jpg' },
    { id: 16, name: 'Bloodforged Battle-Axe',
      types: 'Legendary Artifact - Equipment', seat: 1,
      image: 'https://example.test/axe.jpg' },
    { id: 17, name: 'Kaheera, the Orphanguard',
      types: 'Legendary Creature - Cat Beast', seat: 1,
      image: 'https://example.test/kaheera.jpg' },
    { id: 20, name: 'Dragonlord Atarka', types: 'Creature - Dragon', seat: 2,
      image: 'https://example.test/atarka.jpg' },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [{ id: 11, zone: 'land', seat: 1 }] },
    { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
    { turn: 2, seat: 1, changes: [{ id: 12, zone: 'graveyard', seat: 1 }] },
  ],
} as unknown as ForgeBoard

/** A beat as the room stages it: `who` is a **name**, already shortened off the
 *  shelf (`shortName` in `lib/theater.ts`), because that is what reaches the
 *  plate. `run` is what `countRuns` puts there — one, unless a test is about
 *  several of one card arriving at once. */
function said(over: Partial<StagedBeat> & { key: string }): StagedBeat {
  return {
    game: 1, turn: 2, kind: 'cast', who: 'Arahbo', text: '', run: 1, ...over,
  }
}

/** The board, drawn again with a different beat — which is the mechanism the
 *  stage is *made of*, so these drive it rather than a proxy for it. */
function replay(first: StagedBeat | null,
  opts: { speed?: Speed; game?: number; shown?: number } = {}) {
  const props = (beat: StagedBeat | null, over: typeof opts = {}) => ({
    board: MATCH, shown: over.shown ?? opts.shown ?? 3,
    game: over.game ?? opts.game ?? 1, running: false, beat,
    name: (_slug: string | null, fallback: string) => fallback,
    speed: over.speed ?? opts.speed ?? ('play' as Speed), setSpeed: vi.fn(),
    of: 3, seek: vi.fn(), games: [1], playing: 1, chooseGame: vi.fn(),
  })
  const view = render(<MatchBoard {...props(first)} />)
  return {
    ...view,
    then: (next: StagedBeat | null, over: typeof opts = {}) =>
      view.rerender(<MatchBoard {...props(next, over)} />),
  }
}

const cards = (c: HTMLElement) => c.querySelectorAll('.stage-card')
const face = (c: HTMLElement) =>
  c.querySelector('.stage-card:not(.is-parting) .stage-face')

it('gives an instant that never touched the board its moment', () => {
  // **The whole feature in one assertion.** A Lightning Bolt is cast, resolves
  // and is in a graveyard inside a single beat, so it is on no battlefield the
  // board is ever folded to — and before this the room drew nothing at all for
  // it (Aaron, 2026-08-26: *"there is nothing to mark their existence"*). The
  // beat carries a name and no id, so the picture has to be *found*, and the
  // place it is found is the match's own card list, which names every card the
  // game touched rather than only the ones that stayed.
  const { container } = replay(said({ card: 'Lightning Bolt', key: 'a1' }))

  // It is nowhere on the sand — which is the premise, so it is asserted rather
  // than assumed. If a Bolt ever starts appearing in a row, this feature is
  // solving a problem that has moved.
  expect(container.querySelector('.field-row img[alt="Lightning Bolt"]'),
    'an instant is on no battlefield').toBeNull()

  const shown = face(container)
  expect(shown?.getAttribute('src'), 'and its picture is found by name')
    .toBe('https://example.test/bolt.jpg')
  // **And the plate names the player and the kind**, which is the whole of why
  // either is worth drawing: this card is on no battlefield and never will be,
  // so the word "Instant" is the only thing telling somebody at their first
  // game that it was never going to stay, and the name is the only thing
  // saying whose spell it was. The type comes off the match's own card list —
  // the same lookup that found the picture, asked once.
  expect(container.querySelector('.stage-plate-word')?.textContent)
    .toBe('Arahbo casts Instant')
  expect(container.querySelector('.stage-plate-title')?.textContent)
    .toBe('Lightning Bolt')
  // Nothing under it: a cast names no target on this wire, and a line saying
  // so would be the room filling space.
  expect(container.querySelector('.stage-plate-note')).toBeNull()
})

it('sets a card in type when the match never painted one', () => {
  // **A missing painting is not a missing card.** Drawing nothing here would
  // put the hole back exactly where this feature came from, and drawing a
  // broken image would be worse than the hole. So the name is set in type, in
  // a frame — which is a real card, legible, and on a phone arguably the more
  // readable of the two (commandment 2: nothing that makes a newcomer feel
  // shut out).
  const { container } = replay(said({ card: 'Ancestral Vision', key: 'p1' }))

  expect(container.querySelector('.stage-face img'), 'no broken picture')
    .toBeNull()
  expect(container.querySelector('.stage-plate-name')?.textContent)
    .toBe('Ancestral Vision')
  expect(container.querySelector('.stage-plate')?.className)
    .toContain('stage-face')
})

it('lets a card go, rather than leaving it over the board', async () => {
  // **The one failure this may never have.** A card stuck over the arena is
  // worse than no card at all: the board is the thing a person came to watch,
  // and this is drawn on top of it. So the timer is driven rather than
  // reasoned about, and the assertion is that the arena comes back.
  vi.useFakeTimers()
  try {
    const { container, then } = replay(said({ card: 'Lightning Bolt', key: 'l1' }))
    expect(cards(container)).toHaveLength(1)

    // A beat with nothing of its own to say leaves the card alone — the card
    // is timed by the card, not by the beat, which is the correction the marks
    // had to make first. See `MARK_LIFE` in `board.tsx`.
    then(said({ kind: 'life', card: undefined, key: 'l2' }))
    expect(cards(container), 'a silent beat does not take it down')
      .toHaveLength(1)

    // 1150ms at watching pace, plus the tail that lets it finish fading.
    await act(async () => { await vi.advanceTimersByTimeAsync(1150 + 90 + 5) })
    expect(cards(container), 'and then it is gone on its own').toHaveLength(0)
    expect(container.querySelector('.stage'),
      'and the arena is left as it was').toBeNull()
  } finally {
    vi.useRealTimers()
  }
})

it('shoves one spell off with the next rather than piling them up', async () => {
  // A ritual and the spell it paid for, a cascade, a storm count: two casts a
  // beat apart is an ordinary thing, and the honest failure modes are both
  // visible from here. Three cards on screen is a pile; a hard cut is a
  // glitch. So there are at most two — the one arriving and the one leaving —
  // and the leaving one is gone inside a fifth of a second.
  vi.useFakeTimers()
  try {
    const { container, then } = replay(said({ card: 'Lightning Bolt', key: 'b1' }))
    then(said({ card: 'Fleecemane Lion', key: 'b2' }))
    then(said({ card: 'Dragonlord Atarka', key: 'b3' }))

    expect(cards(container), 'never more than the two')
      .toHaveLength(2)
    expect(container.querySelectorAll('.stage-card.is-parting'),
      'and exactly one of them is on its way out').toHaveLength(1)
    expect(face(container)?.getAttribute('src'),
      'the one holding the stage is the newest').toBe(
      'https://example.test/atarka.jpg')

    await act(async () => { await vi.advanceTimersByTimeAsync(200 + 5) })
    expect(cards(container), 'and the shoved one leaves').toHaveLength(1)
  } finally {
    vi.useRealTimers()
  }
})

it('plays the skull over the dying card, on the marks own clock', () => {
  // **One event, one clock.** The skull on the card in its row, the ghost
  // rising off the graveyard pile and this are the same death drawn in three
  // places, and two clocks for one event is precisely how they end up out of
  // step — which is the bug `MARK_LIFE` was written to fix, one layer down. So
  // the stage does not choose a second answer: it is handed the marks' own
  // number and renders it, and this reads what a browser would read.
  const { container } = replay(said({ kind: 'dies', card: 'Fleecemane Lion',
    key: 'd1' }))

  const card = container.querySelector('.stage-card') as HTMLElement | null
  expect(card?.className, 'the manner is on the card').toContain('is-dies')
  expect(card?.style.getPropertyValue('--stage-life'),
    'and its length is the mark\'s own 2000ms, not a second opinion')
    .toBe('2000ms')
  // The skull is *inside* the frame, so it travels down with the card when the
  // card sinks. A sibling would hang in the air over a card that had left,
  // which is three effects rather than one motion.
  expect(container.querySelector('.stage-frame .stage-skull'),
    'the stone falls onto the card, and goes down with it').toBeTruthy()
  // **And the plate does not name a player, even though the beat carries
  // one.** A creature dying is not something its controller did, and "Arahbo
  // dies" would name the wrong subject entirely — so `PLATE.dies` has no
  // third-person form at all and cannot grow one by accident.
  expect(container.querySelector('.stage-plate-word')?.textContent).toBe('Dies')
})

it('opens a vault under a dying card, without taking the stone off it', () => {
  // **The scene is under the card and the stone is on it**, and that pairing
  // is the decision this test exists to hold. A death now draws both, which
  // was the one real risk in adding the crypt — two objects saying "dead" over
  // one card. They survive together because they are saying different things
  // at different depths: the vault is *where the creature is going* and lives
  // outside the frame, the stone is *what happened to it* and lives inside the
  // frame so it travels down with the card. `components/stage.tsx` argues it
  // in full, including the two reasons the stone had to stay.
  //
  // jsdom has no layout engine, so what is asserted is the structure — which
  // is the half that carries the meaning anyway. Whether the chamber reads as
  // a chamber, and whether the stone is legible against it, is Aaron's walk.
  const { container } = replay(said({ kind: 'dies', card: 'Fleecemane Lion',
    key: 'v1' }))

  const crypt = container.querySelector('.stage-crypt')
  expect(crypt, 'a death opens onto somewhere').toBeTruthy()
  expect(container.querySelector('.stage-crypt .stage-crypt-art'),
    'and the somewhere is the photograph').toBeTruthy()
  // Outside the frame. Inside it, the vault would sink and shrink with the
  // card, which is a card carrying a room down a hole rather than going into
  // one.
  expect(container.querySelector('.stage-frame .stage-crypt'),
    'the vault is not something the card carries with it').toBeNull()
  // ...and the stone is still on the card, still inside the frame.
  expect(container.querySelector('.stage-frame .stage-skull'),
    'the stone did not give way to it').toBeTruthy()

  // **It belongs to the death alone.** A spell being cast is not going
  // anywhere, and an exile is going somewhere else entirely — down the road,
  // which is its own scene. Two scenes under one card would be the room
  // saying a card was buried and sent away at once.
  cleanup()
  const cast = replay(said({ card: 'Lightning Bolt', key: 'v2' }))
  expect(cast.container.querySelector('.stage-crypt'),
    'nothing opens under a spell being cast').toBeNull()
  cleanup()
  const gone = replay(said({ kind: 'exiled', card: 'Fleecemane Lion',
    key: 'v3' }))
  expect(gone.container.querySelector('.stage-crypt'),
    'and an exile takes the road, not the vault').toBeNull()
  expect(gone.container.querySelector('.stage-road'),
    'which it still has').toBeTruthy()
})

it('opens a battlefield under a creature nobody cast, and under nothing '
  + 'else', () => {
  // Aaron, 2026-08-27: *"Same thing for 'Enters the Battelfield', we should be
  // able to find something cool. A free use painting or picture of a battle
  // before us, like down in a valley...?"*
  //
  // The scene is a Brueghel battle panorama under a card that arrived without
  // being cast — an Atla Palani egg cracking into something enormous. Whether
  // it reads as a valley, whether the card looks like it *lands*, and whether
  // the dust is felt rather than watched are all Aaron's walk; jsdom has no
  // layout engine and would say yes to any of them. What is held here is which
  // beats get it, which is the half that has a wrong answer.
  const { container } = replay(said({ kind: 'enters', card: 'Fleecemane Lion',
    entered: 'put', key: 'p1' }))
  expect(container.querySelector('.stage-field'),
    'an uncast arrival opens onto the field it arrived on').toBeTruthy()
  expect(container.querySelector('.stage-field .stage-field-art'),
    'and the field is the painting').toBeTruthy()
  // Outside the frame, for the road's reason: an arrival happens to the card's
  // *place* rather than to the card, so the arena opens onto somewhere and the
  // card comes out of it. Inside the frame it would travel with the card,
  // which is a creature carrying a battlefield around.
  expect(container.querySelector('.stage-frame .stage-field'),
    'the field is not something the card brought with it').toBeNull()
  expect(container.querySelector('.stage-card')?.className).toContain('is-put')
  expect(container.querySelector('.stage-plate-word')?.textContent,
    'the plate names the player and the deed').toBe('Arahbo puts')
  expect(container.querySelector('.stage-plate-note')?.textContent,
    'and says the part that makes it worth a scene').toBe('nothing cast it')

  // **A match narrated before the scribe could answer draws nothing at all**,
  // and this is the assertion the whole feature turns on. Every game already
  // in the ledger sends an `enters` beat with no `entered` on it; if absence
  // read as "put" then every creature anybody ever cast, in every one of those
  // matches, would suddenly be shown arriving out of nowhere — the archive
  // rewritten as a room full of cheating.
  cleanup()
  const old = replay(said({ kind: 'enters', card: 'Fleecemane Lion',
    key: 'p2' }))
  expect(old.container.querySelector('.stage-field'),
    'nobody said how this one arrived, so nothing is drawn').toBeNull()
  expect(old.container.querySelector('.stage'),
    'and the beat takes no part of the arena at all').toBeNull()

  // And a creature somebody paid for keeps its own moment, one beat earlier,
  // rather than getting a second one here.
  cleanup()
  const paid = replay(said({ kind: 'enters', card: 'Fleecemane Lion',
    entered: 'cast', key: 'p3' }))
  expect(paid.container.querySelector('.stage'),
    'the room already showed this one being cast').toBeNull()

  // **It belongs to the arrival alone**, the way the vault belongs to the
  // death and the road to the exile. Three scenes under one card would be the
  // room saying three things happened.
  cleanup()
  const gone = replay(said({ kind: 'exiled', card: 'Fleecemane Lion',
    key: 'p4' }))
  expect(gone.container.querySelector('.stage-field'),
    'an exile leaves the field rather than arriving on it').toBeNull()
  cleanup()
  const dead = replay(said({ kind: 'dies', card: 'Fleecemane Lion',
    key: 'p5' }))
  expect(dead.container.querySelector('.stage-field'),
    'and a death goes into the vault').toBeNull()
})

it('draws the light going out of a dying card, and never filters it', () => {
  // **A licence rule, held by a test rather than by a memory.** The light used
  // to go out of a dying card through `filter: grayscale()` on `.stage-face` —
  // which is Wizards' painting, and Scryfall's imagery guidelines forbid
  // desaturating card imagery outright. ADR 32 had already written that
  // boundary into this repo; the stylesheet had simply drifted off it.
  //
  // So the layer is asserted *structurally*, because jsdom has no layout
  // engine and cannot see a shadow — what it can see, and what actually
  // matters, is that a separate element exists to carry the dark and that it
  // is a sibling of the face rather than something wrapped around it. A future
  // session that deletes the pall and reaches for a filter again fails here.
  const dying = replay(said({ kind: 'dies', card: 'Fleecemane Lion',
    key: 'd2' }))
  const pall = dying.container.querySelector('.stage-frame .stage-pall')
  expect(pall, 'the grave is drawn as a layer over the card').toBeTruthy()
  expect(pall?.previousElementSibling?.className,
    'and it lies over the painting rather than around it')
    .toContain('stage-face')
  cleanup()

  // ...and it belongs to the death, not to the stage. A spell being cast is
  // not going anywhere, so nothing is drawn over it at all.
  const cast = replay(said({ card: 'Lightning Bolt', key: 'd3' }))
  expect(cast.container.querySelector('.stage-pall'),
    'a card being cast is under no shadow').toBeNull()
})

it('hands the stylesheet the length a card is actually watched for', () => {
  // **Rendering a value audits it.** The animation's duration and the
  // element's lifetime are one number, and the only way they stay one number
  // is if the stylesheet is *told* rather than asked to remember.
  const read = (c: HTMLElement) =>
    (c.querySelector('.stage-card') as HTMLElement | null)
      ?.style.getPropertyValue('--stage-life')

  const watch = replay(said({ card: 'Lightning Bolt', key: 'w1' }))
  expect(read(watch.container)).toBe('1150ms')
  cleanup()

  // Fast is 150ms a beat, and the cap is what stops a Bolt filling the arena
  // while eight later beats go past behind it — four beats, floored so no pace
  // can cut the reveal down to a strobe.
  const skim = replay(said({ card: 'Lightning Bolt', key: 'w2' }),
    { speed: 'fast' })
  expect(read(skim.container)).toBe('620ms')
  cleanup()

  // Paused is not a slow pace, it is the absence of one: nothing is draining,
  // so there is nothing to cap a card against and it keeps its full length.
  const still = replay(said({ card: 'Lightning Bolt', key: 'w3' }),
    { speed: 'paused' })
  expect(read(still.container)).toBe('1150ms')
})

it('clears the stage at a game boundary rather than carrying it over', () => {
  // Choosing another game of the same match keeps this mounted, and a Bolt
  // held over from game one appearing over game two's opening is the room
  // lying about what it is showing.
  const { container, then } = replay(said({ card: 'Lightning Bolt', key: 'g1' }))
  expect(cards(container)).toHaveLength(1)

  then(said({ kind: 'life', card: undefined, key: 'g2', game: 2 }), { game: 2 })
  expect(cards(container), 'a new game starts with an empty stage')
    .toHaveLength(0)
})

it('expires a card while the transport is paused, like every other event', () => {
  // **This is the one place the stage parts company with the marks, on
  // purpose.** A mark is given back to its beat while paused, because a mark
  // is part of the board that beat describes and a scrub must land on the
  // board its beat describes. A cast is not part of any board — it is an
  // event, and an event happened and is over. If it were held while paused,
  // scrubbing onto a cast beat would park a card over the arena permanently,
  // which is exactly the thing this must never do.
  vi.useFakeTimers()
  try {
    const { container } = replay(said({ card: 'Lightning Bolt', key: 's1' }),
      { speed: 'paused' })
    expect(cards(container), 'the spell still plays for a scrubber')
      .toHaveLength(1)

    act(() => { vi.advanceTimersByTime(1150 + 90 + 5) })
    expect(cards(container), 'and it still gets out of the way afterwards')
      .toHaveLength(0)
  } finally {
    vi.useRealTimers()
  }
})

it('takes no pointer, so the timeline stays draggable underneath', () => {
  // Somebody dragging the scrubber through a match is dragging it through
  // sixty casts. `pointer-events: none` is the mechanism and jsdom cannot see
  // it; what it *can* see is that the overlay claims nothing — no role, no
  // label, nothing focusable — which is the same fact in the half of it a
  // suite with no stylesheet can hold. The words are in the play-by-play
  // beside it, which is the accessible channel for all of this.
  const { container } = replay(said({ card: 'Lightning Bolt', key: 'n1' }))
  const stage = container.querySelector('.stage')

  expect(stage?.getAttribute('aria-hidden')).toBe('true')
  expect(stage?.querySelectorAll('button, a, input, [tabindex]'))
    .toHaveLength(0)
})

it('gives the middle of the arena to what happened, and to nothing else', () => {
  // **A land is played, not cast**, and this is the one Magic judgement in the
  // file rather than a rendering one. A land does not use the stack, is not
  // cast, and is the most routine thing that happens in a game — eight or ten
  // a game, one almost every turn — so filling the arena with a Forest would
  // spend the effect on the beat that needs it least. `resolve` is left alone
  // from the other end: a spell cast and then resolving is *one* spell, and
  // drawing it twice a beat apart would read as two.
  //
  // **`enters` answers three ways rather than two**, and it cost three matches
  // and a decompiler to get there (Aaron, 2026-08-27, asked for a scene on
  // it). Nearly every arrival is either a token the board already marks or a
  // card the room showed as it was cast — which is `resolve` under another
  // name, refused just above. The exception is real but rare, and until the
  // beat could say *nothing cast this* the room could not tell one from the
  // other. It can now; `lib/stage.ts` above `mannerOf` carries the counts and
  // the Atla Palani game that produced four of them.
  expect(mannerOf('cast')).toBe('cast')
  expect(mannerOf('dies')).toBe('dies')
  expect(mannerOf('exiled')).toBe('exiled')
  expect(mannerOf('attach')).toBe('attach')
  expect(mannerOf('companion')).toBe('companion')
  for (const quiet of ['land', 'resolve', 'attack', 'block', 'turn', 'damage',
    'life', 'unblocked', 'mulligan', 'outcome', 'ability']) {
    expect(mannerOf(quiet), `${quiet} does not take the middle`).toBeNull()
  }

  // **The two that cannot answer from the kind alone**, and both are the same
  // rule: nothing is drawn twice.
  //
  // A permanent entering play was cast one beat earlier and drawn then — so
  // `enters` is silent for it and speaks only for a token, which nothing cast
  // and which has no other moment.
  const clue = { id: 14, name: 'Clue Token', token: true } as const
  const lion = { id: 10, name: 'Fleecemane Lion', types: 'Creature - Cat' }
  expect(mannerOf('enters', clue)).toBe('made')
  expect(mannerOf('enters', lion), 'this card had its moment when it was cast')
    .toBeNull()

  // **And the third answer: a real card put onto the battlefield.** An Atla
  // Palani egg cracking into a Blightsteel Colossus, a reanimation, a blink.
  // `entered` is Forge's own `wasCast`, carried word for word.
  expect(mannerOf('enters', lion, 'put')).toBe('put')
  expect(mannerOf('enters', lion, 'cast'), 'the room already showed somebody '
    + 'paying for this one').toBeNull()
  // The token wins over the word, because both are true of a token — Forge's
  // `wasCast` is false for everything it conjures — and only one of the two
  // scenes is right for it. Drawing a Clue on a battlefield instead of
  // drawing it being conjured is the wrong one.
  expect(mannerOf('enters', clue, 'put'), 'a token is conjured, not put')
    .toBe('made')

  // **The case that matters most, and the one this whole feature could have
  // got wrong silently.** Every match already in the ledger was narrated
  // before the scribe learned to ask, and every worker running an older image
  // still sends nothing. Reading an absent word as "put" would redraw the
  // entire archive as a room full of creatures appearing from nowhere — every
  // older game would read as cheating.
  expect(mannerOf('enters', lion, undefined), 'nobody said, so nothing is '
    + 'drawn').toBeNull()
  expect(mannerOf('enters', lion, ''), 'and an empty word is the same silence')
    .toBeNull()

  // And a creature sacrificed *also dies* — the scribe raises both, a beat
  // apart — so the death keeps rule 700.4's own list and the sacrifice takes
  // everything else: the Food, the Treasure, the cracked fetchland.
  expect(mannerOf('sacrificed', { id: 15, name: 'Food Token',
    types: 'Artifact - Food', token: true })).toBe('sacrificed')
  expect(mannerOf('sacrificed', lion), 'a creature sacrificed is drawn dying, '
    + 'once').toBeNull()
  expect(mannerOf('sacrificed', { id: 99, name: 'Nameless' }),
    'and a card whose type line nobody recorded is left alone rather than '
    + 'guessed at').toBeNull()

  // And through the board, because a table of kinds is only a claim about what
  // the room does with it.
  const { container } = replay(said({ kind: 'land', card: 'Forest', key: 'f1' }))
  expect(container.querySelector('.stage'), 'a land drop is not a spell')
    .toBeNull()
})

it('draws several tokens as one pile with a number on it', () => {
  // Aaron, 2026-08-27: *"Token creation should show in the center stage, with
  // how many tokens were created represented by one of our stacked cards x's,
  // like lands."*
  //
  // **The count is counted, never guessed.** Forge announces a token by moving
  // a card into a zone, one card at a time, and there is no `amount` anywhere
  // on that path — so three Clue Tokens reach the browser as three identical
  // beats in a row and `countRuns` in `lib/reel.ts` turns that back into one
  // moment with a three on it. This drives the *rendering* half of that: a
  // beat carrying `run: 3` is one card, one fan and one tally.
  const { container, then } = replay(said({ kind: 'enters', card: 'Clue Token',
    key: 't1', run: 3 }))

  expect(container.querySelector('.stage-plate-word')?.textContent,
    'a token was conjured, not cast, and the plate says who by')
    .toBe('Arahbo makes Artifact')
  expect(container.querySelector('.stage-count')?.textContent,
    'the board\'s own tally, in the board\'s own words').toBe('3×')
  expect(container.querySelectorAll('.stage-leaf'),
    'two leaves below four, which is the board\'s cap and its reason')
    .toHaveLength(2)

  // **And the beats that follow take nothing**, which is what makes it one
  // moment rather than three. `countRuns` marks a run's followers `0`, and a
  // beat with nothing to show leaves the card that is up alone — so the pile
  // holds for its own life instead of being replayed under two more keys.
  then(said({ kind: 'enters', card: 'Clue Token', key: 't2', run: 0 }))
  expect(cards(container), 'the second of a run does not raise a second card')
    .toHaveLength(1)
  expect(container.querySelector('.stage-count')?.textContent).toBe('3×')

  // Four or more thickens the fan rather than widening it — more edges in the
  // same sweep, which is what a thicker pile looks like under a thumb.
  cleanup()
  const many = replay(said({ kind: 'enters', card: 'Food Token', key: 't3',
    run: 6 }))
  expect(many.container.querySelectorAll('.stage-leaf')).toHaveLength(4)
  expect(many.container.querySelector('.stage-count')?.textContent).toBe('6×')

  // A single token is a single card: no fan, no tally, nothing to count.
  cleanup()
  const one = replay(said({ kind: 'enters', card: 'Clue Token', key: 't4' }))
  expect(one.container.querySelector('.stage-pile')).toBeNull()
  expect(one.container.querySelector('.stage-count')).toBeNull()
})

it('shows a token going to the ether, and never draws one death twice', () => {
  // Aaron, 2026-08-27: *"We also should show token deaths like any other in
  // the center stage before they go to the ether."* The ether is where a Food,
  // a Clue and a Treasure go — they are artifacts, so rule 700.4 does not let
  // them *die*, and `dies` was correctly silent about the commonest
  // disappearance in a Gyome game.
  const { container } = replay(said({ kind: 'sacrificed', card: 'Food Token',
    key: 'e1', run: 2 }))
  expect(container.querySelector('.stage-plate-word')?.textContent)
    .toBe('Arahbo sacrifices')
  expect(container.querySelector('.stage-count')?.textContent,
    'two Foods cracked at once is one moment with a two on it').toBe('2×')
  cleanup()

  // **A creature sacrificed raises both beats, one after the other, and only
  // one of them is drawn.** Drawing both would shove a card off the stage to
  // put the same card straight back — the doubling this file refuses for
  // `resolve`, arriving by another door.
  const both = replay(said({ kind: 'sacrificed', card: 'Fleecemane Lion',
    key: 'e2' }))
  expect(both.container.querySelector('.stage'),
    'the death is the moment; the cost paid is not a second one').toBeNull()
})

it('names the host when a card goes onto another one', () => {
  // The one beat on this wire that carries a target. A cast does not: Forge
  // announces the spell and the attachment as two separate moments, so the
  // sword being cast and the sword being *worn* are two different sentences
  // and only the second one knows who is wearing it.
  const { container } = replay(said({ kind: 'attach',
    card: 'Bloodforged Battle-Axe', target: 'Fleecemane Lion', key: 'a1' }))

  expect(container.querySelector('.stage-plate-word')?.textContent)
    .toBe('Arahbo puts')
  expect(container.querySelector('.stage-plate-title')?.textContent)
    .toBe('Bloodforged Battle-Axe')
  expect(container.querySelector('.stage-plate-note')?.textContent)
    .toBe('on Fleecemane Lion')
})

it('walks a companion in from outside the game, and says what it cost', () => {
  // **The room used to draw a card appearing in a hand and say nothing at
  // all**, which is a beginner being shown a game that cheats — Aaron watched
  // exactly that and did not believe it (*"I swear Kaheera was dealt in a
  // hand? That should not be possible"*). He was right about the rules and
  // Forge was right about the game: a companion waits outside the game and its
  // controller pays {3} to bring it into their hand.
  const { container } = replay(said({ kind: 'companion',
    card: 'Kaheera, the Orphanguard', key: 'c1' }))

  const card = container.querySelector('.stage-card')
  expect(card?.className, 'the manner is on the card').toContain('is-companion')
  expect(container.querySelector('.stage-plate-word')?.textContent)
    .toBe("Arahbo's companion")
  expect(container.querySelector('.stage-plate-note')?.textContent)
    .toBe('from outside the game — three mana paid')

  // **The road, and it is the exile's road run backwards.** Magic keeps one
  // elsewhere — the place outside the game — and a second picture for it would
  // say there were two. The structure is what jsdom can hold: the same
  // element, under the card rather than over it, so the arena opens onto
  // somewhere else and the card walks up out of it.
  const road = container.querySelector('.stage-road')
  expect(road, 'a companion comes in from somewhere').toBeTruthy()
  expect(road?.nextElementSibling?.className,
    'and the road is behind the card, never over it').toContain('stage-frame')

  // It is long enough to read a rule rather than a name — the one plate in the
  // room carrying one — and still inside the four-beat cap, so watching pace
  // never clips it.
  expect((card as HTMLElement).style.getPropertyValue('--stage-life'))
    .toBe('1800ms')
})

it('finds a face through a spelling Forge does not use', () => {
  // Forge names a **face**; the board's names come from the scribe and can
  // carry Scryfall's combined `A // B`. A transforming card would otherwise be
  // cast sixty times a match and found not once — silently, and only in the
  // decks that play them. One matcher does this for the marks and for the
  // stage, which is why `sameCard` is shared rather than written twice.
  const twofaced = {
    ...MATCH,
    cards: [{ id: 40, name: 'Delver of Secrets // Insectile Aberration',
      types: 'Creature - Human Wizard', seat: 1,
      image: 'https://example.test/delver.jpg' }],
  } as unknown as ForgeBoard
  expect(faceFor(twofaced, 'Delver of Secrets', 1)?.image)
    .toBe('https://example.test/delver.jpg')

  // A name nothing in the match answers to is a null, and a null is the plate.
  expect(faceFor(MATCH, 'Black Lotus', 1)).toBeNull()
  expect(faceFor(null, 'Lightning Bolt', 1)).toBeNull()

  // Two seats can each run one card, and in a singleton format that is the
  // only way one name is two cards. Nearly cosmetic — same printing, same
  // picture — but the copy shown is the copy that was cast.
  const both = {
    ...MATCH,
    cards: [
      { id: 50, name: 'Swords to Plowshares', seat: 1,
        image: 'https://example.test/swords-one.jpg' },
      { id: 51, name: 'Swords to Plowshares', seat: 2,
        image: 'https://example.test/swords-two.jpg' },
    ],
  } as unknown as ForgeBoard
  expect(faceFor(both, 'Swords to Plowshares', 2)?.id).toBe(51)
  expect(faceFor(both, 'Swords to Plowshares', null)?.id).toBe(50)
})

it('never cuts a reveal down to a flicker, at any pace', () => {
  // The floor and the cap in one reading. Half a fade is worse than nothing —
  // it draws the eye to a thing that is already gone — and a card that outlasts
  // a dozen beats is the failure Aaron named in reverse (*"it shouldn't linger
  // for too long"*).
  for (const speed of ['paused', 'study', 'play', 'fast'] as Speed[]) {
    const life = stageLife('cast', speed, null)
    expect(life, `${speed} is watchable`).toBeGreaterThanOrEqual(620)
    expect(life, `${speed} does not overstay`).toBeLessThanOrEqual(1150)
  }
  // **A death is the marks' number and nothing else**, and this assertion is
  // here because the first version of it was not. The stage capped at four
  // beats and the marks cap at five, so at watching pace a 2000ms mark met a
  // 1920ms stage life — the stylesheet still running on `--mark-life-dies`
  // while the element was pulled out from under it, and a card popping off
  // eighty milliseconds before its own last frame. Being careful twice
  // reintroduced exactly the drift that one-number-for-both exists to prevent.
  expect(stageLife('dies', 'play', 2000)).toBe(2000)
  expect(stageLife('dies', 'fast', 750), 'the mark already answered for pace')
    .toBe(750)
  // And it falls back to a real length rather than a plausible default when
  // nobody hands it one.
  expect(stageLife('dies', 'paused', null)).toBe(2000)

  // **The arrival is inside the four-beat cap at watching pace**, which is the
  // property rather than the number: 480ms a beat gives 1920, so anything at
  // or under that is never clipped by the transport and only the fast end
  // shortens it. It is the longest thing a *player* does short of buying in a
  // companion, and it is the rarest.
  expect(stageLife('put', 'play', null), 'watching pace never clips it')
    .toBe(1600)
  expect(stageLife('put', 'fast', null), 'and a fast pace still shortens it '
    + 'like anything else').toBeLessThan(1600)
})

it('names the kind of card on the plate, in the word a player would use', () => {
  // Aaron, 2026-08-27: *"I would also like to display the type, like 'CAST
  // CREATURE' or 'CAST SORCERY'."* The reason it earns its place is
  // commandment 2: half of what crosses this stage never reaches the
  // battlefield, so the picture is all a newcomer gets, and the type is the
  // difference between *a card happened* and *a spell resolved and is gone*.

  // **The priority is the point, not the lookup.** Most cards carry more than
  // one type and only one of them is what a player calls the card.
  expect(castType('Creature - Cat Warrior')).toBe('Creature')
  expect(castType('Artifact Creature - Golem'), 'an Artifact Creature is a '
    + 'creature: it is what attacks, and what anybody at the table calls it')
    .toBe('Creature')
  expect(castType('Legendary Enchantment Creature - God')).toBe('Creature')
  expect(castType('Legendary Artifact - Equipment')).toBe('Artifact')
  expect(castType('Enchantment - Aura')).toBe('Enchantment')
  expect(castType('Instant')).toBe('Instant')
  expect(castType('Kindred Sorcery - Elf'), 'Kindred never stands alone, so '
    + 'reading it would only ever hide the type beside it').toBe('Sorcery')
  expect(castType('Legendary Planeswalker - Teferi')).toBe('Planeswalker')

  // Scryfall's long dash and Forge's hyphen are both cut, because both reach
  // this wire — and everything past either one is a subtype, which is the
  // card's own name said a second time.
  expect(castType('Creature — Human Wizard')).toBe('Creature')

  // Absent is a real answer: a match that never described the card still gets
  // a plate, it just gets the shorter one.
  expect(castType(undefined)).toBeNull()
  expect(castType('')).toBeNull()
  expect(castType('Attraction')).toBeNull()

  // **Only a cast and a token take the type**, and the asymmetry is
  // deliberate. "Casts Creature" answers a question somebody watching actually
  // has; a token gets it because a Servo Token is a creature and a Food Token
  // is not, and the name alone does not say which. "Dies Creature" answers
  // none — rule 700.4 gives that word to creatures and planeswalkers, so the
  // noun is already nearly settled and the picture on the stage is the rest of
  // it. Exile sits with the death: it is drawn on the way out, and where the
  // card has gone matters more than what it was.
  expect(plateWord('cast', 'Creature - Cat Warrior')).toBe('Cast Creature')
  expect(plateWord('cast', 'Instant')).toBe('Cast Instant')
  expect(plateWord('cast', undefined)).toBe('Cast')
  expect(plateWord('made', 'Artifact - Food')).toBe('Made Artifact')
  expect(plateWord('dies', 'Creature - Cat Warrior')).toBe('Dies')
  expect(plateWord('exiled', 'Creature - Cat Warrior')).toBe('Exiled')
  expect(plateWord('attach', 'Legendary Artifact - Equipment')).toBe('Put on')
  // An uncast arrival takes none either, and it is the closest call of the
  // six: the type says whether to expect a card again, and this one has
  // landed — it is standing in a row on the board a beat later, where its own
  // lane says what it is better than a word could.
  expect(plateWord('put', 'Creature - Phyrexian Golem'))
    .toBe('Put onto the battlefield')
})

it('writes the plate as a sentence about a player doing something', () => {
  // Aaron, 2026-08-27: *"It would be nice if we added the players name too,
  // Gyome CASTS Creature, etc."* — so a deed somebody did is written the way
  // they did it, third person, name first.
  expect(plateWord('cast', 'Creature - Cat Warrior', 'Gyome'))
    .toBe('Gyome casts Creature')
  expect(plateWord('made', 'Artifact - Clue', 'Gyome')).toBe('Gyome makes Artifact')
  expect(plateWord('sacrificed', 'Artifact - Food', 'Gyome'))
    .toBe('Gyome sacrifices')
  expect(plateWord('attach', 'Enchantment - Aura', 'Gyome')).toBe('Gyome puts')

  // **"Puts" twice, and it is one English verb doing one job in two places.**
  // A sword is *put on* a creature and a Colossus is *put onto the
  // battlefield*; Magic uses the same word in both rules sentences, and the
  // note line under each says which. Inventing a second verb here would teach
  // a distinction the game does not draw.
  expect(plateWord('put', 'Creature - Phyrexian Golem', 'Gyome'))
    .toBe('Gyome puts')

  // **The companion wears the mechanic's own name and not a smoother verb.**
  // The whole reason that beat exists is that Aaron watched Kaheera arrive in
  // a hand and thought the game had cheated; the word "companion" on screen is
  // what makes it a thing a person can look up (commandment 2), which is the
  // same argument `beatLine` makes for keeping "exiled".
  expect(plateWord('companion', 'Legendary Creature - Cat Beast', 'Gyome'))
    .toBe("Gyome's companion")

  // **A death and an exile refuse a player even when one is offered**, which
  // is the property rather than the string: a creature dying is not something
  // its controller chose, `beatLine` returns no player for exactly that
  // reason, and a plate reading "Gyome dies" would name the wrong subject.
  expect(plateWord('dies', 'Creature - Cat', 'Gyome')).toBe('Dies')
  expect(plateWord('exiled', 'Creature - Cat', 'Gyome')).toBe('Exiled')

  // And the nameless form is a real state, not dead code: the room turns a
  // slug into a name off the shelf, and a finished match reopened before the
  // shelf answers has beats nobody can name a player for yet.
  expect(plateWord('cast', 'Instant', null)).toBe('Cast Instant')
  expect(plateWord('made', undefined, '')).toBe('Made')
})

it('names the target under the card, and the rule under a companion', () => {
  // **The target is the half the picture cannot show.** An Aura on the stack
  // is a card; an Aura on a creature is a different creature (Aaron,
  // 2026-08-27: *"for an aura or something that targets something if the text
  // box called out the target too"*). It is the attach beat that carries it —
  // a cast does not, because Forge announces the spell and the attachment as
  // two separate moments.
  expect(plateNote('attach', 'Syr Gwyn, Hero of Ashvale'))
    .toBe('on Syr Gwyn, Hero of Ashvale')
  expect(plateNote('attach', undefined), 'a curse names a player and finds no '
    + 'host, and the line still has to read').toBe('on a permanent')

  // **The three is the rulebook speaking, not a number read off the board.**
  // Rule 702.139b fixes it for every companion there has ever been; the wire
  // attributes no mana to the ability at all, and an amount inferred from
  // whichever lands happened to tap is exactly what ADR 44 forbids.
  expect(plateNote('companion', undefined))
    .toBe('from outside the game — three mana paid')

  // **And the one fact nothing else on the screen carries.** A creature
  // standing on the battlefield that nobody was seen to pay for looks like
  // somebody skipped a step — the companion's own failure, a beat earlier in a
  // newcomer's evening. Deliberately *"nothing cast it"* rather than "no mana
  // was paid": mana may well have been spent, on the Egg or the reanimation
  // spell, and the room does not know. What it knows is Forge's own boolean.
  expect(plateNote('put', 'Fleecemane Lion'), 'and it takes no target, '
    + 'because there is nothing on the other end of an arrival')
    .toBe('nothing cast it')

  // Everything else says nothing. "Gyome sacrifices Food Token, into the
  // graveyard" spends a line on what the graveyard pile is already drawing.
  for (const quiet of ['cast', 'made', 'dies', 'exiled', 'sacrificed'] as const) {
    expect(plateNote(quiet, 'Fleecemane Lion'), `${quiet} is a whole sentence`)
      .toBeNull()
  }
})
