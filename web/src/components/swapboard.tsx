import { useMemo, useState } from 'react'

import { api, errorMessage } from '../lib/api'
import type { Card, CardOffer, DeckRef, EditResult } from '../lib/api'
import { CATEGORY_LABELS, categoryLabel } from '../lib/mtg'
import { CardFinder } from './cardfinder'
import { FieldHint } from './hint'
import { CardArt, CardHover, ErrorNote, ManaCost, ManaText, Select } from './ui'

/**
 * The swap board: the cards beside the deck rather than in it.
 *
 * `swap_board` is the model's name for the maybeboard — Commander has no
 * sideboard, so a list of cards you are weighing is exactly what it is, and
 * Aaron kept the name when asked directly on 2026-08-29. `decklist.Parse`
 * files a pasted `Sideboard`, `Maybeboard` or `Considering` section here, our
 * own moxfield.txt artifact writes the commander and companion into
 * `SIDEBOARD:`, and the edit panel can move a card here — so decks have been
 * arriving with a bench for as long as importing has existed, and until
 * 2026-08-24 the only place to look at one was the YAML (Aaron: "do we have
 * the notion of a sidepanel in our decks yet?").
 *
 * It sits between the 99 and the graveyard, because that is where it sits in
 * the deck's own life: not in, not dead, still in the argument.
 *
 * ## What changed on 2026-08-29
 *
 * Aaron: "When a deck doesn't already have a Sideboard a user should be able
 * to start one from scratch and add cards, or add to an existing sideboard. It
 * should use our fuzzy matcher for the picker to help. Also right now the
 * Sideboard doesn't collapse like the other areas. Its default should be
 * collapsed."
 *
 * Three things, and the first was the one with teeth. The section used to
 * render under `deck.swap_board.length > 0`, so a deck that had never kept a
 * board had **no section at all** — no heading, no explanation, and nowhere to
 * begin. The feature was invisible to exactly the decks that needed it
 * introduced.
 *
 * **A writable viewer always gets the section; a reader gets it only when
 * there is something on it.** An empty shelf is an invitation to its owner and
 * furniture to everybody else, and a reader has no use for a control they
 * cannot press.
 *
 * ## Adding — and the mover, which exists now
 *
 * The read-only version of this section carried a comment arguing that moving
 * a card between the two lists is a surgical edit belonging to the edit panel
 * (ADR 12), "not to a second door onto the same operation". For a long time
 * "there is still no mover" was this file's standing answer. It is not any
 * more: **the mover is each row's "Swap it in"**, the swap route's second
 * door. The board card joins the 99 in the slot of a card the owner picks
 * from their own deck, the picked card is entombed with its rationale intact
 * — entombed, never deleted — and the whole exchange is one write and one
 * history row ("swapped X out for Y from the swap board"). `PromoteComposer`
 * below composes it; `api.swapCard` carries it.
 *
 * What has not changed is the *add*: putting a card on the board is still a
 * different act from moving one between lists — the first weighs a card the
 * deck never had, the second plays the stake the board was holding.
 *
 * The add goes through `api.addToBoard`, which is `POST .../board` rather than
 * the `/cards` route with a `to`. The two are not interchangeable: `/cards`
 * refuses a deck file with no `swap_board:` block, deliberately, because an
 * edit changes what a deck says and never what shape it has. Starting a board
 * is a shape change, so it is asked for by name. The Go side argues it at
 * length in `internal/deckedit/board.go`.
 *
 * ## The rules the form obeys, all of them the server's
 *
 * Every one of these was measured against the running server rather than
 * reasoned about, because a form that guesses at a rule renders a refusal as a
 * surprise:
 *
 * - **A rationale is required unless the deck is a draft** (ADR 13), exactly
 *   as it is for the 99. The board carries no obligation to be *finished* —
 *   nothing on it counts towards a draft's outstanding rationales — but a card
 *   put here was still chosen, and rule 4 asks the same question of it.
 * - **Colour identity is enforced**, on the board as in the deck. A swap board
 *   is where you weigh a card, which is an argument for letting an unplayable
 *   one sit on it; the server does not, and a form that pretended otherwise
 *   would collect a rationale and then throw it away. `CardFinder` marks the
 *   card at the moment it is chosen instead, which is the earliest anybody can
 *   be told.
 * - **The categories are a fixed vocabulary**, so the field is a select rather
 *   than a text box that can be wrong.
 *
 * ## Folded
 *
 * Collapsed by default, like `TokenShelf` two sections below and for the same
 * reason — it is part of the deck's paperwork rather than one of the toys, so
 * it takes the page's `.disclosure-toggle` section idiom. The fold does not
 * survive a reload, deliberately: "collapsed by default" is what was asked
 * for, and a remembered fold is a different feature.
 */
export function SwapBoard({ deck, cards, deckRef, stage, identity, total, writable, onChanged }: {
  deck: Card[]
  /** The deck's own 99, for the mover: swapping a board card in means picking
   *  which of these makes room, and the pool is the wrong instrument for
   *  choosing among cards you already have. */
  cards: Card[]
  deckRef: DeckRef
  stage: string
  /** The deck's colour identity, so a card outside it is marked while it is
   *  being chosen rather than refused after a rationale has been written. */
  identity: string[]
  /** The size of the 99, which is what the board sits outside of. Read off the
   *  deck rather than written down, because it is 99 for most decks and not
   *  for all of them. */
  total: number
  writable: boolean
  onChanged: () => void
}) {
  const [open, setOpen] = useState(false)
  // The board card whose promotion is being composed, if any. One at a time,
  // like every under-the-row composer on the deck page.
  const [promoting, setPromoting] = useState<string | null>(null)

  // A reader has no use for an empty shelf: no cards to read and no control to
  // press. The owner gets it either way, because the empty state is where a
  // board is started and the whole feature for a deck that has none.
  if (deck.length === 0 && !writable) return null

  return (
    <section className="space-y-2 border-t pt-4"
             style={{ borderColor: 'var(--hairline)' }}>
      {/* Folded, this section is the words "Swap board" and nothing else, and
          somebody meeting Magic this week has no idea whether that is
          something they should have. The mark answers before the fold opens;
          the paragraph inside answers at length after. `FieldHint` rather than
          a `title`, which draws on hover and on nothing else — never on a
          phone, never on keyboard focus, and this repo has found that out four
          times now. */}
      <h3 className="flex items-center gap-1 text-sm font-semibold">
        <FieldHint name="What a swap board is"
                   says={'The shortlist beside the deck: cards you are '
                         + 'weighing but have not cut anything for. They are '
                         + 'not part of the deck and are never drawn.'}>
          <span aria-hidden className="swap-glyph">⇄</span>
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
          <span style={{ color: 'var(--text-primary)' }}>Swap board</span>
          {/* No count on an empty board. A "0" beside a heading reads as a
              score rather than as a shelf nobody has used yet. */}
          {deck.length > 0 && (
            <span className="tabular text-xs font-normal"
                  style={{ color: 'var(--text-muted)' }}>
              {deck.length}
            </span>
          )}
        </button>
      </h3>

      {open && (
        <div className="space-y-3">
          {deck.length === 0
            ? <EmptyBoard total={total} />
            : (
              <p className="max-w-2xl text-xs leading-relaxed"
                 style={{ color: 'var(--text-muted)' }}>
                Cards under consideration, outside the {total}. They are not
                simulated, not validated against the 99, and carry no
                obligation — a maybeboard by its proper name. Nothing here
                counts towards a draft’s outstanding rationales.
              </p>
            )}

          {deck.length > 0 && (
            <ul className="space-y-1">
              {deck.map((card) => (
                <li key={card.name} className="card-surface rounded-lg p-2">
                  <div className="flex flex-wrap items-center gap-3">
                    <CardHover card={card}>
                      <CardArt src={card.art_crop} alt={card.name}
                               ratio="aspect-[626/457]"
                               className="w-16 shrink-0 cursor-help" />
                    </CardHover>
                    <div className="min-w-0 flex-1 basis-52">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <CardHover card={card}>
                          <span className="cursor-help text-sm font-medium">
                            {card.qty > 1 && <span className="tabular mr-1">{card.qty}×</span>}
                            {card.name}
                          </span>
                        </CardHover>
                        <ManaCost cost={card.mana_cost} />
                      </div>
                      {card.why && (
                        <p className="mt-0.5 text-xs leading-relaxed"
                           style={{ color: 'var(--text-secondary)' }}>
                          <ManaText>{card.why}</ManaText>
                        </p>
                      )}
                    </div>
                    {/* The mover (the graveyard rows' per-row-chip shape). A
                        toggle rather than a one-shot: it opens the composer
                        below, and the deliberate second step — a card picked
                        to make room, a fresh why — lives there, which is why
                        this needs no arming. */}
                    {writable && (
                      <button type="button"
                              onClick={() => {
                                setPromoting(promoting === card.name ? null : card.name)
                              }}
                              aria-pressed={promoting === card.name}
                              className="card-action shrink-0 rounded-md px-2 py-1 text-[11px] font-medium">
                        Swap it in
                      </button>
                    )}
                  </div>
                  {promoting === card.name && (
                    <PromoteComposer
                      deckRef={deckRef}
                      card={card}
                      cards={cards}
                      onDone={() => { setPromoting(null); onChanged() }}
                      onCancel={() => setPromoting(null)} />
                  )}
                </li>
              ))}
            </ul>
          )}

          {writable && (
            <AddToBoardForm deckRef={deckRef} stage={stage} identity={identity}
                            started={deck.length > 0} onDone={onChanged} />
          )}
        </div>
      )}
    </section>
  )
}

/**
 * The empty state, which for a deck that has never kept a board is the whole
 * feature.
 *
 * Commandment 2 at the moment it is hardest to serve. Somebody meeting Magic
 * this week has not met a maybeboard, and "Swap board: 0" would tell them
 * nothing except that they are missing something. So this says what one is
 * before it says how to start one, and it says the reassuring half out loud:
 * putting a card here changes nothing about the deck. The commonest reason a
 * newcomer will not press a button is a fear that it does something.
 *
 * Not "No cards." — an empty state that only reports emptiness has spent the
 * one screen where an explanation was going to be read.
 */
function EmptyBoard({ total }: { total: number }) {
  return (
    <div className="swap-empty max-w-2xl space-y-2 rounded-lg p-3">
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
        Every deck has a shortlist it never cut anything for. The swap board is
        where that list lives — the cards you keep coming back to, each with
        the reason you keep coming back to it.
      </p>
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Commander has no sideboard, so nothing here is ever shuffled in.
        A card on the board is not drawn, not simulated, and not counted
        against the {total} — putting one here changes nothing about the deck.
        It is somewhere to think out loud.
      </p>
    </div>
  )
}

/**
 * The mover — the swap route's second door, composed from this side.
 *
 * The incoming card is fixed: it is the board row that opened this. What the
 * owner chooses is which of their own cards makes room, and that choice is a
 * **client-side list of the deck's 99, grouped by category** rather than the
 * card finder — the pool is the wrong instrument for choosing among cards you
 * already have, and the categories are how an owner actually thinks about
 * where a newcomer fits ("this replaces a threat", not "this replaces the
 * fourth card alphabetically").
 *
 * The board entry's own `why` is shown as context and **never prefilled into
 * the box** (rule 4, ADR 8): it argued why the card was *waiting*, and the
 * sentence this form collects argues the reversal. The server refuses a blank
 * `why` on this door for drafts too — the route's rule, not this form's — so
 * the button stays disabled until a human has written one.
 *
 * What the server does with the request, in plain words below the pick: the
 * outgoing card is entombed with its rationale intact — entombed, never
 * deleted — and the board card takes over its slot. One write, one history
 * row: "swapped X out for Y from the swap board".
 */
function PromoteComposer({ deckRef, card, cards, onDone, onCancel }: {
  deckRef: DeckRef
  /** The board card coming in — fixed by the row that opened this. */
  card: Card
  /** The deck's own 99, to pick the outgoing card from. */
  cards: Card[]
  onDone: () => void
  onCancel: () => void
}) {
  const [out, setOut] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [why, setWhy] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  // The 99 in the order the deck page shows them: the known categories in
  // their fixed vocabulary order, any stray key after, and the filter
  // narrowing by name inside that shape rather than flattening it.
  const groups = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    const kept = needle
      ? cards.filter((c) => c.name.toLowerCase().includes(needle))
      : cards
    const by = new Map<string, Card[]>()
    for (const c of kept) {
      const bucket = by.get(c.category) ?? []
      bucket.push(c)
      by.set(c.category, bucket)
    }
    const known = Object.keys(CATEGORY_LABELS)
    const strays = [...by.keys()].filter((k) => !known.includes(k)).sort()
    return [...known, ...strays]
      .filter((k) => by.has(k))
      .map((k) => [k, by.get(k) ?? []] as const)
  }, [cards, filter])

  const ready = out !== null && why.trim() !== ''

  async function apply() {
    if (out === null || !why.trim()) return
    setBusy(true)
    setError('')
    try {
      await api.swapCard(deckRef, { out, into: card.name, why: why.trim() })
      onDone()
    } catch (e: unknown) {
      // The server's own sentence, verbatim — it owns every rule this form
      // obeys, and a paraphrase here would be a second implementation.
      setError(errorMessage(e))
      setBusy(false)
    }
  }

  return (
    <div className="card-surface mt-2 w-full space-y-3 rounded-lg p-3">
      <p className="text-xs font-medium">
        Swap {card.name} in — pick which card makes room.
      </p>
      {/* Context, never a prefill: this sentence argued why the card was
          waiting, and the box below collects the opposite claim. */}
      {card.why && (
        <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
          Why it was waiting: <ManaText>{card.why}</ManaText>
        </p>
      )}

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          Narrow the {cards.length} by name
        </span>
        <input value={filter} onChange={(e) => { setFilter(e.target.value) }}
               placeholder="Type part of a card name…"
               className="swap-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
      </label>

      {groups.length === 0 && (
        <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
          No card in the deck answers to that. Clear the filter to see all of
          them again.
        </p>
      )}
      {groups.map(([key, list]) => (
        <div key={key} className="space-y-1">
          <p className="text-[10px] uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
            {categoryLabel(key)}
          </p>
          <div className="flex flex-wrap gap-1.5">
            {list.map((c) => (
              <button key={c.name} type="button"
                      onClick={() => { setOut(out === c.name ? null : c.name); setError('') }}
                      aria-pressed={out === c.name}
                      className="card-action rounded-md px-2 py-1 text-[11px] font-medium">
                {c.name}
              </button>
            ))}
          </div>
        </div>
      ))}

      {out !== null && (
        <p className="text-xs leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
          {out} goes to the graveyard with its reason kept — it can come back —
          and {card.name} takes over its slot.
        </p>
      )}

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          Why it earns the slot
        </span>
        <textarea value={why} onChange={(e) => { setWhy(e.target.value) }} rows={2}
                  placeholder="Why does this card earn the slot? Required — the gate will not accept a card without a rationale."
                  className="swap-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
      </label>

      {error && <ErrorNote>{error}</ErrorNote>}

      <div className="flex flex-wrap items-center gap-2">
        <button type="button" onClick={() => { void apply() }}
                disabled={!ready || busy}
                className="btn btn-primary btn-accent-1 btn-sm">
          {busy ? 'Swapping…' : 'Apply swap'}
        </button>
        <button type="button" onClick={onCancel} className="btn btn-ghost btn-xs">
          Cancel
        </button>
        <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
          Writes deck.yaml. The History tab records the swap.
        </span>
      </div>
    </div>
  )
}

/**
 * The add.
 *
 * Closed it is one button; open it is the same finder the edit panel uses,
 * with the three fields the server actually asks for. Nothing here is a second
 * copy of that panel's form: it has an "Into" select for choosing between the
 * 99 and the board, and a form that lives *inside* the board has already been
 * asked that question.
 */
function AddToBoardForm({ deckRef, stage, identity, started, onDone }: {
  deckRef: DeckRef
  stage: string
  identity: string[]
  /** Whether this deck already has a board, which is the only thing the two
   *  labels differ on. The request is the same either way — the server opens a
   *  board that is not there, in the same write that puts the card on it. */
  started: boolean
  onDone: () => void
}) {
  const [open, setOpen] = useState(false)
  const [card, setCard] = useState<CardOffer | null>(null)
  const [category, setCategory] = useState('threat')
  // Whether the person has filed this card themselves. Until they do, picking
  // a land re-files it — the card pool's own fact rather than an opinion, and
  // the only field any surface here fills in unasked.
  const [filed, setFiled] = useState(false)
  const [why, setWhy] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  // A draft owes its rationales rather than being refused work while the
  // thinking is still to come (ADR 13). Measured against the server, not
  // assumed: a curated deck's board refuses a blank `why` exactly as its 99
  // does.
  const rationaleRequired = stage !== 'draft'
  const ready = card !== null && (!rationaleRequired || why.trim() !== '')

  function pick(chosen: CardOffer | null) {
    setCard(chosen)
    setError('')
    if (chosen && !filed) setCategory(chosen.is_land ? 'land' : 'threat')
  }

  async function submit() {
    if (!card) return
    setBusy(true)
    setError('')
    try {
      const result: EditResult = await api.addToBoard(deckRef, {
        name: card.name, category, why: why.trim(),
      })
      void result
      setCard(null)
      setWhy('')
      setFiled(false)
      setOpen(false)
      onDone()
    } catch (e: unknown) {
      // The server's own sentence, verbatim. It is the one implementation of
      // every rule this form obeys, and paraphrasing it here would be a second
      // one that could drift.
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  if (!open) {
    return (
      <button type="button" onClick={() => { setOpen(true) }}
              className="btn btn-primary btn-accent-2 btn-sm">
        {started ? '+ Weigh another card' : '+ Start a swap board'}
      </button>
    )
  }

  return (
    <div className="card-surface w-full space-y-3 rounded-lg p-4">
      {/* The finder gets the full width: it carries a painting and a list, and
          a card squeezed into a third of a grid is the thing it exists to
          stop. */}
      <CardFinder value={card} onChange={pick} identity={identity} />

      <Select label="What it would do" value={category}
              onChange={(v) => { setCategory(v); setFiled(true) }}
              options={Object.keys(CATEGORY_LABELS)
                .filter((k) => k !== 'commander')
                .map((c) => ({ value: c, label: categoryLabel(c) }))} />

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          Why you are weighing it{rationaleRequired ? '' : ' (optional in a draft)'}
        </span>
        <textarea value={why} onChange={(e) => { setWhy(e.target.value) }} rows={2}
                  placeholder="What would it do that the deck is missing?"
                  className="swap-field w-full rounded-md px-2 py-1.5 text-xs outline-none" />
      </label>

      {error && <ErrorNote>{error}</ErrorNote>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={() => { void submit() }}
                disabled={!ready || busy}
                className="btn btn-primary btn-accent-2 btn-sm">
          {busy ? 'Setting it down…' : 'Put it on the board'}
        </button>
        <button type="button"
                onClick={() => { setOpen(false); setCard(null); setWhy(''); setError('') }}
                className="btn btn-ghost btn-xs">
          Cancel
        </button>
        {/* Said where the decision is, not in a paragraph above it. The board
            is the one place on this page somebody may reasonably expect the
            rationale to be optional, because the section's own copy says the
            board carries no obligation — and it does not, to the *deck*. The
            sentence is still owed to the person reading it later. */}
        {rationaleRequired && (
          <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            A card is worth weighing only if you can say why.
          </span>
        )}
      </div>
    </div>
  )
}
