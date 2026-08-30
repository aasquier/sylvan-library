import {
  ApiError, api, type IntakeResult, type IntakeSheet, type IntakeStepKey,
  type Job,
} from './api'

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

/**
 * What each action is called, wherever a person is shown one.
 *
 * **One copy, used twice.** The sheet puts these on its chips before the run
 * and the account of what happened puts them on its sentences afterwards, and
 * those have to be the same words: somebody who ticked "Sort the cards" and is
 * then told something about "categorisation" has been handed a second name for
 * the thing they chose.
 *
 * A total `Record` rather than a list, deliberately, and it is doing two jobs
 * the typechecker will not let it drop. A sixth action added to the wire
 * without a name here fails to compile — where a list would simply have
 * rendered its outcome as nothing at all. And object key order is insertion
 * order for string keys, so **this literal is also the reading order**: the
 * outcomes come back down the page in the order the actions were offered,
 * rather than in whatever order the wire happened to serialise them.
 *
 * Each is what the action *does* rather than what it is called, because the
 * audience for this page is somebody who has just pasted their one deck
 * (commandment 2): "commander dossier" is a name, and "read up on your
 * commander" is what they get.
 */
export const INTAKE_TITLES: Record<IntakeStepKey, string> = {
  categories: 'Sort the cards',
  rationales: 'Draft the reasons',
  description: 'Describe the deck',
  dossier: 'Read up on your commander',
  argue: 'Argue with every card',
}

/** One thing the finished intake has to say for itself, ready to render:
 *  the action it is about, in the sheet's own words, and the sentence the
 *  server wrote for it. */
export interface IntakeAftermath {
  key: IntakeStepKey | 'asked'
  /** Empty for the whole-run refusal, which is about no single action. */
  title: string
  note: string
}

/**
 * What the intake wants to tell you, once it has finished.
 *
 * **Every step could already say this and nothing ever listened.** A step's
 * `note` is written by the server for exactly the moments where the two
 * numbers beside it would read as a failure — "eighty-four of eighty-five" is
 * a fine result and a frightening sentence — and the import page's answer to
 * all of them was to navigate to the deck the instant the job resolved. So the
 * one thing a run could never tell you was the thing worth knowing: **which
 * cards it left**.
 *
 * That is the omission this exists for. Leaving a card out is the design —
 * `internal/claude/intake.go` argues it: a card Claude cannot ground is left
 * alone and its owner writes that one, which is exactly where they were before
 * the intake ran. What was missing was the sentence, and without it the only
 * way to find your own card was to read ninety-nine reasons looking for the
 * gap.
 *
 * `unknown` in, because that is honestly what a job's `result` is — the shape
 * belongs to the job's `kind` and this is the only file that knows which. A
 * result that is not the shape expected yields nothing to say rather than a
 * thrown error: the deck is saved either way, and a page that crashed on the
 * way to reporting good news would be the worse failure by a distance.
 */
export function intakeAftermath(result: unknown): IntakeAftermath[] {
  if (result === null || typeof result !== 'object') return []
  const done = result as IntakeResult

  // The whole run refused — the stance would not speak — and `reason` is the
  // only thing it has to say. Its steps are empty by construction, so this is
  // the sentence or nothing.
  if (done.asked === false) {
    return done.reason ? [{ key: 'asked', title: '', note: done.reason }] : []
  }

  // `?? {}` against a field the type calls required, because the type is a
  // claim about a wire, and this function is explicitly handed `unknown`. A
  // result of another shape reached here and threw on the missing `steps`,
  // which would have taken the page down at the exact moment it was about to
  // say the deck was safe.
  const steps: Partial<Record<IntakeStepKey, { note?: string }>> = done.steps ?? {}

  // Walked over the titles rather than over the wire's own keys, because that
  // record is complete by construction: no step can arrive with something to
  // say and be dropped for want of a name. Its order is the sheet's order.
  return (Object.keys(INTAKE_TITLES) as IntakeStepKey[]).flatMap((key) => {
    const note = steps[key]?.note
    return note ? [{ key, title: INTAKE_TITLES[key], note }] : []
  })
}
