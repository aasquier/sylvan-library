/**
 * Agatha's room: a pot already boiling, and three things waiting to go in.
 *
 * The witch's interview is the fortune-teller's séance one room over, and the
 * argument is the same one restated in a different material. Three ingredients
 * are picked from a seed before a word is said (`internal/brew`), one per slot
 * kind — `taste` is The Base, `temperament` is The Heat, `posture` is The
 * Binding, which are ADR 20's first three kinds unchanged. When the querent's
 * own words ground a slot, that slot's ingredient drops in and the brew
 * reacts. Three in and the brew is ready; the proposal is the potion, poured.
 *
 * **The pot reports the grounding. It never performs it.** It cannot count
 * anything the transcript does not already carry — the readiness floor, the
 * grounded-quote check and `may_propose` are all exactly where they were, and
 * this file reads their answer. The séance says the same thing about cards:
 * *the cards colour the questions; they are never mistaken for something
 * anybody said*.
 *
 * Three things about the construction, each a choice rather than a default.
 *
 * **Every layer is a SIBLING of the plate, never nested.** `mix-blend-mode`
 * blends against its stacking context, so a wrapper around two blended layers
 * blends them with *each other* and hands the plate a flat composite.
 * `.cauldron-pot` carries `isolation: isolate` so the stack blends against the
 * painting rather than against whatever the page has behind it. This is
 * `tarot.tsx`'s `SeanceRoom` note verbatim, and it is the same bug both times.
 *
 * **Four percentages are the only numbers that mean anything.** `--pot-x`,
 * `--pot-y`, `--pot-d` and `--pot-dy` are the cauldron's mouth as fractions of
 * the plate (index.css, under "Agatha's hut"), so the splash lands in the pot
 * at every window width instead of at one — and the folded strip is the *same*
 * composition cropped to a band around those numbers rather than a second
 * composition with a second measurement in it. Re-measure the four if the
 * plate is ever recut; nothing else moves.
 *
 * **Commandment 19: every `filter` here is on a layer we drew.** There is no
 * card art in this room at all — the witch's painting reaches the page only
 * through `SceneBackdrop`, credited in words in the same room — and the plate
 * is a committed public-domain painting whose treatment (the dusk) is a
 * stylesheet layer over it rather than a level baked into the bytes.
 *
 * Cycle times, for the walk (commandment 16 — nobody should stare at a hole
 * waiting for a snake that comes out once a minute): a bubble breaks every
 * 1.4–2.9s, a steam wisp sets off every 3.4–5.8s and takes 8.6–13s to cross
 * the frame, the fire breathes on 4.4s, the brew on 5.4s, the surface caustic
 * runs on 12.6s, the room's light on 8.3s, and the vortex — ready only — turns
 * once every 22s. **The fire is the one that is always going**: everything
 * else in this room is an event, and a painting with no idle beat in it is a
 * page that just sits there (commandment 6).
 */

import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import type { BrewIngredient } from '../lib/api'
import { STILL_TO_COME, stirredLine } from '../lib/cauldroncopy'
import { reducedMotion } from '../lib/motion'
import { VideoBackdrop } from './videofx'
import plateUrl from '../assets/agatha/agatha-hut-plate.webp'

/**
 * Where each place's ingredient hits the surface, as a fraction across the
 * mouth. Keyed on the slot kind because that is what the wire carries; the
 * names beside them are the pot's own (`brew`'s `Order`).
 *
 * Fixed per slot rather than random, for the tarot deal's reason — the same
 * table on a reload — and *offset* rather than centred for a different one: a
 * pot that splashes dead centre three times is a progress bar wearing a
 * cauldron.
 *
 * **These are render coordinates and they stay client-side.** They never go on
 * the wire (commandment 10: no seeds, no wire tokens, and no layout numbers
 * either). A slot kind with no entry lands in the middle, which is what an
 * ADR 20 kind this room was not built for should do.
 */
const LANDING: Record<string, number> = {
  taste: 0.38, // The Base
  temperament: 0.58, // The Heat
  posture: 0.47, // The Binding
}

/** The fall, and the beat the colour steps on. The brew changes AT the
 *  landing, never before it: a pot that turns while the herb is still in the
 *  air is a state machine rather than a pot. */
const FALL_MS = 900
/** The ready beat lands a breath after the third bloom, because the climax is
 *  the colour arriving and stacking a vortex on top of it hides it.
 *  `tarot.tsx`'s `waited`/`settled` wait with a shorter fuse. */
const READY_MS = 1500

/* --------------------------------------------------------------- the herbs
 *
 * Thin-stroked botanical silhouettes rather than illustrations: a herbal
 * plate's line, dark with the brew's own light on its edge, and things
 * falling through a dark room rather than pictures of plants (commandment 5 —
 * no clip art, and a vector cartoon of a leaf is exactly that).
 *
 * **Drawn per PLACE, not per ingredient**, and that is the honest version
 * rather than the cheap one. The shelf holds twenty-four things and nobody is
 * drawing twenty-four; what the socket is a socket *for* is the place, the
 * place is what is named under it whether it is filled or not, and the
 * ingredient's own name arrives in words the moment it is in. A glyph that
 * claimed to be a specific herb while standing for eight of them would be the
 * one lie in the room.
 *
 * So: The Base is a hip cluster on its stem — the fruit left after the flower,
 * which is what a base is. The Heat is a hooded spike, the shape every
 * poisonous thing in a hedgerow shares. The Binding is a three-leaved spray
 * whose leaves overlap, because a binding is what holds the other two
 * together.
 */
export function HerbGlyph({ slot, className }: {
  slot: string
  className?: string
}) {
  const stroke = {
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.7,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
  }
  const body = slot === 'temperament'
    ? (
      /* The Heat: a hooded spike. */
      <>
        <path d="M20 36 L 20 15" {...stroke} />
        <path d="M20 15 C 13 15 11 10 13 5 C 16 2 20 4 20 8 C 20 4 24 2 27 5 C 29 10 27 15 20 15 Z"
              {...stroke} fill="currentColor" fillOpacity="0.6" />
        <path d="M20 23 C 15 23 13 20 13 17" {...stroke} />
        <path d="M20 29 C 26 29 28 26 28 23" {...stroke} />
      </>
      )
    : slot === 'posture'
      ? (
        /* The Binding: three leaves, overlapping. */
        <>
          <path d="M20 36 L 20 6" {...stroke} />
          <path d="M20 28 C 12 28 9 23 9 18 C 15 18 19 22 20 28 Z"
                {...stroke} fill="currentColor" fillOpacity="0.55" />
          <path d="M20 22 C 28 22 31 17 31 12 C 25 12 21 16 20 22 Z"
                {...stroke} fill="currentColor" fillOpacity="0.45" />
          <path d="M20 15 C 13 15 11 11 11 7 C 16 7 19 10 20 15 Z"
                {...stroke} fill="currentColor" fillOpacity="0.35" />
        </>
        )
      : (
        /* The Base: hips on the stem. */
        <>
          <path d="M20 34 C 20 26 20 20 20 14" {...stroke} />
          <path d="M20 20 C 14 19 11 15 12 10" {...stroke} />
          <path d="M20 24 C 26 23 29 19 28 14" {...stroke} />
          <circle cx="13" cy="9" r="4.4" {...stroke} fill="currentColor"
                  fillOpacity="0.8" />
          <circle cx="27" cy="13" r="3.8" {...stroke} fill="currentColor"
                  fillOpacity="0.65" />
          <circle cx="20" cy="6" r="3.2" {...stroke} fill="currentColor"
                  fillOpacity="0.5" />
        </>
        )
  return (
    <svg viewBox="0 0 40 40" className={className} aria-hidden="true">
      {body}
    </svg>
  )
}

/* --------------------------------------------------------------- the boil */

interface Bubble { id: number; x: number; y: number; size: number; dur: number; wobble: number }
interface Wisp { id: number; x: number; size: number; drift: number; dur: number; peak: number }

/** Two bubbles and a wisp, parked at a frame that reads. Under reduced motion
 *  the room stays and only the weather stills — a surface with nothing on it
 *  stops reading as liquid, which is a thing withheld rather than a thing
 *  calmed. */
const HELD_BUBBLES: Bubble[] = [
  { id: -1, x: 62, y: 35.2, size: 9, dur: 0, wobble: 0 },
  { id: -2, x: 70.5, y: 36.8, size: 6, dur: 0, wobble: 0 },
]
const HELD_WISP: Wisp[] = [{ id: -1, x: 66, size: 120, drift: 0, dur: 0, peak: 0.18 }]

/**
 * The simmer: bubbles and steam, spawned rather than looped.
 *
 * A `setTimeout` chain rather than `setInterval`, so no two are on the same
 * clock and the boil never visibly repeats — and every pending timer is
 * cleared on the way out, `tarot.tsx`'s `timers.current` pattern for its own
 * reason: a bubble landing in an unmounted tree is a console warning and a
 * state update nobody wanted.
 *
 * `live` is the element being on screen. The plate pauses its own decoder off
 * screen (`videofx.tsx`), and a boil nobody can see should stop with it.
 */
function useSimmer(ready: boolean, still: boolean, live: boolean) {
  const [bubbles, setBubbles] = useState<Bubble[]>([])
  const [wisps, setWisps] = useState<Wisp[]>([])
  const timers = useRef<number[]>([])
  const next = useRef(0)

  useEffect(() => {
    // Under reduced motion nothing is spawned at all; the held pair below is
    // chosen during render rather than written into state, which keeps this
    // effect a subscription to an external clock and nothing else.
    if (still) return
    if (!live) return
    const pending = timers.current
    const later = (fn: () => void, ms: number) => {
      pending.push(window.setTimeout(fn, ms))
    }
    const blow = () => {
      // Somewhere on the mouth ellipse, favouring the middle: `sqrt(random)`
      // for the radius, because a boil is busiest where the heat is and the
      // rim is where bubbles go to die.
      const angle = Math.random() * Math.PI * 2
      const radius = Math.sqrt(Math.random()) * 0.86
      const id = next.current++
      // Half of `--pot-d` and half of `--pot-dy`, in the plate's own
      // percentages. The four numbers again; there is no fifth here either.
      setBubbles((live) => [...live.slice(-5), {
        id,
        x: 66 + Math.cos(angle) * radius * 11.5,
        y: 36 + Math.sin(angle) * radius * 3.27,
        size: 4 + Math.random() * (ready ? 11 : 8),
        dur: 1400 + Math.random() * 1500,
        wobble: Math.random() * 8 - 4,
      }])
      later(() => setBubbles((live) => live.filter((b) => b.id !== id)), 3200)
      later(blow, (ready ? 700 : 1400) + Math.random() * 1500)
    }
    // **Thinner, slower and further apart than the first cut.** The painting
    // has its own smoke in it, and at their first weight the wisps read as
    // white smudges laid over Waterhouse's rather than as steam rising through
    // it — a thing the eye finds sitting still rather than a thing it catches
    // moving. The peaks are down by about two fifths, the crossing takes half
    // again as long, and they set off further apart so the same two or three
    // are on the plate at once rather than a queue of them. The removal timer
    // is derived from the longest crossing plus a beat, because a wisp cleared
    // before its animation ends is a wisp that vanishes mid-frame.
    const steam = () => {
      const id = next.current++
      const size = (ready ? 96 : 66) + Math.random() * 60
      setWisps((live) => [...live.slice(-2), {
        id,
        x: 66 + (Math.random() * 22 - 11),
        size,
        drift: Math.random() * 120 - 40,
        dur: 8600 + Math.random() * 4400,
        peak: ready ? 0.43 : 0.25,
      }])
      later(() => setWisps((live) => live.filter((w) => w.id !== id)), 13600)
      later(steam, (ready ? 2400 : 3400) + Math.random() * 2400)
    }
    blow()
    steam()
    return () => {
      pending.forEach(clearTimeout)
      timers.current = []
    }
  }, [ready, still, live])

  // Two bubbles and one wisp, held at a frame that reads. "Stilled" and "held
  // at its first keyframe" are not the same thing, and a surface with nothing
  // on it stops reading as liquid.
  return still
    ? { bubbles: HELD_BUBBLES, wisps: HELD_WISP }
    : { bubbles, wisps }
}

/** Whether this element is on screen. Answers `true` where there is no
 *  observer to ask (jsdom), because a boil that never starts is a worse
 *  failure than one that runs behind a scrolled-away panel. */
function useOnScreen(node: React.RefObject<HTMLElement | null>) {
  const [seen, setSeen] = useState(true)
  useEffect(() => {
    const el = node.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const watch = new IntersectionObserver((entries) => {
      for (const entry of entries) setSeen(entry.isIntersecting)
    })
    watch.observe(el)
    return () => watch.disconnect()
  }, [node])
  return seen
}

/* ------------------------------------------------------------- the landing */

/** One thing going in, as the surface answers it. */
interface Splash { id: number; hit: number; level: number }
/** One thing still in the air. */
interface Toss { id: number; hit: number; slot: string; name: string; spin: number }

/** The mouth's centre, in plate percentages, offset by where this thing
 *  landed across it. `--pot-x` is 66% and `--pot-d` is 23%, so a landing at
 *  0.38 of the mouth is 66 - 0.12 × 23 = 63.2%. Written here rather than in
 *  CSS because it is one number per drop, and `calc()` would need the
 *  fraction on a custom property to do the same arithmetic. */
function mouthX(hit: number): number {
  return 66 + (hit - 0.5) * 23
}
/** `--pot-y`, as a bare number, for the same reason. */
const MOUTH_Y = 36

/* ----------------------------------------------------------------- the pot */

export interface CauldronProps {
  /** `room` is the full 16:9 hut the querent arrives in; `strip` is the same
   *  composition cropped to a band around the mouth, which is what rides above
   *  the conversation once the talking starts. One geometry, two windows. */
  view: 'room' | 'strip'
  /** The three things the seed picked, in pot order. */
  ingredients: BrewIngredient[]
  /** Which slot kinds the transcript has grounded, in the order they landed.
   *  The pot reads this and reports it; it never decides it. */
  grounded: string[]
  /** The pour is running. */
  serving?: boolean
  /** Every beat, in the room's own words, so the caller can print it and say
   *  it in one place. Fires when something lands, never for anything an eye
   *  cannot also read. */
  onBeat?: (line: string) => void
  /** The pot has come up green and settled — a breath after the third bloom,
   *  because the climax is the colour arriving. Nothing on the plate says the
   *  word; this is how the conversation column gets to. Does not fire for a
   *  pot that arrived full from a stash: there is nothing to announce about a
   *  thing that happened on a previous visit. */
  onReady?: () => void
  /** The line printed across the foot of the plate. `room` only, and it is
   *  the same string the live region says — never a second wording of it. */
  caption?: string
  /** The room's own chrome, riding the dark upper corners. `room` only. */
  children?: ReactNode
}

export function Cauldron({
  view, ingredients, grounded, serving = false, onBeat, onReady, caption,
  children,
}: CauldronProps) {
  const host = useRef<HTMLDivElement>(null)
  // Read once per mount, the deal `videofx.tsx` offers: a live change of the
  // OS setting takes effect on the next navigation.
  const [still] = useState(() => reducedMotion())
  const onScreen = useOnScreen(host)

  /** What is actually in the pot: an ingredient whose slot the transcript has
   *  grounded, in the order the pot took them. `anchor` grounds too and has no
   *  ingredient, which is why this is a filter rather than a count. */
  const inPot = ingredients.filter((i) => grounded.includes(i.slot))
  const level = inPot.length

  // **A conversation restored from a stash must not replay three drops.**
  // `tarot.tsx`'s `turnedHere` is the precedent: ingredients present at mount
  // are seated, the room opens at that level with no animation, and only a
  // grounding that happens here, in front of somebody, gets its arc.
  const seen = useRef<string[] | null>(null)
  const [tosses, setTosses] = useState<Toss[]>([])
  const [splashes, setSplashes] = useState<Splash[]>([])
  const [ready, setReady] = useState(false)
  const timers = useRef<number[]>([])
  const next = useRef(0)
  useEffect(() => () => { timers.current.forEach(clearTimeout) }, [])

  // The callbacks through refs, so `drop` below can stay stable while the
  // caller re-creates its handlers every render. Assigned in an effect rather
  // than during render, because a render is not allowed to have effects.
  const beat = useRef(onBeat)
  const rang = useRef(onReady)
  useEffect(() => {
    beat.current = onBeat
    rang.current = onReady
  })

  const drop = useCallback((ingredient: BrewIngredient, count: number, arc: boolean) => {
    const id = next.current++
    const hit = LANDING[ingredient.slot] ?? 0.5
    const land = () => {
      setSplashes((live) => [...live, { id, hit, level: count }])
      timers.current.push(window.setTimeout(
        () => setSplashes((live) => live.filter((s) => s.id !== id)), 2800))
      beat.current?.(stirredLine(ingredient, count))
    }
    if (!arc) {
      land()
      return
    }
    setTosses((live) => [...live, {
      id, hit, slot: ingredient.slot, name: ingredient.name,
      // A deterministic tumble per drop rather than a random one: a hand
      // wobbles, a render must not (`InkText`'s rule, one room over).
      spin: 140 + (id * 53) % 160,
    }])
    timers.current.push(window.setTimeout(() => {
      setTosses((live) => live.filter((t) => t.id !== id))
      land()
    }, FALL_MS))
  }, [])

  useEffect(() => {
    const kinds = inPot.map((i) => i.slot)
    const before = seen.current
    seen.current = kinds
    if (before === null) {
      // Seated: whatever was already in when this window opened.
      if (kinds.length >= ingredients.length && ingredients.length > 0) setReady(true)
      return
    }
    const fresh = inPot.filter((i) => !before.includes(i.slot))
    if (fresh.length === 0) return
    fresh.forEach((ingredient, n) => {
      const count = before.length + n + 1
      drop(ingredient, count, !still)
    })
    if (kinds.length >= ingredients.length && ingredients.length > 0) {
      timers.current.push(window.setTimeout(() => {
        setReady(true)
        rang.current?.()
      }, still ? 200 : FALL_MS + READY_MS))
    }
    // `inPot` is derived from the two arrays in the list; listing it as well
    // would re-run this on every render, which is how a pot replays a drop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [grounded, ingredients, still, drop])

  const { bubbles, wisps } = useSimmer(ready, still, onScreen)

  const scene = (
    <>
      {/* LAYER 0 · the plate. `art` mode, not `ambience`: this is a painting
          brought to life, so the still is the floor and the loop is what
          plays over it — remove it and the ingredients fall into nothing.
          Today there is no loop and `VideoBackdrop` answers with the
          fallback, which is exactly the shape the footage drops into: add
          `webmSrc`/`mp4Src` and nothing else on this page moves. */}
      <VideoBackdrop mode="art" className="cauldron-plate" poster={plateUrl}
                     fallback={<img className="cauldron-plate" src={plateUrl}
                                    alt="" aria-hidden />} />
      {/* LAYER 0b · the dusk. The painting is daylit — mean luma 104 against
          the séance room's 52 — and it is held back HERE rather than in the
          file, `fabrica.recipe.yaml`'s rule: a level baked into the byte
          stream is a decision the stylesheet can no longer argue with. Open
          over the cauldron, closing to dusk at the frame's edges. */}
      <span className="cauldron-dusk" aria-hidden="true" />
      {/* LAYER 0c · the fire, and it is the room's idle heartbeat. A
          photograph of a painting does not move, and every other beat here is
          an event somebody caused; the embers are the one thing that would
          never be still whether anybody was in the room or not. Anchored on
          the fire itself (67.8% / 52.6% of the plate) rather than on the
          mouth — the two are different hotspots and this is the only rule
          that reads the second one. */}
      <span className="cauldron-embers" aria-hidden="true" />
      {/* LAYERS 1-4 · siblings, every one. The dim is the layer the whole
          effect turns on: the mouth's interior is multiplied down to a warm
          near-black before anything is put into it (`.seance-glass-dim`'s
          argument, one room over) — but not all the way down, or the painted
          rim light goes with it and the brew has to supply every photon in
          the pot, which is how it came out the colour of milk. */}
      <span className="cauldron-dim" aria-hidden="true" />
      <span className="cauldron-brew" aria-hidden="true" />
      <span className="cauldron-caustic" aria-hidden="true" />
      {/* The one highlight that says the surface is WET, lying on the ember
          side of the mouth because that is where the light in this room comes
          from. */}
      <span className="cauldron-gloss" aria-hidden="true" />
      <span className="cauldron-vortex" aria-hidden="true" />
      <span className="cauldron-glow" aria-hidden="true" />
      <div className="cauldron-steam" aria-hidden="true">
        {wisps.map((w) => (
          <span key={w.id} className="cauldron-wisp" style={{
            left: `${w.x}%`,
            width: `${w.size}px`,
            height: `${w.size * 0.72}px`,
            '--wisp-drift': `${w.drift}px`,
            '--wisp-dur': `${w.dur}ms`,
            '--wisp-peak': w.peak,
          } as React.CSSProperties} />
        ))}
      </div>
      {/* The masked host: everything that happens IN the liquid lives here, so
          a ring dies against the pot wall instead of running out across the
          hut. The mask is the same four percentages as a gradient. */}
      <div className="cauldron-surface" aria-hidden="true">
        {bubbles.map((b) => (
          <span key={b.id} className="cauldron-bubble" style={{
            left: `${b.x}%`,
            top: `${b.y}%`,
            width: `${b.size}px`,
            height: `${b.size * 0.78}px`,
            '--bubble-dur': `${b.dur}ms`,
            '--bubble-wobble': `${b.wobble}px`,
          } as React.CSSProperties} />
        ))}
        {splashes.map((s) => (
          <Splashing key={s.id} hit={s.hit} level={s.level} />
        ))}
      </div>
      {/* The unmasked host: what happens ABOVE the pot. The falling thing is
          not clipped by the surface, and neither is the wash the bloom throws
          off the pot and onto the hut. */}
      <div className="cauldron-air" aria-hidden="true">
        {tosses.map((t) => (
          <Falling key={t.id} hit={t.hit} slot={t.slot} name={t.name}
                   spin={t.spin} />
        ))}
        {splashes.map((s) => (
          <span key={s.id} className="cauldron-wash" style={{
            left: `${mouthX(s.hit)}%`,
            '--bloom-to': `var(--brew-${s.level})`,
          } as React.CSSProperties} />
        ))}
      </div>
      {/* The pour: a ladle of light drawn up out of the surface, and a vial
          taking that light as it rises clear. */}
      <div className="cauldron-serve" aria-hidden="true">
        <span className="cauldron-ladle" />
        <span className="cauldron-vial">
          <svg viewBox="0 0 44 88" aria-hidden="true">
            <ellipse className="cauldron-vial-halo" cx="22" cy="58" rx="17" ry="22" />
            <path className="cauldron-vial-fill"
                  d="M13 40 h18 v26 a9 9 0 0 1 -9 9 h0 a9 9 0 0 1 -9 -9 z" />
            <path className="cauldron-vial-glass"
                  d="M15 8 h14 v32 l2 4 v22 a11 11 0 0 1 -11 11 h0 a11 11 0 0 1 -11 -11 v-22 l2 -4 z" />
            <path className="cauldron-vial-glass" d="M12 8 h20" />
          </svg>
        </span>
      </div>
    </>
  )

  // **One element per window carries the state**, and the default `--brew` is
  // declared against the same attribute at the same specificity rather than on
  // the plate — an element's own declaration always beats the one it would
  // inherit, so a `--brew` sitting on the inner stage would silently outrank
  // the level on the strip and the folded pot would simmer umber forever while
  // the full-size one went green. (`an-undefined-custom-property-inherits-
  // silently`, with the polarity flipped: here the property is defined, in the
  // wrong place.)
  const state = `cauldron-window${ready ? ' is-ready' : ''}`
    + `${serving ? ' is-serving' : ''}`

  if (view === 'strip') {
    return (
      <div className={`${state} cauldron-strip`} data-level={level}>
        {/* The band. The same 16:9 room, magnified and pushed so the mouth
            lands on the window's midline — derived from the four percentages,
            never from a second measurement. */}
        <div className="cauldron-strip-window">
          <div ref={host} className="cauldron-pot cauldron-strip-stage">
            {scene}
          </div>
        </div>
        <CauldronSockets ingredients={ingredients} grounded={grounded} />
      </div>
    )
  }

  return (
    <div className={`${state} cauldron-room`} data-level={level}>
      <div ref={host} className="cauldron-pot">{scene}</div>
      {children}
      {caption && (
        /* The caption the eye reads. Keyed on the text so a new line rises in
           rather than swapping in place — and `aria-hidden`, because the live
           region above the interview says this exact string and a reader
           hearing it twice is a reader being told it twice. */
        <p className="cauldron-beat" aria-hidden="true">
          <span key={caption}>{caption}</span>
        </p>
      )}
    </div>
  )
}

/** How far left of the landing point a thing is thrown from, and how far
 *  above the frame, as percentages of the plate. */
const THROW_X = 17
const THROW_Y = 6

/**
 * The thing falling in, and the physics is the nesting.
 *
 * The outer element translates X at a constant rate and the inner falls on an
 * ease-in. One element cannot be both, and a single eased diagonal is a slide
 * rather than an arc — which is the difference between something being thrown
 * into a pot and something sliding down a ramp into one.
 *
 * Both moving elements are **plate-sized boxes** (`inset: 0`), which is the
 * one detail here that is easy to get wrong and invisible when you do: a
 * percentage in `translate` resolves against the element's *own* box, so a
 * token-sized box travelling `17%` would travel seventeen per cent of a
 * thirty-eight-pixel glyph. Sized to the plate, the arc is the same arc at
 * every window width, which is the whole point of the hotspot contract.
 */
function Falling({ hit, slot, name, spin }: {
  hit: number
  slot: string
  name: string
  spin: number
}) {
  return (
    <span className="cauldron-toss" style={{
      '--toss-x': `${THROW_X}%`,
      '--toss-y': `${MOUTH_Y + THROW_Y}%`,
    } as React.CSSProperties}>
      <span className="cauldron-toss-x">
        <span className="cauldron-toss-y">
          <span className="cauldron-token" style={{
            left: `${mouthX(hit) - THROW_X}%`,
            top: `${-THROW_Y}%`,
            '--toss-spin': `${spin}deg`,
          } as React.CSSProperties}>
            <HerbGlyph slot={slot} className="cauldron-token-glyph" />
            <span className="cauldron-token-name">{name}</span>
          </span>
        </span>
      </span>
    </span>
  )
}

/**
 * The surface answering a weight: a collar of liquid standing up where the
 * thing went in, eight beads out of it, three rings, and the colour.
 *
 * The collar is the beat that sells the weight and the two eases on the beads
 * are the whole trick — out is fast and decelerating, back is slow and
 * accelerating, which is gravity in two curves. Their Y throw is squashed to
 * a third of their X throw because the surface is seen at a shallow angle.
 */
function Splashing({ hit, level }: { hit: number; level: number }) {
  const x = mouthX(hit)
  const place = {
    '--hit-x': `${x}%`,
    '--hit-y': `${MOUTH_Y}%`,
  } as React.CSSProperties
  return (
    <>
      <span className="cauldron-collar" style={place} />
      <span className="cauldron-crown" style={place}>
        {Array.from({ length: 8 }, (_, i) => {
          const angle = (i / 8) * Math.PI * 2
          const throwTo = 24 + ((i * 7) % 5) * 6
          return (
            <span key={i} className="cauldron-bead" style={{
              '--bead-x': `${(Math.cos(angle) * throwTo).toFixed(1)}px`,
              '--bead-y': `${(Math.sin(angle) * throwTo * 0.34).toFixed(1)}px`,
              animationDelay: `${(i * 13) % 60}ms`,
            } as React.CSSProperties} />
          )
        })}
      </span>
      {[0, 1, 2].map((i) => (
        <span key={i} className="cauldron-ripple" style={{
          ...place,
          '--ripple-dur': `${1400 + i * 460}ms`,
          '--ripple-delay': `${i * 170}ms`,
        } as React.CSSProperties} />
      ))}
      <span className="cauldron-bloom" style={{
        ...place,
        '--bloom-to': `var(--brew-${level})`,
      } as React.CSSProperties} />
    </>
  )
}

/* ------------------------------------------------------------- the sockets */

/**
 * Three places, one per slot, in pot order.
 *
 * **Not a control, and it must not be dressed as one** — commandment 20's
 * test, both halves: it does not take you anywhere *and* it does not change
 * what is in front of you. It is a state readout.
 *
 * Empty, a newcomer can still see that there are three places, what each one
 * is called, and that one is still to come. Filled, the ring goes solid
 * copper, the glyph takes the brew's light and the ingredient's name arrives
 * under the place's — in words, so the state is legible without colour and
 * without a pointer to rest (`components/hint.tsx`'s rule: hover-only is half
 * the room). The `note` rides along as the socket's `title`: flavour for a
 * pointer that stops, never a thing you must hover to understand.
 */
export function CauldronSockets({ ingredients, grounded }: {
  ingredients: BrewIngredient[]
  grounded: string[]
}) {
  return (
    <ul className="cauldron-sockets">
      {ingredients.map((i) => {
        const filled = grounded.includes(i.slot)
        return (
          <li key={i.key}
              className={`cauldron-socket${filled ? ' is-in' : ''}`}
              title={filled ? `${i.name} — ${i.note}` : undefined}>
            <span className="cauldron-socket-ring">
              <HerbGlyph slot={i.slot} className="cauldron-socket-glyph" />
            </span>
            <span className="cauldron-socket-place">{i.position}</span>
            <span className="cauldron-socket-herb">
              {filled ? i.name : STILL_TO_COME}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

/**
 * The ready beat, played wide rather than tall.
 *
 * The folded pot deliberately does **not** grow back for it: a panel that
 * reflows mid-conversation moves the text somebody is reading, and the ready
 * moment is already carried by four changes at once — the brew at its vivid
 * stop, the rim light at full, the vortex starting, the steam thickening. So
 * the payoff goes wide: the room behind the conversation takes one slow pulse
 * of the brew's green, on the same beat. Motion, no reflow.
 *
 * Portalled onto the body for `SceneBackdrop`'s own reason: the routed page is
 * wrapped in a transform, and a transformed ancestor is the containing block
 * for every `position: fixed` descendant, so rendered in place this would
 * resolve `inset: 0` against a panel.
 */
export function ReadyPulse({ onDone }: { onDone: () => void }) {
  if (typeof document === 'undefined') return null
  return createPortal(
    <span className="cauldron-ready-pulse" aria-hidden="true"
          onAnimationEnd={onDone} />,
    document.body,
  )
}
