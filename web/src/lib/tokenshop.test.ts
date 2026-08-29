/**
 * The shopping list's rules.
 *
 * This is the half of the token export a suite can actually hold: the name a
 * shop files a token under, the merge across printings, and the number. What
 * it looks like is Aaron's walk, as ever — jsdom lays nothing out, and a list
 * that reads correctly here can still be unreadable on a phone.
 *
 * Every fixture below is a shape the real library actually produced: measured
 * against eight decks through `/api/decks/{owner}/{slug}/tokens` on 2026-08-29,
 * not invented. Arahbo really does make four different Cats; Gyome really does
 * have twenty cards that make a Food.
 */
import { expect, it } from 'vitest'

import type { TokenPlate } from './api'
import { shoppingList, shoppingText } from './tokenshop'

function plate(name: string, over: Partial<TokenPlate> = {}): TokenPlate {
  return {
    name,
    type_line: `Token Creature — ${name}`,
    image: `https://cards.scryfall.io/normal/front/a/b/${name}.jpg`,
    art_crop: `https://cards.scryfall.io/art_crop/front/a/b/${name}.jpg`,
    artist: 'Randy Gallegos',
    set_code: 'TELD',
    set_name: 'Throne of Eldraine Tokens',
    made_by: ['Gyome, Master Chef'],
    ...over,
  }
}

// The shelf is a row of *printings* — Arahbo's deck makes Cats from four
// different sets and shows four plates, because they are four different
// paintings. A shop has one product called Cat Token. Four lines of
// `1 Cat Token` arrive in a basket as four separate entries of the same thing,
// which is the bug this merge exists to not have.
it('merges the printings of one token into a single line', () => {
  expect(shoppingList([
    plate('Cat', { made_by: ['Leonin Warleader', 'Ocelot Pride'] }),
    plate('Cat', { made_by: ['Brimaz, King of Oreskos'] }),
    plate('Beast', { made_by: ['Terastodon'] }),
  ])).toEqual([
    { qty: 3, name: 'Cat Token' },
    { qty: 1, name: 'Beast Token' },
  ])
})

// A card that makes two different printings of the same token is still one
// card, and one card is one reason to own one token.
it('counts a card once however many printings it points at', () => {
  expect(shoppingList([
    plate('Spirit', { made_by: ['Bitterblossom'] }),
    plate('Spirit', { made_by: ['Bitterblossom'] }),
  ])).toEqual([{ qty: 1, name: 'Spirit Token' }])
})

// **The number the whole feature turns on.** Gyome's food deck has twenty
// cards in it that make a Food; `20 Food Token` would be true and useless,
// because those twenty are made across a game and eaten as they go.
it('never asks anybody to buy twenty Food tokens', () => {
  const twenty = Array.from({ length: 20 }, (_, i) => `Card ${String(i)}`)
  expect(shoppingList([plate('Food', { made_by: twenty })]))
    .toEqual([{ qty: 4, name: 'Food Token' }])
})

// The floor is one, never nought: a token the server sent with nobody making
// it still earns its line rather than a quantity nothing can buy.
it('gives a token with no makers a line of its own', () => {
  expect(shoppingList([plate('Clue', { made_by: [] })]))
    .toEqual([{ qty: 1, name: 'Clue Token' }])
})

// **The suffix is the whole translation between the two catalogues.** Shops
// file these as "Beast Token", never as "Beast", and a bare word would miss —
// or land on a real card wearing it.
it('asks for the name a shop actually sells the card under', () => {
  expect(shoppingList([plate('Treasure', { made_by: ['Smothering Tithe'] })]))
    .toEqual([{ qty: 1, name: 'Treasure Token' }])
})

// And never twice. The pool's own "Copy" token is the near miss here: a name
// one word away from carrying the suffix already.
it('never writes the word Token twice', () => {
  expect(shoppingList([plate('Copy Token', { made_by: ['Scute Swarm'] })]))
    .toEqual([{ qty: 1, name: 'Copy Token' }])
})

// **No set code and no collector number, and that is a decision rather than an
// omission.** A bulk-add box takes them; for a token they would all miss. The
// code we hold is Scryfall's token set (`TELD`, `MPR`), shops file a token
// under the parent set, and the printing we chose was chosen to be
// recognisable rather than to be in stock — the shelf deliberately draws a
// token's *earliest* painting.
it('writes the name alone, never the set the picture came from', () => {
  const lines = shoppingList([
    plate('Food', {
      set_code: 'TELD', set_name: 'Throne of Eldraine Tokens',
      made_by: ['Gyome, Master Chef', 'The Shire'],
    }),
    plate('Elephant', {
      set_code: 'MPR', set_name: 'Magic Player Rewards',
      made_by: ['Generous Gift'],
    }),
  ])
  expect(shoppingText(lines)).toBe('2 Food Token\n1 Elephant Token\n')
})

// Nothing in, nothing out — and not a lone newline, which pasted into a
// bulk-add box is a blank line somebody has to wonder about.
it('writes nothing at all for a deck that makes nothing', () => {
  expect(shoppingList([])).toEqual([])
  expect(shoppingText([])).toBe('')
})
