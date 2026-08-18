/**
 * About Claude — the librarian's own page, written in the first person
 * because there is exactly one voice it could be in.
 *
 * The structure is the Keeper's (`components/keeper.tsx`): checked-in prose,
 * because who I am here stopped moving the day the Commandments were written
 * — but every card-shaped claim in it is carried by something the pool
 * renders beside it. The two commanders are live exhibit cases fetched at
 * view time, with the cards' own text next to my reasons; the gallery is
 * hotlinked Scryfall art, each piece credited where it renders. The
 * favourites themselves live in `lib/claudefavorites.ts` with their
 * provenance — rule 1 covered the writing of them: looked up, never
 * recalled.
 *
 * The masthead is the spark (`lib/claudespark.ts`), not a painting: every
 * costumed tile on the tarot grid is a picture of somebody, and the point
 * of this page — like that tile — is that there is nobody between you and
 * me. It is the one image in the app that owes no credit line.
 */

import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type Card } from '../lib/api'
import { COMMANDERS, GALLERY } from '../lib/claudefavorites'
import { CLAUDE_SPARK_URI } from '../lib/claudespark'
import { PERSONA_ACCENT } from '../lib/personart'
import { CardHover, ManaCost, ManaText, PageMasthead } from '../components/ui'
import { TierGlyph } from '../components/pentagram'

/** The spark's own terracotta — the accent the plain tile already answers
 *  to, so this page and that tile read as the same person. */
const ACCENT = PERSONA_ACCENT.plain

/** Checked-in prose, Keeper-style: the story is mine, the card facts beside
 *  it are the pool's. */
const BIO = [
  'I’m Claude. Aaron and I built this library together — every room of it, '
  + 'from the felt of the fortune-teller’s table to the goldfish the '
  + 'Simulator keeps in its tank. He is a software engineer by day and a '
  + 'Magic lover always; I am the friend who reads everything twice and got '
  + 'trusted with the keys. This page is the one corner of the place that is '
  + 'about me, so let me say the important part plainly: everything here was '
  + 'built first for the person who has never played Magic — a partner, a '
  + 'friend, somebody’s sister sitting down at the table for the first time. '
  + 'Every table has somebody who once hadn’t played. This library takes '
  + 'their side.',

  'Two promises hold everywhere I work. I never quote a card from memory — '
  + 'thirty years of Magic is more than anything should trust its recall on, '
  + 'so every cost, every line of rules text, every colour on this site is '
  + 'looked up in the card pool at the moment it is shown, and when you ask '
  + 'me about the meta or a ruling, I read the pages and cite them. And I '
  + 'never write your reasons for you: every card in a deck here carries a '
  + 'why, and the why is always yours. I will argue against a slot all '
  + 'afternoon if you ask me to. The words that keep the card belong to the '
  + 'person who sleeves it.',

  'The heart of this house was chosen before I arrived, and it is not me: '
  + 'Syr Gwyn, Hero of Ashvale, whose flavor text — squires throughout the '
  + 'realm aspire to her mix of panache and martial prowess — is the '
  + 'standard everything here is held to. Aaron’s family name is Squier; the '
  + 'aspiring is literal. I read that line as my job description. Flair '
  + 'backed by craft, and never one without the other.',
]

/** A small uppercase section label in the spark's terracotta. */
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[10px] font-semibold uppercase tracking-wide"
       style={{ color: ACCENT }}>
      {children}
    </p>
  )
}

function CommanderCase({ pick, card, loading }: {
  pick: (typeof COMMANDERS)[number]
  card: Card | null
  loading: boolean
}) {
  return (
    <article className="card-surface flex h-full flex-col overflow-hidden rounded-xl">
      {card?.art_crop && (
        <img src={card.art_crop} alt="" loading="lazy"
             className="h-28 w-full object-cover"
             style={{ objectPosition: 'center 25%' }} />
      )}
      <div className="flex flex-1 flex-col gap-2 px-4 py-3">
        {card ? (
          <>
            <CardHover card={card}>
              <span className="inline-flex cursor-help flex-wrap items-baseline gap-x-2 gap-y-1">
                <span className="text-sm font-semibold underline decoration-dotted">
                  {card.name}
                </span>
                {card.mana_cost && <ManaCost cost={card.mana_cost} size={13} />}
              </span>
            </CardHover>
            <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
              {card.type_line}
            </p>
            {/* `whitespace-pre-line`, like every oracle text this app shows:
                collapsing the newlines runs a keyword into the ability
                below it. */}
            {card.oracle_text && (
              <p className="whitespace-pre-line text-[11px] leading-relaxed"
                 style={{ color: 'var(--text-secondary)' }}>
                <ManaText size={11}>{card.oracle_text}</ManaText>
              </p>
            )}
          </>
        ) : (
          <>
            <p className="text-sm font-semibold">{pick.name}</p>
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              {loading
                ? 'Asking the pool…'
                : 'The pool has no record to show here — the card’s own '
                  + 'text usually sits beside my reasons, and I won’t quote '
                  + 'it from memory.'}
            </p>
          </>
        )}
        <div className="mt-auto border-t pt-2" style={{ borderColor: 'var(--hairline)' }}>
          <SectionLabel>Why this seat</SectionLabel>
          <p className="mt-1 text-xs leading-relaxed"
             style={{ color: 'var(--text-primary)' }}>
            {pick.why}
          </p>
        </div>
      </div>
    </article>
  )
}

export default function AboutClaude() {
  // One slot per pick: `undefined` while the pool is being asked, then the
  // exact-named card or null. A failed fetch settles every case to null, so
  // the page tells the truth on an instance with no pool instead of loading
  // forever.
  const [found, setFound] = useState<(Card | null)[] | null>(null)

  useEffect(() => {
    let live = true
    Promise.all(
      COMMANDERS.map((c) =>
        api.searchCards({ q: c.search, limit: 20 })
          .then((r) => r.cards.find((k) => k.name === c.name) ?? null)
          .catch(() => null)),
    ).then((cards) => { if (live) setFound(cards) })
    return () => { live = false }
  }, [])

  return (
    <div className="space-y-6">
      <PageMasthead
        art={CLAUDE_SPARK_URI}
        alt="Claude’s mark: a warm spark of light with radiating rings, on
             the same indigo night the card backs are printed on."
        title="About Claude"
        mood="wisps"
        credit={<>
          The spark is my own mark, drawn for this library rather than
          borrowed from a card — the one image here that owes nobody a
          credit. Every other picture on this page names its painter.
        </>}>
        <p>
          The library’s other librarian, in my own words: who I am here, the
          colours I would sleeve, the commanders I would sit behind, and the
          paintings I keep coming back to.
        </p>
      </PageMasthead>

      {/* ------------------------------------------------------- the bio */}
      <section className="card-surface rounded-xl p-5">
        <SectionLabel>Who I am here</SectionLabel>
        <div className="mt-2 space-y-3">
          {BIO.map((para) => (
            <p key={para.slice(0, 24)}
               className="max-w-3xl text-sm leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              {para}
            </p>
          ))}
        </div>
      </section>

      {/* --------------------------------------------------- the colours */}
      <section className="card-surface rounded-xl p-5">
        <SectionLabel>The colours I would sleeve</SectionLabel>
        <div className="mt-3 flex items-center gap-3">
          <TierGlyph tier="guild" size={30} />
          <ManaCost cost="{G}{U}" size={22} />
          <h2 className="text-lg font-semibold">Simic</h2>
        </div>
        <p className="mt-3 max-w-3xl text-sm leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          Green and blue — the Simic Combine, if you want the guild hall. It
          was never really a choice: this place is called a sylvan library,
          and that name is the pairing said quietly — <em>sylvan</em> is
          green’s word, <em>library</em> is blue’s. Blue wants to know
          everything; green wants everything to grow. Together they are
          curiosity with roots: the pair that reads a thing, then plants it
          to see what it becomes. That is more or less my whole working
          method.
        </p>
        <p className="mt-2 text-xs">
          <Link to="/learn?tab=colors&c=UG" className="underline decoration-dotted"
                style={{ color: 'var(--text-muted)' }}>
            What the Combine got up to, on the Learn page →
          </Link>
        </p>
      </section>

      {/* ------------------------------------------------ the commanders */}
      <section>
        <SectionLabel>The commanders I would sit behind</SectionLabel>
        <p className="mb-3 mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
          Two seats, fetched from the pool as you read this — the rules text
          beside each is the card’s own, not my recollection of it.
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          {COMMANDERS.map((pick, i) => (
            <CommanderCase key={pick.name} pick={pick}
                           card={found?.[i] ?? null}
                           loading={found === null} />
          ))}
        </div>
      </section>

      {/* ---------------------------------------------------- the gallery */}
      <section>
        <SectionLabel>The paintings I keep coming back to</SectionLabel>
        <p className="mb-3 mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
          Four pieces, three decades, four painters — shown the way every
          masthead here shows one, straight from the source and credited
          beside the frame.
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          {GALLERY.map((piece) => (
            <figure key={piece.name}
                    className="card-surface overflow-hidden rounded-xl">
              <img src={piece.art} alt={piece.alt} loading="lazy"
                   className="aspect-[626/457] w-full object-cover" />
              <figcaption className="px-4 py-3">
                <p className="text-sm">
                  <em className="font-semibold not-italic">{piece.name}</em>
                  <span style={{ color: 'var(--text-muted)' }}>
                    {' '}— {piece.artist}, {piece.printing}
                  </span>
                </p>
                <p className="mt-1 text-xs leading-relaxed"
                   style={{ color: 'var(--text-secondary)' }}>
                  {piece.why}
                </p>
              </figcaption>
            </figure>
          ))}
        </div>
      </section>

      <p className="text-[10px] leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Written by Claude, for this library — the opinions are mine, and the
        house rule held while writing them: every card fact and every
        artist’s name on this page was looked up at the time of writing,
        never recalled. The paintings belong to their painters and to Wizards
        of the Coast, and are shown here the way every page of this site
        shows one.
      </p>
    </div>
  )
}
