/**
 * The deck's description, and the way in.
 *
 * The empty state carries most of the weight here. Until 2026-08-29 nothing in
 * this app could write `strategy` — the field the library shelf, the deck page
 * and the printed primer all render — so a deck without one showed nothing at
 * all, and an imported deck is precisely the deck without one. The silence was
 * worst exactly where somebody had just arrived.
 *
 * The second half is the assist, and its tests are about one thing: **a draft
 * never overwrites words somebody wrote without a second, undoable choice.**
 *
 * That sentence used to read "without somebody choosing it twice", and the
 * change on 2026-08-30 is the point of most of what is below. Two presses were
 * covering two different risks and only one of them was real. Over a paragraph
 * a person wrote, the second press is the guard on *their words*: the box is
 * where the undo lives, so the draft goes into the box and the box gets saved.
 * Over an empty deck it guarded nothing — it made somebody press "Use this
 * draft" and then hunt upward for "Save description" to accept a paragraph
 * they had just been shown and had no earlier version of.
 *
 * So there are two shapes here and the suite drives both. **Lead** is the empty
 * deck: the panel is the whole screen, it reads as two beats — Claude working,
 * then approve / edit / ask again / leave — and approving saves in one press.
 * **Side** is the pen's path over an existing description, and it is unchanged;
 * the suite still drives that hard case for most of what follows.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ClaudeStatus, DeckDescriptionDraft, StanceView } from '../lib/api'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: { setDeckField: vi.fn(), claudeStatus: vi.fn(), describeDeck: vi.fn() },
}))

const { api } = await import('../lib/api')
const { StrategyEditor } = await import('./deckedit')

const REF = { owner: 'aasquier', slug: 'gyome-food' }
const GYOME = 'Golgari Food aristocrats. Gyome turns every nontoken creature '
  + 'into a meal, and the deck drains the table sacrificing them.'
const DRAFTED = 'Cook Food, sacrifice it, drain the table. Slow to start, and '
  + 'it folds to a board wipe on turn six.'

function view(level: string): StanceView {
  return {
    preset: 'consultant',
    allows_calls: level !== 'off',
    may_write: false,
    axes: [{ axis: 'initiative', question: '', level, means: '', levels: [] }],
  }
}

/** An instance with a key, at a stance that answers. `level` is the initiative
 *  axis, which is the one that decides whether a call happens at all. */
function status(level = 'on-request'): ClaudeStatus {
  return {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: view(level), ceiling: view('interjects'), default: view('on-request'),
    presets: [], never: '', modes: [],
  }
}

function drafted(over: Partial<DeckDescriptionDraft> = {}): DeckDescriptionDraft {
  return {
    answered_by: 'claude', mode: 'deck-description', slug: 'gyome-food',
    asked: true, reason: '', stance: view('on-request'),
    strategy: DRAFTED, themes: ['food', 'aristocrats'],
    fact: 'Gyome makes Food on every death; fourteen sacrifice outlets.',
    never: 'This is a draft in your own box. Nothing is saved until you save it.',
    ...over,
  }
}

/** Open the editor over an existing description — the hard case, and the one
 *  whose two presses are a real guard. This is "side" mode. */
async function openOver(text: string) {
  render(<StrategyEditor deck={REF} value={text} writable onDone={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: 'Edit description' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Ask Claude for a draft' })).toBeTruthy())
}

/** The newcomer's path: an empty deck, and the button that opens the editor
 *  *and* asks in one press. This is "lead" mode — the panel is the screen.
 *  `onDone` is returned so a caller can assert the save reached the page. */
async function askFromEmpty(onDone = vi.fn()) {
  render(<StrategyEditor deck={REF} value="" writable onDone={onDone} />)
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Ask Claude for a draft' })).toBeTruthy())
  fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))
  return onDone
}

describe('StrategyEditor', () => {
  afterEach(cleanup)
  beforeEach(() => {
    vi.mocked(api.setDeckField).mockReset()
    vi.mocked(api.describeDeck).mockReset()
    vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(status())
  })

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

describe('StrategyEditor: the draft', () => {
  afterEach(cleanup)
  beforeEach(() => {
    vi.mocked(api.setDeckField).mockReset()
    vi.mocked(api.describeDeck).mockReset()
    vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(status())
  })

  // The blank deck is where a newcomer is most likely to want help and least
  // likely to go looking for it, so the way in sits beside the pen rather than
  // behind it.
  it('offers the draft beside the pen on a deck that says nothing yet', async () => {
    render(<StrategyEditor deck={REF} value="" writable onDone={vi.fn()} />)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Ask Claude for a draft' })).toBeTruthy())
    expect(screen.getByRole('button', { name: 'Describe this deck' })).toBeTruthy()
  })

  // A control that appears and then refuses is worse than one that is honestly
  // absent (ADR 15) — and a deck somebody else owns is not a deck this can
  // help with, so nothing is even asked about it.
  it('asks nothing at all about a deck this person cannot write', () => {
    render(<StrategyEditor deck={REF} value={GYOME} writable={false} onDone={vi.fn()} />)
    expect(api.claudeStatus).not.toHaveBeenCalled()
  })

  // Pressing the way in opens the editor AND asks: the decision was made by
  // the click that opened it, and a second button for it is a hole to stare
  // into. Once, on the deck — a re-render must not buy a second call.
  it('asks once when the editor is opened by the ask', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    render(<StrategyEditor deck={REF} value="" writable onDone={vi.fn()} />)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Ask Claude for a draft' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() => expect(screen.getByText(/Cook Food, sacrifice it/)).toBeTruthy())
    expect(api.describeDeck).toHaveBeenCalledTimes(1)
  })

  // **The load-bearing one.** The draft renders beside the box and lands in
  // nothing until somebody presses a button that said what it would do.
  it('shows a draft without putting a word of it in the box', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() => expect(screen.getByText(/Cook Food, sacrifice it/)).toBeTruthy())
    const box = screen.getByLabelText('What this deck is trying to do') as HTMLTextAreaElement
    expect(box.value).toBe(GYOME)
    // And nothing has been written anywhere: this route proposes, the person
    // saves, and the save is a separate button.
    expect(api.setDeckField).not.toHaveBeenCalled()
  })

  // The label names the cost before it is paid: over a paragraph somebody
  // wrote, the button says it will replace it.
  it('says it will replace what you wrote, when there is something to replace', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Replace what you wrote' })).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Use this draft' })).toBeNull()
  })

  // Over an empty deck the draft is the screen, and the row under it is the
  // whole decision: approve, edit, ask again, leave.
  it('offers approve, edit, another and a way out over an empty deck', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await askFromEmpty()

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save this description' })).toBeTruthy())
    for (const name of ['Edit it first', 'Ask for another', 'Cancel']) {
      expect(screen.getByRole('button', { name })).toBeTruthy()
    }
    // The box's own labels belong to the other shape.
    expect(screen.queryByRole('button', { name: 'Replace what you wrote' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Use this draft' })).toBeNull()
  })

  // A replacement is one click from being undone, and neither version is ever
  // the one you cannot get back: the draft stays on screen after it is used.
  it('gives the words back after a replacement', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Replace what you wrote' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Replace what you wrote' }))
    const box = screen.getByLabelText('What this deck is trying to do') as HTMLTextAreaElement
    expect(box.value).toBe(DRAFTED)
    // Still nothing saved: the swap happened in this box and nowhere else.
    expect(api.setDeckField).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Put your own words back' }))
    expect((screen.getByLabelText('What this deck is trying to do') as HTMLTextAreaElement)
      .value).toBe(GYOME)
    // Offered only while there is something to put back.
    expect(screen.queryByRole('button', { name: 'Put your own words back' })).toBeNull()
  })

  // Once the draft IS the box, "replace what you wrote" is a sentence about
  // nothing. Said rather than disabled: a greyed control reads as "this is
  // broken" where the truth is "this is already done" — and it comes back the
  // moment somebody types over it.
  it('stops offering the draft once the box already holds it', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Replace what you wrote' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Replace what you wrote' }))

    expect(screen.getByText(/in the box above, and yours to edit/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Replace what you wrote' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Use this draft' })).toBeNull()

    fireEvent.change(screen.getByLabelText('What this deck is trying to do'),
      { target: { value: `${DRAFTED} And it never blocks.` } })
    expect(screen.getByRole('button', { name: 'Replace what you wrote' })).toBeTruthy()
  })

  // Taking a draft into the box does not offer an undo of nothing.
  it('offers no undo when the draft displaced nothing', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit it first' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Edit it first' }))
    expect(screen.queryByRole('button', { name: 'Put your own words back' })).toBeNull()
  })

  // "Edit it first" is the other half of beat two, and the box it opens holds
  // the draft — the point of putting it in a box at all is changing it there.
  it('writes what the box holds after the draft is edited', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    vi.mocked(api.setDeckField).mockResolvedValue({ slug: 'gyome-food' } as never)
    await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit it first' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Edit it first' }))

    const box = screen.getByLabelText('What this deck is trying to do') as HTMLTextAreaElement
    expect(box.value).toBe(DRAFTED)
    const edited = `${DRAFTED} And it never blocks.`
    fireEvent.change(box, { target: { value: edited } })
    fireEvent.click(screen.getByRole('button', { name: 'Save description' }))

    await waitFor(() =>
      expect(api.setDeckField).toHaveBeenCalledWith(REF, 'strategy', edited))
  })

  // Commandment 2's "shut out" includes shut out by a screen reader. The
  // answer lands ten to twenty seconds after the button, which a sighted
  // person watches happen and a reader would otherwise meet as silence, then
  // silence. An announcement is not a thing a screenshot can show, so nothing
  // else in this suite would notice a refactor dropping it.
  it('announces the draft in a region a reader is watching', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() =>
      expect(screen.getByRole('status').textContent).toContain('Cook Food, sacrifice it'))
  })

  // ADR 14 boundary 3: the gate's answer is reproducible and this is not, so
  // they never share a surface without a label. It names the system, never a
  // model id (commandment 10).
  it('says who answered, what it read, and that nothing is saved yet', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() => expect(screen.getByText(/A draft by Claude, not the gate/)).toBeTruthy())
    expect(screen.getByText(/fourteen sacrifice outlets/)).toBeTruthy()
    expect(screen.getByText(/Nothing is saved until you save it/)).toBeTruthy()
    // The themes are shown, and the panel says they are only shown.
    expect(screen.getByText(/food, aristocrats/)).toBeTruthy()
    expect(screen.getByText(/the description is the paragraph above/)).toBeTruthy()
  })

  // `asked: false` is a real answer, not a failure: the dial is down, no call
  // was made, and it costs nothing. Rendering it as an error would tell
  // somebody their instance is broken when their preference is merely off.
  it('renders a stance that made no call as a reason, not an error', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted({
      asked: false, strategy: '', themes: [],
      reason: 'The stance is off, so no call was made.',
    }))
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() => expect(screen.getByText(/no call was made/)).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Replace what you wrote' })).toBeNull()
  })

  // A dial set to silence is a position somebody chose. Say where the dial is
  // rather than showing a button that would refuse.
  it('points at the dial rather than offering a control that would refuse', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(status('off'))
    render(<StrategyEditor deck={REF} value={GYOME} writable onDone={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Edit description' }))

    await waitFor(() => expect(screen.getByText(/set to stay silent/)).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Ask Claude for a draft' })).toBeNull()
    expect(api.describeDeck).not.toHaveBeenCalled()
  })

  // And the empty state's way in is absent for the same reason.
  it('does not offer the way in when the dial is down', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(status('off'))
    render(<StrategyEditor deck={REF} value="" writable onDone={vi.fn()} />)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Describe this deck' })).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Ask Claude for a draft' })).toBeNull()
  })

  // **A fallback that reads as a fact is the mistake this repo makes most
  // often.** A panel that vanished when the status call failed would be
  // claiming "this instance has no Claude", which nobody checked.
  it('says the question failed rather than silently having no assist', async () => {
    vi.mocked(api.claudeStatus).mockRejectedValue(new Error('the network went away'))
    render(<StrategyEditor deck={REF} value={GYOME} writable onDone={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: 'Edit description' }))

    await waitFor(() => expect(screen.getByText(/could not be reached just now/)).toBeTruthy())
    // The box still works, which is what the sentence promises.
    expect(screen.getByLabelText('What this deck is trying to do')).toBeTruthy()
  })

  // A refused call is the caller's to see. It must not read as "your deck has
  // nothing worth saying about it".
  it('surfaces a refused call instead of an empty panel', async () => {
    vi.mocked(api.describeDeck).mockRejectedValue(new Error('claude is unavailable'))
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))

    await waitFor(() => expect(screen.getByText(/claude is unavailable/)).toBeTruthy())
  })

  // Commandment 17: every control answers the hand that reaches for it, and
  // the `.btn` family in `index.css` is that commandment in code. jsdom has no
  // layout, so what this can check is that the family is worn at all — the
  // hover, focus and press states themselves are Aaron's walk.
  it('dresses the lead panel’s controls in the button family too', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save this description' })).toBeTruthy())

    for (const name of ['Save this description', 'Edit it first', 'Ask for another',
      'Cancel']) {
      expect(screen.getByRole('button', { name }).className,
        `${name} is a bare button`).toContain('btn')
    }
  })
  it('dresses every control in the button family', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await openOver(GYOME)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude for a draft' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Replace what you wrote' })).toBeTruthy())

    for (const name of ['Ask again', 'Replace what you wrote']) {
      expect(screen.getByRole('button', { name }).className,
        `${name} is a bare button`).toContain('btn')
    }
    // The undo only exists on the far side of a replacement.
    fireEvent.click(screen.getByRole('button', { name: 'Replace what you wrote' }))
    expect(screen.getByRole('button', { name: 'Put your own words back' }).className)
      .toContain('btn')
  })
})

/**
 * The two beats, which is the whole of the 2026-08-30 redesign.
 *
 * Aaron's report was that the flow "isn't clear what the user should do or that
 * Claude is already working on a draft". Both halves of that were true and both
 * had the same cause: pressing "Ask Claude for a draft" dropped somebody into
 * an **empty textarea** over a greyed-out Save, with the only sign of the call
 * a disabled button at the bottom of the page whose label had quietly changed.
 * The screen asked them to write while Claude wrote.
 *
 * So beat one is a visible working state and beat two is a decision, and the
 * tests below are about the things a screenshot would not settle: that the box
 * is genuinely *gone* while Claude works, that the working state actually says
 * Claude is working, that approving is one press, and that every dead end has a
 * way out. The motion itself is `index.css` and Aaron's walk.
 */
describe('StrategyEditor: the two beats', () => {
  afterEach(cleanup)
  beforeEach(() => {
    vi.mocked(api.setDeckField).mockReset()
    vi.mocked(api.describeDeck).mockReset()
    vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(status())
  })

  /** A draft that has been asked for and has not come back yet — the ten to
   *  twenty seconds beat one exists for. Built per call rather than handed to
   *  `mockResolvedValue`, so the pending state is a real pending promise. */
  function pending() {
    let land: (draft: DeckDescriptionDraft) => void = () => {}
    vi.mocked(api.describeDeck).mockImplementation(
      () => new Promise<DeckDescriptionDraft>((resolve) => { land = resolve }))
    return (over?: Partial<DeckDescriptionDraft>) => { land(drafted(over)) }
  }

  // **Beat one.** The complaint in one assertion: while Claude is working there
  // is no empty box asking to be typed into, and there is something on screen
  // that says what is happening.
  it('shows Claude working, and no empty box, while the draft is in flight',
     async () => {
       const land = pending()
       await askFromEmpty()

       await waitFor(() => expect(screen.getByText(/Claude is reading your deck/))
         .toBeTruthy())
       // The box is gone rather than merely disabled: nothing to type into,
       // nothing greyed out, nothing to misread as "your turn".
       expect(screen.queryByLabelText('What this deck is trying to do')).toBeNull()
       expect(screen.queryByRole('button', { name: 'Save description' })).toBeNull()
       // And the wait is announced, not just drawn.
       expect(screen.getByRole('status').textContent)
         .toContain('Claude is reading your deck')

       land()
       await waitFor(() => expect(screen.getByText(/Cook Food, sacrifice it/)).toBeTruthy())
       // Beat two replaces beat one in the same place.
       expect(screen.queryByText(/Claude is reading your deck/)).toBeNull()
     })

  // **Beat two.** One press, and it is the same call a person's own typing goes
  // through. Nothing was in the box, so nothing could be lost by not stopping.
  it('saves the draft in one press over a deck that said nothing', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    vi.mocked(api.setDeckField).mockResolvedValue({ slug: 'gyome-food' } as never)
    const onDone = await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save this description' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Save this description' }))

    await waitFor(() =>
      expect(api.setDeckField).toHaveBeenCalledWith(REF, 'strategy', DRAFTED))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
  })

  // A refusal must not also lose the paragraph. The box comes back holding the
  // draft, so the second loss never happens and the retry is a keystroke away.
  it('leaves the draft in the box when the save is refused', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    vi.mocked(api.setDeckField).mockImplementation(
      () => Promise.reject(new Error('the deck moved under you')))
    await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save this description' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Save this description' }))

    await waitFor(() => expect(screen.getByText(/the deck moved under you/)).toBeTruthy())
    expect((screen.getByLabelText('What this deck is trying to do') as HTMLTextAreaElement)
      .value).toBe(DRAFTED)
  })

  // The lead panel is the whole screen, so every state it can reach has to
  // leave somebody somewhere. A panel that can only say "no" is a dead end.
  it('offers the pen when the draft comes back with nothing in it', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(
      drafted({ strategy: '', themes: [], reason: 'Nothing usable came back.' }))
    await askFromEmpty()

    await waitFor(() => expect(screen.getByText(/Nothing usable came back/)).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Write it myself' }))
    // An empty box, which is what was asked for — not the failed draft.
    expect((screen.getByLabelText('What this deck is trying to do') as HTMLTextAreaElement)
      .value).toBe('')
  })

  it('offers the pen when the call is refused outright', async () => {
    vi.mocked(api.describeDeck).mockImplementation(
      () => Promise.reject(new Error('claude is unavailable')))
    await askFromEmpty()

    await waitFor(() => expect(screen.getByText(/claude is unavailable/)).toBeTruthy())
    expect(screen.getByRole('button', { name: 'Write it myself' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeTruthy()
  })

  // Leaving saves nothing, which is the discard half of the beat.
  it('writes nothing when the draft is left', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Cancel' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(api.setDeckField).not.toHaveBeenCalled()
    // Back to the empty state's own invitation.
    expect(screen.getByText(/does not say what it is trying to do/)).toBeTruthy()
  })

  // The draft survives the move out of lead mode. It would not if the panel
  // remounted — which is why `StrategyEditor` renders it in one position and
  // hides the box around it, rather than branching into two editors.
  it('keeps the draft on screen after the box is opened over it', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(drafted())
    await askFromEmpty()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Edit it first' })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: 'Edit it first' }))
    expect(screen.getByText(/A draft by Claude, not the gate/)).toBeTruthy()
    expect(screen.getByText(/in the box above, and yours to edit/)).toBeTruthy()
    // And it was not asked for a second time on the way.
    expect(api.describeDeck).toHaveBeenCalledTimes(1)
  })
})
