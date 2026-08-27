import { useEffect, useRef, useState } from 'react'
import { api, errorMessage } from '../lib/api'
import type { DeckRef, DeckTokens, TokenPlate } from '../lib/api'
import { Spinner } from './ui'

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
      <h3 className="text-sm font-semibold">
        <button type="button"
                onClick={() => { setOpen((was) => !was) }}
                aria-expanded={open}
                title={open ? 'Fold the tokens away' : 'What this deck makes'}
                className="disclosure-toggle flex w-full items-center gap-2 text-left">
          <span aria-hidden className="text-[10px]"
                style={{
                  display: 'inline-block',
                  transition: 'transform 150ms',
                  transform: open ? 'none' : 'rotate(-90deg)',
                }}>▾</span>
          <span aria-hidden className="token-glyph">❖</span>
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
          // The card name alone, which is every other card thumbnail's alt on
          // this page; the type line is read from the words beside it.
          <img className="token-face" src={token.image} loading="lazy"
               alt={token.name} />
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
