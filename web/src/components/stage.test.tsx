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
import { castType, faceFor, mannerOf, plateWord, stageLife }
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
    { id: 20, name: 'Dragonlord Atarka', types: 'Creature - Dragon', seat: 2,
      image: 'https://example.test/atarka.jpg' },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [{ id: 11, zone: 'land', seat: 1 }] },
    { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
    { turn: 2, seat: 1, changes: [{ id: 12, zone: 'graveyard', seat: 1 }] },
  ],
} as unknown as ForgeBoard

function said(over: Partial<StagedBeat> & { key: string }): StagedBeat {
  return {
    game: 1, turn: 2, kind: 'cast', who: 'Arahbo — Cats', text: '', ...over,
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
  // **And the plate names the kind**, which is the whole of why the type is
  // worth drawing: this card is on no battlefield and never will be, so the
  // word "Instant" is the only thing telling somebody at their first game that
  // it was never going to stay. The type comes off the match's own card list —
  // the same lookup that found the picture, asked once.
  expect(container.querySelector('.stage-plate-word')?.textContent)
    .toBe('Cast Instant')
  expect(container.querySelector('.stage-plate-title')?.textContent)
    .toBe('Lightning Bolt')
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
  expect(container.querySelector('.stage-plate-word')?.textContent).toBe('Dies')
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

it('gives the middle of the arena to casts, deaths and exiles, and to nothing else', () => {
  // **A land is played, not cast**, and this is the one Magic judgement in the
  // file rather than a rendering one. A land does not use the stack, is not
  // cast, and is the most routine thing that happens in a game — eight or ten
  // a game, one almost every turn — so filling the arena with a Forest would
  // spend the effect on the beat that needs it least. `resolve` is left alone
  // from the other end: a spell cast and then resolving is *one* spell, and
  // drawing it twice a beat apart would read as two.
  expect(mannerOf('cast')).toBe('cast')
  expect(mannerOf('dies')).toBe('dies')
  expect(mannerOf('exiled')).toBe('exiled')
  for (const quiet of ['land', 'resolve', 'attack', 'block', 'turn', 'damage',
    'life', 'enters', 'attach', 'unblocked', 'mulligan', 'outcome']) {
    expect(mannerOf(quiet), `${quiet} does not take the middle`).toBeNull()
  }

  // And through the board, because a table of kinds is only a claim about what
  // the room does with it.
  const { container } = replay(said({ kind: 'land', card: 'Forest', key: 'f1' }))
  expect(container.querySelector('.stage'), 'a land drop is not a spell')
    .toBeNull()
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

  // **Only a cast takes the type**, and the asymmetry is deliberate. "Cast
  // Creature" answers a question somebody watching actually has. "Dies
  // Creature" answers none — rule 700.4 gives that word to creatures and
  // planeswalkers, so the noun is already nearly settled and the picture on
  // the stage is the rest of it. Exile sits with the death: it is drawn on the
  // way out, and where the card has gone matters more than what it was.
  expect(plateWord('cast', 'Creature - Cat Warrior')).toBe('Cast Creature')
  expect(plateWord('cast', 'Instant')).toBe('Cast Instant')
  expect(plateWord('cast', undefined)).toBe('Cast')
  expect(plateWord('dies', 'Creature - Cat Warrior')).toBe('Dies')
  expect(plateWord('exiled', 'Creature - Cat Warrior')).toBe('Exiled')
})
