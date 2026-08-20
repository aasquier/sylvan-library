/**
 * The match theater's two pure functions, in `lib/` for the reason
 * `lib/motion.ts` gives at the top of its own file: oxlint's fast-refresh
 * rule is right, these are not components, and each of them is needed by more
 * than one file. `theaterRows` is called by the Simulator (which owns the
 * job) and `shortName` by the stage (which owns the row); putting either in
 * `components/theater.tsx` costs that file its fast refresh to save a import.
 */

import type { ForgeGameRow } from './api'

/** The Forge's `partial` payload, narrowed.
 *
 * `Job.partial` is `unknown` because the shape belongs to the job's kind, so
 * somebody has to do this once. It is deliberately total: a job that has not
 * ticked yet, a pre-theater worker streaming counts with no rows at all (the
 * skew `worker.run_match` tolerates on purpose), and a finished job whose
 * partial the server has cleared all arrive here and all mean "no rows yet".
 */
export function theaterRows(partial: unknown): ForgeGameRow[] {
  if (!partial || typeof partial !== 'object') return []
  const rows = (partial as { rows?: unknown }).rows
  return Array.isArray(rows) ? (rows as ForgeGameRow[]) : []
}

/** What to call a deck in a line that has to stay one line.
 *
 * A deck is named for its commander and then for what it does — "Arahbo, Roar
 * of the World — Cats" — and a feed row wants the first part of that: the
 * general's name, which is how anybody actually refers to the deck out loud.
 * So take everything before the dash, then everything before the title's
 * comma. A name with neither is already short and comes back untouched.
 */
export function shortName(name: string): string {
  const beforeDash = name.split('—')[0] ?? name
  return (beforeDash.split(',')[0] ?? beforeDash).trim() || name
}
