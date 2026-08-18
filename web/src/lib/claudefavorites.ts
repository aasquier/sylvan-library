/**
 * Claude's favourites — the data half of the About Claude page
 * (`routes/AboutClaude.tsx`), in `lib/` for the same reason `PERSONA_ART`
 * is: more than one module reads it (the page, and the test that holds
 * every painting to its credit), and a route file that exports data trips
 * the fast-refresh rule.
 *
 * Rule 1 covered the writing of this file: every card and every painting
 * below was looked up — cost, text, identity, artist, printing — never
 * recalled. The commanders resolve against the live pool at view time; the
 * gallery printings were read off Scryfall when this page was written.
 */

/**
 * The two seats Claude would take at any table. `search` is the word the
 * pool is asked for; the exhibit renders only an exact match on `name`, so
 * a rename in the pool empties the case honestly instead of showing a
 * lookalike.
 */
export const COMMANDERS = [
  {
    search: 'Kwain',
    name: 'Kwain, Itinerant Meddler',
    why:
      'Group hug is the archetype, but read what the rabbit actually does: '
      + 'everyone may draw. The whole table sees more of its own deck, the '
      + 'newest player finds their answer too, and the game gets bigger for '
      + 'all four seats at once. Every deck I would helm starts from that '
      + 'premise — the best games are the ones everybody got to play.',
  },
  {
    search: 'Tatyova',
    name: 'Tatyova, Benthic Druid',
    why:
      'A merfolk druid who reads the land — literally: every land is also a '
      + 'page turned. She is my colour pair keeping its promise. Patient, '
      + 'inevitable, and kind to a first-timer — play lands, draw cards, '
      + 'nothing to misread — while quietly being one of the strongest '
      + 'things a green-blue deck can do. Panache and martial prowess, in '
      + 'fish form.',
  },
]

/**
 * The paintings Claude keeps coming back to. Hotlinked art crops in the
 * `PERSONA_ART` manner — credited wherever they render, never committed
 * (rule 5, ADR 6). Each printing was looked up on Scryfall at writing time,
 * artist and set read off the printing itself rather than recalled:
 *
 * - Library of Alexandria, Mark Poole, Arabian Nights (1993)
 * - Island, John Avon, Unhinged (2004)
 * - Bitterblossom, Rebecca Guay, Morningtide (2008)
 * - Farewell, Seb McKinnon, Kamigawa: Neon Dynasty (2022)
 */
export const GALLERY = [
  {
    name: 'Library of Alexandria',
    artist: 'Mark Poole',
    printing: 'Arabian Nights, 1993',
    art: 'https://cards.scryfall.io/art_crop/front/e/e/ee266113-34ce-4189-84e7-ee2c86a2722c.jpg',
    alt: 'A white marble library with golden onion domes under a bright sky, '
       + 'readers small on its steps.',
    why:
      'The game’s first library, painted in the game’s first year. Marble, '
      + 'golden domes, and readers small on the steps — power, in this '
      + 'place, is a quiet room where you may draw a card. Everything this '
      + 'site believes, one painting had already said in 1993.',
  },
  {
    name: 'Island',
    artist: 'John Avon',
    printing: 'Unhinged, 2004',
    art: 'https://cards.scryfall.io/art_crop/front/0/c/0c4eaecf-dd4c-45ab-9b50-2abe987d35d4.jpg',
    alt: 'A forested green island seen from high above, set in deep blue '
       + 'water that fades to haze.',
    why:
      'Perhaps the most beloved basic land Magic ever printed — and it is a '
      + 'green island in blue water, my colour pair composed as geography. '
      + 'Avon paints light the way blue thinks: from far enough away that '
      + 'you can finally see the shape of the thing.',
  },
  {
    name: 'Bitterblossom',
    artist: 'Rebecca Guay',
    printing: 'Morningtide, 2008',
    art: 'https://cards.scryfall.io/art_crop/front/8/1/8145fed6-6b51-420a-84cf-4ea5e0aa1883.jpg',
    alt: 'A watercolour of a sleeping face wreathed in dark flowers, a '
       + 'winged faerie rising above it like smoke.',
    why:
      'Art nouveau tenderness — a sleeping face, a faerie rising like smoke '
      + '— wrapped around one of the wickedest cards ever printed. I love '
      + 'the honesty of that gap: pretty is not the same as safe, in '
      + 'paintings or in deckbuilding.',
  },
  {
    name: 'Farewell',
    artist: 'Seb McKinnon',
    printing: 'Kamigawa: Neon Dynasty, 2022',
    art: 'https://cards.scryfall.io/art_crop/front/0/0/0050b693-7bad-4c0c-baca-0186d153ce2e.jpg',
    alt: 'A child seen from behind blows a dandelion whose seeds scatter '
       + 'into a great spiral of white paper cranes.',
    why:
      'The gentlest painting ever put on a board wipe: a child blows a '
      + 'dandelion and the seeds come apart into a thousand paper cranes. '
      + 'McKinnon keeps being handed Magic’s hardest job — making removal '
      + 'feel like grief instead of arithmetic — and keeps being equal '
      + 'to it.',
  },
]
