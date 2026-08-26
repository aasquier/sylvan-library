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

import { act, cleanup, render, screen, fireEvent, waitFor } from '@testing-library/react'
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
  return fetchMock.mock.calls.map((call) => String(call[0]).split('?')[0] ?? '')
}

function renderApp(at = '/') {
  return render(<MemoryRouter initialEntries={[at]}><App /></MemoryRouter>)
}

beforeEach(() => {
  routes = {
    '/api/health': { body: { pool: true, oracle_cards: 21, printings: 21 } },
    '/api/decks': { body: [] },
    '/api/auth/me': { body: auth() },
    '/api/auth/logout': { body: { authenticated: false } },
  }
  fetchMock = vi.fn(async (input: string) => {
    const path = String(input).split('?')[0] ?? ''
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

  it('still offers the Admin page, because the local caller is the admin', async () => {
    renderApp()
    // `is_admin` is true with auth off — there is nobody else for it to be
    // true relative to — so `mtglab users` as a page keeps working on a laptop.
    expect(await screen.findByText('Admin')).toBeTruthy()
  })
})

describe('the nav', () => {
  it('labels the research surface as the Laboratory, at the same address', async () => {
    renderApp()
    const lab = await screen.findByRole('link', { name: 'Laboratory' })
    expect(lab.getAttribute('href')).toBe('/research')
    expect(lab.getAttribute('title')).toMatch(/ask about magic/i)
  })

  it('gives every entry a hover hint saying what is behind it', async () => {
    renderApp()
    await screen.findByRole('link', { name: 'Laboratory' })
    for (const label of ['Library', 'Start a deck', 'Import', 'Card search',
                         'Simulator', 'Laboratory', 'Learn', 'About Claude',
                         'Admin']) {
      const title = screen.getByRole('link', { name: label }).getAttribute('title')
      expect(title, `${label} has no hint`).toBeTruthy()
    }
  })
})

/*
 * The header's fold (punch list 2026-08-26: "our top menu is awkward, it
 * really should fold up and disappear like the ivy when we scroll down").
 *
 * `lib/canopy.test.ts` holds the arithmetic — thresholds, direction, the dead
 * zone. What is left for this file is the wiring: that the bar is the thing
 * wearing the class, and that folding it never costs the keyboard its way in.
 * That second one is the whole reason the CSS uses a transform and an opacity
 * instead of `display: none`, and it is the kind of decision a later
 * "simplification" undoes without any visible symptom.
 */
describe('the header fold', () => {
  const header = () => document.querySelector('header.site-header')

  function scrollTo(y: number) {
    window.scrollY = y
    fireEvent.scroll(window)
  }

  /**
   * Mount, and do not come back until the canopy is actually listening.
   *
   * The bar is not in the first commit: the app renders its gate, asks
   * `/api/auth/me` and `/api/health`, and only then puts the nav on the page.
   * The canopy attaches its scroll listener from a passive effect, which is a
   * flush *after* that commit. `findBy*` resolves the instant the DOM holds
   * the link — which can be the commit and not the flush — and a scroll fired
   * in between is read by nobody at all: no listener, no reading, the bar
   * never moves, and the failure is indistinguishable from the fold being
   * broken. That was the flake #324 shipped, and it is load-sensitive because
   * what varies is how much of React's pending work gets a turn.
   *
   * So: `findBy*` for the commit, then `act` for the effects it scheduled.
   * `act` is a flush, not a wait — nothing here polls, times out, or gives a
   * wrong answer a second chance to be right.
   */
  async function openAtTheTop() {
    renderApp()
    await screen.findByRole('link', { name: 'Laboratory' })
    await act(async () => {})
  }

  afterEach(() => {
    window.scrollY = 0
  })

  it('is open at the top of a page and folds on the way down', async () => {
    await openAtTheTop()
    expect(header()?.className).not.toContain('is-furled')

    scrollTo(600)
    expect(header()?.className).toContain('is-furled')

    // And comes back on the way up, deep in the page — never only at the top.
    scrollTo(300)
    expect(header()?.className).not.toContain('is-furled')
  })

  it('keeps the nav reachable by keyboard while it is folded', async () => {
    // A focused link nobody can see is a real bug. The bar leaves on a
    // transform for exactly this reason: `display: none` and
    // `visibility: hidden` both take these links out of the tab order, and
    // then `:focus-within` — which is what brings the bar back — can never
    // fire, because focus can never get in. So the guard is that a Tab still
    // lands, folded or not; the unfolding it triggers is CSS, and CSS is
    // empty in this suite.
    await openAtTheTop()
    const lab = screen.getByRole('link', { name: 'Laboratory' })

    scrollTo(600)
    expect(header()?.className).toContain('is-furled')

    lab.focus()
    expect(document.activeElement).toBe(lab)
  })

  /**
   * The bar asks the stylesheet where it sits, never a utility class.
   *
   * This is the guard on a real regression (Aaron, 2026-08-26: "when I fast
   * scroll up the top menu appears awkwardly under the main page"). The bar
   * carried a z-index utility, and this was the *only* place that class was
   * written in the whole frontend. #324 rewrote the line into a template
   * literal that put it flush against the interpolation, the scanner that
   * decides which utility rules get generated stopped seeing it, and no rule
   * was emitted — so the computed z-index was `auto`, which loses to the 10
   * that `.page-main` carries. The header was underneath the page: invisible while
   * folded, buried under whatever was on screen the moment a scroll up brought
   * it back, and swallowing the taps aimed at the nav the whole time.
   *
   * Nothing about that is visible from here, and this test does not pretend
   * otherwise. jsdom has no layout, and this suite is handed the stylesheet as
   * the empty string however it asks for it — through `?raw`, through
   * `import.meta.glob`, and `node:fs` neither typechecks nor runs under this
   * config (all three measured, 2026-08-26). So the paint order itself is a
   * browser's answer and was verified in one.
   *
   * What *is* checkable here is the decision that made the bar safe: its
   * stacking is a rule in `index.css` keyed on `.site-header`, not a request
   * for a class that something else has to generate. A `z-` utility reappearing
   * on this element is the tempting one-word "fix" for a stacking bug, it is
   * exactly what was already there, and it can silently do nothing at all.
   *
   * Note that neither the assertion nor this prose spells such a class out.
   * They cannot: every file under `src/` is scanned for candidates — tests and
   * comments included — so writing one here generates its rule and papers over
   * the exact failure this guard exists to catch. Measured, not assumed: the
   * first draft of this comment named it, and the class reappeared in the
   * built stylesheet.
   */
  it('leaves its stacking to the stylesheet, not to a generated utility',
     async () => {
    await openAtTheTop()
    const classes = [...(header()?.classList ?? [])]

    expect(classes).toContain('site-header')
    expect(classes.filter((c) => /^z-/.test(c)),
      'the header wears a z-index utility again — put it in `.site-header`')
      .toHaveLength(0)
  })
})

describe('the ambience switch', () => {
  it('removes the weather layer entirely when opted out', async () => {
    localStorage.setItem('mtglab-ambience', '0')
    renderApp()
    await screen.findByText('Card search')
    expect(document.querySelector('.forest-ambience')).toBeNull()
    localStorage.removeItem('mtglab-ambience')
  })

  it('the weather is on by default — the character is not opt-in', async () => {
    renderApp()
    await screen.findByText('Card search')
    expect(document.querySelector('.forest-ambience')).toBeTruthy()
    expect(document.querySelectorAll('.firefly').length).toBeGreaterThan(0)
  })
})

describe('the Claude menu in the header', () => {
  const claude = {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: { preset: 'consultant', allows_calls: true, may_write: false, axes: [] },
    ceiling: { preset: null, allows_calls: true, may_write: false, axes: [] },
    default: { preset: 'consultant', allows_calls: true, may_write: false, axes: [] },
    presets: [
      { name: 'off', blurb: 'No calls.', available: true,
        stance: { preset: 'off', allows_calls: false, may_write: false, axes: [] } },
    ],
    never: 'One rule holds at every setting: Claude never writes a card’s rationale. The why is always yours.',
    modes: [],
  }

  it('shows on the settings gear when this instance has Claude configured', async () => {
    routes['/api/claude'] = { body: claude }
    renderApp()
    // The gear itself is unconditional; the Claude readout on it is not.
    expect(await screen.findByRole('button', { name: 'Settings' })).toBeTruthy()
    expect(await screen.findByText(/Claude ·/)).toBeTruthy()
  })

  it('stays out of the gear when it is not', async () => {
    // The default mock 404s `/api/claude`, which is what an instance without
    // the extra effectively is to this panel: nothing to control. The gear
    // still renders — theme, ambience and sound are about the person.
    renderApp()
    await screen.findByText('Card search')
    expect(screen.getByRole('button', { name: 'Settings' })).toBeTruthy()
    expect(screen.queryByText(/Claude ·/)).toBeNull()
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
          : { pool: false, oracle_cards: 0, printings: 0 }),
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

  it('offers the authoring doors to somebody who is not an admin', async () => {
    // They were gated on `is_admin` because there was one library and it was
    // the maintainer's, so a deck started by anybody else had nowhere to go.
    // Every account has its own library now (ADR 22) and the gate is gone —
    // not moved somewhere better, gone, which is what it said it would do.
    renderApp()
    await screen.findByText('Card search')

    expect(screen.getByText('Start a deck')).toBeTruthy()
    expect(screen.getByText('Import')).toBeTruthy()
    // Admin is a different question and still an admin's. ADR 17's prefix
    // rule refuses `/api/admin` to everybody else before routing.
    expect(screen.queryByText('Admin')).toBeNull()
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
    //
    // Signed in as an admin purely so `Import` is in the nav to navigate
    // through — this test is about a session ending, not about who may write,
    // and it needs some second lazy screen to leave the library for.
    routes['/api/auth/me'] = { body: signedIn('ada', true) }
    renderApp()
    await screen.findByText('Card search')
    // Off the library and back, so its mount fetch runs again — the session
    // ends between the two, which is what it does in a browser. The Import
    // screen is lazy, and the router keeps the old screen up until the chunk
    // lands, so wait for it to actually render: clicking straight back would
    // leave Library mounted the whole time and its fetch never re-run.
    fireEvent.click(screen.getByText('Import'))
    await screen.findByText('Import a decklist')

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
