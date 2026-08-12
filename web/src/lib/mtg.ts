/** Magic-specific display helpers. */

export const COLOR_NAMES: Record<string, string> = {
  W: 'White',
  U: 'Blue',
  B: 'Black',
  R: 'Red',
  G: 'Green',
  C: 'Colorless',
}

export const COLOR_VAR: Record<string, string> = {
  W: 'var(--mtg-w)',
  U: 'var(--mtg-u)',
  B: 'var(--mtg-b)',
  R: 'var(--mtg-r)',
  G: 'var(--mtg-g)',
  C: 'var(--mtg-c)',
}

/** Colour-combination names players actually use. */
const GUILDS: Record<string, string> = {
  WU: 'Azorius',
  UB: 'Dimir',
  BR: 'Rakdos',
  RG: 'Gruul',
  GW: 'Selesnya',
  WB: 'Orzhov',
  UR: 'Izzet',
  BG: 'Golgari',
  RW: 'Boros',
  GU: 'Simic',
  WUB: 'Esper',
  UBR: 'Grixis',
  BRG: 'Jund',
  RGW: 'Naya',
  GWU: 'Bant',
  WBG: 'Abzan',
  URW: 'Jeskai',
  BGU: 'Sultai',
  RWB: 'Mardu',
  GUR: 'Temur',
  // Four-colour: the Nephilim convention, keyed by the missing colour. Each
  // name's identity is verified against the corpus (the Nephilim are cards),
  // not recalled -- see the pinning test.
  WUBR: 'Yore-Tiller',
  UBRG: 'Glint-Eye',
  WBRG: 'Dune-Brood',
  WURG: 'Ink-Treader',
  WUBG: 'Witch-Maw',
  WUBRG: 'Five-colour',
}

const WUBRG = ['W', 'U', 'B', 'R', 'G']

export function identityName(identity: string[]): string {
  if (!identity.length) return 'Colorless'
  const key = WUBRG.filter((c) => identity.includes(c)).join('')
  if (GUILDS[key]) return GUILDS[key]
  // Try every rotation, since guild names are cycle-invariant in the wedge case.
  for (let i = 0; i < WUBRG.length; i++) {
    const rotated = [...WUBRG.slice(i), ...WUBRG.slice(0, i)]
      .filter((c) => identity.includes(c))
      .join('')
    if (GUILDS[rotated]) return GUILDS[rotated]
  }
  return identity.length === 1 ? `Mono-${COLOR_NAMES[identity[0]]}` : key
}

/** Split "{2}{B}{G}" into ["2","B","G"] for pip rendering. */
export function manaSymbols(cost?: string | null): string[] {
  if (!cost) return []
  return Array.from(cost.matchAll(/\{([^}]+)\}/g)).map((m) => m[1])
}

/**
 * A mana symbol inside prose. Deliberately narrower than `manaSymbols`, which
 * runs over a `mana_cost` field where everything between braces is a symbol by
 * definition. In prose it is not: `{note}` and `{}` are ordinary text, and
 * drawing them as pips would be the UI claiming to have read something it did
 * not.
 *
 * The alternatives, in order: a generic cost like `{10}`; a hybrid or
 * Phyrexian symbol like `{G/W}` or `{U/P}`; a run of colour letters; or one of
 * the standalone symbols X/Y/Z, tap, untap, snow, energy.
 *
 * The colour run is why this is not simply `manaSymbols`. `validate.py` writes
 * a colour identity as `{GW}` — one brace, two colours — because that is how
 * Magic writes an identity. It is not a mana cost, and rendering it as a
 * single two-letter blob would be wrong, so it expands to one pip per colour.
 */
const MANA_IN_TEXT = /\{(\d+|[WUBRG2]\/[WUBRGP]|[WUBRGC]+|[XYZSTQE])\}/gi

export interface ManaTextPart {
  text: string
  /** True when `text` is a single symbol to draw, false for literal prose. */
  pip: boolean
}

export function splitManaText(text: string): ManaTextPart[] {
  const parts: ManaTextPart[] = []
  let at = 0

  for (const match of text.matchAll(MANA_IN_TEXT)) {
    const start = match.index
    if (start > at) parts.push({ text: text.slice(at, start), pip: false })

    const symbol = match[1].toUpperCase()
    // A colour run is several pips; everything else is exactly one.
    if (/^[WUBRGC]{2,}$/.test(symbol)) {
      for (const colour of symbol) parts.push({ text: colour, pip: true })
    } else {
      parts.push({ text: symbol, pip: true })
    }
    at = start + match[0].length
  }

  if (at < text.length) parts.push({ text: text.slice(at), pip: false })
  return parts
}

export const CATEGORY_LABELS: Record<string, string> = {
  land: 'Lands',
  ramp: 'Ramp',
  'card-advantage': 'Card advantage',
  tutor: 'Tutors',
  interaction: 'Interaction',
  protection: 'Protection',
  threat: 'Threats',
  engine: 'Engines',
  'sac-outlet': 'Sacrifice outlets',
  payoff: 'Payoffs',
  recursion: 'Recursion',
  'win-con': 'Win conditions',
  utility: 'Utility',
  commander: 'Commander',
}

export function categoryLabel(key: string): string {
  return CATEGORY_LABELS[key] ?? key
}

export function money(usd?: number | null): string {
  if (usd === null || usd === undefined) return '—'
  return usd < 1 ? `$${usd.toFixed(2)}` : `$${usd.toFixed(2)}`
}

export function percent(fraction: number, digits = 1): string {
  return `${(fraction * 100).toFixed(digits)}%`
}
