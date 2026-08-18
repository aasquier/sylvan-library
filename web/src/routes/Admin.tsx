import { useCallback, useEffect, useState } from 'react'
import {
  api, errorMessage, type Account, type AccountList, type AdminActivity,
  type AdminClaude, type AdminStorage, type AdminSystem,
} from '../lib/api'
import { Badge, ErrorNote, PageMasthead, Spinner } from '../components/ui'
import { EditsChart } from '../components/charts'

/**
 * Admin: the instance at a glance, and the account levers.
 *
 * Two halves. The dashboard on top is four read-only views of the box —
 * system, storage, Claude's ledger, activity — refreshed every thirty
 * seconds, each one a fact the process can report without an external API
 * or a new secret (`api/adminstats.py` says what was deliberately left
 * out, the dollar figure first among them). The accounts table below is
 * `mtglab users` as a page, unchanged by the rename: who exists, who can
 * sign in, and the four levers.
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

function ClaudePanel({ claude }: { claude: AdminClaude }) {
  // `span`, not `window`: shadowing the global in a file that also calls
  // `window.setInterval` is a bug that reads as a typo.
  const [span, setSpan] = useState<WindowKey>('month')
  const rows = claude.windows[span]
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
                <th className="pb-2 pr-4 font-medium">Mode</th>
                <th className="pb-2 pr-4 font-medium">Conversations</th>
                <th className="pb-2 pr-4 font-medium">Requests</th>
                <th className="pb-2 pr-4 font-medium">Tokens in</th>
                <th className="pb-2 pr-4 font-medium">Tokens out</th>
                <th className="pb-2 font-medium">Cache reads</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.mode} style={{ borderTop: '1px solid var(--hairline)' }}>
                  <td className="py-1.5 pr-4 font-medium">{row.mode}</td>
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
        {claude.caveat}{' '}
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
 * The dashboard: four views, fetched together, refreshed every thirty
 * seconds. `Promise.allSettled` rather than `all`, so one failing view
 * costs its own tiles and not the whole glance; a view that has never
 * answered renders em-dashes rather than zeros.
 */
function Dashboard() {
  const [system, setSystem] = useState<AdminSystem | null>(null)
  const [storage, setStorage] = useState<AdminStorage | null>(null)
  const [claude, setClaude] = useState<AdminClaude | null>(null)
  const [activity, setActivity] = useState<AdminActivity | null>(null)

  const refresh = useCallback(async () => {
    const [sys, sto, cla, act] = await Promise.allSettled([
      api.adminSystem(), api.adminStorage(),
      api.adminClaude(), api.adminActivity(),
    ])
    if (sys.status === 'fulfilled') setSystem(sys.value)
    if (sto.status === 'fulfilled') setStorage(sto.value)
    if (cla.status === 'fulfilled') setClaude(cla.value)
    if (act.status === 'fulfilled') setActivity(act.value)
  }, [])

  useEffect(() => {
    void refresh()
    const id = window.setInterval(() => { void refresh() }, 30_000)
    return () => window.clearInterval(id)
  }, [refresh])

  const jobsLine = activity && Object.keys(activity.jobs).length > 0
    ? Object.entries(activity.jobs)
        .map(([status, count]) => `${count} ${status}`).join(' · ')
    : 'none right now'

  return (
    <section className="space-y-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
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
                  hint={storage
                    ? `motion ${fmtBytes(storage.cache.cardmotion_bytes)} · symbols ${fmtBytes(storage.cache.symbols_bytes)}`
                    : undefined} />
        <StatTile label="Decks on the volume"
                  value={storage ? storage.decks.count : '—'}
                  hint={storage
                    ? `${fmtBytes(storage.decks.bytes)}${storage.decks.trashed > 0
                        ? ` · ${storage.decks.trashed} in the trash` : ''}`
                    : undefined} />
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

      {claude && <ClaudePanel claude={claude} />}
    </section>
  )
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

function AccountRow({ account, lastAdmin, isSelf, onChanged, onNote, onError }: {
  account: Account
  /** True when this is the only admin who can sign in. */
  lastAdmin: boolean
  /** True when this row is the account the caller is signed in as. */
  isSelf: boolean
  onChanged: () => Promise<void>
  onNote: (message: string) => void
  onError: (message: string) => void
}) {
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

export default function Admin() {
  const [data, setData] = useState<AccountList | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [note, setNote] = useState<string | null>(null)
  // Who the caller is, only so the delete button on their own row can be greyed
  // out with a reason. The server refuses it either way (409); this is the same
  // courtesy the last-admin buttons get. `null` with auth off, where there is
  // no account to be signed in as and every row is somebody else's.
  const [me, setMe] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setData(await api.accounts())
      setError(null)
    } catch (e) {
      setError(errorMessage(e))
    }
  }, [])

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
          The instance at a glance — the box it runs on, what the volume
          holds, where Claude’s tokens went, who has been in — and the
          account levers below.
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

      <Dashboard />

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
                  onChanged={load}
                  onNote={report}
                  onError={complain}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        There is no password field on this page, deliberately: nobody chooses a
        password for anybody else. An account that has lost one gets a reset
        link, and sets it themselves. Disabling revokes every session and is
        reversible; the last admin who can sign in can be neither demoted nor
        disabled, so hand an instance over by promoting the successor first.
      </p>
    </div>
  )
}
