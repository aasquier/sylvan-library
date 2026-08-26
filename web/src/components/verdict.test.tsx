/**
 * Who the verdict crowns, and what it does when nobody won.
 *
 * The panel is mostly picture, and jsdom cannot see a picture — it has no
 * layout engine, so nothing here can claim the wreath sits over the name or
 * that either object is visible at all. That was verified in a browser.
 *
 * What *is* logic, and is worth holding, is the decision underneath: the
 * standing is sorted rather than scanned for a maximum, because the second
 * place is needed too; and a tie at the top is a drawn match rather than an
 * arbitrary winner. A `find`-the-highest-wins implementation passes every
 * eyeball test and quietly crowns whichever deck the server happened to list
 * first on a 6-6 split, which is the bug this file exists to refuse.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'

import type { ForgeResult } from '../lib/api'
import { MatchVerdict } from './verdict'

afterEach(cleanup)

function result(wins: number[]): ForgeResult {
  const names = ['Gyome, Master Chef', 'Trostani, Selesnya\'s Voice']
  return {
    decks: wins.map((w, i) => ({
      slug: `deck-${i}`, name: names[i] ?? `Deck ${i}`,
      address: `a-${i}`, wins: w,
    })),
    games: 12, played: wins.reduce((a, b) => a + b, 0), draws: 0,
    timed_out: 0, median_seconds: 90, max_seconds: 200,
    startup_seconds: 12, wall_seconds: 400, clock: 300, seed: 7,
    rows: [], caveat: 'Forge is bad at combo.', beats: [],
  }
}

it('crowns the deck with the most wins and names the other as fallen', () => {
  render(<MatchVerdict result={result([7, 5])} />)
  expect(screen.getByText('Takes the wreath')).toBeTruthy()
  expect(screen.getByText('Leaves the sand')).toBeTruthy()
  expect(screen.getByText('Gyome, Master Chef')).toBeTruthy()
  expect(screen.getByText('Trostani, Selesnya\'s Voice')).toBeTruthy()
})

it('crowns the winner even when the server lists it second', () => {
  // The standing is sorted, not read in order. Listing the loser first is the
  // arrangement that catches a `decks[0]` implementation.
  render(<MatchVerdict result={result([4, 9])} />)
  const crowned = screen.getByText('Takes the wreath').parentElement
  expect(crowned?.textContent).toContain('Trostani')
  expect(crowned?.textContent).not.toContain('Gyome')
})

it('crowns nobody on a tie, and puts no deck on the sand', () => {
  render(<MatchVerdict result={result([6, 6])} />)
  expect(screen.getByText('Unclaimed')).toBeTruthy()
  expect(screen.queryByText('Takes the wreath')).toBeNull()
  // The half that would name a loser is not rendered at all: a drawn match has
  // no fallen deck, and inventing one is the room taking a view the games did
  // not support.
  expect(screen.queryByText('Leaves the sand')).toBeNull()
  expect(screen.queryByText('Gyome, Master Chef')).toBeNull()
})

it('goes away when the sand is cleared, and does not come back on its own', () => {
  render(<MatchVerdict result={result([7, 5])} />)
  fireEvent.click(screen.getByRole('button', { name: 'Clear the sand' }))
  expect(screen.queryByText('Takes the wreath')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Clear the sand' })).toBeNull()
})

it('renders nothing rather than crashing when no deck answered', () => {
  const { container } = render(<MatchVerdict result={result([])} />)
  expect(container.textContent).toBe('')
})
