/**
 * The deck's description, and the way in.
 *
 * The empty state carries most of the weight here. Until 2026-08-29 nothing in
 * this app could write `strategy` — the field the library shelf, the deck page
 * and the printed primer all render — so a deck without one showed nothing at
 * all, and an imported deck is precisely the deck without one. The silence was
 * worst exactly where somebody had just arrived.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: { setDeckField: vi.fn() },
}))

const { api } = await import('../lib/api')
const { StrategyEditor } = await import('./deckedit')

const REF = { owner: 'aasquier', slug: 'gyome-food' }
const GYOME = 'Golgari Food aristocrats. Gyome turns every nontoken creature '
  + 'into a meal, and the deck drains the table sacrificing them.'

describe('StrategyEditor', () => {
  afterEach(cleanup)
  beforeEach(() => vi.mocked(api.setDeckField).mockReset())

  it('offers the pen when the deck says nothing yet', () => {
    render(<StrategyEditor deck={REF} value="" writable onDone={vi.fn()} />)
    expect(screen.getByText(/does not say what it is trying to do/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Describe this deck' })).toBeTruthy()
  })

  // An empty invitation to edit a deck you cannot edit is furniture.
  it('shows nothing at all on somebody else’s empty deck', () => {
    const { container } = render(
      <StrategyEditor deck={REF} value="" writable={false} onDone={vi.fn()} />)
    expect(container.firstChild).toBeNull()
  })

  // The prose is content, not an affordance: a reader sees it either way.
  it('shows the description to a reader, without the pen', () => {
    render(<StrategyEditor deck={REF} value={GYOME} writable={false} onDone={vi.fn()} />)
    expect(screen.getByText(/Golgari Food aristocrats/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Edit description/ })).toBeNull()
  })

  it('writes the deck’s strategy and hands the result back', async () => {
    const onDone = vi.fn()
    vi.mocked(api.setDeckField).mockResolvedValue({ slug: 'gyome-food' } as never)
    render(<StrategyEditor deck={REF} value="" writable onDone={onDone} />)

    fireEvent.click(screen.getByRole('button', { name: 'Describe this deck' }))
    fireEvent.change(screen.getByLabelText('What this deck is trying to do'),
      { target: { value: `  ${GYOME}  ` } })
    fireEvent.click(screen.getByRole('button', { name: 'Save description' }))

    await waitFor(() => expect(api.setDeckField).toHaveBeenCalled())
    // Trimmed, and named as the deck's own field rather than a note.
    expect(api.setDeckField).toHaveBeenCalledWith(REF, 'strategy', GYOME)
    await waitFor(() => expect(onDone).toHaveBeenCalled())
  })

  // Blanking is refused by the editor as well as the server: an empty
  // `strategy:` puts a blank paragraph at the top of a generated primer.
  it('will not save an empty description', () => {
    render(<StrategyEditor deck={REF} value={GYOME} writable onDone={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Edit description' }))
    fireEvent.change(screen.getByLabelText('What this deck is trying to do'),
      { target: { value: '   ' } })

    const save = screen.getByRole('button', { name: 'Save description' })
    expect(save.hasAttribute('disabled')).toBe(true)
    fireEvent.click(save)
    expect(api.setDeckField).not.toHaveBeenCalled()
  })

  it('leaves the description alone when the edit is cancelled', () => {
    render(<StrategyEditor deck={REF} value={GYOME} writable onDone={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Edit description' }))
    fireEvent.change(screen.getByLabelText('What this deck is trying to do'),
      { target: { value: 'something else entirely' } })
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(api.setDeckField).not.toHaveBeenCalled()
    expect(screen.getByText(/Golgari Food aristocrats/)).toBeTruthy()
  })
})
