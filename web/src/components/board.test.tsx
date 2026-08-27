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

import { act, cleanup, fireEvent, render, screen, within }
  from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import type { ForgeBoard } from '../lib/api'
import type { Speed, StagedBeat } from '../lib/reel'
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

  // And the sand still carries neither hand. A seat has **three lanes, always
  // the same three**, in the same order, whatever is standing on them — which
  // is the whole of "the board is one size" as a DOM can state it (Aaron,
  // 2026-08-27). It used to be five, two of which drew only when they held
  // something, so a board grew a lane the first time somebody cast a Signet
  // and every card on the *other* half moved to make room for it.
  //
  // This fixture is a Lion and a Forest: two lanes with something in them, and
  // a middle lane holding nothing at all — which still draws, and is the case
  // that would regress if anybody made an empty lane cheap again.
  const lanes = ['Lands and mana', 'Artifacts, enchantments and planeswalkers',
    'Creatures']
  for (const side of container.querySelectorAll('.field-side')) {
    const labels = [...side.querySelectorAll('.field-row')]
      .map((r) => (r.getAttribute('aria-label') ?? '').replace(/: \d+$/, ''))
    expect(labels.some((l) => l.startsWith('Hand')),
      'a hand is not a row on the battlefield').toBe(false)
    // The far player's lanes run outward from the seam and the near player's
    // mirror them, so the set is the invariant and the order is the mirror.
    expect([...labels].sort(), `three lanes, always: ${labels}`)
      .toEqual([...lanes].sort())
  }
  // Outermost first for the far seat, and reversed for the near one: the two
  // creature lanes finish up either side of the seam, which is where combat
  // happens.
  const laneNames = (facing: string) =>
    [...container.querySelectorAll(`.field-side-${facing} .field-row`)]
      .map((r) => (r.getAttribute('aria-label') ?? '').replace(/: \d+$/, ''))
  expect(laneNames('far')).toEqual(lanes)
  expect(laneNames('near')).toEqual([...lanes].reverse())

  // The card that went to hand is drawn in its owner's hand, and counted.
  const far = container.querySelector('.field-hand-far') as HTMLElement
  expect(within(far).getByLabelText('Hand: 1')).toBeTruthy()
})

/** A board where one commander is cast, dies back home, and is cast again —
 *  and a creature carries counters of both signs at once. */
const THRONE: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40,
      commanders: [30] },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 30, name: 'Arahbo, Roar of the World',
      types: 'Legendary Creature - Cat', seat: 1 },
    { id: 31, name: 'Fleecemane Lion', types: 'Creature - Cat', seat: 1 },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [{ id: 30, zone: 'command', seat: 1 }] },
    // **`casts` is the server's count**, not a transition the browser watches.
    // It is the number of times this card has left the command zone, and the
    // tax is two generic for each — so it arrives *with* the cast rather than
    // being worked back out of the zone changes around it. Going back into the
    // zone carries none: a commander coming home is not a cast.
    { turn: 2, seat: 1,
      changes: [{ id: 30, zone: 'battlefield', seat: 1, casts: 1 }] },
    { turn: 3, seat: 1, changes: [{ id: 30, zone: 'command', seat: 1 }] },
    // **A beat with nothing in it, and it is load-bearing.** A card that
    // leaves the battlefield is held standing there for exactly the beat that
    // says so, so the step above is the commander being *seen to die* rather
    // than the commander being home. It is home on the next beat, and that is
    // the one the questions below are asking about.
    { turn: 3, seat: 1, changes: [] },
    { turn: 4, seat: 1,
      changes: [{ id: 30, zone: 'battlefield', seat: 1, casts: 2 }] },
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
                speed="play" setSpeed={vi.fn()} of={6} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

it('seats an empty throne when the commander is out, and prices the return', () => {
  // Home after one cast: a card in the zone, and two generic on the next one.
  // Seat one's own rail: seat two never gets a commander in this fixture, so
  // its zone is legitimately an empty chair the whole way through and would
  // answer either question the wrong way.
  const railOf = (c: HTMLElement) =>
    c.querySelector('.field-zone-l-far') as HTMLElement

  const home = throne(4)
  expect(railOf(home.container).querySelector('.field-pile.is-vacant'),
    'a zone with the commander in it is not an empty chair').toBeNull()
  expect(railOf(home.container).querySelector('.field-tax')?.textContent?.trim())
    .toBe('+2')
  cleanup()

  // Out again: the chair is empty and the price has gone up.
  const away = throne(5)
  expect(railOf(away.container).querySelectorAll('.field-pile.is-vacant'),
    'the commander is on the battlefield, so the seat is empty').toHaveLength(1)
  expect(railOf(away.container).querySelector('.field-tax')?.textContent?.trim())
    .toBe('+4')
})

it('draws counters as their own signed chips rather than one sum', () => {
  const { container } = throne(6)
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
  const { container } = throne(6)
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

/** A click, the way a mouse makes one — pressed and lifted, main button.
 *
 *  `button` is defined and `tap` does not define it, which is the whole
 *  difference: `FieldCard` turns away a right- or middle-click, because that
 *  is the browser opening its own menu rather than somebody asking to read a
 *  card. jsdom has no `PointerEvent`, so a plain `Event` reports `undefined`
 *  for `button` and would be refused — a mouse test that forgets this passes
 *  for the wrong reason. Touch never reaches that test at all. */
function click(el: Element, button = 0) {
  for (const type of ['pointerdown', 'pointerup']) {
    const ev = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(ev, 'pointerType', { value: 'mouse' })
    Object.defineProperty(ev, 'button', { value: button })
    fireEvent(el, ev)
  }
}

/** The one card in `BOARD` the pool gave a painting to. */
function lion(container: HTMLElement) {
  return container.querySelector('img[alt="Fleecemane Lion"]')
    ?.closest('.field-card') as Element
}

it('hands the whole card to a mouse too, which had no way in at all', () => {
  const { container } = show()
  const card = lion(container)

  // **Hover keeps its job.** The peek is the mouse's fast path — a glance at
  // one card without committing to anything — and the sheet is the long look.
  // Two questions, two answers; the click must not have eaten the first one.
  fireEvent.mouseEnter(card)
  expect(document.querySelector('.field-peek'),
    'hover still peeks').toBeTruthy()
  expect(screen.queryByRole('dialog'),
    'and peeking is not holding the card up').toBeNull()

  // The gap this closes: `onPointerUp` used to return on `pointerType ===
  // 'mouse'`, so a desktop player could not open this sheet by any means.
  click(card)
  expect(screen.getByRole('dialog', { name: 'Fleecemane Lion' })).toBeTruthy()
  expect(document.querySelector('.field-peek'),
    'and the peek stands down rather than sitting behind it').toBeNull()

  // **Nor can it come back while the card is up.** Both are portalled to the
  // body, so a peek armed behind a sheet does not hide politely underneath
  // it — it draws a second copy of the same card over the dimmed room. Seen
  // in Chrome on a suited-up creature, where the sheet mounting under the
  // cursor was enough to re-arm the card it had just covered.
  fireEvent.mouseEnter(card)
  fireEvent.focus(card)
  expect(document.querySelector('.field-peek'),
    'one answer at a time, whatever the pointer does').toBeNull()
})

it('leaves a right-click to the browser, whose menu it already is', () => {
  const { container } = show()
  click(lion(container), 2)
  expect(screen.queryByRole('dialog')).toBeNull()
  click(lion(container), 1)
  expect(screen.queryByRole('dialog'), 'nor a middle-click').toBeNull()
})

it('opens on Enter and on Space, and Escape puts the card back down', () => {
  const { container } = show()
  const card = lion(container)

  for (const key of ['Enter', ' ']) {
    fireEvent.keyDown(card, { key })
    expect(screen.getByRole('dialog', { name: 'Fleecemane Lion' }),
      `${key === ' ' ? 'Space' : key} holds the card up`).toBeTruthy()
    fireEvent.keyDown(card, { key: 'Escape' })
    expect(screen.queryByRole('dialog'), 'and Escape puts it down').toBeNull()
  }

  // A key nobody bound does nothing, which is worth holding: the handler sits
  // on every card on the board and reads every key pressed inside one.
  fireEvent.keyDown(card, { key: 'a' })
  expect(screen.queryByRole('dialog')).toBeNull()
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
  const grave = container.querySelector('.field-zone-l-far .field-pile-wrap:has('
    + '[aria-label^="Graveyard"]) .field-tray') as HTMLElement
  expect(grave, 'the graveyard has a tray').toBeTruthy()
  expect(grave.getAttribute('aria-label')).toBe('Graveyard, all 2')
  expect(grave.querySelectorAll('.field-card')).toHaveLength(2)

  // An empty zone gets no tray at all rather than an empty one: there is
  // nothing to look through, and a panel that opens onto nothing is a worse
  // answer than a pile that stays shut.
  const exileWrap = container.querySelector(
    '.field-zone-l-near .field-pile-wrap:has([aria-label^="Exile"])')
  expect(exileWrap?.querySelector('.field-tray'),
    "the near seat's exile is empty, so it does not open").toBeNull()
})

/** A whole tap — pressed and lifted.
 *
 *  `tap` above lifts without ever pressing, which is all a handler listening
 *  on the way *up* needs and not enough for the dismissal, which listens on
 *  the way down so a tray gets out of the way before the thing underneath it
 *  is touched. */
function press(el: Element) {
  for (const type of ['pointerdown', 'pointerup']) {
    const ev = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(ev, 'pointerType', { value: 'touch' })
    fireEvent(el, ev)
  }
}

/** The zones of one seat's rail, by the label on the tile. */
function zone(container: HTMLElement, rail: 'far' | 'near', label: string) {
  const wrap = container.querySelector(`.field-zone-l-${rail} .field-pile-wrap`
    + `:has([aria-label^="${label}"])`) as HTMLElement
  return { wrap, pile: wrap?.querySelector('.field-pile') as HTMLElement,
    tray: wrap?.querySelector('.field-tray') as HTMLElement }
}

function zones() {
  return render(
    <MatchBoard board={ZONES} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

/* **What these four cannot see, and where it was seen instead.**
 *
 * Half of this behaviour is a stylesheet — `:hover` gated on `(hover: hover)`
 * so a phone never latches it, and `:focus-visible` in place of
 * `:focus-within` so a click stops pinning the panel open. jsdom has no
 * layout and this suite reads `index.css` as an empty string, so a guard
 * written here against either would read nothing and cheerfully report no
 * problem. Both halves were driven in a real browser instead: Chrome at
 * 1280x900 for the click, and an emulated phone at 375x812 with `hover: none`
 * and trusted touch events for the tap.
 *
 * What is left is the half that *is* structure — which class this component
 * puts on the tray, and when — and that is exactly what these hold. */

it('shuts the tray when you tap the same zone a second time', () => {
  const { container } = zones()
  const grave = zone(container, 'far', 'Graveyard')

  // The thing you opened is the thing that closes it. Nothing used to: the
  // second tap did clear this class, and a `:hover` latched by the first tap
  // went on holding the panel up, so the zone had no way to shut.
  press(grave.pile)
  expect(grave.tray.className, 'the first tap opens it').toContain('is-open')

  press(grave.pile)
  expect(grave.tray.className, 'the second tap shuts it')
    .not.toContain('is-open')
  // **Shut on purpose, which is a different state from never-opened.** Only
  // this one outranks a hover the browser has not let go of yet.
  expect(grave.tray.className).toContain('is-shut')
})

it('shuts a tray from outside it, but never from inside it', () => {
  const { container } = zones()
  const grave = zone(container, 'far', 'Graveyard')
  press(grave.pile)
  expect(grave.tray.className).toContain('is-open')

  // **The trap.** A dismissal that only asks "was this tap somewhere else"
  // shuts the graveyard the instant somebody reaches for a card in the
  // graveyard — which is the one thing they opened it to do.
  press(grave.tray.querySelector('.field-card') as Element)
  expect(grave.tray.className, 'reaching into the pile is not a dismissal')
    .toContain('is-open')

  press(container.querySelector('.field-zone-l-near') as Element)
  expect(grave.tray.className, 'a tap anywhere else puts it away')
    .toContain('is-shut')
})

it('lets Escape out of an open tray', () => {
  const { container } = zones()
  const grave = zone(container, 'far', 'Graveyard')
  press(grave.pile)
  expect(grave.tray.className).toContain('is-open')

  // From the document, not from the tile: a tap opens this tray without
  // focusing anything, so the key is pressed with the focus still on nothing
  // in particular.
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(grave.tray.className, 'Escape is the way out everywhere else')
    .toContain('is-shut')
})

/** A graveyard whose cards have paintings, so one can be lifted out of it.
 *
 *  Its own fixture rather than an image added to `ZONES`: the four tests above
 *  press cards in that tray to prove a dismissal, and giving those cards a
 *  painting would open a sheet in the middle of each one. */
const BURIED = {
  seats: [
    { seat: 1, slug: 'arahbo', name: 'Arahbo — Cats', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 50, name: 'Qasali Pridemage', types: 'Creature - Cat', seat: 1,
      image: 'https://example.test/qasali.jpg' },
    { id: 51, name: 'Regal Caracal', types: 'Creature - Cat', seat: 1,
      image: 'https://example.test/caracal.jpg' },
  ],
  steps: [{ turn: 1, seat: 1, changes: [
    { id: 50, zone: 'graveyard', seat: 1 },
    { id: 51, zone: 'graveyard', seat: 1 },
  ] }],
} as unknown as ForgeBoard

function buried() {
  const { container } = render(
    <MatchBoard board={BURIED} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
  const grave = zone(container, 'far', 'Graveyard')
  return { container, grave,
    card: grave.tray.querySelector('.field-card') as Element }
}

it('keeps the pile spread while one of its cards is being read', () => {
  const { grave, card } = buried()

  // **A tray is held open by two different things and only one survives a
  // modal.** A tap sets `is-open`; a mouse gets nothing but `:hover` on the
  // wrapper — and the sheet is a full-viewport panel that takes the pointer
  // the instant it opens, so the pile shut itself behind the very card it had
  // just handed over. Lifting a card out of a pile now puts that pile into
  // the same state a tap does, which is the half of this a suite can see.
  expect(grave.tray.className, 'nothing has opened it yet')
    .not.toContain('is-open')
  click(card)
  expect(screen.getByRole('dialog', { name: 'Qasali Pridemage' })).toBeTruthy()
  expect(grave.tray.className, 'the pile stays spread behind the card')
    .toContain('is-open')

  // And putting it back finds the graveyard as it was left, which is the
  // whole point — the state outlives the sheet rather than tracking it.
  fireEvent.click(screen.getByRole('dialog'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(grave.tray.className, 'and it is still spread afterwards')
    .toContain('is-open')

  // Not a latch: every dismissal the zone already had still ends it.
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(grave.tray.className).toContain('is-shut')
})

it('puts the card down on Escape without shutting the pile it came from', () => {
  const { grave, card } = buried()
  click(card)
  expect(grave.tray.className).toContain('is-open')

  // **One press, one meaning.** The zone wrapper closes its tray on Escape,
  // and this card is inside that wrapper — so without stopping the key here,
  // a single Escape would put the card down *and* shut the pile behind it,
  // and the reader would be two gestures from where they were.
  fireEvent.keyDown(card, { key: 'Escape' })
  expect(screen.queryByRole('dialog'), 'the card goes back').toBeNull()
  expect(grave.tray.className, 'the pile does not').not.toContain('is-shut')
  expect(grave.tray.className).toContain('is-open')

  // A second Escape, with no card held, is the tray's own again.
  fireEvent.keyDown(card, { key: 'Escape' })
  expect(grave.tray.className, 'and now it shuts').toContain('is-shut')
})

it('spreads one zone at a time', () => {
  const { container } = zones()
  const grave = zone(container, 'far', 'Graveyard')
  const exile = zone(container, 'far', 'Exile')

  press(grave.pile)
  expect(grave.tray.className).toContain('is-open')

  // Two piles spread at once is two panels answering one question, and the
  // second one lands over the sand the first was there to be compared
  // against. It costs nothing to arrange: opening the exile is a press
  // *outside* the graveyard, so the graveyard puts itself away on the way
  // past and no zone has to know that any other zone exists.
  press(exile.pile)
  expect(exile.tray.className, 'the one you just asked for').toContain('is-open')
  expect(grave.tray.className, 'the one you had open').toContain('is-shut')
})

it('says what a creature does in the one corner that was free', () => {
  // A painting at fifty-eight pixels tells you this is a Dragon. Whether the
  // Dragon *flies* is the question when the other side has ground blockers,
  // and the only way to ask it was to hover forty cards one at a time.
  const board = {
    seats: [{ seat: 1, slug: 'a', name: 'A', life: 40 },
      { seat: 2, slug: 'b', name: 'B', life: 40 }],
    cards: [
      { id: 1, name: 'A flier', types: 'Creature - Dragon', seat: 1,
        image: 'https://example.test/d.jpg',
        // Scryfall's own casing, and a keyword with no sign, both on purpose.
        keywords: ['Flying', 'Lifelink', 'Kicker'] },
      { id: 2, name: 'Held back', types: 'Creature - Bear', seat: 1,
        image: 'https://example.test/b.jpg', keywords: ['Trample'] },
    ],
    steps: [
      { changes: [{ id: 1, zone: 'battlefield', seat: 1 }] },
      { changes: [{ id: 2, zone: 'hand', seat: 1 }] },
    ],
  } as unknown as ForgeBoard
  const { container } = render(
    <MatchBoard board={board} shown={2} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={2} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)

  const flier = container.querySelector('.field-row .field-card') as HTMLElement
  const marks = flier.querySelectorAll('.field-keyword')
  expect(marks, 'flying and lifelink are drawn; Kicker is not')
    .toHaveLength(2)
  // On the arm, so they ride the card round when it turns and stay upright —
  // the same reason the loupe and the counters are there.
  expect(marks[0]?.closest('.field-card-arm')).toBeTruthy()
  // The arm is `aria-hidden`, so the card's own title is how anybody not
  // looking at ten-pixel pictures gets these.
  expect(flier.getAttribute('title')).toContain('flying, lifelink')
  expect(flier.getAttribute('title'), 'and only what is drawn')
    .not.toContain('Kicker')

  // Not in a hand. Keywords are facts about a fight, and a card being held is
  // not in one — the same line `inPlay` draws for the loupe.
  // Seat one is the far side of the table, which is where its hand is.
  const held = container.querySelector('.field-hand-far .field-card')
  expect(held, 'the fixture put a card in a hand').toBeTruthy()
  expect(held?.querySelectorAll('.field-keyword')).toHaveLength(0)
})

/** The four paintings the board's own zones wear. Checked-in prose on the
 *  server, pinned to a printing — see `ColiseumZone`. */
const DRESSING = [
  { key: 'command' as const, card: 'Throne of Eldraine', why: '',
    art: { url: 'https://example.test/throne.jpg', artist: 'Kieran Yanner',
           printing: 'Wilds of Eldraine Commander' } },
  { key: 'graveyard' as const, card: 'Ancient Tomb', why: '',
    art: { url: 'https://example.test/tomb.jpg', artist: 'Colin MacNeil',
           printing: 'Tempest' } },
  { key: 'exile' as const, card: 'Path to Exile', why: '',
    art: { url: 'https://example.test/path.jpg', artist: 'Torgeir Fjereide',
           printing: 'Tales of Middle-earth Commander' } },
  { key: 'ghost' as const, card: 'Crypt Ghast', why: '',
    art: { url: 'https://example.test/ghast.jpg', artist: 'Chris Rahn',
           printing: 'Gatecrash' } },
]

it('draws life as how much is left, not just as a number', () => {
  // **A bare numeral is the wrong fact.** What a player reads off a life total
  // is how much is *left*, and a "23" makes you do that arithmetic. The ring
  // is the arithmetic done: the arc is life over forty and the figure sits in
  // it, so trouble is visible without reading a digit.
  const at = (life: number) => {
    const board = {
      seats: [{ seat: 1, slug: 'a', name: 'A', life: 40 },
        { seat: 2, slug: 'b', name: 'B', life: 40 }],
      cards: [{ id: 1, name: 'X', types: 'Creature', seat: 1 }],
      steps: [{ life: [{ seat: 1, life }], changes: [] }],
    } as unknown as ForgeBoard
    const { container } = render(
      <MatchBoard board={board} shown={1} game={1} running={false}
                  name={(_s, f) => f} speed="play" setSpeed={vi.fn()}
                  of={1} seek={vi.fn()} games={[1]} playing={1}
                  chooseGame={vi.fn()} />)
    const ring = container.querySelector('.field-plate-far .field-life') as HTMLElement
    return {
      shown: ring?.querySelector('.field-life-n')?.textContent,
      left: ring?.style.getPropertyValue('--life-left'),
      spent: ring?.style.getPropertyValue('--life-spent'),
    }
  }

  const full = at(40)
  expect(full.shown).toBe('40')
  expect(Number(full.left)).toBe(1)
  expect(full.spent).toBe('0%')
  cleanup()

  const half = at(20)
  expect(half.shown).toBe('20')
  expect(Number(half.left)).toBeCloseTo(0.5, 5)
  expect(half.spent).toBe('50%')
  cleanup()

  // Clamped both ways, because Commander does both: a lifegain deck goes past
  // forty and the ring simply reads full, and a dead player reads empty rather
  // than winding the arc backwards.
  const gained = at(63)
  expect(gained.shown, 'the figure is always the truth').toBe('63')
  expect(Number(gained.left), 'the ring stops at full').toBe(1)
  cleanup()

  const dead = at(-3)
  expect(dead.shown).toBe('-3')
  expect(Number(dead.left)).toBe(0)
  expect(dead.spent).toBe('100%')
})

it('tucks each seat\'s zones into the corner of that seat\'s own half', () => {
  // **The property, and it has been got wrong in both directions.** The zones
  // were once a column inside the field, which stood them across the middle of
  // the sand and took their width out of the battlefield; then a band under
  // the arena, which cost a band's height and put a player's graveyard as far
  // from their own creatures as the page could manage.
  //
  // What settles it is a fact about Magic rather than about layout: a
  // battlefield is wide and shallow, so each half's outer corner is sand
  // nothing is ever played onto. The zones go *there* — on the sand, and in
  // the one part of it no card competes for.
  //
  // Held as containment rather than as geometry, for the reason it always was:
  // the grid places these differently at each breakpoint, and *whose half is
  // this in* is the thing neither branch can quietly get wrong. It is also the
  // half of this that a jsdom suite can actually answer — see
  // [[css-sizing-is-invisible-to-the-suite]]; the corner being the *corner* is
  // Aaron's walk, not this file's.
  const { container } = show()
  const field = container.querySelector('.field') as HTMLElement
  expect(field).toBeTruthy()
  expect(container.querySelector('.field-zones'),
    'the band under the arena is gone').toBeNull()
  for (const facing of ['far', 'near'] as const) {
    expect(
      container.querySelector(`.field-side-${facing} > .field-zone-l-${facing}`),
      `${facing}'s zones are in ${facing}'s own half`).toBeTruthy()
  }
  // And the L is an L: three cells, and the fourth corner left as open sand.
  const cells = [...container.querySelectorAll(
    '.field-zone-l-far > .field-zone-cell')].map((c) => c.className)
  expect(cells).toHaveLength(3)
  expect(cells.join(' ')).toContain('is-command')
  expect(cells.join(' ')).toContain('is-graveyard')
  expect(cells.join(' ')).toContain('is-exile')

  // The hands stay in the arena, on the player's left, where a hand is held.
  expect(field.querySelector('.field-hand-far')).toBeTruthy()
  expect(field.querySelector('.field-hand-near')).toBeTruthy()

  // And within the field, hand comes before the half it belongs to at every
  // width — document order is what survives both breakpoints.
  const order = [...field.children]
    .map((el) => ['field-hand-far', 'field-side-far', 'field-seam',
      'field-side-near', 'field-hand-near']
      .find((c) => el.classList.contains(c)))
    .filter(Boolean)
  expect(order).toEqual(['field-hand-far', 'field-side-far', 'field-seam',
    'field-side-near', 'field-hand-near'])
})

it('gives each zone its whole name, now there is room for one', () => {
  // "GY" and "CMD" are what you write when a tile is 26px wide.
  const { container } = show()
  const names = [...container.querySelectorAll('.field-zone-l-far .field-pile-label')]
    .map((n) => n.textContent)
  expect(names).toEqual(['Command Zone', 'Graveyard', 'Exile'])
})

it('puts both players in the trench, each plate against its own half', () => {
  // The names and the life rings were carved into the stone band under the
  // arena — as far from the sand as the page could put them, so finding out
  // whether somebody was dying meant looking away from the game (Aaron,
  // 2026-08-27). They are in the seam now, which is the one strip of this
  // board both players are already looking at.
  //
  // **Two plates and two only.** A third would mean the old band had not
  // actually gone, which is exactly the kind of leftover this file has caught
  // before.
  const { container } = show()
  const seam = container.querySelector('.field-seam') as HTMLElement
  const plates = [...seam.querySelectorAll('.field-plate')]
  expect(plates).toHaveLength(2)
  expect(container.querySelectorAll('.field-plate')).toHaveLength(2)

  // Far first, near second, and each carries its own seat's name and life —
  // which is the whole of "which one is mine" that a DOM can answer.
  expect(plates[0]?.className).toContain('field-plate-far')
  expect(plates[1]?.className).toContain('field-plate-near')
  expect(plates[0]?.querySelector('.field-plate-name')?.textContent)
    .toContain('Arahbo')
  expect(plates[1]?.querySelector('.field-plate-name')?.textContent)
    .toContain('Atla')
  for (const plate of plates) {
    expect(plate.querySelector('.field-life'),
      'a plate carries the life ring, not just the name').toBeTruthy()
  }

  // The turn stands between them rather than beside them: it is the fixed
  // point of this band, and a long deck name must not shove it off centre.
  const order = [...seam.children].map((c) => c.className.split(' ')[0])
  expect(order).toEqual(['field-plate', 'field-seam-turn', 'field-plate'])
})

it('dresses each zone in its own painting, and names the painter', () => {
  // Three-letter labels on a 26px tile is what a scoreboard does; a player
  // knows these three places by sight. The painting says *which* zone.
  const { container } = render(
    <MatchBoard board={COMBAT} shown={2} game={1} running={false} beat={null}
                zones={DRESSING} name={(_s, f) => f}
                speed="play" setSpeed={vi.fn()} of={2} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)

  const pileFor = (label: string) => container.querySelector(
    `.field-zone-l-far .field-pile[aria-label^="${label}"]`)
  const groundOf = (label: string) =>
    pileFor(label)?.querySelector('.field-pile-ground')?.getAttribute('src')

  expect(groundOf('Graveyard')).toBe('https://example.test/tomb.jpg')
  expect(groundOf('Exile')).toBe('https://example.test/path.jpg')
  expect(groundOf('Command zone')).toBe('https://example.test/throne.jpg')

  // Somebody painted it, and rule 9 says name them. The arm the marks ride is
  // `aria-hidden`, so the pile's own title is where this has to live.
  expect(pileFor('Graveyard')?.getAttribute('title'))
    .toContain('Ancient Tomb, art by Colin MacNeil')

  // The ghost is not a zone and must not be drawn as one.
  expect(container.querySelectorAll('.field-pile-ground')).toHaveLength(6)
})

it('draws the brass tiles it always had when no painting arrives', () => {
  // `/api/coliseum` answers before any match is asked for, and with no pool at
  // all — but a deployment that has not been refreshed, or a future room that
  // forgets to pass them, must still get a board. Empty is a legible state.
  const { container } = render(
    <MatchBoard board={COMBAT} shown={2} game={1} running={false} beat={null}
                name={(_s, f) => f}
                speed="play" setSpeed={vi.fn()} of={2} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
  expect(container.querySelectorAll('.field-pile-ground')).toHaveLength(0)
  expect(container.querySelectorAll('.field-pile').length).toBeGreaterThan(0)
})

it('raises a ghost off the grave that just received something', () => {
  // Two beats, two pictures: the skull lands on the creature as it dies, held
  // on the sand for that beat, and this rises from the zone it is bound for.
  const { container } = render(
    <MatchBoard board={COMBAT} shown={2} game={1} running={false}
                beat={said({ kind: 'dies', card: 'Qasali Pridemage' })}
                zones={DRESSING} name={(_s, f) => f}
                speed="play" setSpeed={vi.fn()} of={2} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)

  const ghosts = container.querySelectorAll('.field-pile-ghost img')
  expect(ghosts, 'one grave, one ghost').toHaveLength(1)
  expect(ghosts[0]?.getAttribute('src')).toBe('https://example.test/ghast.jpg')
  // On the graveyard, never on exile or the command zone.
  expect(container.querySelector('.field-pile-ghost')
    ?.closest('.field-pile')?.getAttribute('aria-label')).toMatch(/^Graveyard/)
  // And the skull is still on the creature — these are two marks, not one.
  expect(container.querySelector('.field-row .field-card.is-dies '
    + '.field-mark-dies img')).toBeTruthy()
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
  expect(throne(6).container
    .querySelectorAll('.field-row .field-card-lens').length)
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

/** Where a preview ended up, and what it hit. jsdom measures nothing, so the
 *  rectangles are the ones the test hands out — which is no worse than the
 *  truth: this placement is only ever as good as the numbers it is given. */
function placed(tray: DOMRect | { left: number; right: number; top: number
  bottom: number }) {
  const peek = document.querySelector('.field-peek') as HTMLElement
  const left = Number.parseFloat(peek.style.left)
  const top = Number.parseFloat(peek.style.top)
  const width = Number.parseFloat(peek.style.width)
  // 680/488 plus the artist line, which this fixture's cards do not carry.
  const height = width * (680 / 488)
  return {
    left, top, width, right: left + width, bottom: top + height,
    clear: left >= tray.right || left + width <= tray.left
      || top >= tray.bottom || top + height <= tray.top,
  }
}

it('steps a preview over or under a tray no flank can hold', () => {
  // **The case the two-flank rule could not answer.** It tried the right side
  // of the tray, then the left, and gave up — so a tray with under 318px
  // beside it fell through to the ordinary placement and landed *on the panel
  // somebody had just opened in order to read it*. On a phone that is every
  // tray there is, and the hands live in the left-hand column at every width
  // above 62rem, which is exactly where Aaron kept seeing it (2026-08-26:
  // "full hand previews look clipped when they are on the lefthand side").
  viewport(375, 812)
  const { container } = show()
  fireEvent.click(container.querySelector(
    '.field-hand-far .field-hand-label') as Element)
  const tray = container.querySelector(
    '.field-hand-far .field-hand-tray') as Element
  // A hand tray on a phone: nearly the full width, hung under the plate.
  const box = { left: 96, right: 366, top: 133, bottom: 382 }
  standing(tray, { ...box, width: 270, height: 249 })
  const card = tray.querySelector('.field-card') as Element
  standing(card, { left: 104, top: 157, right: 180, bottom: 263,
    width: 76, height: 106 })
  fireEvent.mouseEnter(card)

  const at = placed(box)
  // Neither flank can hold anything — 96 on the left, 9 on the right — so it
  // goes *under*, which is the room a phone actually has.
  expect(at.clear, 'never on top of the tray it came out of').toBe(true)
  expect(at.top, 'below the tray').toBeGreaterThanOrEqual(box.bottom)
  // And inside the glass, which the old `PEEK_MIN_W` floor could not promise:
  // it forbade anything under 160px, so on a narrow viewport the only way to
  // obey it was to hang off the edge.
  expect(at.left).toBeGreaterThanOrEqual(8)
  expect(at.right).toBeLessThanOrEqual(375 - 8)
  expect(at.bottom, 'and inside it top to bottom').toBeLessThanOrEqual(812 - 8)
  // Shrunk to fit the room under the tray rather than clipped to it. Still
  // far bigger than the 76px cards in the tray, which is the whole point of
  // stepping aside rather than covering.
  expect(at.width).toBeLessThan(300)
  expect(at.width).toBeGreaterThan(160)
})

it('covers a tray rather than drawing a sliver beside it', () => {
  // The other end of the same decision. When no clearing round the tray can
  // hold a *legible* card, the honest answer is the one this has always
  // given in that corner: cover the tray. A 40px preview beside it would be
  // smaller than the cards already in it.
  viewport(360, 300)
  const { container } = show()
  fireEvent.click(container.querySelector(
    '.field-hand-far .field-hand-label') as Element)
  const tray = container.querySelector(
    '.field-hand-far .field-hand-tray') as Element
  const box = { left: 30, right: 330, top: 30, bottom: 270 }
  standing(tray, { ...box, width: 300, height: 240 })
  const card = tray.querySelector('.field-card') as Element
  standing(card, { left: 40, top: 60, right: 116, bottom: 166,
    width: 76, height: 106 })
  fireEvent.mouseEnter(card)

  const at = placed(box)
  expect(at.clear, 'there was nowhere to step to').toBe(false)
  // Covering is allowed; leaving the glass is not, and that is the invariant
  // that has to hold at *every* width.
  expect(at.left).toBeGreaterThanOrEqual(8)
  expect(at.right).toBeLessThanOrEqual(360 - 8)
  expect(at.top).toBeGreaterThanOrEqual(8)
  expect(at.bottom).toBeLessThanOrEqual(300 - 8)
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
      // **It has to stand somewhere before it can be seen to fall.** This card
      // used to be put straight into a graveyard, which was an honest fixture
      // when the skull had nowhere to land but the pile. Now the mark goes on
      // the card, and a card that was never on the battlefield never leaves it.
      { id: 52, zone: 'battlefield', seat: 1, power: 2, toughness: 2 },
    ] },
    { turn: 1, seat: 1, changes: [{ id: 52, zone: 'graveyard', seat: 1 }] },
  ],
} as unknown as ForgeBoard

/** `shown` is 1 for the beats that happen while everything is still standing,
 *  and 2 for the death — which is the last step, so the card that took it is
 *  held on the sand for exactly that beat. */
function combat(beat: StagedBeat | null, shown = 1) {
  return render(
    <MatchBoard board={COMBAT} shown={shown} game={1} running={false} beat={beat}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={2} seek={vi.fn()}
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

it('lays the skull on the creature, held where it fell', () => {
  // **The constraint that used to make this impossible.** Forge reports a
  // death and the zone change on one line, so the step that tells the beat is
  // the step that moves the card — there was no instant at which the board
  // held a dead creature still standing, and a mark aimed at the battlefield
  // landed on nothing. The skull went on the *pile* instead, which is where a
  // headstone goes and not where a death happens (Aaron, 2026-08-25: "it
  // should appear over the card being destroyed itself, like the shield").
  //
  // `foldBoard` holds a card that left the battlefield on the final applied
  // step in the row it left, for exactly that beat. So the mark lands on the
  // card, and the grave it is about to enter shows nothing yet.
  const { container } = combat(
    said({ kind: 'dies', card: 'Qasali Pridemage' }), 2)

  const fallen = container.querySelector('img[alt="Qasali Pridemage"]')
    ?.closest('.field-card')
  expect(fallen, 'the dead creature is still on the sand for its own beat')
    .toBeTruthy()
  expect(fallen?.className).toContain('is-dies')
  expect(fallen?.className, 'and it is visibly on its way out')
    .toContain('is-leaving')
  expect(fallen?.querySelector('.field-mark-dies img'),
    'the skull is on the card').toBeTruthy()

  // It is in one place, not two: the graveyard it is bound for does not also
  // hold it this beat. A card in two zones is a worse answer than a card a
  // beat behind.
  const grave = container.querySelector(
    '.field-zone-l-far .field-pile-wrap [aria-label^="Graveyard"]')
  expect(grave?.getAttribute('aria-label')).toBe('Graveyard: 0')

  // The creatures that did not die carry nothing.
  const lion = container.querySelector('img[alt="Fleecemane Lion"]')
    ?.closest('.field-card')
  expect(lion?.querySelector('.field-mark')).toBeNull()
  expect(lion?.className).not.toContain('is-leaving')
})

it('lets the dead go on the next beat, rather than holding them forever', () => {
  // The hold is a function of the count, like everything else in the fold —
  // which is what makes it survive a scrub in both directions.
  const { container } = combat(null, 2)
  expect(container.querySelector('img[alt="Qasali Pridemage"]')
    ?.closest('.field-row'), 'held on the beat it fell').toBeTruthy()
  cleanup()

  // One beat earlier it was alive and the graveyard was empty.
  const before = combat(null, 1)
  expect(before.container.querySelector(
    '.field-zone-l-far .field-pile-wrap [aria-label^="Graveyard"]')
    ?.getAttribute('aria-label')).toBe('Graveyard: 0')
  expect(before.container.querySelector('.field-card.is-leaving')).toBeNull()
})

it('draws no mark at all for a beat that is not one of the three', () => {
  const { container } = combat(said({ kind: 'life', card: undefined }))
  expect(container.querySelectorAll('.field-mark')).toHaveLength(0)
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

/** The same board, drawn again with a different beat — which is the mechanism
 *  the linger is *made of*, so the tests below drive it rather than a proxy.
 *  Nothing here is about pixels: whether a mark is on the page at all is
 *  structure, and it is the half of this that a suite with no layout engine
 *  can still hold honestly. */
function replay(first: StagedBeat | null,
  opts: { speed?: Speed; game?: number; shown?: number } = {}) {
  const props = (beat: StagedBeat | null, over: typeof opts = {}) => ({
    board: COMBAT, shown: over.shown ?? opts.shown ?? 1,
    game: over.game ?? opts.game ?? 1, running: false, beat,
    name: (_slug: string | null, fallback: string) => fallback,
    speed: over.speed ?? opts.speed ?? ('play' as Speed), setSpeed: vi.fn(),
    of: 2, seek: vi.fn(), games: [1], playing: 1, chooseGame: vi.fn(),
  })
  const view = render(<MatchBoard {...props(first)} />)
  return {
    ...view,
    then: (next: StagedBeat | null, over: typeof opts = {}) =>
      view.rerender(<MatchBoard {...props(next, over)} />),
  }
}

it('holds a mark past the beat that raised it, while beats are draining', () => {
  // **The bug this is here for was invisible from the stylesheet.** A mark used
  // to be a pure function of the beat, so it was torn down the moment the next
  // sentence landed — 480ms at watching pace, against a shield animation of a
  // second. Every mark was being cut off before it was half told, and making
  // the CSS longer did nothing at all, because the CSS was never what ended it.
  const { container, then } = replay(
    said({ kind: 'block', card: 'Regal Caracal', key: 'b1' }))
  const shield = () => container.querySelector('.field-mark-blocks img')
  expect(shield(), 'the block raises a shield').toBeTruthy()

  // The next beat says something the board has no mark for. Before this change
  // that alone took the shield down.
  then(said({ kind: 'life', card: undefined, key: 'b2' }))
  expect(shield(), 'a silent beat leaves the mark alone').toBeTruthy()

  // And so does the one after it, and the one after that.
  then(said({ kind: 'draw', card: undefined, key: 'b3' }))
  then(said({ kind: 'cast', card: undefined, key: 'b4' }))
  expect(shield(), 'and so does every beat with nothing of its own to say')
    .toBeTruthy()
})

it('lets a new mark replace the held one rather than joining it', () => {
  // The invariant that survived the change: **one mark at a time.** Two marks a
  // beat apart on two different cards is a board asking to be read in two
  // places at once, and a hold that accumulated would produce exactly that.
  const { container, then } = replay(
    said({ kind: 'block', card: 'Regal Caracal', key: 'c1' }))
  then(said({ kind: 'attack', card: 'Fleecemane Lion', key: 'c2' }))

  expect(container.querySelectorAll('.field-mark'), 'one mark, not two')
    .toHaveLength(1)
  expect(container.querySelector('.field-mark-blocks'),
    'and it is the new one').toBeNull()
  const lion = container.querySelector('img[alt="Fleecemane Lion"]')
    ?.closest('.field-card')
  expect(lion?.querySelector('.field-mark-attacks')).toBeTruthy()
})

it('gives the mark back to its beat while the transport is paused', () => {
  // Stepping and scrubbing both pause first, and that is exactly where holding
  // a mark past its beat would be wrong: a scrub has to land on the board its
  // beat describes and never on the one before it. Paused, nothing is
  // draining, so there is nothing for a mark to outlive.
  const { container, then } = replay(
    said({ kind: 'block', card: 'Regal Caracal', key: 'p1' }),
    { speed: 'paused' })
  expect(container.querySelector('.field-mark-blocks')).toBeTruthy()

  then(said({ kind: 'life', card: undefined, key: 'p2' }), { speed: 'paused' })
  expect(container.querySelectorAll('.field-mark'),
    'scrubbed off the beat, the mark goes with it').toHaveLength(0)
})

it('drops a held mark at a game boundary rather than carrying it over', () => {
  // Choosing another game of the same match keeps this component mounted, so a
  // shield held over from game one could land on a same-named creature in game
  // two. Both games here have a Regal Caracal, which is what makes the mistake
  // possible and invisible.
  const { container, then } = replay(
    said({ kind: 'block', card: 'Regal Caracal', key: 'g1' }))
  expect(container.querySelector('.field-mark-blocks')).toBeTruthy()

  then(said({ kind: 'life', card: undefined, key: 'g2', game: 2 }), { game: 2 })
  expect(container.querySelectorAll('.field-mark'),
    'a new game starts with nothing marked').toHaveLength(0)
})

it('hands the stylesheet the length a mark is actually watched for', () => {
  // **Rendering the value audits it.** The animation's duration and the
  // element's lifetime are one number, and the only way they stay one number is
  // if the stylesheet is *told* rather than asked to remember. So this reads
  // what a browser would read, which is the question a test about a duration
  // can honestly ask without a layout engine.
  const read = (el: Element | null) => ({
    attacks: (el as HTMLElement | null)?.style.getPropertyValue('--mark-life-attacks'),
    blocks: (el as HTMLElement | null)?.style.getPropertyValue('--mark-life-blocks'),
    dies: (el as HTMLElement | null)?.style.getPropertyValue('--mark-life-dies'),
  })

  const watch = replay(null)
  expect(read(watch.container.querySelector('.field-stage')))
    .toEqual({ attacks: '1250ms', blocks: '1800ms', dies: '2000ms' })
  cleanup()

  // Fast is 150ms a beat, and the cap is what stops a skull sitting on a board
  // that moved on thirteen beats ago: five beats, floored so no pace can clip a
  // mark to a flicker.
  const skim = replay(null, { speed: 'fast' })
  expect(read(skim.container.querySelector('.field-stage')))
    .toEqual({ attacks: '750ms', blocks: '750ms', dies: '750ms' })
  cleanup()

  // Paused is not a slow pace, it is the absence of one: nothing is draining,
  // so there is nothing to cap a mark against and each keeps its full length.
  const still = replay(null, { speed: 'paused' })
  expect(read(still.container.querySelector('.field-stage')))
    .toEqual({ attacks: '1250ms', blocks: '1800ms', dies: '2000ms' })
})

it('lights the half of the table whose turn it is, and only that half', () => {
  // Forge has always said whose turn it is — its turn event names the seat and
  // `foldBoard` carries it — and the board never drew it. Both of COMBAT's
  // steps belong to seat 1, which is the far seat.
  const { container } = replay(null)
  const far = container.querySelector('.field-side-far')
  const near = container.querySelector('.field-side-near')

  expect(far?.className, 'seat one is on turn').toContain('is-active')
  expect(near?.className, 'and the other half is not').not.toContain('is-active')
  // The trench points at the same half. Two signals rather than one, because a
  // warm wash on sand is a soft thing to have to learn to read.
  expect(container.querySelector('.field')?.className).toContain('is-far-on')
  // Each half carries the light itself, so it can lie on the floor under the
  // cards rather than as a scrim over them.
  expect(far?.querySelector(':scope > .field-side-lit')).toBeTruthy()
  expect(near?.querySelector(':scope > .field-side-lit')).toBeTruthy()
  // And the whole fact, in words, for anyone who is not looking at the sand.
  expect(container.querySelector('.field-seam')?.textContent)
    .toContain('Arahbo — Cats is on turn')
})

it('lights neither half before the first turn has begun', () => {
  // `active` is zero until a turn event names a seat, and that is a true thing
  // to say about that moment rather than a hole to fill with a guess.
  const { container } = replay(null, { shown: 0 })
  expect(container.querySelectorAll('.field-side.is-active')).toHaveLength(0)
  expect(container.querySelector('.field')?.className)
    .not.toContain('-on')
  expect(container.querySelector('.field-seam')?.textContent)
    .not.toContain('is on turn')
})

/** A board where one seat ran a partner pair *and* a companion — the whole
 *  reason the command zone stopped being one pile.
 *
 *  Kaheera really does begin in the command zone beside the two commanders:
 *  Forge moves it there at setup, and that is exactly what made it
 *  indistinguishable from them. The server names it, so the room does not
 *  have to know the companion rules to draw it. */
const PAIRING: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'pair', name: 'Thrasios and Tymna', life: 40,
      commanders: [40, 41], companion: 42 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    { id: 40, name: 'Thrasios, Triton Hero',
      types: 'Legendary Creature - Merfolk Wizard', seat: 1 },
    { id: 41, name: 'Tymna the Weaver',
      types: 'Legendary Creature - Human Cleric', seat: 1 },
    { id: 42, name: 'Kaheera, the Orphanguard',
      types: 'Legendary Creature - Cat Beast', seat: 1 },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [
      { id: 40, zone: 'command', seat: 1 },
      { id: 41, zone: 'command', seat: 1 },
      { id: 42, zone: 'command', seat: 1 },
    ] },
    // Tymna goes to the sand; Kaheera is bought into a hand for {3}.
    //
    // `casts` is the server's count of the times a card has left the command
    // zone, and the tax is read off it. The browser used to count the same
    // transition itself — which is why Kaheera carries none here rather than
    // carrying a zero: a companion leaving the zone is not a cast, and the
    // side of the wire that reads the game is the side that knows it.
    { turn: 2, seat: 1, changes: [
      { id: 41, zone: 'battlefield', seat: 1, casts: 1 },
      { id: 42, zone: 'hand', seat: 1 },
    ] },
  ],
} as unknown as ForgeBoard

function pairing(shown: number) {
  return render(
    <MatchBoard board={PAIRING} shown={shown} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={2} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

it('gives a pairing two seats and the companion one of its own', () => {
  const railOf = (c: HTMLElement) =>
    c.querySelector('.field-zone-l-far') as HTMLElement
  const { container } = pairing(1)
  const rail = railOf(container)
  const seats = [...rail.querySelectorAll('.field-command .field-pile')]
  // Two thrones and a companion, and **nothing else**: a fourth tile would be
  // the old catch-all pile drawing the same three cards a second time.
  expect(seats, 'two commanders and a companion are three places')
    .toHaveLength(3)
  // Order is the deck's, which is the server's. Tymna has the higher id and
  // arrives second in the payload, so a room sorting by the board would put
  // her first and the same commander would change sides between games.
  expect(seats.map((s) => s.querySelector('.field-pile-label')?.textContent))
    .toEqual(['Thrasios', 'Tymna', 'Kaheera'])
  // The companion's seat is marked as one — it is in this zone and it is not
  // one of them.
  expect(seats[2]?.className).toContain('field-seat-companion')
  expect(seats[0]?.className).not.toContain('field-seat-companion')
  // A seat holds one named card, so it carries no count. "1" beside a
  // commander is a number nobody asked for.
  expect(rail.querySelectorAll('.field-command .field-pile-n')).toHaveLength(0)
  // Everybody is home, so no seat is vacant.
  expect(rail.querySelectorAll('.field-pile.is-vacant')).toHaveLength(0)
})

it('charges no commander tax for a companion leaving the command zone', () => {
  const railOf = (c: HTMLElement) =>
    c.querySelector('.field-zone-l-far') as HTMLElement
  const { container } = pairing(2)
  const rail = railOf(container)
  // **The bug this whole split exists to kill.** A companion sits in the
  // command zone and leaves it for a hand, and the board's old rule —
  // "nothing but a commander begins in the command zone" — read that as a
  // commander being cast. Kaheera has never been cast from anywhere and owes
  // nothing; Tymna went to the battlefield and owes two.
  expect(rail.querySelector('.field-tax')?.textContent?.trim()).toBe('+2')
  // Both empty seats are drawn, and they do not say the same thing: a chair
  // with nobody in it is a commander out on the sand, and a horn is a
  // companion already bought into a hand.
  const vacant = [...rail.querySelectorAll('.field-pile.is-vacant')]
  expect(vacant, 'Tymna is out and Kaheera has been called').toHaveLength(2)
  expect(vacant[0]?.className).not.toContain('field-seat-companion')
  expect(vacant[1]?.className).toContain('field-seat-companion')
})

/** A board thick with copies of one thing: six Forests, four Treasures and a
 *  pair of Bears, which is what a real board looks like by turn eight. */
const PILES: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'gyome', name: 'Gyome — Food', life: 40 },
    { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
  ],
  cards: [
    ...[1, 2, 3, 4, 5, 6].map((id) => ({ id, name: 'Forest',
      types: 'Basic Land - Forest', seat: 1,
      image: 'https://example.test/forest.jpg' })),
    ...[7, 8, 9, 10].map((id) => ({ id, name: 'Treasure',
      types: 'Artifact - Treasure', seat: 1, token: true,
      image: 'https://example.test/treasure.jpg' })),
    ...[11, 12].map((id) => ({ id, name: 'Grizzly Bears',
      types: 'Creature - Bear', seat: 1,
      image: 'https://example.test/bear.jpg' })),
    // The singleton, and it is load-bearing: without one card standing on its
    // own, "a lone card grows no pile" is a question asked of nothing.
    { id: 13, name: 'Llanowar Elves', types: 'Creature - Elf Druid', seat: 1,
      image: 'https://example.test/elves.jpg' },
  ],
  steps: [
    { turn: 8, seat: 1, changes: [
      ...[1, 2, 3, 4, 5, 6].map((id) => ({ id, zone: 'land', seat: 1 })),
      ...[7, 8, 9, 10].map((id) => ({ id, zone: 'battlefield', seat: 1 })),
      ...[11, 12].map((id) => ({ id, zone: 'battlefield', seat: 1,
        power: 2, toughness: 2 })),
      { id: 13, zone: 'battlefield', seat: 1, power: 1, toughness: 1 },
    ] },
  ],
} as unknown as ForgeBoard

function piles() {
  return render(
    <MatchBoard board={PILES} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

it('makes a stack out of the card it is a stack of', () => {
  const { container } = piles()

  const stacks = [...container.querySelectorAll(
    '.field-side-far .field-card.is-stacked')]
  // Six Forests, four Treasures, two Bears — three piles, not twelve cards.
  expect(stacks.map((s) => s.querySelector('.field-card-count')?.textContent))
    .toEqual(['6×', '4×', '2×'])

  // **The leaves wear the painting of the card they are leaves of.** They used
  // to be blank grey plates, which say *something is behind this* and nothing
  // about what — and what is behind it is the entire reason a stack exists.
  // Every card in a stack is identical in play by construction (`stackRow`),
  // so the same painting is a truthful picture of the next card down rather
  // than a stand-in for it.
  const forest = stacks[0] as HTMLElement
  const pile = forest.querySelector('.field-card-pile') as HTMLElement
  expect(pile.style.getPropertyValue('--leaf-art'))
    .toBe('url(https://example.test/forest.jpg)')

  // **The arc is fixed; the density is what grows.** Four or more gets four
  // leaves and a pair gets two, and both fans reach exactly as far — the
  // outermost leaf is at |--leaf| == 1 in both, which is the value the arena
  // wall was measured against. A fan that widened with the count would take
  // the leftmost stack in a row through the field's own clip; that was the
  // first draft, and it lost thirteen pixels of leaf to the wall.
  const leaves = (s: Element) => [...s.querySelectorAll('.field-card-leaf')]
    .map((l) => Number((l as HTMLElement).style.getPropertyValue('--leaf')))
  expect(leaves(forest)).toHaveLength(4)
  expect(leaves(stacks[1] as Element), 'four Treasures fan the same')
    .toHaveLength(4)
  expect(leaves(stacks[2] as Element), 'a pair gets two').toHaveLength(2)
  for (const s of stacks) {
    expect(Math.max(...leaves(s).map(Math.abs)),
      'no fan reaches further than another').toBe(1)
  }

  // A lone card is not a stack and grows nothing.
  const lone = container.querySelector(
    '.field-side-far .field-card:not(.is-stacked)')
  expect(lone?.getAttribute('title'), 'the Elf stands alone')
    .toContain('Llanowar Elves')
  expect(lone?.querySelector('.field-card-pile'),
    'one card has nothing behind it').toBeNull()
})

it('carries a stack’s count into the sheet a phone opens', () => {
  // **The fan is the pleasure and the count is the fact.** Hover does not
  // exist on a touch screen, and this project has shipped hover-only reading
  // affordances twice. Nothing about a stack is learnable only by fanning it:
  // the count chip draws at every width on every device, and a tap opens the
  // sheet — so the sheet is where "there are six of these" has to be sayable.
  const { container } = piles()
  tap(container.querySelector(
    '.field-side-far .field-card.is-stacked') as Element)
  expect(screen.getByRole('dialog', { name: '6 × Forest' })).toBeTruthy()
})

it('draws every card at one size, and never below it', () => {
  // **Two sizes, and the smaller one was unreadable.** Lands, artifacts,
  // enchantments and planeswalkers were 42x59 while creatures were 58x81 —
  // and this board draws the whole card face, not an art crop, so at
  // forty-two pixels the printed type became a grey vibration that reads as a
  // rendering fault (Aaron, 2026-08-26: "all cards should be at least the size
  // we have been using on creatures so the text doesn't look funny").
  //
  // Held as the absence of a class rather than as a measurement, because
  // jsdom has no layout engine and could not see a size if it tried. The
  // class is what carried the second size; nothing may reintroduce it.
  const { container } = piles()
  expect(container.querySelectorAll('.field-card').length).toBeGreaterThan(0)
  expect(container.querySelector('.field-card-small'),
    'the small card is retired; a land is drawn like a creature').toBeNull()
})

it('crowns the commander standing on the sand, and only there', () => {
  // The command zone can say a commander is *home*. Once it is cast it stands
  // in the creature row like any other body, and nothing said which of forty
  // permanents the whole deck was built around (Aaron, 2026-08-26).
  //
  // `pairing(2)` is the fixture for it: Tymna has gone to the battlefield,
  // Thrasios is still home, and Kaheera has been bought into a hand.
  const { container } = pairing(2)
  const crowned = [...container.querySelectorAll('.field-card.is-commander')]
  expect(crowned, 'Tymna is out; Thrasios is home in a seat').toHaveLength(1)
  expect(crowned[0]?.getAttribute('title')).toContain('Tymna the Weaver')
  // Said in words as well as drawn: a picture is a thing you have to already
  // know, and this room is for people playing their first game.
  expect(crowned[0]?.getAttribute('title')).toContain('the commander')
  expect(crowned[0]?.querySelector('.field-card-crown'),
    'the mark rides the card').toBeTruthy()
  // On the arm, which is what carries a card's corner furniture round when the
  // card leans. A crown pinned to the slot sits on the neighbour the moment
  // its card is tapped, and no rendered angle in jsdom would say so.
  expect(crowned[0]?.querySelector('.field-card-crown')
    ?.closest('.field-card-arm')).toBeTruthy()

  // **A companion is not a commander, and both are in the data.** Kaheera is
  // in a hand here, and a hand is a fan overlapped to a 27px strip — a mark
  // pinned there is drawn on the *next card's* painting. So the crowns cover
  // the battlefield and nothing else, which is the scope `FieldSide` provides
  // them at.
  expect(container.querySelectorAll('.field-card.is-companion'),
    'the companion is in a hand, and a hand wears no marks').toHaveLength(0)
  const hand = container.querySelector('.field-hand-far') as HTMLElement
  expect(hand.querySelectorAll('.field-card-crown'),
    'nothing in a hand is crowned').toHaveLength(0)
})

it('puts a hand under its own sign', () => {
  // Four plates on this board open four different things, and the hand's was
  // a deck name over a row of card backs with nothing to say what it held
  // (Aaron, 2026-08-26: "we need a hand icon to show that is what we are
  // showing people with the cards in hand"). Beside the label, never instead
  // of it — an icon-only control asks a newcomer to already know the app.
  const { container } = render(
    <MatchBoard board={ZONES} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
  const plate = container.querySelector(
    '.field-hand-far .field-hand-label') as HTMLElement
  expect(plate.querySelector('.field-hand-mark svg'),
    'the plate carries the fanned hand').toBeTruthy()
  expect(plate.textContent, 'and still says whose hand it is')
    .toContain('Arahbo')
  // The mark is decoration on a control that already names itself, so it is
  // hidden from anybody being read to rather than announced twice.
  expect(plate.querySelector('.field-hand-mark')
    ?.getAttribute('aria-hidden')).toBe('true')

  // It answers the hand that reaches for it (commandment 17): the fan opens
  // when the hand does. Driven through the real control, because a click is
  // what a tap, a mouse and the Enter key all produce.
  const angle = () =>
    plate.querySelector('.glyph-fan')?.getAttribute('transform')
  const shut = angle()
  fireEvent.click(plate)
  expect(angle(), 'the fan spreads while the hand is open').not.toBe(shut)
})

it('lets the command zone say only what a command zone can say', () => {
  // **Aaron, 2026-08-26:** *"hovering on the command zone pops up some things
  // I don't understand, like 'Olinda the Oblivious (99)'s effect? I don't get
  // that."* Two faults met there. A Forge EFFECT card was leaking past a
  // server-side filter — fixed where it is made — and the zone was drawn as a
  // *pile*, which answers "how many, what is on top", and neither question is
  // one anybody asks the command zone.
  //
  // His ruling fixes the vocabulary: *"at most it should just be two slots for
  // partners, one for a singular commander, or a second companion devoted slot
  // for Kaheera, et al. Those are the only combinations possible in that
  // zone."*
  const { container } = pairing(2)
  const rail = container.querySelector('.field-zone-l-far') as HTMLElement
  const seats = [...rail.querySelectorAll('.field-command .field-pile')]
  // Three places and no fourth: the catch-all pile that drew whatever else
  // the zone was holding is exactly the surface the effect card arrived on,
  // and with the seats named there is nothing left for it to say.
  expect(seats, 'two chairs and a companion, and nothing else').toHaveLength(3)
  expect(seats.map((s) => s.getAttribute('title'))).toEqual([
    'Thrasios, Triton Hero — the commander, waiting in the command zone',
    'Tymna the Weaver — the commander, out on the battlefield',
    'Kaheera, the Orphanguard — the companion, already called into hand',
  ])
  // None of them counts, and none of them names a top card. A seat holds one
  // named place; "1" beside a commander is a number nobody asked for and
  // "X's effect on top" is a sentence nobody can act on.
  for (const s of seats) {
    expect(s.getAttribute('title')).not.toContain('on top')
    expect(s.getAttribute('title')).not.toMatch(/: \d+$/)
  }
})

it('prices each chair rather than the whole zone', () => {
  // **The tax rode outside the group** — a red chip in the gap between the
  // command zone and the graveyard, belonging to neither (Aaron, 2026-08-26:
  // "it is outside the perimeter of the command zone and it makes for awkward
  // styling"). It is on the tile now, in the bottom-right corner a Magic card
  // keeps for the number saying what it is worth at this moment.
  //
  // And it is per chair, which one pile could never be. Thrasios has never
  // been cast and says nothing; Tymna has, and costs two more.
  const { container } = pairing(2)
  const rail = container.querySelector('.field-zone-l-far') as HTMLElement
  const seats = [...rail.querySelectorAll('.field-command .field-pile')]
  expect(seats.map((s) => s.querySelector('.field-tax')?.textContent?.trim()))
    .toEqual([undefined, '+2', undefined])
  // **Structurally out of the companion's reach**, which is the half that
  // matters. Kaheera has never been cast from anywhere and owes nothing; a
  // badge anchored to the group's own corner would have landed on her tile,
  // and this board has told that lie once already.
  expect(seats[2]?.querySelector('.field-tax'),
    'a companion is never taxed').toBeNull()
  // Inside the tile it prices, rather than beside the zone.
  expect(rail.querySelector('.field-tax')?.closest('.field-pile'))
    .toBe(seats[1])
})

/** A board of turned mana sources, and one turned for something else.
 *
 * **What the board may say about a turned permanent and what it must never
 * say.** That a card taps for green is a fact about the *printing*, true
 * whether it was turned to pay for something, to swing, or by an opponent —
 * and it is said in the card's own words. That *this* activation filled
 * somebody's pool is not said anywhere and could not be: the mana event
 * carries a seat and a pool, the tap event carries a card, and no key joins
 * the two (ADR 44). */
const TAPPED: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'green', name: 'Green', life: 40 },
    { seat: 2, slug: 'other', name: 'Other', life: 40 },
  ],
  cards: [
    { id: 40, name: 'Llanowar Elves', types: 'Creature - Elf Druid', seat: 1,
      mana: true, makes: ['G'] },
    { id: 41, name: 'Temple Garden', types: 'Land - Forest Plains', seat: 1,
      mana: true, makes: ['G', 'W'] },
    { id: 42, name: 'Birds of Paradise', types: 'Creature - Bird', seat: 1,
      mana: true, makes: ['B', 'G', 'R', 'U', 'W'] },
    { id: 43, name: 'Forest', types: 'Basic Land - Forest', seat: 1,
      mana: true, makes: ['G'] },
    { id: 44, name: 'Craterhoof Behemoth', types: 'Creature - Beast', seat: 1 },
    // Held, not played: a card in a hand is not turned for anything.
    { id: 45, name: 'Birds of Paradise', types: 'Creature - Bird', seat: 1,
      mana: true, makes: ['B', 'G', 'R', 'U', 'W'] },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [
      { id: 40, zone: 'battlefield', seat: 1, tapped: true, power: 1,
        toughness: 1 },
      { id: 41, zone: 'land', seat: 1, tapped: true },
      { id: 42, zone: 'battlefield', seat: 1, tapped: true, power: 0,
        toughness: 1 },
      // Standing untapped, and taps for mana all the same.
      { id: 43, zone: 'land', seat: 1 },
      // Turned, and taps for nothing.
      { id: 44, zone: 'battlefield', seat: 1, tapped: true, power: 5,
        toughness: 5 },
      { id: 45, zone: 'hand', seat: 1 },
    ] },
  ],
} as unknown as ForgeBoard

function tapped() {
  return render(
    <MatchBoard board={TAPPED} shown={1} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={1} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

/** The one card on the sand with this name. */
function sand(container: HTMLElement, name: string) {
  return [...container.querySelectorAll('.field-row .field-card')]
    .find((c) => c.getAttribute('title')?.startsWith(name)) as HTMLElement
}

it('says what a turned permanent taps for without drawing it on the card', () => {
  const { container } = tapped()

  // **The bead is gone and the sentence is not.** #337 drew a mana mark on the
  // right edge of every turned mana source; #341 took it off, because the same
  // fact is now told bigger in the middle of the arena at the moment the mana
  // arrives — and because a bead sitting beside a live mana pool starts
  // implying *this made that*, which is the one inference ADR 44 refuses.
  expect(container.querySelectorAll('.field-card-bead'),
    'no card wears a mana mark any more').toHaveLength(0)

  // What the bead was *for* survives in words, which is where it was always
  // the more useful of the two for anybody not looking at a fifteen-pixel
  // picture. **"taps for", never "made"** — the second is a claim about this
  // activation and no such claim exists anywhere on this wire.
  const elves = sand(container, 'Llanowar Elves')
  expect(elves, 'the fixture put the Elves on the sand').toBeTruthy()
  expect(elves.getAttribute('title')).toContain('taps for green mana')
  expect(elves.getAttribute('title')).not.toContain('made')

  // A turned permanent that taps for nothing still says nothing. Craterhoof is
  // sideways because it swung, and the board has never suggested otherwise.
  expect(sand(container, 'Craterhoof Behemoth').getAttribute('title'))
    .not.toContain('taps for')

  // **Untapped is untapped**, even on a Forest: the sentence is about what a
  // *turned* source is doing, and a standing one is doing nothing.
  expect(sand(container, 'Forest').getAttribute('title'))
    .not.toContain('taps for')
})

/**
 * A seat whose pool fills and drains inside one beat.
 *
 * **This is the shape the real wire has, copied off a recorded match** rather
 * than the tidier one anybody would invent. Two things about it are the whole
 * reason the room draws a movement instead of a value:
 *
 * - Forge raises **no beat at all** for a mana ability — it does not use the
 *   stack — so every tap between two sentences arrives attached to whichever
 *   beat comes next, which is very often the cast that spent it.
 * - Forge **taps one land and spends that mana before tapping the next**, so
 *   the sequence is `G, '', G, '', G, ''` and not `G, GG, GGG`. The pool is
 *   never holding more than one mana, which is why `raised` is what arrived
 *   across the beat and not the pool's high-water mark: the high-water mark
 *   here is one, and one pip flickering three times says nothing.
 *
 * Three green are raised and all three are spent. Seat 1 is `sides[0]`, the
 * **far** player; seat 2 does nothing at all, which is the other half of the
 * test — a pool that did not move must not be animated, and a seat with no
 * mana must still have somewhere to put it.
 */
const POOLS: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'green', name: 'Green', life: 40 },
    { seat: 2, slug: 'other', name: 'Other', life: 40 },
  ],
  cards: [
    { id: 60, name: 'Forest', types: 'Basic Land - Forest', seat: 1,
      mana: true, makes: ['G'] },
    { id: 61, name: 'Llanowar Elves', types: 'Creature - Elf Druid', seat: 1,
      mana: true, makes: ['G'] },
  ],
  steps: [
    { turn: 1, seat: 1, changes: [
      { id: 60, zone: 'land', seat: 1 },
      { id: 61, zone: 'battlefield', seat: 1, power: 1, toughness: 1 },
    ] },
    { turn: 1, seat: 1, changes: [
      { id: 60, zone: 'land', seat: 1, tapped: true },
      { id: 61, zone: 'battlefield', seat: 1, tapped: true, power: 1,
        toughness: 1 },
    ], floating: [
      { seat: 1, pool: 'G' }, { seat: 1, pool: '' },
      { seat: 1, pool: 'G' }, { seat: 1, pool: '' },
      { seat: 1, pool: 'G' }, { seat: 1, pool: '' },
    ] },
  ],
} as unknown as ForgeBoard

function pools(speed: Speed = 'play') {
  return render(
    <MatchBoard board={POOLS} shown={2} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed={speed} setSpeed={vi.fn()} of={2} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

/** The pips standing in a seat's pool, and the ones on their way out of it. */
function poolRow(container: HTMLElement, facing: 'far' | 'near') {
  const pool = container
    .querySelector(`.field-hand-${facing} .field-pool`) as HTMLElement
  return {
    pool,
    held: pool.querySelectorAll('.field-pool-pip:not(.is-spent)'),
    spent: pool.querySelectorAll('.field-pool-pip.is-spent'),
  }
}

it('fills the pool beside a hand and then drains it', () => {
  vi.useFakeTimers()
  try {
    const { container } = pools()

    // **It fills to what was raised, not to what the pool held at any one
    // instant.** Forge never had more than one green in the pool at a time,
    // and three is the number that answers "what paid for that". Rendered on
    // the first commit rather than from an effect, so the mana and the beat
    // that spent it reach the screen together.
    expect(poolRow(container, 'far').held,
      'three mana were raised across the beat').toHaveLength(3)
    expect(poolRow(container, 'far').spent).toHaveLength(0)

    // **And then it drains, which is the ask.** All three go — and they are
    // drawn on their way out rather than simply deleted, because watching it
    // deplete is the entire point (Aaron, 2026-08-26).
    act(() => { vi.advanceTimersByTime(400) })
    expect(poolRow(container, 'far').held,
      'nothing is left floating').toHaveLength(0)
    expect(poolRow(container, 'far').spent,
      'the three that were spent are drawn leaving').toHaveLength(3)

    // ...and then they are gone, and nothing is stranded. Mana sitting beside
    // a hand that the game no longer has is the room lying about the one
    // number a player is counting.
    act(() => { vi.advanceTimersByTime(1200) })
    expect(poolRow(container, 'far').held).toHaveLength(0)
    expect(poolRow(container, 'far').spent,
      'nothing is left draining forever').toHaveLength(0)
  } finally {
    vi.useRealTimers()
  }
})

it('gives a seat with no mana somewhere to put it', () => {
  const { container } = pools()

  // **The trough is drawn whether or not there is anything in it.** A row that
  // appeared and vanished fifty times a match would shove the hand up and down
  // the screen all game, and somebody at their first game needs to know where
  // mana lives before there is any to see (commandment 2).
  const other = poolRow(container, 'near')
  expect(other.pool, 'the empty seat still has a pool').toBeTruthy()
  expect(other.held, 'and nothing in it').toHaveLength(0)
  expect(other.spent).toHaveLength(0)

  // Said in words, because the pips are a drawing and a drawing is a thing you
  // have to already know how to read. **The words are the resting truth, not
  // the animation's current frame** — the far player raised three green and
  // spent all three, so what a screen reader is told about their pool is that
  // it is empty, which is the only thing about it that was ever a fact about
  // the game. `poolSaid` is tested on its own for the filled spellings.
  expect(other.pool.getAttribute('title')).toBe('Mana pool: empty')
  expect(poolRow(container, 'far').pool.getAttribute('title'))
    .toBe('Mana pool: empty')
})

it('flashes the mana that arrived, in the half it arrived for', () => {
  const { container } = pools()

  // The ask, in Aaron's own words (2026-08-26): *"I wanted the mana symbol to
  // maybe just show in the middle like the cast cards do now."*
  const flash = container.querySelector('.stage-mana') as HTMLElement
  expect(flash, 'the mana that arrived is drawn on the sand').toBeTruthy()
  expect(flash.querySelectorAll('.stage-mana-pip'),
    'three arrived, so three are flashed — not the one that was left')
    .toHaveLength(3)

  // **In the gaining seat's own half**, which is what lets it share a beat
  // with the card it paid for instead of fighting it for the middle. Seat 1 is
  // the far player.
  expect(flash.classList.contains('is-far')).toBe(true)
  expect(container.querySelectorAll('.stage-mana.is-near'),
    'the seat that gained nothing flashes nothing').toHaveLength(0)

  // It says nothing to a screen reader and takes no pointer: the words are
  // already in the play-by-play, and a flash that swallowed a drag would break
  // the timeline at the moment it is most wanted.
  expect(flash.getAttribute('aria-hidden')).toBe('true')
})

/* ------------------------------------------------- poison, in the trench
 *
 * **The slot beside the life dial held its width and drew nothing** until the
 * scribe subscribed to `GameEventPlayerCounters`. Two things are worth a gate
 * now that it draws: that an absent counter still renders as *absence*, and
 * that a present one reaches the plate at all.
 *
 * The first is the one that matters. A `0` would be a claim — it would say
 * this game has poison in it — and nearly every game has none, so nearly every
 * plate must draw life alone. That is a property no screenshot will ever catch
 * failing, because the failure looks like a board with an extra dial on it.
 */
const POISONED: ForgeBoard = {
  seats: [
    { seat: 1, slug: 'skithiryx', name: 'Skithiryx — Infect', life: 40 },
    { seat: 2, slug: 'mono-green', name: 'Mono-Green Fixture', life: 40 },
  ],
  cards: [{ id: 30, name: 'Plague Stinger', types: 'Creature - Insect', seat: 1 }],
  steps: [
    { turn: 1, seat: 1, changes: [{ id: 30, zone: 'battlefield', seat: 1 }] },
    // The numbers a real Skithiryx match produced: totals, not arrivals.
    { turn: 2, seat: 1, counters: [{ seat: 2, counters: [{ kind: 'Poison', n: 4 }] }] },
    { turn: 3, seat: 1, counters: [{ seat: 2, counters: [{ kind: 'Poison', n: 12 }] }] },
  ],
} as unknown as ForgeBoard

function poisoned(shown: number) {
  return render(
    <MatchBoard board={POISONED} shown={shown} game={1} running={false}
                name={(_slug, fallback) => fallback}
                speed="play" setSpeed={vi.fn()} of={3} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

it('draws no bead at all in a game with no poison in it', () => {
  const { container } = poisoned(1)
  expect(container.querySelectorAll('.field-bead')).toHaveLength(0)
})

it('puts the count on the poisoned seat and nowhere else', () => {
  const { container } = poisoned(2)
  const beads = container.querySelectorAll('.field-bead')
  expect(beads).toHaveLength(1)
  expect(beads[0]?.textContent).toBe('4')
  expect(beads[0]?.className).toContain('is-poison')
  // Four of the ten that kill, so not lethal yet — the ring is a fraction and
  // the class is the threshold.
  expect(beads[0]?.className).not.toContain('is-lethal')
})

it('says so when the tenth counter has landed', () => {
  // Twelve, because Skithiryx swings for four and a player really does go
  // straight past ten. The arc clamps; the class and the figure do not.
  const { container } = poisoned(3)
  const bead = container.querySelector('.field-bead')
  expect(bead?.textContent).toBe('12')
  expect(bead?.className).toContain('is-lethal')
  expect(bead?.getAttribute('title')).toContain('ten is lethal')
})
