/**
 * The match theater, and the properties the stage has to hold in both of its
 * phases:
 *
 * - **Wins are counted from the rows.** The finished result carries its own
 *   tally and this deliberately does not read it, so the check that matters
 *   is that the count here matches the ledger's rule: a clocked-out game
 *   lights nobody's pip, and neither does a draw.
 * - **The feed is the last few games, newest first** — it is a live view, not
 *   the record, and the full table renders below it.
 * - **The gauge is a progressbar to a screen reader.** The heat is how it
 *   looks; the numbers are what it means.
 * - **A commander line that only repeats the deck's name is not printed.**
 */

import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { DeckSummary, ForgeGameRow } from '../lib/api'
import { MatchTheater, type TheaterSeat } from './theater'

afterEach(cleanup)

function deck(over: Partial<DeckSummary> = {}): DeckSummary {
  return {
    slug: 'arahbo-cats', owner: 'local', shared: false, pilot: '',
    name: 'Arahbo, Roar of the World — Cats', status: 'built',
    stage: 'curated', writable: true, needs_rationale: 0,
    commander: ['Arahbo, Roar of the World'], companion: null, bracket: 3,
    total_cards: 100, land_count: 36, strategy: '',
    art_crop: 'https://cards.scryfall.io/art_crop/front/a/a/arahbo.jpg',
    color_identity: ['G', 'W'],
    ...over,
  } as DeckSummary
}

function row(over: Partial<ForgeGameRow> = {}): ForgeGameRow {
  return {
    game: 1, winner: 'arahbo-cats', seconds: 6.2, turns: 9, draw: false,
    timed_out: false, ...over,
  }
}

const opponent = deck({
  slug: 'goreclaw-stompy', name: 'Goreclaw, Terror of Qal Sisma — Mono-Green Stompy',
  commander: ['Goreclaw, Terror of Qal Sisma'],
})

function stage(rows: ForgeGameRow[],
  over: { games?: number; running?: boolean; seats?: TheaterSeat[] } = {}) {
  return render(
    <MatchTheater
      seats={over.seats ?? [{ deck: deck(), slug: 'arahbo-cats' },
                            { deck: opponent, slug: 'goreclaw-stompy' }]}
      games={over.games ?? 8} rows={rows}
      running={over.running ?? true} />,
  )
}

describe('the stage', () => {
  it('counts a win for the deck that took the game', () => {
    const { container } = stage([
      row({ game: 1 }), row({ game: 2, winner: 'goreclaw-stompy' }),
      row({ game: 3 }),
    ])
    // Two for Arahbo, one for Goreclaw — read off the panels rather than off
    // any tally the caller passed, because there is no such tally. Scoped to
    // the scores: a bare `getByText('2')` also matches the game numbered 2 in
    // the feed, which is how this test first passed for the wrong reason.
    const scores = [...container.querySelectorAll('.theater-score')]
      .map((el) => el.textContent?.trim().split(' ')[0])
    expect(scores).toEqual(['2', '1'])
  })

  // The ledger's own rule (`_shape`): a game called off at the clock counts
  // for neither seat, and neither does a real draw. The stage must not be a
  // second opinion about that.
  it('lights nobody for a clock-out or a draw', () => {
    const { container } = stage([
      row({ game: 1, winner: null, timed_out: true }),
      row({ game: 2, winner: null, draw: true }),
    ])
    expect(container.querySelectorAll('.theater-pip-lit')).toHaveLength(0)
    expect(screen.getByText('called off at the clock')).toBeTruthy()
    expect(screen.getByText('a draw')).toBeTruthy()
  })

  it('gives each side one pip per game in the match', () => {
    const { container } = stage([row()], { games: 5 })
    // Five each, ten in total: the track is the length of the match, not of
    // what has been played.
    expect(container.querySelectorAll('.theater-pip')).toHaveLength(10)
    expect(container.querySelectorAll('.theater-pip-lit')).toHaveLength(1)
  })

  it('shows the last six games, newest first', () => {
    const { container } = stage(
      Array.from({ length: 9 }, (_, i) => row({ game: i + 1 })), { games: 9 })
    const rows = [...container.querySelectorAll('.theater-row')]
    expect(rows).toHaveLength(6)
    const first = rows[0]
    const last = rows[5]
    expect(first && within(first as HTMLElement).getByText('9')).toBeTruthy()
    expect(last && within(last as HTMLElement).getByText('4')).toBeTruthy()
  })

  it('reports its progress as a progressbar, not only as heat', () => {
    stage([row(), row({ game: 2 })], { games: 8 })
    const bar = screen.getByRole('progressbar')
    expect(bar.getAttribute('aria-valuenow')).toBe('2')
    expect(bar.getAttribute('aria-valuemax')).toBe('8')
  })

  it('says the forge is being lit, without promising when', () => {
    stage([])
    expect(screen.getByText(/forge is being lit/i)).toBeTruthy()
    // **No estimate this wait can overrun.** The copy used to promise the
    // first game "within half a minute", which was true of a forge already
    // burning and false of the deployed one — it lights from cold, and the
    // promise expired long before anything happened. A wait that outlives its
    // own estimate reads as a broken page, so the words say "a minute or two,
    // longer when cold" and nothing tighter.
    expect(screen.queryByText(/half a minute/i)).toBeNull()
  })

  // **The wait has a clock in it once there is something to clock.** The
  // stage says how far in a match is; this is the half that says how much
  // longer, which is the question somebody deciding whether to wait is
  // actually asking.
  it('says how much longer, once two games have been fought', () => {
    stage([row({ game: 1, seconds: 45 }), row({ game: 2, seconds: 45 })],
      { games: 8 })
    // Six to come at 45s each: 4m 30s. Said as a count of fights and a
    // stretch of time, never as a percentage or a raw second count.
    expect(screen.getByText(/6 more games to fight/)).toBeTruthy()
    expect(screen.getByText(/about 4m 30s/)).toBeTruthy()
  })

  it('names one remaining game in the singular', () => {
    stage([row({ game: 1, seconds: 30 }), row({ game: 2, seconds: 30 })],
      { games: 3 })
    expect(screen.getByText(/One more game to fight/)).toBeTruthy()
  })

  // The estimate is a guide and the sentence says so. The copy above it lost
  // a "within half a minute" for being a promise the deployed arena could not
  // keep, and an estimate that does not admit what moves it is the same
  // mistake one layer up.
  it('calls the figure a guide rather than a promise', () => {
    stage([row({ game: 1, seconds: 45 }), row({ game: 2, seconds: 45 })],
      { games: 8 })
    expect(screen.getByText(/a guide and not a promise/)).toBeTruthy()
    // And it is audible: somebody who cannot see the bar is told each time a
    // game lands, which is the one piece of news this wait carries.
    expect(screen.getByRole('status').textContent)
      .toMatch(/6 more games to fight/)
  })

  // A sample of one is the first game of the match, and a first game is the
  // slowest of them because the forge lights from cold — so an estimate drawn
  // from it would promise a wait about double the truth. The room falls back
  // to its general words instead of guessing.
  it('promises nothing from a single game', () => {
    stage([row({ game: 1, seconds: 180 })], { games: 10 })
    expect(screen.queryByText(/more games to fight/)).toBeNull()
    expect(screen.getByText(/a typical game takes a few seconds/)).toBeTruthy()
  })

  it('stops counting down when the match is over', () => {
    stage([row({ game: 1, seconds: 45 }), row({ game: 2, seconds: 45 })],
      { games: 8, running: false })
    expect(screen.queryByText(/more games to fight/)).toBeNull()
  })

  it('never flips a painting to make the two face each other', () => {
    const { container } = stage([row()])
    for (const art of container.querySelectorAll('.theater-art')) {
      // The face-off is two gradients and a mirrored layout. A transform on
      // the image itself would be the shortcut, and it would be somebody's
      // painting printed backwards.
      expect((art as HTMLElement).style.transform).toBeFalsy()
    }
  })

  it('does not repeat the deck name as its commander line', () => {
    const { container } = stage([row()])
    // Both decks here are named for their commanders, so neither earns one.
    expect(container.querySelectorAll('.theater-commander')).toHaveLength(0)
  })

  it('prints a commander line when the deck is not named for them', () => {
    const { container } = render(
      <MatchTheater
        seats={[{ deck: deck({ name: 'The Cat Deck' }), slug: 'arahbo-cats' },
                { deck: opponent, slug: 'goreclaw-stompy' }]}
        games={4} rows={[row()]} running />,
    )
    const lines = [...container.querySelectorAll('.theater-commander')]
    expect(lines).toHaveLength(1)
    expect(lines[0]?.textContent).toBe('Arahbo, Roar of the World')
  })

  it('seats a deck the shelf has not handed it yet, under its slug', () => {
    render(
      <MatchTheater
        seats={[{ deck: null, slug: 'arahbo-cats' },
                { deck: null, slug: 'goreclaw-stompy' }]}
        games={2} rows={[]} running />,
    )
    expect(screen.getByText('arahbo-cats')).toBeTruthy()
    expect(screen.getByText('goreclaw-stompy')).toBeTruthy()
  })

  // The forge breathes only while it is being worked. Finished metal glows
  // and holds still, which is also what stops an idle page animating forever.
  it('only breathes while the match is running', () => {
    const { container: live } = stage([row()], { running: true })
    expect(live.querySelector('.theater-gauge-lit')).toBeTruthy()
    cleanup()
    const { container: done } = stage([row()], { running: false })
    expect(done.querySelector('.theater-gauge-lit')).toBeFalsy()
  })
})

// A pod seats four, and the theater showed two: the stage was three columns —
// champion, anvil, challenger — so seats three and four had nowhere to go
// (Aaron, 2026-09-06: "Our preview is only two tiles on four player too").
describe('a four-player match', () => {
  const pod: TheaterSeat[] = [
    { deck: deck(), slug: 'arahbo-cats' },
    { deck: opponent, slug: 'goreclaw-stompy' },
    { deck: null, slug: 'atla-palani-dinos' },
    { deck: null, slug: 'gyome-food' },
  ]

  it('seats all four, not the first two', () => {
    const { container } = stage([], { seats: pod })
    expect(container.querySelectorAll('.theater-champion')).toHaveLength(4)
    // A seat the shelf has not handed over yet is named by its slug, which is
    // what it was submitted under — so the two unnamed chairs prove the third
    // and fourth seats are really rendered rather than merely counted.
    expect(screen.getByText('atla-palani-dinos')).toBeTruthy()
    expect(screen.getByText('gyome-food')).toBeTruthy()
  })

  it('lays the four out as a table rather than a duel', () => {
    const { container } = stage([], { seats: pod })
    expect(container.querySelector('.theater-stage')?.className)
      .toContain('is-pod')
  })

  it('leaves the duel stage alone', () => {
    const { container } = stage([])
    expect(container.querySelectorAll('.theater-champion')).toHaveLength(2)
    expect(container.querySelector('.theater-stage')?.className)
      .not.toContain('is-pod')
  })
})
