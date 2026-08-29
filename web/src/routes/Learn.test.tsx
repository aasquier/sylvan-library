/**
 * The Learn page, and the properties that are easy to lose.
 *
 * The colours tab is an **index** now — the depth moved to `/colors/:slug`
 * and `ColorPage.test.tsx` went with it — so what is pinned here is what an
 * index owes:
 *
 * - all thirty-two are on it, each on the shelf it belongs to;
 * - every entry is a **link out**, not a control that swaps a panel
 *   underneath, which is what the old tab did and what made it look like
 *   navigation while not being any;
 * - the `?c=` links that were shared before the pages existed still land, and
 *   land on the room rather than on an error;
 * - nothing here holds a second copy of the taxonomy, so renaming a guild in
 *   the data renames it on the page *and moves its address* — which is the
 *   same rule the wheel is pinned by from both sides, with one more
 *   consequence than it used to have.
 */

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ColorTaxonomy, Combination, Glossary } from '../lib/api'
import { resetColorTaxonomyCache } from '../lib/colors'
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
    aliases: [], verified_by: 'a real card', creed: null, sigil: null, lore: '',
    champions: [], signature: [], ...over,
  }
}

const TAXONOMY: ColorTaxonomy = {
  colors: ['W', 'U', 'B', 'R', 'G'].map((code) => ({
    code, name: code, wants: 'x', fears: 'y',
  })),
  tiers: [
    { key: 'mono', label: 'Mono-colour', blurb: 'One colour.' },
    { key: 'guild', label: 'Guild — two colours', blurb: 'Two colours.' },
  ],
  eras: [{ name: 'Ravnica', setting: 'a city', named: 'the guilds',
           story: 'Ten of them, under a treaty.' }],
  combinations: [
    ...Object.entries({ W: 'White', U: 'Blue', B: 'Black', R: 'Red', G: 'Green' })
      .map(([key, colour]) =>
        combo({ key, name: `Mono-${colour}`, tier: 'mono' })),
    combo({
      key: 'BG', name: 'Golgari',
      lore: 'The Swarm holds the undercity and the whole decomposition '
          + 'contract, which is leverage nobody thinks about until it stops.',
      creed: {
        words: 'Let the rest of Ravnica sneer.', speaker: 'Jarad',
        card: 'Golgari Charm', printing: 'Return to Ravnica',
      },
      champions: [{ card: 'Jarad, Golgari Lich Lord', role: 'Dead, and in charge.' }],
      signature: ["Assassin's Trophy"],
    }),
    combo({ key: 'WU', name: 'Azorius', lore: '', champions: [], signature: ['Supreme Verdict'] }),
  ],
}

/**
 * Where each of the fixture's entries should link to, written out rather than
 * computed. Computing them with `comboSlug` would be the index agreeing with
 * itself; `lib/colors.test.ts` is where the scheme is checked against the
 * table that actually ships.
 */
const SLUGS: Record<string, string> = {
  W: '/colors/white', U: '/colors/blue', B: '/colors/black',
  R: '/colors/red', G: '/colors/green',
  BG: '/colors/golgari', WU: '/colors/azorius',
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

/** Stands in for a colour page, and reports the address it was reached at —
 *  which is the half of a redirect that matters. */
function ColourPageProbe() {
  const { pathname } = useLocation()
  return <p>a colour page{': '}<span>{pathname}</span></p>
}

function renderLearn(path = '/learn') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/learn" element={<Learn />} />
        <Route path="/colors/:slug" element={<ColourPageProbe />} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  resetGlossaryCache()
  resetColorTaxonomyCache()
  vi.mocked(api.colors).mockResolvedValue(TAXONOMY)
  vi.mocked(api.glossary).mockResolvedValue(GLOSSARY)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the colours tab, which is now an index', () => {
  it('lists every combination, on the shelf it belongs to, as a link out',
    async () => {
      renderLearn()
      expect(await screen.findByRole('heading', { name: 'Guild — two colours' }))
        .toBeTruthy()
      expect(screen.getByRole('heading', { name: 'Mono-colour' })).toBeTruthy()
      // Every combination the table holds is on the page, and every one of
      // them is a link out rather than a control that swaps a panel
      // underneath — which is what the old tab did, and what made it look
      // like navigation while not being any.
      for (const c of TAXONOMY.combinations) {
        const entry = screen.getByRole('link', { name: new RegExp(c.name) })
        expect(entry.getAttribute('href')).toBe(SLUGS[c.key])
      }
    })

  it('sends a shared ?c= link to the room it names', async () => {
    renderLearn('/learn?c=BG')
    // The redirect resolves through the served table, so it waits for that —
    // and what matters is the address it lands on, which the probe reports.
    expect(await screen.findByText('/colors/golgari')).toBeTruthy()
  })

  it('keeps the vocabulary tab out of it, ?c= and all', async () => {
    renderLearn('/learn?tab=words&c=BG')
    // A stale `c` on the words tab must not teleport a reader who asked for
    // the glossary, which is what a redirect reading the query string without
    // checking the tab would do.
    expect(await screen.findByText('Mulligan')).toBeTruthy()
    expect(screen.queryByText(/a colour page/)).toBeNull()
  })

  it('shrugs at a ?c= naming nothing, rather than showing an error', async () => {
    renderLearn('/learn?c=NOPE')
    expect(await screen.findByRole('heading', { name: 'Guild — two colours' }))
      .toBeTruthy()
    expect(screen.queryByText(/a colour page/)).toBeNull()
  })

  it('renames a combination when the data renames it', async () => {
    // The same property `pentagram.test.tsx` pins for the wheel: no second
    // copy of the taxonomy lives in the component — and now that the name is
    // also the address, renaming moves the page too.
    vi.mocked(api.colors).mockResolvedValue({
      ...TAXONOMY,
      combinations: TAXONOMY.combinations.map(
        (c) => (c.key === 'BG' ? { ...c, name: 'The Swarm' } : c)),
    })
    resetColorTaxonomyCache()
    renderLearn()
    const entry = await screen.findByRole('link', { name: /The Swarm/ })
    expect(entry.getAttribute('href')).toBe('/colors/the-swarm')
  })

  it('carries every tier’s own blurb, so the shelf says what it is', async () => {
    renderLearn()
    expect(await screen.findByText('Two colours.')).toBeTruthy()
    expect(screen.getByText('One colour.')).toBeTruthy()
  })

  it('says so when the guide itself will not open', async () => {
    vi.mocked(api.colors).mockRejectedValue(new Error('nope'))
    resetColorTaxonomyCache()
    renderLearn()
    await waitFor(() =>
      expect(screen.getByText(/Could not load the colour guide/)).toBeTruthy())
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

// ---- the shelves filter -----------------------------------------------------

// **These were jump links wearing a toggle's clothes.** Aaron, 2026-08-29:
// "I am not a fan of a toggle being presented as a link. That is awkward" —
// and, asked what they should be instead: "They should be real toggles."
//
// The old comment argued that a reader who came for the clans should not have
// to scroll past the guilds. That is true, and it is a better argument for
// narrowing the page than for scrolling it.
describe('the shelf filter', () => {
  it('starts with every shelf on, so the page opens as it always has', async () => {
    renderLearn()
    await waitFor(() => expect(screen.getByRole('button', { name: /Mono-colour/ })).toBeTruthy())

    for (const label of [/Mono-colour/, /Guild/]) {
      const chip = screen.getByRole('button', { name: label })
      expect(chip.getAttribute('aria-pressed')).toBe('true')
      expect(chip.className).toContain('is-on')
    }
    // And both shelves are on the page.
    expect(screen.getByText('Golgari')).toBeTruthy()
    expect(screen.getByText('Mono-White')).toBeTruthy()
  })

  it('hides a shelf when its chip is turned off, and brings it back', async () => {
    renderLearn()
    await waitFor(() => expect(screen.getByText('Golgari')).toBeTruthy())

    const guilds = screen.getByRole('button', { name: /Guild/ })
    fireEvent.click(guilds)
    await waitFor(() => expect(screen.queryByText('Golgari')).toBeNull())
    expect(guilds.getAttribute('aria-pressed')).toBe('false')
    // The shelf that was left on is untouched: this narrows, it does not
    // select one.
    expect(screen.getByText('Mono-White')).toBeTruthy()

    fireEvent.click(guilds)
    await waitFor(() => expect(screen.getByText('Golgari')).toBeTruthy())
  })

  // Turning everything off is a thing a thumb does by accident, and a blank
  // page with no explanation reads as broken rather than as chosen.
  it('says so when every shelf is hidden, and offers the way back', async () => {
    renderLearn()
    await waitFor(() => expect(screen.getByText('Golgari')).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: /Mono-colour/ }))
    fireEvent.click(screen.getByRole('button', { name: /Guild/ }))

    await waitFor(() =>
      expect(screen.getByText(/Every shelf is hidden/)).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Show them all again' }))
    await waitFor(() => expect(screen.getByText('Golgari')).toBeTruthy())
    expect(screen.getByText('Mono-White')).toBeTruthy()
  })

  // The vocabulary's see-also chips go somewhere; they hold no state and must
  // not claim one.
  it('gives the see-also trail a place, not a pressed state', async () => {
    renderLearn('/learn?tab=words')
    await waitFor(() => expect(screen.getByText(/See also/)).toBeTruthy())

    const trail = screen.getAllByRole('button').filter((b) =>
      b.className.includes('chip-place'))
    expect(trail.length).toBeGreaterThan(0)
    for (const chip of trail) {
      expect(chip.getAttribute('aria-pressed')).toBeNull()
      expect(chip.className).not.toContain('chip-toggle')
    }
  })
})
