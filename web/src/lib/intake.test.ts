/**
 * What the finished intake has to say for itself.
 *
 * **The server has been writing these sentences all along and nothing read
 * them.** Every step carries a `note` for the moments its two numbers would
 * read as a failure, and the import page navigated to the deck the instant the
 * job resolved — so the run's account of itself was written, serialised, and
 * dropped on the floor. The sentence that matters most is the one naming a
 * card Claude had nothing to say about: leaving it out is the design, saying
 * nothing about it was not, and finding your own card meant reading
 * ninety-nine reasons looking for the gap.
 *
 * Tested here rather than only through the page because the ordering and the
 * completeness are arithmetic, and a DOM assertion that "three things
 * rendered" cannot tell a correct order from a lucky one.
 */

import { describe, expect, it } from 'vitest'

import { INTAKE_TITLES, intakeAftermath } from './intake'
import type { IntakeResult, IntakeStepKey } from './api'

/** A finished run, as the job's `result` carries it. */
function finished(over: Partial<IntakeResult> = {}): IntakeResult {
  return { slug: 'arahbo-cats', asked: true, steps: {}, ...over }
}

describe('what a finished intake has to say', () => {
  it('says nothing when every step went cleanly', () => {
    // The ordinary run, and the one that must not stop the page: three actions
    // that each did what was asked and wrote no sentence about it.
    expect(intakeAftermath(finished({
      steps: {
        categories: { changed: 99, considered: 99 },
        rationales: { changed: 99, considered: 99 },
        dossier: { changed: 1, considered: 1 },
      },
    }))).toEqual([])
  })

  it('carries the sentence naming the card it left, under the action’s name',
     () => {
    const said = intakeAftermath(finished({
      steps: {
        rationales: {
          changed: 84, considered: 85,
          note: 'No reason was drafted for Virtue of Persistence — that one '
            + 'is yours to write.',
        },
      },
    }))
    expect(said).toHaveLength(1)
    expect(said[0]!.key).toBe('rationales')
    // The sheet's own words for the thing that was ticked, so the account of
    // what happened and the choice that caused it are named the same.
    expect(said[0]!.title).toBe('Draft the reasons')
    expect(said[0]!.note).toContain('Virtue of Persistence')
  })

  // The wire is a Go map's worth of keys and its order is not a decision
  // anybody made; the sheet's order is. A reader going down this list should
  // meet the actions in the order they were offered them.
  it('reads the actions back in the order the sheet offers them', () => {
    const said = intakeAftermath(finished({
      steps: {
        argue: { changed: 0, considered: 4, note: 'the sweep stopped' },
        rationales: { changed: 1, considered: 2, note: 'one was left' },
        categories: { changed: 3, considered: 4, note: 'one stayed put' },
      },
    }))
    expect(said.map((row) => row.key))
      .toEqual(['categories', 'rationales', 'argue'])
  })

  // **A step with something to say must never be dropped for want of a name.**
  // The titles are a total record over the wire's own key union, so the
  // typechecker already refuses an unnamed action — this drives all five to
  // prove the record is what the reading actually walks.
  it('has a name for every action the wire can report on', () => {
    const every: IntakeStepKey[] =
      ['categories', 'rationales', 'description', 'dossier', 'argue']
    for (const key of every) {
      const said = intakeAftermath(finished({
        steps: { [key]: { changed: 0, considered: 1, note: 'a sentence' } },
      }))
      expect(said).toHaveLength(1)
      expect(said[0]!.title).toBe(INTAKE_TITLES[key])
      expect(said[0]!.title).not.toBe('')
    }
  })

  // The whole run refused — the stance would not speak — and `reason` is the
  // only thing it has to say. It is about no single action, so it wears no
  // action's name.
  it('carries the refusal when nothing was asked at all', () => {
    const said = intakeAftermath(finished({
      asked: false,
      reason: 'Claude is turned off for this deck, so nothing was asked.',
    }))
    expect(said).toHaveLength(1)
    expect(said[0]!.key).toBe('asked')
    expect(said[0]!.title).toBe('')
    expect(said[0]!.note).toContain('turned off')
  })

  it('says nothing about a refusal that gave no reason', () => {
    expect(intakeAftermath(finished({ asked: false }))).toEqual([])
  })

  // **A page that crashed on the way to reporting good news is the worse
  // failure by a distance.** The deck is already saved by the time any of this
  // runs, so a result that is not the shape expected — an older instance, a
  // job of another kind, a null — has nothing to say rather than a throw.
  it('has nothing to say about a result it does not recognise', () => {
    expect(intakeAftermath(null)).toEqual([])
    expect(intakeAftermath(undefined)).toEqual([])
    expect(intakeAftermath('done')).toEqual([])
    expect(intakeAftermath(7)).toEqual([])
    expect(intakeAftermath({})).toEqual([])
  })
})
