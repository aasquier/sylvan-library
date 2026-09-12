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
 * - witch — *Agatha of the Vile Cauldron*, Jason A. Engle, Wilds of
 *   Eldraine: a witch mid-brew, one hand over a cauldron going green, in a
 *   hut hung with jars and cages. Aaron asked for "a witch brewing a secret
 *   concoction in her pot" and named Agatha; the painting is that sentence.
 *   Her artifact — *Agatha's Soul Cauldron*, the same artist and set — is the
 *   more famous card, but it is a painting of a pot with nobody at it, and a
 *   persona tile is a face.
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

/**
 * The rooms this build knows something about, by key.
 *
 * A union rather than `string`, because every table below is keyed by hand and
 * a mistyped key in one of them is not an error of any kind today — it is a
 * painting that silently never renders, on a tile that looks merely plain.
 *
 * **The server is still the roster, and this is not a copy of it.** A voice
 * added server-side (ADR 21's whole payoff) arrives with a key that is not in
 * this union, and that has to keep working: it renders from the roster's own
 * label and blurb with no painting and the default accent, exactly as
 * `necromancer` does in `routes/NewDeck.test.tsx`. So every lookup takes a
 * plain `string` and answers with a fallback, and the one cast that spans the
 * two worlds lives inside these two functions rather than at each call site.
 */
export type PersonaKey =
  | 'plain'
  | 'fortune-teller'
  | 'therapist'
  | 'scientist'
  | 'chef'
  | 'storyteller'
  | 'barkeep'
  | 'witch'

export interface PersonaArt {
  art: string
  credit: string
}

const PERSONA_ART: Partial<Record<PersonaKey, PersonaArt>> = {
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
  witch: {
    art: 'https://cards.scryfall.io/art_crop/front/d/6/d6c48f07-63b7-4a60-8da6-ce77405abf1e.jpg',
    credit: 'Jason A. Engle',
  },
}

/** The painting a room wears, or nothing — `plain` has no painting by design. */
export function personaArt(persona: string): PersonaArt | undefined {
  return PERSONA_ART[persona as PersonaKey]
}

/** What a room whose accent nobody has picked wears: the app's own first
 *  series colour, which is the chrome every other surface uses. */
const DEFAULT_ACCENT = 'var(--series-1)'

const PERSONA_ACCENT: Partial<Record<PersonaKey, string>> = {
  plain: '#e8956d',
  'fortune-teller': '#8f79e8',
  therapist: '#7ab8d9',
  scientist: '#3aa08c',
  chef: '#c98a3a',
  storyteller: '#d97a5a',
  barkeep: '#c9a227',
  // Sampled off the brew itself rather than chosen: the greens in the lower
  // third of the painting average #90af72, which is that colour under a dim
  // hut. Lifted to the saturation and value the rest of this table lives at.
  witch: '#7eb846',
}

/** A room's colour, on its chrome and its backdrop. */
export function personaAccent(persona: string): string {
  return PERSONA_ACCENT[persona as PersonaKey] ?? DEFAULT_ACCENT
}
