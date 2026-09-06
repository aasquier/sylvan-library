import { describe, expect, it } from 'vitest'

import type { CardOffer, CardPlayable } from './api'
import { cardAllowance, cardWarning, outsideIdentity } from './cardoffer'

/**
 * The two readings the card finder shows before anything is written — and the
 * one that used to lie.
 *
 * Until 2026-09-06 the identity half was worked out here, from `color_identity`
 * against the commander's colours. Mystery Booster Commander Edition printed
 * eight commanders that widen colour identity (ADR 51), and this file began
 * telling people that a card their own commander expressly allows "cannot go in
 * this deck" — a sentence written to be believed by a beginner, and false.
 *
 * So the verdict now rides in on the offer, and these tests hold both halves:
 * the server's answer is used when there is one, and the local reading survives
 * for the surfaces that have no deck to measure against.
 */

function offer(over: Partial<CardOffer> = {}): CardOffer {
  return {
    name: 'Firja, Judge of Valor', mana_cost: '{2}{W}{B}',
    type_line: 'Legendary Creature — Angel Cleric', oracle_text: 'Flying, lifelink',
    color_identity: ['B', 'W'], image: null, artist: null,
    legal_commander: true, is_land: false, score: 1, via: 'exact', ...over,
  }
}

function playable(over: Partial<CardPlayable> = {}): CardPlayable {
  return { ok: true, banned: false, outside: [], allowed_by: '', ...over }
}

describe('the local reading, for a card nobody measured', () => {
  it('names the colours the commander cannot carry', () => {
    expect(outsideIdentity(offer(), ['W'])).toEqual(['B'])
    expect(cardWarning(offer(), ['W']))
      .toBe('Firja, Judge of Valor is black, and your commander is not — '
        + 'so it cannot go in this deck.')
  })

  it('says nothing about a card inside the identity', () => {
    expect(cardWarning(offer(), ['B', 'W'])).toBe('')
  })
})

describe('the library\'s own answer, when the request named a deck', () => {
  // The bug, exactly: mono-white commander, black-and-white Angel, and a
  // clause that allows it. The local reading would refuse; the server's does
  // not, and the server's is the one that counts.
  it('does not second-guess a card the library has cleared', () => {
    const card = offer({ playable: playable({ allowed_by: 'Seluma, Light of Aysen' }) })
    expect(cardWarning(card, ['W'])).toBe('')
  })

  it('says whose rule let an off-colour card in', () => {
    const card = offer({ playable: playable({ allowed_by: 'Seluma, Light of Aysen' }) })
    expect(cardAllowance(card))
      .toBe('Firja, Judge of Valor sits outside your commander\'s colours, and '
        + 'Seluma, Light of Aysen\'s Rulebreaker allows it anyway.')
  })

  // The other direction, and the reason this is not just "trust the server
  // when it says yes": a card the clause does NOT cover is still refused, and
  // still named by the colours it actually carries.
  it('still refuses a card no clause covers', () => {
    const demon = offer({
      name: 'Rune-Scarred Demon', type_line: 'Creature — Demon',
      color_identity: ['B'], playable: playable({ ok: false, outside: ['B'] }),
    })
    expect(cardWarning(demon, ['W']))
      .toBe('Rune-Scarred Demon is black, and your commander is not — '
        + 'so it cannot go in this deck.')
    expect(cardAllowance(demon)).toBe('')
  })

  it('lets the format refuse what no commander bends', () => {
    const banned = offer({ playable: playable({ ok: false, banned: true }) })
    expect(cardWarning(banned, ['B', 'W']))
      .toBe('Firja, Judge of Valor is banned in Commander, so this deck will not take it.')
  })

  // A payload from before the key existed lands in a browser that has it.
  // Undefined is "nobody asked", never "no".
  it('falls back to the local reading when the key is absent', () => {
    expect(cardWarning(offer({ playable: undefined }), ['W'])).not.toBe('')
    expect(cardAllowance(offer({ playable: undefined }))).toBe('')
  })

  // An allowance is only worth saying about a card that is actually in.
  it('says nothing about an allowance on a card that is out', () => {
    const card = offer({ playable: playable({ ok: false, outside: ['B'], allowed_by: 'Someone' }) })
    expect(cardAllowance(card)).toBe('')
  })
})
