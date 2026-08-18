/**
 * The sign-in screen, and the "I have forgotten it" door beside it.
 *
 * This exists only when the server says it does. `GET /api/auth/me` reports
 * `auth_required` and `authenticated` as two separate flags precisely so this
 * screen never renders on a laptop — the local app is one person at a desk, and
 * a login in front of `mtglab ui` would be a regression. `App` owns that
 * decision; this component is what it shows once it has made it.
 *
 * Three rules the server owns, which this screen may not soften:
 *
 * - **One refusal for every bad login.** The server never says which half of
 *   the credentials was wrong, so neither does this — the message it sends is
 *   rendered as written, and nothing here guesses at a friendlier one.
 * - **The reset endpoint's answer says nothing.** It is the same fixed 202 for
 *   an address with an account and an address without one. So the answer is
 *   shown verbatim, in a note that is neither green nor a confirmation: "check
 *   your inbox" phrased as success would leak, from the UI, exactly what ADR 16
 *   built the endpoint not to leak.
 * - **There is no sign-up.** Accounts are invited. Saying so is friendlier than
 *   letting a stranger hunt for a button that does not exist.
 */

import { useState } from 'react'
import { api, errorMessage } from '../lib/api'
import { useRetryAfter } from '../lib/retry'
import { AuthCard, AuthField, AuthSubmit, PlainNote } from '../components/auth'
import { ErrorNote } from '../components/ui'

function ResetPanel() {
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [answer, setAnswer] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [wait, holdFor] = useRetryAfter()

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!email.trim() || busy || wait > 0) return
    setBusy(true)
    setError(null)
    try {
      const { detail } = await api.requestReset(email.trim())
      setAnswer(detail)
    } catch (e) {
      holdFor(e)
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="mt-6 space-y-3 border-t pt-5"
          style={{ borderColor: 'var(--hairline)' }}>
      <h2 className="text-sm font-semibold tracking-tight">Forgotten password</h2>
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Give the address your invite went to. You will get the same answer either
        way — whether or not there is an account behind it — so this page can
        never be used to find out who has one.
      </p>

      <AuthField label="Email" type="email" autoComplete="email"
                 value={email} onChange={setEmail} />

      <div className="flex items-center gap-3">
        <AuthSubmit label="Send a reset link" busyLabel="Sending…"
                    busy={busy} disabled={!email.trim() || wait > 0} />
        {wait > 0 && (
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            try again in {wait}s
          </span>
        )}
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}
      {/* The server's sentence, unedited and unadorned. */}
      {answer && <PlainNote>{answer}</PlainNote>}
    </form>
  )
}

export default function Login({ initialUsername = '', onSignedIn }: {
  /** Filled in after a claim, which hands the username back for exactly this. */
  initialUsername?: string
  onSignedIn: () => void | Promise<void>
}) {
  const [username, setUsername] = useState(initialUsername)
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [forgotten, setForgotten] = useState(false)
  const [wait, holdFor] = useRetryAfter()

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (busy || wait > 0) return
    setBusy(true)
    setError(null)
    try {
      await api.login({ username: username.trim(), password })
      // Not kept a moment longer than the request that carried it.
      setPassword('')
      await onSignedIn()
    } catch (e) {
      holdFor(e)
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthCard
      title="Sign in"
      blurb={<>Accounts here are invited, never signed up for. If you do not have
        one, ask whoever runs this instance.</>}
    >
      <form onSubmit={submit} className="space-y-3">
        <AuthField label="Username" autoComplete="username" autoFocus={!initialUsername}
                   value={username} onChange={setUsername} />
        <AuthField label="Password" type="password" autoComplete="current-password"
                   autoFocus={Boolean(initialUsername)}
                   value={password} onChange={setPassword} />

        <div className="flex items-center gap-3 pt-1">
          <AuthSubmit label="Sign in" busyLabel="Signing in…" busy={busy}
                      disabled={!username.trim() || !password || wait > 0} />
          {wait > 0 && (
            <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
              try again in {wait}s
            </span>
          )}
        </div>

        {/* Whatever the server said, as it said it. It refuses an unknown user
            and a wrong password with one sentence on purpose. */}
        {error && <ErrorNote>{error}</ErrorNote>}
      </form>

      {forgotten
        ? <ResetPanel />
        : (
          <button type="button" onClick={() => setForgotten(true)}
                  className="btn btn-ghost btn-xs mt-5">
            Forgotten your password?
          </button>
          )}
    </AuthCard>
  )
}
