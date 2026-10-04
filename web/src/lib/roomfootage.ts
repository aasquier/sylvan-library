/**
 * The rooms that are footage rather than a painting.
 *
 * A persona's tile wears a Scryfall painting (`lib/personart.ts`); its room,
 * once you are in it, used to wear that painting's palette washed across the
 * viewport. These rooms wear a loop instead — Aaron's own renders, seamed and
 * encoded the séance room's way (`assets/seance/PROVENANCE.md` is the recipe
 * and `assets/<room>/PROVENANCE.md` is each clip's record). Still is the
 * reduced-motion floor and the ambience-off picture; tone is the room's own
 * sound, which plays only behind the table-sound switch.
 *
 * Keyed by hand like the paintings, and for the same reason the painting
 * table takes a `string` and answers with a fallback: a voice the server adds
 * tomorrow arrives with no footage and must render exactly as the plain room
 * does, with the painting it does have (`every room is the painting's room
 * until it has footage` is the whole contract).
 *
 * The witch's hut is NOT here: her plate is the cauldron's own layer stack
 * (`components/cauldron.tsx`), where the four mouth percentages live, and a
 * second import of the same clip would be a second place to keep them.
 */

import type { PersonaKey } from './personart'
import barkeepMp4 from '../assets/barkeep/barkeep-room-loop.mp4'
import barkeepStill from '../assets/barkeep/barkeep-room-still.webp'
import barkeepTone from '../assets/barkeep/barkeep-room-tone.m4a'
import barkeepWebm from '../assets/barkeep/barkeep-room-loop.webm'
import storytellerMp4 from '../assets/storyteller/storyteller-camp-loop.mp4'
import storytellerStill from '../assets/storyteller/storyteller-camp-still.webp'
import storytellerTone from '../assets/storyteller/storyteller-camp-tone.m4a'
import storytellerWebm from '../assets/storyteller/storyteller-camp-loop.webm'

export interface RoomFootage {
  webm: string
  mp4: string
  still: string
  tone: string
}

const ROOM_FOOTAGE: Partial<Record<PersonaKey, RoomFootage>> = {
  barkeep: {
    webm: barkeepWebm, mp4: barkeepMp4, still: barkeepStill, tone: barkeepTone,
  },
  storyteller: {
    webm: storytellerWebm, mp4: storytellerMp4, still: storytellerStill,
    tone: storytellerTone,
  },
}

/** The loop a room wears, or nothing — in which case it wears its painting. */
export function roomFootage(persona: string): RoomFootage | undefined {
  return ROOM_FOOTAGE[persona as PersonaKey]
}
