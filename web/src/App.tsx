import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import { NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import Claim from './routes/Claim'
import Library from './routes/Library'
import Login from './routes/Login'
import { Spinner } from './components/ui'
import { api, onSessionLost, type AuthState, type Health } from './lib/api'

// Route-level code splitting. Library stays eager because it is the landing
// page — a spinner on first paint would tax every visit to pay for none — and
// Login/Claim stay eager because they are the gate, which must not depend on
// a second fetch succeeding. Everything else loads when navigated to, which
// keeps Recharts (the single heaviest dependency, used only by DeckDetail and
// Simulator) out of the entry chunk entirely.
const Admin = lazy(() => import('./routes/Admin'))
const CardSearch = lazy(() => import('./routes/CardSearch'))
const DeckDetail = lazy(() => import('./routes/DeckDetail'))
const DeckRedirect = lazy(() => import('./routes/DeckRedirect'))
const Import = lazy(() => import('./routes/Import'))
const Learn = lazy(() => import('./routes/Learn'))
const NewDeck = lazy(() => import('./routes/NewDeck'))
const Simulator = lazy(() => import('./routes/Simulator'))

const NAV = [
  { to: '/', label: 'Library', end: true },
  { to: '/search', label: 'Card search', end: false },
  // `/new` and `/import` are spliced in after Library — see AUTHORING_NAV.
  { to: '/simulate', label: 'Simulator', end: false },
  // Last, and deliberately not first: it is reference rather than a task, and
  // somebody who needs it usually arrives from a word on another screen.
  { to: '/learn', label: 'Learn', end: false },
]

// Appended for admins only. Hiding it is a courtesy — every route the page
// calls is refused to anybody else by the middleware, before routing (ADR 17),
// so this decides what is offered and never what is allowed.
const ADMIN_NAV = { to: '/admin', label: 'Accounts', end: false }

// The two doors that create a deck, and they are shown to everybody.
//
// They used to be gated on `is_admin`, because there was exactly one library
// and it was the maintainer's: a deck started by anybody else had nowhere to
// go but onto somebody else's shelf. That gate said it would not move
// somewhere better when decks got owners — it would **disappear** — and this
// is that. Every account has its own library now (ADR 22), so there is nobody
// left for whom these doors open onto nothing.
const AUTHORING_NAV = [
  { to: '/new', label: 'Start a deck', end: false },
  { to: '/import', label: 'Import', end: false },
]

/** Where the emailed link lands. Mirrors `auth.invites.CLAIM_PATH`. */
const CLAIM_PATH = '/auth/claim'

type Theme = 'light' | 'dark'

function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem('mtglab-theme')
    if (saved === 'light' || saved === 'dark') return saved
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    localStorage.setItem('mtglab-theme', theme)
  }, [theme])
  return [theme, () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))]
}

function ThemeButton({ theme, onToggle }: { theme: Theme; onToggle: () => void }) {
  return (
    <button onClick={onToggle}
            aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
            className="rounded-md px-2 py-1.5 text-sm"
            style={{ color: 'var(--text-secondary)', border: '1px solid var(--hairline)' }}>
      {theme === 'dark' ? '☀' : '☾'}
    </button>
  )
}

/**
 * The chrome the two logged-out screens get: a wordmark, a theme toggle, and
 * nothing else.
 *
 * No nav and no health line, deliberately. Every link in the nav goes somewhere
 * the server is about to refuse, and a header that offers them is a header that
 * lies about what this session can do.
 */
function AuthScreen({ theme, onToggleTheme, children }: {
  theme: Theme
  onToggleTheme: () => void
  children: React.ReactNode
}) {
  return (
    <div className="mx-auto flex min-h-screen w-full max-w-md flex-col justify-center px-6 py-12">
      <div className="mb-5 flex items-center justify-between">
        <span className="flex items-center gap-2">
          <span aria-hidden className="text-lg">🌳</span>
          <span className="font-semibold tracking-tight">sylvan-library</span>
        </span>
        <ThemeButton theme={theme} onToggle={onToggleTheme} />
      </div>
      {children}
    </div>
  )
}

/**
 * The app, with a gate in front of it that only exists on an instance that
 * asked for one.
 *
 * `GET /api/auth/me` reports `auth_required` and `authenticated` separately, and
 * this is what that separation is for. With auth off — which is how `mtglab ui`
 * runs on a laptop, and the only way it has ever run — `auth_required` is false,
 * the gate is not reached, and there is no login screen, no sign-out button and
 * no sign of any of this. Collapsing the two flags into one boolean would put a
 * sign-in form in front of a single-user local app that has no server for it.
 *
 * When auth *is* on, the gate replaces the whole shell rather than living
 * beside it as a route, which mirrors what the server does: the middleware
 * refuses every path outside `PUBLIC_PATHS` before routing, so there is no
 * half-logged-in view for a nav bar to be useful in.
 *
 * The one thing that renders ahead of the gate is the claim page. Its whole
 * audience is people who cannot log in yet — that is what the emailed link is
 * for — so a gate in front of it would be a door locked with its own key
 * inside.
 */
export default function App() {
  const [theme, toggleTheme] = useTheme()
  const [health, setHealth] = useState<Health | null>(null)
  const [auth, setAuth] = useState<AuthState | null>(null)
  const [checked, setChecked] = useState(false)
  const [prefill, setPrefill] = useState('')
  const location = useLocation()
  const navigate = useNavigate()

  const refreshAuth = useCallback(async () => {
    try {
      // `is_admin` and not `user?.is_admin`: with auth off the caller is the
      // local single user, who is an admin and is authenticated as nobody, so
      // the nested flag is null exactly when the page is most usable.
      setAuth(await api.me())
    } catch {
      // An instance that cannot answer the public endpoint cannot be logged
      // into either. Falling through to the app is what it did before there
      // was a login screen, and the individual routes report their own errors.
      setAuth(null)
    } finally {
      setChecked(true)
    }
  }, [])

  useEffect(() => {
    api.health().then(setHealth).catch(() => setHealth(null))
    void refreshAuth()
  }, [refreshAuth])

  // A 401 anywhere means the session ended under us — expired, revoked from the
  // Accounts page, or signed out in another tab. Re-ask rather than assume:
  // `me` is public, so it answers when nothing else will, and it distinguishes
  // "logged out" from "this instance stopped requiring a login".
  useEffect(() => onSessionLost(() => { void refreshAuth() }), [refreshAuth])

  async function signOut() {
    try {
      await api.logout()
    } finally {
      await refreshAuth()
    }
  }

  if (location.pathname === CLAIM_PATH) {
    return (
      <AuthScreen theme={theme} onToggleTheme={toggleTheme}>
        <Claim onClaimed={(username) => {
          // Claiming issues no session, so this is a hand-off and not a
          // redirect-after-login: the username travels to the form the gate is
          // about to render.
          setPrefill(username)
          navigate('/')
        }} />
      </AuthScreen>
    )
  }

  // Nothing at all until `me` has answered. A login form rendered for the half
  // second before it lands would flash on every single load of the local app,
  // which has no login.
  if (!checked) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner label="Loading…" />
      </div>
    )
  }

  if (auth?.auth_required && !auth.authenticated) {
    return (
      <AuthScreen theme={theme} onToggleTheme={toggleTheme}>
        <Login initialUsername={prefill} onSignedIn={refreshAuth} />
      </AuthScreen>
    )
  }

  // Library first, then the authoring doors — everybody has somewhere to put a
  // deck now — then the rest, then Accounts for an admin.
  const nav = [
    NAV[0], ...AUTHORING_NAV, ...NAV.slice(1),
    ...(auth?.is_admin ? [ADMIN_NAV] : []),
  ]
  // Only ever true on an instance that requires a login. With auth off there is
  // nobody to be signed out of, and offering it would be the regression this
  // whole flag exists to avoid.
  const signedIn = Boolean(auth?.auth_required && auth.authenticated)

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-40 backdrop-blur"
              style={{
                background: 'color-mix(in srgb, var(--page) 88%, transparent)',
                borderBottom: '1px solid var(--hairline)',
              }}>
        <div className="mx-auto flex max-w-7xl items-center gap-6 px-6 py-3">
          {/* `shrink-0` and no wrapping: the signed-in name and Sign out add a
              second cluster to this row, and without it a narrow window breaks
              the wordmark across two lines before it touches anything else. */}
          <NavLink to="/" className="flex shrink-0 items-center gap-2 whitespace-nowrap">
            <span aria-hidden className="text-lg">🌳</span>
            <span className="font-semibold tracking-tight">sylvan-library</span>
          </NavLink>

          <nav className="flex gap-1">
            {nav.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.end}
                       className="rounded-md px-3 py-1.5 text-sm font-medium transition"
                       style={({ isActive }) => ({
                         color: isActive ? 'var(--text-primary)' : 'var(--text-muted)',
                         background: isActive ? 'var(--gridline)' : 'transparent',
                       })}>
                {item.label}
              </NavLink>
            ))}
          </nav>

          <div className="ml-auto flex items-center gap-3">
            {health && (
              <span className="hidden text-xs sm:inline"
                    style={{ color: health.pool ? 'var(--text-muted)' : 'var(--status-warning)' }}>
                {health.pool
                  ? `${health.oracle_cards.toLocaleString()} cards`
                  : 'no card pool'}
              </span>
            )}
            {signedIn && (
              <span className="flex items-center gap-2">
                <span className="hidden text-xs sm:inline" style={{ color: 'var(--text-muted)' }}>
                  {auth?.user?.username}
                </span>
                <button onClick={() => void signOut()}
                        className="whitespace-nowrap rounded-md px-2 py-1.5 text-sm"
                        style={{ color: 'var(--text-secondary)',
                                 border: '1px solid var(--hairline)' }}>
                  Sign out
                </button>
              </span>
            )}
            <ThemeButton theme={theme} onToggle={toggleTheme} />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-8">
        <Suspense fallback={<Spinner label="Loading…" />}>
        <Routes>
          <Route path="/" element={<Library />} />
          {/* A deck's address is `owner/slug` now: slugs are unique per owner
              rather than globally, so the shorter form below cannot identify
              one on its own (ADR 22). It is kept as a resolver for links made
              before this changed — it looks the slug up and redirects. */}
          <Route path="/decks/:owner/:slug" element={<DeckDetail />} />
          <Route path="/decks/:slug" element={<DeckRedirect />} />
          <Route path="/new" element={<NewDeck />} />
          <Route path="/import" element={<Import />} />
          <Route path="/search" element={<CardSearch />} />
          <Route path="/simulate" element={<Simulator />} />
          <Route path="/learn" element={<Learn />} />
          {/* Declared unconditionally. A non-admin who types the URL gets the
              page's own 403 from the API rather than the catch-all's "nothing
              here", which is the more honest of the two answers. */}
          <Route path="/admin" element={<Admin />} />
          <Route path="*" element={
            <div className="card-surface rounded-xl px-6 py-10 text-center">
              <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
                Nothing here. Try the library.
              </p>
            </div>
          } />
        </Routes>
        </Suspense>
      </main>

      <footer className="mx-auto max-w-7xl px-6 pb-10 pt-4 text-xs"
              style={{ color: 'var(--text-muted)' }}>
        Card data and images from Scryfall. Unofficial Fan Content permitted under
        the Wizards of the Coast Fan Content Policy. Not approved or endorsed by
        Wizards. Portions of the materials used are property of Wizards of the
        Coast. ©Wizards of the Coast LLC.
      </footer>
    </div>
  )
}
