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
