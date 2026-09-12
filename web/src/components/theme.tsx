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
 * that already exists — it does not make a deck. Nothing in the server's
 * Claude modes can reach a write path, and this is the UI telling the
 * same truth: the deck is made by the person whose deck it is.
 */

import { useCallback, useEffect, useId, useMemo, useRef, useState,
         type CSSProperties, type ReactNode } from 'react'
import {
  ApiError,
  api,
  errorMessage,
  followJob,
  type BrewReading,
  type ClaudeStatus,
  type ThemeCombination,
  type ThemeCommander,
  type ThemeFact,
  type ThemeProposal,
  type ThemeReport,
  type ThemeSlot,
  type ThemeTurn,
} from '../lib/api'
import { Cauldron, ReadyPulse } from './cauldron'
import { POURED, POURING, SERVE_MS, potLabel } from '../lib/cauldroncopy'
import { costumeFor } from '../lib/costumes'
import { reducedMotion } from '../lib/motion'
import { COLOR_VAR } from '../lib/mtg'
import { personaAccent, personaArt } from '../lib/personart'
import { useStashed } from '../lib/stash'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'
import { SceneBackdrop } from './forest'
import { ArmedButton, CardHover, ColorRing, ErrorNote, Spinner } from './ui'
import { ReplayGlyph } from './glyphs'
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

/* ------------------------------------------- when something does not arrive
 *
 * **Commandment 10, which this screen was breaking three ways.** A job the
 * server no longer held said *"the server restarted while it was working"*; a
 * browser that could not reach it printed whatever it had thrown, so the room
 * answered `Load failed`; and the door with no key recited an environment
 * variable and the file to put it in. None of that is a sentence about Magic,
 * or about anything the person reading it can do.
 *
 * **The detail is not deleted, it is redirected** — `forgeTrouble`'s argument
 * in the Go tree, one surface over. Everything above is exactly what somebody
 * fixing this wants and none of it is what somebody waiting on a question
 * wants, so it goes to the console, where the first audience reads, and the
 * room is handed a sentence written for the second.
 *
 * What is deliberately *not* rewritten is a refusal the server wrote itself:
 * "that is as long as this conversation goes", "the stance is off, so no call
 * was made", a transcript it will not take. Those are sentences about the
 * interview, they are already in its voice, and anything this file put in
 * their place would say less.
 */

/** No key, or a Claude nobody here can reach. The same sentence whether it is
 *  found by the dial before a word is said or by a 503 mid-conversation. */
const CLOSED = 'This door needs Claude, and Claude cannot be reached from '
  + 'here at the moment. Nothing is wrong with anything you said — the other '
  + 'way in still works.'
const GONE_QUESTION = 'That question was lost on its way to you. Nothing you '
  + 'have said has gone anywhere — say something and it will be asked again.'
const LOST_QUESTION = 'The question did not make it through. Nothing you have '
  + 'said has gone anywhere — try that again.'
const GONE_READING = 'That reading was lost before it was finished. Nothing '
  + 'you have said has gone anywhere — ask for it again when you are ready.'
const LOST_READING = 'The reading did not make it through. Nothing you have '
  + 'said has gone anywhere — ask for it again.'

/** The room's words for a call that did not come back: `gone` when the server
 *  no longer has the run, `lost` when nothing reached it at all. */
function trouble(e: unknown, gone: string, lost: string): string {
  // Anything that is not a refusal is a browser's own account of a connection,
  // and a browser writes for developers.
  if (!(e instanceof ApiError)) return lost
  // Two statuses are facts about the machinery rather than about the
  // conversation, and both arrive worded for whoever runs the place: a run
  // that is no longer in memory, and an endpoint with no key behind it.
  if (e.status === 404) return gone
  if (e.status === 503) return CLOSED
  return errorMessage(e)
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
      case 'witch':
        // A three-legged cauldron with something pale-green working in it.
        // The chef two cases above also has a pot, so this one had to be
        // unmistakably the other kind, and the legs are what do it: a lidded
        // pan sits on a stove, a cauldron stands in a fire on its own feet.
        //
        // Fire was drawn first and thrown out twice. Under the belly at
        // 64x48 the tongues read as orange legs; moved to the sides they
        // read as leaves. The bubbles already say the thing is hot, and a
        // sign this small can carry one idea.
        //
        // The brew keeps a colour of its own, the barkeep's foam rule — but
        // paler than the room's accent rather than a different hue, because
        // in this one room the accent IS green, and two greens of the same
        // weight merge into a green pot of green.
        return (
          <svg viewBox="0 0 64 48" className="h-12 w-16">
            {/* Two feet splayed and the third behind, which is what a
                three-legged pot looks like from the front. */}
            <path d="M21 36 L 17 45 M32 39 L 32 44 M43 36 L 47 45"
                  stroke="currentColor" strokeWidth="3" strokeLinecap="round"
                  opacity="0.85" />
            {/* The three named parts carry a class each so that ONE placement
                — the header over the hut, below — can re-ink them without the
                drawing changing. Inert everywhere else: nothing styles them
                unless `.room-sign-hut` is on the wrapper. */}
            <path className="hut-sign-body"
                  d="M15 21 C 15 33 22 39 32 39 C 42 39 49 33 49 21 Z"
                  fill="currentColor" opacity="0.85" />
            <path className="hut-sign-rim" d="M11 21 H 53" stroke="currentColor"
                  strokeWidth="3.4" strokeLinecap="round" />
            <ellipse className="hut-sign-brew" cx="32" cy="19.4" rx="15"
                     ry="3.2" fill="#c8ef86" />
            <circle className="lab-bubble" cx="26" cy="15" r="2.4"
                    fill="#c8ef86" opacity="0.85" />
            <circle className="lab-bubble lab-bubble-2" cx="37" cy="12" r="1.9"
                    fill="#c8ef86" opacity="0.7" />
            <circle className="lab-steam" cx="31" cy="7" r="2.7"
                    fill="#c8ef86" opacity="0.4" />
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
  // **The hut's sign is struck smaller and in iron, and only here.** A door
  // sign is an emblem — flat, in the room's accent, read at a glance — and
  // that is the right thing on a tile in a grid. Standing on top of a
  // Waterhouse oil at forty-eight pixels of spring green it is a cartoon
  // pasted onto a painting, which is commandment 5 exactly. `.room-sign-hut`
  // in index.css is the whole change: half the size, cast iron in ash, and the
  // green kept as a line rather than a fill. The drawing is untouched, so the
  // sign is the sign it was anywhere it is rendered without this class.
  const hut = persona === 'witch' ? ' room-sign-hut' : ''
  return (
    <span className={`room-sign shrink-0${hut}`} aria-hidden="true">{sign}</span>
  )
}

/** The hand's pace (the brief's front two, third pass). A fixed,
 *  comfortable rate — a long sentence takes longer, which is the honest
 *  arithmetic the old 8-second cap inverted (past ~308 characters it made
 *  the hand HURRY, faster the more it had to say). The floor stays so a
 *  two-word answer is not instant; the cap is gone, replaced by a skip —
 *  click the line and it finishes at once, because making the reader wait
 *  is a choice they should be able to decline without making the hand
 *  write like a machine. */
const INK_MS_PER_CHAR = 52
const INK_MIN_MS = 900
/** Where the pen pauses: lifted between words, longer at a breath, longer
 *  still at a full stop. This unevenness is most of what separates a hand
 *  from a metronome. */
const INK_PAUSE_WORD = 90
const INK_PAUSE_BREATH = 260   // , ; : and the em dash
const INK_PAUSE_STOP = 420     // . ? !

/**
 * A hand writing on the parchment (overhaul item 5; third pass gave it
 * the rhythm, and the fourth gave it back its joins). The unit of TIMING
 * is the character; the unit of MARKUP is the word — and the difference
 * is not pedantry. The third pass wrapped every glyph in its own span,
 * and a connected script shapes its letters from their neighbours:
 * OpenType shaping does not cross element boundaries, so Parisienne
 * rendered every letter in its isolated form and "Before" came out
 * "Belore" — and Caveat, its successor, joins by contextual alternates
 * and is bound by exactly the same rule, so the word span stays and
 * nothing may ever go back to per-letter spans. A word kept whole shapes
 * correctly, and one CONTINUOUS wipe across it, timed from its character
 * count, reads as the pen travelling — precisely because in a joined
 * hand the leading edge never leaves a stroke. (Handwriting does not
 * join across spaces, so the word boundary costs nothing.)
 *
 * Each word carries a hair of tilt and drop (deterministic per index — a
 * hand wobbles, a render must not); the ink starts wet-brown and dries
 * dark; the pen pauses between words, longer at punctuation, and
 * resettles slightly on each word's opening. A drawn quill rides the
 * wipe's leading edge, and lifts off when the sentence is done.
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
    const spans = Array.from(el.querySelectorAll<HTMLElement>('.ink-word'))
    if (spans.length === 0) return
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
      let i = schedule.findIndex((s) => t < s.start + s.dur)
      if (i === -1) i = spans.length - 1
      const word = spans[i]
      const s = schedule[i]
      if (word && s) {
        const frac = Math.min(Math.max((t - s.start) / s.dur, 0), 1)
        const box = word.getBoundingClientRect()
        const home = el.getBoundingClientRect()
        // The nib rides the wipe's leading edge, a touch above the baseline.
        const x = box.left - home.left + box.width * frac
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
    // Reachable by the keyboard, and deliberately NOT a button (green's pass,
    // 2026-09-12). A button's accessible name is its own contents, so wrapping
    // the question in one would have a screen reader announce the whole
    // sentence as the label of a control — where what this is is the question,
    // which happens to be able to hurry itself up. So: focusable, Enter or
    // Space finishes the line, no role, and nothing an eye can see changes
    // until the focus ring arrives. Nothing is withheld from a reader by
    // leaving the role off, either — the text is in the DOM from the first
    // frame and the wipe is CSS, so a skip is a courtesy to a waiting eye and
    // means nothing at all to a reader.
    <span ref={host} className={`ink-text${skipped ? ' is-dry' : ''}`}
          tabIndex={0}
          onClick={() => setSkipped(true)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              setSkipped(true)
            }
          }}
          title={skipped ? undefined
            : 'Click, or press Enter, to let the ink dry at once'}>
      {text.split(/(\s+)/).map((part, i) => {
        if (/^\s*$/.test(part)) return part
        const n = word++
        const s = schedule[n]
        return (
          <span key={`${i}-${part}`} className="ink-word"
                style={{
                  '--ink-delay': `${s?.start ?? 0}ms`,
                  '--ink-dur': `${s?.dur ?? 200}ms`,
                  // A hand wobbles; a render must not. Deterministic jitter.
                  '--ink-tilt': `${(((n * 7) % 5) - 2) * 0.35}deg`,
                  '--ink-drop': `${(((n * 3) % 3) - 1) * 0.6}px`,
                } as CSSProperties}>
            {part}
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

/** Readings the server threw away because it could not find them in the
 *  transcript. One sentence in one place: the sidebar prints it and the live
 *  region says it, and a count somebody hears must be the count they can then
 *  go and read. */
function droppedNote(n: number): string {
  return `${n} reading${n === 1 ? '' : 's'} did not match anything you said, `
    + `and ${n === 1 ? 'was' : 'were'} dropped.`
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

/** Who to credit under a fun fact.
 *
 * Three origins reach here and they are credited differently. A fact read off
 * a page carries its URL and gets a link. A fact from the colour reference
 * data carries the literal source `taxonomy`, and saying so plainly is the
 * whole point — it is this tool talking about itself.
 *
 * The third is the trap this function exists for. When the fortune-teller
 * cites the deck's own history the server sends a real citation (a book, a
 * year) and **no URL**, and the old code keyed on the URL alone: anything
 * without one was captioned "From this tool’s own colour reference data",
 * so a fact about Pamela Colman Smith would have been credited to a table of
 * Magic colours. Key on the source, not on the absence of a link.
 */
function factCredit(fact: NonNullable<ThemeReport['fact']>): ReactNode {
  if (fact.url) {
    return <a href={fact.url} target="_blank" rel="noreferrer noopener"
              className="underline">{fact.source}</a>
  }
  if (!fact.source || fact.source === 'taxonomy') {
    return 'From this tool’s own colour reference data.'
  }
  return fact.source
}


function FactNote({ fact, dressed }: {
  fact: NonNullable<ThemeReport['fact']>
  /** The room's class for this aside, or '' for the plain chrome — whose paint
   *  stays here, since a room with no costume should not have to name one. */
  dressed: string
}) {
  return (
    <aside className={`rounded-lg px-4 py-3${dressed ? ` ${dressed}` : ''}`}
           style={dressed ? undefined : { background: 'var(--gridline)' }}>
      <p className="text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        While you are here
      </p>
      <p className="mt-1 text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>{fact.text}</p>
      <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
        {factCredit(fact)}
      </p>
    </aside>
  )
}

function CommanderTile({ card, onPick }: {
  card: ThemeCommander
  onPick: () => void
}) {
  return (
    <CardHover tapOpens={false} card={card} className="block">
      <button onClick={onPick}
              className="pick-tile card-surface block w-full overflow-hidden rounded-xl text-left">
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
  onPick, onLeave, persona = 'plain', seed = null, pot = null, intro,
  leaveLabel,
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
  /** What is in the witch's pot, when this room has one on. The same seed
   *  picks the same three things server-side, so the strip and the questions
   *  can never disagree about what is going in.
   *
   *  **The pot reports the grounding; it never performs it.** Everything it
   *  reads is `slots`, which is the readiness instrument's own answer, and
   *  there is no field anywhere on this page that lets a pot count something
   *  the transcript does not carry. */
  pot?: BrewReading | null
  /** The reader's own framing, when somebody else is setting the scene. */
  intro?: { title: string; blurb: string }
  leaveLabel?: string
}) {
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [saved, setSaved] = useStashed(SAVED, () => load(persona, seed))
  const { transcript, slots, proposal, facts } = saved
  const [report, setReport] = useState<ThemeReport | null>(null)
  const [answer, setAnswer] = useState('')
  const [budget, setBudget] = useState('')
  const [busy, setBusy] = useState<'' | 'asking' | 'proposing'>('')
  const [elapsed, setElapsed] = useState(0)
  const [error, setError] = useState<string | null>(null)
  /* What the pot last said, in the pot's own words. One string, printed under
     the strip and said by the live region — never two wordings of it. Cleared
     when the next question starts being written, so the beat that follows is a
     change rather than a repeat of the same joined sentence. */
  const [stirred, setStirred] = useState('')
  /* The pour, which runs on the plate while the reading is worked out. */
  const [serving, setServing] = useState(false)
  /* One slow pulse of the brew's green across the room behind the
     conversation, on the beat the pot comes up ready. Motion, no reflow. */
  const [pulsing, setPulsing] = useState(false)
  const box = useRef<HTMLTextAreaElement>(null)
  // The answer box's label is invisible but it is a real `<label>`, and a label
  // needs an id to point at. One per mounted interview.
  const answerId = useId()
  const [pin, setPin] = useStance()

  // No slug, because there is no deck yet — which is the whole point of this
  // mode. With neither a deck nor a pin `theme._stance` supplies its own
  // default (`SECOND_OPINION`, since a deck nobody has built is as theoretical
  // as a deck gets), so "follow the deck" is not an empty position here.
  useEffect(() => {
    fetchClaudeStatus({ surface: 'theme' }, pin, () => setPin(null))
      .then(setStatus).catch(() => setStatus(null))
  }, [pin, setPin])

  // The half of a shut door that a player must never read, said where the
  // person who can open it is reading instead (commandment 10 — `trouble`
  // above carries the argument). Not in the branch that renders it: that
  // branch runs on every keystroke this screen sees.
  useEffect(() => {
    if (status && (!status.installed || !status.configured)) {
      console.error('the theme interview is shut: no ANTHROPIC_API_KEY in this '
        + 'server’s environment — put one in .env (see .env.example), or '
        + '`fly secrets set` it when deployed')
    }
  }, [status])

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
      // The diagnosis to the console, the sentence to the room — see `trouble`.
      console.error('the theme interview: a turn did not come back', e)
      setError(trouble(e, GONE_QUESTION, LOST_QUESTION))
    } finally {
      setBusy('')
      box.current?.focus()
    }
    // `setSaved` is `useState`'s own setter arriving through `useStashed`, so
    // it is exactly as stable as it was when it was destructured here — it is
    // listed because the rule cannot see that through a custom hook, not
    // because anything about it moves.
  }, [persona, seed, pin, status, setSaved])

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
   * declarative sentence and the server deletes it (a statement here is the
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

  // The proposal is a background job now, because it was
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
        console.error('the theme interview: a reading did not come back', e)
        setError(trouble(e, GONE_READING, LOST_READING))
        setSaved((s) => ({ ...s, job: null }))
      })
      .finally(() => { clearInterval(clock); setBusy('') })
    // Stable, and listed for the reason `send` above gives.
  }, [setSaved])

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

  // The pot's beat has passed once the next question is being written. The
  // live region is empty for that whole stretch anyway (see `said` below), so
  // clearing here is what makes the question that follows a *change* rather
  // than a re-render of the same joined string with the stir still on the
  // front of it.
  useEffect(() => {
    if (busy === 'asking') setStirred('')
  }, [busy])

  /* The pour, and it is the only thing on this screen that is theatre rather
     than report: the ladle, the dip and the vial take about four and a half
     seconds and the reading takes minutes, so this is a beat at the start of a
     wait rather than a progress bar for it. Cleared on a timer rather than
     when the job lands, for exactly that reason. */
  const serve = useRef<number | null>(null)
  useEffect(() => () => { if (serve.current) clearTimeout(serve.current) }, [])

  async function proposeIt() {
    setBusy('proposing')
    setError(null)
    setElapsed(0)
    if (pot && !reducedMotion()) {
      setServing(true)
      setStirred(POURING)
      if (serve.current) clearTimeout(serve.current)
      serve.current = window.setTimeout(() => setServing(false), SERVE_MS)
    } else if (pot) {
      // Reduced motion gets the sentence and no ladle: the pour is reported in
      // words and by the brew settling, which is what §10's rule asks for.
      setStirred(POURING)
    }
    try {
      const job = await api.themePropose({
        transcript, slots,
        budget: budget ? Number(budget) : undefined,
        persona, seed: seed ?? undefined, stance: effectivePin(pin, status),
      })
      setSaved((s) => ({ ...s, job: job.id }))
    } catch (e) {
      // 409 below the floor and 422 for a transcript the server will not take
      // are still answered by the POST itself, which is why they read as
      // sentences here rather than as a job that failed — `trouble` passes
      // those through and keeps its own words for the two that are about the
      // machinery instead of about the conversation.
      console.error('the theme interview: the reading was not accepted', e)
      setError(trouble(e, GONE_READING, LOST_READING))
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
    // The pot is doused with everything else. Starting over keeps the reader
    // and the same three ingredients — those were picked on the way in and are
    // the door's to change — but nothing is in it any more.
    setStirred('')
    setServing(false)
    setPulsing(false)
  }

  // The potion, handed over. Said once, when the panels arrive.
  useEffect(() => {
    if (pot && proposal) setStirred(POURED)
  }, [pot, proposal])

  // The proposal's twin of `noQuestion` below: a run that produced nothing
  // usable speaks in the room's own words on screen, and the server's
  // recorded reason — which may carry a wire token, and the crossing corpus
  // freezes that sentence — goes to the console instead of the room.
  useEffect(() => {
    if (proposal?.asked && !proposal.combinations.length && proposal.reason)
      console.error('the proposal came back empty:', proposal.reason)
  }, [proposal])

  // Everything the screen is made of, derived once — and above the early
  // returns rather than below them, because the live region at the foot of
  // this block has to be declared before the first `return` can render it.
  // `components/tarot.tsx` has the same shape for the same reason.
  //
  // **One count, read one way.** `grounded` and `ready` used to disagree about
  // a restored tab: the number came off `slots.length` and the readiness off
  // the distinct kinds among them, which is two answers to "how much does it
  // know". They cannot differ today — the server keys its readings by kind, so
  // there is at most one of each — but "they happen to agree" is not a reason
  // for two spellings, and the kind is the one the floor is counted in.
  const kinds = new Set(slots.map((s) => s.kind)).size
  const grounded = report?.grounded ?? kinds
  /* The same set again, as the pot reads it: which kinds, in the order they
     landed. **A slot kind appearing that was not there on the previous render
     IS the grounding** — no server change, no new field, and the pot can never
     count something the transcript does not carry. Memoised on the joined
     spelling because the array is an argument to an effect one component down,
     and a fresh array every render is an effect that runs every render. */
  const kindList = slots.map((s) => s.kind)
  const kindKey = kindList.join(',')
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const inPot = useMemo(() => kindList, [kindKey])
  const floor = report?.floor ?? 3
  // A conversation restored from a previous tab has no report yet, and every
  // one of these has to come off the transcript instead — otherwise coming
  // back to it shows a stranger's blank screen with your own answers above it.
  const ready = report?.may_propose ?? (kinds >= floor)
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

  // The other way a turn comes back with no question, and the opposite kind of
  // thing: not a failure to retry but a conversation that has ended. Ten
  // exchanges is the ceiling (`theme.MAX_EXCHANGES`), and past it the server
  // answers without calling anybody — `asked: false`, a reason, no question.
  //
  // It needed a name because until it had one this was a wall. The scroll
  // printed "that is as long as this conversation goes", the answer box stayed
  // open under it, and every further answer fetched the same sentence back;
  // "Suggest my colours" was disabled if the floor had not been met, and the
  // only live controls on the screen threw the evening away. Somebody who
  // answers ten questions shyly and is handed that has been told, by a room
  // built for them, that they answered wrong.
  const finished = !busy && !question && report?.asked === false
    && spent >= ceiling

  // The room (punch list item 8): each voice's own painting washed across
  // the viewport, its accent on the chrome, its sign by the door. `plain`
  // keeps a bare room on purpose — no costume includes the walls.
  const roomArt = personaArt(persona)
  const accent = personaAccent(persona)
  // And what this room is wearing (`lib/costumes.ts`): the fortune-teller's
  // question card is a scroll, her words arrive wet, her answer box takes a
  // quill. This was a boolean and six ternaries until there was a second
  // costumed room to build — the costume is the reader's, not the
  // interview's, and now it is a record rather than an `if`.
  const costume = costumeFor(persona)

  /**
   * What is printed where the question goes when there is no question.
   *
   * **A turn that RAN and produced nothing gets the room's words; a turn that
   * never ran keeps the server's.** The first is a shrug — the answer did not
   * end in a question mark, or did not parse — and "Nothing usable came back."
   * is the app talking about itself in a room that has a voice of its own
   * (commandment 10, and `lib/costumes.ts` is where the voices live). The
   * second is `asked: false`: the stance is off, or this is as long as the
   * conversation goes, and those sentences are real explanations that no
   * costume may paint over — a witch saying "the steam took that one" when
   * what actually happened is a setting would be a room lying about itself.
   *
   * The plain room's entry is the sentence that was already here, so nothing
   * uncostumed moves. Read by both the card and the live region below, because
   * the eye and the ear are told the same thing in the same words.
   */
  const noQuestion = !question && report?.asked && report.reason
    ? costume.emptyReply
    : report?.reason ?? ''

  /**
   * The interview, said out loud: the question arriving, the moment there is
   * enough to read from, and any reading that was thrown away.
   *
   * **The tarot table's mechanism, copied deliberately** — that file argues it
   * at length and `tarot.test.tsx` pins it. A live region has to be in the
   * document *before* its text changes; one that mounts with its sentence
   * already in it is initial content, which readers do not announce. This
   * screen's entire business is one sentence arriving after a wait of seconds,
   * and it had no live region at all: a question that took thirty seconds to
   * write landed in silence.
   *
   * Empty while the question is being written, for the same reason in reverse:
   * the emptying is what makes the next one a change rather than a re-render
   * of the same string.
   *
   * Failures are deliberately not here. They go through `ErrorNote`, whose
   * `role="alert"` is assertive, because somebody waiting on an answer that is
   * never coming should not hear about it in turn.
   *
   * Every sentence is one an eye can also read, never a second wording of it —
   * the question as the card prints it, the banner's own line, the sidebar's
   * count of dropped readings.
   */
  const said = busy === 'asking' ? '' : [
    // The newest thing in the pot, led by its PLACE — "The Base takes the rose
    // hips. One of three…" — because the shelf holds "Quince" beside "Rose
    // hips" and "the X goes in" is ungrammatical for half of it. It comes
    // first because it is what just happened.
    stirred,
    // Once the panels are up, the question and the banner are no longer on
    // the screen — and a sentence an eye cannot find is exactly what this
    // region's second rule forbids. What is left is the one line about the
    // thing that just arrived.
    // `noQuestion` rather than `report.reason` straight, so the ear is told
    // what the eye is shown: the card above prints the same value, and a room
    // that said one thing on the slate and another in the live region would
    // be two rooms. Uncostumed, it IS `report.reason`.
    proposal ? '' : question || noQuestion || '',
    proposal || !ready ? '' : costume.ready,
    report?.slots_dropped ? droppedNote(report.slots_dropped) : '',
  ].filter(Boolean).join(' ')
  const say = <span className="sr-only" role="status">{said}</span>

  if (!status) {
    return <>{say}<Spinner label="Opening the interview…" /></>
  }
  // One way this door stays shut: no key. The second arm this branch used to
  // carry — Claude absent from the server — cannot happen, because the client
  // is linked into the binary and the dial's `installed` is a constant.
  //
  // What it may not do is say *which* key, or where to put it: that sentence
  // is for whoever runs this, and it is in the console (see the effect above).
  if (!status.installed || !status.configured) {
    return (
      <>
      {say}
      <div className="card-surface rounded-xl px-6 py-8">
        <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
          {CLOSED}
        </p>
        <button onClick={onLeave} className="btn btn-quiet btn-sm mt-3">
          {leaveLabel ?? '← Pick colours myself'}
        </button>
      </div>
      </>
    )
  }

  return (
    <>
    {say}
    <section className="persona-room space-y-5"
             style={{ '--room-accent': accent } as CSSProperties}>
      {/* The fortune-teller's room drifts with mana rather than mist — the
          crystal ball's violet light, given the whole floor. */}
      {roomArt && <SceneBackdrop art={roomArt.art} mood={costume.mood} />}
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
                  {/* And the hut is a whole painting rather than a card crop,
                      so it is named the way a painting is named. Credited
                      where it renders, which is the rule whether the licence
                      makes it one or not. */}
                  {pot && (
                    <> The pot is John William Waterhouse&rsquo;s{' '}
                    <em>The Magic Circle</em>, 1886.</>
                  )}
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
          <button onClick={onLeave} className="btn btn-quiet btn-sm">
            {leaveLabel ?? '← Pick colours myself'}
          </button>
        </div>
      </div>

      {/* Through the one component every refusal in the app comes through, for
          its `role="alert"` — a failure is the one message that must not wait
          its turn behind whatever is being read. It was a coloured line of
          text, which is a failure only an eye can find. */}
      {error && <ErrorNote>{error}</ErrorNote>}

      {!proposal && (
        <div className="grid gap-5 lg:grid-cols-[1fr_18rem]">
          <div className="space-y-4">
            {/* The pot, folded (`components/cauldron.tsx`). A rail **above the
                conversation column** rather than floating in the middle of the
                page — where the séance's small spread sits, and for its
                reason: the person is reading this column, so this is where the
                room has to be. The plate commits to it too, since the cauldron
                sits at 66% across the picture.

                It reports `slots` and nothing else. A kind appearing that was
                not there on the last render is the grounding, which drops that
                slot's ingredient; three in and the brew is ready. Nothing here
                can count something the transcript does not carry. */}
            {pot && pot.ingredients.length > 0 && (
              <div className="space-y-2">
                <div role="img"
                     aria-label={`The pot. ${potLabel(pot.ingredients, inPot)}`}>
                  <Cauldron view="strip" ingredients={pot.ingredients}
                            grounded={inPot} serving={serving}
                            onBeat={setStirred}
                            onReady={() => setPulsing(true)} />
                </div>
                {/* The beat, printed. The folded phase has no room for the
                    plate's own caption, and a reveal that happens only in
                    colour is a reveal half the room misses. Same string the
                    live region says. */}
                {stirred && (
                  <p className="cauldron-beat-line">{stirred}</p>
                )}
              </div>
            )}
            {pulsing && !reducedMotion() && (
              <ReadyPulse onDone={() => setPulsing(false)} />
            )}
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
                              costume.bubble && t.role === 'assistant'
                                ? ` ${costume.bubble}` : ''}`}
                            style={t.role === 'user'
                              // The room's accent, not the app's: your own
                              // words wear the colour of whoever you are
                              // talking to (item 8).
                              ? { background: 'var(--room-accent, var(--series-1))',
                                  color: '#fff' }
                              : costume.bubble
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

            {report?.fact && <FactNote fact={report.fact} dressed={costume.note} />}

            {/* The scroll carries its own geometry: the deckle mask sets its
                silhouette and its padding (letters that reach the torn edge
                get torn with it), which is why the costume owns this whole
                class list rather than adding to one. */}
            <div className={costume.scroll}>
              <p className={`text-base leading-relaxed${
                   busy === 'asking' ? ' thinking-pulse' : ''}${
                   costume.question ? ` ${costume.question}` : ''}`}
                 style={{ color: busy === 'asking'
                   ? 'var(--text-muted)' : 'var(--text-primary)' }}>
                {busy === 'asking'
                  ? costume.thinking
                  : costume.ink && question
                    // The reader's words arrive as ink soaking into the
                    // page; everyone else's questions simply print.
                    ? <InkText text={question} />
                    : question || noQuestion
                    // A thrown turn sets no report, so there is no `reason` to
                    // show — and "Starting…" under a red error line describes
                    // the one thing that is definitely not happening.
                    || (stuck ? 'That question did not arrive.' : 'Starting…')}
              </p>
              {/* Gone once the conversation is over. A box that accepts
                  typing and returns the same closing sentence every time is
                  worse than no box: it reads as the person failing to say the
                  magic word. */}
              {!finished && (
              <>
              {/* A real label rather than a placeholder doing a label's work.
                  Invisible, because the question above it is the label as far
                  as the eye is concerned and a second one would be clutter —
                  but a placeholder is not a name, and a reader landing in this
                  box was being told nothing at all about what it is for. */}
              <label htmlFor={answerId} className="sr-only">
                Your answer
              </label>
              <textarea
                id={answerId}
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
                placeholder={costume.placeholder}
                className={`mt-3 w-full rounded-md px-3 py-2 text-sm${
                  costume.quill ? ` ${costume.quill}` : ''}`}
                style={costume.quill
                  ? undefined
                  : { background: 'var(--page)', color: 'var(--text-primary)',
                      border: '1px solid var(--hairline)' }}
              />
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <button onClick={answerIt} disabled={!answer.trim() || !!busy}
                        className="btn btn-primary btn-accent-1">
                  Answer
                </button>
                {/* The way out of a turn that produced nothing. Sits next to
                    Answer rather than replacing it, because the textarea is
                    still perfectly usable — this is the control for the case
                    where there is nothing to answer *yet*, which is the
                    opening turn most of the time. */}
                {stuck && (
                  <button onClick={retry}
                          className="btn btn-quiet">
                    <ReplayGlyph />
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
              </>
              )}

              {/* The end of the road, when the floor was never reached. The
                  reading cannot be given — `may_propose` counts grounded
                  quotes and there are not three of them — so the honest thing
                  is to say that plainly, put it on the reading rather than on
                  the person, and open the door that does still work.
                  Commandment 2: a newcomer must never be left holding a
                  disabled button as the last thing that happened. */}
              {finished && !ready && (
                <div className="mt-4 flex flex-wrap items-center gap-3">
                  <button onClick={onLeave}
                          className="btn btn-primary btn-accent-1">
                    {leaveLabel ?? 'Pick colours with me instead'}
                  </button>
                  <button onClick={startOver}
                          className="btn btn-quiet">
                    <ReplayGlyph />
                    Ask me again
                  </button>
                  <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                    Nothing here was wrong — the cards just did not settle on
                    enough to read from.
                  </span>
                </div>
              )}
            </div>

            {/* The short circuit, where the eye already is. The sidebar
                button lights up when the floor is met, but the person is
                reading the conversation column — so the conversation column
                is where "you can stop now" has to be said. */}
            {ready && busy !== 'proposing' && (
              <div className="ready-banner rounded-xl px-5 py-4">
                <p className="text-sm font-medium"
                   style={{ color: 'var(--text-primary)' }}>
                  {costume.ready}
                </p>
                <p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>
                  Keep talking if you’re enjoying it, or get your colours now.
                </p>
                {/* Worded apart from the sidebar's "Suggest my colours" —
                    two controls, one act, and a reader (or a test) should be
                    able to tell which one they pressed.

                    The room's own tailoring when it has any (`costume.action`),
                    and the app's accent button when it does not — a blank
                    costume field means the plain chrome, whose paint stays
                    here so an undressed room never has to restate the
                    default. */}
                <button onClick={proposeIt} disabled={!!busy}
                        className={`btn ${costume.action || 'btn-primary btn-accent-2'}`}
                        style={costume.action
                          ? undefined
                          : { '--btn-ink': '#fff' } as CSSProperties}>
                  {costume.readyAction}
                </button>
              </div>
            )}

            {/* The same argument the banner above it is made of: the person is
                reading the conversation column, so that is where the answer to
                "did my click do anything" has to be. The sidebar has carried
                this clock since the proposal became a job, and the sidebar is
                *below* the conversation on anything narrower than a laptop
                (`lg:grid-cols-…`) — so pressing the button made it vanish and
                put the only sign of life off the bottom of the screen, for the
                two to four minutes this takes. That is a working feature that
                looks exactly like a broken one. */}
            {busy === 'proposing' && (
              <div className="ready-banner rounded-xl px-5 py-4">
                <p className="text-sm font-medium thinking-pulse"
                   style={{ color: 'var(--text-primary)' }}>
                  {`${costume.reading} ${elapsed}s`}
                </p>
                <p className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>
                  A few minutes — it reads around and checks every card against
                  the pool. It carries on if you reload or close the tab.
                </p>
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
                {droppedNote(report.slots_dropped)}
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
                      className={`btn mt-3 w-full ${ready
                        ? costume.action || 'btn-primary btn-accent-2'
                        : 'btn-quiet'}`}
                      style={ready && !costume.action
                        ? { '--btn-ink': '#fff' } as CSSProperties
                        : undefined}>
                {/* The room's own words for the wait, not the app's. This
                    button and the banner in the column say the same thing at
                    the same moment; they used to say "Reading around…" and
                    "Reading the cards…" simultaneously, because one keyed on
                    the persona and the other on whether a spread had been
                    dealt. A room speaks in one voice. */}
                {busy === 'proposing'
                  ? `${costume.reading} ${elapsed}s`
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
              {/* `table`, because there is no deck here yet — this is the
                  surface somebody arrives at before there is one, and the
                  readout's default phrase named a thing the room does not
                  have. */}
              <StanceReadout status={status} pin={pin} surface="table" />
            </div>
          </aside>
        </div>
      )}

      {proposal && (
        <div className="space-y-4">
          {/* The room's own line for the moment the potion is handed over,
              above the app's promise rather than instead of it: the sentence
              below carries "nothing is created until you say so", which is
              commandment 2's and is not a costume's to soften. Said by the
              live region in these same words. */}
          {pot && (
            <p className="text-sm font-medium"
               style={{ color: 'var(--text-primary)' }}>
              {POURED}
            </p>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
              Pick a commander to carry on — you will name the deck next, and
              nothing is created until you say so.
            </p>
            <button onClick={() => setSaved((s) => ({ ...s, proposal: null }))}
                    className="btn btn-quiet btn-sm ml-auto">
              ← Keep talking
            </button>
          </div>

          {proposal.combinations.length === 0 && (
            <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
              {/* A run that produced nothing gets the costume's sentence —
                  `noQuestion`'s rule, applied to the pour. `asked: false` is
                  the server explaining itself in a person's words, and those
                  pass through untouched. */}
              {proposal.asked && proposal.reason
                ? costume.emptyReply
                : proposal.reason || costume.emptyReply}
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
    </>
  )
}
