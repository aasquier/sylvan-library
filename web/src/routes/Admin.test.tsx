/** The accounts page: what it offers, and the two things it must never offer.
 *
 * The page is not the protection — every route it calls is refused to a
 * non-admin by the middleware before routing (ADR 17), and `tests/test_isolation.py`
 * is what proves that. What is worth pinning here is the pair of server rules
 * the UI *reflects*, because a UI that reflects them wrongly is a UI that
 * offers a click which can only fail:
 *
 * - the last admin who can sign in is not offered Demote or Disable;
 * - there is no password field anywhere on the page, ever (ADR 16).
 *
 * The second is asserted structurally rather than by exercising a flow. "Just
 * set it for them" is a helpful-looking change at all times, and a test that
 * only walks today's buttons would pass straight through it.
 */

import { cleanup, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Account, AccountList } from '../lib/api'
import Admin from './Admin'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: {
      accounts: vi.fn(),
      inviteAccount: vi.fn(),
      updateAccount: vi.fn(),
      sendReset: vi.fn(),
      revokeSessions: vi.fn(),
      deleteAccount: vi.fn(),
      me: vi.fn(),
      adminSystem: vi.fn(),
      adminStorage: vi.fn(),
      adminClaude: vi.fn(),
      adminActivity: vi.fn(),
      adminTraffic: vi.fn(),
      adminFly: vi.fn(),
    },
  }
})

const { api } = await import('../lib/api')

function account(overrides: Partial<Account> & { username: string }): Account {
  return {
    id: overrides.username.length,
    email: `${overrides.username}@example.com`,
    is_admin: false,
    disabled: false,
    created_at: '2026-08-12T00:00:00+00:00',
    state: 'active',
    sessions: 0,
    ...overrides,
  }
}

/** One admin and one ordinary account — the shape of this deployment. */
const SOLO: AccountList = {
  admins: 1,
  users: [account({ username: 'root', is_admin: true, sessions: 2 }),
          account({ username: 'friend' })],
}

function rowFor(username: string) {
  return screen.getByText(username).closest('tr') as HTMLElement
}

beforeEach(() => {
  vi.mocked(api.accounts).mockResolvedValue(SOLO)
  // Signed in as `root`, which is what makes the self-delete guard testable.
  vi.mocked(api.me).mockResolvedValue({
    auth_required: true,
    authenticated: true,
    is_admin: true,
    user: { id: 4, username: 'root', is_admin: true },
  })
  // The dashboard asks all four on mount. Answers with data in them so the
  // tests below can also assert the tiles render what the box reported.
  vi.mocked(api.adminSystem).mockResolvedValue({
    process: { bytes: 120 * 1024 * 1024, kind: 'current' },
    memory: { total_bytes: 1024 ** 3, available_bytes: 512 * 1024 ** 2 },
    load: [0.12, 0.2, 0.25],
    cpus: 1,
    disk: { path: '/data', total_bytes: 10 * 1024 ** 3,
            used_bytes: 4 * 1024 ** 3, free_bytes: 6 * 1024 ** 3 },
  })
  vi.mocked(api.adminStorage).mockResolvedValue({
    app_db_bytes: 2 * 1024 ** 2,
    pool_bytes: null,
    scryfall_bulk_bytes: null,
    cache_bytes: 3 * 1024 ** 2,
    cache: { symbols_bytes: 1024, cardmotion_bytes: null },
    decks: { count: 7, bytes: 90 * 1024, trashed: 1 },
  })
  vi.mocked(api.adminClaude).mockResolvedValue({
    windows: {
      week: [],
      month: [{ mode: 'dossier', conversations: 2, requests: 6,
                input_tokens: 2000, output_tokens: 400,
                cache_read_tokens: 800,
                first_at: '2026-08-10T00:00:00+00:00',
                last_at: '2026-08-17T00:00:00+00:00' }],
      all: [],
    },
    caveat: 'Token counts are a floor on the bill, not the bill.',
  })
  vi.mocked(api.adminActivity).mockResolvedValue({
    accounts: { active: 2 },
    sessions: { total: 3, seen_day: 1, seen_week: 2 },
    deck_edits_by_day: [{ day: '2026-08-17', edits: 4 }],
    sim_cache_rows: 12,
    jobs: { running: 1 },
  })
  vi.mocked(api.adminTraffic).mockResolvedValue({
    days: [{ day: '2026-08-17', total: 41, '2xx': 38, '4xx': 3 }],
    top_routes: [{ route: '/api/decks/{owner}/{slug}', count: 17 },
                 { route: '/api/health', count: 12 }],
    note: 'Route templates and status classes only — the ledger never '
      + 'records an address, an agent, a name, or a concrete path.',
  })
  // Unconfigured by default, which is what a laptop is — the tests that
  // are about the glass configure it themselves.
  vi.mocked(api.adminFly).mockResolvedValue({
    configured: false, ok: false, values: {},
  })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the account list', () => {
  it('shows the state, the address and the live session count', async () => {
    render(<Admin />)
    await screen.findByText('root')

    const row = rowFor('root')
    expect(within(row).getByText('admin')).toBeTruthy()
    expect(within(row).getByText('root@example.com')).toBeTruthy()
    expect(within(row).getByText('active')).toBeTruthy()
    expect(within(row).getByText('2')).toBeTruthy()
  })

  it('says how many admins can actually sign in', async () => {
    render(<Admin />)
    expect(await screen.findByText('1 admin can sign in')).toBeTruthy()
  })

  it('reports a failure to load rather than rendering an empty table', async () => {
    vi.mocked(api.accounts).mockRejectedValue(new Error('admin only'))
    render(<Admin />)
    expect(await screen.findByText('admin only')).toBeTruthy()
  })
})

describe('the last admin', () => {
  it('is offered neither Demote nor Disable', async () => {
    render(<Admin />)
    await screen.findByText('root')

    const row = rowFor('root')
    expect(within(row).getByText('Demote').hasAttribute('disabled')).toBe(true)
    expect(within(row).getByText('Disable').hasAttribute('disabled')).toBe(true)
  })

  it('says why, so a greyed-out button is not a mystery', async () => {
    render(<Admin />)
    await screen.findByText('root')

    // Both greyed-out buttons carry it, which is the point — whichever one
    // somebody hovers, they get the reason rather than a dead control.
    expect(within(rowFor('root')).getAllByTitle(/only admin who can sign in/i))
      .toHaveLength(2)
  })

  it('stops being the last one when a second admin exists', async () => {
    vi.mocked(api.accounts).mockResolvedValue({
      admins: 2,
      users: [account({ username: 'root', is_admin: true }),
              account({ username: 'heir', is_admin: true })],
    })
    render(<Admin />)
    await screen.findByText('root')

    expect(within(rowFor('root')).getByText('Demote').hasAttribute('disabled'))
      .toBe(false)
  })

  it('still surfaces the server refusal if one arrives anyway', async () => {
    // The buttons are a courtesy; the 409 is the rule. A page that swallowed it
    // would leave somebody believing a demotion had happened.
    vi.mocked(api.accounts).mockResolvedValue({
      admins: 2,
      users: [account({ username: 'root', is_admin: true }),
              account({ username: 'heir', is_admin: true })],
    })
    vi.mocked(api.updateAccount).mockRejectedValue(
      new Error('refusing to revoke admin: this is the only admin who can sign in.'))
    render(<Admin />)
    await screen.findByText('root')

    fireEvent.click(within(rowFor('root')).getByText('Demote'))
    expect(await screen.findByText(/only admin who can sign in/)).toBeTruthy()
  })
})

describe('the levers', () => {
  it('promotes through one route and reloads the list', async () => {
    vi.mocked(api.updateAccount).mockResolvedValue(
      account({ username: 'friend', is_admin: true }))
    render(<Admin />)
    await screen.findByText('friend')

    fireEvent.click(within(rowFor('friend')).getByText('Promote'))

    await waitFor(() => {
      expect(api.updateAccount).toHaveBeenCalledWith('friend', { is_admin: true })
    })
    // Reloaded, not patched locally: the server owns `admins`, and a page that
    // guessed it would grey out the wrong button.
    await waitFor(() => expect(api.accounts).toHaveBeenCalledTimes(2))
  })

  it('offers a reset link instead of a password', async () => {
    vi.mocked(api.sendReset).mockResolvedValue({ detail: 'a reset link is on its way' })
    render(<Admin />)
    await screen.findByText('friend')

    fireEvent.click(within(rowFor('friend')).getByText('Send reset link'))

    await waitFor(() => expect(api.sendReset).toHaveBeenCalledWith('friend'))
    expect(await screen.findByText(/on its way/)).toBeTruthy()
  })

  it('does not offer a reset link to a disabled account', async () => {
    vi.mocked(api.accounts).mockResolvedValue({
      admins: 1,
      users: [account({ username: 'root', is_admin: true }),
              account({ username: 'friend', disabled: true, state: 'disabled' })],
    })
    render(<Admin />)
    await screen.findByText('friend')

    const button = within(rowFor('friend')).getByText('Send reset link')
    expect(button.hasAttribute('disabled')).toBe(true)
  })

  it('does not offer to revoke sessions an account does not have', async () => {
    render(<Admin />)
    await screen.findByText('friend')

    expect(within(rowFor('friend')).getByText('Sign out everywhere')
      .hasAttribute('disabled')).toBe(true)
    expect(within(rowFor('root')).getByText('Sign out everywhere')
      .hasAttribute('disabled')).toBe(false)
  })
})

describe('inviting', () => {
  it('sends the address and lets the server default the username', async () => {
    vi.mocked(api.inviteAccount).mockResolvedValue(account({ username: 'new.person' }))
    render(<Admin />)
    await screen.findByText('root')

    fireEvent.change(screen.getByPlaceholderText('them@example.com'),
                     { target: { value: 'new.person@example.com' } })
    fireEvent.click(screen.getByText('Send invite'))

    await waitFor(() => {
      expect(api.inviteAccount).toHaveBeenCalledWith({
        email: 'new.person@example.com',
      })
    })
  })

  it('reports the refusal when the address is already claimed', async () => {
    vi.mocked(api.inviteAccount).mockRejectedValue(
      new Error('friend has already claimed that address -- send them a reset link'))
    render(<Admin />)
    await screen.findByText('root')

    fireEvent.change(screen.getByPlaceholderText('them@example.com'),
                     { target: { value: 'friend@example.com' } })
    fireEvent.click(screen.getByText('Send invite'))

    expect(await screen.findByText(/already claimed/)).toBeTruthy()
  })
})

describe('deleting an account', () => {
  it('does not delete on the first click — it asks for the username', async () => {
    render(<Admin />)
    await screen.findByText('friend')

    fireEvent.click(within(rowFor('friend')).getByText('Delete'))

    expect(api.deleteAccount).not.toHaveBeenCalled()
    expect(within(rowFor('friend')).getByPlaceholderText('type friend')).toBeTruthy()
  })

  it('keeps the confirm button dead until the name matches', async () => {
    render(<Admin />)
    await screen.findByText('friend')
    fireEvent.click(within(rowFor('friend')).getByText('Delete'))

    const row = rowFor('friend')
    const confirm = within(row).getByText('Delete for good') as HTMLButtonElement
    expect(confirm.disabled).toBe(true)

    fireEvent.change(within(row).getByPlaceholderText('type friend'),
                     { target: { value: 'freind' } })
    expect((within(row).getByText('Delete for good') as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(within(row).getByPlaceholderText('type friend'),
                     { target: { value: 'friend' } })
    expect((within(row).getByText('Delete for good') as HTMLButtonElement).disabled).toBe(false)
  })

  it('sends the typed name and reports what went with the account', async () => {
    vi.mocked(api.deleteAccount).mockResolvedValue({
      username: 'friend', revoked: 2, jobs_dropped: 1,
    })
    render(<Admin />)
    await screen.findByText('friend')
    fireEvent.click(within(rowFor('friend')).getByText('Delete'))
    fireEvent.change(within(rowFor('friend')).getByPlaceholderText('type friend'),
                     { target: { value: 'friend' } })
    fireEvent.click(within(rowFor('friend')).getByText('Delete for good'))

    await waitFor(() => {
      expect(api.deleteAccount).toHaveBeenCalledWith('friend', 'friend')
    })
    expect(await screen.findByText(/friend is gone/)).toBeTruthy()
    expect(await screen.findByText(/2 session\(s\) ended, 1 job\(s\) dropped/)).toBeTruthy()
  })

  it('can be cancelled without deleting anything', async () => {
    render(<Admin />)
    await screen.findByText('friend')
    fireEvent.click(within(rowFor('friend')).getByText('Delete'))
    fireEvent.click(within(rowFor('friend')).getByText('Cancel'))

    expect(api.deleteAccount).not.toHaveBeenCalled()
    expect(within(rowFor('friend')).getByText('Delete')).toBeTruthy()
  })

  it('does not offer it on the row the caller is signed in as', async () => {
    render(<Admin />)
    await screen.findByText('root')

    // The server answers 409 to this too; greying it is the courtesy, and the
    // title is where the CLI alternative is named.
    await waitFor(() => {
      const own = within(rowFor('root')).getByText('Delete') as HTMLButtonElement
      expect(own.disabled).toBe(true)
    })
    expect((within(rowFor('friend')).getByText('Delete') as HTMLButtonElement).disabled)
      .toBe(false)
  })

  it('reports a refusal against the row it was about', async () => {
    vi.mocked(api.deleteAccount).mockRejectedValue(
      new Error('refusing to delete that account: this is the only admin who can sign in.'))
    render(<Admin />)
    await screen.findByText('friend')
    fireEvent.click(within(rowFor('friend')).getByText('Delete'))
    fireEvent.change(within(rowFor('friend')).getByPlaceholderText('type friend'),
                     { target: { value: 'friend' } })
    fireEvent.click(within(rowFor('friend')).getByText('Delete for good'))

    expect(await screen.findByText(/only admin who can sign in/)).toBeTruthy()
  })
})

describe('the rule that does not bend', () => {
  it('has no password input anywhere on the page', async () => {
    const { container } = render(<Admin />)
    await screen.findByText('root')

    expect(container.querySelectorAll('input[type="password"]')).toHaveLength(0)
    // Nothing named for one either, which is what a hand-rolled text field
    // would look like. ADR 16: no password is ever chosen by one person for
    // another, and this page is where that would be convenient to break.
    for (const input of container.querySelectorAll('input')) {
      const described = `${input.name} ${input.placeholder} ${input.getAttribute('aria-label') ?? ''}`
      expect(described.toLowerCase()).not.toContain('password')
    }
  })
})

describe('the dashboard', () => {
  it('wears the masthead and renders what the box reported', async () => {
    render(<Admin />)
    await screen.findByText('root')

    // The rename: the page is Admin now, and the masthead owns the h1.
    expect(screen.getByRole('heading', { level: 1, name: 'Admin' })).toBeTruthy()
    expect(screen.getByText(/Minttu Hynninen/)).toBeTruthy()

    // A present store is sized, an absent one is an em-dash — never a zero.
    expect(await screen.findByText('120 MB')).toBeTruthy()
    const poolTile = screen.getByText('Card pool').closest('div') as HTMLElement
    expect(within(poolTile).getByText('—')).toBeTruthy()
    const decksTile = screen.getByText('Decks on the volume')
      .closest('div') as HTMLElement
    expect(within(decksTile).getByText('7')).toBeTruthy()
  })

  it('shows the ledger with its caveat riding along', async () => {
    render(<Admin />)
    await screen.findByText('root')

    // The month window is the default and holds the one recorded mode.
    expect(await screen.findByText('dossier')).toBeTruthy()
    expect(screen.getByText('2,000')).toBeTruthy()
    // The caveat is the server's sentence, rendered, not paraphrased away.
    expect(screen.getByText(/floor on the bill/)).toBeTruthy()

    // Switching windows re-reads the same payload — an empty one says so.
    fireEvent.click(screen.getByText('7 days'))
    expect(await screen.findByText(/No conversations recorded/)).toBeTruthy()
  })

  it('reports the job registry as counts, never labels', async () => {
    render(<Admin />)
    await screen.findByText('root')

    expect(await screen.findByText('1 running')).toBeTruthy()
  })
})

describe('the visitor ledger', () => {
  it('renders templates and counts, with the privacy note beside them', async () => {
    render(<Admin />)
    await screen.findByText('root')

    expect(await screen.findByText('Visitors, last thirty days')).toBeTruthy()
    // The top routes are templates — the payload's own shape — and the
    // note travels with the numbers, the same rule the ledger panel keeps.
    expect(screen.getByText('/api/decks/{owner}/{slug}')).toBeTruthy()
    expect(screen.getByText(/never records an address/)).toBeTruthy()
  })

  it('stays absent while the ledger has nothing to show', async () => {
    vi.mocked(api.adminTraffic).mockResolvedValue({
      days: [], top_routes: [], note: 'n/a',
    })
    render(<Admin />)
    await screen.findByText('root')

    expect(screen.queryByText('Visitors, last thirty days')).toBeNull()
  })
})

describe('the far-seeing glass', () => {
  it('is absent entirely when no token is configured', async () => {
    render(<Admin />)
    await screen.findByText('root')

    // A panel of em-dashes reads as breakage; absence reads as "not set
    // up", which is what an instance without the token actually is.
    expect(screen.queryByText('The far-seeing glass')).toBeNull()
  })

  it('shows what the platform sees when it is configured', async () => {
    vi.mocked(api.adminFly).mockResolvedValue({
      configured: true, ok: true, app: 'sylvan-library', org: 'personal',
      values: { memory_bytes: 256 * 1024 ** 2,
                memory_total_bytes: 1024 ** 3,
                edge_2xx: 1240.4, edge_4xx: 12, edge_5xx: null },
    })
    render(<Admin />)
    await screen.findByText('root')

    expect(await screen.findByText('The far-seeing glass')).toBeTruthy()
    expect(screen.getByText('256 MB')).toBeTruthy()
    // A float from `increase()` renders as a whole number of requests.
    expect(screen.getByText('1,240')).toBeTruthy()
    // An empty series is an em-dash, never a zero.
    const failed = screen.getByText('Edge failed').closest('div') as HTMLElement
    expect(within(failed).getByText('—')).toBeTruthy()
  })

  it('says the glass is clouded when the platform cannot be reached', async () => {
    vi.mocked(api.adminFly).mockResolvedValue({
      configured: true, ok: false, error: 'Fly answered HTTP 401', values: {},
    })
    render(<Admin />)
    await screen.findByText('root')

    expect(await screen.findByText(/glass is clouded/)).toBeTruthy()
    expect(screen.getByText(/HTTP 401/)).toBeTruthy()
    // The rest of the dashboard is unaffected — the box's own numbers stay.
    expect(screen.getByText('120 MB')).toBeTruthy()
  })
})
