/**
 * Friendly names for the stance vocabulary.
 *
 * The server speaks in wire tokens — `second-opinion`, `on-request` — because
 * those are enum values with tests hanging off them. The UI used to render
 * them verbatim, which put an implementation detail in a radio label. This is
 * the one place the translation lives; a component that needs a human name
 * asks here rather than keeping its own copy, so a renamed preset changes one
 * table instead of five strings.
 *
 * The fallback matters more than the table. A pin can name a preset this
 * build no longer serves (the self-heal in `lib/stance.ts` clears it, but not
 * before a render), and the server may grow a preset before this file hears
 * about it — so an unknown token de-hyphenates and capitalises rather than
 * rendering raw or rendering nothing.
 */

const PRESET_LABELS: Record<string, string> = {
  off: 'Off',
  consultant: 'Consultant',
  'second-opinion': 'Second opinion',
  collaborator: 'Collaborator',
}

const LEVEL_LABELS: Record<string, string> = {
  // initiative
  off: 'stays silent',
  'on-request': 'answers when asked',
  volunteers: 'volunteers observations',
  interjects: 'interjects on its own',
  // scope
  flagged: 'flagged cards only',
  adjacent: 'the card and its neighbours',
  rethink: 'the whole deck',
  // write autonomy
  none: 'never edits',
  proposes: 'proposes edits for you to apply',
}

function humanise(token: string): string {
  const words = token.split('-').join(' ')
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/** A preset's display name. `null`/`undefined` reads as the unpinned position. */
export function presetLabel(name: string | null | undefined): string {
  if (!name) return 'Follow the deck'
  return PRESET_LABELS[name] ?? humanise(name)
}

/** An axis level's display name, for the resolved-stance readout. */
export function levelLabel(level: string): string {
  return LEVEL_LABELS[level] ?? humanise(level)
}
