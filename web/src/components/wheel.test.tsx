/**
 * The wheel says where it stopped.
 *
 * Everything this toy does while it turns is a picture — the disc, the pawl,
 * the clicker, the storm, the rim's travelling light — and every one of them
 * is `aria-hidden`, correctly, because a turning wheel is nothing at all to
 * describe. The *landing* is the answer, and until now it was as silent as the
 * theatre: the fate, its meaning and the card it turned up simply appeared in
 * a panel nobody was told to look at. Green's 08-24 pass gave the app its
 * waits and its refusals out loud and deliberately left arrivals for a pass
 * with the wording written on purpose; this is one of the four surfaces it
 * named, and the wheel is the one where the whole point is a result.
 *
 * The theatre is skipped here two ways, both of them real paths rather than
 * conveniences: `transitionend` on the disc is what fires the reveal in a
 * browser, and the reduced-motion branch is what somebody with that setting
 * actually gets. Neither invents a shortcut this component does not have.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { WheelSpin } from '../lib/api'
import { WheelOfFortune } from './wheel'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { wheelSpin: vi.fn() } }
})

// The clearing's rain, the pawl and the fate's own voice all reach for an
// AudioContext jsdom does not have. They are switched off by default anyway —
// `soundOn` is the preference and this spread keeps it, so the module still
// behaves like itself; only the four that would build an audio graph are
// silenced, and independently of whatever preference the suite inherits.
vi.mock('../lib/tablesounds', async (importOriginal) => ({
  ...await importOriginal<typeof import('../lib/tablesounds')>(),
  fateLand: vi.fn(), swampStart: vi.fn(), swampStop: vi.fn(),
  thunderRoll: vi.fn(), wheelTurn: vi.fn(),
}))

const { api } = await import('../lib/api')

function landing(over: Partial<WheelSpin> = {}): WheelSpin {
  return {
    pool_available: true,
    symbol: 'cup',
    label: 'The Cup',
    meaning: 'The cup runneth over — a card that refills your hand.',
    seed: 7,
    caveat: 'Dice, not judgement.',
    card: {
      name: 'Rhystic Study', mana_cost: '{2}{U}', type_line: 'Enchantment',
      oracle_text: 'Whenever an opponent casts a spell…',
      color_identity: ['U'], image: null, art_crop: null,
    },
    ...over,
  } as WheelSpin
}

/** The live region, which stands whether or not the wheel has stopped. */
function said(): string {
  return document.querySelector('[role="status"].sr-only')?.textContent ?? ''
}

/** Unfold the clearing and start a spin. */
async function spin() {
  render(<WheelOfFortune deckRef={{ owner: 'local', slug: 'goreclaw' }} />)
  fireEvent.click(screen.getByTitle('Unfold the Wheel of Fortune'))
  fireEvent.click(await screen.findByText('Spin the wheel'))
}

/** What the browser does when the disc has finished decelerating. */
function stop() {
  fireEvent.transitionEnd(document.querySelector('.wheel-disc')!)
}

beforeEach(() => {
  vi.mocked(api.wheelSpin).mockResolvedValue(landing())
  // jsdom has none, and the component optional-chains it — an explicit stub
  // so each test says which branch it is on rather than inheriting one.
  vi.stubGlobal('matchMedia', (media: string) => ({ matches: false, media }))
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('the wheel, said out loud', () => {
  it('has its live region up while the wheel is still turning', async () => {
    await spin()
    await waitFor(() => expect(screen.getByText('The wheel turns…')).toBeTruthy())

    // Standing, and holding nothing: a region that arrived with the answer in
    // it would be initial content, which readers do not announce.
    expect(document.querySelector('[role="status"].sr-only')).not.toBeNull()
    expect(said()).toBe('')
  })

  it('says the fate, what it means, and the card it turned up', async () => {
    await spin()
    await waitFor(() => expect(screen.getByText('The wheel turns…')).toBeTruthy())
    stop()

    await waitFor(() => expect(said()).toBe(
      'The wheel stops: The Cup. The cup runneth over — a card that refills '
      + 'your hand. It turns up Rhystic Study.'))
  })

  it('stops short of the card when the shelves had none to offer', async () => {
    vi.mocked(api.wheelSpin).mockResolvedValue(landing({
      card: null, reason: 'No legal card in these colours answers to the cup.',
    }))
    await spin()
    await waitFor(() => expect(screen.getByText('The wheel turns…')).toBeTruthy())
    stop()

    // The fate still landed, which is the thing that was announced; the reason
    // there is no card renders below and is a different sentence.
    await waitFor(() => expect(said()).toBe(
      'The wheel stops: The Cup. The cup runneth over — a card that refills '
      + 'your hand.'))
  })

  it('speaks the refusal when there are no cards at all', async () => {
    vi.mocked(api.wheelSpin).mockResolvedValue({
      pool_available: false, symbol: null, card: null,
      message: 'No cards on the shelves yet.',
    } as WheelSpin)
    await spin()

    // No theatre on this path — the component reveals immediately, because
    // there is nothing to decelerate onto.
    await waitFor(() => expect(said()).toBe('No cards on the shelves yet.'))
  })

  it('announces the landing for somebody who asked for no motion', async () => {
    vi.stubGlobal('matchMedia', (media: string) => ({
      matches: media.includes('reduced-motion'), media,
    }))
    await spin()

    // The wheel jumps and nothing clicks — and the answer still has to be
    // said, or the setting that removes the theatre removes the result.
    await waitFor(() => expect(said()).toMatch(/^The wheel stops: The Cup\./))
  })
})
