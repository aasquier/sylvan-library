import { describe, expect, it } from 'vitest'

import type { Combo, ComboCardRef } from './api'
import {
  blankDraft, headingOf, steps, toDraft, withEntryAt, withoutEntryAt,
} from './combos'

/**
 * The combos block's pure parts.
 *
 * The heading is the one that matters: a combo has no name field, so this is
 * the entry's only name and it has to derive the same string the server does.
 * The rest is the shape the block moves between — resolved on the way down,
 * names on the way back up — which is where a whole-block write is easiest to
 * get quietly wrong.
 */

function ref(name: string, in_deck = true): ComboCardRef {
  return { name, image: null, art_crop: null, in_deck }
}

function combo(over: Partial<Combo> = {}): Combo {
  return {
    cards: [ref('Axebane Guardian'), ref('High Alert')],
    produces: 'infinite colored mana',
    how: '1) Tap. 2) Untap. 3) Again.',
    setup: 'six mana',
    needs: null, cut: null,
    ...over,
  }
}

it('names a combo after its pieces, joined', () => {
  expect(headingOf(combo())).toBe('Axebane Guardian + High Alert')
  // One piece is a heading of one, not a heading with a stray separator.
  expect(headingOf(combo({ cards: [ref('Sol Ring')] }))).toBe('Sol Ring')
  expect(headingOf(combo({ cards: [] }))).toBe('')
})

describe('the instructions', () => {
  it('reads a numbered `how` as steps, with the markers stripped', () => {
    expect(steps('1) Tap the Guardian. 2) Pay to untap it. 3) Repeat.'))
      .toEqual(['Tap the Guardian.', 'Pay to untap it.', 'Repeat.'])
  })

  it('leaves prose as prose rather than inventing a structure', () => {
    // **The property, and it is the one a renderer gets wrong.** A `how`
    // written as a sentence is somebody's sentence; splitting it into steps
    // would be the page claiming a shape they did not write.
    expect(steps('Tap the Guardian, then untap it, and keep going.')).toBeNull()
    expect(steps('')).toBeNull()
    expect(steps('   ')).toBeNull()
  })

  // **The bug this test found.** Counting markers instead of requiring one at
  // the front read a stray "5)" mid-sentence as a list, and produced a first
  // step with no number at all: ["You need", "defenders for this to go
  // infinite."]. A numbered list starts at its first step.
  it('does not read a stray marker mid-sentence as a list', () => {
    expect(steps('You need 5) defenders for this to go infinite.')).toBeNull()
    expect(steps('Untap it, and 2) repeat as often as you like.')).toBeNull()
  })

  it('draws a one-step list as one step rather than as a marker in prose', () => {
    // Rare, but the alternative is rendering the literal "1)" in a paragraph,
    // which reads as a formatting mistake somebody made.
    expect(steps('1) Cast it and win.')).toEqual(['Cast it and win.'])
  })

  it('keeps mana symbols out of the numbering', () => {
    // `{2}` is a cost, not a step marker, and a split that read it as one
    // would cut a step in half at the exact point somebody is reading for.
    expect(steps('1) Tap for {2}. 2) Pay {2}{W}{U} to untap.'))
      .toEqual(['Tap for {2}.', 'Pay {2}{W}{U} to untap.'])
  })

  it('survives a `how` written across lines', () => {
    expect(steps('1) Equip.\n2) Untap for {3}.\n3) Repeat.'))
      .toEqual(['Equip.', 'Untap for {3}.', 'Repeat.'])
  })
})

describe('the block on its way back', () => {
  it('sends names rather than the references it was given', () => {
    const near = combo({
      cards: [ref('Axebane Guardian')],
      needs: ref('Umbral Mantle', false),
      cut: ref('Suspicious Bookcase'),
    })
    expect(toDraft(near)).toEqual({
      cards: ['Axebane Guardian'],
      produces: 'infinite colored mana',
      how: '1) Tap. 2) Untap. 3) Again.',
      setup: 'six mana',
      needs: 'Umbral Mantle',
      cut: 'Suspicious Bookcase',
    })
  })

  it('sends no trade at all when there is none', () => {
    const draft = toDraft(combo())
    expect(draft.needs).toBe('')
    expect(draft.cut).toBe('')
  })

  // **The mark is never sent.** `by` says Claude drafted an entry (ADR 41),
  // and a client that could send it could claim it — so it is not in the
  // shape at all, and the server carries it forward from the deck file.
  it('never sends a provenance mark, even off an entry that carries one', () => {
    expect(Object.keys(toDraft(combo({ by: 'claude' })))).not.toContain('by')
  })

  it('replaces one entry and leaves the rest of the block alone', () => {
    const block = [combo(), combo({ cards: [ref('Sol Ring')], produces: 'two mana' })]
    const next = withEntryAt(block, 1, { ...blankDraft(), cards: ['Black Lotus'], produces: 'three' })
    expect(next).toHaveLength(2)
    expect(next[0]?.cards).toEqual(['Axebane Guardian', 'High Alert'])
    expect(next[1]?.cards).toEqual(['Black Lotus'])
  })

  it('removes one entry and leaves the rest of the block alone', () => {
    const block = [combo(), combo({ cards: [ref('Sol Ring')], produces: 'two mana' })]
    const next = withoutEntryAt(block, 0)
    expect(next).toHaveLength(1)
    expect(next[0]?.cards).toEqual(['Sol Ring'])
  })
})
