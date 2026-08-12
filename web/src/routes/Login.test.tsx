/** The sign-in screen: what it sends, and the three things it must not say.
 *
 * The API is finished and tested; what can go wrong here is the UI being more
 * helpful than the server was willing to be. So the assertions that matter are
 * negative ones:
 *
 * - a failed login is shown as the server's one sentence, not translated into
 *   "no such user" or "wrong password";
 * - the reset answer is rendered verbatim, and never dressed up as a
 *   confirmation that the address exists (ADR 16);
 * - a 429 is a wait with a number on it, not a button that keeps failing.
 */

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/api'
import Login from './Login'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: { login: vi.fn(), requestReset: vi.fn() },
  }
})

const { api } = await import('../lib/api')

/** The fixed 202 from `POST /api/auth/reset`, which says nothing either way. */
const RESET_ANSWER = 'if that address has an account, a link is on its way -- '
  + 'check your mail, and the spam folder'

function fillIn(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

function signIn() {
  return screen.getByRole('button', { name: 'Sign in' })
}

async function openResetPanel() {
  fireEvent.click(screen.getByText(/Forgotten your password/))
  await screen.findByLabelText('Email')
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('signing in', () => {
  it('sends what was typed and hands off to the shell', async () => {
    vi.mocked(api.login).mockResolvedValue({
      user: { id: 1, username: 'root', is_admin: true },
    })
    const onSignedIn = vi.fn()
    render(<Login onSignedIn={onSignedIn} />)

    fillIn('Username', 'root')
    fillIn('Password', 'a-long-enough-password')
    fireEvent.click(signIn())

    await waitFor(() => {
      expect(api.login).toHaveBeenCalledWith({
        username: 'root', password: 'a-long-enough-password',
      })
    })
    // The cookie is `HttpOnly`, so there is nothing for this screen to store.
    // Telling the shell to re-ask `/api/auth/me` is the whole of success.
    await waitFor(() => expect(onSignedIn).toHaveBeenCalled())
  })

  it('trims the username and leaves the password exactly as typed', async () => {
    vi.mocked(api.login).mockResolvedValue({
      user: { id: 1, username: 'root', is_admin: false },
    })
    render(<Login onSignedIn={vi.fn()} />)

    fillIn('Username', '  root  ')
    fillIn('Password', '  spaces are characters  ')
    fireEvent.click(signIn())

    await waitFor(() => {
      expect(api.login).toHaveBeenCalledWith({
        username: 'root', password: '  spaces are characters  ',
      })
    })
  })

  it('starts from the username a claim handed over', () => {
    render(<Login initialUsername="new.person" onSignedIn={vi.fn()} />)
    expect((screen.getByLabelText('Username') as HTMLInputElement).value)
      .toBe('new.person')
  })

  it('shows the refusal as the server wrote it, and does not sign anybody in', async () => {
    vi.mocked(api.login).mockRejectedValue(
      new ApiError('invalid username or password', 401))
    const onSignedIn = vi.fn()
    render(<Login onSignedIn={onSignedIn} />)

    fillIn('Username', 'root')
    fillIn('Password', 'wrong')
    fireEvent.click(signIn())

    expect(await screen.findByText('invalid username or password')).toBeTruthy()
    expect(onSignedIn).not.toHaveBeenCalled()
  })

  it('does not guess which half of the credentials was wrong', async () => {
    vi.mocked(api.login).mockRejectedValue(
      new ApiError('invalid username or password', 401))
    const { container } = render(<Login onSignedIn={vi.fn()} />)

    fillIn('Username', 'nobody')
    fillIn('Password', 'wrong')
    fireEvent.click(signIn())
    await screen.findByText('invalid username or password')

    // The server refuses an unknown user and a wrong password identically, and
    // a UI that narrowed it would undo that on the client.
    expect(container.textContent).not.toMatch(/no such (user|account)/i)
    expect(container.textContent).not.toMatch(/incorrect password|wrong password/i)
  })

  it('offers no way to sign up, because there is none', () => {
    const { container } = render(<Login onSignedIn={vi.fn()} />)

    expect(container.textContent).toMatch(/invited/i)
    expect(screen.queryByRole('button', { name: /sign up|create an account|register/i }))
      .toBeNull()
  })
})

describe('being rate limited', () => {
  it('counts the wait down and refuses to try again until it is over', async () => {
    vi.useFakeTimers()
    try {
      vi.mocked(api.login).mockRejectedValue(
        new ApiError('too many attempts -- wait and try again', 429, 3))
      render(<Login onSignedIn={vi.fn()} />)

      fillIn('Username', 'root')
      fillIn('Password', 'wrong')
      await act(async () => {
        fireEvent.click(signIn())
      })

      expect(screen.getByText('try again in 3s')).toBeTruthy()
      expect(signIn().hasAttribute('disabled')).toBe(true)

      // Clicking anyway sends nothing. The server would answer 429 again; not
      // spending the attempt is the point of showing the number.
      fireEvent.click(signIn())
      expect(api.login).toHaveBeenCalledTimes(1)

      await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
      expect(screen.getByText('try again in 2s')).toBeTruthy()

      for (let i = 0; i < 2; i++) {
        await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
      }
      expect(screen.queryByText(/try again in/)).toBeNull()
      expect(signIn().hasAttribute('disabled')).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows no countdown for a 429 that carried no Retry-After', async () => {
    vi.mocked(api.login).mockRejectedValue(
      new ApiError('too many attempts -- wait and try again', 429))
    render(<Login onSignedIn={vi.fn()} />)

    fillIn('Username', 'root')
    fillIn('Password', 'wrong')
    fireEvent.click(signIn())

    // The message still arrives; only the number is missing, because inventing
    // one would be worse than not having it.
    expect(await screen.findByText(/too many attempts/)).toBeTruthy()
    expect(screen.queryByText(/try again in/)).toBeNull()
  })
})

describe('the forgotten-password door', () => {
  it('stays shut until it is asked for', () => {
    render(<Login onSignedIn={vi.fn()} />)
    expect(screen.queryByLabelText('Email')).toBeNull()
  })

  it('warns before you type that the answer says nothing', async () => {
    render(<Login onSignedIn={vi.fn()} />)
    await openResetPanel()

    // The honest place for this is beside the field, not after the request:
    // said afterwards it reads as an apology for a non-answer.
    expect(screen.getByText(/same answer either way/i)).toBeTruthy()
  })

  it('shows the fixed answer exactly as the server wrote it', async () => {
    vi.mocked(api.requestReset).mockResolvedValue({ detail: RESET_ANSWER })
    render(<Login onSignedIn={vi.fn()} />)
    await openResetPanel()

    fillIn('Email', 'someone@example.com')
    fireEvent.click(screen.getByRole('button', { name: 'Send a reset link' }))

    await waitFor(() => {
      expect(api.requestReset).toHaveBeenCalledWith('someone@example.com')
    })
    const note = await screen.findByText(RESET_ANSWER)
    // Nothing appended inside it, which is where a cheerful "we have sent you
    // an email" would land. The endpoint answers identically for an address
    // with an account and one without; the screen has to as well.
    expect(note.textContent).toBe(RESET_ANSWER)
  })

  it('never repeats the address back, and never says one exists', async () => {
    vi.mocked(api.requestReset).mockResolvedValue({ detail: RESET_ANSWER })
    const { container } = render(<Login onSignedIn={vi.fn()} />)
    await openResetPanel()

    fillIn('Email', 'someone@example.com')
    fireEvent.click(screen.getByRole('button', { name: 'Send a reset link' }))
    await screen.findByText(RESET_ANSWER)

    // `getByText` does not read input values, so this is about rendered prose:
    // "a link is on its way to someone@example.com" is the leak this forbids.
    expect(screen.queryByText(/someone@example\.com/)).toBeNull()
    expect(container.textContent).not.toMatch(/we (have )?sent/i)
    expect(container.textContent).not.toMatch(/account (was |has been )?found/i)
  })

  it('reports a throttled reset without pretending it went out', async () => {
    vi.mocked(api.requestReset).mockRejectedValue(
      new ApiError('too many requests -- wait and try again', 429, 60))
    render(<Login onSignedIn={vi.fn()} />)
    await openResetPanel()

    fillIn('Email', 'someone@example.com')
    fireEvent.click(screen.getByRole('button', { name: 'Send a reset link' }))

    expect(await screen.findByText(/too many requests/)).toBeTruthy()
    expect(screen.queryByText(RESET_ANSWER)).toBeNull()
  })
})
