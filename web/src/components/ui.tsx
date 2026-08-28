import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { COLOR_NAMES, COLOR_VAR, manaSymbols, splitManaText, symbolName } from '../lib/mtg'
import { ManaGlyph, OfficialSymbol } from './manasymbol'
import { hasGlyph } from '../lib/managlyphs'
import { SceneBackdrop, type RoomMood } from './forest'
import { VideoBackdrop } from './videofx'

/* ------------------------------------------------------------------ pips */

/**
 * One mana symbol. Shared so a cost and a pip in prose cannot drift apart.
 *
 * Every pip is the official symbol now (ADR 33) — the real tap arrow, the
 * real hybrid split disc, the numeral in its proper circle — served from our
 * own origin. The coloured disc underneath is both the loading placeholder
 * and the fallback's canvas: with the cache cold and no network, the five
 * colours fall back to the hand-drawn sun/drop/skull/flame/tree and
 * everything else to its character, which is exactly what every pip was
 * before this ADR.
 */
function Pip({ sym, size }: { sym: string; size: number }) {
  const name = symbolName(sym)
  return (
    <span
      title={name}
      // The name has to be stated: a drawn pip contributes nothing to the
      // accessibility tree, and since ADR 33 every pip is a drawing — the
      // numeral in `{3}` included. `img` rather than nothing, because a pip
      // is one indivisible mark and a screen reader should say "Green"
      // rather than walking into an image.
      role="img"
      aria-label={name}
      className="inline-flex items-center justify-center overflow-hidden rounded-full align-middle font-semibold text-[#141414]"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.62,
        background: COLOR_VAR[sym] ?? 'var(--mtg-c)',
        // A ring keeps adjacent pips from merging into one blob, which is the
        // 2px surface-gap rule applied to inline marks.
        boxShadow: '0 0 0 1px var(--hairline)',
      }}
    >
      <OfficialSymbol
        symbol={sym}
        size={size}
        // The drawn glyph keeps its slight inset so the ink has a ring of
        // colour around it; the official image carries its own margins.
        fallback={hasGlyph(sym)
          ? <ManaGlyph symbol={sym} size={size * 0.74} />
          : sym.replace('/', '')}
      />
    </span>
  )
}

export function ManaCost({ cost, size = 16 }: { cost?: string | null; size?: number }) {
  const symbols = manaSymbols(cost)
  if (!symbols.length) return null
  return (
    <span className="inline-flex items-center gap-[2px] align-middle">
      {symbols.map((sym, i) => <Pip key={i} sym={sym} size={size} />)}
    </span>
  )
}

/**
 * Prose with its mana symbols drawn rather than spelled.
 *
 * The deck files carry 174 of them across `why`, `strategy` and `notes` — "a
 * {G}{W} activation" reads as punctuation noise until the pips are pips. The
 * gate's own messages get it too, which is where it pays best: "identity {GW}
 * includes {W}, outside the commander's {G}" is the single densest sentence in
 * the app, and it is the one people have to act on.
 *
 * Only recognised symbols are converted. A stray `{note}` stays literal text —
 * turning arbitrary braces into pips would be the tool asserting something
 * about prose it did not understand.
 */
export function ManaText({ children, size = 13 }: {
  children?: string | null
  size?: number
}) {
  if (!children) return null

  // Consecutive pips are one cost, so they wrap as a unit. Left as loose
  // siblings, a line break lands inside {1}{G}{W} and strands the {W} at the
  // start of the next line, where it reads as a bullet rather than as part of
  // the cost -- which is what the first render of this actually did.
  const runs: (string | string[])[] = []
  for (const part of splitManaText(children)) {
    const last = runs[runs.length - 1]
    if (part.pip && Array.isArray(last)) last.push(part.text)
    else runs.push(part.pip ? [part.text] : part.text)
  }

  return (
    <>
      {runs.map((run, i) =>
        Array.isArray(run)
          ? (
            <span key={i} className="whitespace-nowrap" style={{ marginInline: 1 }}>
              {run.map((sym, j) => <Pip key={j} sym={sym} size={size} />)}
            </span>
            )
          : <span key={i}>{run}</span>,
      )}
    </>
  )
}

/**
 * A colour identity at a size you are meant to notice, each colour lettered.
 *
 * The loud counterpart to `ColorPips`, which is 12px dots for a dense list.
 * This is for the places where the identity *is* the headline — the deck
 * hero and the builder's carousel — and it is shared between them rather than
 * written twice, for the same reason `Pip` is shared: two copies of the same
 * mark drift, and MTG's colours are fixed semantics that must not.
 */
export function ColorRing({ colors, size = 34 }: { colors: string[]; size?: number }) {
  if (!colors.length) {
    return (
      <span
        title="Colourless"
        role="img"
        aria-label="Colourless"
        className="inline-flex items-center justify-center overflow-hidden rounded-full font-semibold"
        style={{
          width: size, height: size, fontSize: size * 0.4,
          background: 'var(--mtg-c)', color: '#141414',
          boxShadow: '0 0 0 1px var(--hairline)',
        }}
      >
        <OfficialSymbol symbol="C" size={size} fallback="C" />
      </span>
    )
  }
  return (
    <span className="inline-flex items-center" style={{ gap: 2 }}>
      {colors.map((c) => (
        <span
          key={c}
          title={COLOR_NAMES[c]}
          role="img"
          aria-label={COLOR_NAMES[c] ?? c}
          className="inline-flex items-center justify-center overflow-hidden rounded-full font-semibold"
          style={{
            width: size, height: size, fontSize: size * 0.45,
            background: COLOR_VAR[c], color: '#141414',
            boxShadow: '0 0 0 1px var(--hairline)',
          }}
        >
          {/* The identity headline, so it gets the real icons for the same
              reason `Pip` does. `title` still carries the colour's name, which
              is what a drawing cannot say to a screen reader. */}
          <OfficialSymbol
            symbol={c}
            size={size}
            fallback={hasGlyph(c) ? <ManaGlyph symbol={c} size={size * 0.7} /> : c}
          />
        </span>
      ))}
    </span>
  )
}

/**
 * A colour identity at list density — the deck shelf, a table row.
 *
 * These were bare dots, which was defensible while a `Pip` was a lettered disc
 * (a 12px letter is unreadable, so a dot said strictly more). Now that a pip
 * carries the real icon the dots are the odd mark out, and a shelf where every
 * deck shows two pale circles is not saying which two colours. 14px is the
 * floor found by checking the glyphs down the sizes: the five stay apart by
 * silhouette well below where their interior detail gives up.
 */
export function ColorPips({ identity }: { identity: string[] }) {
  if (!identity.length) {
    return <span className="text-xs" style={{ color: 'var(--text-muted)' }}>colorless</span>
  }
  return (
    <span className="inline-flex gap-[3px]">
      {identity.map((c) => (
        <span
          key={c}
          title={COLOR_NAMES[c] ?? c}
          role="img"
          aria-label={COLOR_NAMES[c] ?? c}
          className="inline-flex h-3.5 w-3.5 items-center justify-center overflow-hidden rounded-full"
          style={{ background: COLOR_VAR[c], boxShadow: '0 0 0 1px var(--hairline)' }}
        >
          {/* 14 matches h-3.5; the official image fills the disc it draws. */}
          <OfficialSymbol
            symbol={c}
            size={14}
            fallback={hasGlyph(c) ? <ManaGlyph symbol={c} size={10} /> : null}
          />
        </span>
      ))}
    </span>
  )
}

/* --------------------------------------------------------------- controls */

/**
 * `help` is a node rather than a string, and that is a dependency decision.
 *
 * The affordance that renders it is `<HelpTip>`, which reads the glossary and
 * therefore imports `ManaText` from this file. Taking the rendered node from
 * the caller keeps the arrow pointing one way: these controls stay ignorant of
 * the glossary, and nothing here has to import the thing that imports it.
 *
 * It sits inside the `<label>` but outside the label text, so a click on the
 * word still focuses the field and a click on the mark does not.
 */
interface SelectProps {
  label: string
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  className?: string
  help?: React.ReactNode
}

/**
 * **`max-w-full` on both halves, and it is a phone fix rather than tidiness.**
 * A `<select>` is as wide as its widest `<option>` and will not wrap, so the
 * deck picker on `/simulate` — whose options are deck names — measured 384px
 * inside a 293px column and pushed 49px of horizontal scroll onto a 375px
 * phone. Every route that scrolls sideways does it for somebody else's
 * element; this one does it for a control the page cannot see the end of.
 *
 * The cap belongs on the `<label>`, and that is the half that is easy to get
 * wrong. These sit in `flex flex-wrap` filter rows, where a flex item's
 * automatic minimum size is its content — so a label capped at nothing still
 * refuses to shrink below the widest option. A definite `max-width` clamps
 * that automatic minimum (Flexbox §4.5), which is why `max-w-full` here does
 * the work that `min-w-0` is usually reached for, without `min-w-0`'s habit of
 * crushing a row that should have wrapped. The `<select>`'s own cap is for the
 * blocks these are used in outside a flex row, where the label is not the
 * thing being measured. Wrapping is untouched: a flex line still breaks on
 * base sizes, so a row of six short filters wraps exactly as before and only a
 * control too wide for a line of its own ever shrinks.
 */
export function Select({
  label, value, onChange, options, className = '', help,
}: SelectProps) {
  return (
    <label className={`flex max-w-full flex-col gap-1 ${className}`}>
      <span className="flex items-center text-[11px] font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
        {label}{help}
      </span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-9 max-w-full rounded-md px-2 text-sm outline-none focus:ring-2"
        style={{
          background: 'var(--surface-1)',
          color: 'var(--text-primary)',
          border: '1px solid var(--hairline)',
        }}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}

export function NumberField({
  label, value, onChange, min, max, step = 1, suffix, help,
}: {
  label: string
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  step?: number
  suffix?: string
  help?: React.ReactNode
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="flex items-center text-[11px] font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
        {label}{help}
      </span>
      <div className="flex items-center gap-1">
        <input
          type="number"
          value={value}
          min={min}
          max={max}
          step={step}
          onChange={(e) => onChange(Number(e.target.value))}
          className="tabular h-9 w-28 rounded-md px-2 text-sm outline-none focus:ring-2"
          style={{
            background: 'var(--surface-1)',
            color: 'var(--text-primary)',
            border: '1px solid var(--hairline)',
          }}
        />
        {suffix && (
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>{suffix}</span>
        )}
      </div>
    </label>
  )
}

export function TextField({
  label, value, onChange, placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  // basis-64 gives the field a real preferred width so the wrapping filter row
  // breaks onto a new line instead of crushing it; flex-1 with the default
  // basis-0 collapsed it to ~14px next to the fixed-width selects.
  return (
    <label className="flex min-w-48 flex-1 basis-64 flex-col gap-1">
      <span className="text-[11px] font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
        {label}
      </span>
      <input
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
        style={{
          background: 'var(--surface-1)',
          color: 'var(--text-primary)',
          border: '1px solid var(--hairline)',
        }}
      />
    </label>
  )
}

/* ----------------------------------------------------------------- chrome */

export function StatTile({
  label, value, hint, tone, help,
}: {
  label: string
  value: string
  hint?: string
  tone?: 'good' | 'warning' | 'critical'
  help?: React.ReactNode
}) {
  const toneColor =
    tone === 'good' ? 'var(--status-good)'
      : tone === 'warning' ? 'var(--status-warning)'
        : tone === 'critical' ? 'var(--status-critical)'
          : 'var(--text-primary)'
  return (
    <div className="card-surface rounded-lg px-4 py-3">
      <div className="flex items-center text-[11px] font-medium uppercase tracking-wide"
           style={{ color: 'var(--text-muted)' }}>
        {label}{help}
      </div>
      <div className="mt-1 text-2xl font-semibold" style={{ color: toneColor }}>
        {value}
      </div>
      {hint && (
        <div className="mt-0.5 text-xs" style={{ color: 'var(--text-secondary)' }}>
          {hint}
        </div>
      )}
    </div>
  )
}

export function Badge({
  children, tone = 'neutral',
}: {
  children: React.ReactNode
  tone?: 'neutral' | 'good' | 'warning' | 'critical'
}) {
  const map = {
    neutral: { bg: 'var(--gridline)', fg: 'var(--text-secondary)' },
    good: { bg: 'color-mix(in srgb, var(--status-good) 16%, transparent)', fg: 'var(--status-good)' },
    warning: { bg: 'color-mix(in srgb, var(--status-warning) 22%, transparent)', fg: 'var(--text-primary)' },
    critical: { bg: 'color-mix(in srgb, var(--status-critical) 16%, transparent)', fg: 'var(--status-critical)' },
  }[tone]
  return (
    <span className="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium"
          style={{ background: map.bg, color: map.fg }}>
      {children}
    </span>
  )
}

/**
 * The wait, said out loud.
 *
 * `role="status"` is an implicit `aria-live="polite"` region, and it is on the
 * shared spinner rather than on twenty call sites because that is where it
 * covers every surface at once: a turning ring is a picture of waiting, and a
 * picture of waiting is nothing at all to somebody who cannot see it. With a
 * `label` a screen reader now says "Reading the colour guide…" when the wait
 * starts; without one there is nothing to announce and nothing is, which is
 * correct rather than a gap.
 *
 * **What this does not do, deliberately.** The region lives on the spinner, so
 * it goes when the spinner goes — the wait is announced, the *answer* is not.
 * Announcing arrivals means keeping one region mounted across both states at
 * each surface, which is a per-surface change and a design call; it is queued
 * rather than smuggled in here. Half the promise kept out loud beats all of it
 * kept in silence.
 */
export function Spinner({ label }: { label?: string }) {
  return (
    <div role="status" className="flex items-center gap-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
      <span
        className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
        style={{ borderTopColor: 'transparent' }}
        aria-hidden="true"
      />
      {label}
    </div>
  )
}

/**
 * Something went wrong, said out loud.
 *
 * `role="alert"` is an assertive live region, and assertive is right here in a
 * way it would be wrong almost anywhere else: a failure is the one message that
 * must not wait its turn behind whatever is being read, because the person is
 * otherwise still waiting for an answer that is never coming. Every refusal in
 * the app comes through this one component, so the role belongs on it and not
 * on twenty-three copies of it.
 */
export function ErrorNote({ children }: { children: React.ReactNode }) {
  return (
    <div
      role="alert"
      className="rounded-lg px-4 py-3 text-sm"
      style={{
        background: 'color-mix(in srgb, var(--status-critical) 10%, transparent)',
        border: '1px solid color-mix(in srgb, var(--status-critical) 35%, transparent)',
        color: 'var(--text-primary)',
      }}
    >
      {children}
    </div>
  )
}

/** Caveat text that must ride along with any Tier 1 number. */
export function Caveat({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
      {children}
    </p>
  )
}

/* ------------------------------------------------------------------- art */

/**
 * A page's nameplate: a whole painting beside the title, never behind it.
 *
 * Generalised from the library's masthead, which is where the layout was
 * argued: `art_crop` is 626x457, about 1.37:1, and a full-bleed band across
 * the page keeps less than half of the painting's height, so the art is shown
 * at its own ratio and the words sit next to it. Every painting is a Scryfall
 * hotlink (rule 5 and ADR 6 — never redistribute), chosen by naming a
 * printing at the call site with the provenance in a comment, and credited
 * under the title — the credit is the licence made visible, not decoration.
 *
 * The `h1` lives here, so a page that uses this must not render another.
 */
export function PageMasthead({ art, alt, title, credit, children, mood, video }: {
  art: string
  alt: string
  title: React.ReactNode
  /** Card, artist, printing — and one clause on why this painting. */
  credit: React.ReactNode
  children?: React.ReactNode
  /** What drifts along this room's floor — see `RoomMood`. Absent means
   *  the forest mist, which is every room's default weather. */
  mood?: RoomMood
  /** The painting brought to life (`videofx`'s `art` mode): a loop whose
   *  still IS `art`, so the poster, the reduced-motion floor and the room's
   *  backdrop wash all show the same world the video moves in. */
  video?: { webm: string; mp4: string }
}) {
  // A painting and a loop are not shown the same way, and the difference is
  // measured rather than felt. The stills are Scryfall `art_crop`s -- 626x457
  // -- and a 260px column suits one exactly. The two loops are 16:9, and in
  // that same column they rendered 260x191: a quarter of the width cropped
  // off by `object-cover` and the whole picture at a fifth of the band. That
  // is how the Simulator shipped a goldfish nobody could see move. A loop
  // gets its own shape and a share of the row instead (`masthead-loop`).
  // **The plate is the flex item now, and the painting sits inside it.**
  // `.masthead-art` carried `brightness(1.12) saturate(1.1)` in dark mode --
  // Scryfall's "color-shift" clause, on the one image every mastheaded route
  // shows. The lift is a screened layer over the plate instead, which needs a
  // positioned box to be `inset: 0` of, and an `<img>` cannot be one. So the
  // width classes move out to the plate and the media fills it; what renders
  // is what rendered before.
  const plateClass = video
    ? 'masthead-plate masthead-loop w-full sm:w-[46%] sm:max-w-[620px]'
    : 'masthead-plate w-full sm:w-[260px]'
  const artClass = 'masthead-art'
  return (
    <section className="card-surface overflow-hidden rounded-xl">
      <div className="flex flex-col sm:flex-row">
        {/* No ratio and no crop: the container takes the image's own shape,
            so there is nothing to anchor and nothing to lose. Eager, not
            lazy — the masthead is above the fold on every page that has one.
            With a `video`, the loop wears the same box and the still is its
            poster and its reduced-motion fallback; the `<video>` is
            aria-hidden, so the sr-only line keeps the alt where a screen
            reader will meet it. */}
        <span className={plateClass}>
          {video ? (
            <>
              <span className="sr-only">{alt}</span>
              <VideoBackdrop webmSrc={video.webm} mp4Src={video.mp4}
                             poster={art} mode="art" className={artClass}
                             fallback={<img src={art} alt="" className={artClass} />} />
            </>
          ) : (
            <img src={art} alt={alt} className={artClass} />
          )}
          {/* Dark mode's lift, laid on the painting rather than mixed into
              it. Nothing at all in light mode, where it was never wanted. */}
          <span className="art-lift" aria-hidden="true" />
        </span>
        <div className="flex min-w-0 flex-col justify-center gap-1 px-5 py-4 sm:px-6">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          <div className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            {children}
          </div>
          <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
            {credit}
          </p>
        </div>
      </div>
      {/* The page's painting, again, as the room (punch list 2026-08-15 item
          10): every masthead page gets its art washed dim across the whole
          viewport — one component, no second source, no second credit. It is
          fixed-position, so rendering it from inside the section costs
          nothing layout-wise; it renders *after* the real image so the
          masthead art stays the section's first `img` — the one a screen
          reader should meet, and the one the accessibility test checks. */}
      <SceneBackdrop art={art} mood={mood} />
    </section>
  )
}

export function CardArt({
  src, alt, className = '', ratio = 'aspect-[626/457]', position, eager,
}: {
  src?: string | null
  alt: string
  className?: string
  ratio?: string
  /** Load immediately instead of lazily — for art that is above the fold on
   *  the screen it opens on (the persona tiles), where lazy is all cost. */
  eager?: boolean
  /**
   * `object-position` for the crop, when the default centre is wrong.
   *
   * Scryfall's `art_crop` is 626x457 — about 1.37:1 — so any container wider
   * than that throws away height, and a centred crop throws it away from the
   * top and bottom equally. Card art is composed with its subject above the
   * middle far more often than below it, so a wide band wants to keep the top.
   */
  position?: string
}) {
  // *Which* picture has landed, rather than a bare "one has". Derived rather
  // than reset in an effect, and that is a fade rather than a flash: clearing
  // it after the commit meant the new painting drew at full opacity for a
  // frame -- under the old one's fade class -- before snapping to nothing and
  // starting over. Comparing against `src` during render makes the change take
  // effect in the same paint that swapped the picture.
  const [shown, setShown] = useState<string | null>(null)
  const loaded = shown !== null && shown === src
  const imgRef = useRef<HTMLImageElement>(null)

  // A cached image can finish decoding before React attaches onLoad, in which
  // case the event never fires and the fade-in leaves it stuck at opacity 0 --
  // an invisible image over a grey box. Ask the element directly instead.
  useEffect(() => {
    if (src && imgRef.current?.complete) setShown(src)
  }, [src])

  if (!src) {
    return (
      <div className={`${ratio} ${className} flex items-center justify-center rounded-md`}
           style={{ background: 'var(--gridline)' }}>
        <span className="px-2 text-center text-xs" style={{ color: 'var(--text-muted)' }}>
          {alt}
        </span>
      </div>
    )
  }
  return (
    <div className={`${ratio} ${className} overflow-hidden rounded-md`}
         style={{ background: 'var(--gridline)' }}>
      <img
        ref={imgRef}
        src={src}
        alt={alt}
        loading={eager ? 'eager' : 'lazy'}
        onLoad={() => setShown(src)}
        // Never leave a broken URL as an invisible element -- show the frame.
        onError={() => setShown(src)}
        style={position ? { objectPosition: position } : undefined}
        className={`art-fade h-full w-full object-cover ${loaded ? 'loaded' : ''}`}
      />
    </div>
  )
}

/** A card carried by the card being held up — an Equipment or an Aura.
 *
 *  Structural on purpose: the board's `BoardCard` satisfies it without this
 *  file learning what a board is. */
export interface SheetWorn {
  name: string
  image: string
}

/** Nothing is attached. Hoisted rather than defaulted to a fresh `[]`, which
 *  would be a new identity on every render and would churn everything
 *  downstream that watches this prop. */
const NOTHING: readonly SheetWorn[] = []

/** How far a finger travels before it is a riffle and not a tap that
 *  wandered. Below it the sheet closes, which is what a tap here has always
 *  done. */
const SWIPE_MIN = 44

/** How many cards deep the carousel draws before a card is simply gone. Two
 *  is one hinted neighbour on each side plus one in reserve mid-flight; a
 *  creature wearing five things does not want five ghosts on a phone. */
const SEEN = 2

/**
 * One card, held up: centred over a dimmed room, dismissed by a tap anywhere
 * or by Escape — and, when that card is carrying something, **the whole
 * assemblage**, one card at a time.
 *
 * Its own component because two very different surfaces need the same
 * gesture — a thumbnail in a list (`CardHover` below) and a permanent on the
 * arena floor (`components/board.tsx`) — and both need it for the same
 * reason: their reading affordance was `:hover`, and half the room has no
 * pointer to hover with.
 *
 * Portalled to the body. A `position: fixed` sheet rendered in place is
 * trapped by any ancestor carrying a transform, and both callers sit inside
 * several that do — the board's cards are literally mid-rotation.
 *
 * **`worn`, and why the sheet grew a carousel.** The board draws a suited-up
 * creature as one thing — the sword tucked down-and-right under the creature
 * holding it — and then opening it showed the creature *alone* (Aaron,
 * 2026-08-26: *"I want to see all of the cards, especially the equipment and
 * what not underneath, as I swipe left or right through them"*). The tucked
 * corners are the one part of this board that says **that** a creature is
 * carrying something without ever saying **what**, and on a phone the sheet
 * is the only way to find out, because the tuck's own answer is a hover.
 *
 * **The creature comes first.** A player opened the Bear, not the
 * Bonesplitter, so index zero is the card that was tapped and the attachments
 * follow in the order they went on. It clamps at both ends rather than
 * wrapping: this is an ordered assemblage and not a ring, and a dead end you
 * can see is kinder to a newcomer than a loop that silently returns them to
 * the start (commandment 2).
 *
 * **Three ways to move, because a swipe is not a control.** Swipe, the arrow
 * keys, and a rail of controls that is visible without touching anything.
 * This project has shipped a touch-only or hover-only reading affordance
 * twice and lost half the audience both times; a carousel a laptop cannot
 * work is the same bug wearing a third costume. The pips are buttons too, so
 * the fourth way is jumping straight to the sword.
 *
 * **One card is still one card.** With nothing attached this renders exactly
 * what it always rendered — one image, no rail, no caption and no "1 of 1" —
 * which is also what keeps the twenty-three `CardHover` call sites untouched.
 * An attachment the pool gave no painting is dropped rather than drawn as a
 * blank slide, on the board's own rule that no painting opens no sheet.
 */
export function CardSheet({ name, image, onClose, worn = NOTHING }: {
  name: string
  image: string
  onClose: () => void
  /** What is attached to the card being held up, in the order it went on.
   *  Empty everywhere but the battlefield. */
  worn?: readonly SheetWorn[]
}) {
  const [want, setWant] = useState(0)
  // The card that was opened, then what it is carrying. Rebuilt each render
  // rather than memoised: it is at most a handful of entries, and a memo on
  // an array prop that the board rebuilds on every scrub buys nothing.
  const opened: SheetWorn = { name, image }
  const cards: SheetWorn[] = [opened, ...worn.filter((w) => w.image)]
  const many = cards.length > 1
  // Clamped on read rather than on write. A board being scrubbed backwards
  // can take the sword off the creature while its sheet is open, and an index
  // that outlived its card would render nothing at all.
  const at = Math.min(want, cards.length - 1)
  // The fallback is unreachable — `at` is clamped into a list that always has
  // the opened card in it — and is written down because an index into an
  // array is not a promise the compiler will take on trust.
  const here = cards[at] ?? opened
  const step = (d: number) =>
    setWant(Math.min(cards.length - 1, Math.max(0, at + d)))

  const box = useRef<HTMLSpanElement>(null)
  // Where a drag began, and whether it went far enough to be a riffle. Both
  // refs: neither is drawn, and a re-render mid-swipe would lose the origin.
  const from = useRef<{ x: number; y: number } | null>(null)
  const swiped = useRef(false)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { onClose(); return }
      if (!many) return
      if (e.key === 'ArrowRight') { e.preventDefault(); step(1) }
      else if (e.key === 'ArrowLeft') { e.preventDefault(); step(-1) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // Deliberately every render, so the handler reads *this* card's position
    // rather than the one the sheet opened on. One listener, re-hung only
    // when something actually changed the sheet — a modal does not render on
    // a timer.
  })

  // Only when there is something to steer. Moving focus for a lone card would
  // change what a tap does on every card list in the app; here it is what
  // makes Tab reach the rail at all.
  useEffect(() => { if (many) box.current?.focus() }, [many])

  if (!many) {
    return createPortal(
      <span role="dialog" aria-modal="true" aria-label={name}
            className="card-sheet" onClick={onClose}>
        <img src={image} alt={name} className="card-sheet-art" />
      </span>,
      document.body)
  }

  return createPortal(
    <span role="dialog" aria-modal="true" aria-label={name} ref={box}
          tabIndex={-1} className="card-sheet is-many"
          onPointerDown={(e) => {
            swiped.current = false
            from.current = { x: e.clientX, y: e.clientY }
          }}
          onPointerUp={(e) => {
            const start = from.current
            from.current = null
            if (!start) return
            const dx = e.clientX - start.x
            const dy = e.clientY - start.y
            // Horizontal, and decisively so. A drag that is mostly vertical is
            // somebody scrolling the room behind the sheet, not riffling.
            if (Math.abs(dx) < SWIPE_MIN || Math.abs(dx) <= Math.abs(dy)) return
            swiped.current = true
            step(dx < 0 ? 1 : -1)
          }}
          // A swipe ends in a click, and a click here closes. Without this the
          // sheet would change card and then vanish in the same gesture.
          onClick={() => { if (!swiped.current) onClose() }}>
      <span className="card-sheet-hold">
        <span className="card-sheet-stage">
          {cards.map((c, n) => {
            const o = n - at
            const d = Math.min(Math.abs(o), SEEN)
            return (
              <span key={`${n}-${c.name}`}
                    className={'card-sheet-slide'
                               + (o === 0 ? ' is-front' : '')
                               + (d >= SEEN ? ' is-gone' : '')}
                    aria-hidden={o !== 0}
                    style={{ '--o': Math.max(-SEEN, Math.min(SEEN, o)),
                             '--d': d } as CSSProperties}
                    // The hinted neighbour is a target, which is how a mouse
                    // riffles without going near the rail.
                    onClick={(e) => {
                      if (o === 0) return
                      e.stopPropagation()
                      setWant(n)
                    }}>
                <img src={c.image} alt={c.name} className="card-sheet-art"
                     draggable={false} />
              </span>
            )
          })}
        </span>
        {/* Said in words, and said again every time the card changes: the
            painting is the answer for somebody who can see it, and the name
            is the answer for everybody else. */}
        <span className="card-sheet-say" aria-live="polite">
          <span className="card-sheet-say-name">{here.name}</span>
          {at > 0 && (
            <span className="card-sheet-say-on">attached to {name}</span>
          )}
        </span>
        {/* The rail is inside the sheet, so a click on it would close the
            sheet on the way past. It does not. */}
        <span className="card-sheet-rail"
              onClick={(e) => e.stopPropagation()}>
          <button type="button" className="btn card-sheet-step"
                  aria-label="The card before this one"
                  disabled={at === 0} onClick={() => step(-1)}>
            <Chevron back />
          </button>
          <span className="card-sheet-pips">
            {/* The position is in the name rather than drawn as "2 of 3"
                beside them. A sighted person already has it — the lit pip is
                second of three — and a second copy in words is clutter. A
                reader has no pips, so without this the only way to find out
                where you are is to walk all of them. */}
            {cards.map((c, n) => (
              <button key={`${n}-${c.name}`} type="button"
                      className={`card-sheet-pip${n === at ? ' is-on' : ''}`}
                      aria-label={`${c.name}, ${n + 1} of ${cards.length}`}
                      aria-current={n === at ? true : undefined}
                      onClick={() => setWant(n)} />
            ))}
          </span>
          <button type="button" className="btn card-sheet-step"
                  aria-label="The card after this one"
                  disabled={at === cards.length - 1} onClick={() => step(1)}>
            <Chevron />
          </button>
        </span>
      </span>
    </span>,
    document.body)
}

/** The arrow on a rail button. Drawn rather than typed: a `‹` is a font's
 *  opinion about a quotation mark, and it sits off-centre in half of them. */
function Chevron({ back = false }: { back?: boolean }) {
  return (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"
         fill="none" stroke="currentColor" strokeWidth="2.2"
         strokeLinecap="round" strokeLinejoin="round"
         style={back ? { transform: 'scaleX(-1)' } : undefined}>
      <path d="M9 5l7 7-7 7" />
    </svg>
  )
}

/**
 * Full card image on hover, positioned near the cursor — and on a tap,
 * centred, for the half of the audience that has no cursor at all.
 *
 * `className` is for callers whose child is a block — a grid tile or a
 * full-width row. The wrapper is a `span` and therefore inline by default,
 * which silently collapses a `w-full` child to its content width; passing
 * `block` (or `contents`) is how a tile keeps its own layout.
 *
 * **The tap path, and why it is not the hover path.** This was
 * `onMouseEnter` and nothing else, which meant that on a phone the card
 * behind a 96x64 crop was simply unreachable — in twenty-three places,
 * including card search, the deck page and the reading room. A hover-only
 * mechanism locks out every touch user, and here it was the *only* way to
 * answer "what is this card", which is the question a newcomer has most
 * (commandment 2).
 *
 * A tap does not get the cursor treatment. There is no cursor to sit beside,
 * a finger covers the point it would sit at, and near either edge the card
 * would clip. So touch gets a centred sheet over a dimmed room — the same
 * card, bigger, dismissed by tapping anywhere or pressing Escape.
 *
 * **`tapOpens` exists because four of the twenty-three wrap a real button** —
 * the commander picker on the create flow, and the reading room's own tiles.
 * There a tap already means *choose this*, and stealing it would break the
 * newcomer's most important decision to show them a picture they did not ask
 * for. Those four pass `false`; everywhere the tile is inert, the tile
 * answers.
 */
export function CardHover({
  card, children, className = '', tapOpens = true,
}: {
  card: { name: string; image?: string | null }
  children: React.ReactNode
  className?: string
  /** False where the child is itself a control and the tap is already spoken
   *  for. See the note above. */
  tapOpens?: boolean
}) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const [held, setHeld] = useState(false)
  // Which kind of hand last touched this. Touch browsers fire mouseenter and
  // mousemove *synthetically* after a tap, so without this a tap would open
  // the sheet and then immediately arm the cursor preview behind it.
  const coarse = useRef(false)
  const ref = useRef<HTMLSpanElement>(null)

  // The scroll listener exists to clear a preview the wheel scrolled out from
  // under, so it is registered only while a preview is showing. Registered on
  // mount it was ~200 capture-phase listeners on a deck's cards tab — two per
  // row — every one of them run on every scroll frame, which is a measurable
  // slice of why that tab dragged. At most one instance shows a preview at a
  // time, so this is now at most one listener.
  const showing = pos !== null
  useEffect(() => {
    if (!showing) return
    const clear = () => setPos(null)
    window.addEventListener('scroll', clear, true)
    return () => window.removeEventListener('scroll', clear, true)
  }, [showing])

  return (
    <span
      ref={ref}
      className={className}
      onPointerDown={(e) => { coarse.current = e.pointerType !== 'mouse' }}
      onPointerUp={(e) => {
        if (e.pointerType === 'mouse' || !tapOpens || !card.image) return
        setPos(null)
        setHeld(true)
      }}
      onMouseEnter={(e) => !coarse.current && card.image
        && setPos({ x: e.clientX, y: e.clientY })}
      onMouseMove={(e) => !coarse.current && card.image
        && setPos({ x: e.clientX, y: e.clientY })}
      onMouseLeave={() => setPos(null)}
    >
      {children}
      {held && card.image && (
        <CardSheet name={card.name} image={card.image}
                   onClose={() => setHeld(false)} />
      )}
      {pos && card.image && (
        <span
          className="pointer-events-none fixed z-50 block"
          style={{
            left: Math.min(pos.x + 18, window.innerWidth - 250),
            top: Math.min(pos.y + 12, window.innerHeight - 350),
          }}
        >
          <img src={card.image} alt={card.name} width={230}
               className="rounded-xl shadow-2xl" />
        </span>
      )}
    </span>
  )
}

/**
 * A magnifying glass over a card that is already shown whole (second punch
 * list, item 3). `CardHover` answers "what is this card?" for a thumbnail;
 * on the deck hero the full card is the thing being hovered, so popping up a
 * second, smaller copy of it answered a question nobody asked. This instead
 * holds a lens over the printing itself: the same image, drawn at several
 * times the size through a round glass that follows the cursor, rim and
 * handle styled in the house gold. The detail is real — the source bitmap is
 * far larger than the card is displayed — so the lens shows brush strokes
 * and rules text, not bigger pixels.
 *
 * Hover-only by nature, and honestly so: on a touch screen it simply never
 * appears, and the card underneath was already whole. jsdom has no layout,
 * so every rect read guards against zero.
 */
export function CardLoupe({ src, alt, className = '', zoom = 2.6, imgStyle }: {
  src: string
  alt: string
  className?: string
  zoom?: number
  imgStyle?: React.CSSProperties
}) {
  // Where the cursor is over the card, *and* how big the card was when it was
  // there. The size travels with the position because the two are read from
  // one measurement in one event: taking the rect again at render time meant
  // the glass was sized by whatever the layout happened to be at the last
  // paint while the cursor came from the last mouse move, and a card that
  // resized between the two put the lens somewhere its own maths denied.
  // jsdom has no layout, so a zero rect never becomes a position at all --
  // which is the same "no lens off the test rig" this had before.
  const [at, setAt] = useState<
    { x: number; y: number; w: number; h: number } | null>(null)
  const ref = useRef<HTMLImageElement>(null)

  function track(e: React.MouseEvent) {
    const rect = ref.current?.getBoundingClientRect()
    if (!rect || rect.width === 0 || rect.height === 0) return
    setAt({
      x: (e.clientX - rect.left) / rect.width,
      y: (e.clientY - rect.top) / rect.height,
      w: rect.width,
      h: rect.height,
    })
  }

  return (
    <span className={`card-loupe-host relative inline-block ${className}`}
          onMouseEnter={track} onMouseMove={track}
          onMouseLeave={() => setAt(null)}>
      <img ref={ref} src={src} alt={alt} className="block w-full rounded-xl"
           style={imgStyle} />
      {at && (
        <span aria-hidden className="card-loupe"
              style={{
                left: `${at.x * 100}%`,
                top: `${at.y * 100}%`,
                backgroundImage: `url(${src})`,
                backgroundSize: `${at.w * zoom}px ${at.h * zoom}px`,
                backgroundPosition: `${-(at.x * at.w * zoom - 66)}px ${-(at.y * at.h * zoom - 66)}px`,
              }} />
      )}
    </span>
  )
}

/**
 * A destructive control that arms on the first click and acts on the second.
 *
 * The confirmation structure ADR 27 chose over a dialog: one accidental click
 * does nothing except turn the button solid red for a few seconds, and a
 * deliberate deletion is still two quick clicks in place — no modal to dismiss
 * ten times while tuning a deck. The arm times out rather than staying
 * cocked, because a button left armed yesterday firing today is the mis-click
 * this exists to prevent.
 *
 * It lives here rather than beside the deck page because it has a second
 * caller now: the reading room's "Start over", which is destructive in the
 * way this pattern was built for *and* spends money — it discards the
 * transcript and the next turn is a paid one.
 */
export function ArmedButton({ children, armedLabel, title, onConfirm }: {
  children: ReactNode
  /** What the armed state says — name the consequence, not "are you sure". */
  armedLabel: string
  title?: string
  onConfirm: () => void
}) {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const timer = setTimeout(() => setArmed(false), 4000)
    return () => clearTimeout(timer)
  }, [armed])
  return (
    <button
      type="button"
      title={title}
      aria-pressed={armed}
      className={'card-action card-action-danger rounded-md px-2 py-1 '
                 + `text-[11px] font-medium${armed ? ' armed' : ''}`}
      onClick={() => {
        if (armed) {
          setArmed(false)
          onConfirm()
        } else {
          setArmed(true)
        }
      }}
    >
      {armed ? armedLabel : children}
    </button>
  )
}
