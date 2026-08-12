/** The gate: when the app asks for a login, and when it must not.
 *
 * `GET /api/auth/me` reports `auth_required` and `authenticated` as two flags,
 * and this file is the reason that costs a field. With auth off — the only way
 * `mtglab ui` has ever run, and how it runs on a laptop today — there is no
 * login screen, no sign-out button, and nothing else that would suggest this
 * instance has accounts. A gate in front of the local single-user app would be
 * a regression, and one collapsed boolean is how it would happen.
 *
 * `fetch` is stubbed rather than the API client mocked, so the 401 interceptor
 * is the real one: the test for an expiring session goes through
 * `api.decks -> refuse -> onSessionLost -> App`, which is the whole chain it
 * has to work through in a browser.
 */

import { cleanup, render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import type { AuthState } from './lib/api'

interface Reply { status?: number; body?: unknown }

let routes: Record<string, Reply | (() => Reply)> = {}
let fetchMock: ReturnType<typeof vi.fn>

function auth(overrides: Partial<AuthState> = {}): AuthState {
  return {
    auth_required: false,
    authenticated: false,
    is_admin: true,
    user: null,
    ...overrides,
  }
}

/** The signed-in shape: an account, on an instance that requires one. */
function signedIn(username = 'root', isAdmin = false): AuthState {
  return {
    auth_required: true,
    authenticated: true,
    is_admin: isAdmin,
    user: { id: 1, username, is_admin: isAdmin },
  }
}

function paths(): string[] {
  return fetchMock.mock.calls.map((call) => String(call[0]).split('?')[0])
}

function renderApp(at = '/') {
  return render(<MemoryRouter initialEntries={[at]}><App /></MemoryRouter>)
}

beforeEach(() => {
  routes = {
    '/api/health': { body: { corpus: true, oracle_cards: 21, printings: 21 } },
    '/api/decks': { body: [] },
    '/api/auth/me': { body: auth() },
    '/api/auth/logout': { body: { authenticated: false } },
  }
  fetchMock = vi.fn(async (input: string) => {
    const path = String(input).split('?')[0]
    const entry = routes[path]
    const reply = typeof entry === 'function' ? entry() : (entry ?? { status: 404, body: {} })
    const status = reply.status ?? 200
    return {
      ok: status < 400,
      status,
      statusText: status === 200 ? 'OK' : 'Refused',
      headers: { get: () => null },
      json: async () => reply.body ?? {},
    }
  })
  vi.stubGlobal('fetch', fetchMock)
  // jsdom has no `matchMedia`, and the theme hook asks it what the system
  // prefers before anything renders. Only `.matches` is read.
  vi.stubGlobal('matchMedia', (media: string) => ({ matches: false, media }))
  window.history.replaceState(null, '', '/')
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('with auth off', () => {
  it('looks exactly as it did before there was a login screen', async () => {
    renderApp()
    await screen.findByText('Card search')

    // No gate, no form, and nothing offering to sign anybody out of a session
    // that does not exist.
    expect(screen.queryByRole('button', { name: 'Sign in' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Sign out' })).toBeNull()
    expect(screen.queryByLabelText('Password')).toBeNull()
  })

  it('still offers the Accounts page, because the local caller is the admin', async () => {
    renderApp()
    // `is_admin` is true with auth off — there is nobody else for it to be
    // true relative to — so `mtglab users` as a page keeps working on a laptop.
    expect(await screen.findByText('Accounts')).toBeTruthy()
  })
})

describe('with auth on and nobody signed in', () => {
  beforeEach(() => {
    routes['/api/auth/me'] = { body: auth({ auth_required: true, is_admin: false }) }
  })

  it('asks for a login instead of the app', async () => {
    renderApp()

    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeTruthy()
    expect(screen.queryByText('Card search')).toBeNull()
    expect(screen.queryByText('Simulator')).toBeNull()
  })

  it('does not mount the screens behind it, so nothing fetches a 401', async () => {
    renderApp()
    await screen.findByRole('button', { name: 'Sign in' })

    // The nav being hidden is cosmetic; not mounting the library is the point.
    // This is the client mirroring what the middleware does before routing.
    expect(paths()).not.toContain('/api/decks')
  })

  it('shows the app once a login succeeds', async () => {
    routes['/api/auth/login'] = { body: { user: { id: 1, username: 'root', is_admin: false } } }
    renderApp()
    await screen.findByRole('button', { name: 'Sign in' })

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'root' } })
    fireEvent.change(screen.getByLabelText('Password'),
                     { target: { value: 'a-long-enough-password' } })
    // The session is the cookie; what makes the app appear is `me` answering
    // again, which is what the form asks the shell to do.
    routes['/api/auth/me'] = { body: signedIn() }
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(await screen.findByText('Card search')).toBeTruthy()
  })

  it('renders nothing at all until `me` has answered', async () => {
    // Otherwise a login form flashes on every load of an app that has no
    // login, for as long as one request takes.
    let answer: (() => void) | null = null
    const held = new Promise<void>((resolve) => { answer = resolve })
    routes['/api/auth/me'] = () => ({ body: auth({ auth_required: true }) })
    fetchMock.mockImplementation(async (input: string) => {
      if (String(input) === '/api/auth/me') await held
      return {
        ok: true, status: 200, statusText: 'OK',
        headers: { get: () => null },
        json: async () => (String(input) === '/api/auth/me'
          ? auth({ auth_required: true })
          : { corpus: false, oracle_cards: 0, printings: 0 }),
      }
    })

    renderApp()
    expect(screen.queryByRole('button', { name: 'Sign in' })).toBeNull()
    expect(screen.queryByText('Card search')).toBeNull()

    answer!()
    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeTruthy()
  })
})

describe('with auth on and somebody signed in', () => {
  beforeEach(() => {
    routes['/api/auth/me'] = { body: signedIn('ada') }
  })

  it('names who is signed in, and offers a way out', async () => {
    renderApp()
    await screen.findByText('Card search')

    expect(screen.getByText('ada')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeTruthy()
  })

  it('signs out to the login screen', async () => {
    renderApp()
    await screen.findByRole('button', { name: 'Sign out' })

    routes['/api/auth/me'] = { body: auth({ auth_required: true, is_admin: false }) }
    fireEvent.click(screen.getByRole('button', { name: 'Sign out' }))

    await waitFor(() => expect(paths()).toContain('/api/auth/logout'))
    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeTruthy()
  })

  it('falls back to the login screen when a session expires under it', async () => {
    // The real chain: a 401 from any endpoint reaches the interceptor in
    // api.ts, which announces a lost session; the shell re-asks the one
    // endpoint that answers without one rather than assuming what happened.
    renderApp()
    await screen.findByText('Card search')
    // Off the library and back, so its mount fetch runs again — the session
    // ends between the two, which is what it does in a browser.
    fireEvent.click(screen.getByText('Import'))

    routes['/api/decks'] = { status: 401, body: { detail: 'authentication required' } }
    routes['/api/auth/me'] = { body: auth({ auth_required: true, is_admin: false }) }
    fireEvent.click(screen.getByText('Library'))

    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeTruthy()
  })
})

describe('the emailed link', () => {
  it('opens the claim page even though nobody can sign in yet', async () => {
    // The audience for this page is precisely the people the gate would stop.
    routes['/api/auth/me'] = { body: auth({ auth_required: true, is_admin: false }) }
    window.history.replaceState(null, '', '/auth/claim#token=a-256-bit-token')
    renderApp('/auth/claim')

    expect(await screen.findByText('Choose a password')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Sign in' })).toBeNull()
  })

  it('opens on an instance with auth off too, rather than 404ing', async () => {
    window.history.replaceState(null, '', '/auth/claim#token=a-256-bit-token')
    renderApp('/auth/claim')

    expect(await screen.findByText('Choose a password')).toBeTruthy()
  })
})
