/**
 * Tier 1.5 on screen, and the four properties a green backend suite cannot see.
 *
 * - a colour is rendered as a **ladder**, so a deck short for one greedy card
 *   does not read as a deck short of that colour (commandment 2, and the
 *   whole reason `PipTier` exists);
 * - the heatmap prints its numbers, so the colour wash is never the only
 *   carrier of a value;
 * - the verdict words come from the server's `flat`, never from the client
 *   re-deciding it off `gain`;
 * - a card past the horizon says so instead of reading as "never", because
 *   `on_curve: null` means "not asked" rather than "no".
 *
 * Assertions match this file's own strings rather than the payload's, per
 * Simulator.test.tsx: a test that asserts the server's text back at itself is
 * not testing the renderer.
 */

import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  DeckCheck, ManaCurve, PolicyResult, ShelfResult,
} from '../lib/api'
import { resetGlossaryCache } from '../lib/glossary'
import { heatPercent } from '../lib/heat'
import {
  ClosedForm, DeckVerdict, ManaCurvePanel, PolicyReport,
} from './closedform'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { glossary: vi.fn() } }
})

const { api } = await import('../lib/api')

// The tooltips are not what these tests are about, but `HelpTip` fetches the
// glossary on mount and an unresolved mock rejects inside an effect. Empty is
// enough: a missing term renders as plain text, which is the same DOM these
// assertions read.
beforeEach(() => {
  resetGlossaryCache()
  vi.mocked(api.glossary).mockResolvedValue({ sections: [], terms: [] })
})

function shelfOf(over: Partial<ShelfResult> = {}): ShelfResult {
  return {
    slug: 'goreclaw', deck_name: 'Goreclaw Stompy', deck_size: 99, lands: 34,
    target: 0.9, on_the_play: true, horizon: 10,
    colors: [{
      color: 'G', have: 37, have_lands: 30, met: false, shortfall: 11,
      tiers: [
        { pips: 1, turn: 1, need: 27, have: 37, met: true, shortfall: 0,
          odds_now: 0.967, cards: ['Arbor Elf'], card_count: 32 },
        { pips: 3, turn: 3, need: 48, have: 37, met: false, shortfall: 11,
          odds_now: 0.727, cards: ['Chord of Calling'], card_count: 9 },
      ],
    }],
    lands_estimate: {
      lands_now: 34, recommended: 41, delta: 7, average_mana_value: 3.83,
      cheap_accelerants: 7,
      caveats: ['fitted to 60-card tournament decks and scaled'],
    },
    cards: [
      { name: 'Chord of Calling', mv: 3, on_curve: 0.44, reliable_turn: null,
        lag: null, by_turn: [0, 0, 0.44, 0.6, 0.7, 0.8, 0.85, 0.88, 0.9, 0.92] },
      { name: 'Llanowar Elves', mv: 1, on_curve: 0.97, reliable_turn: 1,
        lag: 0, by_turn: [0.97, 1, 1, 1, 1, 1, 1, 1, 1, 1] },
    ],
    approximated: [],
    caveat: 'the closed form assumes the card is in your hand',
    ...over,
  }
}

function policyOf(over: Partial<PolicyResult> = {}): PolicyResult {
  const best = {
    min_lands: 1, max_lands: 5, min_pieces: 2,
    describe: 'keep 1-5 lands AND lands + ramp(mv<=2) >= 2',
    spells_through_t8: 9.52, mulligan_rate: 0.131, avg_mulligans: 0.15,
    median_commander_turn: 7, color_screw_rate: 0.4, stalled_turns: 1.1,
  }
  const baseline = {
    min_lands: 2, max_lands: 5, min_pieces: 3,
    describe: 'keep 2-5 lands AND lands + ramp(mv<=2) >= 3',
    spells_through_t8: 9.17, mulligan_rate: 0.411, avg_mulligans: 0.52,
    median_commander_turn: 7, color_screw_rate: 0.4, stalled_turns: 1.2,
  }
  return {
    slug: 'tivit', deck_name: 'Tivit cEDH', games: 2000, turns: 10, seed: 7,
    rows: [best, baseline], best, baseline, gentlest: best,
    spread: 1.86, gain: 0.35, flat: false,
    caveat: 'judged on spells deployed through turn 8',
    cached: false, computed_at: null,
    ...over,
  }
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the colour ladder', () => {
  it('reports each symbol count separately rather than one verdict per colour', () => {
    render(<ClosedForm shelf={shelfOf()} />)
    // The single-symbol rung passes while the triple fails, and both are on
    // screen: a deck that reads only "green: short" tells a beginner their
    // mana base is broken when one card is greedy.
    expect(screen.getByText(/1 symbol on T1/)).toBeTruthy()
    expect(screen.getByText(/3 symbols on T3/)).toBeTruthy()
    expect(screen.getByText('covered')).toBeTruthy()
    expect(screen.getByText('short 11')).toBeTruthy()
  })

  it('names the cards making a demand, so the reader knows what to blame', () => {
    render(<ClosedForm shelf={shelfOf()} />)
    // Twice over: once as the card setting the triple-symbol rung, once as a
    // row of the heatmap. Both are wanted, so this asserts presence and not
    // uniqueness.
    expect(screen.getAllByText(/Chord of Calling/).length).toBeGreaterThan(0)
  })

  it('counts the unlisted cards against what it actually showed', () => {
    // The rung names one card out of 32, so the remainder is 31 -- not 29.
    // Subtracting a constant three reports the wrong number whenever the
    // server sent fewer names than the cap, which it does for most rungs.
    render(<ClosedForm shelf={shelfOf()} />)
    expect(screen.getByText(/Arbor Elf and 31 more/)).toBeTruthy()
  })
})

describe('the castability heatmap', () => {
  it('prints a number in every cell, so colour is never the only signal', () => {
    render(<ClosedForm shelf={shelfOf()} />)
    const row = screen.getByText('Llanowar Elves').closest('tr')
    expect(row).toBeTruthy()
    // Ten turn columns, each carrying its own digits rather than a bare wash.
    const cells = within(row as HTMLElement).getAllByText(/^\d+$|^·$/)
    expect(cells.length).toBeGreaterThanOrEqual(10)
  })

  it('says "never" for a card inside the horizon that never becomes reliable', () => {
    render(<ClosedForm shelf={shelfOf()} />)
    const row = screen.getByText('Chord of Calling').closest('tr')
    expect(within(row as HTMLElement).getByText('never')).toBeTruthy()
  })

  it('does not claim a card past the horizon is uncastable', () => {
    // `on_curve: null` means the shelf was not asked about turn 12, which is
    // a different thing from a zero, and the row must not read as "never".
    const shelf = shelfOf({
      cards: [{
        name: 'Ghalta, Primal Hunger', mv: 12, on_curve: null,
        reliable_turn: null, lag: null, by_turn: Array(10).fill(0),
      }],
    })
    render(<ClosedForm shelf={shelf} />)
    expect(screen.queryByText('never')).toBeNull()
  })
})

describe('the land estimate', () => {
  it('renders the caveats rather than the bare number', () => {
    render(<ClosedForm shelf={shelfOf()} />)
    expect(screen.getByText(/fitted to 60-card tournament decks/)).toBeTruthy()
  })

  it('points at the land sweep, which prices what the formula cannot', () => {
    render(<ClosedForm shelf={shelfOf()} />)
    expect(screen.getByText(/prices/)).toBeTruthy()
  })
})

describe('the policy verdict', () => {
  it('recommends a rule when the server says the sweep is not flat', () => {
    render(<PolicyReport policy={policyOf()} />)
    expect(screen.getAllByText(/keep 1-5 lands/).length).toBeGreaterThan(0)
    expect(screen.queryByText(/already right/)).toBeNull()
  })

  it('withholds the recommendation when the server says flat', () => {
    render(<PolicyReport policy={policyOf({ flat: true, gain: 0.01 })} />)
    expect(screen.getByText(/already right/)).toBeTruthy()
  })

  it('reads `flat` from the server rather than deciding it from `gain`', () => {
    // A large gain with `flat: true` is not a state the server produces, and
    // that is the point: if the client ever recomputed the verdict from
    // `gain`, this would render the recommendation and fail. The rule lives
    // on the server, where it is measured against the default rather than the
    // grid's range.
    render(<PolicyReport policy={policyOf({ flat: true, gain: 5 })} />)
    expect(screen.getByText(/already right/)).toBeTruthy()
  })

  it('surfaces a gentler rule that deploys the same but keeps more hands', () => {
    const policy = policyOf({
      flat: true, gain: 0.04,
      gentlest: {
        min_lands: 2, max_lands: 6, min_pieces: 2,
        describe: 'keep 2-6 lands AND lands + ramp(mv<=2) >= 2',
        spells_through_t8: 9.16, mulligan_rate: 0.211, avg_mulligans: 0.24,
        median_commander_turn: 7, color_screw_rate: 0.4, stalled_turns: 1.2,
      },
    })
    render(<PolicyReport policy={policy} />)
    expect(screen.getByText(/Still worth knowing/)).toBeTruthy()
    expect(screen.getByText(/fewer hands thrown away/)).toBeTruthy()
  })

  it('stays quiet about a gentler rule that is barely gentler', () => {
    // The default already mulligans 41%; a rule at 39% is not news, and a
    // panel that always finds something to say trains people to ignore it.
    const policy = policyOf({
      flat: true, gain: 0.02,
      gentlest: { ...policyOf().baseline, mulligan_rate: 0.39 },
    })
    render(<PolicyReport policy={policy} />)
    expect(screen.queryByText(/Still worth knowing/)).toBeNull()
  })
})

describe('the cell percentage', () => {
  it('reads 90 exactly when the odds have actually reached the 90% bar', () => {
    // The invariant that keeps the grid from contradicting itself. The lag
    // column calls a card reliable at 0.90, so a cell holding 0.8996 must not
    // print "90" beside a lag of "never" — which is what rounding did, and
    // what a real Goreclaw run showed on screen before this was floored.
    expect(heatPercent(0.8996)).toBe('89')
    expect(heatPercent(0.9)).toBe('90')
    expect(heatPercent(0.9004)).toBe('90')
  })

  it('renders a vanishing chance as a dot rather than a zero', () => {
    // A column of zeroes reads as data; a column of dots reads as "not yet",
    // which is what a turn before the card's own mana value actually is.
    expect(heatPercent(0)).toBe('·')
    expect(heatPercent(1)).toBe('100')
  })
})

function checkOf(over: Partial<DeckCheck> = {}): DeckCheck {
  return {
    ok: true, error_count: 0, warning_count: 0, errors: [],
    affects_numbers: false, unresolved: [], unresolved_count: 0,
    commander_unresolved: false, declared_size: 99, simulated_size: 99,
    ...over,
  }
}

describe('the deck verdict', () => {
  it('says nothing at all about a clean deck', () => {
    const { container } = render(<DeckVerdict check={checkOf()} />)
    expect(container.firstChild).toBeNull()
  })

  it('says nothing when the server sent no verdict', () => {
    const { container } = render(<DeckVerdict check={undefined} />)
    expect(container.firstChild).toBeNull()
  })

  it('warns that the figures describe the deck as written when a card is banned', () => {
    render(<DeckVerdict check={checkOf({
      ok: false, error_count: 1, affects_numbers: true,
      errors: [{ code: 'banned', message: 'not legal in Commander',
                 card: 'Primeval Titan' }],
    })} />)
    expect(screen.getByText(/not a deck you can legally play/)).toBeTruthy()
    expect(screen.getByText(/Primeval Titan/)).toBeTruthy()
  })

  it('says a paperwork failure leaves the figures alone', () => {
    // A missing rationale blocks a curated deck and has nothing to do with
    // mana. Telling somebody their numbers are suspect because of it would
    // train them to ignore the banner that matters.
    render(<DeckVerdict check={checkOf({
      ok: false, error_count: 1, affects_numbers: false,
      errors: [{ code: 'missing-rationale', message: 'no why', card: 'Forest' }],
    })} />)
    expect(screen.getByText(/paperwork rather than its mana/)).toBeTruthy()
    expect(screen.queryByText(/legally play/)).toBeNull()
  })

  it('leads with the size when cards were dropped, because that is worse', () => {
    // An unresolved card is not merely a legality complaint: it shrinks the
    // population every probability was computed over, so the figures are
    // about a different deck rather than about an illegal one.
    render(<DeckVerdict check={checkOf({
      ok: false, error_count: 3, affects_numbers: true,
      unresolved: ['Berserk', 'Rancor'], unresolved_count: 2,
      declared_size: 99, simulated_size: 97,
      errors: [{ code: 'unknown-card', message: 'not in the pool',
                 card: 'Berserk' }],
    })} />)
    expect(screen.getByText(/97 of 99 cards were simulated/)).toBeTruthy()
    expect(screen.getByText(/Berserk, Rancor/)).toBeTruthy()
    expect(screen.getByText(/about that deck rather than about yours/)).toBeTruthy()
  })

  it('mentions a commander that did not resolve', () => {
    render(<DeckVerdict check={checkOf({
      ok: false, error_count: 1, affects_numbers: true,
      commander_unresolved: true, declared_size: 99, simulated_size: 99,
    })} />)
    expect(screen.getByText(/commander did not resolve/)).toBeTruthy()
  })

  it('caps the error list and says how many it left out', () => {
    render(<DeckVerdict check={checkOf({
      ok: false, error_count: 20, affects_numbers: true,
      errors: [{ code: 'banned', message: 'nope', card: 'A' }],
    })} />)
    expect(screen.getByText(/and 19 more/)).toBeTruthy()
  })
})

function curveOf(over: Partial<ManaCurve['advice']> = {},
                 top: Partial<ManaCurve> = {}): ManaCurve {
  return {
    deck_size: 99, lands: 34, accelerants: 12,
    target_turn: 4, target_mana: 4, target: 0.9,
    turns: Array.from({ length: 10 }, (_, i) => ({
      turn: i + 1, from_lands: i + 1, from_ramp: i * 0.3,
      expected_mana: i + 1 + i * 0.3,
      land_drop_odds: 0.9 - i * 0.08, odds: 0.85 - i * 0.05,
    })),
    advice: {
      target_turn: 4, target_mana: 4, odds: 0.71,
      odds_per_land: 0.73, odds_per_ramp: 0.73,
      recommend: 'either', slots: 13, ramp_is_generic: false,
      beyond_the_curve: false, lands_for_every_drop: 54,
      ...over,
    },
    caveat: 'these odds assume you keep your opening seven',
    ...top,
  }
}

describe('the mana curve', () => {
  it('says add ramp, in words, when the server says ramp', () => {
    render(<ManaCurvePanel curve={curveOf({ recommend: 'ramp',
                                            beyond_the_curve: true })} />)
    expect(screen.getByText('Add ramp, not lands.')).toBeTruthy()
  })

  it('explains why a land cannot help past the curve', () => {
    // The rule the whole feature teaches: you may play one land a turn, so a
    // fifth land does nothing on turn four. Left unsaid, the recommendation
    // reads as an opinion instead of arithmetic.
    render(<ManaCurvePanel curve={curveOf({ recommend: 'ramp',
                                            target_mana: 6,
                                            beyond_the_curve: true })} />)
    expect(screen.getByText(/more mana than the turn number/)).toBeTruthy()
    expect(screen.getByText(/fifth land does nothing/)).toBeTruthy()
  })

  it('stays quiet about the cap when the target is on curve', () => {
    render(<ManaCurvePanel curve={curveOf({ beyond_the_curve: false })} />)
    expect(screen.queryByText(/more mana than the turn number/)).toBeNull()
  })

  it('reports a tie as a tie rather than picking one', () => {
    render(<ManaCurvePanel curve={curveOf({ recommend: 'either' })} />)
    expect(screen.getByText(/they buy the same thing here/)).toBeTruthy()
  })

  it('congratulates rather than advising when the target is already met', () => {
    render(<ManaCurvePanel curve={curveOf({ recommend: 'none', slots: 0 })} />)
    expect(screen.getByText(/often enough/)).toBeTruthy()
  })

  it('says a target is unreachable instead of printing a slot count', () => {
    render(<ManaCurvePanel curve={curveOf({ recommend: 'ramp', slots: null })} />)
    expect(screen.getByText(/a target to move, not a deck to fix/)).toBeTruthy()
  })

  it('admits when the ramp comparison is a stand-in', () => {
    render(<ManaCurvePanel curve={curveOf({ ramp_is_generic: true })} />)
    expect(screen.getByText(/stand-in/)).toBeTruthy()
  })

  it('shows the absurd land count, because that is the point of it', () => {
    // 54 lands is the answer to "a land drop every turn", and the panel
    // carries it to talk the reader out of the question rather than to
    // recommend it.
    render(<ManaCurvePanel curve={curveOf()} />)
    expect(screen.getByText(/54 lands/)).toBeTruthy()
    expect(screen.getByText(/not the question worth asking/)).toBeTruthy()
  })
})
