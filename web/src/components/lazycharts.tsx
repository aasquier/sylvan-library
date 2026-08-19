/** The charts, fetched when one is actually about to be drawn.
 *
 * recharts is 113kB gzipped — larger than the whole rest of the application
 * put together — and every route that draws a chart was paying for it up
 * front, whether or not a chart was ever on the screen. The deck page's are
 * behind the Stats tab; the simulator's cannot appear until a job that takes
 * seconds has finished; the admin dashboard's sit below the fold. In all
 * three the library arrives well before the data it draws.
 *
 * Each export here has the same name and props as the real component, so a
 * screen imports from this module instead of `./charts` and changes nothing
 * else. The `Suspense` is per chart rather than per page on purpose: a deck
 * page that is waiting on one chart still renders its tiles, its tables and
 * its other sections, so nothing that is already loaded is held hostage by
 * something that is not.
 *
 * `DataTable` is deliberately NOT here — it draws no chart and lives in
 * `./datatable`, which is what keeps a table from pulling recharts in.
 */

import { deferred } from '../lib/deferred'

// Heights match the `ResponsiveContainer` each component declares, so the
// placeholder is the same size as the thing replacing it.
export const CurveChart = deferred(
  async () => (await import('./charts')).CurveChart, 200)
export const ByTurnChart = deferred(
  async () => (await import('./charts')).ByTurnChart, 260)
export const CommanderCurve = deferred(
  async () => (await import('./charts')).CommanderCurve, 220)
export const LandSweepChart = deferred(
  async () => (await import('./charts')).LandSweepChart, 260)
export const LandTradeoffChart = deferred(
  async () => (await import('./charts')).LandTradeoffChart, 260)
export const WastedManaChart = deferred(
  async () => (await import('./charts')).WastedManaChart, 220)
export const CategoryCoverage = deferred(
  async () => (await import('./charts')).CategoryCoverage, 260)
export const ColorNeedsChart = deferred(
  async () => (await import('./charts')).ColorNeedsChart, 220)
export const EditsChart = deferred(
  async () => (await import('./charts')).EditsChart, 200)
export const TrafficChart = deferred(
  async () => (await import('./charts')).TrafficChart, 220)
