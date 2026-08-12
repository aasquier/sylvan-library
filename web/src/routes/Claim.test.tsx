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
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/api'
import Claim from './Claim'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    // `login` is mocked so the test can assert it is never reached. A claim
    // that quietly signed somebody in would otherwise look like a nicer flow.
    api: { claim: vi.fn(), login: vi.fn() },
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
