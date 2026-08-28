import { describe, expect, it } from 'vitest'

import { type CounterMoment } from './board'
import { COUNTER_MEANS, counterMeans, counterSaid, counterSign, counterStory }
  from './counters'

/** A moment, spelled once so the tests below read as the account they are
 *  about rather than as four fields each. */
function moment(kind: string, was: number, now: number, turn: number):
CounterMoment {
  return { kind, was, now, turn }
}

describe('counterSign', () => {
  it('reads the sign off the kind and never off the count', () => {
    // The whole bug this function was written for: a single -1/-1 arrives as
    // `{kind: '-1/-1', n: 1}`, and `n` is a count that is never negative.
    expect(counterSign('-1/-1')).toBe('down')
    expect(counterSign('+1/+1')).toBe('up')
  })

  it('calls everything else neither', () => {
    for (const kind of ['Charge', 'Loyalty', 'Stun', 'Quest', 'Oil']) {
      expect(counterSign(kind)).toBe('flat')
    }
  })
})

describe('counterMeans', () => {
  it('builds a sized counter\'s sentence from its own figures', () => {
    expect(counterMeans('+1/+1')).toContain('one bigger in both figures')
    expect(counterMeans('+2/+2')).toContain('two bigger in both figures')
  })

  it('says what dying looks like for a shrinking counter, and only there', () => {
    expect(counterMeans('-1/-1')).toContain('toughness reaches nought dies')
    expect(counterMeans('+1/+1')).not.toContain('dies')
  })

  it('does not assume both halves are the same figure', () => {
    // A `+1/+0` is a real counter and the tidy sentence would be wrong about
    // it in a way nobody would notice from the common case.
    expect(counterMeans('+1/+0'))
      .toContain('one bigger in power and nought in toughness')
  })

  it('handles a counter whose halves disagree without inventing a reading', () => {
    const said = counterMeans('+2/-1')
    expect(said).toContain('power by +2')
    expect(said).toContain('toughness by -1')
  })

  it('answers the named counters from the table', () => {
    expect(counterMeans('Loyalty')).toBe(COUNTER_MEANS.loyalty)
    // Forge builds a display name off an enum constant, so the case that
    // reaches a browser is Forge's business and not this board's.
    expect(counterMeans('STUN')).toBe(COUNTER_MEANS.stun)
    expect(counterMeans('stun')).toBe(COUNTER_MEANS.stun)
  })

  it('sends an unknown counter to the card that made it', () => {
    // Not a shrug: a charge counter's rule really is printed on its own card,
    // and a general sentence about charge counters would be invented.
    const said = counterMeans('Charge')
    expect(said).toContain('written on the card that put them here')
    expect(counterMeans('Petal')).toBe(said)
  })
})

describe('counterStory', () => {
  it('says an arrival as an arrival and everything after as an addition', () => {
    const history = [moment('+1/+1', 0, 2, 4), moment('+1/+1', 2, 3, 6)]
    expect(counterStory(history, '+1/+1'))
      .toBe('two on turn 4, one more on turn 6.')
  })

  it('says a removal', () => {
    expect(counterStory([moment('-1/-1', 3, 1, 7)], '-1/-1'))
      .toBe('two off on turn 7.')
  })

  it('reads only its own kind', () => {
    const history = [moment('+1/+1', 0, 1, 2), moment('Charge', 0, 3, 2)]
    expect(counterStory(history, 'Charge')).toBe('three on turn 2.')
  })

  it('elides an account longer than the panel, and says it elided', () => {
    const history = [1, 2, 3, 4, 5].map((t) => moment('+1/+1', t - 1, t, t))
    const said = counterStory(history, '+1/+1')
    expect(said).toBe('… one more on turn 3, one more on turn 4, '
      + 'one more on turn 5.')
  })

  it('phrases a trimmed account\'s first moment as an addition', () => {
    // The elided version above must not open with "one on turn 3" — there were
    // two before it, and that phrasing would claim the account starts here.
    const history = [1, 2, 3, 4, 5].map((t) => moment('+1/+1', t - 1, t, t))
    expect(counterStory(history, '+1/+1')).not.toContain('one on turn 3')
  })

  it('is silent about a card that walked on already wearing them', () => {
    expect(counterStory([], '+1/+1')).toBeUndefined()
    expect(counterStory([moment('Charge', 0, 1, 3)], '+1/+1')).toBeUndefined()
  })

  it('drops a moment in which nothing actually moved', () => {
    expect(counterStory([moment('+1/+1', 2, 2, 5)], '+1/+1')).toBeUndefined()
  })
})

describe('counterSaid', () => {
  it('puts the count in the heading and keeps it out of the meaning', () => {
    const said = counterSaid('+1/+1', 3, [])
    expect(said.name).toBe('3 +1/+1 counters')
    expect(said.says).not.toContain('3')
  })

  it('counts one counter singular', () => {
    expect(counterSaid('Stun', 1, []).name).toBe('1 Stun counter')
  })

  it('carries the account as the note when there is one', () => {
    const said = counterSaid('+1/+1', 3, [moment('+1/+1', 0, 3, 4)])
    expect(said.note).toBe('three on turn 4.')
  })

  it('has no note at all rather than an empty one', () => {
    // A line reading "no account" points at a hole instead of at the card.
    expect(counterSaid('+1/+1', 3, []).note).toBeUndefined()
  })
})
