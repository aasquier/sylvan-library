import { useEffect, useMemo, useRef, useState } from 'react'
import { api, errorMessage } from '../lib/api'
import type { DeckRef, DeckTokens, TokenPlate } from '../lib/api'
import { shoppingList, shoppingText } from '../lib/tokenshop'
import { FieldHint } from './hint'
import { CardHover, Spinner } from './ui'

/**
 * What a deck makes: the tokens its 99 and its commanders put onto the
 * battlefield.
 *
 * **Collapsed by default, and at the very foot of the page** (Aaron,
 * 2026-08-27: "another section that is collapsed by default ... on the very
 * bottom with sideboard when we get that far"). It is a *section* rather than
 * one of the two toys above it, so it takes the page's section idiom — the
 * `.disclosure-toggle` header the 99's own categories fold with — and not the
 * card-shaped fold the Wheel and the deal use. Those two are dessert; this is
 * part of the deck's own paperwork, and it will have the sideboard beside it.
 *
 * **Nothing is asked for until it is opened.** Folded, this component has made
 * no request and is loading no pictures — the Wheel's and the deal's rule, for
 * the Wheel's and the deal's reason: a thing that costs a click to open should
 * cost nothing at all to ignore. The fold does not survive a reload either,
 * and deliberately: "collapsed by default" is what was asked for, and a
 * remembered fold is a different feature.
 *
 * **The three empty states are three different sentences**, because a reader
 * lands somewhere different from each. No pool means nothing can be looked up;
 * a pool that predates the reading means nobody has looked *yet*; and a read
 * pool with nothing in it means this deck genuinely makes nothing. Collapsing
 * the middle one into the last would tell somebody their deck makes no tokens
 * when it may make a dozen, which is the one failure this section must not
 * have.
 *
 * **And one thing to do about it**: [TokenShopping] turns the shelf into a
 * list somebody can take to a shop (Aaron, 2026-08-29). It is the only action
 * in this section — everything else here is a picture — which is why it sits
 * directly under the paragraph that says to have a few to hand rather than
 * below a grid that can run to two dozen plates.
 */
export function TokenShelf({ deckRef }: { deckRef: DeckRef }) {
  const [open, setOpen] = useState(false)
  const [sheet, setSheet] = useState<DeckTokens | null>(null)
  const [error, setError] = useState('')
  // Keyed on the whole address, because two owners may both have a `goreclaw`
  // and a component that only watched the slug would show one deck's tokens
  // under the other's name.
  const address = `${deckRef.owner}/${deckRef.slug}`
  const asked = useRef('')

  useEffect(() => {
    if (!open || asked.current === address) return
    asked.current = address
    setSheet(null)
    setError('')
    api.deckTokens(deckRef)
      .then((made) => { setSheet(made) })
      .catch((e: unknown) => { setError(errorMessage(e)) })
  }, [open, address, deckRef])

  const tokens = sheet?.tokens ?? []
  const count = sheet && sheet.read ? tokens.length : null

  return (
    <section className="space-y-2 border-t pt-4"
             style={{ borderColor: 'var(--hairline)' }}>
      {/* **The brass mark moved out of the button and became the thing that
          explains the section.** It used to sit inside the toggle as pure
          decoration while a `title` carried the only words the header had —
          and a `title` draws on hover and on nothing else: never on a phone,
          never on keyboard focus (`components/hint.tsx` carries the full
          argument, and this room has now found it out four times). Aaron's
          only test surface today is a phone, so the sentence a folded section
          shows was reaching precisely nobody who needed it.

          The other half of what that `title` said — "Fold the tokens away" —
          is not re-homed anywhere, because `aria-expanded` already says it to
          every hand and said it better the whole time.

          No new furniture: the header still carries a chevron, a mark and a
          count. One of them answers now. */}
      <h3 className="flex items-center gap-1 text-sm font-semibold">
        {/* Commandment 2, at the one moment it is hardest to serve: folded,
            this section is the word "Tokens" and nothing else, and somebody
            meeting Magic this week has no idea whether that is a thing they
            need. The paragraph inside says it at length once the fold opens;
            this says it before.

            **Beside the word rather than at the end of the row**, which is
            where it was first put and where it read as a brass fleck 1,200
            pixels from anything it was about. A mark that explains a heading
            has to be next to the heading; the toggle keeps the rest of the
            row, so nothing is taken away from the fold's own target.

            Named for the question rather than for the section, because the
            toggle beside it is already called Tokens, and two controls in one
            heading answering to the same name is a reader being asked to
            guess which is which. */}
        <FieldHint name="What a token is" className="token-ask"
                   says={'The creatures and other pieces this deck makes '
                         + 'while you play. They are not cards in the deck '
                         + 'itself.'}>
          <span aria-hidden className="token-glyph">❖</span>
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
          <span style={{ color: 'var(--text-primary)' }}>Tokens</span>
          {count !== null && (
            <span className="tabular text-xs font-normal"
                  style={{ color: 'var(--text-muted)' }}>
              {count}
            </span>
          )}
        </button>
      </h3>

      {open && (
        <div className="space-y-3">
          {/* Commandment 2: somebody meeting Magic this week does not know
              what a token is, and a section headed Tokens that assumes they
              do has shut them out. One sentence, no lecture — what they are,
              why they are not in the list above, and the practical thing to
              do about it. */}
          <p className="max-w-2xl text-xs leading-relaxed"
             style={{ color: 'var(--text-muted)' }}>
            These are made during the game rather than drawn from the deck, so
            they never sit in the list above. Worth having a few to hand before
            you sit down.
          </p>

          {!sheet && !error && <Spinner label="Reading what this deck makes…" />}

          {error && (
            <p className="text-xs" style={{ color: 'var(--status-critical)' }}>
              {error}
            </p>
          )}

          {sheet && !sheet.pool_available && (
            <p className="text-xs" style={{ color: 'var(--status-warning)' }}>
              No card pool — the tokens cannot be looked up.
            </p>
          )}

          {sheet && sheet.pool_available && !sheet.read && (
            <p className="text-xs leading-relaxed"
               style={{ color: 'var(--status-warning)' }}>
              This card pool was gathered before tokens were catalogued, so it
              cannot say yet. It fills in the next time the pool is brought up
              to date.
            </p>
          )}

          {sheet?.read && tokens.length === 0 && (
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              Nothing in this deck makes a token — every permanent you play
              comes off the top.
            </p>
          )}

          {/* **Above the plates, not below them.** The paragraph directly
              overhead says "worth having a few to hand before you sit down",
              and this is the answer to that sentence — so it stands next to
              it rather than twenty-four plates further down a phone. It is
              also the only thing in this section that is an *action*, and an
              action a reader has to scroll to find is an action nobody
              finds.

              **Absent, never disabled, when there is nothing to buy.** The
              same `tokens.length > 0` that draws the shelf: a deck that makes
              nothing has already been told so in its own sentence above, and
              a greyed-out Copy button beside that sentence would be a
              second, worse way of saying it. The three declining states
              never reach here at all. */}
          {tokens.length > 0 && <TokenShopping tokens={tokens} />}

          {tokens.length > 0 && (
            <ul className="token-grid">
              {tokens.map((token, i) => (
                // Keyed on the picture as well as the words, because two
                // tokens can share both: "Spirit" / "Token Creature — Spirit"
                // is a dozen different bodies, and the printing is what tells
                // them apart.
                <TokenPlateCard
                  key={`${token.name}|${token.type_line}|${token.image ?? ''}`}
                  token={token} rank={i} />
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  )
}

/** Plain English for a count, because "1 tokens" is how a page tells somebody
 *  it was written by nobody. */
function many(n: number, one: string, more: string): string {
  return `${String(n)} ${n === 1 ? one : more}`
}

/**
 * The shopping list, and the one press that puts it on the clipboard.
 *
 * Aaron, 2026-08-29: *"I want there to be an 'Export' button in the token
 * area that can give you a TCGPlayer friendly list to help shop for tokens
 * when needed."*
 *
 * **One press does the thing and shows its work.** The list is copied *and*
 * revealed by the same press, which is the shape this needs for two separate
 * reasons. A copy that succeeds silently is a copy that gets pressed four
 * times — the button says "Copied" and the sentence under it says how much
 * went across, so the press is answered twice over. And a clipboard that
 * *refuses* — an old browser, a page without permission — leaves the text
 * sitting on screen with a sentence saying to select it, rather than leaving
 * a reader pressing a button that has quietly done nothing. A `catch` that
 * turned into "nothing happened" would be this repo's most-made mistake
 * wearing a shopping list.
 *
 * **No file, and no automation past the paste.** A download is worse than a
 * clipboard here — a phone has nowhere to put a `.txt` — and rule 9's "price
 * and cart, never checkout" is the ceiling: this hands somebody a list and
 * points at the box it goes in. Nothing here buys anything.
 */
function TokenShopping({ tokens }: { tokens: TokenPlate[] }) {
  const [shown, setShown] = useState(false)
  const [took, setTook] = useState(false)
  const [refused, setRefused] = useState(false)
  const lines = useMemo(() => shoppingList(tokens), [tokens])
  const text = useMemo(() => shoppingText(lines), [lines])
  const cards = lines.reduce((sum, line) => sum + line.qty, 0)

  // The label goes back to what it says at rest, so the *next* press is
  // answered too. Cleared on unmount, because a fold that closes mid-beat
  // would otherwise set state on a component that has gone.
  useEffect(() => {
    if (!took) return
    const beat = window.setTimeout(() => { setTook(false) }, 1800)
    return () => { window.clearTimeout(beat) }
  }, [took])

  async function take() {
    setShown(true)
    try {
      await navigator.clipboard.writeText(text)
      setRefused(false)
      setTook(true)
    } catch {
      // Not an error box: nothing is broken and nothing was lost. The list is
      // right there, and the sentence below says to take it by hand.
      setTook(false)
      setRefused(true)
    }
  }

  return (
    <div className="token-shop">
      <button type="button" className="btn btn-brass btn-sm"
              onClick={() => { void take() }}>
        <span aria-hidden className="token-shop-glyph">❖</span>
        {took ? 'Copied' : 'Copy shopping list'}
      </button>

      {/* Before the press, the button has to say what it is for — "shopping
          list" alone does not tell somebody who has never bought a single
          card what they would do with one (commandment 2). */}
      {!shown && (
        <p className="token-shop-note">
          A list to paste into a shop's bulk-add box, for the tokens this deck
          needs.
        </p>
      )}

      {shown && (
        <>
          {/* `role="status"` so the press is answered to a screen reader as
              well as to an eye — the same press, the same sentence. */}
          <p role="status"
             className={refused ? 'token-shop-said is-refused' : 'token-shop-said'}>
            {refused
              ? 'The clipboard would not take it — the list is below, ready to '
                + 'select.'
              : `Copied — ${many(lines.length, 'token', 'tokens')} to look for, `
                + `${many(cards, 'card', 'cards')} in all.`}
          </p>

          {/* The exact text that went across, so nobody pastes a surprise. */}
          <pre className="token-shop-list">{text}</pre>

          <p className="token-shop-note">
            The number is one for every card in your deck that makes it, up to
            four — a starting pile rather than a count. The same token can be
            made a dozen times in a game and still never need more than a few
            on the table at once, and a coin stands in perfectly well until the
            real ones arrive.
          </p>

          <p className="token-shop-note">
            Each line is the name the card is sold under — a Beast token is
            listed as <em>Beast Token</em> — so the whole list can go into a
            bulk-add box at once. Have a look at what it found before you buy
            anything. One such box is at{' '}
            <a href="https://www.tcgplayer.com/massentry"
               target="_blank" rel="noreferrer noopener"
               className="token-shop-link">tcgplayer.com/massentry</a>.
          </p>
        </>
      )}
    </div>
  )
}

/**
 * One token, as a card lying on a table.
 *
 * The whole face rather than an art crop: a token is a thing you *put down*,
 * and the face is what that looks like. The picture is hotlinked from the
 * pool, never committed (ADR 6, ADR 29).
 *
 * **A token with no printing here still gets a plate.** Every library is
 * missing something, and the honest fallback is the one players already use
 * at a real table — a blank card with the name written on it. That is a
 * likeness of the game rather than a hole in the page, and it is why the
 * missing case is a *style* here and not a `null`.
 *
 * **The face is held up on a hover or a tap** (Aaron, 2026-08-28: *"it would
 * be nice if a hover in our token menu for a deck gave a card preview"*).
 * `CardHover` is the mechanism the other twenty-three card thumbnails in this
 * app already use, and it is not a hover: a mouse gets the picture beside the
 * cursor, a thumb gets it centred over a dimmed room. That second half is the
 * one that matters here, because a token face is drawn at 5.75rem and the
 * only text this plate carries is the name, the type and who makes it —
 * everything a token actually *does* is printed on the card, at a size a
 * phone cannot read.
 *
 * **The blank plate is not wrapped, and that is the point of it.** There is
 * no painting behind it to enlarge, so a preview would open on the same
 * dashed rectangle at four times the size — a joke where a plate had been
 * honest. It is `CardSheet`'s own rule one surface over: no painting opens no
 * sheet. `CardHover` would in fact refuse on its own (every path in it tests
 * `card.image` first), so this is the same answer said where a reader can
 * see it rather than left to a guard two files away.
 */
function TokenPlateCard({ token, rank }: { token: TokenPlate; rank: number }) {
  return (
    <li className="token-plate"
        // Staggered so the row deals rather than appearing (commandment 6);
        // capped, because a deck with twenty tokens must not make anybody
        // wait for the last one. The whole animation is off under
        // prefers-reduced-motion, in the stylesheet.
        style={{ animationDelay: `${String(Math.min(rank, 8) * 45)}ms` }}>
      {token.image
        ? (
          // `contents`, so the wrapper generates no box at all: `.token-face`
          // stays the plate's own flex item, at the width and the `flex: 0 0
          // auto` the stylesheet gave it. A plain `span` would become the
          // flex item instead, inherit `flex: 0 1 auto`, and start shrinking
          // the face on a narrow phone — a layout change nothing in the web
          // suite can see, because jsdom lays nothing out.
          //
          // `tapOpens` is left at its default `true`. The call sites that
          // pass `false` wrap a real control whose tap is already spoken for
          // — the commander picker, the reading room's tiles — and a plate is
          // inert: nothing happens when it is tapped today, so the tap is
          // free and it is the only way a thumb reaches the picture.
          <CardHover className="contents"
                     card={{ name: token.name, image: token.image }}>
            {/* The card name alone, which is every other card thumbnail's alt
                on this page; the type line is read from the words beside it. */}
            <img className="token-face" src={token.image} loading="lazy"
                 alt={token.name} />
          </CardHover>
          )
        : (
          <div className="token-face token-face-blank" aria-hidden>
            <span className="token-blank-name">{token.name}</span>
          </div>
          )}
      <div className="token-plate-body">
        <p className="token-name">{token.name}</p>
        <p className="token-type">{token.type_line}</p>
        <p className="token-makers">
          <span className="token-makers-label">Made by </span>
          {token.made_by.join(', ')}
        </p>
        {token.artist && (
          // Rule 9: somebody painted it, and every other surface here says so.
          <p className="token-credit">
            <em>{token.name}</em> by {token.artist}
            {token.set_name ? `, ${token.set_name}` : ''}
          </p>
        )}
      </div>
    </li>
  )
}
