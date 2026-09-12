/**
 * What each room is dressed in.
 *
 * The fortune-teller's table is not a different interview — it is the same
 * interview wearing parchment, and until now that costume was a boolean.
 * `const seance = persona === 'fortune-teller'` in `components/theme.tsx` fed
 * six inline ternaries: the question card, the question's own hand, the
 * reader's speech, the fun fact beside it, the answer box and its prompt. One
 * room is a boolean's worth of difference. **Two rooms are not**, and the
 * second one is a cauldron, so the costume becomes a record before it becomes
 * a second `||`.
 *
 * Three rules this table follows, and each of them is a thing the ternaries
 * got wrong or could not say:
 *
 * **A class, never a style.** Everything a costume changes is a class the
 * stylesheet owns, so the look lives in `index.css` beside the rest of the
 * room (`.seance-*` is the whole of the fortune-teller's). An inline style
 * would be a costume a `:hover` could never reach — commandment 17's exact
 * mechanism, one surface over.
 *
 * **An empty class means the plain chrome**, whose inline paint stays at the
 * call site. A room with no costume must not have to restate the default here
 * to get it, or `plain` becomes a costume too and the table stops having a
 * floor.
 *
 * **One room, one voice, while it is working.** The asking line used to key on
 * the persona and the proposing line on whether a spread had been dealt, which
 * is how the fortune-teller's own table came to say "Reading the cards… 14s"
 * in the column and "Reading around… 14s" in the sidebar at the same moment.
 * Both come off this record now, so a room that speaks cannot answer in two
 * voices.
 */

import type { PersonaKey } from './personart'

export interface Costume {
  /** The weather over the room's painting. A subset of `RoomMood` in
   *  `components/forest.tsx`. The witch's hut wanted the embers, which is the
   *  case the previous wording of this docstring left the door open for. */
  mood: 'mist' | 'wisps' | 'embers'
  /** The whole class list for the card the question sits on. The one field
   *  that is never empty: something has to carry the box. */
  scroll: string
  /** Extra class on the question itself — the hand it is written in. */
  question: string
  /** Extra class on the reader's own speech bubbles. */
  bubble: string
  /** Extra class on the fun fact beside the conversation. */
  note: string
  /** Extra class on the answer box. */
  quill: string
  /** What the empty answer box invites. */
  placeholder: string
  /** Whether the question is *written* rather than printed — `InkText`, the
   *  wet-ink reveal with a quill riding it. A property of the room rather than
   *  of the interview: the reader writes on parchment, the app prints. */
  ink: boolean
  /** While the next question is being written. */
  thinking: string
  /** When a turn ran and came back with nothing to ask.
   *
   *  It is a costume field rather than the server's sentence for the reason
   *  every other field here is one: "Nothing usable came back." is the app
   *  talking about itself, and a witch does not have a *came back*. The plain
   *  entry is that sentence unchanged, so an undressed room says what it has
   *  always said — and a turn that never ran at all (`asked: false`: the
   *  stance is off, the ceiling is reached) keeps the server's own words,
   *  because those are a real explanation rather than a shrug. */
  emptyReply: string
  /** While the reading itself is being worked out. The clock is appended by
   *  the caller, so this ends where the seconds begin. */
  reading: string
  /** The line over the ready banner, and the button under it. */
  ready: string
  readyAction: string
  /** The class list for the room's own primary action — the button that
   *  *does the thing this room is for*. Empty means the app's own accent
   *  button, whose paint stays at the call site like every other blank field
   *  here.
   *
   *  It exists because a room with its own palette gets its own tailoring
   *  rather than a hand-me-down (`.btn-felt`'s argument, and commandment 17's):
   *  a `.btn-accent-2` standing on a witch's hut is the page's furniture in
   *  somebody else's house. It is one field rather than two because the banner
   *  in the column and the button in the sidebar are one act — the same reason
   *  `reading` is one field. */
  action: string
}

/**
 * No costume: the interview as it has always looked.
 *
 * Exported because it is the floor every other room is measured against, and
 * because a room the server adds tomorrow gets exactly this.
 */
export const PLAIN: Costume = {
  mood: 'mist',
  scroll: 'rounded-xl px-5 py-4 card-surface',
  question: '',
  bubble: '',
  note: '',
  quill: '',
  placeholder: 'However much or little you like…',
  ink: false,
  thinking: 'Thinking…',
  emptyReply: 'Nothing usable came back.',
  reading: 'Reading around…',
  ready: 'That’s enough to go on.',
  readyAction: 'Get my colours',
  action: '',
}

const COSTUMES: Partial<Record<PersonaKey, Costume>> = {
  /* The séance (overhaul item 5, commandment 15): the question card is a
     scroll, the reader's words soak into it, the answer box takes a quill,
     and the crystal ball's violet light drifts across the floor instead of
     mist. Every class here is defined in `index.css` under "the séance". */
  'fortune-teller': {
    mood: 'wisps',
    scroll: 'seance-scroll',
    question: 'seance-question',
    bubble: 'seance-bubble',
    note: 'seance-note',
    quill: 'seance-quill',
    placeholder: 'Write your answer on the parchment…',
    ink: true,
    thinking: 'The quill hovers…',
    emptyReply: 'The cards went quiet. Ask the table again.',
    reading: 'Reading the cards…',
    ready: 'Three cards, three answers — the reading is ready.',
    readyAction: 'Read my cards',
    action: '',
  },

  /* The cauldron (commandment 3, and the room this file was refactored for —
     see the docstring above). The question sits on a slate propped against the
     pot; her words arrive in chalk rather than ink, because a witch writes on
     what is to hand and the quill belongs to the reader across the village.
     Every class here is defined in `index.css` under "Agatha's hut". */
  witch: {
    mood: 'embers',
    scroll: 'cauldron-slate',
    question: 'cauldron-question',
    // `-said`, not `-bubble`: the pot already has a `.cauldron-bubble` and it
    // is the thing that rises through the brew and breaks.
    bubble: 'cauldron-said',
    note: 'cauldron-note',
    quill: 'cauldron-scratch',
    placeholder: 'Go on. The pot only takes true things…',
    // Deliberately. `InkText` is the *reader's* wet-ink reveal on parchment
    // and it belongs to her: the witch does not write anything down, she
    // listens and puts things in a pot. Her questions print.
    ink: false,
    // No ellipsis on this one, and that is the stylesheet's business rather
    // than an oversight: `.cauldron-question.thinking-pulse::after` draws three
    // dots and brings them in one at a time, so the wait is something the room
    // *does* instead of the whole sentence dimming to 45% while somebody is
    // trying to read it. Under reduced motion all three stand still.
    thinking: 'She leans over the steam',
    emptyReply: 'The steam took that one — say it again, plainer.',
    reading: 'Bringing it up to a boil…',
    ready: 'Three things in the pot, and it has stopped arguing with itself.',
    readyAction: 'Pour it out',
    action: 'btn-copper-hot',
  },
}

/** What this room is wearing. A voice with no costume — which is seven of the
 *  eight, and every voice the server grows tomorrow — gets the plain room. */
export function costumeFor(persona: string): Costume {
  return COSTUMES[persona as PersonaKey] ?? PLAIN
}
