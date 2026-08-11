import { useEffect, useRef, useState } from 'react'
import { COLOR_NAMES, COLOR_VAR, manaSymbols, splitManaText } from '../lib/mtg'

/* ------------------------------------------------------------------ pips */

/** One mana symbol. Shared so a cost and a pip in prose cannot drift apart. */
function Pip({ sym, size }: { sym: string; size: number }) {
  const solid = COLOR_VAR[sym]
  return (
    <span
      title={COLOR_NAMES[sym] ?? sym}
      className="inline-flex items-center justify-center rounded-full align-middle font-semibold text-[#141414]"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.62,
        background: solid ?? 'var(--mtg-c)',
        // A ring keeps adjacent pips from merging into one blob, which is the
        // 2px surface-gap rule applied to inline marks.
        boxShadow: '0 0 0 1px var(--hairline)',
      }}
    >
      {sym.replace('/', '')}
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
          className="inline-block h-3 w-3 rounded-full"
          style={{ background: COLOR_VAR[c], boxShadow: '0 0 0 1px var(--hairline)' }}
        />
      ))}
    </span>
  )
}

/* --------------------------------------------------------------- controls */

interface SelectProps {
  label: string
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  className?: string
}

export function Select({ label, value, onChange, options, className = '' }: SelectProps) {
  return (
    <label className={`flex flex-col gap-1 ${className}`}>
      <span className="text-[11px] font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
        {label}
      </span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
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
  label, value, onChange, min, max, step = 1, suffix,
}: {
  label: string
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  step?: number
  suffix?: string
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[11px] font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
        {label}
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
  label, value, hint, tone,
}: {
  label: string
  value: string
  hint?: string
  tone?: 'good' | 'warning' | 'critical'
}) {
  const toneColor =
    tone === 'good' ? 'var(--status-good)'
      : tone === 'warning' ? 'var(--status-warning)'
        : tone === 'critical' ? 'var(--status-critical)'
          : 'var(--text-primary)'
  return (
    <div className="card-surface rounded-lg px-4 py-3">
      <div className="text-[11px] font-medium uppercase tracking-wide"
           style={{ color: 'var(--text-muted)' }}>
        {label}
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

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
      <span
        className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
        style={{ borderTopColor: 'transparent' }}
      />
      {label}
    </div>
  )
}

export function ErrorNote({ children }: { children: React.ReactNode }) {
  return (
    <div
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

export function CardArt({
  src, alt, className = '', ratio = 'aspect-[626/457]',
}: {
  src?: string | null
  alt: string
  className?: string
  ratio?: string
}) {
  const [loaded, setLoaded] = useState(false)
  const imgRef = useRef<HTMLImageElement>(null)

  // A cached image can finish decoding before React attaches onLoad, in which
  // case the event never fires and the fade-in leaves it stuck at opacity 0 --
  // an invisible image over a grey box. Ask the element directly instead.
  useEffect(() => {
    setLoaded(false)
    if (imgRef.current?.complete) setLoaded(true)
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
        loading="lazy"
        onLoad={() => setLoaded(true)}
        // Never leave a broken URL as an invisible element -- show the frame.
        onError={() => setLoaded(true)}
        className={`art-fade h-full w-full object-cover ${loaded ? 'loaded' : ''}`}
      />
    </div>
  )
}

/** Full card image on hover, positioned near the cursor. */
export function CardHover({
  card, children,
}: {
  card: { name: string; image?: string | null }
  children: React.ReactNode
}) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const clear = () => setPos(null)
    window.addEventListener('scroll', clear, true)
    return () => window.removeEventListener('scroll', clear, true)
  }, [])

  return (
    <span
      ref={ref}
      onMouseEnter={(e) => card.image && setPos({ x: e.clientX, y: e.clientY })}
      onMouseMove={(e) => card.image && setPos({ x: e.clientX, y: e.clientY })}
      onMouseLeave={() => setPos(null)}
    >
      {children}
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
