/**
 * Renaming a deck, and the one thing the control must never imply.
 *
 * Aaron asked for this directly on 2026-08-29: an imported deck wore whatever
 * name the import form was given, forever. The heading is the obvious place to
 * fix that, and the trap in the obvious place is conflating the two identities
 * a deck has — what it is called, and where it lives. The editor changes the
 * first and says so about the second, and that sentence is a test rather than
 * a comment because a reassurance nobody renders is worth nothing.
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: { setDeckField: vi.fn() },
}))

const { api } = await import('../lib/api')
const { DeckNameHeading, DECK_NAME_MAX } = await import('./deckname')

const REF = { owner: 'aasquier', slug: 'gyome-food' }
const NAME = 'Gyome, Master Chef — Food'

function open() {
  fireEvent.click(screen.getByRole('button', { name: `Rename ${NAME}` }))
  return screen.getByLabelText('Deck name') as HTMLInputElement
}

// `artifacts.test.tsx`'s shape, and it is load-bearing rather than stylistic:
// resetting the mock in a `beforeEach` instead makes the refusal test below
// report the caught rejection as an unhandled one. The catch runs — the alert
// renders and the assertion passes — and the run still fails. Measured both
// ways rather than reasoned about; clearing after is the form that holds.
describe('DeckNameHeading', () => {
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('shows the name to a reader, without the pen', () => {
    render(<DeckNameHeading name={NAME} writable={false} deckRef={REF}
                            onRenamed={vi.fn()} />)
    expect(screen.getByRole('heading', { name: NAME })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Rename/ })).toBeNull()
  })

  it('offers the pen on a deck you own', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    expect(screen.getByRole('button', { name: `Rename ${NAME}` })).toBeTruthy()
  })

  it('writes the deck’s own name, tidied', async () => {
    const onRenamed = vi.fn()
    vi.mocked(api.setDeckField).mockResolvedValue({ slug: 'gyome-food' } as never)
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={onRenamed} />)

    fireEvent.change(open(), { target: { value: '  Gyome,   Master Chef  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save name' }))

    await waitFor(() => expect(api.setDeckField).toHaveBeenCalled())
    expect(api.setDeckField).toHaveBeenCalledWith(REF, 'name', 'Gyome, Master Chef')
    // The shelf's copy and this page's have to agree afterwards.
    await waitFor(() => expect(onRenamed).toHaveBeenCalled())
  })

  // Enter is how a one-line field is submitted, and the reason there is no
  // <form> here: this heading sits inside a page, not inside one.
  it('saves on Enter', async () => {
    vi.mocked(api.setDeckField).mockResolvedValue({ slug: 'gyome-food' } as never)
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)

    const box = open()
    fireEvent.change(box, { target: { value: 'Something Else' } })
    fireEvent.keyDown(box, { key: 'Enter' })

    await waitFor(() => expect(api.setDeckField)
      .toHaveBeenCalledWith(REF, 'name', 'Something Else'))
  })

  it('gives the name back on Escape, and writes nothing', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    const box = open()
    fireEvent.change(box, { target: { value: 'A mistake' } })
    fireEvent.keyDown(box, { key: 'Escape' })

    expect(screen.getByRole('heading', { name: NAME })).toBeTruthy()
    expect(api.setDeckField).not.toHaveBeenCalled()
  })

  // The ruling, rendered. A person renaming a deck should not find out later
  // that the link they sent somebody says something else.
  it('says the deck does not move', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    open()
    expect(screen.getByText(/link you have already shared still opens it/)).toBeTruthy()
  })

  // Blanking is refused in the control as well as at the server. A deck with
  // no name does not render blank — it renders its address, which reads as a
  // name somebody chose.
  it('refuses to save nothing, and says why', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    fireEvent.change(open(), { target: { value: '   ' } })

    const save = screen.getByRole('button', { name: 'Save name' }) as HTMLButtonElement
    expect(save.disabled).toBe(true)
    expect(screen.getByText('A deck needs a name.')).toBeTruthy()
  })

  // A disabled button with no reason beside it reads as broken.
  it('says why saving the name it already has is off', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    open()
    expect((screen.getByRole('button', { name: 'Save name' }) as HTMLButtonElement)
      .disabled).toBe(true)
    expect(screen.getByText('That is the name it already has.')).toBeTruthy()
  })

  // Counted in characters a reader sees, not in bytes, and the box says so
  // itself rather than leaving the count as the only voice.
  it('counts a long name and refuses one past the cap', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    const box = open()
    fireEvent.change(box, { target: { value: '—'.repeat(DECK_NAME_MAX + 1) } })

    expect(screen.getByText(new RegExp(`${DECK_NAME_MAX + 1} of ${DECK_NAME_MAX}`)))
      .toBeTruthy()
    expect(box.getAttribute('aria-invalid')).toBe('true')
    expect((screen.getByRole('button', { name: 'Save name' }) as HTMLButtonElement)
      .disabled).toBe(true)
  })

  // The truncation this control deliberately does not do: a hard `maxLength`
  // would eat the tail of a pasted name with nothing saying why.
  it('keeps a name too long to save, rather than trimming it silently', () => {
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)
    const box = open()
    const long = 'a'.repeat(DECK_NAME_MAX + 20)
    fireEvent.change(box, { target: { value: long } })
    expect(box.value).toBe(long)
  })

  // The server has the last word, and its sentence is the one shown: it knows
  // things this control does not.
  it('shows the server’s own refusal', async () => {
    vi.mocked(api.setDeckField).mockRejectedValue(new Error('a deck needs a name'))
    render(<DeckNameHeading name={NAME} writable deckRef={REF} onRenamed={vi.fn()} />)

    fireEvent.change(open(), { target: { value: 'Anything At All' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save name' }))

    await waitFor(() => expect(screen.getByRole('alert').textContent)
      .toContain('a deck needs a name'))
    // Still open, with what was typed: a refused save must not throw the
    // typing away.
    expect((screen.getByLabelText('Deck name') as HTMLInputElement).value)
      .toBe('Anything At All')
  })
})
