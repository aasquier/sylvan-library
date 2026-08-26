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
  // Four-colour: the canonical names from the colour taxonomy the server
  // owns, which is the project's authority on combination
  // naming -- it makes the Scryfall/Commander-2016 names primary and keeps
  // the Nephilim names as aliases. This table must agree with it, or the
  // library and the Start-a-deck grid call the same deck two different
  // things in one session.
  WUBR: 'Artifice',
  UBRG: 'Chaos',
  WBRG: 'Aggression',
  WURG: 'Altruism',
  WUBG: 'Growth',
  WUBRG: 'Five-colour',
}

/**
 * Canonical colour order, and the mirror of the order the server writes
 * every combination key in.
 *
 * Exported because the pentagram is drawn from it: the five vertices go round
 * in this order, which is what makes adjacency mean "allied" and two-apart
 * mean "enemy". A different order here would draw a diagram that is merely
 * decorative.
 */
export const WUBRG = ['W', 'U', 'B', 'R', 'G']

export function identityName(identity: string[]): string {
  if (!identity.length) return 'Colorless'
  const key = WUBRG.filter((c) => identity.includes(c)).join('')
  // Bound once rather than testing and re-reading. Two lookups of a mutable
  // table are two chances to disagree, and under noUncheckedIndexedAccess the
  // second one is `string | undefined` however the first one went.
  const exact = GUILDS[key]
  if (exact) return exact
  // Try every rotation, since guild names are cycle-invariant in the wedge case.
  for (let i = 0; i < WUBRG.length; i++) {
    const rotated = [...WUBRG.slice(i), ...WUBRG.slice(0, i)]
      .filter((c) => identity.includes(c))
      .join('')
    const cycled = GUILDS[rotated]
    if (cycled) return cycled
  }
  // `identity[0]` exists -- the empty case returned above -- but the checker
  // cannot see that, and a colour outside WUBRG has no entry in COLOR_NAMES
  // either. Both fall back to the key, which is the identity as written.
  const mono = identity.length === 1 ? COLOR_NAMES[identity[0] ?? ''] : undefined
  return mono ? `Mono-${mono}` : key
}

/**
 * A symbol's spoken name — the tooltip and the accessible name of a pip.
 *
 * Every pip is a drawing now (ADR 33), so no symbol contributes its letter
 * to the page as text; the name is the only way any of them read aloud.
 * Extends `COLOR_NAMES` with the rest of the vocabulary the prose regex
 * recognises. Anything unknown falls back to the symbol as written, which
 * is thin but never wrong.
 */
export function symbolName(sym: string): string {
  const colour = COLOR_NAMES[sym]
  if (colour) return colour
  if (sym === 'T') return 'Tap'
  if (sym === 'Q') return 'Untap'
  if (sym === 'S') return 'Snow'
  if (sym === 'E') return 'Energy'
  if (sym === 'X') return 'X'
  if (/^\d+$/.test(sym)) return `Generic ${sym}`
  const hybrid = /^([WUBRG2C])\/([WUBRGP])$/.exec(sym)
  if (hybrid) {
    // The pattern's groups always participate, but the checker cannot see
    // that; `?? sym` keeps a miss loud enough to notice instead of "undefined".
    const left = hybrid[1] ?? sym
    const right = hybrid[2] ?? sym
    if (right === 'P') return `Phyrexian ${COLOR_NAMES[left] ?? left}`
    const first = left === '2' ? 'Two' : COLOR_NAMES[left] ?? left
    return `${first} or ${COLOR_NAMES[right] ?? right}`
  }
  return sym
}

/* ------------------------------------------ what a permanent taps for */

/** WUBRG, then colourless — Magic's own order, so two cards that tap for the
 *  same mana always draw the same mark. */
const MANA_ORDER = ['W', 'U', 'B', 'R', 'G', 'C']

/**
 * The ten official hybrid symbols, keyed on their pair in `MANA_ORDER`.
 *
 * Every two-colour pair has one — five allied, five enemy — and **the order
 * inside each is not ours to pick**: the official set spells it `{G/W}`, so
 * `GW` is a symbol and `WG` is a 404 wearing a fallback.
 */
const HYBRID: Record<string, string> = {
  WU: 'W/U', UB: 'U/B', BR: 'B/R', RG: 'R/G', WG: 'G/W',
  WB: 'W/B', UR: 'U/R', BG: 'B/G', WR: 'R/W', UG: 'G/U',
}

/**
 * The colours a permanent taps for: deduped, in Magic's order, and with
 * anything that is not a mana colour dropped rather than drawn.
 */
export function producedColors(makes: readonly string[] | undefined): string[] {
  if (!makes?.length) return []
  const seen = new Set(makes.map((m) => m.toUpperCase()))
  return MANA_ORDER.filter((c) => seen.has(c))
}

/**
 * The one symbol that says what a permanent taps for, or null when no single
 * official symbol says it.
 *
 * **One mark, never a row of them — and that is a rules point, not a space
 * one.** `{G}{W}` means two mana. A Temple Garden makes *one*, green or white,
 * so two pips side by side would teach a beginner something false about the
 * card in front of them (commandment 2). A pair becomes the official hybrid
 * symbol, which is Magic's own way of writing "or"; anything wider has no
 * official symbol at all and falls to the prism.
 */
export function producedSymbol(colors: readonly string[]): string | null {
  if (colors.length === 1) return colors[0] ?? null
  if (colors.length === 2) return HYBRID[colors.join('')] ?? null
  return null
}

/**
 * What a permanent taps for, in words — because the mark is a drawing, and a
 * drawing is a thing you have to already know.
 */
export function producedName(colors: readonly string[]): string {
  if (!colors.length) return ''
  // What a player would actually say about a Birds of Paradise. Naming the
  // five in a list is technically the same claim and nobody talks that way.
  if (colors.length === 5 && !colors.includes('C')) return 'mana of any colour'
  // **The sentence reads the drawing.** A pair is drawn as `{G/W}`, which the
  // official set spells green-first, while `MANA_ORDER` would say "white or
  // green" — and the words are how anybody not looking at a fifteen-pixel
  // picture gets that picture. Two orders for one mark is one order too many.
  const sym = producedSymbol(colors)
  const said = sym?.includes('/') ? sym.split('/') : colors
  const names = said.map((c) =>
    c === 'C' ? 'colourless' : (COLOR_NAMES[c] ?? c).toLowerCase())
  const last = names.pop() ?? ''
  return names.length ? `${names.join(', ')} or ${last} mana` : `${last} mana`
}

/** Split "{2}{B}{G}" into ["2","B","G"] for pip rendering. */
export function manaSymbols(cost?: string | null): string[] {
  if (!cost) return []
  // `flatMap` over `map` to drop a match with no group 1 rather than assert it
  // cannot happen. The pattern has exactly one group so a match always fills
  // it, but that is a fact about the regex two lines up, not one the checker
  // can see -- and `!` here would be a habit that costs nothing until the
  // pattern gains an alternation with a group that does not always
  // participate, at which point it silently yields `undefined` as a pip.
  return Array.from(cost.matchAll(/\{([^}]+)\}/g)).flatMap((m) => m[1] ?? [])
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
 * The colour run is why this is not simply `manaSymbols`. The gate writes
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

    const symbol = match[1]?.toUpperCase()
    if (!symbol) {
      // Unreachable today: MANA_IN_TEXT's single group always participates in
      // a match. Written as a branch rather than a `!` so that if the pattern
      // ever grows an alternation whose group does not, the brace text renders
      // as the prose it came from instead of vanishing from the output.
      parts.push({ text: match[0], pip: false })
    } else if (/^[WUBRGC]{2,}$/.test(symbol)) {
      // A colour run is several pips; everything else is exactly one.
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
