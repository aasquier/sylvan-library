/**
 * The Learn page, and the properties that are easy to lose.
 *
 * Three of these are about honesty rather than layout, and each pins something
 * a screenshot would not catch:
 *
 * - a combination that is **not** a faction must not render an empty "What
 *   happened" heading, because writing to fill a field is exactly how
 *   Mono-Blue ends up with a story;
 * - a named card that the pool does not have is **dropped and counted**,
 *   not drawn from its name;
 * - with no card pool at all the page still teaches — names and prose, and an
 *   honest note about what is missing.
 *
 * The fourth is the same rule the wheel is pinned by from both sides: nothing
 * here holds a second copy of the taxonomy, so renaming a guild in the data
 * renames it on the page.
 */

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ColorTaxonomy, Combination, CombinationDetail, Glossary } from '../lib/api'
import { resetGlossaryCache } from '../lib/glossary'
import Learn from './Learn'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: { colors: vi.fn(), combination: vi.fn(), glossary: vi.fn() },
  }
})

const { api } = await import('../lib/api')

function combo(over: Partial<Combination> & { key: string; name: string }): Combination {
  return {
    tier: 'guild', colors: over.key.split(''), size: over.key.length,
    tagline: `${over.name} tagline.`, history: `${over.name} history.`,
    aliases: [], verified_by: 'a real card', lore: '', champions: [],
    signature: [], ...over,
  }
}

const TAXONOMY: ColorTaxonomy = {
  colors: ['W', 'U', 'B', 'R', 'G'].map((code) => ({
    code, name: code, wants: 'x', fears: 'y',
  })),
  tiers: [
    { key: 'mono', label: 'Mono-colour', blurb: 'One colour.' },
    { key: 'guild', label: 'Guild', blurb: 'Two colours.' },
  ],
  eras: [{ name: 'Ravnica', setting: 'a city', named: 'the guilds',
           story: 'Ten of them, under a treaty.' }],
  combinations: [
    ...['W', 'U', 'B', 'R', 'G'].map((c) =>
      combo({ key: c, name: `Mono-${c}`, tier: 'mono' })),
    combo({
      key: 'BG', name: 'Golgari',
      lore: 'The Swarm holds the undercity and the whole decomposition '
          + 'contract, which is leverage nobody thinks about until it stops.',
      champions: [{ card: 'Jarad, Golgari Lich Lord', role: 'Dead, and in charge.' },
                  { card: 'A Card That Does Not Exist', role: 'Dropped on the way.' }],
      signature: ["Assassin's Trophy"],
    }),
    combo({ key: 'WU', name: 'Azorius', lore: '', champions: [], signature: ['Supreme Verdict'] }),
  ],
}

const GOLGARI_DETAIL: CombinationDetail = {
  ...TAXONOMY.combinations.find((c) => c.key === 'BG')!,
  pool: true,
  champions: [{
    name: 'Jarad, Golgari Lich Lord', role: 'Dead, and in charge.',
    mana_cost: '{B}{B}{G}{G}', type_line: 'Legendary Creature — Zombie Elf',
    oracle_text: 'Sacrifice another creature: each opponent loses life.',
    color_identity: ['B', 'G'], image: null, art_crop: null,
  }],
  signature: [{
    name: "Assassin's Trophy", mana_cost: '{B}{G}', type_line: 'Instant',
    oracle_text: 'Destroy target permanent an opponent controls.',
    color_identity: ['B', 'G'], image: null, art_crop: null,
  }],
  // One champion name went in and did not come back.
  dropped: 1,
  exact_total: 812,
}

const GLOSSARY: Glossary = {
  sections: [
    { key: 'format', label: 'The format', blurb: 'Commander’s own rules.' },
    { key: 'simulator', label: 'Reading a simulation', blurb: 'The numbers.' },
  ],
  terms: [
    { key: 'mulligan', term: 'Mulligan', short: 'A new opening hand.',
      long: 'Commander uses the London mulligan, so you draw a fresh seven.',
      section: 'format', see_also: ['sim.min_pieces'] },
    { key: 'sim.min_pieces', term: 'Min mana pieces',
      short: 'Lands plus cheap ramp must reach this.',
      long: 'A piece is a land or a ramp card of mana value 2 or less.',
      section: 'simulator', see_also: [] },
  ],
}

function renderLearn(path = '/learn') {
  return render(
    <MemoryRouter initialEntries={[path]}><Learn /></MemoryRouter>,
  )
}

beforeEach(() => {
  resetGlossaryCache()
  vi.mocked(api.colors).mockResolvedValue(TAXONOMY)
  vi.mocked(api.combination).mockResolvedValue(GOLGARI_DETAIL)
  vi.mocked(api.glossary).mockResolvedValue(GLOSSARY)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the colours tab', () => {
  it('opens on a combination named by the query string', async () => {
    renderLearn('/learn?c=WU')
    expect(await screen.findByRole('heading', { name: 'Azorius', level: 2 }))
      .toBeTruthy()
  })

  it('tells a faction’s story and does not invent one for the rest', async () => {
    renderLearn('/learn?c=BG')
    expect(await screen.findByText(/holds the undercity/)).toBeTruthy()
    expect(screen.getByText('What happened')).toBeTruthy()
    // Ravnica's era paragraph rides along with the story, on the tiers that
    // have one.
    expect(screen.getByText(/under a treaty/)).toBeTruthy()

    cleanup()
    renderLearn('/learn?c=WU')
    await screen.findByRole('heading', { name: 'Azorius', level: 2 })
    expect(screen.queryByText('What happened')).toBeNull()
    expect(screen.queryByText('Who they are')).toBeNull()
  })

  it('shows the champion the pool resolved and drops the one it did not',
     async () => {
       renderLearn('/learn?c=BG')
       // Wait for the resolved cards rather than the name: before the fetch
       // lands the page shows the taxonomy's own list, which is the no-pool
       // rendering and legitimately includes every name.
       expect(await screen.findByText('Legendary Creature — Zombie Elf'))
         .toBeTruthy()
       expect(screen.getByText('Jarad, Golgari Lich Lord')).toBeTruthy()
       // The name that did not resolve is nowhere on the page...
       expect(screen.queryByText('A Card That Does Not Exist')).toBeNull()
       // ...and its absence is stated rather than silent.
       expect(screen.getByText(/1 named card could not be found/)).toBeTruthy()
     })

  it('counts how many cards are exactly these colours', async () => {
    renderLearn('/learn?c=BG')
    expect(await screen.findByText('812')).toBeTruthy()
  })

  it('says the whole set is on screen when there are only a handful', async () => {
    vi.mocked(api.combination).mockResolvedValue(
      { ...GOLGARI_DETAIL, exact_total: 2, dropped: 0 })
    renderLearn('/learn?c=BG')
    expect(await screen.findByText(/the whole set/)).toBeTruthy()
  })

  it('teaches without a card pool, and says what is missing', async () => {
    vi.mocked(api.combination).mockResolvedValue({
      ...GOLGARI_DETAIL, pool: false, champions: [], signature: [],
      dropped: 0, exact_total: null,
    })
    renderLearn('/learn?c=BG')
    // The names and the roles come from the taxonomy, which needs no card pool,
    // so the page still says who Jarad is.
    expect(await screen.findByText('Jarad, Golgari Lich Lord')).toBeTruthy()
    expect(screen.getByText(/Dead, and in charge/)).toBeTruthy()
    expect(screen.getByText(/pool is stocked/)).toBeTruthy()
  })

  it('renames the combination when the data renames it', async () => {
    // The same property `pentagram.test.tsx` pins for the wheel: no second
    // copy of the taxonomy lives in the component.
    vi.mocked(api.colors).mockResolvedValue({
      ...TAXONOMY,
      combinations: TAXONOMY.combinations.map(
        (c) => (c.key === 'BG' ? { ...c, name: 'The Swarm' } : c)),
    })
    renderLearn('/learn?c=BG')
    expect(await screen.findByRole('heading', { name: 'The Swarm', level: 2 }))
      .toBeTruthy()
  })

  it('offers a way to start the deck it just described', async () => {
    renderLearn('/learn?c=BG')
    const build = await screen.findByRole('link', { name: /Build Golgari/ })
    expect(build.getAttribute('href')).toBe('/new?c=BG')
  })
})

describe('the vocabulary tab', () => {
  it('groups terms under their section and renders the long form', async () => {
    renderLearn('/learn?tab=words')
    const section = (await screen.findByRole('heading', { name: 'The format' }))
      .closest('section')!
    expect(within(section).getByText('Mulligan')).toBeTruthy()
    expect(within(section).getByText(/London mulligan/)).toBeTruthy()
  })

  it('leads with the short form, because a long form may be sentence two',
     async () => {
    renderLearn('/learn?tab=words')
    await screen.findByText('Mulligan')
    const entry = document.getElementById('term-sim.min_pieces')!
    // The min-pieces `long` opens "A piece is…", commenting on a control that
    // only `short` names — the shape a third of the real entries have. Both
    // render, and the definition comes first.
    const definition = within(entry).getByText(/Lands plus cheap ramp/)
    const paragraph = within(entry).getByText(/mana value 2 or less/)
    expect(definition.compareDocumentPosition(paragraph)
           & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('filters on a search, and says so when nothing matches', async () => {
    renderLearn('/learn?tab=words')
    await screen.findByText('Mulligan')
    const box = screen.getByPlaceholderText(/mulligan, ramp, shuffle/)
    fireEvent.change(box, { target: { value: 'ramp' } })
    // "cheap ramp" is in the min-pieces entry and nowhere in the mulligan one.
    await waitFor(() => expect(screen.queryByText('Mulligan')).toBeNull())
    expect(screen.getByText('Min mana pieces')).toBeTruthy()

    fireEvent.change(box, { target: { value: 'zzzz' } })
    await waitFor(() => expect(screen.getByText(/may still be a real word/))
      .toBeTruthy())
  })

  it('links a cross-reference to a term in another section', async () => {
    renderLearn('/learn?tab=words')
    await screen.findByText('Mulligan')
    // `see_also` renders the *other* term's display name, looked up rather
    // than printed as its key.
    expect(screen.getAllByRole('button', { name: 'Min mana pieces' }).length)
      .toBeGreaterThan(0)
  })
})

describe('the reading room', () => {
  it('hangs the Bookworm at the foot of the page, credited', async () => {
    renderLearn()
    // The painting is a committed asset (bookworm.recipe.yaml), so the page
    // must name the painter and the licence -- the caption is the plaque.
    await screen.findByText(/Der Bücherwurm/)
    expect(screen.getByText(/Carl Spitzweg/)).toBeTruthy()
    expect(screen.getByText(/public domain/)).toBeTruthy()
    // The loop or its still: one of the two must be mounted. jsdom plays no
    // video, so what this pins is that the figure renders something at all.
    const painting = document.querySelector('.reading-room-frame video, .reading-room-frame img')
    expect(painting).not.toBeNull()
  })
})
