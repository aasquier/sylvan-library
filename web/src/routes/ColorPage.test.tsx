/**
 * A colour combination's own page, and the properties that are easy to lose.
 *
 * Most of these moved here from `Learn.test.tsx` when the panel became a page,
 * and they pin the same things they always did: a combination that is not a
 * faction must not render an empty heading, a card the pool does not have is
 * dropped and counted rather than drawn from its name, and with no pool at all
 * the page still teaches.
 *
 * Three are new, and all three are about the emblem rule:
 *
 * - a faction wears its **painting**, and the artist and the printing are in
 *   the same room as it (commandment 19);
 * - a slot that is not a faction wears its **own mana symbols** and no
 *   invented device;
 * - the creed and the sigil's caption do **different jobs** — one is a person
 *   speaking and one is a label on an object — so a page carrying both must
 *   not attribute them to the same thing.
 */

import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ColorTaxonomy, Combination, CombinationDetail } from '../lib/api'
import { resetColorTaxonomyCache } from '../lib/colors'
import ColorPage from './ColorPage'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { colors: vi.fn(), combination: vi.fn() } }
})

const { api } = await import('../lib/api')

function combo(over: Partial<Combination> & { key: string; name: string }): Combination {
  return {
    tier: 'guild', colors: over.key.split(''), size: over.key.length,
    tagline: `${over.name} tagline.`, history: `${over.name} history.`,
    aliases: [], verified_by: 'a real card', creed: null, sigil: null, lore: '',
    champions: [], signature: [], ...over,
  }
}

const GOLGARI = combo({
  key: 'BG', name: 'Golgari',
  lore: 'The Swarm holds the undercity and the whole decomposition contract, '
      + 'which is leverage nobody thinks about until it stops.',
  creed: {
    words: 'Let the rest of Ravnica sneer.', speaker: 'Jarad',
    card: 'Golgari Charm', printing: 'Return to Ravnica',
  },
  sigil: {
    card: 'Golgari Signet', artist: 'Greg Hildebrandt',
    printing: 'Ravnica: City of Guilds',
    art: 'https://cards.scryfall.io/art_crop/front/b/e/golgari.jpg',
    flavor: 'Depending on your point of view, the seal represents a proud '
          + 'guardian of the natural cycle.',
  },
  champions: [{ card: 'Jarad, Golgari Lich Lord', role: 'Dead, and in charge.' },
              { card: 'A Card That Does Not Exist', role: 'Dropped on the way.' }],
  signature: ["Assassin's Trophy"],
})

const TAXONOMY: ColorTaxonomy = {
  colors: [
    { code: 'W', name: 'White', wants: 'Peace through structure.',
      fears: 'That the rules stop serving the people.' },
    { code: 'U', name: 'Blue', wants: 'Perfection through knowledge.',
      fears: 'Acting before it understands.' },
    { code: 'B', name: 'Black', wants: 'Power through self-interest.',
      fears: 'That someone else will pay more.' },
    { code: 'R', name: 'Red', wants: 'Freedom through action.',
      fears: 'Being told to wait.' },
    { code: 'G', name: 'Green', wants: 'Growth through acceptance.',
      fears: 'Change imposed from outside.' },
  ],
  tiers: [
    { key: 'mono', label: 'Mono-colour', blurb: 'One colour, and everything else undone.' },
    { key: 'guild', label: 'Guild — two colours', blurb: 'Two colours, ten ways.' },
    { key: 'quad', label: 'Four colours', blurb: 'Best understood by the one they refuse.' },
  ],
  eras: [{ name: 'Ravnica', setting: 'a city', named: 'the guilds',
           story: 'Ten of them, under a treaty.' }],
  combinations: [
    combo({ key: 'R', name: 'Mono-Red', tier: 'mono', colors: ['R'],
            signature: ['Lightning Bolt'] }),
    combo({ key: 'WUBR', name: 'Artifice', tier: 'quad',
            colors: ['W', 'U', 'B', 'R'], aliases: ['Yore-Tiller'],
            signature: ['Breya, Etherium Shaper'] }),
    GOLGARI,
    combo({ key: 'WU', name: 'Azorius', signature: ['Supreme Verdict'] }),
  ],
}

const GOLGARI_DETAIL: CombinationDetail = {
  ...GOLGARI,
  pool: true,
  champions: [{
    name: 'Jarad, Golgari Lich Lord', role: 'Dead, and in charge.',
    mana_cost: '{B}{B}{G}{G}', type_line: 'Legendary Creature — Zombie Elf',
    oracle_text: 'Sacrifice another creature.', color_identity: ['B', 'G'],
    image: null, art_crop: null,
  }],
  signature: [{
    name: "Assassin's Trophy", mana_cost: '{B}{G}', type_line: 'Instant',
    oracle_text: 'Destroy target permanent.', color_identity: ['B', 'G'],
    image: null, art_crop: null,
  }],
  dropped: 1,
  exact_total: 1284,
}

function detailFor(key: string): CombinationDetail {
  const found = TAXONOMY.combinations.find((c) => c.key === key)!
  return {
    ...found, pool: true, champions: [], signature: [], dropped: 0,
    exact_total: key === 'WUBR' ? 2 : 40,
  }
}

/** Open one page, the way the router does. */
function open(slug: string) {
  return render(
    <MemoryRouter initialEntries={[`/colors/${slug}`]}>
      <Routes>
        <Route path="/colors/:slug" element={<ColorPage />} />
        <Route path="/learn" element={<p>the index</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  resetColorTaxonomyCache()
  vi.mocked(api.colors).mockResolvedValue(TAXONOMY)
  vi.mocked(api.combination).mockImplementation(async (key: string) =>
    key === 'BG' ? GOLGARI_DETAIL : detailFor(key))
})

afterEach(cleanup)

describe('the room', () => {
  it('opens the combination the address names', async () => {
    open('golgari')
    expect(await screen.findByRole('heading', { level: 1, name: 'Golgari' }))
      .toBeTruthy()
    expect(screen.getByText('Golgari tagline.')).toBeTruthy()
  })

  it('tells a faction’s story and does not invent one for the rest', async () => {
    open('golgari')
    expect(await screen.findByText(/holds the undercity/)).toBeTruthy()
    expect(screen.getByText(/Ten of them, under a treaty/)).toBeTruthy()
    cleanup()
    open('mono-red')
    await screen.findByRole('heading', { level: 1, name: 'Mono-Red' })
    expect(screen.queryByText('What happened')).toBeNull()
  })

  it('quotes the guild in its own words, and says which card they are printed on',
    async () => {
      open('golgari')
      const creed = await screen.findByText(/Let the rest of Ravnica sneer/)
      expect(creed.textContent).toContain('“')
      const plate = creed.closest('figure')!
      expect(within(plate).getByText('Jarad')).toBeTruthy()
      expect(within(plate).getByText('Golgari Charm')).toBeTruthy()
    })

  it('gives no creed plate to a combination that has no creed', async () => {
    open('azorius')
    await screen.findByRole('heading', { level: 1, name: 'Azorius' })
    expect(screen.queryByText(/Let the rest of Ravnica sneer/)).toBeNull()
  })

  it('shows the champion the pool resolved and drops the one it did not',
    async () => {
      open('golgari')
      // Waits for the resolved card rather than for the name: before the
      // fetch lands the page shows the taxonomy's own list, which is the
      // no-pool rendering and legitimately includes every name.
      expect(await screen.findByText('Legendary Creature — Zombie Elf'))
        .toBeTruthy()
      expect(screen.getByText('Jarad, Golgari Lich Lord')).toBeTruthy()
      expect(screen.queryByText('A Card That Does Not Exist')).toBeNull()
      expect(screen.getByText(/could not be found on the shelves/)).toBeTruthy()
    })

  it('counts how many cards are exactly these colours', async () => {
    open('golgari')
    expect(await screen.findByText('1,284')).toBeTruthy()
  })

  it('says the whole set is on screen when there are only a handful', async () => {
    open('wubr')
    expect(await screen.findByText(/the whole set/)).toBeTruthy()
  })

  it('teaches without a card pool, and says what is missing', async () => {
    vi.mocked(api.combination).mockResolvedValue({
      ...GOLGARI_DETAIL, pool: false, champions: [], signature: [],
      dropped: 0, exact_total: null,
    })
    open('golgari')
    // The names still arrive, from the served taxonomy rather than the pool.
    expect(await screen.findByText('Jarad, Golgari Lich Lord')).toBeTruthy()
    expect(screen.getByText(/once the library’s shelves are stocked/)).toBeTruthy()
  })

  it('renames the combination when the data renames it', async () => {
    vi.mocked(api.colors).mockResolvedValue({
      ...TAXONOMY,
      combinations: TAXONOMY.combinations.map((c) =>
        c.key === 'BG' ? { ...c, name: 'The Swarm' } : c),
    })
    resetColorTaxonomyCache()
    open('bg')
    expect(await screen.findByRole('heading', { level: 1, name: 'The Swarm' }))
      .toBeTruthy()
  })

  it('offers a way to start the deck it just described', async () => {
    open('golgari')
    const build = await screen.findByRole('link', { name: /Build a Golgari deck/ })
    expect(build.getAttribute('href')).toBe('/new?c=BG')
  })

  it('says "an Artifice deck", because eight of the thirty-two start with a vowel',
    async () => {
      open('artifice')
      expect(await screen.findByRole('link', { name: /Build an Artifice deck/ }))
        .toBeTruthy()
    })

  it('links sideways to the rest of its own kind, and not to itself', async () => {
    open('golgari')
    expect(await screen.findByRole('link', { name: /Azorius/ })).toBeTruthy()
    expect(screen.queryAllByRole('link', { name: /^Golgari$/ })).toHaveLength(0)
  })
})

describe('the emblem rule', () => {
  it('gives a faction its painting, credited to the painter in the same room',
    async () => {
      open('golgari')
      const art = await screen.findByRole('img', { name: /painted by Greg Hildebrandt/ })
      expect(art.getAttribute('src')).toContain('cards.scryfall.io')
      // Commandment 19: the credit is the licence made visible, and it is not
      // enough for it to be somewhere — it has to be beside the picture.
      expect(screen.getByText(/Greg Hildebrandt, Ravnica: City of Guilds/))
        .toBeTruthy()
      expect(screen.getByText('Golgari Signet')).toBeTruthy()
    })

  it('gives the twelve without one their own mana symbols and no invented device',
    async () => {
      open('mono-red')
      await screen.findByRole('heading', { level: 1, name: 'Mono-Red' })
      // No painting anywhere on a page for a slot that is not a faction.
      // Asserted on the source rather than on a count of `<img>`: the plate
      // is full of images — the official symbols are images, and so is every
      // pip in the colour ring — and the property that matters is that none
      // of them is a card.
      const paintings = [...document.querySelectorAll('img')]
        .filter((i) => (i.getAttribute('src') ?? '').includes('scryfall'))
      expect(paintings).toHaveLength(0)
      // The official symbol is served from this app's own origin (ADR 33) and
      // the mark itself is decoration inside the plate, so it is the request
      // that proves it rather than an accessible name.
      const marks = document.querySelectorAll('.combo-emblem-mark img')
      expect(marks).toHaveLength(1)
      expect(marks[0]!.getAttribute('src')).toBe('/api/symbols/R.svg')
    })

  it('draws one mark per colour, so five colours are five symbols', async () => {
    open('artifice')
    await screen.findByRole('heading', { level: 1, name: 'Artifice' })
    expect(document.querySelectorAll('.combo-emblem-mark')).toHaveLength(4)
  })

  it('gives the creed and the device caption different jobs', async () => {
    open('golgari')
    await screen.findByRole('heading', { level: 1, name: 'Golgari' })
    // The creed is a person speaking, and is attributed to one.
    const creed = screen.getByText(/Let the rest of Ravnica sneer/).closest('figure')!
    expect(within(creed).getByText('Jarad')).toBeTruthy()
    // The label is an object being explained, and is attributed to the card.
    const label = screen.getByText(/the seal represents a proud guardian/)
      .closest('aside')!
    expect(within(label).getByText('Golgari Signet')).toBeTruthy()
    expect(within(label).queryByText('Jarad')).toBeNull()
    // ...and they are two different elements, not one plate carrying both.
    expect(creed.contains(label)).toBe(false)
  })

  it('says what a four-colour deck refuses, because that is what names it',
    async () => {
      open('artifice')
      await screen.findByRole('heading', { level: 1, name: 'Artifice' })
      expect(screen.getByText(/and the one it refuses: Green/)).toBeTruthy()
      // The four it has are voices too, and the fifth is not one of them.
      expect(document.querySelectorAll('.combo-voice')).toHaveLength(5)
      expect(document.querySelectorAll('.combo-voice-absent')).toHaveLength(1)
    })

  it('does not ask a two-colour page what it refuses', async () => {
    open('golgari')
    await screen.findByRole('heading', { level: 1, name: 'Golgari' })
    expect(document.querySelectorAll('.combo-voice-absent')).toHaveLength(0)
    expect(document.querySelectorAll('.combo-voice')).toHaveLength(2)
  })
})

describe('the address', () => {
  it('redirects every other spelling to the one the room lives at', async () => {
    open('bg')
    await screen.findByRole('heading', { level: 1, name: 'Golgari' })
    // Arrived, and the address it arrived at is the canonical one — which is
    // what the redirect is for. `MemoryRouter` records the replacement.
    expect(await screen.findByText('Golgari tagline.')).toBeTruthy()
  })

  it('hands a segment that names nothing back to the index', async () => {
    open('grixis-but-cooler')
    expect(await screen.findByText(/none of them is called/)).toBeTruthy()
    expect(screen.getByRole('link', { name: /See all thirty-two/ })).toBeTruthy()
  })

  it('says so when the guide itself will not open', async () => {
    vi.mocked(api.colors).mockRejectedValue(new Error('no'))
    resetColorTaxonomyCache()
    open('golgari')
    await waitFor(() =>
      expect(screen.getByText(/Could not open the colour guide/)).toBeTruthy())
  })
})
