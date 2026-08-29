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
 *
 * **`stance` is required, and it may be `undefined`.** Those are not the same
 * thing, and the difference is the bug this parameter was written for. It was
 * optional, the one call site never passed it, and the server therefore
 * resolved the deck's own default instead of the stance the sheet had just
 * shown the user a control for — so ADR 41's drafting was offered by the page
 * and refused by the route, every time, from the day it shipped. `undefined`
 * is a real position here ("no pin; let the surface default"), so the type
 * cannot forbid it — but it can insist that somebody decided, and that is what
 * a required parameter buys. Pass what `IntakeChoices` reported through
 * `onStance` and nothing else: the value that decided what the sheet showed is
 * the only value that can honestly decide what the server does.
 */
export async function runIntake(
  ref: { owner: string; slug: string },
  sheet: IntakeSheet,
  stance: string | undefined,
): Promise<Job | null> {
  if (!Object.values(sheet).some((on) => on === true)) return null
  return api.intake(ref, { ...sheet, stance })
}
