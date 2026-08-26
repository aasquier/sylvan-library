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

  // And the sand still carries neither hand. A seat's rows are the five a
  // player sorts a table into — lands and mana, enchantments, artifacts,
  // planeswalkers, creatures — and the ones holding nothing are not drawn at
  // all, so this fixture (a Lion and a Forest) shows exactly two. That is the
  // property: no row is a hand, and no row is invented.
  const known = ['Lands and mana', 'Enchantments', 'Artifacts',
    'Planeswalkers', 'Creatures']
  for (const side of container.querySelectorAll('.field-side')) {
    const labels = [...side.querySelectorAll('.field-row')]
      .map((r) => (r.getAttribute('aria-label') ?? '').replace(/: \d+$/, ''))
    expect(labels.some((l) => l.startsWith('Hand')),
      'a hand is not a row on the battlefield').toBe(false)
    expect(labels.every((l) => known.includes(l)),
      `every row is one of the five: ${labels}`).toBe(true)
    expect(labels).toContain('Creatures')
    expect(labels).toContain('Lands and mana')
    expect(labels, 'a row with nothing in it is not drawn')
      .not.toContain('Planeswalkers')
  }

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
    { turn: 2, seat: 1, changes: [{ id: 30, zone: 'battlefield', seat: 1 }] },
    { turn: 3, seat: 1, changes: [{ id: 30, zone: 'command', seat: 1 }] },
    // **A beat with nothing in it, and it is load-bearing.** A card that
    // leaves the battlefield is held standing there for exactly the beat that
    // says so, so the step above is the commander being *seen to die* rather
    // than the commander being home. It is home on the next beat, and that is
    // the one the questions below are asking about.
    { turn: 3, seat: 1, changes: [] },
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
                speed="play" setSpeed={vi.fn()} of={6} seek={vi.fn()}
                games={[1]} playing={1} chooseGame={vi.fn()} />)
}

it('seats an empty throne when the commander is out, and prices the return', () => {
  // Home after one cast: a card in the zone, and two generic on the next one.
  // Seat one's own rail: seat two never gets a commander in this fixture, so
  // its zone is legitimately an empty chair the whole way through and would
  // answer either question the wrong way.
  const railOf = (c: HTMLElement) =>
    c.querySelector('.field-rail-far') as HTMLElement

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
  const grave = container.querySelector('.field-rail-far .field-pile-wrap:has('
    + '[aria-label^="Graveyard"]) .field-tray') as HTMLElement
  expect(grave, 'the graveyard has a tray').toBeTruthy()
  expect(grave.getAttribute('aria-label')).toBe('Graveyard, all 2')
  expect(grave.querySelectorAll('.field-card')).toHaveLength(2)

  // An empty zone gets no tray at all rather than an empty one: there is
  // nothing to look through, and a panel that opens onto nothing is a worse
  // answer than a pile that stays shut.
  const exileWrap = container.querySelector(
    '.field-rail-near .field-pile-wrap:has([aria-label^="Exile"])')
  expect(exileWrap?.querySelector('.field-tray'),
    "the near seat's exile is empty, so it does not open").toBeNull()
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

  const flier = container.querySelector('.field-rows .field-card') as HTMLElement
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
    const ring = container.querySelector('.field-rail-far .field-life') as HTMLElement
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

it('stands the zones off the sand, beside the arena rather than on it', () => {
  // **The property, and it is the one that was got wrong twice.** The zones
  // were briefly a column *inside* the field, which stood them on the
  // battlefield's own sand and took their width out of it — so the arena came
  // out scaled awkwardly (Aaron, 2026-08-25). A graveyard is not somewhere you
  // stand; it is somewhere cards go, and it belongs off the floor.
  //
  // Held as containment rather than as geometry: the grid places these
  // differently at each breakpoint, and "is it inside the arena" is the thing
  // neither branch can quietly get wrong.
  const { container } = show()
  const field = container.querySelector('.field') as HTMLElement
  expect(field).toBeTruthy()
  expect(field.querySelector('.field-rail'),
    'the zones are not on the sand').toBeNull()
  expect(container.querySelector('.field-stage > .field-zones'),
    'they stand beside the arena, in the stage').toBeTruthy()

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
  const names = [...container.querySelectorAll('.field-rail-far .field-pile-label')]
    .map((n) => n.textContent)
  expect(names).toEqual(['Command Zone', 'Graveyard', 'Exile'])
})

it('heads each seat\'s zones with whose they are', () => {
  // The name used to float on a grey strip under the rows, and it came off
  // with the strip. It is back for a different reason: the zones are a band
  // under the arena holding *both* players side by side, and a band holding
  // two players has to say which half is whose. A label on a bar and a heading
  // over the thing it names are not the same object.
  const { container } = show()
  const heads = [...container.querySelectorAll('.field-zones .field-rail-name')]
    .map((n) => n.textContent)
  expect(heads).toHaveLength(2)
  expect(heads[0]).toContain('Arahbo')
  expect(heads[1]).toContain('Atla')

  // Each heading sits inside its own seat's panel, not loose in the band.
  expect(container.querySelector('.field-rail-far .field-rail-name')
    ?.textContent).toContain('Arahbo')
  expect(container.querySelector('.field-rail-near .field-rail-name')
    ?.textContent).toContain('Atla')
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
    `.field-rail-far .field-pile[aria-label^="${label}"]`)
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
  expect(container.querySelector('.field-rows .field-card.is-dies '
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
    '.field-rail-far .field-pile-wrap [aria-label^="Graveyard"]')
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
    ?.closest('.field-rows'), 'held on the beat it fell').toBeTruthy()
  cleanup()

  // One beat earlier it was alive and the graveyard was empty.
  const before = combat(null, 1)
  expect(before.container.querySelector(
    '.field-rail-far .field-pile-wrap [aria-label^="Graveyard"]')
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
    { turn: 2, seat: 1, changes: [
      { id: 41, zone: 'battlefield', seat: 1 },
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
    c.querySelector('.field-rail-far') as HTMLElement
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
    c.querySelector('.field-rail-far') as HTMLElement
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
