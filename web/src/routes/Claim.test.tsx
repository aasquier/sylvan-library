/** The claim page, and the two rules it exists to keep.
 *
 * **The token comes from `location.hash`.** A fragment is never sent to a
 * server, so it stays out of every access log between here and the app; the
 * query-string spelling would write a live credential into all of them. That is
 * asserted from both directions below — a token in the fragment is used, and a
 * token in the query string is not seen at all.
 *
 * **Claiming does not sign you in.** The API sets no cookie here on purpose, so
 * the page's success state is a username and a trip to the login form, not a
 * logged-in session.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/api'
import Claim from './Claim'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    // `login` is mocked so the test can assert it is never reached. A claim
    // that quietly signed somebody in would otherwise look like a nicer flow.
    api: { claim: vi.fn(), login: vi.fn(), claimPreview: vi.fn() },
  }
})

const { api } = await import('../lib/api')

const GOOD_PASSWORD = 'correct horse battery staple'

/** Put the browser at a URL, the way the emailed link would. */
function arriveAt(url: string) {
  window.history.replaceState(null, '', url)
}

function fillIn(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

function setBoth(password: string, confirm = password) {
  fillIn('New password', password)
  fillIn('Again', confirm)
}

function setPassword() {
  return screen.getByRole('button', { name: 'Set password' })
}

beforeEach(() => {
  // A reset unless a test says otherwise: the password-only form, which is what
  // this page did before invites could name themselves. Every assertion below
  // that is not about the username field is an assertion about that form.
  vi.mocked(api.claimPreview).mockResolvedValue({
    purpose: 'reset', username: 'ada',
  })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  arriveAt('/')
})

describe('where the token comes from', () => {
  it('reads the fragment, and sends it in a body rather than a URL', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    vi.mocked(api.claim).mockResolvedValue({ detail: 'password set', username: 'ada' })
    render(<Claim onClaimed={vi.fn()} />)

    setBoth(GOOD_PASSWORD)
    fireEvent.click(setPassword())

    await waitFor(() => {
      expect(api.claim).toHaveBeenCalledWith({
        token: 'a-256-bit-token', password: GOOD_PASSWORD,
      })
    })
  })

  it('does not read a token out of the query string', () => {
    // If a link is ever spelled this way, the credential is already in an
    // access log somewhere. Using it would be the client agreeing that was
    // fine; there is no form here to use it with.
    arriveAt('/auth/claim?token=leaked-into-every-log')
    render(<Claim onClaimed={vi.fn()} />)

    expect(screen.getByText(/This page needs a link/)).toBeTruthy()
    expect(screen.queryByLabelText('New password')).toBeNull()
    expect(api.claim).not.toHaveBeenCalled()
  })

  it('explains itself when the fragment carries no token at all', () => {
    arriveAt('/auth/claim')
    render(<Claim onClaimed={vi.fn()} />)

    expect(screen.getByText(/This page needs a link/)).toBeTruthy()
  })

  it('treats an empty token as no token', () => {
    arriveAt('/auth/claim#token=')
    render(<Claim onClaimed={vi.fn()} />)

    expect(screen.queryByLabelText('New password')).toBeNull()
  })
})

describe('when the fragment did not survive the trip', () => {
  /** The failure this whole path exists for, observed on the instance
   *  2026-08-13: the click arrives at the claim page with an empty hash, and
   *  the server cannot see that it happened. */
  function arriveStripped() {
    arriveAt('/auth/claim')
    render(<Claim onClaimed={vi.fn()} />)
    return screen.getByLabelText('Link from your email')
  }

  it('takes the whole address, fragment and all', async () => {
    const field = arriveStripped()
    fireEvent.change(field, {
      target: { value: 'https://sylvan-libraries.com/auth/claim#token=a-real-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // The password form, which means the token was accepted and the normal
    // flow resumed — including the preview that decides invite from reset.
    expect(await screen.findByLabelText('New password')).toBeTruthy()
    await waitFor(() => {
      expect(api.claimPreview).toHaveBeenCalledWith({ token: 'a-real-token' })
    })
  })

  it('takes a bare token, for somebody who already picked it out', async () => {
    const field = arriveStripped()
    fireEvent.change(field, { target: { value: 'UOrtOlsXjkkyez2PJhy5JO3nAJvDV' } })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    expect(await screen.findByLabelText('New password')).toBeTruthy()
  })

  it('trims what was pasted, because selecting a line picks up spaces', async () => {
    const field = arriveStripped()
    fireEvent.change(field, {
      target: { value: '  https://x.test/auth/claim#token=spaced-token \n' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    await waitFor(() => {
      expect(api.claimPreview).toHaveBeenCalledWith({ token: 'spaced-token' })
    })
  })

  it('names the actual problem when the pasted link is the stripped one', () => {
    const field = arriveStripped()
    // Precisely what somebody copies out of the address bar after clicking a
    // link a mail app cut short. Saying "invalid" here would send them back to
    // ask for another link, which fails the same way.
    fireEvent.change(field, {
      target: { value: 'https://sylvan-libraries.com/auth/claim' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    expect(screen.getByText(/no token in it/)).toBeTruthy()
    expect(screen.queryByLabelText('New password')).toBeNull()
    expect(api.claimPreview).not.toHaveBeenCalled()
  })

  it('never sends a pasted token anywhere but the body', async () => {
    const field = arriveStripped()
    fireEvent.change(field, {
      target: { value: 'https://x.test/auth/claim#token=stays-out-of-the-url' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    await screen.findByLabelText('New password')

    // The recovery must not undo what the fragment was for: nothing that lands
    // in an access log may carry the token, including the address bar of the
    // page it was pasted into.
    expect(window.location.href).not.toContain('stays-out-of-the-url')
    expect(window.location.search).toBe('')
  })
})

describe('choosing a password', () => {
  it('will not spend the link on one the server would refuse', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    render(<Claim onClaimed={vi.fn()} />)

    setBoth('short')
    fireEvent.click(setPassword())

    // Exact, because the field's own hint says "At least 12 characters" too —
    // and the hint being there is why this refusal never needs a round trip.
    expect(await screen.findByText('Password must be at least 12 characters.'))
      .toBeTruthy()
    // The link is single-use. Sending a password the server is certain to
    // reject would burn a round trip, not the token — but the round trip is
    // avoidable and the rule is already known here.
    expect(api.claim).not.toHaveBeenCalled()
  })

  it('will not send two that do not match', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    render(<Claim onClaimed={vi.fn()} />)

    setBoth(GOOD_PASSWORD, `${GOOD_PASSWORD}!`)
    fireEvent.click(setPassword())

    expect(await screen.findByText(/do not match/i)).toBeTruthy()
    expect(api.claim).not.toHaveBeenCalled()
  })

  it('shows the server sentence for a link that is spent or stale', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    vi.mocked(api.claim).mockRejectedValue(
      new ApiError('that link has already been used', 400))
    render(<Claim onClaimed={vi.fn()} />)

    setBoth(GOOD_PASSWORD)
    fireEvent.click(setPassword())

    // "already used" and "expired" are actionable in a way "invalid" is not,
    // which is why the server distinguishes them and this page repeats them.
    expect(await screen.findByText('that link has already been used')).toBeTruthy()
  })

  it('keeps the token in the fragment when the attempt was refused', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    vi.mocked(api.claim).mockRejectedValue(
      new ApiError('password must be at least 12 characters long', 422))
    render(<Claim onClaimed={vi.fn()} />)

    setBoth(GOOD_PASSWORD)
    fireEvent.click(setPassword())
    await screen.findByText('password must be at least 12 characters long')

    // A 422 leaves the token intact, so the retry has to still have it. This is
    // why the fragment is cleared on success and not on submit.
    expect(window.location.hash).toBe('#token=a-256-bit-token')
  })
})

describe('naming yourself, on an invite only', () => {
  function asInvite(username = 'ada.lovelace') {
    vi.mocked(api.claimPreview).mockResolvedValue({ purpose: 'invite', username })
  }

  function createAccount() {
    return screen.getByRole('button', { name: 'Create account' })
  }

  it('offers a username field, prefilled with the derived handle', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    asInvite()
    render(<Claim onClaimed={vi.fn()} />)

    const field = await screen.findByLabelText('Username')
    // Prefilled rather than empty: the suggestion is usually fine, and somebody
    // who does not care should be able to press the button.
    expect((field as HTMLInputElement).value).toBe('ada.lovelace')
  })

  it('sends the name the person actually chose', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    asInvite()
    vi.mocked(api.claim).mockResolvedValue({ detail: 'ok', username: 'countess' })
    render(<Claim onClaimed={vi.fn()} />)
    await screen.findByLabelText('Username')

    fillIn('Username', 'countess')
    setBoth(GOOD_PASSWORD)
    fireEvent.click(createAccount())

    await waitFor(() => {
      expect(api.claim).toHaveBeenCalledWith({
        token: 'a-256-bit-token', password: GOOD_PASSWORD, username: 'countess',
      })
    })
  })

  it('shows no username field on a reset', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    render(<Claim onClaimed={vi.fn()} />)   // the default preview is a reset

    await screen.findByLabelText('New password')
    // A forgotten password is not a reason to be handed a rename. The server
    // refuses it too; this is the form not offering what would be declined.
    expect(screen.queryByLabelText('Username')).toBeNull()
    expect(screen.getByRole('button', { name: 'Set password' })).toBeTruthy()
  })

  it('never sends a username with a reset', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    vi.mocked(api.claim).mockResolvedValue({ detail: 'ok', username: 'ada' })
    render(<Claim onClaimed={vi.fn()} />)
    await screen.findByLabelText('New password')

    setBoth(GOOD_PASSWORD)
    fireEvent.click(setPassword())

    await waitFor(() => {
      expect(api.claim).toHaveBeenCalledWith({
        token: 'a-256-bit-token', password: GOOD_PASSWORD,
      })
    })
  })

  it('keeps the link alive when the name is taken', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    asInvite()
    vi.mocked(api.claim).mockRejectedValue(
      new ApiError('that username is already taken', 409))
    render(<Claim onClaimed={vi.fn()} />)
    await screen.findByLabelText('Username')

    setBoth(GOOD_PASSWORD)
    fireEvent.click(createAccount())

    expect(await screen.findByText('that username is already taken')).toBeTruthy()
    // The whole point of the 409: a collision must leave a retryable invite
    // rather than a spent link and an account nobody can get into.
    expect(window.location.hash).toBe('#token=a-256-bit-token')
  })

  it('falls back to the password-only form if the preview fails', async () => {
    arriveAt('/auth/claim#token=a-256-bit-token')
    vi.mocked(api.claimPreview).mockRejectedValue(new ApiError('nope', 500))
    render(<Claim onClaimed={vi.fn()} />)

    // Degrading to what the page did before is never wrong, only less helpful.
    // The server still gates the rename, so nothing is lost but the field.
    expect(await screen.findByLabelText('New password')).toBeTruthy()
    expect(screen.queryByLabelText('Username')).toBeNull()
  })
})

describe('after it works', () => {
  async function claimSuccessfully(onClaimed = vi.fn()) {
    arriveAt('/auth/claim#token=a-256-bit-token')
    vi.mocked(api.claim).mockResolvedValue({
      detail: 'password set -- you can sign in now', username: 'new.person',
    })
    render(<Claim onClaimed={onClaimed} />)
    setBoth(GOOD_PASSWORD)
    fireEvent.click(setPassword())
    await screen.findByText(/Password set/)
    return onClaimed
  }

  it('reports the username, which is the only thing that comes back', async () => {
    await claimSuccessfully()
    expect(screen.getByText('new.person')).toBeTruthy()
  })

  it('does not sign anybody in, and says so', async () => {
    const onClaimed = await claimSuccessfully()

    expect(api.login).not.toHaveBeenCalled()
    expect(screen.getByText(/does not sign you in/i)).toBeTruthy()
    // The hand-off is a button, not something that happened while you read.
    expect(onClaimed).not.toHaveBeenCalled()
  })

  it('hands the username to the login form when asked', async () => {
    const onClaimed = await claimSuccessfully()

    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(onClaimed).toHaveBeenCalledWith('new.person')
  })

  it('clears the spent token out of the address bar', async () => {
    await claimSuccessfully()

    // Single-use and now used. There is no reason for it to survive into a
    // bookmark or a shoulder-surf.
    expect(window.location.hash).toBe('')
    expect(window.location.pathname).toBe('/auth/claim')
  })
})
