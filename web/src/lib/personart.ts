/**
 * Each costumed voice's painting, and now its room (punch list 2026-08-15
 * items 1 and 8). Scryfall art crops — hotlinked with the artist credited
 * wherever they render, never committed (rule 5, ADR 6), exactly as the
 * page mastheads do it. Looked up on Scryfall rather than recalled (rule 1
 * covers art too); artist and set are what Scryfall reports for the default
 * printing:
 *
 * - fortune-teller — *The Deck of Many Things*, Volkan Baǵa, Adventures in
 *   the Forgotten Realms: a spread of cards nobody should trust.
 * - therapist — *Alandra, Sky Dreamer*, Caroline Gariba, Murders at Karlov
 *   Manor Commander: somebody paid to sit with what you dream.
 * - scientist — *Rukarumel, Biologist*, Fariba Khamseh, Commander Masters:
 *   a field scientist delighted by her specimen.
 * - chef — *Gyome, Master Chef*, Steve Prescott: the house chef, and a nod
 *   to the deck he leads in this library.
 * - storyteller — *Birgi, God of Storytelling*, Eric Deschamps, Kaldheim.
 * - barkeep — *Edgewall Innkeeper*, Matt Stewart, Throne of Eldraine.
 *
 * This lives in `lib/` because two components need it — the persona tiles
 * (`components/tarot.tsx`) and the interview's rooms (`components/theme.tsx`)
 * — and tarot already imports theme, so a table in either would be a cycle.
 *
 * `accent` is the room's colour: picked from each painting's own palette so
 * the conversation's chrome and the backdrop agree. `plain` is deliberately
 * absent from the art table — Claude's tile draws its own mark — but has an
 * accent, the warm terracotta of that mark.
 */

export const PERSONA_ART: Record<string, { art: string; credit: string }> = {
  'fortune-teller': {
    art: 'https://cards.scryfall.io/art_crop/front/f/e/feddbdc6-0757-43cb-bb41-dc83c6cf42ea.jpg',
    credit: 'Volkan Baǵa',
  },
  therapist: {
    art: 'https://cards.scryfall.io/art_crop/front/5/4/54bf48d4-e350-4ca7-87da-ce04fefd4610.jpg',
    credit: 'Caroline Gariba',
  },
  scientist: {
    art: 'https://cards.scryfall.io/art_crop/front/0/b/0b2f7397-9d75-4667-8872-e58a39512583.jpg',
    credit: 'Fariba Khamseh',
  },
  chef: {
    art: 'https://cards.scryfall.io/art_crop/front/8/2/8279d421-dd86-49d1-93f7-65f6046c542d.jpg',
    credit: 'Steve Prescott',
  },
  storyteller: {
    art: 'https://cards.scryfall.io/art_crop/front/4/4/44657ab1-0a6a-4a5f-9688-86f239083821.jpg',
    credit: 'Eric Deschamps',
  },
  barkeep: {
    art: 'https://cards.scryfall.io/art_crop/front/7/c/7c5d0560-f9e6-4c70-8cce-cae61e4e74bc.jpg',
    credit: 'Matt Stewart',
  },
}

export const PERSONA_ACCENT: Record<string, string> = {
  plain: '#e8956d',
  'fortune-teller': '#8f79e8',
  therapist: '#7ab8d9',
  scientist: '#3aa08c',
  chef: '#c98a3a',
  storyteller: '#d97a5a',
  barkeep: '#c9a227',
}
