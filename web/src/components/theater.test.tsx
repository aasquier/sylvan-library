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
import { MatchTheater } from './theater'

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

function stage(rows: ForgeGameRow[], over: { games?: number; running?: boolean } = {}) {
  return render(
    <MatchTheater a={deck()} b={opponent} aSlug="arahbo-cats"
                  bSlug="goreclaw-stompy" games={over.games ?? 8} rows={rows}
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
      <MatchTheater a={deck({ name: 'The Cat Deck' })} b={opponent}
                    aSlug="arahbo-cats" bSlug="goreclaw-stompy" games={4}
                    rows={[row()]} running />,
    )
    const lines = [...container.querySelectorAll('.theater-commander')]
    expect(lines).toHaveLength(1)
    expect(lines[0]?.textContent).toBe('Arahbo, Roar of the World')
  })

  it('seats a deck the shelf has not handed it yet, under its slug', () => {
    render(
      <MatchTheater a={null} b={null} aSlug="arahbo-cats"
                    bSlug="goreclaw-stompy" games={2} rows={[]} running />,
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
