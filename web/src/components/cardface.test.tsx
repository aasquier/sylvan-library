/**
 * The turn, and — more importantly — which cards are offered one.
 *
 * Two names on a card is not two paintings, and the difference is the whole
 * behaviour here: a modal double-faced card has a second picture to turn to,
 * an Adventure has two names printed on one. Offering the turn on the second
 * would be the interface claiming a painting that does not exist, so the
 * absence is as much the contract as the presence and is tested as one.
 *
 * The third case is the one that would be quietest in the wild: three arrays
 * that disagree. The server sends names, paintings and crops index-aligned,
 * and a browser that trusted the lengths would index confidently into the
 * wrong painting rather than failing — so the plate stands down instead.
 *
 * And the click is checked for where it does *not* go. These plates sit inside
 * the 99's rows, which carry their own click and key handlers whenever the
 * action bar is armed; a turn that also entombed the card would be a real bug
 * with a very quiet cause.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import type { Card } from '../lib/api'
import { CardFacePlate } from './cardface'

afterEach(cleanup)

const FRONT = 'https://cards.scryfall.io/art_crop/front/7/e/hengegate.jpg'
const BACK = 'https://cards.scryfall.io/art_crop/back/7/e/mistgate.jpg'

function card(extra: Partial<Card> = {}): Card {
  return {
    name: 'Hengegate Pathway // Mistgate Pathway',
    category: 'lands', why: 'It taps for either half.', qty: 1, known: true,
    art_crop: FRONT,
    image: 'https://cards.scryfall.io/normal/front/7/e/hengegate.jpg',
    ...extra,
  }
}

const PAINTED_TWICE: Partial<Card> = {
  faces: ['Hengegate Pathway', 'Mistgate Pathway'],
  face_images: ['https://cards.scryfall.io/normal/front/7/e/hengegate.jpg',
    'https://cards.scryfall.io/normal/back/7/e/mistgate.jpg'],
  face_art_crops: [FRONT, BACK],
}

const draw = (c: Card) =>
  render(<MemoryRouter><CardFacePlate card={c} /></MemoryRouter>)

it('turns a card that is painted twice, and says where it is going', () => {
  draw(card(PAINTED_TWICE))

  expect(screen.getByRole('img').getAttribute('src')).toBe(FRONT)
  const turn = screen.getByRole('button',
    { name: 'Turn Hengegate Pathway over to Mistgate Pathway' })

  fireEvent.click(turn)

  expect(screen.getByRole('img').getAttribute('src')).toBe(BACK)
  expect(screen.getByRole('img').getAttribute('alt')).toBe('Mistgate Pathway')
  // The sentence turns with it, so the control never offers a face you are
  // already looking at.
  screen.getByRole('button',
    { name: 'Turn Mistgate Pathway over to Hengegate Pathway' })
})

it('turns back, rather than only ever forward', () => {
  draw(card(PAINTED_TWICE))
  const turn = () => screen.getByRole('button', { name: /^Turn / })
  fireEvent.click(turn())
  fireEvent.click(turn())
  expect(screen.getByRole('img').getAttribute('src')).toBe(FRONT)
})

it('offers no turn on a card whose two names share one painting', () => {
  // An Adventure. The server sends no faces for it precisely so that this
  // question never has to be asked in the browser.
  draw(card({ name: 'Giant Killer // Chop Down' }))
  expect(screen.queryByRole('button', { name: /^Turn / })).toBeNull()
})

it('stands down when the three arrays disagree', () => {
  draw(card({
    faces: ['Hengegate Pathway', 'Mistgate Pathway'],
    face_images: ['a', 'b'],
    face_art_crops: [FRONT],
  }))
  expect(screen.queryByRole('button', { name: /^Turn / })).toBeNull()
})

it('does not let the turn reach the row behind it', () => {
  const rowClicked = vi.fn()
  render(
    <MemoryRouter>
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions */}
      <li onClick={rowClicked}>
        <CardFacePlate card={card(PAINTED_TWICE)} />
      </li>
    </MemoryRouter>,
  )

  fireEvent.click(screen.getByRole('button', { name: /^Turn / }))

  expect(screen.getByRole('img').getAttribute('src')).toBe(BACK)
  expect(rowClicked).not.toHaveBeenCalled()
})
