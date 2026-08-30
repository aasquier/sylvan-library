/** The import page.
 *
 * Two things here are load-bearing rather than cosmetic, and both come from
 * ADR 13. The preview must be the *same* request the import sends, with
 * `dry_run` flipped — a preview that estimates is worse than none, because it
 * looks authoritative. And the page must never offer to write a rationale: the
 * result it shows ends on a count of the work still owed.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, ImportResult, Job, StanceView } from '../lib/api'
import Import from './Import'

const navigate = vi.fn()
vi.mock('react-router-dom', async () => ({
  ...(await vi.importActual<typeof import('react-router-dom')>('react-router-dom')),
  useNavigate: () => navigate,
}))

vi.mock('../lib/api', async () => ({
  errorMessage: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).errorMessage,
  // Real: it is what turns the response into the deck's address, and a stub
  // would let this navigate to the pre-ADR-22 path with every test passing.
  deckUrl: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).deckUrl,
  // `claudeStatus` and `intake` are the two halves of ADR 41's second gate as
  // this page meets it: one decides what the sheet shows, the other carries
  // the submit. They are mocked side by side because the question worth asking
  // is whether the same value reaches both.
  api: { importDeck: vi.fn(), claudeStatus: vi.fn(), intake: vi.fn() },
  // Stubbed: this file asks what the page *sends*, and the real poller would
  // put a timer between the click and the assertion for nothing.
  followJob: vi.fn(),
  // Carries `status`, because `intakeTrouble` reads it: a lost job is a 404
  // and gets its own sentence, and a stub without the field would send every
  // test down the generic branch.
  ApiError: class extends Error {
    status: number
    constructor(message: string, status = 0) { super(message); this.status = status }
  },
}))

const { api, followJob, ApiError } = await import('../lib/api')

function result(overrides: Partial<ImportResult> = {}): ImportResult {
  return {
    slug: 'arahbo-cats',
    owner: 'aasquier',
    name: 'Arahbo — Cats',
    stage: 'draft',
    status: 'theoretical',
    created: false,
    commander: ['Arahbo, Roar of the World'],
    companion: null,
    total_cards: 99,
    land_count: 36,
    swap_board: [],
    needs_rationale: 85,
    rationales: 0,
    why_by: '',
    unknown: [],
    read: [],
    did_you_mean: [],
    did_you_mean_skipped: 0,
    unreadable: [],
    skipped: [],
    notes: [],
    yaml: 'slug: arahbo-cats\nstage: draft\n',
    ok: true,
    errors: [],
    warnings: [],
    ...overrides,
  }
}

function stanceView(preset: string, mayWrite: boolean): StanceView {
  return { preset, allows_calls: true, may_write: mayWrite, axes: [] }
}

/** What the dial answers the intake sheet. `mayWrite` is the whole gate: it
 *  decides whether "Draft the reasons" is on the screen at all. */
function dial(mayWrite: boolean): ClaudeStatus {
  return {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: stanceView('collaborator', mayWrite),
    ceiling: stanceView('collaborator', true),
    default: stanceView('consultant', false),
    presets: [], never: '', modes: [],
  }
}

function job(): Job {
  return {
    id: 'j1', kind: 'claude.intake', status: 'done', done: 1, total: 1,
    percent: 100, label: 'intake: cats', result: null, partial: null,
    error: null, created_at: '2026-08-29T00:00:00Z',
  }
}

function renderImport() {
  return render(<MemoryRouter><Import /></MemoryRouter>)
}

function paste(text: string) {
  fireEvent.change(screen.getByLabelText('Decklist'), { target: { value: text } })
}

/**
 * Pin the dial the way a person does, and wait until the sheet has heard.
 *
 * `lib/stance` keeps the pin outside React and re-reads it on a `storage`
 * event, which is how a second tab corrects this one — so writing the
 * preference and firing the event drives the real mechanism instead of
 * reaching past it. It only works while something is subscribed, which is why
 * every caller renders the page first.
 *
 * The wait is on the dial having been *re-asked* with the new pin, because
 * that request is the one whose answer decides what the sheet shows. Anything
 * asserted before it would be asserted against the previous answer.
 */
async function pinTheDial(preset: string) {
  localStorage.setItem('mtglab-stance', preset)
  window.dispatchEvent(new StorageEvent('storage', { key: 'mtglab-stance' }))
  await waitFor(() => expect(
    vi.mocked(api.claudeStatus).mock.calls.at(-1)?.[0]?.stance).toBe(preset))
}

/** What the dial was last asked with — the request whose answer is on screen. */
function decidedWith(): string | undefined {
  return vi.mocked(api.claudeStatus).mock.calls.at(-1)?.[0]?.stance
}

beforeEach(() => {
  navigate.mockReset()
  vi.mocked(api.importDeck).mockReset().mockResolvedValue(result())
  // A dial that answers, and answers "may not write" — the position most
  // people are on, and the one that keeps the drafting toggle off the screen
  // for every test that is not about it.
  vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(dial(false))
  vi.mocked(api.intake).mockReset().mockResolvedValue(job())
  vi.mocked(followJob).mockReset().mockReturnValue({
    promise: Promise.resolve(job()), cancel: () => {},
  })
})

afterEach(cleanup)

describe('Import', () => {
  it('will not submit an empty list, or one with no slug', () => {
    renderImport()
    expect(screen.getByText('Preview').closest('button')!.disabled).toBe(true)

    paste('1 Sol Ring')
    // Still no name, so no slug.
    expect(screen.getByText('Preview').closest('button')!.disabled).toBe(true)

    fireEvent.change(screen.getByLabelText('Deck name'), {
      target: { value: 'Arahbo — Cats' },
    })
    expect(screen.getByText('Preview').closest('button')!.disabled).toBe(false)
  })

  it('derives a slug from the name until the slug is edited by hand', () => {
    renderImport()
    fireEvent.change(screen.getByLabelText('Deck name'), {
      target: { value: 'Arahbo — Cats!' },
    })
    const slug = screen.getByLabelText('Slug') as HTMLInputElement
    expect(slug.value).toBe('arahbo-cats')

    fireEvent.change(slug, { target: { value: 'my-own-slug' } })
    fireEvent.change(screen.getByLabelText('Deck name'), {
      target: { value: 'Something Else' },
    })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('my-own-slug')
  })

  it('previews with dry_run and writes nothing', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0]).toMatchObject({
      slug: 'cats', text: '1 Sol Ring', dry_run: true,
    })
    expect(navigate).not.toHaveBeenCalled()
  })

  it('sends the identical payload when importing for real', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })

    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledTimes(1))

    vi.mocked(api.importDeck).mockResolvedValue(result({ created: true }))
    fireEvent.click(screen.getByText('Import as draft'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledTimes(2))

    const [previewed, imported] = vi.mocked(api.importDeck).mock.calls.map((c) => c[0])
    expect({ ...previewed, dry_run: false }).toEqual(imported)
  })

  it('goes to the new deck once it is created', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({ created: true }))
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Import as draft'))
    // Owner-qualified, and read off the response rather than assumed: the
    // server chooses which library a new deck lands in (ADR 22).
    await waitFor(() => expect(navigate)
      .toHaveBeenCalledWith('/decks/aasquier/arahbo-cats'))
  })

  /**
   * The commander field is sent WHOLE, and this test used to assert the
   * opposite.
   *
   * It split on commas here, before the request, because a partner pair is
   * two commanders — and `Ley Weaver, Lore Weaver` really is a pair. What the
   * old test could not see is that the same rule turned `Arahbo, Roar of the
   * World` into two commanders, neither of them a card: a comma is
   * punctuation inside most legendary names, and every deck in this library
   * is led by one.
   *
   * Telling those apart takes the card pool, which is on the other side of
   * this wire. So the client sends what was typed and the server decides by
   * looking both readings up (`deckimport.commanderReading`) — the pairing
   * still works, and now it works because the parts are cards rather than
   * because a comma was present.
   */
  it('sends the commander field whole, commas and all', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.change(screen.getByLabelText('Commander'), {
      target: { value: 'Arahbo, Roar of the World' },
    })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0].commander)
      .toEqual(['Arahbo, Roar of the World'])
  })

  it('sends a pairing whole too, and lets the pool decide', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.change(screen.getByLabelText('Commander'), {
      target: { value: 'Ley Weaver + Lore Weaver' },
    })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0].commander)
      .toEqual(['Ley Weaver + Lore Weaver'])
  })

  it('sends no commander at all when the field is blank', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0].commander).toEqual([])
  })

  // -------------------------------------------------------------- the report

  it('leads on the count of rationales still owed, and offers to write none', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(screen.getByText(/85 cards still need a/)).toBeTruthy())
    expect(screen.getByText('draft')).toBeTruthy()
    expect(screen.queryByText(/generate|suggest|write .* for you/i)).toBeNull()
  })

  // ADR 49: the paste may declare its quoted reasons were Claude's drafting,
  // and the declaration rides the import itself — off by default, because an
  // unmarked reason means a person wrote it.
  it('sends why_by only when the drafting is declared, and says the reasons land signed', async () => {
    renderImport()
    paste('1 Sol Ring "fast mana"')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(api.importDeck).toHaveBeenCalledTimes(1))
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0]).not.toHaveProperty('why_by')

    const declare = screen.getByText('Claude drafted these reasons').closest('button')!
    expect(declare.getAttribute('aria-pressed')).toBe('false')
    fireEvent.click(declare)
    expect(declare.getAttribute('aria-pressed')).toBe('true')

    vi.mocked(api.importDeck).mockResolvedValue(result({
      rationales: 1, why_by: 'claude',
    }))
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledTimes(2))
    expect(vi.mocked(api.importDeck).mock.calls[1]?.[0]).toMatchObject({
      why_by: 'claude',
    })
    await waitFor(() =>
      expect(screen.getByText(/Claude’s drafting, and the file says so/)).toBeTruthy())
  })

  it('shows the gate errors the list already has', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({
      ok: false,
      errors: [{ code: 'banned', card: 'Primeval Titan',
                 message: 'not legal in Commander' }],
    }))
    renderImport()
    paste('1 Primeval Titan')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(screen.getByText(/not legal in Commander/)).toBeTruthy())
    expect(screen.getByText('1 error(s)')).toBeTruthy()
  })

  it('reports an unresolved name as unresolved, not as a suggestion', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({ unknown: ['Sol Rng'] }))
    renderImport()
    paste('1 Sol Rng')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() =>
      expect(screen.getByText(/1 name the\s+pool does not know/)).toBeTruthy())
    expect(screen.getByText('Sol Rng')).toBeTruthy()
    // The name is reported as written and the list is untouched. This
    // mattered before shortlists existed and it matters more now.
    expect(screen.getByText(/nothing below has been applied/)).toBeTruthy()
    expect((screen.getByLabelText('Decklist') as HTMLTextAreaElement).value)
      .toBe('1 Sol Rng')
  })

  it('names the lines it could not read, with their numbers', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({
      unreadable: [{ line: 7, text: '(LTC) 284' }],
    }))
    renderImport()
    paste('junk')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(screen.getByText('line 7: (LTC) 284')).toBeTruthy())
  })

  // The label no longer names the file format (commandment 10 -- a player is
  // never told what this is written in), so the disclosure is found by its
  // ROLE and its state rather than by its prose. That is the better handle
  // anyway: `aria-expanded` is the thing a screen reader is told, so asserting
  // on it is asserting on what a person actually receives.
  it('can show the file it would write, and says when it is open', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    const shown = () => screen.getByRole('button', { name: /the file this writes/ })
    await waitFor(() => expect(shown()).toBeTruthy())
    expect(shown().getAttribute('aria-expanded')).toBe('false')

    fireEvent.click(shown())
    expect(screen.getByText(/stage: draft/)).toBeTruthy()
    expect(shown().getAttribute('aria-expanded')).toBe('true')
  })

  it('surfaces a refusal instead of failing silently', async () => {
    vi.mocked(api.importDeck).mockRejectedValue(new Error('no commander in this list'))
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Import as draft'))

    await waitFor(() => expect(screen.getByText(/no commander in this list/)).toBeTruthy())
    expect(navigate).not.toHaveBeenCalled()
  })
})

describe('the deck that exists nowhere online', () => {
  /* Every import path is text, so a deck that only exists as a stack of
     cards has nothing to paste — and that is the newcomer's deck. Until the
     camera lands (ROADMAP item 14) this paragraph is the whole feature, so
     it is pinned: it names the apps, and it states the one cap that bites
     at card ninety rather than at card one. */

  beforeEach(() => { cleanup() })

  it('tells someone holding the cards where to start', () => {
    render(<MemoryRouter><Import /></MemoryRouter>)
    expect(screen.getByText(/Only have the cards\?/)).toBeTruthy()
    expect(screen.getByText('Dragon Shield MTG Scanner')).toBeTruthy()
    expect(screen.getByText('ManaBox')).toBeTruthy()
  })

  it('states the export cap rather than letting it be discovered', () => {
    render(<MemoryRouter><Import /></MemoryRouter>)
    expect(screen.getByText(/100 cards a session/)).toBeTruthy()
  })
})

/**
 * The shortlist beside a name the pool does not know.
 *
 * The strictness is the feature -- `deckimport` guesses nothing, ever -- so
 * everything here is about the person accepting a correction, and about the
 * pasted list staying the one source of what gets written.
 */
describe('did you mean', () => {
  const TYPOS = result({
    unknown: ['Sol Rng', 'Cultivate', 'Wgrsdlkj'],
    did_you_mean: [
      { written: 'Sol Rng',
        candidates: [{ name: 'Sol Ring', score: 0.975 }] },
      { written: 'Cultivate',
        candidates: [{ name: 'Cultivator Drone', score: 0.905 },
                     { name: 'Cultivator Colossus', score: 0.902 }] },
    ],
    did_you_mean_skipped: 0,
  })

  async function preview(text: string, payload = TYPOS) {
    vi.mocked(api.importDeck).mockResolvedValue(payload)
    renderImport()
    paste(text)
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await screen.findByRole('heading', { name: /does not know/ })
  }

  it('offers the near names, and applies none of them by itself', async () => {
    await preview('1 Sol Rng\n1 Cultivate\n1 Wgrsdlkj')
    expect(screen.getByRole('button', { name: 'Sol Ring' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Cultivator Drone' })).toBeTruthy()
    expect((screen.getByLabelText('Decklist') as HTMLTextAreaElement).value)
      .toContain('Sol Rng')
  })

  it('says so when nothing in the pool is close', async () => {
    await preview('1 Sol Rng\n1 Cultivate\n1 Wgrsdlkj')
    expect(screen.getByText('Wgrsdlkj')).toBeTruthy()
    expect(screen.getByText(/nothing in the pool is close to this one/))
      .toBeTruthy()
  })

  it('warns that a miss can be a new card rather than a typo', async () => {
    await preview('1 Sol Rng')
    expect(screen.getByText(/printed since this pool was last refreshed/))
      .toBeTruthy()
  })

  it('rewrites the pasted list when a name is pressed, and previews again',
     async () => {
    await preview('4 Forest\n1 Sol Rng\n1 Cultivate')
    vi.mocked(api.importDeck).mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Sol Ring' }))

    const box = screen.getByLabelText('Decklist') as HTMLTextAreaElement
    expect(box.value).toBe('4 Forest\n1 Sol Ring\n1 Cultivate')
    // And the preview is re-run from the corrected list rather than from
    // state that has not landed yet.
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledWith(
      expect.objectContaining({
        text: '4 Forest\n1 Sol Ring\n1 Cultivate', dry_run: true })))
  })

  it('rewrites whole names, never substrings of a card that was right',
     async () => {
    // `Cultivate` is the misspelling here AND a substring of a correct card
    // on the line above it. A bare replace would corrupt the good one.
    await preview('1 Cultivator Colossus\n1 Cultivate')
    fireEvent.click(screen.getByRole('button', { name: 'Cultivator Drone' }))
    expect((screen.getByLabelText('Decklist') as HTMLTextAreaElement).value)
      .toBe('1 Cultivator Colossus\n1 Cultivator Drone')
  })

  it('reports the misses it did not check rather than hiding the cap',
     async () => {
    await preview('1 Sol Rng', result({
      unknown: ['Sol Rng'],
      did_you_mean: [{ written: 'Sol Rng',
        candidates: [{ name: 'Sol Ring', score: 0.975 }] }],
      did_you_mean_skipped: 20,
    }))
    expect(screen.getByText(/20 more went unchecked/)).toBeTruthy()
  })
})

/**
 * The correction itself, which happens on the server and is only reported
 * here.
 *
 * Aaron's ruling, 2026-08-24: do the matching on the backend and do not let
 * misspelled things in. So by the time this page renders, the deck already
 * holds the real card -- and the whole obligation on the client is to say so
 * where somebody will see it.
 */
describe('names that were read', () => {
  const READ = result({
    read: [
      { written: 'Sol Rng', read: 'Sol Ring', score: 0.975 },
      { written: 'Rhystic Studdy', read: 'Rhystic Study', score: 0.9857 },
    ],
  })

  async function preview(payload: ImportResult) {
    vi.mocked(api.importDeck).mockResolvedValue(payload)
    renderImport()
    paste('1 Sol Rng\n1 Rhystic Studdy')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
  }

  it('says what was read, and as what', async () => {
    await preview(READ)
    await waitFor(() =>
      expect(screen.getByText(/2 names were read as the card/)).toBeTruthy())
    expect(screen.getByText('Sol Rng')).toBeTruthy()
    expect(screen.getByText('Sol Ring')).toBeTruthy()
    expect(screen.getByText('Rhystic Study')).toBeTruthy()
  })

  it('says the deck holds the real card, not the string that was typed',
     async () => {
    await preview(READ)
    await waitFor(() =>
      expect(screen.getByText(/its cost, its colours and its\s+legality/))
        .toBeTruthy())
  })

  it('counts one correction in the singular', async () => {
    await preview(result({
      read: [{ written: 'Sol Rng', read: 'Sol Ring', score: 0.975 }],
    }))
    await waitFor(() =>
      expect(screen.getByText(/1 name was read as the card/)).toBeTruthy())
  })

  it('says nothing at all when nothing needed reading', async () => {
    await preview(result())
    await waitFor(() => expect(screen.getByText(/99 cards in the 99/)).toBeTruthy())
    expect(screen.queryByText(/read as the card/)).toBeNull()
  })

  // The quoted rationale column. The page is the only place the format is
  // written down, and the summary is the only place a person learns their
  // reasons arrived at all.
  it('teaches the quoted column without being asked', () => {
    render(<MemoryRouter><Import /></MemoryRouter>)
    expect(screen.getByText(/Say why a card is in the deck/)).toBeTruthy()
    expect(screen.getByText(/Deathtouch body that kills artifacts too/)).toBeTruthy()
  })

  it('counts the reasons that arrived, not only the ones still owed',
     async () => {
    await preview(result({ rationales: 60, needs_rationale: 39 }))
    await waitFor(() =>
      expect(screen.getByText(/60 cards arrived with your reason already written/))
        .toBeTruthy())
    expect(screen.getByText(/39 cards still need a/)).toBeTruthy()
  })

  it('says nothing is owed when every card came with its reason', async () => {
    await preview(result({ rationales: 99, needs_rationale: 0 }))
    await waitFor(() => expect(screen.getByText('Nothing is owed.')).toBeTruthy())
    // The promotion is a real control on the deck page, so the page says where
    // it is rather than leaving a finished deck sitting in draft unexplained.
    expect(screen.getByText(/promote it to curated from the\s+deck page/)).toBeTruthy()
  })

  it('keeps the old wording when a paste carried no reasons at all', async () => {
    await preview(result({ rationales: 0, needs_rationale: 85 }))
    await waitFor(() =>
      expect(screen.getByText(/85 cards still need a/)).toBeTruthy())
    expect(screen.queryByText(/arrived with your reason/)).toBeNull()
  })
})

/**
 * **The bug Aaron hit on 2026-08-29**, and the shape of the test that would
 * have caught it.
 *
 * He set the dial to a stance that may write, ticked "Draft the reasons" on
 * the import sheet, and was told his stance "is set to change nothing". It was
 * not. The sheet asked the dial *with his pin* and showed the toggle because
 * the answer said it may write; the submit sent no stance at all, so the
 * server resolved the deck's own default — `consultant`, write `none` — and
 * refused. Two requests, two different questions, one gate.
 *
 * So what is pinned here is not a value but an **equality**: the stance the
 * sheet made its decision with is the stance the submit carries. Asserting
 * against a literal would pass against a page that hardcoded one, which is why
 * the last test changes the dial underneath the page and asks again — a page
 * that captured the pin once, or that read it a second time for itself, gets
 * through the first test and falls over on that one.
 *
 * Everything here drives the real mechanism: the real `IntakeChoices`, the
 * real stance store, the real `runIntake`. Nothing is asserted about a call
 * that was made by hand.
 */
describe('the stance that decided the sheet', () => {
  async function pasteAndPin(preset: string) {
    vi.mocked(api.importDeck).mockResolvedValue(result({ created: true }))
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    // Rendered first, so the store has a subscriber to tell.
    await screen.findByRole('button', { name: 'Sort the cards' })
    await pinTheDial(preset)
  }

  it('submits the drafting with the stance the sheet asked the dial with',
     async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(dial(true))
    await pasteAndPin('collaborator')

    // The toggle exists only because the dial said this stance may write.
    fireEvent.click(await screen.findByRole('button', { name: 'Draft the reasons' }))
    fireEvent.click(screen.getByText('Import as draft'))
    await waitFor(() => expect(api.intake).toHaveBeenCalled())

    // The premise, stated so the equality below cannot be satisfied by two
    // undefineds agreeing with each other — which is exactly what the broken
    // version did on a page whose user had never touched the dial.
    expect(decidedWith()).toBe('collaborator')

    const sent = vi.mocked(api.intake).mock.calls[0]![1]
    expect(sent.rationales).toBe(true)
    expect(sent.stance).toBe(decidedWith())
  })

  // The miss path, which is the half a shallower test leaves out. Below the
  // write axis there is nothing to tick — and the four actions that were never
  // gated still travel with the same stance, because the dial is the user's
  // and it applies to all five.
  it('offers no drafting below the write axis, and still carries the stance',
     async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(dial(false))
    await pasteAndPin('consultant')

    expect(screen.queryByRole('button', { name: 'Draft the reasons' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Sort the cards' }))
    fireEvent.click(screen.getByText('Import as draft'))
    await waitFor(() => expect(api.intake).toHaveBeenCalled())

    const sent = vi.mocked(api.intake).mock.calls[0]![1]
    expect(sent.rationales).toBeUndefined()
    expect(sent.categories).toBe(true)
    expect(decidedWith()).toBe('consultant')
    expect(sent.stance).toBe(decidedWith())
  })

  // **One value, and it is the sheet's own.** The dial moves — another tab,
  // the settings panel, a pin this build refused and dropped — the sheet asks
  // again, and the submit has to follow it there. A page holding its own copy
  // of the answer is the bug this whole file is about, one refactor later.
  it('follows the dial when it moves under the page', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(dial(true))
    await pasteAndPin('collaborator')
    fireEvent.click(await screen.findByRole('button', { name: 'Draft the reasons' }))

    await pinTheDial('second-opinion')
    fireEvent.click(screen.getByText('Import as draft'))
    await waitFor(() => expect(api.intake).toHaveBeenCalled())

    const sent = vi.mocked(api.intake).mock.calls[0]![1]
    expect(decidedWith()).toBe('second-opinion')
    expect(sent.stance).toBe('second-opinion')
  })
})

/* The extra work can fail, and the deck is still there.
 *
 * The intake runs after the deck has been created, in a different request, so
 * a failure here is about what was going to be *added*. Before this the
 * rejection fell to the page's outer handler: the server's own `no such job`
 * went on screen — lowercase machinery, commandment 10 — and the navigate was
 * skipped, stranding somebody on the import page with a saved deck and no door
 * offered to it. That is exactly what happened on the first real intake ever
 * run against a ninety-nine, because merging deploys here (ADR 23) and the job
 * registry is in memory.
 */
describe('when the extra work does not finish', () => {
  async function importWithLostJob(status = 404) {
    vi.mocked(api.importDeck).mockResolvedValue(result({ created: true }))
    // Built per call, and marked handled on the spot. A rejected promise
    // created at mock-setup time is unhandled until the page happens to await
    // it, which Node reports as a failure of this file rather than of the code.
    // `.catch` returns a new promise; the original still rejects for the page.
    vi.mocked(followJob).mockImplementation(() => {
      const promise = Promise.reject(new ApiError('no such job', status))
      promise.catch(() => { /* the page is the real handler */ })
      return { promise, cancel: () => {} }
    })
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    // Any ticked action starts the run, and this one is ungated — the lost
    // job is the same lost job whatever was asked for, and using the drafting
    // toggle here would drag ADR 41's write gate into a test about failure.
    fireEvent.click(await screen.findByRole('button', { name: 'Sort the cards' }))
    fireEvent.click(screen.getByText('Import as draft'))
  }

  it('says so in words, and never in the server\'s', async () => {
    await importWithLostJob()
    const note = await screen.findByText(/Your deck is saved and safe/)
    expect(note).toBeTruthy()
    // The machinery's own account of itself belongs in the log.
    expect(screen.queryByText(/no such job/)).toBeNull()
  })

  it('offers the door to the deck it did save', async () => {
    await importWithLostJob()
    const door = await screen.findByRole('link', { name: 'Open your deck' })
    // Owner-qualified, off the response, exactly as the happy path is.
    expect(door.getAttribute('href')).toBe('/decks/aasquier/arahbo-cats')
  })

  it('does not navigate away from the explanation', async () => {
    await importWithLostJob()
    await screen.findByText(/Your deck is saved and safe/)
    // Leaving would take the only account of what happened with it. The link
    // above is the way on, chosen rather than done to them.
    expect(navigate).not.toHaveBeenCalled()
  })

  it('names the restart when the work was lost, and not otherwise', async () => {
    await importWithLostJob(404)
    expect(await screen.findByText(/library was most likely restarting/)).toBeTruthy()
    cleanup()
    await importWithLostJob(500)
    expect(await screen.findByText(/Your deck is saved and safe/)).toBeTruthy()
    expect(screen.queryByText(/library was most likely restarting/)).toBeNull()
  })
})
