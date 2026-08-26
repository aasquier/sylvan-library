/**
 * The two shared primitives that speak for every waiting surface in the app.
 *
 * Commandment 2's "shut out" includes shut out by a screen reader, and until
 * this pair carried a role the app had **no live region anywhere**: not one
 * `aria-live`, `role="status"`, `role="alert"` or `aria-busy` in `web/src`,
 * across thirty-odd surfaces where an answer arrives after an await. A sighted
 * person sees a ring start turning and sees the answer replace it; somebody
 * using a reader got silence, then silence.
 *
 * `Spinner` and `ErrorNote` are where the fix belongs because they are the
 * whole app's waiting and the whole app's refusal — twenty and twenty-three
 * call sites — so the roles are stated once and reach all of them. These tests
 * hold that: an announcement is not a thing you can see in a screenshot, so if
 * a refactor drops the role nothing else in the suite would ever notice.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { CardHover, CardSheet, ErrorNote, Spinner } from './ui'

afterEach(cleanup)

it('says what it is waiting for, in a region a reader is watching', () => {
  render(<Spinner label="Reading the colour guide…" />)
  // Found by role, not by class: what matters is the thing a screen reader
  // walks, and `status` is what makes the label an announcement rather than
  // text that happens to appear.
  expect(screen.getByRole('status').textContent).toBe('Reading the colour guide…')
})

it('announces nothing when there is nothing to announce', () => {
  render(<Spinner />)
  // A wait with no words is still a live region -- it just has nothing to say,
  // which is correct. The assertion that matters is that it does not invent
  // something: an unlabelled spinner must not read out its own markup.
  expect(screen.getByRole('status').textContent).toBe('')
})

it('keeps the turning ring out of the reader\'s way', () => {
  render(<Spinner label="Gathering the readers…" />)
  const ring = document.querySelector('.animate-spin')
  expect(ring).not.toBeNull()
  // The ring is the picture of the wait; the label is the wait. Without this
  // the region has a child element with no accessible name inside it, which is
  // noise in the one place noise costs the most.
  expect(ring?.getAttribute('aria-hidden')).toBe('true')
})

it('interrupts for a refusal, because a failure that waits its turn is a wait', () => {
  render(<ErrorNote>The reading could not be dealt.</ErrorNote>)
  // `alert` rather than `status`: everywhere else polite is right, and here it
  // is not -- somebody is still waiting for an answer that is never coming.
  expect(screen.getByRole('alert').textContent).toBe('The reading could not be dealt.')
})

/** A tap, the way a phone makes one. jsdom has no PointerEvent of its own, so
 *  the type rides on a plain Event — which is all the handler reads. */
function tap(el: Element) {
  for (const type of ['pointerdown', 'pointerup']) {
    const ev = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(ev, 'pointerType', { value: 'touch' })
    fireEvent(el, ev)
  }
}

const CARD = { name: 'Jareth, Leonine Titan', image: 'https://example.test/j.jpg' }

it('hands the whole card to a tap, for the half of the room with no cursor', () => {
  // `CardHover` was `onMouseEnter` and nothing else, which meant that on a
  // phone the card behind a 96x64 crop was unreachable in twenty-three places.
  // A hover-only mechanism locks out every touch user, and here it was the
  // only answer to "what is this card" — the newcomer's first question.
  render(<CardHover card={CARD}><span>tile</span></CardHover>)
  expect(screen.queryByRole('dialog')).toBeNull()

  tap(screen.getByText('tile'))

  const sheet = screen.getByRole('dialog', { name: CARD.name })
  expect(sheet).toBeTruthy()
  expect(screen.getByAltText(CARD.name).getAttribute('src')).toBe(CARD.image)
})

it('leaves the tap alone where the tile is already a control', () => {
  // Four of the twenty-three wrap a real button — the commander picker on the
  // create flow, and the reading room's tiles. There a tap already means
  // *choose this*, and stealing it would break the newcomer's most important
  // decision to show them a picture they never asked for.
  const chosen: string[] = []
  render(
    <CardHover card={CARD} tapOpens={false}>
      <button type="button" onClick={() => chosen.push(CARD.name)}>pick</button>
    </CardHover>)

  tap(screen.getByRole('button', { name: 'pick' }))
  expect(screen.queryByRole('dialog')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: 'pick' }))
  expect(chosen).toEqual([CARD.name])
})

it('closes the held card on a tap anywhere and on Escape', () => {
  render(<CardHover card={CARD}><span>tile</span></CardHover>)

  tap(screen.getByText('tile'))
  fireEvent.click(screen.getByRole('dialog'))
  expect(screen.queryByRole('dialog')).toBeNull()

  tap(screen.getByText('tile'))
  fireEvent.keyDown(window, { key: 'Escape' })
  expect(screen.queryByRole('dialog')).toBeNull()
})

/* ── The sheet's carousel ─────────────────────────────────────────────────
 *
 * A creature on the arena floor wears its Equipment and Auras as corners
 * tucked under it, which says *that* it is carrying something and never
 * *what* — and the tuck's own answer is a hover, so on a phone the question
 * had nowhere to go. The sheet now riffles the whole assemblage.
 *
 * jsdom has no layout engine, so none of this can see the carousel *move*:
 * the settle, the turned neighbours and the brass are verified in a real
 * browser and nowhere else. What these hold is the part a stylesheet cannot
 * carry and a screenshot cannot prove — which card is at the front, that
 * three separate hands can change it, and that a lone card grows nothing.
 */

const SWORD = { name: 'Bonesplitter', image: 'https://example.test/b.jpg' }
const AURA = { name: 'Rancor', image: 'https://example.test/r.jpg' }

/** The card the carousel is showing, read the way the component decides it
 *  rather than by looking for a transform jsdom will never compute. */
function front() {
  return document.querySelector('.card-sheet-slide.is-front img')
    ?.getAttribute('alt')
}

/** A finger crossing the sheet. Negative goes left, which advances — the card
 *  follows the finger, the way every carousel a person has ever touched
 *  does. A real gesture ends in a click, and the click is the whole reason
 *  this helper fires one: without it the sheet closes mid-riffle. */
function swipe(el: Element, dx: number) {
  for (const [type, x] of [['pointerdown', 200], ['pointerup', 200 + dx]] as const) {
    const ev = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(ev, 'clientX', { value: x })
    Object.defineProperty(ev, 'clientY', { value: 300 })
    fireEvent(el, ev)
  }
  fireEvent.click(el)
}

it('opens the creature first and brings everything on it along', () => {
  // A player opened the Bear, not the Bonesplitter. The order is the order
  // the board holds — the host, then what went on it, in the order it went.
  render(<CardSheet name={CARD.name} image={CARD.image}
                    worn={[SWORD, AURA]} onClose={() => {}} />)

  expect(front()).toBe(CARD.name)
  expect(document.querySelectorAll('.card-sheet-slide')).toHaveLength(3)
  // The dialog is still named for the card that was opened. The rest is what
  // is *inside* it, and the caption says which one aloud on every change.
  expect(screen.getByRole('dialog', { name: CARD.name })).toBeTruthy()
})

it('changes card on a swipe, on the arrow keys, and on a click', () => {
  // **Three hands, because a swipe is not a control.** This project has
  // shipped a touch-only reading affordance twice and lost half the audience
  // both times; a carousel a laptop cannot work is that bug in a third
  // costume. Held together in one test because what matters is that all
  // three land on the same card, not that each fires.
  render(<CardSheet name={CARD.name} image={CARD.image}
                    worn={[SWORD, AURA]} onClose={() => {}} />)
  const sheet = screen.getByRole('dialog')

  swipe(sheet, -120)
  expect(front()).toBe(SWORD.name)
  swipe(sheet, 120)
  expect(front()).toBe(CARD.name)

  fireEvent.keyDown(window, { key: 'ArrowRight' })
  expect(front()).toBe(SWORD.name)
  fireEvent.keyDown(window, { key: 'ArrowLeft' })
  expect(front()).toBe(CARD.name)

  fireEvent.click(screen.getByRole('button', { name: 'The card after this one' }))
  expect(front()).toBe(SWORD.name)
  // And straight to a card, which is what the pips are for.
  fireEvent.click(screen.getByRole('button', { name: AURA.name }))
  expect(front()).toBe(AURA.name)
})

it('does not close the sheet on the swipe that changed the card', () => {
  // A swipe ends in a click and a click on this sheet closes it, so without
  // the gesture being remembered the sheet would change card and vanish in
  // the same movement. A tap that merely drifted still closes.
  let open = true
  render(<CardSheet name={CARD.name} image={CARD.image}
                    worn={[SWORD]} onClose={() => { open = false }} />)
  const sheet = screen.getByRole('dialog')

  swipe(sheet, -120)
  expect(open).toBe(true)
  expect(front()).toBe(SWORD.name)

  swipe(sheet, -4)
  expect(open).toBe(false)
})

it('says where you are, and stops at both ends rather than wrapping', () => {
  // Two of five with no marker is a maze. The pips are the marker and they
  // are buttons, so the answer to "where am I" is also the way to leave.
  render(<CardSheet name={CARD.name} image={CARD.image}
                    worn={[SWORD, AURA]} onClose={() => {}} />)
  const back = screen.getByRole('button', { name: 'The card before this one' })
  const on = screen.getByRole('button', { name: 'The card after this one' })

  expect(document.querySelectorAll('.card-sheet-pip')).toHaveLength(3)
  expect(screen.getByRole('button', { name: CARD.name })
    .getAttribute('aria-current')).toBe('true')
  // An ordered assemblage, not a ring: a dead end you can see is kinder to a
  // newcomer than a loop that silently returns them to the creature.
  expect(back.hasAttribute('disabled')).toBe(true)

  fireEvent.click(on)
  fireEvent.click(on)
  expect(front()).toBe(AURA.name)
  expect(on.hasAttribute('disabled')).toBe(true)
  expect(back.hasAttribute('disabled')).toBe(false)
  // The card is named in words as well as shown, and the quiet line says
  // whose sword this is — the sentence a newcomer needs most.
  expect(screen.getByText(`attached to ${CARD.name}`)).toBeTruthy()
})

it('leaves a card with nothing on it exactly as it was', () => {
  // One card is still one card. No rail, no pips, no caption and no "1 of 1"
  // — which is also what keeps the twenty-three `CardHover` call sites from
  // growing furniture none of them asked for.
  render(<CardSheet name={CARD.name} image={CARD.image} onClose={() => {}} />)

  expect(screen.getByAltText(CARD.name).getAttribute('src')).toBe(CARD.image)
  expect(document.querySelector('.card-sheet-rail')).toBeNull()
  expect(document.querySelectorAll('.card-sheet-pip')).toHaveLength(0)
  expect(document.querySelector('.card-sheet-say')).toBeNull()
  expect(screen.queryAllByRole('button')).toHaveLength(0)
})

it('drops an attachment the pool gave no painting rather than drawing a blank', () => {
  // The board's own rule: no painting opens no sheet. A blank slide in the
  // middle of a riffle is a rendering fault wearing a card's shape — and one
  // paintless attachment must not turn a lone creature into a "1 of 2".
  render(<CardSheet name={CARD.name} image={CARD.image}
                    worn={[{ name: 'Umbral Mantle', image: '' }]}
                    onClose={() => {}} />)

  expect(document.querySelector('.card-sheet-rail')).toBeNull()
  expect(screen.getByAltText(CARD.name)).toBeTruthy()
})
