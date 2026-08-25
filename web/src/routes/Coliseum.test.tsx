/**
 * The Coliseum, and the properties a screenshot would not catch.
 *
 * Four of these are about honesty rather than layout:
 *
 * - a fact renders **the halves its kind promises** — a `magic` slide has no
 *   Roman paragraph and a `paired` slide has both, because a kind whose
 *   fields do not match is a slide that comes out blank on one side;
 * - with **no card pool** the room still teaches: every fact still reads, and
 *   the page says plainly that the paintings are what is missing;
 * - a champion the pool dropped simply is not there, and the page does not
 *   draw one from its name;
 * - the weather is **decoration** — out of the accessibility tree, and never
 *   the thing carrying the meaning.
 *
 * And the fifth is the one Aaron will notice first: walking into a different
 * arena starts that arena's rotation at its own first fact, rather than
 * halfway through the last one's.
 */

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Coliseum, ColiseumArena, ColiseumChampion, ColiseumFact } from '../lib/api'
import ColiseumRoom from './Coliseum'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { coliseum: vi.fn() } }
})

const { api } = await import('../lib/api')

function champion(name: string, role: string): ColiseumChampion {
  return {
    name, role, mana_cost: null, type_line: 'Legendary Creature',
    oracle_text: '', color_identity: [], image: null,
    art_crop: `https://cards.scryfall.io/art_crop/${name}.jpg`,
  }
}

function arena(over: Partial<ColiseumArena> & { key: string; name: string }): ColiseumArena {
  return {
    plane: 'Otaria', motion: 'sand',
    palette: { ink: '#f3e2bd', glow: '#b5762c' },
    backdrop: champion(`${over.name} backdrop`, ''),
    champions: [champion('Jareth, Leonine Titan', 'Fights behind the shield alone.')],
    facts: [{ kind: 'roman', rome: 'The floor was strewn with harena.' }],
    ...over,
  }
}

function room(over: Partial<Coliseum> = {}): Coliseum {
  return {
    pool: true, dropped: 0,
    arenas: [arena({ key: 'grand-coliseum', name: 'The Grand Coliseum' })],
    ...over,
  }
}

function show() {
  return render(<MemoryRouter><ColiseumRoom /></MemoryRouter>)
}

beforeEach(() => { vi.mocked(api.coliseum).mockReset() })
afterEach(cleanup)

describe('the Coliseum', () => {
  it('renders each kind of fact with the halves that kind promises', async () => {
    const facts: ColiseumFact[] = [
      { kind: 'paired', rome: 'Sand drank the blood.', magic: 'And the land still charges for it.', card: 'Grand Coliseum' },
    ]
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [arena({ key: 'a', name: 'A', facts })],
    }))
    const { container } = show()
    await screen.findByText('Sand drank the blood.')
    // Both halves, and the card the Magic half is about — asserted *within
    // the slide*, because the masthead credit names Grand Coliseum too and a
    // bare query cannot tell the room's own nameplate from a fact about it.
    const slide = container.querySelector('.arena-slide')
    expect(slide).toBeTruthy()
    const inSlide = within(slide as HTMLElement)
    expect(inSlide.getByText('And the land still charges for it.')).toBeTruthy()
    expect(inSlide.getByText('Grand Coliseum')).toBeTruthy()
  })

  it('does not invent a Roman half for a Magic fact', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [arena({
        key: 'a', name: 'A',
        facts: [{ kind: 'magic', magic: 'Jareth is a 4/7.' }],
      })],
    }))
    show()
    await screen.findByText('Jareth is a 4/7.')
    expect(screen.getByText('Magic')).toBeTruthy()
    expect(screen.queryByText('Rome, and its echo')).toBeNull()
  })

  it('still teaches with no card pool, and says what is missing', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      pool: false,
      arenas: [arena({
        key: 'a', name: 'A', backdrop: null, champions: [],
        facts: [{ kind: 'coliseum', rome: 'Eighty numbered arches.' }],
      })],
    }))
    show()
    // The prose is the point and it is all text: it survives a missing pool.
    await screen.findByText('Eighty numbered arches.')
    expect(screen.getByText(/showing without their/i)).toBeTruthy()
    // The stage is still legible rather than a hole.
    expect(screen.getByRole('img', { name: /without its painting/i })).toBeTruthy()
  })

  it('walks to another arena and starts that arena at its own first fact', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [
        arena({ key: 'a', name: 'The Grand Coliseum',
          facts: [
            { kind: 'roman', rome: 'First of the Coliseum.' },
            { kind: 'roman', rome: 'Second of the Coliseum.' },
          ] }),
        arena({ key: 'b', name: 'The Cephalid Coliseum', motion: 'water',
          facts: [{ kind: 'roman', rome: 'First of the drowned one.' }] }),
      ],
    }))
    show()
    await screen.findByText('First of the Coliseum.')

    // Advance the first arena's rotation, then leave it.
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText('Second of the Coliseum.')

    fireEvent.click(screen.getByRole('button', { name: 'The Cephalid Coliseum' }))
    // Not "slide 2 of an arena that has one slide", and not a blank.
    await screen.findByText('First of the drowned one.')
  })

  it('keeps the weather out of the accessibility tree', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room())
    const { container } = show()
    await screen.findByText(/harena/)
    const motion = container.querySelector('.arena-motion')
    expect(motion).toBeTruthy()
    // Decoration announces nothing: the facts carry the meaning.
    expect(motion?.getAttribute('aria-hidden')).toBe('true')
    // And the arena's motion is named, so the stylesheet has something to
    // hang six different kinds of weather on.
    expect(motion?.getAttribute('data-motion')).toBe('sand')
  })

  it('shows only the champions the pool actually resolved', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      dropped: 2,
      arenas: [arena({
        key: 'a', name: 'A',
        champions: [champion('Kamahl, Pit Fighter', 'The pits named him.')],
      })],
    }))
    show()
    await screen.findByText('Kamahl, Pit Fighter')
    // A dropped name is absent, never drawn from the name alone.
    expect(screen.queryByText('Jareth, Leonine Titan')).toBeNull()
  })

  it('says so when the doors do not answer', async () => {
    vi.mocked(api.coliseum).mockRejectedValue(new Error('nope'))
    show()
    await waitFor(() => expect(screen.getByText(/did not answer/i)).toBeTruthy())
  })
})
