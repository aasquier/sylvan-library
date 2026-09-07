import { describe, expect, it } from 'vitest'

import type { BoardCard, BoardStack, Clash } from './board'
import { BOUT_BLOCKER_W, boutAt, boutColumn, boutFacing, boutLife, boutPitch,
  seatNames, stagedBout } from './stage'

/** Four seats, named as the room names them — the measured game's own table.
 *  Seats 0 and 1 across the top, 2 and 3 across the bottom. */
const POD = ['Arahbo', 'Atla Palani', 'Goreclaw', 'Gyome']

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
const clash = (blockers: BoardStack[], from = 0, at = 1): Clash => ({
  attacker: beast(1, 'Ghalta, Primal Hunger'),
  blockers,
  from,
  at,
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

  it('says one name once, however many cards carry it', () => {
    // Found on a real board: a gang on Blightsteel Colossus put three Cat
    // Tokens in the wall, and they are three separate CARDS on purpose — one a
    // 6/4 carrying Hammer of Nazahn, one a 3/3 with a Basilisk Collar, one a
    // bare 3/3. The rank draws all three, because a player can see that. The
    // plate must not: "by Cat Token, Cat Token, Elephant Token and 1 more"
    // looks like a fault however true it is.
    const out = stagedBout(clash([
      stack(beast(2, 'Cat Token')), stack(beast(3, 'Cat Token')),
      stack(beast(4, 'Elephant Token')), stack(beast(5, 'Cat Token')),
    ]), null, 'k', 'play')
    expect(out?.blockers).toHaveLength(4)
    expect(out?.note).toBe('by Cat Token ×3 and Elephant Token')
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

describe('a fight that has settled', () => {
  const wall = [stack(beast(2, 'Regal Caracal'))]

  it('says nothing about an outcome while it is only declared', () => {
    // A block declares a fight; a death settles one, a median of thirty-five
    // beats later. Between the two the room must not claim a result.
    const out = stagedBout(clash(wall), null, 'k', 'play')
    expect(out?.outcome).toBeNull()
    expect(out?.word).toBe('Blocked')
  })

  it('changes tense once the damage has landed', () => {
    // Three words for three moments, and the tense is the whole of it: a fight
    // is *Blocked* while it is being declared and something that already
    // happened once it is over.
    expect(stagedBout(clash(wall), null, 'k', 'play', 'fell')?.word)
      .toBe('Cut down')
    expect(stagedBout(clash(wall), null, 'k', 'play', 'held')?.word)
      .toBe('Broke through')
  })

  it('carries the outcome through so the arena can be chosen from it', () => {
    // The scene is picked off this and nothing else — the ossuary if the
    // creature that swung was cut down, the arch if it went through.
    expect(stagedBout(clash(wall), null, 'k', 'play', 'fell')?.outcome)
      .toBe('fell')
    expect(stagedBout(clash(wall), null, 'k', 'play', 'held')?.outcome)
      .toBe('held')
  })

  it('still names the whole wall when the fight is over', () => {
    // The verdict answers for the attacker; the wall is still the sentence's
    // subject matter, and a mixed result must not quietly drop the survivors.
    const out = stagedBout(clash([stack(beast(2, 'Sacred Cat')),
      stack(beast(3, 'Cat Token'), 3)]), null, 'k', 'play', 'held')
    expect(out?.note).toBe('by Sacred Cat and Cat Token ×3')
  })
})

describe('which edge of the arena a seat becomes', () => {
  it('is the duel it has always been at two seats', () => {
    // Seat 0 is the far half and seat 1 the near one, which is what the board
    // handed the stage directly before a table could hold four people.
    expect(boutFacing(0, 2)).toBe('far')
    expect(boutFacing(1, 2)).toBe('near')
  })

  it('reads a pod seat\'s row, because the two-by-two is how it is drawn', () => {
    // Seats 0 and 1 across the top of the table, 2 and 3 across the bottom —
    // `.field-quad-{n}` in DOM order — so a seat's row is which way it faces.
    expect(boutFacing(0, 4)).toBe('far')
    expect(boutFacing(1, 4)).toBe('far')
    expect(boutFacing(2, 4)).toBe('near')
    expect(boutFacing(3, 4)).toBe('near')
  })

  it('carries the seats through unreduced, which is the point of keeping them',
    () => {
      // `facing` is lossy on purpose and the fight's own geometry is not: two
      // different pairs collapse onto one edge, so a drawing that wants to
      // point at the real quadrants needs the pair rather than the word.
      const out = stagedBout(clash([stack(beast(2, 'Sacred Cat'))], 2, 1),
        null, 'k', 'play', null, null, POD)
      expect(out?.from).toBe(2)
      expect(out?.at).toBe(1)
      expect(out?.seats).toBe(4)
      expect(out?.facing).toBe('near')
    })
})

describe('where at the table a fight is drawn', () => {
  it('gives a duel the whole stage, which is what it always had', () => {
    // Every rule this room drew a fight by before there was a table wide
    // enough to have columns: one centre, one span, both ranks on it.
    expect(boutColumn(0, 2)).toEqual(boutColumn(1, 2))
    expect(boutColumn(0, 2).centre).toBe(0.5)
    // And the rank arithmetic comes out identical to the two-argument call.
    for (const n of [1, 3, 8]) {
      const { centre, span } = boutColumn(1, 2)
      expect(boutAt(0, n, centre, span)).toBeCloseTo(boutAt(0, n), 9)
    }
  })

  it('puts each pod seat over its own column, seated clockwise', () => {
    // **Not reading order.** The table is seated 1-2 across the top and 4-3
    // across the bottom so the turn walks the rim rather than zig-zagging —
    // `seatSpot` owns that and this reads it. A fight drawn against reading
    // order would stand over the wrong quadrant while looking perfectly right
    // on its own, which is why these two cannot be allowed to drift apart.
    expect(boutColumn(0, 4).centre).toBeCloseTo(0.25, 9)  // seat 1, top left
    expect(boutColumn(1, 4).centre).toBeCloseTo(0.75, 9)  // seat 2, top right
    expect(boutColumn(2, 4).centre).toBeCloseTo(0.75, 9)  // seat 3, under it
    expect(boutColumn(3, 4).centre).toBeCloseTo(0.25, 9)  // seat 4, bottom left
  })

  it('keeps a pod rank inside its own column, however big the gang', () => {
    // The property the column exists for. A wall that ran past its column
    // would be standing over the seat next door, which is the whole fault this
    // was ruled to fix — and the left column must not run off the stage
    // either. Twenty is far past anything Forge has produced and is the point.
    for (const seat of [0, 1, 2, 3]) {
      const { centre, span } = boutColumn(seat, 4)
      const left = centre - span / 2
      const right = centre + span / 2
      for (let n = 1; n <= 20; n++) {
        expect(boutAt(0, n, centre, span)).toBeGreaterThanOrEqual(left - 1e-9)
        expect(boutAt(n - 1, n, centre, span) + BOUT_BLOCKER_W)
          .toBeLessThanOrEqual(right + 1e-9)
      }
    }
  })

  it('overlaps a pod wall sooner, which is the cost that was chosen', () => {
    // A column is half the stage, so the rank runs out of room at four cards
    // rather than eight. Overlapping rather than shrinking is the standing
    // rule — a card's size must not change for reasons that have nothing to do
    // with the card — so what a column costs is pitch, never width.
    const pod = boutColumn(0, 4).span
    expect(boutPitch(3, pod)).toBeCloseTo(boutPitch(3), 9)
    expect(boutPitch(4, pod)).toBeLessThan(boutPitch(4))
  })

  it('centres the rank on its own column, odd or even', () => {
    for (const seat of [0, 3]) {
      const { centre, span } = boutColumn(seat, 4)
      for (const n of [1, 2, 3, 5]) {
        const first = boutAt(0, n, centre, span)
        const last = boutAt(n - 1, n, centre, span) + BOUT_BLOCKER_W
        expect((first + last) / 2).toBeCloseTo(centre, 9)
      }
    }
  })
})

describe('what the plate calls a seat', () => {
  // The real library, 2026-09-07: not one of the twenty-five decks is named
  // after its commander, which is why the deck's title was never going to be
  // the short form.
  const seat = (general: string, deck: string) => ({ general, deck })

  it('is the general, which is how somebody says it at a table', () => {
    expect(seatNames([
      seat('Atla Palani', 'Life, Uh, Finds a Way'),
      seat('Gyome', 'Kitchen Nightmares'),
      seat('Korvold', 'Eat the Rich'),
      seat('Arahbo', 'Armed and Feline'),
    ])).toEqual(['Atla Palani', 'Gyome', 'Korvold', 'Arahbo'])
  })

  it('falls back to the deck when a general is not this table\'s only one',
    () => {
      // Three of the library's decks run Atraxa. Two of them in one pod would
      // read "Atraxa's, by Atraxa's ...", which is worse than the long version
      // because it is ambiguous rather than merely wordy.
      expect(seatNames([
        seat('Atraxa', 'The Last Chapter Comes Early'),
        seat('Atraxa', 'Counters Without Number'),
        seat('Gyome', 'Kitchen Nightmares'),
        seat('Arahbo', 'Armed and Feline'),
      // The two Atraxa seats lengthen; the other two keep the short name they
      // were entitled to, because a collision is a fact about two chairs.
      ])).toEqual(['The Last Chapter Comes Early', 'Counters Without Number',
        'Gyome', 'Arahbo'])
    })

  it('lengthens only the seats that are ambiguous, never the whole table', () => {
    // The property, stated: a collision is a fact about two chairs and the
    // other two keep the short name they were entitled to.
    const out = seatNames([
      seat('Atraxa', 'The Loyal Opposition'),
      seat('Atraxa', 'Ten Counters, No Cure'),
      seat('Tivit', 'Filibuster on the Floor'),
      seat('Gyome', 'Kitchen Nightmares'),
    ])
    expect(out.slice(2)).toEqual(['Tivit', 'Gyome'])
  })

  it('is the deck when the board never showed a commander', () => {
    // A match played without the scribe carries no command zone at all, and a
    // fact the board has not got is not one to invent.
    expect(seatNames([seat('', 'Eat the Rich'), seat('Gyome', 'Kitchen Nightmares')]))
      .toEqual(['Eat the Rich', 'Gyome'])
    // Two seats with nothing do not collide with each other into a fallback
    // they were already taking.
    expect(seatNames([seat('', 'Eat the Rich'), seat('', 'Armed and Feline')]))
      .toEqual(['Eat the Rich', 'Armed and Feline'])
  })

  it('names both halves of a pairing', () => {
    expect(seatNames([seat('Thrasios & Tymna', 'Squires of Zhalfir'),
      seat('Gyome', 'Kitchen Nightmares')])[0]).toBe('Thrasios & Tymna')
  })
})

describe('the sentence a real four-player match produces', () => {
  // **The live library's own seats**, taken off match `2e86a6e129ed` on
  // 2026-09-07 — the first four-seat game walked on the deployed instance.
  // These two beats are why the plate stopped naming a seat by its deck.
  const LIVE = seatNames([
    { general: 'Arahbo', deck: 'Armed and Feline' },
    { general: 'Atla Palani', deck: 'Life, Uh, Finds a Way' },
    { general: 'Korvold', deck: 'Eat the Rich' },
    { general: 'Gyome', deck: 'Kitchen Nightmares' },
  ])
  const fight = (attacker: string, wall: BoardStack[], from: number, at: number)
  : Clash => ({ attacker: beast(1, attacker), blockers: wall, from, at })

  it('names two seats without spending a line on two deck titles', () => {
    // Was: "Life, Uh, Finds a Way’s, by Eat the Rich’s Pest Token ×3".
    expect(stagedBout(fight('Zacama, Primal Calamity',
      [stack(beast(2, 'Pest Token'), 3)], 1, 2), null, 'k', 'play', null, null,
    LIVE)?.note).toBe('Atla Palani\u2019s, by Korvold\u2019s Pest Token \u00d73')
  })

  it('says almost nothing when the generals are the fight', () => {
    // Was: "Life, Uh, Finds a Way’s, by Kitchen Nightmares’ Gyome" — and both
    // halves of it were repeating a name already on the plate. The attacker is
    // its own seat's general and the title line above says so; the wall is its
    // seat's general and the card in the picture says so. What is left is the
    // one fact neither of them carries.
    expect(stagedBout(fight('Atla Palani, Nest Tender',
      [stack(beast(3, 'Gyome, Master Chef'))], 1, 3), null, 'k', 'play', null,
    null, LIVE)?.note).toBe('by Gyome')
  })
})

describe('what the plate says when four people are at the table', () => {
  const wall = [stack(beast(2, 'Ygra, Eater of All')),
    stack(beast(3, 'Greta, Sweettooth Scourge'))]

  it('leaves a duel\'s sentence exactly as it was', () => {
    // Not caution — *"by Ygra and Greta"* is missing nothing when there is only
    // one other player it could mean.
    expect(stagedBout(clash(wall), null, 'k', 'play')?.note)
      .toBe('by Ygra and Greta')
    expect(stagedBout(clash(wall), null, 'k', 'play', null, null,
      ['Arahbo', 'Atla Palani'])?.note).toBe('by Ygra and Greta')
  })

  it('names both players at a pod, which the geometry cannot', () => {
    // Aaron ruled it 2026-09-07. At four seats the drawing can point at a
    // column at best, and on a phone — one seat open, no two-by-two — it can
    // point at nothing at all.
    expect(stagedBout(clash(wall, 2, 3), null, 'k', 'play', null, null, POD)
      ?.note).toBe('Goreclaw\u2019s, by Gyome\u2019s Ygra and Greta')
  })

  it('writes a name that already ends in s with the apostrophe alone', () => {
    // *Thantis'*, not *Thantis's*, because a commander's name is a name. The
    // Coliseum seats whatever the library holds and Magic prints plenty.
    const seated = ['Thantis', 'Atla Palani', 'Goreclaw', 'Gyome']
    expect(stagedBout(clash(wall, 0, 3), null, 'k', 'play', null, null, seated)
      ?.note).toBe('Thantis\u2019, by Gyome\u2019s Ygra and Greta')
  })

  it('does not name a commander twice when it is in the fight itself', () => {
    // The room calls a deck by its general, so the moment the general is *in*
    // the fight the sentence stutters. Both of these came off the measured
    // game and both look like a fault however true they are.
    const gyome = [stack(beast(2, 'Gyome, Master Chef'))]
    // The wall is Gyome, in Gyome's seat: *"by Gyome"*, not *"by Gyome's
    // Gyome"*. The seat is still named — by the card standing in it.
    expect(stagedBout(clash(gyome, 2, 3), null, 'k', 'play', null, null, POD)
      ?.note).toBe('Goreclaw\u2019s, by Gyome')
    // And the attacker's own seat, when the plate's title above it is already
    // that seat's name. `clash`'s attacker is Ghalta; seat 1 is Atla Palani,
    // so this one keeps its possessive and the next drops it.
    expect(stagedBout(clash(gyome, 1, 3), null, 'k', 'play', null, null, POD)
      ?.note).toBe('Atla Palani\u2019s, by Gyome')
  })

  it('drops only the name that is already said, never the other one', () => {
    // The rule is about repetition and not about commanders: a possessive
    // carrying information stays, in the same sentence as one that is not.
    const wall = [stack(beast(2, 'Gyome, Master Chef')),
      stack(beast(3, 'Greta, Sweettooth Scourge'))]
    expect(stagedBout(clash(wall, 2, 3), null, 'k', 'play', null, null, POD)
      ?.note).toBe('Goreclaw\u2019s, by Gyome and Greta')
  })

  it('falls back to the duel sentence when a seat has no name', () => {
    // A hole in the shelf is not a reason to print an empty possessive.
    expect(stagedBout(clash(wall, 2, 3), null, 'k', 'play', null, null,
      ['A', 'B', '', 'D'])?.note).toBe('by Ygra and Greta')
  })
})
