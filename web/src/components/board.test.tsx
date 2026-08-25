/**
 * The board's two hands, and the card a phone can finally read.
 *
 * Both properties here are about *where a thing is drawn*, which is exactly
 * what a green suite normally cannot see — so they are held as structure
 * rather than as pixels.
 *
 * A hand is not on the battlefield. It used to be drawn as though it were: a
 * full-width row in the same stack as lands and creatures, one per seat, so
 * two players cost eight rows and two of them were cards nobody had played
 * yet. The field is for what has been committed to it.
 *
 * And the card preview was `:hover` and `:focus-visible`, which a phone is
 * neither — forty cards on a floor, none readable at forty pixels, and the one
 * mechanism that made them readable needed a pointer nobody on a touch screen
 * has.
 */

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import type { ForgeBoard } from '../lib/api'
import type { StagedBeat } from '../lib/reel'
import { MatchBoard } from './board'

afterEach(cleanup)

const BOARD: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 10, name: 'Fleecemane Lion', types: 'Creature - Cat', seat: 1,
      image: 'https://example.test/lion.jpg' },
    { id: 11, name: 'Forest', types: 'Basic Land - Forest', seat: 1 },
    { id: 12, name: 'Sacred Foundry', types: 'Land', seat: 1,
      image: 'https://example.test/foundry.jpg' },
    { id: 20, name: 'Dragonlord Atarka', types: 'Legendary Creature', seat: 2 },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [{ id: 11, zone: 'land', seat: 1 }] },
    { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
    { turn: 2, seat: 1, changes: [{ id: 12, zone: 'hand', seat: 1 }] },
  ],
} as unknown as ForgeBoard

function show(shown = 3) {
  return render(
    <MatchBoard board={BOARD} shown={shown} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={3} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

/** A tap, the way a phone makes one: jsdom has no PointerEvent, and the
 *  handler only ever reads the type. */
function tap(el: Element) {
  const ev = new Event('pointerup', { bubbles: true, cancelable: true })
  Object.defineProperty(ev, 'pointerType', { value: 'touch' })
  fireEvent(el, ev)
}

it('gives each hand its own place, on its own side of the seam', () => {
  const { container } = show()

  // **The property is order, not existence.** Both hands used to share one
  // rail, which is right on a wide screen and wrong on a narrow one: there the
  // rail becomes a strip at the foot and the far player's hand ends up stacked
  // under the near player's, two seats from whoever is holding it. Held as
  // document order because that is what survives both breakpoints — the grid
  // places these two areas differently at each width, and the DOM order is the
  // thing neither branch can quietly get wrong.
  const order = [...container.querySelectorAll(
    '.field-hand-far, .field-side-far, .field-side-near, .field-hand-near')]
    .map((el) => ['field-hand-far', 'field-side-far', 'field-side-near',
      'field-hand-near'].find((c) => el.classList.contains(c)))
  expect(order).toEqual([
    'field-hand-far', 'field-side-far', 'field-side-near', 'field-hand-near',
  ])
  expect(container.querySelector('.field-hands'),
    'there is no shared rail left to fall back to').toBeNull()

  // And the sand still carries neither. A seat's rows are lands, the
  // artifacts-and-enchantments row and creatures — three, not four.
  for (const side of container.querySelectorAll('.field-side')) {
    const labels = [...side.querySelectorAll('.field-row')]
      .map((r) => r.getAttribute('aria-label') ?? '')
    expect(labels.some((l) => l.startsWith('Hand')),
      'a hand is not a row on the battlefield').toBe(false)
    expect(labels).toHaveLength(3)
  }

  // The card that went to hand is drawn in its owner's hand, and counted.
  const far = container.querySelector('.field-hand-far') as HTMLElement
  expect(within(far).getByLabelText('Hand: 1')).toBeTruthy()
})

/** A board where one commander is cast, dies back home, and is cast again —
 *  and a creature carries counters of both signs at once. */
const THRONE: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 30, name: 'Arahbo, Roar of the World',
      types: 'Legendary Creature - Cat', seat: 1 },
    { id: 31, name: 'Fleecemane Lion', types: 'Creature - Cat', seat: 1 },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [{ id: 30, zone: 'command', seat: 1 }] },
    { turn: 2, seat: 1, changes: [{ id: 30, zone: 'battlefield', seat: 1 }] },
    { turn: 3, seat: 1, changes: [{ id: 30, zone: 'command', seat: 1 }] },
    { turn: 4, seat: 1, changes: [{ id: 30, zone: 'battlefield', seat: 1 }] },
    { turn: 5, seat: 1, changes: [{ id: 31, zone: 'battlefield', seat: 1,
      power: 3, toughness: 3,
      // `n` is *how many*, and the sign lives on the kind: one -1/-1 counter
      // is `{kind: '-1/-1', n: 1}`. Reading the sign off `n` painted it a
      // cheerful green `+1`, which is what this fixture exists to catch.
      counters: [{ kind: '+1/+1', n: 3 }, { kind: '-1/-1', n: 1 },
        { kind: 'stun', n: 2 }] }] },
  ],
} as unknown as ForgeBoard

function throne(shown: number) {
  return render(
    <MatchBoard board={THRONE} shown={shown} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={5} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

it('seats an empty throne when the commander is out, and prices the return', () => {
  // Home after one cast: a card in the zone, and two generic on the next one.
  // Seat one's own rail: seat two never gets a commander in this fixture, so
  // its zone is legitimately an empty chair the whole way through and would
  // answer either question the wrong way.
  const railOf = (c: HTMLElement) =>
    c.querySelector('.field-side-far .field-rail') as HTMLElement

  const home = throne(3)
  expect(railOf(home.container).querySelector('.field-pile.is-throne'),
    'a zone with the commander in it is not an empty chair').toBeNull()
  expect(railOf(home.container).querySelector('.field-tax')?.textContent?.trim())
    .toBe('+2')
  cleanup()

  // Out again: the chair is empty and the price has gone up.
  const away = throne(4)
  expect(railOf(away.container).querySelectorAll('.field-pile.is-throne'),
    'the commander is on the battlefield, so the seat is empty').toHaveLength(1)
  expect(railOf(away.container).querySelector('.field-tax')?.textContent?.trim())
    .toBe('+4')
})

it('draws counters as their own signed chips rather than one sum', () => {
  const { container } = throne(5)
  const chips = [...container.querySelectorAll('.field-counter')]
  // Three +1/+1 and one -1/-1 is not "2". They are two kinds of counter that
  // have not annihilated yet, and the board must not do that arithmetic for
  // anybody.
  expect(chips.map((c) => c.textContent?.trim())).toEqual(['+3', '-1', '2'])
  expect(chips[0]?.className).toContain('is-up')
  expect(chips[1]?.className).toContain('is-down')
  // A stun counter is neither, and the board does not get to call it good.
  expect(chips[2]?.className).toContain('is-flat')
  // The sign is spelled out as well as coloured, for anyone who does not
  // separate those two hues.
  expect(chips[1]?.textContent?.trim().startsWith('-')).toBe(true)
})

it('puts power and toughness behind glass rather than over the painting', () => {
  const { container } = throne(5)
  expect(container.querySelector('.field-card-stats'),
    'the always-on black tab is gone').toBeNull()
  const lens = container.querySelector('.field-card-lens')
  expect(lens, 'the creature has a loupe').toBeTruthy()
  // Forge reports the figures a creature is *currently* fighting at, so the
  // glass carries them straight: the counters beside it say why, and the view
  // does not re-add them on top.
  expect(lens?.querySelector('.field-card-lens-pt')?.textContent?.trim())
    .toBe('3/3')

  // **It is there before anybody asks.** The glass replaced an always-on
  // black tab and then hid until hovered, which traded the tab's fault for its
  // opposite: a board of forty creatures carrying no numbers at all, learnable
  // only one hover at a time (Aaron, 2026-08-25: "what I meant is that it
  // always appeared"). Nothing was touched above this line.
  expect(lens?.className).not.toContain('is-open')

  // And it hangs on the arm, which is what carries it round to the card's own
  // stat corner when the card turns. The property is the parentage: a loupe
  // pinned to the slot instead of the card sits on the neighbouring permanent
  // the moment a card leans, and no rendered angle in jsdom would say so.
  expect(lens?.closest('.field-card-arm'),
    'the loupe rides the card, not the slot').toBeTruthy()
})

it('hands the whole card to a tap, because a phone cannot hover a peek', () => {
  const { container } = show()
  expect(screen.queryByRole('dialog')).toBeNull()

  // A card the pool gave a painting to — the first `.field-card` on the floor
  // may well be a Forest the pool had no image for, and that one is meant to
  // do nothing.
  const card = container.querySelector('img[alt="Fleecemane Lion"]')
    ?.closest('.field-card')
  expect(card).toBeTruthy()
  tap(card as Element)

  expect(screen.getByRole('dialog', { name: 'Fleecemane Lion' })).toBeTruthy()
})

it('leaves a card with no painting alone rather than opening an empty sheet', () => {
  const { container } = show()
  // Atla's Dragonlord never reaches the field in these steps; the Forest does,
  // and the pool gave it no image.
  const plate = container.querySelector('.field-card-plate')?.closest('.field-card')
  expect(plate, 'the no-art card is drawn as a legible plate').toBeTruthy()

  tap(plate as Element)
  expect(screen.queryByRole('dialog')).toBeNull()
})

/** A board with something in every closed zone, so each one has a tray to
 *  open. */
const ZONES: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 40, name: 'Fleecemane Lion', types: 'Creature - Cat', seat: 1 },
    { id: 41, name: 'Qasali Pridemage', types: 'Creature - Cat', seat: 1 },
    { id: 42, name: 'Swords to Plowshares', types: 'Instant', seat: 1 },
    { id: 43, name: 'Regal Caracal', types: 'Creature - Cat', seat: 1 },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [
      { id: 40, zone: 'graveyard', seat: 1 },
      { id: 41, zone: 'graveyard', seat: 1 },
      { id: 42, zone: 'exile', seat: 1 },
      { id: 43, zone: 'hand', seat: 1 },
    ] },
  ],
} as unknown as ForgeBoard

it('opens a closed zone into something you can look through', () => {
  const { container } = render(
    <MatchBoard board={ZONES} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)

  // **A graveyard is public information in every format**, and it was drawn
  // as a number with a picture on it. Every card in it is in the tray, not
  // just the one on top.
  const grave = container.querySelector('.field-side-far .field-pile-wrap:has('
    + '[aria-label^="Graveyard"]) .field-tray') as HTMLElement
  expect(grave, 'the graveyard has a tray').toBeTruthy()
  expect(grave.getAttribute('aria-label')).toBe('Graveyard, all 2')
  expect(grave.querySelectorAll('.field-card')).toHaveLength(2)

  // An empty zone gets no tray at all rather than an empty one: there is
  // nothing to look through, and a panel that opens onto nothing is a worse
  // answer than a pile that stays shut.
  const exileWrap = container.querySelector(
    '.field-side-near .field-pile-wrap:has([aria-label^="Exile"])')
  expect(exileWrap?.querySelector('.field-tray'),
    "the near seat's exile is empty, so it does not open").toBeNull()
})

it('keeps the loupe on the battlefield, where a fight is the question', () => {
  const { container } = show()
  // A hand is a fan overlapped to the 27px strip carrying each card's name, so
  // a card's bottom-right corner — where the loupe goes — is *under the next
  // card*. Drawn there, a creature's numbers land on a different card's
  // painting, belonging to one nobody can see. Found by measuring a live board
  // rather than by reading this file.
  const fan = container.querySelector('.field-hand-far .field-hand-fan')
  expect(fan?.querySelectorAll('.field-card').length,
    'the far seat is holding something').toBeGreaterThan(0)
  expect(fan?.querySelectorAll('.field-card-lens'),
    'nothing in a hand is fighting at anything').toHaveLength(0)

  // And the reason is not the overlap alone: power and toughness on this board
  // are what a creature is fighting at *now*, counters and anthems included.
  // That is a battlefield question, and the rows still ask it — the fixture
  // that gives a creature real figures is `throne`, which is why the positive
  // half of this property is checked against that board rather than this one.
  cleanup()
  expect(throne(5).container
    .querySelectorAll('.field-rows .field-card-lens').length)
    .toBeGreaterThan(0)
})

/** The viewport the placement tests are argued against, and one card's place
 *  in it. jsdom measures everything as zero, so a test about *where a thing
 *  is put* has to say where everything was — which is no worse than the truth:
 *  this geometry is only ever as good as the numbers it is handed. */
function viewport(w: number, h: number) {
  for (const [prop, value] of [['clientWidth', w], ['clientHeight', h]] as const) {
    Object.defineProperty(document.documentElement, prop,
      { value, configurable: true })
  }
}
function standing(el: Element, at: Partial<DOMRect>) {
  const box = { x: 0, y: 0, top: 0, left: 0, right: 0, bottom: 0,
    width: 0, height: 0, ...at }
  el.getBoundingClientRect = () => ({ ...box, toJSON: () => box }) as DOMRect
}

it('holds a card up clear of the wall it was standing against', () => {
  viewport(1200, 800)
  const { container } = show()
  // The one card the fixture gave a painting to, standing hard against the
  // left edge of the field — which is where the fault was reported (Aaron,
  // 2026-08-25: "they get clipped by the black border").
  const card = [...container.querySelectorAll('.field-card')]
    .find((c) => c.querySelector('.field-card-art')) as Element
  standing(card, { left: 4, right: 62, width: 58,
    top: 300, bottom: 381, height: 81 })
  fireEvent.mouseEnter(card)

  const peek = document.querySelector('.field-peek') as HTMLElement
  expect(peek, 'hovering a card holds it up').toBeTruthy()
  // **Out of the field entirely.** The preview used to be a child of the card,
  // so it inherited the field's `overflow: hidden` and was cut off at the wall.
  // Nothing about widening it or moving it would have survived that parentage.
  expect(peek.closest('.field'), 'drawn in the body, not in the row').toBeNull()
  expect(peek.parentElement).toBe(document.body)
  // Centred on a card 4px from the edge, a 300px preview starts at -117.
  expect(Number.parseFloat(peek.style.left)).toBeGreaterThanOrEqual(8)
  expect(Number.parseFloat(peek.style.left) + Number.parseFloat(peek.style.width))
    .toBeLessThanOrEqual(1200 - 8)

  fireEvent.mouseLeave(card)
  expect(document.querySelector('.field-peek'),
    'and exactly one of them exists at a time').toBeNull()
})

it('steps a preview beside the pile it was opened from, never onto it', () => {
  viewport(1200, 800)
  const { container } = show()
  // The far seat's hand, spread — the panel somebody opened *in order to look
  // at it*, and the one a 300px card was landing in the middle of.
  fireEvent.click(container.querySelector(
    '.field-hand-far .field-hand-label') as Element)
  const tray = container.querySelector(
    '.field-hand-far .field-hand-tray') as Element
  standing(tray, { left: 300, right: 700, width: 400,
    top: 100, bottom: 360, height: 260 })
  const card = tray.querySelector('.field-card') as Element
  standing(card, { left: 320, right: 396, width: 76,
    top: 130, bottom: 236, height: 106 })
  fireEvent.mouseEnter(card)

  const peek = document.querySelector('.field-peek') as HTMLElement
  const left = Number.parseFloat(peek.style.left)
  const width = Number.parseFloat(peek.style.width)
  // Beside the tray, on the flank that has room: the tray's right edge is at
  // 700 and there are 500 pixels of room after it.
  expect(left, 'clear of the tray it came out of').toBeGreaterThanOrEqual(700)
  expect(left + width).toBeLessThanOrEqual(1200 - 8)
})

it('opens the hand from its nameplate, and never from the fan', () => {
  const { container } = render(
    <MatchBoard board={ZONES} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)

  const tray = container.querySelector('.field-hand-far .field-hand-tray')
  expect(tray, 'the hand opens the same way the piles do').toBeTruthy()
  expect(tray?.querySelectorAll('.field-card')).toHaveLength(1)

  // **The handle is a button, and the fan is not part of it.** The tray used
  // to open on `.field-hand:hover`, and the hand includes the fan — so running
  // a pointer along the fan to read one card sprang the whole hand open on top
  // of the preview that was answering the question (Aaron, 2026-08-25). Two
  // panels, one gesture, same patch of sand.
  //
  // Driven through the real control rather than by setting a class: a click is
  // what a tap, a mouse and the Enter key all produce, which is the whole
  // reason this is a `<button>` and not the `<span>` it was.
  const plate = container.querySelector<HTMLButtonElement>(
    '.field-hand-far .field-hand-label')
  expect(plate?.tagName, 'the nameplate is a real control').toBe('BUTTON')
  expect(plate?.getAttribute('aria-expanded')).toBe('false')
  expect(tray?.className).not.toContain('is-open')

  // The fan is inside the hand and must not open it. Hovering a card there is
  // the *other* question — what is this one card — and the preview answers it.
  fireEvent.mouseEnter(
    container.querySelector('.field-hand-far .field-hand-fan') as Element)
  expect(container.querySelector('.field-hand-far .field-hand-tray')?.className,
    'the fan is not the handle').not.toContain('is-open')

  fireEvent.click(plate as Element)
  expect(container.querySelector('.field-hand-far .field-hand-tray')?.className)
    .toContain('is-open')
  expect(plate?.getAttribute('aria-expanded')).toBe('true')
})

/** A board mid-combat, with a creature each side and one of them dead. */
const COMBAT: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 50, name: 'Fleecemane Lion', types: 'Creature - Cat', seat: 1,
      image: 'https://example.test/lion.jpg' },
    { id: 51, name: 'Regal Caracal', types: 'Creature - Cat', seat: 2,
      image: 'https://example.test/caracal.jpg' },
    { id: 52, name: 'Qasali Pridemage', types: 'Creature - Cat', seat: 1,
      image: 'https://example.test/pridemage.jpg' },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [
      { id: 50, zone: 'battlefield', seat: 1, power: 3, toughness: 3 },
      { id: 51, zone: 'battlefield', seat: 2, power: 4, toughness: 4 },
      { id: 52, zone: 'graveyard', seat: 1 },
    ] },
  ],
} as unknown as ForgeBoard

function combat(beat: StagedBeat | null) {
  return render(
    <MatchBoard board={COMBAT} shown={1} game={1} running={false} beat={beat}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

const said = (over: Partial<StagedBeat>): StagedBeat => ({
  key: 'k1', game: 1, turn: 1, kind: 'attack', who: 'Arahbo',
  text: 'attacks', ...over,
})

it('marks the card the beat is about, and only that card', () => {
  const { container } = combat(said({ kind: 'attack', card: 'Fleecemane Lion' }))
  const lion = container.querySelector('img[alt="Fleecemane Lion"]')
    ?.closest('.field-card')
  const caracal = container.querySelector('img[alt="Regal Caracal"]')
    ?.closest('.field-card')
  expect(lion?.className).toContain('is-attacks')
  expect(lion?.querySelector('.field-mark-attacks')).toBeTruthy()
  // The other creature on the board is not attacking and must not say it is.
  expect(caracal?.className).not.toContain('is-attacks')
  expect(caracal?.querySelector('.field-mark')).toBeNull()
})

it('brings a shield up on a blocker and nothing else', () => {
  const { container } = combat(said({ kind: 'block', card: 'Regal Caracal',
    target: 'Fleecemane Lion' }))
  const caracal = container.querySelector('img[alt="Regal Caracal"]')
    ?.closest('.field-card')
  expect(caracal?.querySelector('.field-mark-blocks img')).toBeTruthy()
  // The attacker it stepped in front of does not also get a shield: `target`
  // is who was blocked, not who blocked.
  const lion = container.querySelector('img[alt="Fleecemane Lion"]')
    ?.closest('.field-card')
  expect(lion?.querySelector('.field-mark')).toBeNull()
})

it('lays the skull on the grave, because the creature is already in it', () => {
  // **The constraint this encodes.** Forge reports the death and the zone
  // change on one line, so the step that tells the beat is the step that moves
  // the card: there is no instant at which the board holds a dead creature
  // still standing, and a mark aimed at the battlefield would land on nothing.
  const { container } = combat(said({ kind: 'dies', card: 'Qasali Pridemage' }))
  const grave = container.querySelector(
    '.field-side-far .field-pile-wrap:has([aria-label^="Graveyard"])')
  expect(grave?.querySelector('.field-pile-buried img'),
    'the grave that received it is marked').toBeTruthy()
  // The other seat's graveyard is empty and stays unmarked.
  const other = container.querySelector(
    '.field-side-near .field-pile-wrap:has([aria-label^="Graveyard"])')
  expect(other?.querySelector('.field-pile-buried')).toBeNull()
})

it('draws no mark at all for a beat that is not one of the three', () => {
  const { container } = combat(said({ kind: 'life', card: undefined }))
  expect(container.querySelectorAll('.field-mark')).toHaveLength(0)
  expect(container.querySelectorAll('.field-pile-buried')).toHaveLength(0)
  cleanup()
  // And none before a game has said anything.
  const quiet = combat(null)
  expect(quiet.container.querySelectorAll('.field-mark')).toHaveLength(0)
})

it('marks a transforming creature, whose two names are spelled differently', () => {
  // **The silent one.** Forge names a *face* and the board can carry
  // Scryfall's combined `A // B`, so a strict equality here would let every
  // transforming creature attack, block and die with no mark ever landing —
  // and only on the decks that play them, which is the worst way to be wrong.
  const dfc = {
    ...COMBAT,
    cards: [{ id: 60, name: 'Kazandu Mammoth // Kazandu Valley',
      types: 'Creature - Elephant', seat: 1,
      image: 'https://example.test/mammoth.jpg' }],
    steps: [{ turn: 1, seat: 1, changes: [
      { id: 60, zone: 'battlefield', seat: 1, power: 3, toughness: 3 }] }],
  } as unknown as ForgeBoard

  const { container } = render(
    <MatchBoard board={dfc} shown={1} game={1} running={false}
                beat={said({ kind: 'attack', card: 'Kazandu Mammoth' })}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
  const card = container.querySelector(
    'img[alt="Kazandu Mammoth // Kazandu Valley"]')?.closest('.field-card')
  expect(card?.className).toContain('is-attacks')
})
