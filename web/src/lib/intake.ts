import { api, type IntakeSheet, type Job } from './api'

/**
 * Runs the intake sheet against a deck that has just been created (ADR 41).
 *
 * In `lib/` rather than beside the component that collects the sheet, and the
 * reason is a lint rule with a real point behind it: a file that exports both
 * a component and a function loses fast refresh for the component. The rule
 * is the mechanism; the habit it enforces is that "what the screen looks like"
 * and "what the screen does" are different files.
 *
 * **A sheet with nothing ticked makes no call at all.** Returning null rather
 * than submitting a job is what stops an untouched sheet from putting a
 * progress bar and a five-step spinner in front of somebody who asked for
 * none of it.
 */
export async function runIntake(
  ref: { owner: string; slug: string },
  sheet: IntakeSheet,
  stance?: string,
): Promise<Job | null> {
  if (!Object.values(sheet).some((on) => on === true)) return null
  return api.intake(ref, { ...sheet, stance })
}
