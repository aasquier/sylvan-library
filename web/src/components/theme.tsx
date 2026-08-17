/**
 * The theme interview — the third door into the create flow (ADR 20).
 *
 * The other two doors both open onto the same question: which of the 32 colour
 * combinations do you want? Somebody who has never played cannot answer that,
 * so this one asks about *them* instead — a film, a period, their sign, how
 * they are at game night — and translates. That works because the colour pie
 * is a personality taxonomy before it is a set of mechanics.
 *
 * Three things this component renders that are not decoration:
 *
 * **Every reading shows the words it rests on.** A slot chip carries the
 * user's own quote, because the server threw away any reading it could not
 * find in the transcript and the point of that check is lost if the result is
 * invisible. Somebody should be able to see the interview being held to what
 * they actually said.
 *
 * **A reading and a fact are styled differently and labelled differently.**
 * "Dune sounds like Golgari to me" is an interpretation and cannot be wrong;
 * "Golgari is Ravnica's guild of death and rebirth" is a claim and can be.
 * Merging them into one confident paragraph is the failure ADR 19 named, and
 * the separation survives all the way to here or it did not happen.
 *
 * **The proposal is a proposal.** Picking a commander fills in the create form
 * that already exists — it does not make a deck. Nothing under
 * `src/mtglab/claude/` can reach a write path, and this is the UI telling the
 * same truth: the deck is made by the person whose deck it is.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ApiError,
  api,
  followJob,
  type ClaudeStatus,
  type ThemeCombination,
  type ThemeCommander,
  type ThemeFact,
  type ThemeProposal,
  type ThemeReport,
  type ThemeSlot,
  type ThemeTurn,
} from '../lib/api'
import { COLOR_VAR } from '../lib/mtg'
import { PERSONA_ACCENT, PERSONA_ART } from '../lib/personart'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'
import { SceneBackdrop } from './forest'
import { ArmedButton, CardHover, ColorRing } from './ui'
import { StanceReadout } from './stance'

/** Held here so a closed tab does not cost ten minutes of somebody's thinking.
 *  The server stores nothing (ADR 20), which is also why the most personal
 *  thing this app handles never reaches its disk. */
const SAVED = 'mtglab-theme-conversation'

const SLOT_LABELS: Record<string, string> = {
  taste: 'What you love',
  temperament: 'How you are',
  posture: 'At the table',
  anchor: 'Already a favourite',
}

interface Saved {
  transcript: ThemeTurn[]
  slots: ThemeSlot[]
  /** A proposal in flight, as a job id. Kept for the same reason the
   *  transcript is: the run takes minutes and costs real money, so a reload
   *  should reattach to the one already going rather than start a second. */
  job: string | null
  /** And the answer once it lands, which is the same argument one step on —
   *  four minutes of waiting should not be undone by a refresh. */
  proposal: ThemeProposal | null
  /** Every fun fact already shown, in the order it appeared. Resent with each
   *  turn so the server can quote the covered ground back to the model and
   *  drop a repeat — the transcript's trick, applied to the one output that
   *  never rides in the transcript. */
  facts: ThemeFact[]
  /** Who was speaking, and which three cards were on the table.
   *
   *  Stashed because **a persona is fixed for a conversation** (ADR 21): the
   *  transcript is resent whole every turn, so a voice swapped halfway leaves
   *  every earlier answer speaking in the old one. Restoring a conversation
   *  under a different reader is the same fault with a reload in the middle,
   *  so a stash whose persona or seed does not match what the door is offering
   *  is discarded rather than adopted. */
  persona: string
  seed: number | null
}

const EMPTY: Saved = {
  transcript: [], slots: [], job: null, proposal: null, facts: [],
  persona: 'plain', seed: null,
}

function load(persona: string, seed: number | null): Saved {
  const empty: Saved = { ...EMPTY, persona, seed }
  try {
    const raw = localStorage.getItem(SAVED)
    if (!raw) return empty
    const parsed = JSON.parse(raw) as Partial<Saved>
    // A conversation belongs to one reader and one spread. Anything else is
    // somebody else's conversation wearing this one's costume.
    const was = typeof parsed.persona === 'string' ? parsed.persona : 'plain'
    const dealt = typeof parsed.seed === 'number' ? parsed.seed : null
    if (was !== persona || dealt !== seed) return empty
    return {
      transcript: Array.isArray(parsed.transcript) ? parsed.transcript : [],
      slots: Array.isArray(parsed.slots) ? parsed.slots : [],
      job: typeof parsed.job === 'string' ? parsed.job : null,
      proposal: parsed.proposal ?? null,
      facts: Array.isArray(parsed.facts)
        ? parsed.facts.filter((f) => typeof f?.text === 'string')
        : [],
      persona, seed,
    }
  } catch {
    // A corrupted stash is not worth an error message. Start again.
    return empty
  }
}

/* ------------------------------------------------------------- the pieces */

/**
 * The voice's sign, hung by the door (punch list 2026-08-15 item 8): one
 * small drawn emblem per persona, animated with the laboratory's own
 * classes — a flame is a flame whether it is under a beaker or a story.
 * Hovering it stirs it (`--lab-speed` drops on `.room-sign:hover`), which
 * is the cheapest kind of interactive: the room notices you.
 *
 * Drawn in `currentColor` so each sign wears its room's accent, except
 * where a thing has an unarguable colour — foam is foam.
 */
function RoomSign({ persona }: { persona: string }) {
  const sign = (() => {
    switch (persona) {
      case 'therapist':
        // A crescent, and three dreams getting away.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            <path d="M40 38 A 15 15 0 1 1 40 10 A 12 12 0 1 0 40 38 Z"
                  fill="currentColor" opacity="0.8" />
            <circle className="lab-steam" cx="22" cy="34" r="3.4"
                    fill="currentColor" />
            <circle className="lab-steam lab-steam-2" cx="14" cy="30" r="2.6"
                    fill="currentColor" />
            <circle className="lab-steam" cx="29" cy="28" r="2"
                    fill="currentColor" style={{ animationDelay: '3.2s' }} />
          </svg>
        )
      case 'scientist':
        // The specimen, mid-observation.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            <rect x="27" y="4" width="10" height="40" rx="5"
                  fill="currentColor" opacity="0.25" />
            <rect x="27" y="22" width="10" height="22" rx="5"
                  fill="currentColor" opacity="0.75" />
            <circle className="lab-bubble" cx="30" cy="38" r="1.8" fill="#fff"
                    opacity="0.9" />
            <circle className="lab-bubble lab-bubble-3" cx="34" cy="40" r="1.3"
                    fill="#fff" opacity="0.9" />
            <path d="M24 8 H 40" stroke="currentColor" strokeWidth="2"
                  strokeLinecap="round" />
          </svg>
        )
      case 'chef':
        // The pot, and what escapes it.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            <path d="M14 26 H 50 V 34 A 10 10 0 0 1 40 44 H 24 A 10 10 0 0 1 14 34 Z"
                  fill="currentColor" opacity="0.85" />
            <path d="M12 26 H 52" stroke="currentColor" strokeWidth="3"
                  strokeLinecap="round" />
            <path d="M28 22 C 28 18 36 18 36 22" stroke="currentColor"
                  strokeWidth="2.5" fill="none" strokeLinecap="round" />
            <circle className="lab-steam" cx="24" cy="18" r="3" fill="#cfe4ea" />
            <circle className="lab-steam lab-steam-2" cx="34" cy="14" r="2.4"
                    fill="#cfe4ea" />
            <circle className="lab-steam" cx="42" cy="17" r="2"
                    fill="#cfe4ea" style={{ animationDelay: '2.4s' }} />
          </svg>
        )
      case 'storyteller':
        // The fire the tales are told across.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            <path d="M18 42 L 46 42 M 20 45 L 44 39" stroke="#8a6a33"
                  strokeWidth="3" strokeLinecap="round" />
            <path className="lab-flame"
                  d="M32 40 C 24 32 26 22 32 12 C 38 22 40 32 32 40 Z"
                  fill="currentColor" />
            <path className="lab-flame lab-flame-2"
                  d="M32 38 C 28 33 29 27 32 21 C 35 27 36 33 32 38 Z"
                  fill="#f6d98a" opacity="0.9" />
          </svg>
        )
      case 'barkeep':
        // The pour that settles while you decide what to admit.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            <path d="M22 10 H 42 L 40 44 H 24 Z" fill="currentColor"
                  opacity="0.8" />
            <path d="M42 16 C 50 16 50 30 42 30" stroke="currentColor"
                  strokeWidth="3" fill="none" />
            <rect x="23" y="10" width="18" height="6" rx="3" fill="#f0e4c2" />
            <circle className="lab-bubble" cx="28" cy="36" r="1.6" fill="#f0e4c2"
                    opacity="0.9" />
            <circle className="lab-bubble lab-bubble-2" cx="34" cy="38" r="1.2"
                    fill="#f0e4c2" opacity="0.9" />
          </svg>
        )
      case 'fortune-teller':
        // Three cards, already fanned. The table below deals the real ones.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            {[-14, 0, 14].map((angle) => (
              <rect key={angle} x="26" y="10" width="14" height="24" rx="2"
                    fill="currentColor" opacity="0.75"
                    transform={`rotate(${angle} 33 36)`} />
            ))}
            <circle className="lab-float" cx="33" cy="7" r="2"
                    fill="currentColor" />
          </svg>
        )
      default:
        // Claude's spark, small — the mark from the tile, breathing.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            <g className="lab-float">
              {Array.from({ length: 8 }, (_, i) => {
                const a = (i * Math.PI * 2) / 8
                return (
                  <line key={i}
                        x1={32 + Math.cos(a) * 8} y1={24 + Math.sin(a) * 8}
                        x2={32 + Math.cos(a) * (i % 2 === 0 ? 17 : 12)}
                        y2={24 + Math.sin(a) * (i % 2 === 0 ? 17 : 12)}
                        stroke="currentColor" strokeWidth={i % 2 === 0 ? 3 : 2}
                        strokeLinecap="round" />
                )
              })}
              <circle cx="32" cy="24" r="5.5" fill="currentColor" />
            </g>
          </svg>
        )
    }
  })()
  return <span className="room-sign shrink-0" aria-hidden="true">{sign}</span>
}

/** The hand's pace (the brief's front two, third pass). A fixed,
 *  comfortable rate — a long sentence takes longer, which is the honest
 *  arithmetic the old 8-second cap inverted (past ~308 characters it made
 *  the hand HURRY, faster the more it had to say). The floor stays so a
 *  two-word answer is not instant; the cap is gone, replaced by a skip —
 *  click the line and it finishes at once, because making the reader wait
 *  is a choice they should be able to decline without making the hand
 *  write like a machine. */
const INK_MS_PER_CHAR = 62
const INK_MIN_MS = 900
/** Where the pen pauses: lifted between words, longer at a breath, longer
 *  still at a full stop. This unevenness is most of what separates a hand
 *  from a metronome. */
const INK_PAUSE_WORD = 90
const INK_PAUSE_BREATH = 260   // , ; : and the em dash
const INK_PAUSE_STOP = 420     // . ? !

/**
 * A hand writing on the parchment (overhaul item 5, second pass — Aaron
 * asked for the hand, not just the ink; third pass — the brief asked for
 * the rhythm). The unit is the CHARACTER now, not the word: each glyph is
 * revealed by its own left-to-right wipe on the shared schedule, so the
 * line fills stroke by stroke instead of arriving as blocks. Words keep
 * their own spans around the characters — that is what lets the browser
 * wrap lines at spaces, and it still carries the per-word tilt and drop
 * (deterministic per index — a hand wobbles, a render must not). The ink
 * starts wet-brown and dries dark; the first character of each word comes
 * a touch slower than the run of the word, the way a pen resettles after
 * a lift. A drawn quill travels with the current character, and lifts off
 * when the sentence is done.
 *
 * Reduced motion gets the text already dry and no quill, from the same
 * media query that stills the table.
 */
function InkText({ text }: { text: string }) {
  const host = useRef<HTMLSpanElement>(null)
  const quill = useRef<SVGSVGElement>(null)
  // Clicking the line finishes it at once: every delay collapses and the
  // quill lifts. State rather than a class flip so React owns the DOM.
  const [skipped, setSkipped] = useState(false)

  // One schedule, used twice: inline delays for the CSS, and the quill's
  // itinerary in the effect below. Built per word, spent per character.
  const words = text.split(/\s+/).filter(Boolean)
  const chars = Math.max(words.reduce((n, w) => n + w.length, 0), 1)
  // The floor only ever slows a very short line down; nothing speeds up.
  const rate = Math.max(INK_MS_PER_CHAR, INK_MIN_MS / chars)
  let clock = 0
  const schedule = words.map((w) => {
    const start = clock
    const perChar = Array.from(w).map((_, i) =>
      i === 0 ? rate * 1.35 : rate * 0.95)
    const dur = perChar.reduce((a, b) => a + b, 0)
    clock += dur
    // The pen lifts between words; punctuation holds it in the air longer.
    clock += /[.?!]$/.test(w) ? INK_PAUSE_STOP
      : /[,;:]$/.test(w) || /[—–-]$/.test(w) ? INK_PAUSE_BREATH
        : INK_PAUSE_WORD
    return { start, dur, perChar }
  })
  const total = clock

  useEffect(() => {
    const el = host.current
    const pen = quill.current
    if (!el || !pen) return
    // Optional-chained twice for jsdom, which has no matchMedia at all.
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches) return
    if (skipped) {
      pen.style.opacity = '0'
      return
    }
    const spans = Array.from(el.querySelectorAll<HTMLElement>('.ink-char'))
    if (spans.length === 0) return
    // Each character knows its own start; the quill just asks which one is
    // being written and stands over it.
    const starts = spans.map((s) => Number(s.dataset.start ?? 0))
    pen.style.opacity = '1'
    const t0 = performance.now()
    // An interval rather than requestAnimationFrame, and not for style: rAF
    // is starved in throttled and headless tabs, which left the quill parked
    // on the second word while the ink ran ahead. 50ms is more than smooth
    // enough for a hand.
    const tick = window.setInterval(() => {
      const t = performance.now() - t0
      if (t >= total + 200) {
        pen.style.opacity = '0'
        window.clearInterval(tick)
        return
      }
      let i = starts.findIndex((s) => t < s)
      i = i === -1 ? spans.length - 1 : Math.max(i - 1, 0)
      const ch = spans[i]
      if (ch) {
        const box = ch.getBoundingClientRect()
        const home = el.getBoundingClientRect()
        // The nib rides the current glyph, a touch above the baseline.
        const x = box.left - home.left + box.width * 0.7
        const y = box.top - home.top
        pen.style.transform =
          `translate(${x}px, ${y - 14}px) rotate(${32 + Math.sin(t / 90) * 4}deg)`
      }
    }, 50)
    return () => window.clearInterval(tick)
    // The schedule is derived from `text` alone; keying on the text keeps
    // this honest without re-deriving arrays in the dependency list.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text, skipped])

  let word = 0
  return (
    <span ref={host} className={`ink-text${skipped ? ' is-dry' : ''}`}
          onClick={() => setSkipped(true)}
          title={skipped ? undefined : 'Click to let the ink dry at once'}>
      {text.split(/(\s+)/).map((part, i) => {
        if (/^\s*$/.test(part)) return part
        const n = word++
        const s = schedule[n]
        let charClock = s?.start ?? 0
        return (
          <span key={`${i}-${part}`} className="ink-word"
                style={{
                  // A hand wobbles; a render must not. Deterministic jitter.
                  '--ink-tilt': `${(((n * 7) % 5) - 2) * 0.35}deg`,
                  '--ink-drop': `${(((n * 3) % 3) - 1) * 0.6}px`,
                } as React.CSSProperties}>
            {Array.from(part).map((glyph, k) => {
              const start = charClock
              const dur = s?.perChar[k] ?? INK_MS_PER_CHAR
              charClock += dur
              return (
                <span key={k} className="ink-char" data-start={start}
                      style={{
                        '--ink-delay': `${start}ms`,
                        '--ink-dur': `${dur}ms`,
                      } as React.CSSProperties}>
                  {glyph}
                </span>
              )
            })}
          </span>
        )
      })}
      {/* The hand. A drawn quill — no asset, no licence — angled the way a
          right hand holds one, riding the schedule above. */}
      <svg ref={quill} className="ink-quill" viewBox="0 0 40 40"
           aria-hidden="true">
        <path d="M4 36 C 10 28 14 18 24 10 C 30 5 36 2 38 2
                 C 37 6 34 12 28 18 C 20 26 12 32 6 37 Z"
              fill="#5a4526" opacity="0.9" />
        <path d="M4 36 C 12 27 20 19 30 8" stroke="#2b2013"
              strokeWidth="1.1" fill="none" opacity="0.7" />
        <path d="M2 39 L 6 34" stroke="#2b2013" strokeWidth="1.6"
              strokeLinecap="round" />
      </svg>
    </span>
  )
}

function Chip({ slot }: { slot: ThemeSlot }) {
  return (
    <div className="rounded-lg px-3 py-2"
         style={{ border: '1px solid var(--hairline)', background: 'var(--page)' }}>
      <p className="text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        {SLOT_LABELS[slot.kind] ?? slot.kind}
      </p>
      <p className="text-sm" style={{ color: 'var(--text-primary)' }}>{slot.value}</p>
      {/* The check, made visible. The server dropped anything it could not
          find in the transcript, and showing the quote is how somebody can
          tell that this is a reading of them rather than about them. */}
      <p className="mt-0.5 text-[11px] italic" style={{ color: 'var(--text-muted)' }}>
        because you said “{slot.quote}”
      </p>
    </div>
  )
}

function FactNote({ fact, seance }: {
  fact: NonNullable<ThemeReport['fact']>
  seance?: boolean
}) {
  return (
    <aside className={`rounded-lg px-4 py-3${seance ? ' seance-note' : ''}`}
           style={seance ? undefined : { background: 'var(--gridline)' }}>
      <p className="text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        While you are here
      </p>
      <p className="mt-1 text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>{fact.text}</p>
      <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
        {fact.url
          ? <a href={fact.url} target="_blank" rel="noreferrer noopener"
               className="underline">{fact.source}</a>
          : 'From this tool’s own colour reference data.'}
      </p>
    </aside>
  )
}

function CommanderTile({ card, onPick }: {
  card: ThemeCommander
  onPick: () => void
}) {
  return (
    <CardHover card={card} className="block">
      <button onClick={onPick}
              className="card-surface block w-full overflow-hidden rounded-xl text-left transition hover:opacity-90">
        {card.art_crop && (
          <img src={card.art_crop} alt="" loading="lazy"
               className="h-20 w-full object-cover" />
        )}
        <div className="px-3 py-2">
          <p className="text-sm font-medium">{card.name}</p>
          <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            {card.type_line}
          </p>
          {card.prose && (
            <p className="mt-1 text-xs leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>{card.prose}</p>
          )}
        </div>
      </button>
    </CardHover>
  )
}

function CombinationPanel({ combo, rank, sources, onPick }: {
  combo: ThemeCombination
  rank: number
  sources: ThemeProposal['sources']
  onPick: (card: ThemeCommander) => void
}) {
  const cited = sources.filter((s) => combo.source_ids.includes(s.id))
  return (
    <article
      className="card-surface rounded-xl px-5 py-5"
      style={{
        backgroundImage: combo.colors.length
          ? `linear-gradient(135deg, ${combo.colors
              .map((c, i) => `color-mix(in srgb, ${COLOR_VAR[c]} 18%, transparent) ${
                (i / Math.max(combo.colors.length - 1, 1)) * 100}%`)
              .join(', ')})`
          : 'none',
      }}
    >
      <div className="flex flex-wrap items-center gap-3">
        <ColorRing colors={combo.colors} size={26} />
        <div>
          <h3 className="text-xl font-semibold tracking-tight">{combo.name}</h3>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {rank === 0 ? 'The one I would build' : 'Worth a look for contrast'}
            {' · '}{combo.tagline}
          </p>
        </div>
      </div>

      {/* Two paragraphs, two labels, and they are never merged. One of them
          can be wrong and the other cannot. */}
      {combo.reading && (
        <div className="mt-4 border-l-2 pl-3" style={{ borderColor: 'var(--series-1)' }}>
          <p className="text-[10px] uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
            Claude reading you — an interpretation, not a finding
          </p>
          <p className="mt-1 text-sm leading-relaxed"
             style={{ color: 'var(--text-primary)' }}>{combo.reading}</p>
        </div>
      )}

      {combo.grounding && (
        <div className="mt-3 border-l-2 pl-3" style={{ borderColor: 'var(--baseline)' }}>
          <p className="text-[10px] uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
            What is actually true about these colours
          </p>
          <p className="mt-1 text-sm leading-relaxed"
             style={{ color: 'var(--text-secondary)' }}>{combo.grounding}</p>
          {cited.length > 0 && (
            <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
              {cited.map((s, i) => (
                <span key={s.id}>
                  {i > 0 && ' · '}
                  <a href={s.url} target="_blank" rel="noreferrer noopener"
                     className="underline">{s.title}</a>
                </span>
              ))}
            </p>
          )}
        </div>
      )}

      <p className="mt-4 text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        Legends who lead exactly these colours
      </p>
      <div className="mt-2 grid gap-3 sm:grid-cols-3">
        {combo.commanders.map((c) => (
          <CommanderTile key={c.name} card={c} onPick={() => onPick(c)} />
        ))}
      </div>
    </article>
  )
}

/* --------------------------------------------------------------- the page */

export function ThemeInterview({
  onPick, onLeave, persona = 'plain', seed = null, intro, leaveLabel,
}: {
  onPick: (key: string, card: ThemeCommander) => void
  onLeave: () => void
  /** Which voice is asking (ADR 21). Fixed for the life of the conversation —
   *  the tarot door remounts this component when the reader changes, which is
   *  what makes "changing it restarts" a fact rather than a request. */
  persona?: string
  /** The spread's seed, when the reader was dealt one. Re-deals the identical
   *  three cards server-side, so the conversation and the table can never
   *  disagree about what is face up. */
  seed?: number | null
  /** The reader's own framing, when somebody else is setting the scene. */
  intro?: { title: string; blurb: string }
  leaveLabel?: string
}) {
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [saved, setSaved] = useState<Saved>(() => load(persona, seed))
  const { transcript, slots, proposal, facts } = saved
  const [report, setReport] = useState<ThemeReport | null>(null)
  const [answer, setAnswer] = useState('')
  const [budget, setBudget] = useState('')
  const [busy, setBusy] = useState<'' | 'asking' | 'proposing'>('')
  const [elapsed, setElapsed] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const box = useRef<HTMLTextAreaElement>(null)
  const [pin, setPin] = useStance()

  // No slug, because there is no deck yet — which is the whole point of this
  // mode. With neither a deck nor a pin `theme._stance` supplies its own
  // default (`SECOND_OPINION`, since a deck nobody has built is as theoretical
  // as a deck gets), so "follow the deck" is not an empty position here.
  useEffect(() => {
    fetchClaudeStatus({ surface: 'theme' }, pin, () => setPin(null))
      .then(setStatus).catch(() => setStatus(null))
  }, [pin, setPin])

  useEffect(() => {
    localStorage.setItem(SAVED, JSON.stringify(saved))
  }, [saved])

  // The turn is a background job too now, for the reason `api.themeAsk` gives:
  // measured at 4.3–37.7s with one at 133.8s, against a transport ceiling
  // nobody has measured and that is known only to be at or below 236s.
  //
  // Unlike the proposal the id is **not** saved. That one persists it because
  // a reload inside four minutes would otherwise pay twice; a turn is seconds,
  // and persisting it here would put a second claimant on the transcript
  // alongside the auto-ask effect below — two paths that could both decide
  // what the pending question is. A reload mid-turn re-asks, which is what it
  // did when this was a plain POST.
  const asker = useRef<{ cancel: () => void } | null>(null)

  // Stable, so the opening effect below can depend on it honestly rather than
  // being told to ignore it.
  const send = useCallback(async (next: ThemeTurn[], carried: ThemeSlot[],
                                  told: ThemeFact[]) => {
    setBusy('asking')
    setError(null)
    asker.current?.cancel()
    try {
      const job = await api.themeAsk({
        transcript: next, slots: carried,
        // The ground already covered, so the server can hold the model to
        // "never give the same fact twice" — see `Saved.facts`.
        facts: told.map((f) => f.text),
        persona, seed: seed ?? undefined, stance: effectivePin(pin, status),
      })
      // `initial` is what keeps the cheap case cheap: stance `off` and a
      // finished conversation come back already `done`, and this resolves with
      // no poll at all. 400ms otherwise — this is somebody waiting on a
      // question, not a four-minute proposal, so the 2s the proposal polls at
      // would be most of the latency on a fast turn.
      const run = followJob(job.id, () => {}, 400, job)
      asker.current = { cancel: run.cancel }
      const got = (await run.promise).result as ThemeReport
      setReport(got)
      setSaved((s) => ({
        ...s,
        transcript: got.question
          ? [...next, { role: 'assistant', text: got.question }]
          : next,
        slots: got.slots,
        // Each fact joins the covered-ground list the moment it is shown.
        // Deduplicated here too, because a retried turn must not count its
        // fact twice against the model.
        facts: got.fact && !s.facts.some((f) => f.text === got.fact?.text)
          ? [...s.facts, got.fact]
          : s.facts,
      }))
    } catch (e) {
      setError(e instanceof ApiError && e.status === 404
        // Same sentence the proposal shows, and the same cause: jobs live in
        // the server's memory and die with it (`api/jobs.py`).
        ? 'That question is gone — the server restarted while it was working. Say something and it will ask again.'
        : String((e as Error).message ?? e))
    } finally {
      setBusy('')
      box.current?.focus()
    }
  }, [persona, seed, pin, status])

  // Fetch a question whenever there isn't one pending. That covers the opening
  // turn, and it also covers the case a plain `length > 0` guard got wrong: a
  // conversation restored from a previous tab that ended on *your answer* has
  // an outstanding question nobody ever asked for, and would otherwise sit on
  // "Starting…" with no way forward but answering twice.
  //
  // The ref does the work the dependency array cannot: this must fire once per
  // pending question, and every value it reads changes as the conversation
  // runs. Re-running and returning immediately is cheap; suppressing the lint
  // and being wrong later is not.
  const awaited = useRef(-1)
  useEffect(() => {
    if (!status?.installed || !status.configured || busy) return
    // Not while a proposal is in flight either. The transcript almost always
    // ends on a question by then so this rarely fires, but a tab restored
    // mid-run can end on an answer, and asking a fresh question underneath a
    // running proposal is confusing rather than helpful.
    if (saved.job) return
    if (transcript[transcript.length - 1]?.role === 'assistant') return
    if (awaited.current === transcript.length) return
    awaited.current = transcript.length
    void send(transcript, slots, facts)
  }, [status, busy, transcript, slots, facts, saved.job, send])

  /** Ask the same question again, of the same conversation.
   *
   * A turn can come back with **no question** — the model answered with a
   * declarative sentence and `theme.py` deletes it (a statement here is the
   * mode telling somebody what they think instead of asking), or the JSON did
   * not parse, or the model declined. Every one of those is reported as a
   * `reason` with an empty `question`, and until now that was a dead end: the
   * reason renders where the question goes, Answer is disabled because there
   * is nothing to answer, and the auto-ask effect will not re-fire because
   * `awaited` already holds this transcript length. The only way out was
   * "Start over", which throws away the conversation to fix one bad turn.
   *
   * Nothing is lost by retrying and nothing is said twice: a failed turn
   * appends **no** assistant turn, so the transcript this re-sends is byte for
   * byte the one that was sent before. `awaited` is set rather than cleared,
   * for the same reason it exists — the effect must not decide to ask as well.
   */
  function retry() {
    if (busy) return
    awaited.current = transcript.length
    setReport(null)
    setError(null)
    void send(transcript, slots, facts)
  }

  function answerIt() {
    const said = answer.trim()
    if (!said || busy) return
    setAnswer('')
    // Record the answer and stop. The effect above notices there is no
    // question pending and fetches one — which keeps "who asks for the next
    // question" in exactly one place rather than two that can both fire.
    setSaved((s) => ({
      ...s, transcript: [...s.transcript, { role: 'user', text: said }],
    }))
  }

  // The proposal is a background job now (`api/themeruns.py`), because it was
  // measured at 226 seconds and a four-minute POST does not survive a hosted
  // proxy. So this is the simulator's shape: submit, poll, read the result off
  // the job. What it adds is that the id is *saved* — a reload reattaches to
  // the run in flight rather than paying for a second one.
  const poller = useRef<{ cancel: () => void } | null>(null)
  const followed = useRef<string | null>(null)

  const follow = useCallback((id: string) => {
    setBusy('proposing')
    setError(null)
    poller.current?.cancel()
    // How old the *run* is, not how long this tab has been watching it. They
    // differ exactly when it matters: reattaching after a reload showed 0s
    // against a job already a minute in, which is the confusion a clock was
    // put there to remove. The job's own `created_at` is the answer, and the
    // local start is only the guess held until the first poll corrects it.
    const started = { at: Date.now() }
    setElapsed(0)
    // Seconds rather than a percentage bar. The job reports its turn out of a
    // ceiling it usually does not reach, so a bar would sit at 38% and then
    // jump; an honest clock is more use to somebody deciding whether to wait.
    const clock = setInterval(
      () => setElapsed(Math.round((Date.now() - started.at) / 1000)), 1000)
    // Two seconds, not the 400ms default: this runs for minutes, and a poll
    // every 400ms would be six hundred requests to watch one job.
    const run = followJob(id, (job) => {
      const born = Date.parse(job.created_at)
      if (!Number.isNaN(born)) started.at = born
    }, 2000)
    poller.current = { cancel: () => { run.cancel(); clearInterval(clock) } }
    run.promise
      .then((job) => setSaved((s) => (
        { ...s, proposal: job.result as ThemeProposal, job: null })))
      .catch((e) => {
        setError(e instanceof ApiError && e.status === 404
          // Jobs live in the server's memory and die with it (`api/jobs.py`).
          // Say so rather than showing a bare 404 for something the person
          // never asked to look up.
          ? 'That run is gone — the server restarted while it was working. Ask again when you are ready.'
          : String((e as Error).message ?? e))
        setSaved((s) => ({ ...s, job: null }))
      })
      .finally(() => { clearInterval(clock); setBusy('') })
  }, [])

  // One place decides to follow a job, and it is this — the same argument as
  // the auto-ask effect above. `proposeIt` records the id and stops; a
  // restored tab arrives with the id already in state and lands here too, so
  // there is no second path that could double-poll.
  useEffect(() => {
    if (!saved.job || followed.current === saved.job) return
    followed.current = saved.job
    follow(saved.job)
  }, [saved.job, follow])

  // Both pollers. The tarot door remounts this component when the reader
  // changes (ADR 21 — a persona is fixed for a conversation), so an unmount
  // here is a live turn somebody has walked away from, not just a closing tab.
  useEffect(() => () => {
    poller.current?.cancel()
    asker.current?.cancel()
  }, [])

  async function proposeIt() {
    setBusy('proposing')
    setError(null)
    setElapsed(0)
    try {
      const job = await api.themePropose({
        transcript, slots,
        budget: budget ? Number(budget) : undefined,
        persona, seed: seed ?? undefined, stance: effectivePin(pin, status),
      })
      setSaved((s) => ({ ...s, job: job.id }))
    } catch (e) {
      // 409 below the floor, 503 with no key, 422 for a transcript the server
      // will not take — all still answered by the POST itself, which is why
      // they read as sentences here rather than as a job that failed.
      setError(String((e as Error).message ?? e))
      setBusy('')
    }
  }

  function startOver() {
    localStorage.removeItem(SAVED)
    poller.current?.cancel()
    // The turn in flight goes too, or its question lands in the conversation
    // that was just cleared — the empty transcript would gain an assistant
    // turn nobody asked for and the auto-ask effect would never fire.
    asker.current?.cancel()
    awaited.current = -1
    followed.current = null
    // Starting over keeps the reader and the cards. Those were chosen on the
    // way in and are the door's to change, not this button's — "start over"
    // means these answers, not this table.
    setSaved({ ...EMPTY, persona, seed })
    setReport(null)
    setError(null)
    setBusy('')
  }

  if (!status) {
    return <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
  }
  // Three states kept apart, because collapsing them tells somebody their key
  // is missing when they have simply not installed the extra.
  if (!status.installed || !status.configured) {
    return (
      <div className="card-surface rounded-xl px-6 py-8">
        <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
          {status.installed
            ? 'This door needs Claude, and this server has no key for it yet.'
            : 'This door needs Claude, which isn’t installed on this server.'}
          <span className="mt-1 block text-xs" style={{ color: 'var(--text-muted)' }}>
            {status.installed
              ? <>Set <code>ANTHROPIC_API_KEY</code> — see <code>.env.example</code>.</>
              : <><code>pip install -e &quot;.[claude]&quot;</code> adds it.</>}
          </span>
        </p>
        <button onClick={onLeave} className="mt-3 rounded-md px-3 py-1.5 text-sm"
                style={{ border: '1px solid var(--hairline)',
                         color: 'var(--text-secondary)' }}>
          {leaveLabel ?? '← Pick colours myself'}
        </button>
      </div>
    )
  }

  const grounded = report?.grounded ?? slots.length
  const floor = report?.floor ?? 3
  // A conversation restored from a previous tab has no report yet, and every
  // one of these has to come off the transcript instead — otherwise coming
  // back to it shows a stranger's blank screen with your own answers above it.
  const ready = report?.may_propose ?? (new Set(slots.map((s) => s.kind)).size >= floor)
  const last = transcript[transcript.length - 1]
  const question = report?.question
    || (last?.role === 'assistant' ? last.text : '')
  const spent = report?.exchanges
    ?? transcript.filter((t) => t.role === 'user').length
  const ceiling = report?.max_exchanges ?? 10
  // No question pending, and a reason why — the dead end `retry` exists for.
  //
  // `asked` is what separates a failed turn from a finished one. The stance
  // being `off` and the exchange ceiling both report a reason with no question
  // too, and neither is retryable: nothing was asked, and asking again would
  // get the same answer. Both come back `asked: false`, so this is one field
  // rather than a list of reasons to match on.
  //
  // A thrown error counts as stuck whatever the report says, because it leaves
  // the identical screen: no question, and a disabled Answer under it.
  const stuck = !busy && !question && (error !== null || report?.asked === true)

  // The room (punch list item 8): each voice's own painting washed across
  // the viewport, its accent on the chrome, its sign by the door. `plain`
  // keeps a bare room on purpose — no costume includes the walls.
  const roomArt = PERSONA_ART[persona]
  const accent = PERSONA_ACCENT[persona] ?? 'var(--series-1)'
  // The fortune-teller's table writes in ink on parchment (overhaul item 5,
  // commandment 15): the question card becomes a scroll, the words arrive
  // wet, the answer box takes a quill. Every other room keeps the plain
  // chrome — the costume is the reader's, not the interview's.
  const seance = persona === 'fortune-teller'

  return (
    <section className="persona-room space-y-5"
             style={{ '--room-accent': accent } as React.CSSProperties}>
      {/* The fortune-teller's room drifts with mana rather than mist — the
          crystal ball's violet light, given the whole floor. */}
      {roomArt && <SceneBackdrop art={roomArt.art}
                                 mood={persona === 'fortune-teller'
                                   ? 'wisps' : 'mist'} />}
      <div className="flex flex-wrap items-center gap-3">
        <RoomSign persona={persona} />
        {/* The framing paragraph only frames an *empty* table. Once the
            conversation exists it speaks for itself, and a fixed paragraph
            sitting above every exchange read as a script the interview was
            following — "a hard-coded prompt", reported twice, about the one
            surface where every sentence is actually generated. */}
        {transcript.length === 0
          ? (
            <div>
              <h2 className="text-xl font-semibold tracking-tight">
                {intro?.title ?? 'Let’s work out what you want'}
              </h2>
              <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
                {intro?.blurb ?? 'No Magic knowledge needed — the questions are '
                  + 'about you. Magic’s five colours started life as five '
                  + 'philosophies, so this is less of a detour than it sounds.'}
              </p>
              {roomArt && (
                <p className="mt-1 text-[10px]"
                   style={{ color: 'var(--text-muted)' }}>
                  The room wears {roomArt.credit}&rsquo;s painting.
                </p>
              )}
            </div>
            )
          : (
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              {intro?.title ?? 'Working you out'}
            </p>
            )}
        <div className="ml-auto flex items-center gap-2">
          {/* Armed, on ADR 27's pattern, because this is destructive twice
              over: it throws the conversation away *and* the empty transcript
              immediately draws a fresh opening question, which is a paid turn.
              It was reported as reading like an undo — a control that looks
              free, costs money, and gave no sign it had done anything. The
              armed label names both halves. */}
          {transcript.length > 0 && (
            <ArmedButton armedLabel="Discard and ask again"
                         title="Clears these answers and starts a new opening question"
                         onConfirm={startOver}>
              Start over
            </ArmedButton>
          )}
          <button onClick={onLeave} className="rounded-md px-3 py-1.5 text-sm"
                  style={{ border: '1px solid var(--hairline)',
                           color: 'var(--text-secondary)' }}>
            {leaveLabel ?? '← Pick colours myself'}
          </button>
        </div>
      </div>

      {error && (
        <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
      )}

      {!proposal && (
        <div className="grid gap-5 lg:grid-cols-[1fr_18rem]">
          <div className="space-y-4">
            {/* What has been said, so the conversation reads as one. Only a
                *trailing assistant turn* is held back — that is the pending
                question, and the card below renders it. A trailing *user*
                turn is the answer somebody just sent, and it must appear as
                their bubble immediately: the old `slice(0, -1)` swallowed it
                until the next question landed, so for the length of a Claude
                call your own words looked like they had gone nowhere. */}
            {(() => {
              const shown = transcript[transcript.length - 1]?.role === 'assistant'
                ? transcript.slice(0, -1)
                : transcript
              return shown.length > 0 && (
                <ol className="space-y-3">
                  {shown.map((t, i) => (
                    <li key={`${i}-${t.text.slice(0, 12)}`}
                        className={t.role === 'user' ? 'text-right' : ''}>
                      <span className={`chat-bubble inline-block max-w-[85%] rounded-xl px-3 py-2 text-sm${
                              seance && t.role === 'assistant'
                                ? ' seance-bubble' : ''}`}
                            style={t.role === 'user'
                              // The room's accent, not the app's: your own
                              // words wear the colour of whoever you are
                              // talking to (item 8).
                              ? { background: 'var(--room-accent, var(--series-1))',
                                  color: '#fff' }
                              : seance
                                ? undefined
                                : { border: '1px solid var(--hairline)',
                                    color: 'var(--text-secondary)' }}>
                        {t.text}
                      </span>
                    </li>
                  ))}
                </ol>
              )
            })()}

            {report?.fact && <FactNote fact={report.fact} seance={seance} />}

            {/* The scroll carries its own geometry: the deckle mask sets its
                silhouette and its padding (letters that reach the torn edge
                get torn with it), so the utility classes stay on the plain
                card only. */}
            <div className={seance
              ? 'seance-scroll'
              : 'rounded-xl px-5 py-4 card-surface'}>
              <p className={`text-base leading-relaxed${
                   busy === 'asking' ? ' thinking-pulse' : ''}${
                   seance ? ' seance-question' : ''}`}
                 style={{ color: busy === 'asking'
                   ? 'var(--text-muted)' : 'var(--text-primary)' }}>
                {busy === 'asking'
                  ? (seance ? 'The quill hovers…' : 'Thinking…')
                  : seance && question
                    // The reader's words arrive as ink soaking into the
                    // page; everyone else's questions simply print.
                    ? <InkText text={question} />
                    : question || report?.reason
                    // A thrown turn sets no report, so there is no `reason` to
                    // show — and "Starting…" under a red error line describes
                    // the one thing that is definitely not happening.
                    || (stuck ? 'That question did not arrive.' : 'Starting…')}
              </p>
              <textarea
                ref={box}
                value={answer}
                onChange={(e) => setAnswer(e.target.value)}
                onKeyDown={(e) => {
                  // Enter sends, shift-enter is a newline. A conversation is
                  // mostly one-liners and reaching for a button each time
                  // makes it feel like a form, which is the thing it is not.
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    answerIt()
                  }
                }}
                rows={2}
                placeholder={seance
                  ? 'Write your answer on the parchment…'
                  : 'However much or little you like…'}
                className={`mt-3 w-full rounded-md px-3 py-2 text-sm${
                  seance ? ' seance-quill' : ''}`}
                style={seance
                  ? undefined
                  : { background: 'var(--page)', color: 'var(--text-primary)',
                      border: '1px solid var(--hairline)' }}
              />
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <button onClick={answerIt} disabled={!answer.trim() || !!busy}
                        className="rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                        style={{ background: 'var(--series-1)', color: '#fff' }}>
                  Answer
                </button>
                {/* The way out of a turn that produced nothing. Sits next to
                    Answer rather than replacing it, because the textarea is
                    still perfectly usable — this is the control for the case
                    where there is nothing to answer *yet*, which is the
                    opening turn most of the time. */}
                {stuck && (
                  <button onClick={retry}
                          className="rounded-md px-4 py-2 text-sm font-medium"
                          style={{ border: '1px solid var(--hairline)',
                                   color: 'var(--text-primary)' }}>
                    Try that again
                  </button>
                )}
                {/* Never "3 of 10" once the floor is met. The ceiling is a
                    guard rail, not a quota, and a counter that keeps counting
                    read as one — people sat through ten questions because the
                    number said there were ten.
                    Nor while stuck: a count of questions asked, printed beside
                    a question that never arrived, reads as blaming the person
                    for not answering it. */}
                <span className="text-xs" style={{
                  color: ready ? 'var(--series-2)' : 'var(--text-muted)' }}>
                  {stuck
                    ? ''
                    : ready
                      ? '✓ Enough answered — the rest is optional'
                      : `${spent} of ${ceiling} questions at most`}
                </span>
              </div>
            </div>

            {/* The short circuit, where the eye already is. The sidebar
                button lights up when the floor is met, but the person is
                reading the conversation column — so the conversation column
                is where "you can stop now" has to be said. */}
            {ready && busy !== 'proposing' && (
              <div className="ready-banner rounded-xl px-5 py-4">
                <p className="text-sm font-medium"
                   style={{ color: 'var(--text-primary)' }}>
                  {seed !== null
                    ? 'Three cards, three answers — the reading is ready.'
                    : 'That’s enough to go on.'}
                </p>
                <p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>
                  Keep talking if you’re enjoying it, or get your colours now.
                </p>
                {/* Worded apart from the sidebar's "Suggest my colours" —
                    two controls, one act, and a reader (or a test) should be
                    able to tell which one they pressed. */}
                <button onClick={proposeIt} disabled={!!busy}
                        className="mt-3 rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                        style={{ background: 'var(--series-2)', color: '#fff' }}>
                  {seed !== null ? 'Read my cards' : 'Get my colours'}
                </button>
              </div>
            )}
          </div>

          <aside className="space-y-3">
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              What it has picked up
            </p>
            {slots.length === 0 && (
              <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
                Nothing yet — it only counts things you have actually said.
              </p>
            )}
            {slots.map((s) => <Chip key={s.kind} slot={s} />)}

            {/* A number that climbs is a model inventing preferences, which is
                exactly the failure the grounding check exists for. Rendered
                rather than logged, for the same reason the interview shows
                how many of its answers were not questions. */}
            {!!report?.slots_dropped && (
              <p className="text-xs" style={{ color: 'var(--status-warning)' }}>
                {report.slots_dropped} reading
                {report.slots_dropped === 1 ? '' : 's'} did not match anything
                you said, and {report.slots_dropped === 1 ? 'was' : 'were'} dropped.
              </p>
            )}

            <div className="border-t pt-3" style={{ borderColor: 'var(--hairline)' }}>
              <label className="block">
                <span className="text-[10px] uppercase tracking-wide"
                      style={{ color: 'var(--text-muted)' }}>
                  Budget for the deck (optional)
                </span>
                <input value={budget} inputMode="decimal"
                       onChange={(e) => setBudget(e.target.value.replace(/[^\d.]/g, ''))}
                       placeholder="150"
                       className="mt-1 w-full rounded-md px-3 py-1.5 text-sm"
                       style={{ background: 'var(--page)', color: 'var(--text-primary)',
                                border: '1px solid var(--hairline)' }} />
              </label>
              <button onClick={proposeIt} disabled={!ready || !!busy}
                      className="mt-3 w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                      style={{ background: ready ? 'var(--series-2)' : 'transparent',
                               color: ready ? '#fff' : 'var(--text-muted)',
                               border: ready ? 'none' : '1px solid var(--hairline)' }}>
                {busy === 'proposing'
                  ? `Reading around… ${elapsed}s`
                  : 'Suggest my colours'}
              </button>
              <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
                {/* Measured at three to four minutes: it reads a dozen-odd
                    pages and checks every legend against the pool. Saying so
                    is cheaper than a spinner somebody assumes has hung — and
                    the clock on the button is cheaper still, because a number
                    that moves is the difference between slow and stuck. */}
                {busy === 'proposing'
                  ? 'A few minutes — it reads around and checks every card. It carries on if you reload or close the tab.'
                  : ready
                    ? 'Ready whenever you are — or keep talking.'
                    : `${grounded} of ${floor} things known so far.`}
              </p>
            </div>

            {/* Last in the column, because it is a setting rather than a step.
                The control itself is the header's Claude menu now; this line
                reports what that setting resolves to for this conversation. */}
            <div className="border-t pt-2" style={{ borderColor: 'var(--hairline)' }}>
              <StanceReadout status={status} pin={pin} />
            </div>
          </aside>
        </div>
      )}

      {proposal && (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
              Pick a commander to carry on — you will name the deck next, and
              nothing is created until you say so.
            </p>
            <button onClick={() => setSaved((s) => ({ ...s, proposal: null }))}
                    className="ml-auto rounded-md px-3 py-1.5 text-sm"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              ← Keep talking
            </button>
          </div>

          {proposal.combinations.length === 0 && (
            <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
              {proposal.reason || 'Nothing usable came back.'}
            </p>
          )}

          {proposal.combinations.map((combo, i) => (
            <CombinationPanel key={combo.key} combo={combo} rank={i}
                              sources={proposal.sources}
                              onPick={(card) => onPick(combo.key, card)} />
          ))}

          {/* ADR 14's third boundary, at the bottom of the thing it applies
              to. The counts sit here rather than in a log because a number
              nobody can see is a number nobody checks. */}
          {proposal.combinations.length > 0 && (
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              {proposal.never} Written by Claude over{' '}
              {proposal.searched} page{proposal.searched === 1 ? '' : 's'}
              {proposal.commanders_dropped > 0 &&
                ` · ${proposal.commanders_dropped} named card${
                  proposal.commanders_dropped === 1 ? '' : 's'} did not resolve and ${
                  proposal.commanders_dropped === 1 ? 'was' : 'were'} dropped`}
              {proposal.combinations_dropped > 0 &&
                ` · ${proposal.combinations_dropped} further suggestion${
                  proposal.combinations_dropped === 1 ? '' : 's'} lost every
                  commander to that check`}
              {proposal.sources_dropped > 0 &&
                ` · ${proposal.sources_dropped} citation${
                  proposal.sources_dropped === 1 ? '' : 's'} were not among the pages read`}
            </p>
          )}
        </div>
      )}
    </section>
  )
}
