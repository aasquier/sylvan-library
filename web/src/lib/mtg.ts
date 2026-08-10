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

/** Colour-pair names players actually use. */
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
