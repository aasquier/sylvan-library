/**
 * The page behind the emailed link: choose a password, once.
 *
 * **The token comes out of `location.hash` and nowhere else.** A fragment is
 * never sent to a server, so it stays out of the access log of every hop that
 * serves this page — the platform router, any proxy in front of it, and the
 * `Referer` of anything the page later loads. The query-string spelling would
 * write a live credential into all three. `auth/invites.py` builds the link
 * that way and says so; this is the client half of that contract, and the one
 * request that carries the token is the POST below, in a JSON body.
 *
 * **Claiming does not sign you in.** The API deliberately sets no cookie here —
 * a link that arrived by mail is not a session-minting endpoint — and returns
 * the username instead, so the login form can be filled in. That hand-off is
 * `onClaimed`, and it is the whole of what success does.
 *
 * One invite, one reset, one page: both purposes redeem through
 * `POST /api/auth/claim`, so there is nothing here that branches on which kind
 * of link arrived. The server knows; this screen does not need to.
 */

import { useState } from 'react'
import { api, errorMessage } from '../lib/api'
import { useRetryAfter } from '../lib/retry'
import { AuthCard, AuthField, AuthSubmit, PlainNote } from '../components/auth'
import { ErrorNote } from '../components/ui'

/** Mirrors `auth.passwords.MIN_PASSWORD_LENGTH`. The server owns the rule and
 *  answers 422 with its own sentence; this saves a round trip, never replaces
 *  the check. */
const MIN_PASSWORD_LENGTH = 12

/** The token from the URL fragment, or null.
 *
 * `location.search` is not consulted, and that is the module docstring's point
 * rather than an oversight of this function.
 */
function tokenFromHash(hash: string): string | null {
  const token = new URLSearchParams(hash.replace(/^#/, '')).get('token')
  return token && token.trim() ? token.trim() : null
}

export default function Claim({ onClaimed }: {
  /** Called with the username the server hands back, so the login form beside
   *  this one can be filled in. Claiming issues no session. */
  onClaimed: (username: string) => void
}) {
  // Read once, at mount. Kept in state rather than re-read, so the hash can be
  // cleared from the address bar the moment it is spent.
  const [token] = useState(() => tokenFromHash(window.location.hash))
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [claimed, setClaimed] = useState<string | null>(null)
  const [wait, holdFor] = useRetryAfter()

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!token || busy || wait > 0) return

    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`)
      return
    }
    if (password !== confirm) {
      setError('Those two do not match.')
      return
    }

    setBusy(true)
    setError(null)
    try {
      const result = await api.claim({ token, password })
      setPassword('')
      setConfirm('')
      // Spent, so there is no reason for it to sit in the address bar or ride
      // into a bookmark. Only after success: a 422 for a short password leaves
      // the token intact, and stripping it early would break the retry.
      window.history.replaceState(null, '', window.location.pathname)
      setClaimed(result.username)
    } catch (e) {
      holdFor(e)
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  if (claimed) {
    return (
      <AuthCard title="Password set">
        <div className="space-y-4">
          <PlainNote>
            Your username is <strong style={{ color: 'var(--text-primary)' }}>{claimed}</strong>.
            Setting a password does not sign you in, so the last step is the
            normal one.
          </PlainNote>
          <button type="button" onClick={() => onClaimed(claimed)}
                  className="h-9 rounded-lg px-4 text-sm font-medium"
                  style={{ background: 'var(--text-primary)', color: 'var(--page)' }}>
            Sign in
          </button>
        </div>
      </AuthCard>
    )
  }

  if (!token) {
    return (
      <AuthCard title="This page needs a link">
        <PlainNote>
          Open the link from your email — the part after the <code>#</code> is
          what this page reads, and it is missing here. If the link was copied
          without it, or has already been used, ask for a new one from the sign-in
          screen.
        </PlainNote>
      </AuthCard>
    )
  }

  return (
    <AuthCard
      title="Choose a password"
      blurb="Nobody else ever sees it — not whoever invited you, and not the maintainer."
    >
      <form onSubmit={submit} className="space-y-3">
        <AuthField label="New password" type="password" autoComplete="new-password"
                   autoFocus value={password} onChange={setPassword}
                   hint={`At least ${MIN_PASSWORD_LENGTH} characters.`} />
        <AuthField label="Again" type="password" autoComplete="new-password"
                   value={confirm} onChange={setConfirm} />

        <div className="flex items-center gap-3 pt-1">
          <AuthSubmit label="Set password" busyLabel="Setting…" busy={busy}
                      disabled={!password || !confirm || wait > 0} />
          {wait > 0 && (
            <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
              try again in {wait}s
            </span>
          )}
        </div>

        {/* "that link has expired" and "that link has already been used" are
            the server's words, and both are actionable in a way "invalid" is
            not — so they are shown rather than summarised. */}
        {error && <ErrorNote>{error}</ErrorNote>}
      </form>
    </AuthCard>
  )
}
