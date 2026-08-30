import { ApiError, api, type IntakeSheet, type Job } from './api'

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

/**
 * The import page's own words for extra work that did not finish.
 *
 * `forgeTrouble` in `internal/api/forgevoice.go` is the pattern and the
 * argument is the same one: the machinery's account of itself is for the log,
 * and the person gets a sentence written for them. What reached the screen
 * before this existed was **`no such job`** — lowercase, unpunctuated, and
 * naming a piece of the server nobody using this site has heard of
 * (commandment 10).
 *
 * **The 404 is not a mystery, and it is worth saying plainly.** The job
 * registry is in memory, so a deploy takes every run in flight with it — and
 * merging is deploying here (ADR 23), which makes this ordinary rather than
 * exotic. It is what happened on the first real intake ever run against a
 * ninety-nine.
 *
 * Every one of these says the same first thing, because it is the thing that
 * matters and it is true: **the deck is saved.** The intake writes to a deck
 * the server has already created; nothing that fails here can un-create it.
 */
export function intakeTrouble(err: unknown): string {
  const lost = err instanceof ApiError && err.status === 404
  if (lost) {
    return 'Your deck is saved and safe. The extra work you asked for did not '
      + 'finish — the library was most likely restarting while it ran, which '
      + 'takes a few seconds and happens on its own. Nothing was written '
      + 'wrongly; the reasons simply are not there yet.'
  }
  return 'Your deck is saved and safe. The extra work you asked for did not '
    + 'finish, and nothing was written wrongly — the reasons simply are not '
    + 'there yet.'
}
