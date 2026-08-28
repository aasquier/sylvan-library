import { describe, expect, it } from 'vitest'

import type { BoardCard, BoardStack, Clash } from './board'
import { BOUT_BLOCKER_W, boutAt, boutLife, boutPitch,
  stagedBout } from './stage'

/** A creature, at the one detail the bout reads: a name and an id. */
const beast = (id: number, name: string): BoardCard => ({
  id, name, token: false, types: 'Creature - Cat',
  image: '', art: '', artist: '', zone: 'battlefield', seat: 1, tapped: false,
  mana: false, makes: [], keywords: [], leaving: null, power: 2, toughness: 2,
  counters: [], counterHistory: [], combat: 'blocking', attacking: 0,
  blocking: 1, casts: 0, attachedTo: 0, attachments: [], live: [],
  granted: [], fate: '', copiedBy: 0,
})
const stack = (card: BoardCard, count = 1): BoardStack =>
  ({ card, count, ids: [card.id] })
const clash = (blockers: BoardStack[]): Clash => ({
  attacker: beast(1, 'Ghalta, Primal Hunger'),
  blockers,
  swinging: 'far',
})

describe('how wide a rank of blockers stands', () => {
  it('keeps its natural pitch while the rank still fits', () => {
    // One card and a fourteen-pixel gap, against a stage 940 wide — the two
    // numbers the layout was drawn and measured at.
    const natural = BOUT_BLOCKER_W + 14 / 940
    for (const n of [1, 2, 3, 4, 5, 6, 7]) {
      expect(boutPitch(n)).toBeCloseTo(natural, 6)
    }
  })

  it('overlaps past seven rather than shrinking the cards', () => {
    // Aaron chose the charge knowing it runs out of stage: seven blockers span
    // 874 of 940 pixels and an eighth does not fit. Shrinking would make a
    // rare board illegible to punish it for being rare, so the cards keep
    // their size and close up shoulder to shoulder instead.
    expect(boutPitch(8)).toBeLessThan(boutPitch(7))
    expect(boutPitch(12)).toBeLessThan(boutPitch(8))
  })

  it('never lets a rank run off the stage, however big the gang', () => {
    // The property the pitch exists for, checked rather than trusted: the last
    // card's right edge stays inside the span at every size a real board can
    // reach. Twenty is far past anything Forge has produced and is the point.
    for (let n = 1; n <= 20; n++) {
      expect(boutAt(0, n)).toBeGreaterThanOrEqual(0)
      expect(boutAt(n - 1, n) + BOUT_BLOCKER_W).toBeLessThanOrEqual(1)
    }
  })

  it('centres the rank on the attacker whether it is odd or even', () => {
    // The attacker stands at the middle of the stage, so the wall has to be
    // centred there too — a rank packed from the left would put a lone blocker
    // opposite nothing at all.
    for (const n of [1, 2, 3, 4, 5, 8]) {
      const first = boutAt(0, n)
      const last = boutAt(n - 1, n) + BOUT_BLOCKER_W
      expect((first + last) / 2).toBeCloseTo(0.5, 6)
    }
  })
})

describe('the fight the stage is handed', () => {
  it('is nothing at all when the beat is not a block', () => {
    expect(stagedBout(null, null, 'k', 'play')).toBeNull()
  })

  it('names the attacker on the plate and the wall underneath it', () => {
    // "Blocked" over the attacker's name, because a block is the one beat
    // where the interesting party is the creature being stopped rather than
    // the player doing the stopping — every other plate on this stage is a
    // sentence about somebody doing something.
    const out = stagedBout(clash([stack(beast(2, 'Regal Caracal')),
      stack(beast(3, 'Sacred Cat'))]), null, 'k', 'play')
    expect(out?.word).toBe('Blocked')
    expect(out?.attacker.name).toBe('Ghalta, Primal Hunger')
    expect(out?.note).toBe('by Regal Caracal and Sacred Cat')
  })

  it('says how many when a card stands for several', () => {
    // The stack's count belongs in the sentence too. "by 5 Saproling" rather
    // than five identical names, which is what a player would say and what the
    // card itself is already showing.
    const out = stagedBout(clash([stack(beast(2, 'Saproling'), 5)]),
      null, 'k', 'play')
    expect(out?.note).toBe('by Saproling ×5')
    expect(out?.blockers[0]?.count).toBe(5)
  })

  it('cuts a legend at its title, because half of them carry a comma', () => {
    // "Brimaz, King of Oreskos, Arahbo, Roar of the World" is four names to a
    // reader and two to the game. The room's own `shortName` is the cut.
    const out = stagedBout(clash([stack(beast(2, 'Brimaz, King of Oreskos')),
      stack(beast(3, 'Arahbo, Roar of the World'))]), null, 'k', 'play')
    expect(out?.note).toBe('by Brimaz and Arahbo')
  })

  it('stops naming after three and counts the rest in creatures', () => {
    // Nine names ran the width of the arena and wrapped under the cards they
    // were describing. What is left is counted in creatures rather than cards:
    // a stack of six and a bear is seven more, not two.
    const out = stagedBout(clash([
      stack(beast(2, 'Sacred Cat')), stack(beast(3, 'Regal Caracal')),
      stack(beast(4, 'Fleecemane Lion')), stack(beast(5, 'Saproling'), 6),
      stack(beast(6, 'Bear Cub')),
    ]), null, 'k', 'play')
    expect(out?.note).toBe('by Sacred Cat, Regal Caracal, Fleecemane Lion and 7 more')
  })

  it('carries each blocker its own board id', () => {
    // The identity that makes a gang assemble rather than redraw: keyed on the
    // id, a card already standing keeps its element when the next beat brings
    // one more, so only the arriving card plays its arrival.
    const out = stagedBout(clash([stack(beast(7, 'A')), stack(beast(9, 'B'))]),
      null, 'k', 'play')
    expect(out?.blockers.map((b) => b.id)).toEqual([7, 9])
  })

  it('is watched longer than a single card, and still capped by pace', () => {
    // There are N + 1 cards to read here instead of one, so it holds longer —
    // but a fast pace must still not leave a fight standing over four later
    // beats, which is `stageLife`'s own cap and argument.
    expect(boutLife('paused')).toBe(2000)
    expect(boutLife('fast')).toBeLessThan(boutLife('study'))
    expect(boutLife('fast')).toBeGreaterThanOrEqual(620)
  })
})
