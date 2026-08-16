import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import { NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import Claim from './routes/Claim'
import Library from './routes/Library'
import Login from './routes/Login'
import { Spinner } from './components/ui'
import { SettingsMenu } from './components/settings'
import { ForestAmbience, HeaderCanopy, LibraryMark } from './components/forest'
import { LibraryWhisper } from './components/whisper'
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
const Research = lazy(() => import('./routes/Research'))
const Simulator = lazy(() => import('./routes/Simulator'))

// Every entry carries a `hint`, rendered as the link's `title`: the labels
// are one or two words, and a word like "Laboratory" earns a sentence on
// hover saying what is actually behind it. Native tooltips rather than the
// glossary popover — these are wayfinding, not Magic vocabulary, so they do
// not belong in `glossary.py`'s table.
const NAV = [
  { to: '/', label: 'Library', end: true,
    hint: 'Your decks, and what other people here have shared' },
  { to: '/search', label: 'Card search', end: false,
    hint: 'Search the whole card pool by name or rules text' },
  // `/new` and `/import` are spliced in after Library — see AUTHORING_NAV.
  { to: '/simulate', label: 'Simulator', end: false,
    hint: 'Goldfish a deck: opening hands, castability, land counts' },
  // The two reference screens, and they sit next to each other because the
  // pair *is* the boundary: Learn is the checked-in prose this repo writes
  // once and edits by hand, the Laboratory is the question that prose cannot
  // answer, which costs a search and comes back with its pages (ADR 26).
  { to: '/research', label: 'Laboratory', end: false,
    hint: 'Ask about Magic — the meta, rulings, spoiled cards. '
        + 'Answers cite the pages they were read from' },
  // Last, and deliberately not first: it is reference rather than a task, and
  // somebody who needs it usually arrives from a word on another screen.
  { to: '/learn', label: 'Learn', end: false,
    hint: 'The vocabulary, the five colours, and the 32 combinations' },
]

// Appended for admins only. Hiding it is a courtesy — every route the page
// calls is refused to anybody else by the middleware, before routing (ADR 17),
// so this decides what is offered and never what is allowed.
const ADMIN_NAV = { to: '/admin', label: 'Accounts', end: false,
                    hint: 'Invites, accounts, and the instance’s levers' }

// The two doors that create a deck, and they are shown to everybody.
//
// They used to be gated on `is_admin`, because there was exactly one library
// and it was the maintainer's: a deck started by anybody else had nowhere to
// go but onto somebody else's shelf. That gate said it would not move
// somewhere better when decks got owners — it would **disappear** — and this
// is that. Every account has its own library now (ADR 22), so there is nobody
// left for whom these doors open onto nothing.
const AUTHORING_NAV = [
  { to: '/new', label: 'Start a deck', end: false,
    hint: 'Three doors in: a conversation about you, a tour of the '
        + 'colours, or straight to a commander' },
  { to: '/import', label: 'Import', end: false,
    hint: 'Paste a decklist and it becomes a draft deck, checked '
        + 'against the pool' },
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
          <LibraryMark size={26} />
          <span className="wordmark-title font-semibold tracking-tight">
            Sylvan Libraries
          </span>
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
  // `NAV.slice(0, 1)` and not `NAV[0]`: a slice of a known array is still
  // `NavItem[]`, whereas indexing one is `NavItem | undefined`, which spreads
  // that `undefined` into the element type and makes every field read in the
  // map below possibly-undefined. Same splice, no widening.
  const nav = [
    ...NAV.slice(0, 1), ...AUTHORING_NAV, ...NAV.slice(1),
    ...(auth?.is_admin ? [ADMIN_NAV] : []),
  ]
  // Only ever true on an instance that requires a login. With auth off there is
  // nobody to be signed out of, and offering it would be the regression this
  // whole flag exists to avoid.
  const signedIn = Boolean(auth?.auth_required && auth.authenticated)

  return (
    <div className="min-h-full">
      {/* The weather, behind everything: fireflies at night, leaves by day.
          `main` and the footer carry `relative z-10`, so painted content
          covers this layer and the forest shows through the gaps — which is
          how a firefly gets to pass *behind* a card. */}
      <ForestAmbience />
      <header className="sticky top-0 z-40 backdrop-blur"
              style={{
                background: 'color-mix(in srgb, var(--page) 88%, transparent)',
                borderBottom: '1px solid var(--hairline)',
              }}>
        {/* One row on a laptop; two on a phone. Below `lg` the seven nav
            entries cannot share a 375px line with the wordmark and the theme
            button — they used to wrap *inside their own labels* ("Start a
            deck" on three lines) and clip everything past Import off-screen —
            so the nav drops to its own full-width line and scrolls
            horizontally if it must. `flex-wrap` plus `order-last` is the
            whole mechanism; at `lg` the nav rejoins the row and none of the
            mobile classes apply. */}
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-6 px-4 py-3 sm:px-6">
          {/* `shrink-0` and no wrapping: the signed-in name and Sign out add a
              second cluster to this row, and without it a narrow window breaks
              the wordmark across two lines before it touches anything else. */}
          {/* The name the site answers to — sylvan-libraries.com — with the
              drawn mark instead of an emoji whose rendering belonged to the
              platform. The repo keeps its own name; that mismatch is stated
              in CLAUDE.md and is not a bug to fix. */}
          <NavLink to="/" className="wordmark flex shrink-0 items-center gap-2 whitespace-nowrap">
            <span aria-hidden className="wordmark-tree inline-flex">
              <LibraryMark size={26} />
            </span>
            <span className="wordmark-title text-[1.05rem] font-semibold tracking-tight">
              Sylvan Libraries
            </span>
          </NavLink>

          {/* The negative margin lets the scrolled row bleed to the viewport
              edge rather than clipping mid-padding, which is what makes the
              overflow read as "more this way" instead of as a defect. */}
          {/* The scrollbar is hidden because the cut-off last label already
              says "more this way", and a bar under the nav reads as clutter
              on the one row that is supposed to look effortless. */}
          <nav className="order-last -mx-4 mt-2 flex w-full gap-1 overflow-x-auto px-4
                          [scrollbar-width:none] [&::-webkit-scrollbar]:hidden
                          lg:order-none lg:mx-0 lg:mt-0 lg:w-auto lg:overflow-visible lg:px-0">
            {nav.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.end}
                       title={item.hint}
                       className={({ isActive }) =>
                         'nav-link whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition'
                         + (isActive ? ' is-active' : '')}
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
            {/* Every preference behind one gear (second punch list, item 9):
                theme, ambience, table sound, and — only where the instance
                has it — the Claude stance slider. The logged-out screens
                keep the bare ThemeButton; they have no ambience to switch. */}
            <SettingsMenu theme={theme} onToggleTheme={toggleTheme} />
          </div>
        </div>
        {/* The vine draped along the header's underside. Inside the sticky
            element so it travels with the chrome instead of scrolling away. */}
        <HeaderCanopy />
      </header>

      <main className="relative z-10 mx-auto max-w-7xl px-6 py-8">
        {/* Keyed on the path: navigating re-mounts this wrapper, so every
            page enters the same way — a short rise out of the undergrowth
            (`.page-enter`), gone entirely under reduced motion. */}
        <div key={location.pathname} className="page-enter">
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
          {/* No `:owner` and no `:slug`, and the absence is ADR 26 rather than
              an omission: this surface cannot reach a deck, so there is
              nothing for a path segment to name. */}
          <Route path="/research" element={<Research />} />
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
        </div>
      </main>

      {/* The corner sprout: one glossary whisper at a time, on every page,
          and it never opens itself. */}
      <LibraryWhisper />

      <footer className="relative z-10 mx-auto max-w-7xl px-6 pb-10 pt-4 text-xs"
              style={{ color: 'var(--text-muted)' }}>
        Card data and images from Scryfall. Unofficial Fan Content permitted under
        the Wizards of the Coast Fan Content Policy. Not approved or endorsed by
        Wizards. Portions of the materials used are property of Wizards of the
        Coast. ©Wizards of the Coast LLC.
      </footer>
    </div>
  )
}
