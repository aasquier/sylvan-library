import { Fragment, useState } from 'react'

import { api, errorMessage } from '../lib/api'
import type { Combo, ComboCardRef, ComboDraft, CardOffer, DeckRef } from '../lib/api'
import {
  blankDraft, headingOf, steps, toDraft, withEntryAt, withoutEntryAt,
} from '../lib/combos'
import { CardFinder } from './cardfinder'
import { FieldHint } from './hint'
import { Term } from './term'
import { CardArt, CardHover, ErrorNote, ManaText } from './ui'

/**
 * The combos: the machines this deck can assemble, and the ones it is a card
 * short of.
 *
 * Aaron asked for it on 2026-08-30 — "a Combos section on the 99: names the
 * cards, the setup, whatever it takes; terse, no table manners". The block
 * lives in `deck.yaml` because a deck is one YAML file and that file is the
 * truth: combos are deck data, editable in place, carried by an import and
 * diffed by `swaps.md`, not a generated artifact and not app state.
 *
 * It sits between the 99 and the swap board, which is where it sits in the
 * deck's own life: after what the deck is, before what it is weighing.
 *
 * ## There is no name, and that is the design
 *
 * An entry is called after the cards it is made of, joined with " + ". That is
 * how anybody who plays the deck refers to it, and it is the one heading that
 * cannot go stale when a piece is swapped — a separate title would be a second
 * thing to keep true. It is also why there is no per-entry route: a combo has
 * no address, because its name is the very thing an edit to it changes. The
 * whole block is composed here and PUT in one write.
 *
 * ## Commandment 2 is the whole of the empty state
 *
 * "Combos: 0" tells a newcomer nothing except that they are missing something.
 * Somebody meeting Magic this week has heard "infinite mana" said at a table
 * and has no idea whether it is a rule, a card, or a joke. So the empty state
 * says what a combo *is* before it says how to write one down, the word itself
 * carries its glossary entry, and the copy is careful that cataloguing a
 * machine is a note about the deck rather than a change to it.
 *
 * ## The near-miss, and why it always names the cut
 *
 * An entry with a `needs` is one card short. Aaron's rule is that the cut
 * suggestion is always part of the entry — a card to bring in is only a
 * suggestion once there is a slot for it — so the trade is stated plainly and
 * the deck's own swap board is one press away.
 *
 * **That press does not write a rationale.** Rule 4 (ADR 8, ADR 11) says no
 * surface writes a `why` unasked, and "the app can see what this card is for"
 * is exactly the reasoning that would break it. So on a curated deck the
 * control opens a box for the sentence the person is owed the chance to write;
 * on a draft, where the rationale is honestly still to come, it goes straight
 * through. Both paths are the swap board's own rule, read off the same `stage`.
 *
 * ## Folded
 *
 * Collapsed by default, in the page's `.disclosure-toggle` idiom, like the swap
 * board below it. A reader with nothing to read gets no section at all; the
 * owner gets it either way, because the empty state is the whole feature for a
 * deck that has never catalogued anything.
 */
export function Combos({ combos = [], deckRef, stage, identity, writable, onChanged }: {
  /**
   * The catalogue.
   *
   * **Defaulted, and the default is a fact rather than a fallback.** A deck
   * payload with no `combos` key comes from a server that has no combos block
   * to send, and "this deck catalogues nothing" is exactly true of one. The
   * case is reachable for real: a deploy changes both halves and the browser is
   * the half that lies, so for a few seconds somebody can hold this bundle
   * against the previous server. Without the default that is not a missing
   * section — `combos.length` throws during render and takes the whole deck
   * page with it, the 99 included. Found by the suite, on a fixture that
   * predates the field.
   */
  combos?: Combo[]
  deckRef: DeckRef
  /** Whether a rationale is owed on the swap board (ADR 13), read off the deck
   *  rather than assumed — a draft owes its reasons rather than being refused
   *  work while the thinking is still to come. */
  stage: string
  /** The deck's colour identity, so a piece outside it is marked while it is
   *  being chosen. Marked, never refused: a catalogue may name a card this
   *  deck could not legally run, and saying so is more use than forbidding it. */
  identity: string[]
  writable: boolean
  onChanged: () => void
}) {
  const [open, setOpen] = useState(false)
  /** Which entry is being edited, by index; -1 is the new one; null is none. */
  const [editing, setEditing] = useState<number | null>(null)

  // A reader has no use for an empty shelf: nothing to read and no control to
  // press. The owner gets it either way — the empty state is where a catalogue
  // is started, and the whole feature for a deck that has none.
  if (combos.length === 0 && !writable) return null

  async function write(next: ComboDraft[]) {
    await api.setCombos(deckRef, next)
    setEditing(null)
    onChanged()
  }

  return (
    <section className="space-y-2 border-t pt-4"
             style={{ borderColor: 'var(--hairline)' }}>
      {/* Folded, this section is the word "Combos" and nothing else, and
          somebody meeting Magic this week does not know whether that is
          something they should have. The mark answers before the fold opens.
          `FieldHint` rather than a `title`, which draws on hover and on
          nothing else — never on a phone, never on keyboard focus. */}
      <h3 className="flex items-center gap-1 text-sm font-semibold">
        <FieldHint name="What a combo is"
                   says={'Two or three cards that, put together, do something '
                         + 'none of them can do alone — often over and over. '
                         + 'Writing them down says which cards to protect and '
                         + 'which never to cut.'}>
          <span aria-hidden className="combo-glyph">∞</span>
        </FieldHint>
        <button type="button"
                onClick={() => { setOpen((was) => !was) }}
                aria-expanded={open}
                className="disclosure-toggle flex flex-1 items-center gap-2 text-left">
          <span aria-hidden className="text-[10px]"
                style={{
                  display: 'inline-block',
                  transition: 'transform 150ms',
                  transform: open ? 'none' : 'rotate(-90deg)',
                }}>▾</span>
          <span style={{ color: 'var(--text-primary)' }}>Combos</span>
          {/* No count on an empty catalogue. A "0" beside a heading reads as a
              score rather than as a shelf nobody has used yet. */}
          {combos.length > 0 && (
            <span className="tabular text-xs font-normal"
                  style={{ color: 'var(--text-muted)' }}>
              {combos.length}
            </span>
          )}
        </button>
      </h3>

      {open && (
        <div className="space-y-3">
          {combos.length === 0
            ? <EmptyCatalogue />
            : (
              <p className="max-w-2xl text-xs leading-relaxed"
                 style={{ color: 'var(--text-muted)' }}>
                What this deck can assemble, and what it is a card short of.
                Nothing here is simulated or checked against the rules — a{' '}
                <Term name="combo">combo</Term> is the deck’s own note about
                itself, and the cards it names are the ones never to cut.
              </p>
            )}

          {combos.length > 0 && (
            <ul className="space-y-3">
              {combos.map((combo, i) => (
                <li key={headingOf(combo) + String(i)}>
                  {editing === i
                    ? (
                      <ComboForm
                        combo={combo} identity={identity}
                        onSave={(entry) => write(withEntryAt(combos, i, entry))}
                        onCancel={() => { setEditing(null) }} />
                      )
                    : (
                      <ComboEntry
                        combo={combo} deckRef={deckRef} stage={stage}
                        writable={writable}
                        onEdit={() => { setEditing(i) }}
                        onRemove={() => write(withoutEntryAt(combos, i))}
                        onChanged={onChanged} />
                      )}
                </li>
              ))}
            </ul>
          )}

          {writable && (editing === -1
            ? (
              <ComboForm
                combo={null} identity={identity}
                onSave={(entry) => write([...combos.map(toDraft), entry])}
                onCancel={() => { setEditing(null) }} />
              )
            : editing === null && (
              <button type="button" onClick={() => { setEditing(-1) }}
                      className="btn btn-primary btn-accent-2 btn-sm">
                {combos.length > 0 ? '+ Catalogue another' : '+ Catalogue a combo'}
              </button>
            ))}
        </div>
      )}
    </section>
  )
}

/**
 * The empty state, which for a deck that has never catalogued anything is the
 * whole feature.
 *
 * Not "No combos." — an empty state that only reports emptiness has spent the
 * one screen where an explanation was going to be read. This says what a combo
 * is, says the reassuring half out loud (writing one down changes nothing about
 * the deck), and says the thing a newcomer most needs to hear: you do not need
 * one.
 */
function EmptyCatalogue() {
  return (
    <div className="combo-empty max-w-2xl space-y-2 rounded-lg p-3">
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
        A <Term name="combo">combo</Term> is a small machine: two or three cards
        that, side by side, do something none of them can do alone — untapping
        each other, paying for each other, going round again. Some loop until
        they win the game. Most are quieter than that.
      </p>
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Writing one down here changes nothing about the deck. It is a note to
        yourself and to whoever picks the deck up next: these are the cards to
        protect, this is the order to do it in, and this is the one card the
        deck is still short of. Plenty of good decks have none at all.
      </p>
    </div>
  )
}

/**
 * One machine, drawn.
 *
 * The pieces get their paintings, because that is how anybody actually
 * recognises a combo — and the paintings are `art_crop` rather than the whole
 * card for the reason every other row on this page uses one: a full card at
 * sixty pixels is a grey rectangle. Nothing is filtered, tinted or stretched;
 * a piece the deck no longer has is said in words instead (commandment 19).
 */
function ComboEntry({ combo, deckRef, stage, writable, onEdit, onRemove, onChanged }: {
  combo: Combo
  deckRef: DeckRef
  stage: string
  writable: boolean
  onEdit: () => void
  /** **A promise, and the type is load-bearing.** Declared `() => void` this
   *  caught nothing: TypeScript erases the promise, so the `await` below has
   *  nothing to await and a server refusal becomes an unhandled rejection with
   *  no sentence anywhere on the page. */
  onRemove: () => Promise<void>
  onChanged: () => void
}) {
  const [armed, setArmed] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const near = combo.needs !== null

  async function remove() {
    setBusy(true)
    setError('')
    try {
      await onRemove()
    } catch (e: unknown) {
      setError(errorMessage(e))
      setArmed(false)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={`combo-entry card-surface rounded-lg p-3${near ? ' is-near' : ''}`}>
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-0 flex-1 basis-64 space-y-2">
          {/* Inline rather than a flex row, and the spaces around the "+" are
              real text rather than a `gap`. A flex row reads back as
              "Axebane Guardian+High Alert" — no whitespace between the
              children at all — which is what a screen reader is handed and
              what a copy-paste produces. The heading is the entry's only
              name; it has to survive being read out loud. */}
          <h4 className="text-sm font-semibold leading-relaxed">
            {combo.cards.map((card, i) => (
              <Fragment key={card.name}>
                {i > 0 && <span className="combo-plus"> + </span>}
                <Piece card={card} />
              </Fragment>
            ))}
          </h4>

          <p className="text-xs leading-relaxed">
            <span className="combo-label">Makes</span>{' '}
            <span style={{ color: 'var(--text-primary)' }}>
              <ManaText>{combo.produces}</ManaText>
            </span>
          </p>

          <Instructions how={combo.how} />

          {combo.setup && (
            <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
              <span className="combo-label">Costs</span>{' '}
              <ManaText>{combo.setup}</ManaText>
            </p>
          )}

          {/* The mark, in the same words the rationale editor uses one section
              up — a reader should meet one sentence about who wrote a thing,
              not two. Nothing in this phase writes it; a deck file that carries
              one keeps it, and it comes off the moment somebody edits the
              entry. */}
          {combo.by === 'claude' && (
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              Claude drafted this
            </p>
          )}
        </div>

        <div className="flex shrink-0 flex-wrap gap-1.5">
          {combo.cards.map((card) => (
            <CardHover key={card.name} card={card}>
              <CardArt src={card.art_crop} alt={card.name}
                       ratio="aspect-[626/457]"
                       className="w-16 shrink-0 cursor-help" />
            </CardHover>
          ))}
        </div>
      </div>

      {combo.needs && (
        <Trade needs={combo.needs} cut={combo.cut} deckRef={deckRef}
               stage={stage} writable={writable} onAdded={onChanged} />
      )}

      {error && <div className="mt-2"><ErrorNote>{error}</ErrorNote></div>}

      {writable && (
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <button type="button" onClick={onEdit} className="btn btn-ghost btn-xs">
            Edit
          </button>
          {/* Armed rather than confirmed in a dialog, which is the pattern the
              99's own entomb uses two sections up: the second press is the
              consent, and the button says so rather than a modal asking. */}
          <button type="button" disabled={busy}
                  onClick={() => {
                    if (armed) { void remove() } else { setArmed(true) }
                  }}
                  className="btn btn-ghost btn-xs">
            {armed ? 'Press again to remove' : 'Remove'}
          </button>
          {armed && !busy && (
            <button type="button" onClick={() => { setArmed(false) }}
                    className="btn btn-ghost btn-xs">
              Keep it
            </button>
          )}
        </div>
      )}
    </div>
  )
}

/**
 * One card's name, hovering its own painting — and saying so when the deck no
 * longer has it.
 *
 * A piece missing from the 99 is the commonest way a catalogue goes stale, and
 * the gate warns about it separately. Here it is a quiet mark rather than a
 * red one: the entry is still worth reading, and the person may well be about
 * to put the card back.
 */
function Piece({ card }: { card: ComboCardRef }) {
  return (
    <CardHover card={card}>
      <span className={`combo-piece${card.in_deck ? '' : ' is-absent'}`}>
        {card.name}
        {!card.in_deck && (
          <span className="combo-piece-note"> (not in the deck)</span>
        )}
      </span>
    </CardHover>
  )
}

/**
 * The instructions, numbered.
 *
 * The block is written "1) … 2) … 3) …" because that is how the council emits
 * one and how a person types one, and a wall of prose is the hardest thing on
 * this page to follow at a table. Split into real steps when the numbering is
 * there and left as a paragraph when it is not — a renderer that renumbered
 * somebody's prose would be inventing a structure they did not write.
 */
function Instructions({ how }: { how: string }) {
  const parts = steps(how)
  if (parts === null) {
    return how.trim()
      ? (
        <p className="text-xs leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
          <ManaText>{how}</ManaText>
        </p>
        )
      : null
  }
  return (
    <ol className="combo-steps space-y-0.5 text-xs leading-relaxed">
      {parts.map((step, i) => (
        <li key={i}><ManaText>{step}</ManaText></li>
      ))}
    </ol>
  )
}

/**
 * The trade a near-miss offers: bring in X, cut Y.
 *
 * Said in the plainest words the section has, because it is the one thing here
 * somebody can act on today. The swap board is where the acting happens — the
 * direct trade into the 99 would be a swap, which is the edit panel's job and
 * needs a rationale for the incoming card either way.
 */
function Trade({ needs, cut, deckRef, stage, writable, onAdded }: {
  needs: ComboCardRef
  cut: ComboCardRef | null
  deckRef: DeckRef
  stage: string
  writable: boolean
  onAdded: () => void
}) {
  // The swap board's own rule, read off the same field: a draft owes its
  // rationales rather than being refused work while the thinking is still to
  // come (ADR 13). Measured against the server rather than assumed.
  const rationaleRequired = stage !== 'draft'
  const [asking, setAsking] = useState(false)
  const [why, setWhy] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)

  async function put() {
    setBusy(true)
    setError('')
    try {
      await api.addToBoard(deckRef, {
        name: needs.name, category: 'threat', why: why.trim(),
      })
      setDone(true)
      setAsking(false)
      setWhy('')
      onAdded()
    } catch (e: unknown) {
      // The server's own sentence, verbatim. It is the one implementation of
      // every rule this control obeys.
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="combo-trade mt-2 space-y-2 rounded-md p-2.5">
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
        <span className="combo-away">One card away.</span>{' '}
        Bring in <Piece card={needs} />
        {cut
          ? <> ; cut <Piece card={cut} />.</>
          : <>. Nothing is named to come out for it yet.</>}
      </p>

      {writable && !done && !asking && (
        <button type="button"
                disabled={busy}
                onClick={() => {
                  if (rationaleRequired) { setAsking(true); return }
                  void put()
                }}
                className="btn btn-primary btn-accent-2 btn-xs">
          {busy ? 'Setting it down…' : `Weigh ${needs.name}`}
        </button>
      )}

      {/* **The rationale is asked for, never composed.** Rule 4 (ADR 8, ADR 11)
          says no surface writes a `why` unasked, and "the app can see what this
          card is for" is exactly the reasoning that would break it. The box
          opens empty. */}
      {writable && asking && (
        <div className="space-y-2">
          <label className="block space-y-1">
            <span className="text-[11px] font-medium uppercase tracking-wide"
                  style={{ color: 'var(--text-muted)' }}>
              Why you are weighing it
            </span>
            <textarea value={why} onChange={(e) => { setWhy(e.target.value) }} rows={2}
                      placeholder="In your words — what it would do for this deck."
                      className="combo-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
          </label>
          <div className="flex flex-wrap items-center gap-3">
            <button type="button" onClick={() => { void put() }}
                    disabled={busy || why.trim() === ''}
                    className="btn btn-primary btn-accent-2 btn-xs">
              {busy ? 'Setting it down…' : 'Put it on the swap board'}
            </button>
            <button type="button"
                    onClick={() => { setAsking(false); setWhy(''); setError('') }}
                    className="btn btn-ghost btn-xs">
              Cancel
            </button>
            <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
              A card is worth weighing only if you can say why.
            </span>
          </div>
        </div>
      )}

      {done && (
        <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          {needs.name} is on the swap board.
        </p>
      )}

      {error && <ErrorNote>{error}</ErrorNote>}
    </div>
  )
}

/**
 * The form, for a new entry and for one being changed.
 *
 * The same form both ways, because the fields are the same question: an edit
 * that offered a different shape from the add would be two things to keep
 * true. It composes a `ComboDraft` and hands it up; the section assembles the
 * whole block and writes it in one call, which is the only write there is.
 */
function ComboForm({ combo, identity, onSave, onCancel }: {
  combo: Combo | null
  identity: string[]
  onSave: (entry: ComboDraft) => Promise<void>
  onCancel: () => void
}) {
  const start = combo ? toDraft(combo) : blankDraft()
  const [cards, setCards] = useState<string[]>(start.cards)
  const [produces, setProduces] = useState(start.produces)
  const [how, setHow] = useState(start.how)
  const [setup, setSetup] = useState(start.setup)
  const [near, setNear] = useState(start.needs !== '')
  const [needs, setNeeds] = useState(start.needs)
  const [cut, setCut] = useState(start.cut)
  const [picking, setPicking] = useState<CardOffer | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // The two the server insists on, said here so the button is off rather than
  // the save being refused: an entry with no pieces has no heading, and one
  // that does not say what it produces is a list of cards.
  const ready = cards.length > 0 && produces.trim() !== ''
    && (!near || needs.trim() !== '')

  function addPiece(chosen: CardOffer | null) {
    setPicking(null)
    if (!chosen) return
    if (cards.some((name) => name.toLowerCase() === chosen.name.toLowerCase())) return
    setCards([...cards, chosen.name])
  }

  async function submit() {
    setBusy(true)
    setError('')
    try {
      await onSave({
        cards, produces: produces.trim(), how: how.trim(), setup: setup.trim(),
        needs: near ? needs.trim() : '', cut: near ? cut.trim() : '',
      })
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card-surface w-full space-y-3 rounded-lg p-4">
      <div className="space-y-2">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          The pieces
        </span>
        {cards.length > 0 && (
          <ul className="flex flex-wrap gap-1.5">
            {cards.map((name) => (
              <li key={name}>
                <button type="button"
                        onClick={() => { setCards(cards.filter((c) => c !== name)) }}
                        aria-label={`Remove ${name} from the pieces`}
                        className="combo-chip">
                  {name}<span aria-hidden className="combo-chip-x">×</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {/* The finder gets the full width: it carries a painting and a list,
            and a card squeezed into a third of a grid is the thing it exists
            to stop. */}
        <CardFinder value={picking} onChange={addPiece} identity={identity}
                    label="Add a piece" />
        <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          The heading is the pieces, joined — there is no separate name to
          write.
        </p>
      </div>

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          What it makes
        </span>
        <input value={produces} onChange={(e) => { setProduces(e.target.value) }}
               placeholder="infinite colored mana; infinite untaps of your creatures"
               className="combo-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
      </label>

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          How to turn it
        </span>
        <textarea value={how} onChange={(e) => { setHow(e.target.value) }} rows={3}
                  placeholder="1) Tap the Guardian. 2) Pay to untap it. 3) Repeat."
                  className="combo-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
        <span className="block text-[11px]" style={{ color: 'var(--text-muted)' }}>
          Number the steps and they are drawn as a list.
        </span>
      </label>

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          What it costs to set up
        </span>
        <textarea value={setup} onChange={(e) => { setSetup(e.target.value) }} rows={2}
                  placeholder="six mana across two turns, and a creature that is not summoning sick"
                  className="combo-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
      </label>

      {/* A setting, not a destination, so it is a button that says which state
          it is in (commandment 20). */}
      <button type="button" aria-pressed={near}
              onClick={() => { setNear(!near) }}
              className={`chip-toggle rounded-full px-3 py-1.5 text-xs font-medium${
                near ? ' is-on' : ''}`}>
        This deck is one card short
      </button>

      {near && (
        <div className="combo-trade space-y-3 rounded-md p-2.5">
          <NameField label="The card it needs" value={needs} onChange={setNeeds}
                     identity={identity} />
          <NameField label="What would come out for it" value={cut} onChange={setCut}
                     identity={identity} />
          <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            A card to bring in is only a suggestion once there is a slot for
            it, so name the cut as well.
          </p>
        </div>
      )}

      {error && <ErrorNote>{error}</ErrorNote>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={() => { void submit() }}
                disabled={!ready || busy}
                className="btn btn-primary btn-accent-2 btn-sm">
          {busy ? 'Writing it down…' : combo ? 'Save the combo' : 'Catalogue it'}
        </button>
        <button type="button" onClick={onCancel} className="btn btn-ghost btn-xs">
          Cancel
        </button>
        {!ready && (
          <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            {cards.length === 0
              ? 'Name at least one piece.'
              : produces.trim() === ''
                ? 'Say what it makes.'
                : 'Name the card it needs.'}
          </span>
        )}
      </div>
    </div>
  )
}

/**
 * One card name in the trade, chosen from the library rather than typed.
 *
 * The picker rather than a text box because the deck file's names are the card
 * pool's names — a hover keys on the exact one — and a typo here is a piece
 * with no painting. Clearing it is a real operation, so the chosen name is
 * shown with a way to take it off again.
 */
function NameField({ label, value, onChange, identity }: {
  label: string
  value: string
  onChange: (name: string) => void
  identity: string[]
}) {
  const [picking, setPicking] = useState<CardOffer | null>(null)
  if (value) {
    return (
      <div className="space-y-1">
        <span className="block text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          {label}
        </span>
        <button type="button" onClick={() => { onChange('') }}
                aria-label={`Clear ${label}: ${value}`}
                className="combo-chip">
          {value}<span aria-hidden className="combo-chip-x">×</span>
        </button>
      </div>
    )
  }
  return (
    <CardFinder value={picking} onChange={(chosen) => {
      setPicking(null)
      if (chosen) onChange(chosen.name)
    }} identity={identity} label={label} />
  )
}

