/**
 * The grid of readers: who do you want across the table?
 *
 * The first screen of the most expensive door in the app, and it costs
 * nothing — `/api/claude/personas` is a checked-in table that needs no key, no
 * card pool and no network, so the tiles are up before anybody has committed
 * to spending anything. **The roster is fetched, not written here** (ADR 21):
 * a voice added server-side arrives with its own label and blurb and renders
 * from them alone, which is what `routes/NewDeck.test.tsx`'s third, unknown
 * voice is really asserting.
 *
 * The paintings live in `lib/personart.ts` rather than beside the tiles,
 * because the interview's rooms need them too and `tarot.tsx` already imports
 * `theme.tsx` — a table in either component would be a cycle. `plain` is
 * deliberately absent from that table: it is the tile with no costume, and a
 * borrowed painting would make it one of eight characters rather than the exit
 * from character. It is not artless, though — it wears `ClaudeMark` below,
 * drawn rather than painted, for the same reason the card back and the
 * library's tree are drawn.
 *
 * Lifted out of `components/tarot.tsx` whole (rooms foundation, 2026-09-12).
 * The table is a ceremony with a phase machine in it and the grid is a list of
 * tiles; the only thing they shared was a file.
 */

import { useId } from 'react'
import { dealsTarot, type Persona } from '../lib/api'
import { personaArt } from '../lib/personart'
import { CardArt } from './ui'

/**
 * Claude's own tile art: a spark of warm light on the same night the card
 * backs are printed on.
 *
 * Chosen by Claude, since the tile is Claude (item 1 asked). Not a figure —
 * every costumed tile is a painting of somebody, and the point of this one
 * is that there is nobody between you and the conversation. A light source
 * works where a portrait would lie: warm against the indigo, radiating, with
 * the concentric rings a voice makes. The palette deliberately shares the
 * card back's night so the grid reads as one table, and the spark is the
 * warm terracotta none of the paintings use, so the one drawn tile still
 * reads as its own kind of thing.
 */
function ClaudeMark() {
  const id = useId().replace(/:/g, '')
  return (
    <svg viewBox="0 0 626 457" className="h-full w-full" aria-hidden="true"
         preserveAspectRatio="xMidYMid slice">
      <defs>
        <radialGradient id={`${id}-night`} cx="50%" cy="42%" r="75%">
          <stop offset="0%" stopColor="#2c3f6b" />
          <stop offset="60%" stopColor="#1b2647" />
          <stop offset="100%" stopColor="#101830" />
        </radialGradient>
        <radialGradient id={`${id}-halo`} cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="rgba(255,236,210,0.95)" />
          <stop offset="30%" stopColor="rgba(240,166,122,0.55)" />
          <stop offset="70%" stopColor="rgba(218,119,86,0.18)" />
          <stop offset="100%" stopColor="rgba(218,119,86,0)" />
        </radialGradient>
      </defs>
      <rect width="626" height="457" fill={`url(#${id}-night)`} />
      {/* A scatter of far stars, the card back's sky continued. */}
      <g fill="#f0e4c2">
        {[[62, 70, 1.6], [140, 330, 1.2], [210, 96, 1.1], [318, 40, 1.4],
          [430, 88, 1.2], [538, 150, 1.6], [566, 330, 1.1], [468, 396, 1.3],
          [96, 210, 1.0], [246, 402, 1.2], [388, 372, 1.0], [520, 244, 1.0],
        ].map(([x, y, r]) => (
          <circle key={`${x}-${y}`} cx={x} cy={y} r={r} opacity="0.55" />
        ))}
      </g>
      {/* The rings a voice makes: concentric, fading as they travel. */}
      {[74, 118, 166, 220].map((r, i) => (
        <circle key={r} cx="313" cy="222" r={r} fill="none"
                stroke="#f0a67a" strokeWidth={1.6 - i * 0.3}
                opacity={0.34 - i * 0.07} />
      ))}
      <circle cx="313" cy="222" r="150" fill={`url(#${id}-halo)`} />
      {/* The spark: rays alternating long and short, warm on the night. */}
      <g transform="translate(313 222)">
        {Array.from({ length: 12 }, (_, i) => {
          const a = (i * Math.PI * 2) / 12 - Math.PI / 2
          const inner = 26
          const outer = i % 2 === 0 ? 74 : 50
          return (
            <line key={i}
                  x1={Math.cos(a) * inner} y1={Math.sin(a) * inner}
                  x2={Math.cos(a) * outer} y2={Math.sin(a) * outer}
                  stroke={i % 2 === 0 ? '#f7d9b8' : '#e8956d'}
                  strokeWidth={i % 2 === 0 ? 7 : 4.5}
                  strokeLinecap="round" opacity="0.92" />
          )
        })}
        <circle r="19" fill="#fbe8d0" />
        <circle r="9" fill="#fff6ea" />
      </g>
    </svg>
  )
}

function ReaderPanel({ persona, onPick }: {
  persona: Persona
  onPick: () => void
}) {
  const art = personaArt(persona.key)
  const deals = dealsTarot(persona)
  return (
    // No `art-fade` on the wrapper: that class belongs to the `<img>` inside
    // `CardArt`, which is the element that gets `.loaded` back. On the wrapper
    // it is an opacity-0 that nothing ever lifts, and every tile shipped as a
    // black rectangle with a caption — the exact bug the punch list reported.
    <button onClick={onPick}
            className="reader-tile card-surface flex flex-col overflow-hidden rounded-xl text-center">
      {art && (
        <CardArt src={art.art} alt="" ratio="aspect-[626/457]" eager
                 className="reader-tile-art w-full rounded-none" />
      )}
      {/* The tile with no costume wears Claude's own mark — drawn, so it
          needs no credit line and owes nobody a licence (item 1). */}
      {!art && persona.key === 'plain' && (
        <span className="reader-tile-art block w-full aspect-[626/457]"
              aria-hidden="true">
          <ClaudeMark />
        </span>
      )}
      <span className="flex flex-1 flex-col items-center gap-2 px-5 py-4">
        <span className="text-base font-medium">{persona.label}</span>
        <span className="text-xs leading-relaxed"
              style={{ color: 'var(--text-secondary)' }}>
          {persona.blurb}
        </span>
        {/* Only the reader who deals gets a footer. Every other tile reciting
            "No cards — just the questions" said the same nothing once per
            tile; the blurb already says who each voice is, and the one fact
            worth a line of its own is that this table has cards on it. */}
        {deals && (
          <span className="mt-auto text-[11px] uppercase tracking-wide"
                style={{ color: 'var(--series-1)' }}>
            ✦ Three cards, dealt for you
          </span>
        )}
        {art && (
          <span className={`text-[10px]${deals ? '' : ' mt-auto'}`}
                style={{ color: 'var(--text-muted)' }}>
            Art by {art.credit}
          </span>
        )}
      </span>
    </button>
  )
}

/** Every voice, as a tile. One column on a phone, two on a tablet, three on a
 *  desk — the roster is meant to grow and the grid takes what it is given. */
export function PersonaGrid({ personas, onPick }: {
  personas: Persona[]
  onPick: (persona: Persona) => void
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {personas.map((p) => (
        <ReaderPanel key={p.key} persona={p} onPick={() => onPick(p)} />
      ))}
    </div>
  )
}
