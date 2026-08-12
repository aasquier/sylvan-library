import { useEffect, useState } from 'react'
import { NavLink, Route, Routes } from 'react-router-dom'
import Admin from './routes/Admin'
import CardSearch from './routes/CardSearch'
import DeckDetail from './routes/DeckDetail'
import Import from './routes/Import'
import Library from './routes/Library'
import NewDeck from './routes/NewDeck'
import Simulator from './routes/Simulator'
import { api, type AuthState, type Health } from './lib/api'

const NAV = [
  { to: '/', label: 'Library', end: true },
  { to: '/new', label: 'Start a deck', end: false },
  { to: '/import', label: 'Import', end: false },
  { to: '/search', label: 'Card search', end: false },
  { to: '/simulate', label: 'Simulator', end: false },
]

// Appended for admins only. Hiding it is a courtesy — every route the page
// calls is refused to anybody else by the middleware, before routing (ADR 17),
// so this decides what is offered and never what is allowed.
const ADMIN_NAV = { to: '/admin', label: 'Accounts', end: false }

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

export default function App() {
  const [theme, toggleTheme] = useTheme()
  const [health, setHealth] = useState<Health | null>(null)
  const [auth, setAuth] = useState<AuthState | null>(null)

  useEffect(() => {
    api.health().then(setHealth).catch(() => setHealth(null))
    // `is_admin` and not `user?.is_admin`: with auth off the caller is the
    // local single user, who is an admin and is authenticated as nobody, so
    // the nested flag is null exactly when the page is most usable.
    api.me().then(setAuth).catch(() => setAuth(null))
  }, [])

  const nav = auth?.is_admin ? [...NAV, ADMIN_NAV] : NAV

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-40 backdrop-blur"
              style={{
                background: 'color-mix(in srgb, var(--page) 88%, transparent)',
                borderBottom: '1px solid var(--hairline)',
              }}>
        <div className="mx-auto flex max-w-7xl items-center gap-6 px-6 py-3">
          <NavLink to="/" className="flex items-center gap-2">
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
                    style={{ color: health.corpus ? 'var(--text-muted)' : 'var(--status-warning)' }}>
                {health.corpus
                  ? `${health.oracle_cards.toLocaleString()} cards`
                  : 'no corpus'}
              </span>
            )}
            <button onClick={toggleTheme}
                    aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
                    className="rounded-md px-2 py-1.5 text-sm"
                    style={{ color: 'var(--text-secondary)', border: '1px solid var(--hairline)' }}>
              {theme === 'dark' ? '☀' : '☾'}
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-8">
        <Routes>
          <Route path="/" element={<Library />} />
          <Route path="/decks/:slug" element={<DeckDetail />} />
          <Route path="/new" element={<NewDeck />} />
          <Route path="/import" element={<Import />} />
          <Route path="/search" element={<CardSearch />} />
          <Route path="/simulate" element={<Simulator />} />
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
