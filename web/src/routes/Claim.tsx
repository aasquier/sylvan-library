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
 * One invite, one reset, one page — and it now *does* branch on which kind of
 * link arrived, which it did not before. An invite is somebody's first minute
 * here, so it offers a username rather than handing them the handle derived
 * from their email address; a reset is an account that already has a name
 * other people have seen, and renaming it from a forgotten-password link would
 * make "somebody reached my email" and "somebody took my identity here" the
 * same incident.
 *
 * The page cannot tell the two apart from a token, so it asks
 * `POST /api/auth/claim/preview` on mount. **That is a convenience, not a
 * control**: `tokens.redeem` gates the rename on the token's own purpose read
 * from the database, so a client that renders the field anyway and posts a
 * username gets a 422. If the preview fails for any reason, this falls back to
 * the password-only form, which is the behaviour it had before and is never
 * wrong — only less helpful.
 */

import { useEffect, useState } from 'react'
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

  // What kind of link this is. `null` until the preview answers, and it stays
  // `null` if the preview fails — see the module docstring: that degrades to
  // the password-only form rather than blocking somebody out of a valid link
  // because a lookup went wrong.
  const [kind, setKind] = useState<'invite' | 'reset' | null>(null)
  const [username, setUsername] = useState('')

  useEffect(() => {
    if (!token) return
    let live = true
    api.claimPreview({ token })
      .then(preview => {
        if (!live) return
        setKind(preview.purpose)
        // Prefilled with the derived handle rather than left empty: the
        // suggestion is usually fine, and somebody who does not care should be
        // able to press the button. Rule 4's empty-box argument is about a
        // rationale nobody may write for you — a username is not that.
        if (preview.purpose === 'invite') setUsername(preview.username)
      })
      .catch(() => { /* the form still works; the server is the authority */ })
    return () => { live = false }
  }, [token])

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
      // Sent only for an invite, and only when it differs from what was
      // suggested — there is no reason to ask the server to rename an account
      // to the name it already has.
      const chosen = kind === 'invite' ? username.trim() : ''
      const result = await api.claim({
        token, password, ...(chosen ? { username: chosen } : {}),
      })
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

  const inviting = kind === 'invite'

  return (
    <AuthCard
      title={inviting ? 'Set up your account' : 'Choose a password'}
      blurb="Nobody else ever sees it — not whoever invited you, and not the maintainer."
    >
      <form onSubmit={submit} className="space-y-3">
        {inviting && (
          <AuthField label="Username" autoComplete="username" autoFocus
                     value={username} onChange={setUsername}
                     hint="This is how you sign in and how you appear to others. Letters, digits, dot, dash or underscore." />
        )}
        <AuthField label="New password" type="password" autoComplete="new-password"
                   autoFocus={!inviting} value={password} onChange={setPassword}
                   hint={`At least ${MIN_PASSWORD_LENGTH} characters.`} />
        <AuthField label="Again" type="password" autoComplete="new-password"
                   value={confirm} onChange={setConfirm} />

        <div className="flex items-center gap-3 pt-1">
          <AuthSubmit label={inviting ? 'Create account' : 'Set password'}
                      busyLabel="Setting…" busy={busy}
                      disabled={!password || !confirm || wait > 0
                                || (inviting && !username.trim())} />
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
