import { useCallback, useEffect, useState } from 'react'
// The five readout payload types are gone from this import on purpose: each
// panel now gets its shape from `usePolled`'s inference over the client
// function it was handed, so a renamed field is a type error at the tile
// rather than at a hand-written annotation that had drifted. `AdminClaude`
// stays because `ClaudePanel` takes one as a prop.
import {
  api, errorMessage, type Account, type AccountList, type AdminClaude,
  type AdminStorage, type ModelTier,
} from '../lib/api'
import { Badge, ErrorNote, PageMasthead, Spinner } from '../components/ui'
import { EditsChart, TrafficChart } from '../components/lazycharts'

/**
 * Admin: the account levers, and the instance at a glance behind them.
 *
 * **Five tabs**, and the split is by the question a tab answers rather than by
 * which endpoint fed it — Accounts (the levers), Claude (where the tokens
 * went), Machine (is the box coping), Storage (will the volume fill up),
 * Activity (who has been in, and what changed). It was one long scroll until
 * the 2026-08-18 punch list: six panels and twelve tiles stacked in the order
 * they happened to be built, so the accounts table — the only part with
 * anything to *do* on it — sat below a screenful of numbers.
 *
 * The readouts are facts the process can report without an external API or a
 * new secret (which is why the dollar figure is an estimate from the server's
 * own price table, never a bill); the far-seeing glass on Machine is the one
 * exception and it is absent unless a token was set. The accounts table is
 * `mtglab users` as a page, unchanged by the rename: who exists, who can sign
 * in, and the four levers.
 *
 * Each readout tab **fetches only what it shows**, still on the thirty-second
 * clock, so switching away from a tab is also what stops it asking. The old
 * shape polled all six endpoints together because it displayed all six at
 * once; behind tabs that would be asking the box five questions nobody is
 * reading the answer to.
 *
 * The page is a page for the reason the deck editor is one — a hosted
 * app whose administration is SSH-only is one the maintainer can only run
 * from a laptop with the key on it. The CLI stays regardless: it is the
 * bootstrap path on a fresh volume, the break-glass path when mail is
 * misconfigured, and the only path that still works when the thing that is
 * broken is this page.
 *
 * Nothing here is the protection. Every route it calls is refused to a
 * non-admin by the middleware before routing (ADR 17); the nav hides this page
 * from people it would only 403 at, which is courtesy rather than security.
 *
 * Two rules the server owns and this page only *reflects*, because reflecting
 * them is not the same as enforcing them:
 *
 * - **The last admin who can sign in cannot be demoted or disabled.** The
 *   buttons are disabled when `admins === 1`, so the click is not offered; the
 *   server answers 409 to anybody who sends it anyway.
 * - **No password is ever chosen by one person for another** (ADR 16). There is
 *   no password field on this page and there will not be one. "Send reset link"
 *   is the whole of what an admin can do about a forgotten password.
 * - **Nobody deletes the account they are signed in as.** Greyed here with the
 *   reason, refused with a 409 there. `mtglab users delete` will do it, which is
 *   the right place for it: there is no session on the machine to sign out of.
 *
 * Delete is the only irreversible control here, and the only one that asks for
 * something to be typed. `DeleteAccount` says why the confirmation is the
 * username rather than a y/n.
 */

const STATE_TONE: Record<string, 'good' | 'warning' | 'critical' | 'neutral'> = {
  active: 'good',
  invited: 'neutral',
  'no password': 'warning',
  disabled: 'critical',
}

/** Teferi's Protection, Minttu Hynninen, Strixhaven Mystical Archive (2021)
 *  — the steward raising a ward over everything he owns while the swarm
 *  arrives. Part of the Mystical Archive cycle the page mastheads share;
 *  the argument is in `CardSearch.tsx`. Looked up on Scryfall, not
 *  recalled. */
const TEFERIS_PROTECTION_ART =
  'https://cards.scryfall.io/art_crop/front/2/8/28e21c8c-5ad1-4830-8621-f0fd6500ca79.jpg'

/** Bytes for a human, or an em-dash for "nothing there" — the storage
 *  endpoint sends null for a store that does not exist yet, and `0 B`
 *  would claim an empty file does. */
function fmtBytes(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  const units = ['B', 'kB', 'MB', 'GB', 'TB']
  let value = n
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  const label = units[unit] ?? 'TB'
  return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${label}`
}

/** A count for a human, or an em-dash for a series that returned nothing —
 *  the same null-is-not-zero rule the storage tiles follow. Rounded,
 *  because a Prometheus `increase()` over a window is a float and nobody
 *  wants 41.99999 requests. */
function fmtCount(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return Math.round(n).toLocaleString()
}

function StatTile({ label, value, hint }: {
  label: string
  value: React.ReactNode
  /** One clause of context under the number — units, window, caveat. */
  hint?: React.ReactNode
}) {
  return (
    <div className="card-surface rounded-xl px-4 py-3">
      <p className="text-[10px] font-semibold uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        {label}
      </p>
      <p className="mt-1 text-lg font-semibold tabular-nums">{value}</p>
      {hint && (
        <p className="mt-0.5 text-[11px] leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          {hint}
        </p>
      )}
    </div>
  )
}

/** The three ledger windows, offered as chips. `all` is the honest default
 *  on an instance young enough that a month is its whole history. */
const WINDOWS = [
  { key: 'week', label: '7 days' },
  { key: 'month', label: '30 days' },
  { key: 'all', label: 'All time' },
] as const
type WindowKey = (typeof WINDOWS)[number]['key']

/** The two axes of the ledger. Which surface spent it, and on which Claude —
 *  the second being the question per-account tiers made worth asking. */
const AXES = [
  { key: 'by_mode', label: 'By mode', column: 'Mode' },
  { key: 'by_model', label: 'By model', column: 'Answered by' },
] as const
type AxisKey = (typeof AXES)[number]['key']

/** A dollar figure a person can read.
 *
 *  Sub-cent totals are the ordinary case on an instance this size, and `$0.00`
 *  beside real conversations reads as "nothing happened" rather than as "not
 *  very much" — the wrong lesson to draw from a bill that is working. Small
 *  enough that a served, pre-rendered string would cost more than it saved,
 *  and a test pins the boundary. */
function money(usd: number): string {
  if (usd && Math.abs(usd) < 0.01) return `$${usd.toFixed(4)}`
  return `$${usd.toLocaleString(undefined, { minimumFractionDigits: 2,
                                             maximumFractionDigits: 2 })}`
}

function ClaudePanel({ claude }: { claude: AdminClaude }) {
  // `span`, not `window`: shadowing the global in a file that also calls
  // `window.setInterval` is a bug that reads as a typo.
  const [span, setSpan] = useState<WindowKey>('month')
  const [axis, setAxis] = useState<AxisKey>('by_mode')
  const pane = claude.windows[span]
  const rows = pane[axis]
  const spend = pane.estimated_usd
  const column = AXES.find((a) => a.key === axis)?.column ?? 'Mode'
  return (
    <div className="card-surface rounded-xl p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-sm font-semibold tracking-tight">
          Where Claude’s tokens went
        </h2>
        <div className="flex gap-1">
          {WINDOWS.map((w) => (
            <button key={w.key} type="button"
                    onClick={() => setSpan(w.key)}
                    className={`chip-toggle rounded-md px-2.5 py-1 text-xs font-medium${
                      span === w.key ? ' is-on' : ''}`}>
              {w.label}
            </button>
          ))}
        </div>
      </div>

      {/* The figure, and its two honesty clauses. Estimated from list rates a
          person read on a date, never from an invoice — and a conversation
          whose model carries no rate is counted here rather than quietly
          contributing nothing, which is what would make the total read low
          and reassuring. */}
      <div className="mt-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-2xl font-semibold tabular-nums">
          {money(spend.usd)}
        </span>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          estimated, at list rates read {spend.checked}
        </span>
      </div>
      {/* The ids that could not be priced are deliberately *not* named here.
          They are model ids, which this page does not render — and the
          maintainer who needs one has `mtglab claude usage`, which prints it
          to their own terminal. That is the same carve-out `mtglab users list`
          gets for printing an address: a terminal is not a screen. */}
      {!spend.complete && (
        <p className="mt-1 text-[11px]" style={{ color: 'var(--status-warning)' }}>
          {spend.unpriced.toLocaleString()} conversation
          {spend.unpriced === 1 ? '' : 's'} could not be priced — the figure
          above excludes them. Run <code>mtglab claude usage</code> to see
          which Claude answered them.
        </p>
      )}

      <div className="mt-3 flex gap-1">
        {AXES.map((a) => (
          <button key={a.key} type="button"
                  onClick={() => setAxis(a.key)}
                  className={`chip-toggle rounded-md px-2.5 py-1 text-xs font-medium${
                    axis === a.key ? ' is-on' : ''}`}>
            {a.label}
          </button>
        ))}
      </div>

      {rows.length === 0 ? (
        <p className="mt-3 text-xs" style={{ color: 'var(--text-muted)' }}>
          No conversations recorded in this window.
        </p>
      ) : (
        <div className="mt-3 overflow-x-auto">
          <table className="w-full min-w-[34rem] text-left text-sm">
            <thead>
              <tr className="text-[11px] uppercase tracking-wide"
                  style={{ color: 'var(--text-muted)' }}>
                <th className="pb-2 pr-4 font-medium">{column}</th>
                <th className="pb-2 pr-4 font-medium">Conversations</th>
                <th className="pb-2 pr-4 font-medium">Requests</th>
                <th className="pb-2 pr-4 font-medium">Tokens in</th>
                <th className="pb-2 pr-4 font-medium">Tokens out</th>
                <th className="pb-2 font-medium">Cache reads</th>
              </tr>
            </thead>
            <tbody>
              {/* Keyed on the id, rendered as the label: two ids that share a
                  name are still two rows, and React needs the distinction even
                  where the reader does not. */}
              {rows.map((row) => (
                <tr key={axis === 'by_mode' ? row.mode : row.model}
                    style={{ borderTop: '1px solid var(--hairline)' }}>
                  <td className="py-1.5 pr-4 font-medium">
                    {axis === 'by_mode' ? row.mode : row.model_label}
                  </td>
                  <td className="py-1.5 pr-4 tabular-nums">{row.conversations.toLocaleString()}</td>
                  <td className="py-1.5 pr-4 tabular-nums">{row.requests.toLocaleString()}</td>
                  <td className="py-1.5 pr-4 tabular-nums">{row.input_tokens.toLocaleString()}</td>
                  <td className="py-1.5 pr-4 tabular-nums">{row.output_tokens.toLocaleString()}</td>
                  <td className="py-1.5 tabular-nums">{row.cache_read_tokens.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="mt-3 text-[11px] leading-relaxed"
         style={{ color: 'var(--text-muted)' }}>
        {claude.caveat}{' '}{claude.prices.note}{' '}
        The{' '}
        <a href="https://console.anthropic.com/settings/usage"
           target="_blank" rel="noreferrer"
           className="underline decoration-dotted">
          Console’s usage page
        </a>{' '}
        has the billed truth, and{' '}
        <a href="https://console.anthropic.com/settings/billing"
           target="_blank" rel="noreferrer"
           className="underline decoration-dotted">
          billing
        </a>{' '}
        is where money is added — by a person, never by this page.
      </p>
    </div>
  )
}

/**
 * One admin readout: asked for now, and again every thirty seconds.
 *
 * The dashboard used to fetch all six in one `Promise.allSettled` because it
 * showed all six at once. Behind tabs it does not, and polling five endpoints
 * to render the sixth is asking the box questions nobody is reading the
 * answers to. Each panel now asks for its own, which means switching tabs is
 * also what starts and stops the asking.
 *
 * A rejection is swallowed on purpose and leaves the last good value standing:
 * a view that has never answered renders em-dashes rather than zeros, and one
 * that answered a minute ago should not blank because the box was briefly
 * busy. `alive` is the usual guard — a 30-second interval outlives a tab.
 */
function usePolled<T>(fetcher: () => Promise<T>): T | null {
  const [value, setValue] = useState<T | null>(null)
  useEffect(() => {
    let alive = true
    const ask = () => {
      void fetcher()
        .then((v) => { if (alive) setValue(v) })
        .catch(() => { /* keep what we had; this is a glance, not a console */ })
    }
    ask()
    const id = window.setInterval(ask, 30_000)
    return () => { alive = false; window.clearInterval(id) }
  }, [fetcher])
  return value
}

/** The box this instance runs on, and what the platform says about it. */
function MachinePanel() {
  const system = usePolled(api.adminSystem)
  const fly = usePolled(api.adminFly)

  return (
    <section className="space-y-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5">
        <StatTile label="Process memory"
                  value={system ? fmtBytes(system.process.bytes) : '—'}
                  hint={system
                    ? (system.process.kind === 'peak'
                        ? 'peak since start — this machine cannot report a level'
                        : 'resident, right now')
                    : undefined} />
        <StatTile label="Machine memory"
                  value={system?.memory.total_bytes != null
                    ? fmtBytes(system.memory.total_bytes) : '—'}
                  hint={system?.memory.available_bytes != null
                    ? `${fmtBytes(system.memory.available_bytes)} still available`
                    : undefined} />
        <StatTile label="Load"
                  value={system && system.load.length > 0
                    ? system.load.map((n) => n.toFixed(2)).join(' · ')
                    : '—'}
                  hint={system?.cpus != null
                    ? `1, 5 and 15 minutes, on ${system.cpus} CPU${system.cpus === 1 ? '' : 's'}`
                    : undefined} />
        <StatTile label="Volume"
                  value={system
                    ? `${fmtBytes(system.disk.used_bytes)} used`
                    : '—'}
                  hint={system
                    ? `${fmtBytes(system.disk.free_bytes)} free of ${fmtBytes(system.disk.total_bytes)}`
                    : undefined} />
        {/* Not a measure of how the box is coping, which is what the rest of
            this row is — it is the one number ADR 23 makes worth watching.
            A merge deploys itself, the migration runs on boot with nobody
            looking, and the ladder is forward-only, so rolling the code back
            leaves the schema where it got to. Shown as a pair because a lone
            version number cannot be wrong. */}
        <StatTile label="Schema"
                  value={system?.schema.applied != null
                    ? `v${system.schema.applied}` : '—'}
                  hint={system
                    ? (system.schema.applied === system.schema.expected
                        ? 'matches the code running here'
                        : <span style={{ color: 'var(--status-warning)' }}>
                            code here expects v{system.schema.expected}
                          </span>)
                    : undefined} />
      </div>

      {/* What the platform sees. Absent entirely on an instance with no
          token — every laptop — because a panel of em-dashes reads as
          breakage rather than as "not set up". Configured-but-unreachable
          renders as a clouded glass instead, which is a different fact and
          the one worth acting on. */}
      {fly?.configured && (
        <div className="card-surface rounded-xl p-5">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-sm font-semibold tracking-tight">
              The far-seeing glass
            </h2>
            <a href="https://fly-metrics.net" target="_blank" rel="noreferrer"
               className="text-[11px] underline decoration-dotted"
               style={{ color: 'var(--text-muted)' }}>
              Grafana, where the alerts live →
            </a>
          </div>
          {fly.ok ? (
            <>
              <div className="mt-3 grid grid-cols-2 gap-3 md:grid-cols-4">
                <StatTile label="Instance memory"
                          value={fmtBytes(fly.values.memory_bytes)}
                          hint={fly.values.memory_total_bytes != null
                            ? `of ${fmtBytes(fly.values.memory_total_bytes)}, as the platform counts it`
                            : 'as the platform counts it'} />
                <StatTile label="Edge answered"
                          value={fmtCount(fly.values.edge_2xx)}
                          hint="2xx at the edge, last 24 hours" />
                <StatTile label="Edge refused"
                          value={fmtCount(fly.values.edge_4xx)}
                          hint="4xx at the edge, last 24 hours" />
                <StatTile label="Edge failed"
                          value={fmtCount(fly.values.edge_5xx)}
                          hint="5xx at the edge, last 24 hours" />
              </div>
              <p className="mt-3 text-[11px] leading-relaxed"
                 style={{ color: 'var(--text-muted)' }}>
                The edge counts what reached the platform; the visitor ledger
                under Activity counts what this app answered. The gap between
                them is the requests the app never got to see.
              </p>
            </>
          ) : (
            <p className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
              The glass is clouded — {fly.error ?? 'the platform did not answer'}.
              The rest of this page is the box’s own account of itself and is
              unaffected.
            </p>
          )}
        </div>
      )}
    </section>
  )
}

/** The cache tile's second line: only the shelves that are actually there.
 *
 *  An empty shelf is dropped rather than printed as `— `, so a fresh instance
 *  says nothing instead of saying nothing four times. `other` is printed
 *  whenever it is non-zero and never suppressed: bytes nobody has named are
 *  the one number this line exists to surface. */
function cacheHint(cache: AdminStorage['cache']): string | undefined {
  const parts = [
    ['motion', cache.cardmotion_bytes],
    ['reader', cache.ocr_bytes],
    ['symbols', cache.symbols_bytes],
    ['other', cache.other_bytes],
  ] as const
  const shown = parts
    .filter(([, bytes]) => bytes !== null && bytes > 0)
    .map(([name, bytes]) => `${name} ${fmtBytes(bytes)}`)
  return shown.length > 0 ? shown.join(' · ') : undefined
}

/** What the volume is holding: the pool, the databases, the caches, the decks.
 *  Every figure here is bytes on disk, which is why they are one tab — the
 *  question they answer together is "will this fill up". */
function StoragePanel() {
  const storage = usePolled(api.adminStorage)

  return (
    <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
      <StatTile label="Card pool"
                value={storage ? fmtBytes(storage.pool_bytes) : '—'}
                hint={storage
                  ? `bulk files ${fmtBytes(storage.scryfall_bulk_bytes)}`
                  : undefined} />
      <StatTile label="Accounts db"
                value={storage ? fmtBytes(storage.app_db_bytes) : '—'}
                hint="users, sessions, caches and the ledgers" />
      <StatTile label="Caches"
                value={storage ? fmtBytes(storage.cache_bytes) : '—'}
                hint={storage ? cacheHint(storage.cache) : undefined} />
      <StatTile label="Decks on the volume"
                value={storage ? storage.decks.count : '—'}
                hint={storage
                  ? `${fmtBytes(storage.decks.bytes)}${storage.decks.trashed > 0
                      ? ` · ${storage.decks.trashed} in the trash` : ''}`
                  : undefined} />
    </section>
  )
}

/** Who has been in and what they did — sessions, jobs, the visitor ledger and
 *  the deck edits. The two charts live here because they are the same
 *  question at two grains: what the library was asked for, and what changed. */
function ActivityPanel() {
  const activity = usePolled(api.adminActivity)
  const traffic = usePolled(api.adminTraffic)

  const jobsLine = activity && Object.keys(activity.jobs).length > 0
    ? Object.entries(activity.jobs)
        .map(([status, count]) => `${count} ${status}`).join(' · ')
    : 'none right now'

  return (
    <section className="space-y-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <StatTile label="Accounts"
                  value={activity
                    ? Object.values(activity.accounts)
                        .reduce((a, b) => a + b, 0)
                    : '—'}
                  hint={activity
                    ? Object.entries(activity.accounts)
                        .map(([state, count]) => `${count} ${state}`)
                        .join(' · ') || 'none yet'
                    : undefined} />
        <StatTile label="Sessions"
                  value={activity ? activity.sessions.total : '—'}
                  hint={activity
                    ? `${activity.sessions.seen_day} seen today · ${activity.sessions.seen_week} this week`
                    : undefined} />
        <StatTile label="Memoised simulations"
                  value={activity ? activity.sim_cache_rows : '—'}
                  hint="Tier 1 results answered from cache" />
        <StatTile label="Jobs" value={activity ? jobsLine : '—'}
                  hint="the registry's census — counts, never labels" />
      </div>

      {traffic && traffic.days.length > 0 && (
        <div className="card-surface rounded-xl p-5">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-sm font-semibold tracking-tight">
              Visitors, last thirty days
            </h2>
            <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
              {traffic.note}
            </span>
          </div>
          <div className="mt-3">
            <TrafficChart days={traffic.days} />
          </div>
          {traffic.top_routes.length > 0 && (
            <div className="mt-3">
              <p className="text-[10px] font-semibold uppercase tracking-wide"
                 style={{ color: 'var(--text-muted)' }}>
                Most asked-for
              </p>
              <ul className="mt-1 grid gap-x-6 gap-y-0.5 sm:grid-cols-2">
                {traffic.top_routes.map((row) => (
                  <li key={row.route}
                      className="flex items-baseline justify-between gap-3 text-xs">
                    <code className="truncate" style={{ color: 'var(--text-secondary)' }}>
                      {row.route}
                    </code>
                    <span className="tabular-nums" style={{ color: 'var(--text-muted)' }}>
                      {row.count.toLocaleString()}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}

      {activity && activity.deck_edits_by_day.length > 0 && (
        <div className="card-surface rounded-xl p-5">
          <h2 className="text-sm font-semibold tracking-tight">
            Deck edits, last thirty days
          </h2>
          <div className="mt-3">
            <EditsChart rows={activity.deck_edits_by_day} />
          </div>
        </div>
      )}
    </section>
  )
}

/** The ledger's own tab. `ClaudePanel` takes the payload rather than fetching
 *  it, because it is the piece with the window chips and no business knowing
 *  where its rows came from. */
function ClaudeTab() {
  const claude = usePolled(api.adminClaude)
  if (!claude) return <Spinner label="Reading the ledger…" />
  return <ClaudePanel claude={claude} />
}

/** A button that reports what happened where it happened.
 *
 * Every lever on this page is a write that can be refused for a reason worth
 * reading — a 409 for the last admin, a 502 for mail that did not go out — and
 * a page-level error banner would leave somebody guessing which row it was
 * about.
 */
function RowAction({ label, busyLabel, disabled, title, onRun, danger }: {
  label: string
  busyLabel: string
  disabled?: boolean
  title?: string
  danger?: boolean
  onRun: () => Promise<string | void>
}) {
  const [busy, setBusy] = useState(false)

  return (
    <button
      type="button"
      disabled={disabled || busy}
      title={title}
      onClick={async () => {
        setBusy(true)
        try {
          await onRun()
        } finally {
          setBusy(false)
        }
      }}
      className={`btn btn-xs ${danger ? 'btn-danger' : 'btn-quiet'}`}
    >
      {busy ? busyLabel : label}
    </button>
  )
}

/** Delete, behind a typed username. The one irreversible control on the page.
 *
 * `decks delete` asks for a word back rather than a y/n, and this is the same
 * rule for the same reason: the answer has to be *produced*, and only somebody
 * looking at the right row can produce it. The server requires it too — it is
 * not a nicety this component could drop — so the confirmation is real rather
 * than decorative.
 *
 * It expands in place instead of opening a dialog. A modal that appears over
 * the table hides the row being deleted, which is the one thing worth looking
 * at while deciding.
 */
function DeleteAccount({ account, disabled, title, onRun }: {
  account: Account
  disabled?: boolean
  title?: string
  onRun: (confirm: string) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const matches = typed.trim().toLowerCase() === account.username.toLowerCase()

  if (!open) {
    return (
      <RowAction
        label="Delete"
        busyLabel="Deleting…"
        danger
        disabled={disabled}
        title={title}
        onRun={() => { setOpen(true); return Promise.resolve() }}
      />
    )
  }

  return (
    <span className="inline-flex items-center gap-1.5">
      <label className="sr-only" htmlFor={`confirm-${account.username}`}>
        Type {account.username} to delete this account
      </label>
      <input
        id={`confirm-${account.username}`}
        autoFocus
        value={typed}
        onChange={(event) => setTyped(event.target.value)}
        placeholder={`type ${account.username}`}
        className="rounded-md px-2 py-1 text-xs"
        style={{ border: '1px solid var(--hairline)',
                 background: 'var(--surface)', color: 'var(--text-primary)' }}
      />
      <button
        type="button"
        disabled={!matches || busy}
        className="btn btn-danger btn-xs"
        onClick={async () => {
          setBusy(true)
          try {
            await onRun(typed.trim())
          } finally {
            setBusy(false)
            setOpen(false)
            setTyped('')
          }
        }}
      >
        {busy ? 'Deleting…' : 'Delete for good'}
      </button>
      <button
        type="button"
        className="btn btn-ghost btn-xs"
        onClick={() => { setOpen(false); setTyped('') }}
      >
        Cancel
      </button>
    </span>
  )
}

function InviteForm({ onInvited, onError }: {
  onInvited: (message: string) => void
  onError: (message: string) => void
}) {
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [asAdmin, setAsAdmin] = useState(false)
  const [busy, setBusy] = useState(false)

  async function invite(event: React.FormEvent) {
    event.preventDefault()
    if (!email.trim() || busy) return
    setBusy(true)
    try {
      const account = await api.inviteAccount({
        email: email.trim(),
        ...(username.trim() ? { username: username.trim() } : {}),
        ...(asAdmin ? { is_admin: true } : {}),
      })
      onInvited(`${account.username} was invited; the link works once and `
        + 'expires in a week.')
      setEmail('')
      setUsername('')
      setAsAdmin(false)
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={invite} className="card-surface rounded-xl p-5">
      <h2 className="text-sm font-semibold tracking-tight">Invite somebody</h2>
      <p className="mt-1 text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Creates an account nobody has a password for and mails a single-use link.
        They choose the password; you never see it.
      </p>
      <div className="mt-4 flex flex-wrap items-end gap-3">
        <label className="flex min-w-56 flex-1 basis-64 flex-col gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide"
                style={{ color: 'var(--text-muted)' }}>
            Email
          </span>
          <input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="them@example.com"
            className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
            style={{ background: 'var(--surface-1)', color: 'var(--text-primary)',
                     border: '1px solid var(--hairline)' }}
          />
        </label>
        <label className="flex min-w-40 flex-1 basis-48 flex-col gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide"
                style={{ color: 'var(--text-muted)' }}>
            Username (optional)
          </span>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="from the address"
            className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
            style={{ background: 'var(--surface-1)', color: 'var(--text-primary)',
                     border: '1px solid var(--hairline)' }}
          />
        </label>
        <label className="flex h-9 items-center gap-2 text-xs"
               style={{ color: 'var(--text-secondary)' }}>
          <input type="checkbox" checked={asAdmin}
                 onChange={(e) => setAsAdmin(e.target.checked)} />
          as an admin
        </label>
        <button type="submit" disabled={busy || !email.trim()}
                className="btn btn-primary">
          {busy ? 'Inviting…' : 'Send invite'}
        </button>
      </div>
    </form>
  )
}

/**
 * Which Claude answers one account (punch list 2026-08-18 item 1).
 *
 * A `<select>` and not the `.btn` family, deliberately: this is a choice among
 * three named things rather than an action, which is what commandment 17's own
 * carve-out for `.chip-toggle` and `.strip-tab` is about — controls that are
 * places, not verbs. It still answers the hand, through `.tier-select`.
 *
 * The options come from the server (`AccountList.tiers`), so the page can only
 * offer what the server will accept. The blurb rides as the `title`, which is
 * where prose belongs on a control this narrow — the maintainer choosing gets
 * the argument on hover, and the table stays a table.
 */
function TierPicker({ account, tiers, busy, onPick }: {
  account: Account
  tiers: ModelTier[]
  busy: boolean
  onPick: (tier: string) => void
}) {
  const chosen = tiers.find((t) => t.key === account.model_tier)
  return (
    <select className="tier-select rounded-md px-2 py-1 text-xs"
            value={account.model_tier}
            disabled={busy || tiers.length === 0}
            title={chosen?.blurb ?? 'Which Claude answers this account.'}
            aria-label={`Which Claude answers ${account.username}`}
            onChange={(e) => onPick(e.target.value)}>
      {tiers.map((tier) => (
        <option key={tier.key} value={tier.key}>{tier.label}</option>
      ))}
    </select>
  )
}

function AccountRow({ account, lastAdmin, isSelf, tiers,
                     onChanged, onNote, onError }: {
  account: Account
  /** True when this is the only admin who can sign in. */
  lastAdmin: boolean
  /** True when this row is the account the caller is signed in as. */
  isSelf: boolean
  /** The tiers this instance knows, served rather than written here. */
  tiers: ModelTier[]
  onChanged: () => Promise<void>
  onNote: (message: string) => void
  onError: (message: string) => void
}) {
  const [tierBusy, setTierBusy] = useState(false)
  async function run(work: () => Promise<string>) {
    try {
      onNote(await work())
      await onChanged()
    } catch (e) {
      onError(errorMessage(e))
    }
  }


  const protection = lastAdmin
    ? 'The only admin who can sign in. Promote somebody else first.'
    : undefined

  return (
    <tr style={{ borderTop: '1px solid var(--hairline)' }}>
      <td className="py-2 pr-4">
        <div className="flex items-center gap-2">
          <span className="font-medium">{account.username}</span>
          {account.is_admin && <Badge tone="good">admin</Badge>}
        </div>
      </td>
      <td className="py-2 pr-4 text-xs" style={{ color: 'var(--text-secondary)' }}>
        {account.email ?? '—'}
      </td>
      <td className="py-2 pr-4">
        <Badge tone={STATE_TONE[account.state] ?? 'neutral'}>{account.state}</Badge>
      </td>
      <td className="py-2 pr-4 text-xs tabular-nums"
          style={{ color: 'var(--text-muted)' }}>
        {account.sessions}
      </td>
      <td className="py-2 pr-4">
        <TierPicker account={account} tiers={tiers} busy={tierBusy}
                    onPick={(tier) => {
                      setTierBusy(true)
                      void run(async () => {
                        const next = await api.updateAccount(
                          account.username, { model_tier: tier })
                        const label = tiers.find((t) => t.key === next.model_tier)
                        return `${next.username} is answered by `
                             + `${label?.label ?? next.model_tier}.`
                      }).finally(() => setTierBusy(false))
                    }} />
      </td>
      <td className="py-2">
        <div className="flex flex-wrap gap-1.5">
          <RowAction
            label={account.is_admin ? 'Demote' : 'Promote'}
            busyLabel="Saving…"
            disabled={account.is_admin && lastAdmin}
            title={account.is_admin && lastAdmin ? protection : undefined}
            onRun={() => run(async () => {
              await api.updateAccount(account.username,
                                      { is_admin: !account.is_admin })
              return `${account.username} is now `
                + `${account.is_admin ? 'not an admin' : 'an admin'}.`
            })}
          />
          <RowAction
            label={account.disabled ? 'Enable' : 'Disable'}
            busyLabel="Saving…"
            danger={!account.disabled}
            disabled={!account.disabled && account.is_admin && lastAdmin}
            title={!account.disabled && account.is_admin && lastAdmin
              ? protection : undefined}
            onRun={() => run(async () => {
              await api.updateAccount(account.username,
                                      { disabled: !account.disabled })
              return `${account.username} is now `
                + `${account.disabled ? 'enabled' : 'disabled'}.`
            })}
          />
          <RowAction
            label="Send reset link"
            busyLabel="Sending…"
            disabled={account.disabled || !account.email}
            title={account.disabled
              ? 'A disabled account gets no reset link.'
              : (!account.email ? 'This account has no address.' : undefined)}
            onRun={() => run(async () => {
              const answer = await api.sendReset(account.username)
              return answer.detail
            })}
          />
          <RowAction
            label="Sign out everywhere"
            busyLabel="Revoking…"
            disabled={account.sessions === 0}
            onRun={() => run(async () => {
              const answer = await api.revokeSessions(account.username)
              return `${answer.revoked} session(s) ended for ${answer.username}.`
            })}
          />
          <DeleteAccount
            account={account}
            disabled={isSelf || (account.is_admin && lastAdmin)}
            title={isSelf
              ? 'You cannot delete the account you are signed in as. '
                + 'Use `mtglab users delete` on the machine.'
              : (account.is_admin && lastAdmin ? protection : undefined)}
            onRun={(confirm) => run(async () => {
              const answer = await api.deleteAccount(account.username, confirm)
              const jobs = answer.jobs_dropped
                ? `, ${answer.jobs_dropped} job(s) dropped` : ''
              return `${answer.username} is gone `
                + `(${answer.revoked} session(s) ended${jobs}).`
            })}
          />
        </div>
      </td>
    </tr>
  )
}

/**
 * The five wards, and the order is the answer to "why did you come here".
 *
 * Accounts leads because it is the only tab with levers on it — every other
 * one is a readout, and a page that opened on a readout would make the
 * maintainer click once before doing the thing they came to do. The four
 * behind it are grouped by the question they answer together rather than by
 * which endpoint they came from: Machine is "is the box coping", Storage is
 * "will the volume fill up", Activity is "who has been in and what changed",
 * Claude is "where the tokens went".
 */
const TABS = [
  { id: 'accounts', label: 'Accounts' },
  { id: 'claude', label: 'Claude' },
  { id: 'machine', label: 'Machine' },
  { id: 'storage', label: 'Storage' },
  { id: 'activity', label: 'Activity' },
] as const
type AdminTab = (typeof TABS)[number]['id']

export default function Admin() {
  const [tab, setTab] = useState<AdminTab>('accounts')
  const [data, setData] = useState<AccountList | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [note, setNote] = useState<string | null>(null)
  // Who the caller is, only so the delete button on their own row can be greyed
  // out with a reason. The server refuses it either way (409); this is the same
  // courtesy the last-admin buttons get. `null` with auth off, where there is
  // no account to be signed in as and every row is somebody else's.
  const [me, setMe] = useState<string | null>(null)

  // A promise rather than `await`, and it still returns one: the row actions
  // below await `onChanged` before they let go of their spinner. Written this
  // way because the accounts land after a round trip and never in the effect
  // that asked for them, which is the thing the `await` form hid.
  const load = useCallback(
    () => api.accounts()
      .then((accounts) => { setData(accounts); setError(null) })
      .catch((e: unknown) => { setError(errorMessage(e)) }),
    [],
  )

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    // Failure is deliberately silent: not knowing who you are costs a greyed
    // button, and an error banner about it would be about nothing the person
    // reading this page came here to do.
    void api.me().then((state) => setMe(state.user?.username ?? null))
      .catch(() => setMe(null))
  }, [])

  function report(message: string) {
    setNote(message)
    setError(null)
  }

  function complain(message: string) {
    setError(message)
    setNote(null)
  }

  if (!data && !error) return <Spinner label="Loading the instance…" />

  return (
    <div className="space-y-6">
      <PageMasthead
        art={TEFERIS_PROTECTION_ART}
        alt="Teferi's Protection, painted by Minttu Hynninen: Teferi raises
             his staff and concentric wards spread over his homeland as a
             swarm descends."
        title="Admin"
        credit={<>
          <em>Teferi’s Protection</em> by Minttu Hynninen, Strixhaven
          Mystical Archive — the steward’s spell: everything you keep, kept
          safe until the trouble passes.
        </>}>
        <p>
          The account levers, and the instance at a glance behind them — the
          box it runs on, what the volume holds, who has been in, and where
          Claude’s tokens went.
        </p>
      </PageMasthead>

      {error && <ErrorNote>{error}</ErrorNote>}
      {note && (
        <div className="rounded-lg px-4 py-3 text-sm"
             style={{ background: 'color-mix(in srgb, var(--status-good) 10%, transparent)',
                      border: '1px solid color-mix(in srgb, var(--status-good) 35%, transparent)',
                      color: 'var(--text-primary)' }}>
          {note}
        </div>
      )}

      {/* The wards. Each readout tab fetches only what it shows, so switching
          away from one is also what stops it asking. */}
      <div className="flex flex-wrap gap-1 border-b"
           style={{ borderColor: 'var(--hairline)' }}>
        {TABS.map((t) => (
          <button key={t.id} type="button" onClick={() => setTab(t.id)}
                  aria-current={tab === t.id ? 'page' : undefined}
                  className={`strip-tab -mb-px border-b-2 px-3 py-2 text-sm font-medium${
                    tab === t.id ? ' is-active' : ''}`}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'claude' && <ClaudeTab />}
      {tab === 'machine' && <MachinePanel />}
      {tab === 'storage' && <StoragePanel />}
      {tab === 'activity' && <ActivityPanel />}

      {tab === 'accounts' && (
        <>
          <InviteForm onInvited={report} onError={complain} />

          {data && (
            <div className="card-surface overflow-x-auto rounded-xl p-5">
              <div className="flex items-baseline justify-between gap-4">
                <h2 className="text-sm font-semibold tracking-tight">
                  {data.users.length} account{data.users.length === 1 ? '' : 's'}
                </h2>
                <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                  {data.admins} admin{data.admins === 1 ? '' : 's'} can sign in
                </span>
              </div>
              <table className="mt-4 w-full min-w-[46rem] text-left text-sm">
                <thead>
                  <tr className="text-[11px] uppercase tracking-wide"
                      style={{ color: 'var(--text-muted)' }}>
                    <th className="pb-2 pr-4 font-medium">Username</th>
                    <th className="pb-2 pr-4 font-medium">Email</th>
                    <th className="pb-2 pr-4 font-medium">State</th>
                    <th className="pb-2 pr-4 font-medium">Sessions</th>
                    <th className="pb-2 pr-4 font-medium">Answered by</th>
                    <th className="pb-2 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {data.users.map((account) => (
                    <AccountRow
                      key={account.id}
                      account={account}
                      lastAdmin={data.admins === 1 && account.is_admin
                        && account.state === 'active'}
                      isSelf={me !== null && me === account.username}
                      tiers={data.tiers ?? []}
                      onChanged={load}
                      onNote={report}
                      onError={complain}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <p className="text-xs leading-relaxed"
             style={{ color: 'var(--text-muted)' }}>
            There is no password field on this page, deliberately: nobody
            chooses a password for anybody else. An account that has lost one
            gets a reset link, and sets it themselves. Disabling revokes every
            session and is reversible; the last admin who can sign in can be
            neither demoted nor disabled, so hand an instance over by promoting
            the successor first.
          </p>
        </>
      )}
    </div>
  )
}
