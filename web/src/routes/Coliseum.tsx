/**
 * The Coliseum: the six houses a Forge match is watched in.
 *
 * A Tier 3 match takes minutes, and until now those minutes were a progress
 * bar. This is the room those minutes belong to — and it is a room rather
 * than a panel on the Simulator, because it is worth walking through when no
 * match is running at all.
 *
 * Three decisions worth keeping.
 *
 * **The painting is shown whole.** `art_crop` is 1.37:1 and a full-bleed band
 * keeps less than half of it — the lesson `PageMasthead` exists to enforce and
 * which this project has learned four times. So the arena is a framed stage at
 * its own ratio, and the weather happens *over* it.
 *
 * **The rotation is paced for reading, not for glancing.** Thirty seconds a
 * slide, because these are two or three sentences of real prose and a carousel
 * that moves at banner speed is a carousel nobody finishes a sentence in. A
 * slide replaces its predecessor rather than cross-fading over it, for the
 * same reason.
 *
 * **Nothing here is authored at runtime.** The arenas, the champions' roles
 * and every fact are checked-in prose (`reference/data/coliseum.json`); the
 * card facts and the art come from the pool, and a name the pool cannot
 * resolve is dropped and counted rather than guessed at. With no pool the
 * prose still answers whole — only the paintings go missing, which the stage
 * renders as the arena's palette alone.
 *
 * **And the gates open from here** (Aaron's call, 2026-08-25). Tier 3 used to
 * be a fifth option on the Simulator, which made one screen answer two unlike
 * questions — "what do ten thousand shuffles say about my mana" is a number
 * you read, and "who wins" is a match you *watch*, for minutes. The Simulator
 * kept the arithmetic; the match moved whole to the room that was built to
 * watch one in. Two more decisions came with it.
 *
 * **This room narrates and the measuring surfaces do not.** `narrate: true`
 * crosses on the ask, Forge drops its `-q`, and about a hundred typed beats a
 * game come back on the job's stream. The flag is free in time and expensive
 * in volume (`events.go` measured both), which is exactly the trade a room
 * built for watching should take and a nightly sweep should refuse.
 *
 * **The beats are paced here, not by the server.** They arrive whole, one
 * game at a time, at the moment that game ends — Forge plays a game in
 * seconds and nobody can watch seconds of Commander, so the room holds a queue
 * and drains it at a rate a person can follow.
 *
 * **And a series is told in order** (Aaron, 2026-08-26). A match raises its
 * second fight while the first is still being told, and the arriving one used
 * to take the room mid-sentence. It does not now: a bout is told to its end,
 * the room takes a breath, and the next begins — with every bout the match has
 * raised reachable from the field, so nobody is held behind a slow retelling
 * of game one while game four is being fought. The running order lives in
 * `lib/reel.ts`; what lives *here* is the catching, because the job's
 * `partial` carries only the newest bout and forgets it the moment the next
 * one lands.
 *
 * **The room can be walked back into.** The match is fought on another machine
 * and the arena holds it; all a reload ever lost was this room's handle on it,
 * so the handle rides in the link beside the seats and the room asks for it
 * back on the way in. The shuffle rides there too — which is what makes the
 * link a way to fight the same bout again, and why nobody is asked to have an
 * opinion about a number any more.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import {
  api, errorMessage, followJob,
  type Coliseum, type ColiseumArena, type ColiseumFact,
  type ColiseumStandings, type DeckTile, type ForgeBeats, type ForgeDeckRow,
  type ForgeResult, type Job, type ValidationReport,
} from '../lib/api'
import { ColiseumRecord } from '../components/coliseumrecord'
import { CardHover, Caveat, ErrorNote, NumberField, Select }
  from '../components/ui'
import { DeckCaution } from '../components/closedform'
import { MatchBoard } from '../components/board'
import { MatchVerdict } from '../components/verdict'
import { MatchTheater } from '../components/theater'
import { type Arriving, type Speed, type StagedBeat, useReel }
  from '../lib/reel'
import {
  beatLine, playerTurns, shortName, theaterBeats, theaterRows,
  turnMarks as turnMarksOf, turnsTaken,
} from '../lib/theater'
import { CrossedSwordsGlyph } from '../components/glyphs'
import { reducedMotion, useCardMotion } from '../lib/motion'
import { HelpTip, Term } from '../components/term'
import secutorArt from '../assets/coliseum/secutor.webp'

/** Every control and figure here, keyed to the served glossary — the same
 *  contract the Simulator's own controls use. */
const help = (key: string) => <HelpTip name={key} />

/** The highest shuffle the gate will deal or honour — the server's own
 *  ceiling for the field this replaced. */
const SHUFFLE_CEILING = 999_999

/**
 * A fresh shuffle, drawn for every bout sent in.
 *
 * **The shuffle is not the watcher's problem** (Aaron, 2026-08-26: "I am not a
 * big fan of our shuffle field we let players put in ... I think we should hide
 * the seed from them altogether"). It used to be a numbered field standing in
 * the gate beside the two decks, defaulted to a small round number, and it
 * asked somebody who had come to watch two decks fight to first hold an opinion
 * about an integer. Worse, left alone it meant every match anybody ever ran in
 * this room was dealt from the same shuffle.
 *
 * **It has not gone away.** Determinism is contract here: a match is still
 * dealt from a written-down shuffle, still sent with the ask, and still
 * recorded against the result. It is drawn rather than typed, and it rides in
 * the room's own link — so a bout can be fought again, exactly, by anybody the
 * link reaches. What never happens is a bare number reaching the page
 * (commandment 10).
 */
const drawShuffle = () => Math.floor(Math.random() * SHUFFLE_CEILING) + 1

/**
 * What a deck is called in the gate's two pickers.
 *
 * **Lean, because the control has to hold it** (Aaron, 2026-08-26: "we are
 * cramming a lot of text into the dropdown options, those should be leaner to
 * pick and look at so the whole phrase fits on the button or dropdown
 * easily"). Three things used to be concatenated into one option — the deck's
 * name, its owner, and its pilot — which on a shelf of real decks reads
 * "Arahbo, Roar of the World — Cats (Mark's wife)" inside a control that is
 * eleven rem wide on a phone. Every one of them ellipsised, and a picker that
 * ends in "…" is a picker you cannot pick with.
 *
 * So: the commander's name and what the deck *does*, which together are how
 * anybody refers to a deck out loud. The epithet goes — "Roar of the World" is
 * flavour, and `shortName` exists in `lib/theater.ts` for this exact reason.
 * The theme stays, because it is the half that tells two decks apart.
 *
 * **The general is passed, and it is what decides whether anything is cut at
 * all.** This control is where Aaron found the bug: his Atla Palani deck is
 * called "Life, Uh, Finds a Way" and the dropdown offered it as *"Life"*,
 * because the epithet was being found by punctuation rather than by lookup.
 * A deck named for its commander still reads "Arahbo — Cats"; a deck named
 * for a joke keeps the joke.
 *
 * The pilot is dropped outright: it is a person's name, it was never how
 * anybody chose a deck, and it was most of the overflow. The owner is handled
 * by the caller, and only where it earns its space.
 */
function leanName(name: string, commander?: readonly string[]): string {
  const short = shortName(name, commander)
  const cut = name.indexOf('—')
  if (cut < 0) return short
  const theme = name.slice(cut + 1).trim()
  return theme ? `${short} — ${theme}` : short
}

/** One deck, as the gate offers it.
 *
 *  The owner rides along **only where it earns its space**: your own library
 *  is the common case and naming yourself in every option is a word repeated
 *  down the whole list. Somebody else's deck is where the ambiguity actually
 *  lives — two people may both keep a Gyome — so that is where it is spent,
 *  and with a separator that cannot be mistaken for the deck's own. */
function seatOption(d: DeckTile) {
  return {
    value: `${d.owner}/${d.slug}`,
    label: d.writable
      ? leanName(d.name, d.commander)
      : `${leanName(d.name, d.commander)} · ${d.owner}`,
  }
}

/** How long a slide holds. Long, deliberately: see the note above. */
const SLIDE_MS = 24_000

/** How long the room stays in one arena before walking to the next.
 *
 *  Ninety seconds is roughly four slides, and six arenas is then a nine-minute
 *  circuit — longer than most matches, so a match rarely sees the same arena
 *  twice and never sits in one for its whole length. Both timers restart on a
 *  click: a person who has chosen where to stand should not be walked off. */
const ARENA_MS = 90_000

/** The pennants, laid out once rather than per render.
 *
 *  Every value here is a *different* number on purpose. Nine banners sharing
 *  one period and one phase read as a screensaver; nine with their own read as
 *  wind. The heraldry is five hues so no two neighbours match. */
/** The room's own painting, and it does not rotate.
 *
 *  The six arenas below take turns; this one does not, and the distinction is
 *  load-bearing rather than cosmetic. The banners and the crowd are placed
 *  against *this* painting's geometry — its rim, its gates — so pointing the
 *  hero at whichever arena happens to be selected hangs pennants in the middle
 *  of Valor's Reach's drawing room. An effect tuned to one painting belongs to
 *  that painting.
 *
 *  Grand Coliseum, Onslaught (ONS) #319, art by Carl Critchlow — the arena
 *  this room is named for, from the set that also printed its champion. */
const HERO = {
  url: 'https://cards.scryfall.io/art_crop/front/c/2/c2dc8061-a855-4a81-9eb7-350b355a9b3f.jpg?1783945028',
  printing: 'Onslaught',
  artist: 'Carl Critchlow',
  /** The painting's oracle id, which is how the motion shelf is keyed
   *  (ADR 32). Written down rather than looked up: this room shows one fixed
   *  painting on purpose — see the note above — so a pool round trip would be
   *  a request to learn a constant. */
  oracleId: '1cea9b82-d2e9-4758-8ec8-729fcf4bb7d7',
  alt: 'A vast oval arena of pale stone standing alone on a bare plain, '
     + 'ringed with gatehouses and crowned by two tall statues, under a wide '
     + 'and hazy sky.',
}

/** The hero: the whole painting at page width, with what the painting implies
 *  but cannot do — banners flying from the wall, and a crowd filing in.
 *
 *  **Whole, not cropped.** Spanning the page and cropping the page are
 *  different asks; `art_crop` is 1.37:1 and this frame keeps that ratio, so
 *  none of Critchlow's arena is thrown away to make a letterbox. */
/** The only derivative this banner will accept.
 *
 *  Not the usual ladder. `depth-drift` and `slow-pan` are parallaxes *of the
 *  plate* and would fight a frame that is already moving; this room wants the
 *  one loop that was made for it or the painting, and nothing in between. */
const HERO_EFFECTS = ['daynight'] as const

function Hero() {
  // One status pass. `ready: false`, an error, or an instance that never had
  // the loop pushed to it all land in the same place: the still, which is the
  // page exactly as it was before this existed.
  const motion = useCardMotion(HERO.oracleId, HERO_EFFECTS, HERO.url)
  // **A fifteen-second day-to-night pass is motion by anybody's definition.**
  // Somebody who has asked their machine to stop moving things gets the
  // painting, not a poster frame of the loop — the painting is the better
  // still and it is the one a person made.
  const loop = reducedMotion() ? undefined
    : motion?.ready ? motion.urls?.mp4 : undefined
  // **Slower than it was rendered.** Fifteen seconds is a whole day and night,
  // which is a lot of sky to move through while somebody is reading the
  // paragraph underneath it. Six tenths makes it twenty-five, which is closer
  // to the eighty-second breath the still has always had — this is a room's
  // backdrop, not a thing to watch. `playbackRate` survives no attribute, so
  // it is set on the element and set again whenever the source changes.
  const film = useRef<HTMLVideoElement>(null)
  useEffect(() => {
    if (film.current) film.current.playbackRate = 0.6
  }, [loop])
  return (
    <>
    <div className={`coliseum-hero${loop ? ' is-moving' : ''}`}>
      {loop ? (
        // `role="img"` with the still's own description: what a screen reader
        // needs here is what the frame shows, and that has not changed.
        <video className="coliseum-hero-art" src={loop} ref={film}
               poster={motion?.urls?.poster}
               autoPlay loop muted playsInline
               role="img" aria-label={HERO.alt} />
      ) : (
        <img className="coliseum-hero-art" src={HERO.url} alt={HERO.alt} />
      )}
      <div className="coliseum-hero-sky" aria-hidden="true" />
    </div>
    {/* **The title is read, not seen.** The banner says "coliseum" better than
        the word does (Aaron: "we can drop the Coliseum title since the video
        speaks for itself"), but a page still needs a heading — a screen reader
        has no picture to be spoken for by, and an outline with no `h1` is a
        page that starts nowhere. */}
    <h1 className="sr-only">The Coliseum</h1>
    {/* **The credit sits under the frame now, not on it** (Aaron: "lets move
        the credit underneath the video"). It was a footnote *on* the painting
        over a scrim that existed to make white ink legible over pale stone —
        and the scrim was a wash across the foot of the picture, paid for by
        the picture. Underneath, the credit is simply readable and the frame is
        whole.

        It changes with what is on screen, because the two are different
        claims. Over the painting it is a credit. Over the loop it is an
        *acknowledgement*: what is playing was generated from that painting and
        is not the artist's own work, and "art by" alone over it would put his
        name on something he did not make. */}
    <p className="coliseum-footnote">
      {loop ? <>Motion inspired by </> : null}
      <em>Grand Coliseum</em>, {HERO.printing} — art by {HERO.artist}
    </p>
    </>
  )
}

/** What each kind of slide is called on screen. The label is the promise the
 *  slide keeps — a reader who wants the Magic ones can find them. */
const KIND_LABEL: Record<ColiseumFact['kind'], string> = {
  roman: 'Rome',
  gladiator: 'The gladiators',
  coliseum: 'The Colosseum',
  magic: 'Magic',
  paired: 'Rome, and its echo',
}

function Slide({ fact }: { fact: ColiseumFact }) {
  return (
    <div className="arena-slide">
      <p className="text-[0.68rem] font-semibold uppercase tracking-[0.14em]
                    text-[var(--text-muted)]">
        {KIND_LABEL[fact.kind]}
      </p>
      {fact.rome && (
        <p className="mt-2 text-[0.95rem] leading-relaxed text-[var(--ink)]">
          {fact.rome}
        </p>
      )}
      {fact.magic && (
        <p className={`text-[0.95rem] leading-relaxed ${
          fact.rome
            ? 'mt-3 border-l-2 pl-3 text-[var(--text-muted)]'
            : 'mt-2 text-[var(--ink)]'
        }`}
           style={fact.rome ? { borderColor: 'var(--arena-glow)' } : undefined}>
          {fact.magic}
        </p>
      )}
      {fact.card && (
        <p className="mt-2 text-[0.75rem] italic text-[var(--text-muted)]">
          {fact.card}
        </p>
      )}
    </div>
  )
}

function Stage({ arena }: { arena: ColiseumArena }) {
  return (
    <div
      className="arena-stage"
      style={{
        // The palette belongs to the painting rather than to the theme, so it
        // arrives inline from the served prose and the stylesheet reads it.
        '--arena-ink': arena.palette.ink,
        '--arena-glow': arena.palette.glow,
      } as React.CSSProperties}
    >
      {/* The named printing, never the pool's default — see `ArenaArt`. The
          pool still answers for the card's *facts*; it just does not get to
          choose the painting. */}
      {arena.art.url
        ? <img className="arena-art" src={arena.art.url}
               alt={`${arena.name} — ${arena.art.printing} art by ${arena.art.artist}`}
               loading="lazy" />
        : <div className="arena-art-missing" role="img"
               aria-label={`${arena.name}, without its painting`} />}
      {/* Decoration, and never load-bearing: no pointer events, out of the
          accessibility tree, and removed outright under reduced motion. */}
      <div className="arena-motion" data-motion={arena.motion} aria-hidden="true" />
      <div className="arena-veil" aria-hidden="true" />
    </div>
  )
}

/**
 * Take the link to this bout.
 *
 * The room's answer to "send it to somebody", and it earns its place on the
 * device this room is most often watched on: pulling a link out of the bar by
 * hand is a moment's work with a mouse and a fiddle with a thumb.
 *
 * **Rendered only where the hand can actually be taken.** A browser that will
 * not hand over the clipboard would leave a control that answers a press by
 * doing nothing at all, and a dead button is worse than no button. It wears
 * `.btn-quiet` — the house's patient voice — so it replies to a hover, a focus
 * and a press like everything else here can be reached for (commandment 17).
 */
function CopyTheLink() {
  const [taken, setTaken] = useState(false)
  // The confirmation is temporary on purpose: a button stuck reading "Copied"
  // is a button that has stopped saying what it does.
  useEffect(() => {
    if (!taken) return
    const id = window.setTimeout(() => setTaken(false), 2400)
    return () => window.clearTimeout(id)
  }, [taken])
  const clip: Clipboard | undefined = navigator.clipboard
  if (!clip) return null
  return (
    <button type="button" className="btn btn-quiet btn-sm align-baseline"
            onClick={() => {
              clip.writeText(window.location.href)
                .then(() => setTaken(true))
                .catch(() => undefined)
            }}>
      {taken ? 'Copied' : 'Copy the link'}
    </button>
  )
}

/**
 * The card the gate *is*, and "is" is doing all the work in that sentence.
 *
 * Aaron, 2026-08-27: *"what if our button to 'Send them in' was a button made
 * from the card 'Arena' with a little footnote fun fact, that is one of the
 * prime cards to represent a duel in Magic"*. He is right about the card and
 * he is right for a reason worth writing down: the two selects beside this
 * button are labelled Champion and Challenger, and *Arena* is the one land in
 * Magic whose whole text is that arrangement — you name a creature of yours,
 * the other player names one of theirs, and the two of them fight.
 *
 * **`normal/` — the whole card — and that is compliance rather than taste.**
 * The first build of this control hung the `art_crop/` behind a pill and put a
 * label on top, which Aaron read twice and ruled on twice: *"I can barely see
 * the art. I meant literally the full card is the button"*, and then the half
 * that settles it — *"that was breaking the 'no cropping' scryfall rule"*.
 * Scryfall's imagery guidelines forbid cropping, distorting, desaturating and
 * watermarking a card image; ADR 32 wrote that boundary into this repo and
 * commandment 9 makes it a wall rather than a preference. So the card arrives
 * whole, is laid out at its own aspect ratio, and is never reframed: no
 * `object-fit`, no scaling inside a clipped box, no filter on it or on
 * anything above it in the tree. `components/wheel.tsx`'s folded state is the
 * pattern and was right all along.
 *
 * **The printing is pinned here, exactly as `HERO` above pins the Grand
 * Coliseum, and for the same reason.** Asked for a bare name the pool answers
 * with the card's *newest* printing, and Arena's newest is a 2024 Mystery
 * Booster repaint. The card this control wants is the first one: Rob
 * Alexander's, from the promo given away with a Magic novel in 1994, reprinted
 * timeshifted in Time Spiral. So it is chosen, credited and hotlinked rather
 * than resolved — a round trip to learn a constant is a round trip that can
 * also come back with the wrong constant.
 *
 * **And it survives not loading**, which is the failure the pool-fed art on
 * this page handles with `arena-art-missing`. A hotlink has no pool to fail,
 * but somebody else's host can still not answer — so the card is state, and
 * when the browser says it did not arrive the control falls back to the
 * blood-and-brass plate it has always had. A control that starts fights must
 * not depend on somebody else's host.
 */
const ARENA_CARD = {
  /** Arena, Time Spiral Timeshifted (TSB) #117, art by Rob Alexander. Looked
   *  up rather than recalled, which is the rule and which is also how the
   *  handoff's claim that this card comes from Portal got caught: it does
   *  not. Its printings are the 1994 HarperPrism book promo, Time Spiral
   *  Timeshifted, an online promo and Mystery Booster 2 — no Portal. */
  url: 'https://cards.scryfall.io/normal/front/e/5/e5b7afa9-e07a-4a84-a41f-7a8bc5c1d274.jpg?1783943260',
  /** The face's own pixels, so the browser reserves the card's shape before
   *  a byte of it arrives and the gatehouse does not jump when it does. */
  w: 488,
  h: 680,
  /** **Timeshifted, and the credit has to say so.** The set this face was
   *  printed in is *Time Spiral Timeshifted* (TSB), which is a set of its own
   *  and not Time Spiral (TSP) — a reader who went looking for this card in
   *  Time Spiral would not find it. Re-checked against Scryfall rather than
   *  recalled, 2026-08-28, along with everything the footnote below claims. */
  printing: 'Time Spiral Timeshifted',
  artist: 'Rob Alexander',
}

/**
 * The gate: a whole Magic card that is also a control, and has to speak.
 *
 * The wheel's folded card had the easy half of this — it stands alone at the
 * foot of a list, says nothing, and one click opens it. This one stands in a
 * row of selects, and it has four things to say: three labels and a state
 * where it cannot be pressed at all.
 *
 * **Everything it says is drawn on a layer of its own.** That is the rule the
 * `normal/` note above lands on, restated as a mechanism: the picture is a
 * picture, and every word, wash, glow and bar this control needs sits over it
 * rather than through it. So —
 *
 * - **The word rides a plate across the card's own text box** (`.arena-gate`
 *   in index.css, positioned in percentages of the card so it tracks the
 *   printed box at any width). It is where a Magic card already puts its
 *   words, and it leaves the painting, the title, the type line and the
 *   printed "Illus. Rob Alexander" all visible above and below it. Anywhere
 *   else the label either covers the painting — the miss this replaces — or
 *   floats off the card and stops being part of it.
 * - **Cannot-be-pressed is a portcullis, not a filter.** Greying the card out
 *   is the exact trap ADR 32 names, so the shut gate is a cool wash and a
 *   grate *drawn over* the card, and choosing the second seat lifts them
 *   away. It is the gatehouse's own metaphor, one panel out.
 * - **Running tilts the card a few degrees**, because *Arena* costs `{T}` and
 *   a tapped land is the one thing every Magic player reads without being
 *   told. A whole ninety degrees would be a card lying on its side in a form.
 *
 * The three labels stay exactly as they were. Pressed is a state this control
 * has to be able to show: the job comes back from a POST with a JVM starting
 * behind it, so without its own word the slowest action in the app would be
 * the one whose button sat unchanged for seconds after the click, and the
 * honest reading of that is that the click missed.
 *
 * Commandment 17 governs the rest and nothing here is an inline style: `.btn`
 * and `.btn-arena` still carry the focus ring, the glint across the face and
 * the two swords drawing apart on hover and closing on the press, and
 * `.arena-gate` adds the card's own lift, rotation and warmed shadow — which
 * is `.wheel-folded:hover` doing what it has always done to a card, and is
 * motion on the object rather than a change to the picture.
 */
function SendThemIn({ running, lighting, disabled, onPress }: {
  running: boolean
  lighting: boolean
  disabled: boolean
  onPress: () => void
}) {
  // The card is decoration and it is allowed to be absent. `false` is set by
  // the browser's own `error`, so this is the picture failing rather than a
  // guess about whether it will.
  const [painted, setPainted] = useState(true)
  // Which of the four the gate is in, for the stylesheet to answer. Ordered
  // by what is *happening* rather than by the props' names: a running match
  // also reports `disabled`, and a gate that drew the bars down over a bout
  // in progress would be saying the opposite of the truth.
  const gate = lighting ? 'lighting'
    : running ? 'running'
      : disabled ? 'shut' : 'open'
  // Whole literals on both arms rather than an interpolation, and that is not
  // a style preference: the stylesheet's utilities are generated by scanning
  // this file's text, and `sm:w-auto${...}` reads as one candidate that does
  // not exist — the rule vanished from the built sheet and the gate went
  // full-width on a desktop. A word that is never next to an expression
  // cannot be swallowed by one.
  const face = painted ? 'btn btn-arena arena-gate' : 'btn btn-arena w-full sm:w-auto'
  return (
    <button type="button" onClick={onPress} disabled={disabled} data-gate={gate}
            className={face}>
      {painted && (
        // `alt=""`: the card is this control's material, and the control's
        // name is the word on the plate. A screen reader that heard the card
        // named here would hear it twice — `ArenaFootnote` below says what the
        // card is, in prose, to everybody.
        <img className="arena-gate-card" src={ARENA_CARD.url} alt=""
             width={ARENA_CARD.w} height={ARENA_CARD.h}
             draggable={false} onError={() => setPainted(false)} />
      )}
      <span className={painted ? 'arena-gate-plate' : 'btn-arena-face'}>
        <CrossedSwordsGlyph />
        <span className="arena-gate-word">
          {lighting ? 'Lighting the forge…'
            : running ? 'The match is on…' : 'Send them in'}
        </span>
      </span>
    </button>
  )
}

/**
 * The little fun fact Aaron asked for, in the room's voice.
 *
 * Every clause in it is a fact off the card rather than a remembered one: it
 * is a land, the ability costs three and a tap, the creature on the other side
 * is *their* choice, and the printing and painter are the ones credited at the
 * end. The one number that would have been machinery — the mana cost written
 * as a symbol string — is said in words instead, because this line is for
 * somebody who has never read a card (commandment 2) and because a room
 * explaining a duel should sound like a room.
 *
 * **Re-read against the whole card, and one word changed.** It used to open
 * "cut from", which was honest about a crop and is now the wrong verb twice
 * over — the card is not cut, and not cutting it is the point. Nothing else
 * went: the picture shows the card's own name, type line and painter, but at
 * a hundred and fifty pixels the printed credit is five pixels tall, and ADR
 * 32 asks for attribution somebody can actually read. The novel promo and the
 * bargain the two seats make are nowhere on the card at all.
 */
function ArenaFootnote() {
  return (
    <p className="arena-note">
      That gate is <em>Arena</em>, whole — a land whose entire job is this
      room. Tap it and pay three, and one creature of yours duels one of
      theirs; which of theirs is <em>their</em> call, the same bargain the two
      seats beside it make. It was a promo tucked into a Magic novel in 1994,
      and thirty years on it is still the closest thing the game has to a
      coliseum. {ARENA_CARD.printing} — art by {ARENA_CARD.artist}.
    </p>
  )
}

/**
 * A stretch of time in the room's words rather than in bare seconds.
 *
 * Everything here used to render as a raw count — `seconds: 312`, "called off
 * at 300s" — which is a unit a machine chose. Three hundred seconds is five
 * minutes and nobody has ever thought about it in any other way; a newcomer
 * least of all (commandment 2). Under a minute it stays in seconds, because
 * "0m 47s" is a worse sentence than "47s" and a bout that short is genuinely a
 * matter of seconds.
 */
function spell(seconds: number): string {
  const whole = Math.max(0, Math.round(seconds))
  if (whole < 60) return `${whole}s`
  const minutes = Math.floor(whole / 60)
  const rest = whole % 60
  return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`
}

/**
 * The tale of the tape: what the house printed once the last game landed.
 *
 * **A score, not four measurements** (Aaron, 2026-08-27: "make the numbers
 * prettier that it outputs too, it is pretty basic"). What stood here was a
 * grid of identical `StatTile`s — this deck's wins, that deck's wins, draws,
 * clock-outs, game length — five boxes of equal weight, so the only fact
 * anybody came for had exactly the same size and colour as the median engine
 * time. Two decks fought a dozen times; that has a winner and it has a
 * margin, and a tile grid states neither.
 *
 * So: the two totals facing each other across the room's own crossed swords,
 * and under them a band that *is* the margin — one deck's bouts filling from
 * the left, the other's from the right, and what neither of them took held in
 * the middle.
 *
 * **The middle is the part that had to survive.** A prettier score is exactly
 * where a caveat goes quietly missing, and there are two here that must not:
 * a drawn bout is two decks playing each other to a standstill, and a bout
 * that hit the clock is the *measurement* giving up with the game still
 * going. Folding them together would be Forge's own mistake — it counts a
 * clock-out as a draw and this app has refused to since the beginning. They
 * are separate segments, in different materials, named separately underneath,
 * and they are stated even when they are zero: "no draws" is an answer, an
 * absent line is a reader wondering whether anybody counted.
 *
 * The band's widths are set inline and grown with a `scaleX` keyframe, which
 * is `components/coliseumrecord.tsx`'s rule and its reasoning — the
 * information is true at the first paint and the motion is decoration over
 * it. Nothing counts up, for the same reason: a numeral arriving through
 * animation frames is a numeral that reads zero in a tab nobody is watching.
 */
function TaleOfTheTape({ result, homeSlug, commanderOf }: {
  result: ForgeResult
  homeSlug: string
  /** The general a slug is named for, out of the room's shelf.
   *
   *  Passed in rather than read off the result, because a `ForgeDeckRow`
   *  carries a slug and a name and no commander at all — and `shortName` needs
   *  the general to know whether a deck's title is its commander's or its own
   *  (a deck called "Life, Uh, Finds a Way" is not a deck called "Life").
   *  Undefined for a deck the shelf has not handed us, which is the safe way
   *  round: the whole title, rather than somebody's first word. */
  commanderOf: (slug: string) => readonly string[] | undefined
}) {
  // Seated the way the room seated them, so the total on the left belongs to
  // the deck that has been on the left since the gate opened. Falling back to
  // the wire's own order rather than refusing: a mirror match, or a result
  // whose slugs the shelf cannot match, still has a score worth printing.
  const home = result.decks.find((d) => d.slug === homeSlug) ?? result.decks[0]
  const away = result.decks.find((d) => d !== home) ?? null
  if (!home) return null

  const winsHome = home.wins
  const winsAway = away?.wins ?? 0

  // The denominator is bouts actually played, never bouts asked for: a match
  // that fell over after eight of twelve is eight bouts of evidence, and
  // dividing by twelve would draw four bouts of silence as though somebody
  // had lost them.
  //
  // It is also never smaller than the four counts it is dividing — those
  // should add to exactly `played` and the server builds them so, but a band
  // whose segments summed past its own width would silently squeeze every
  // segment to fit and draw a margin nobody measured. Widened, the surplus
  // shows up as the picture reaching the end early, which is at least a
  // visible symptom rather than a quiet lie about the score.
  const counted = winsHome + winsAway + result.draws + result.timed_out
  const played = Math.max(result.played, counted, 1)
  const share = (n: number) => `${Math.max(0, (n / played) * 100)}%`
  const said = (d: ForgeDeckRow) => shortName(d.name, commanderOf(d.slug))
  const band = away
    ? `${winsHome} of ${result.played} bouts to ${said(home)}`
      + `, ${winsAway} to ${said(away)}`
      + `, ${result.draws} drawn and ${result.timed_out} stopped by the clock`
    : `${winsHome} of ${result.played} bouts to ${said(home)}`

  return (
    <div className="tape">
      <p className="tape-head">The tale of the tape</p>

      <div className="tape-score">
        <div className="tape-seat">
          <span className="tape-seat-name" title={home.name}>
            {leanName(home.name, commanderOf(home.slug))}
          </span>
          <span className="tape-seat-wins tabular">{winsHome}</span>
          <span className="tape-seat-of">
            {winsHome === 1 ? 'bout won' : 'bouts won'}
          </span>
        </div>
        <div className="tape-cross" aria-hidden="true">
          <CrossedSwordsGlyph size={30} />
        </div>
        <div className="tape-seat is-away">
          <span className="tape-seat-name" title={away?.name}>
            {away ? leanName(away.name, commanderOf(away.slug)) : 'No challenger'}
          </span>
          <span className="tape-seat-wins tabular">{winsAway}</span>
          <span className="tape-seat-of">
            {winsAway === 1 ? 'bout won' : 'bouts won'}
          </span>
        </div>
      </div>

      <div className="tape-band" role="img" aria-label={band}>
        <span className="tape-even" aria-hidden="true" />
        <span className="tape-span is-home" data-kind="won" aria-hidden="true"
              style={{ width: share(winsHome) }} />
        {result.draws > 0 && (
          <span className="tape-span is-middle" data-kind="draw"
                aria-hidden="true" style={{ width: share(result.draws) }} />
        )}
        {result.timed_out > 0 && (
          <span className="tape-span is-middle" data-kind="clock"
                aria-hidden="true" style={{ width: share(result.timed_out) }} />
        )}
        <span className="tape-gap" aria-hidden="true" />
        <span className="tape-span is-away" data-kind="won" aria-hidden="true"
              style={{ width: share(winsAway) }} />
      </div>

      <ul className="tape-notes">
        <li className="tape-note">
          <span className="tape-note-mark" data-kind="won" aria-hidden="true" />
          Bouts won{help('stat.forge_wins')}
        </li>
        <li className={`tape-note${result.draws === 0 ? ' is-nil' : ''}`}>
          <span className="tape-note-mark" data-kind="draw" aria-hidden="true" />
          {result.draws === 0 ? 'No draws'
            : `${result.draws} drawn — neither deck took it`}
        </li>
        <li className={`tape-note${result.timed_out === 0 ? ' is-nil' : ''}`}>
          <span className="tape-note-mark" data-kind="clock" aria-hidden="true" />
          {result.timed_out === 0
            ? 'None hit the clock'
            : `${result.timed_out} hit the clock — called off at `
              + `${spell(result.clock)}`}
          {help('stat.forge_timed_out')}
        </li>
        <li className="tape-note">
          <span className="tape-note-mark" data-kind="length" aria-hidden="true" />
          {result.median_seconds == null
            ? 'No bout was timed'
            : `Middling bout ${spell(result.median_seconds)}`
              + (result.max_seconds != null
                ? `, longest ${spell(result.max_seconds)}` : '')}
          {help('stat.forge_length')}
        </li>
      </ul>

      <p className="tape-head" style={{ marginTop: '1.1rem' }}>Every bout</p>
      <ol className="bouts">
        {result.rows.map((r) => {
          const kind = r.timed_out ? 'clock' : r.draw ? 'draw' : 'won'
          const took = result.decks.find((d) => d.slug === r.winner)
          // A player's turns, not Forge's per-player-turn count — the trap
          // `lib/theater.ts` records this project falling into once already.
          const turns = turnsTaken(r.turns)
          return (
            <li key={r.game} className="bout" data-outcome={kind}>
              <span className="bout-no tabular">{r.game}</span>
              <span className="bout-took">
                <span className="bout-mark" aria-hidden="true" />
                {took ? said(took)
                  : kind === 'clock' ? 'Stopped by the clock'
                    : 'Nobody — the bout ended level'}
              </span>
              <span className="bout-len tabular">
                {turns != null ? `${turns} turns · ` : ''}{spell(r.seconds)}
              </span>
            </li>
          )
        })}
      </ol>

      <p className="tape-foot">
        {result.played} {result.played === 1 ? 'bout' : 'bouts'} fought in
        {' '}{spell(result.wall_seconds)} — {spell(result.startup_seconds)} of
        that was lighting the forge.
      </p>
    </div>
  )
}

export default function ColiseumRoom() {
  const [params, setParams] = useSearchParams()
  const [data, setData] = useState<Coliseum | null>(null)
  const [failed, setFailed] = useState(false)
  const [chosen, setChosen] = useState(0)
  const [slide, setSlide] = useState(0)

  // **Two places in one room**, and the strip below picks between them: the
  // sand, where a match is fought, and the record, where every match already
  // fought is weighed. Component state rather than another query parameter —
  // `?a=`, `?b=`, `?m=` and `?s=` all name a *fight*, and a link into this
  // room should seat the fighters it names rather than open a ledger. The
  // record is one click from anywhere and needs no address of its own.
  //
  // Nothing about a running match is torn down by switching: the job poll and
  // the reel are both hooks on this component, so a match keeps going while
  // the record is open and the field picks its replay back up where it was.
  const [view, setView] = useState<'sand' | 'record'>('sand')
  const onSand = view === 'sand'
  // Read once, when the record is first opened, and kept afterwards. The
  // board is a whole-history read and it does not change while somebody is
  // looking at it -- except when a match they are watching finishes, which is
  // what the dependency on a finished result is for.
  const [board, setBoard] = useState<ColiseumStandings | null>(null)
  const [boardFailed, setBoardFailed] = useState(false)

  // The two seats. Each is an address — an owner and a slug (ADR 22) — and
  // each arrives whole in one query parameter, so a link into this room seats
  // both fighters: `/coliseum?a=aaron/gyome&b=aaron/arahbo`.
  const [a, setA] = useState(params.get('a') ?? '')
  const [b, setB] = useState(params.get('b') ?? '')
  const [decks, setDecks] = useState<DeckTile[]>([])
  // The gate (ADR 35). Where Forge is not installed the gates simply do not
  // open — no greyed-out button, no excuse. The room is still worth walking
  // through, which is why this page renders whole either way.
  const [forgeReady, setForgeReady] = useState(false)
  const [games, setGames] = useState(10)
  const [job, setJob] = useState<Job | null>(null)
  const [forge, setForge] = useState<ForgeResult | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [checks, setChecks] = useState<Record<string, ValidationReport>>({})
  const cancelRef = useRef<null | (() => void)>(null)

  /** A shuffle the link asked for, read once at the first render.
   *
   *  Honoured for the *next* bout only, and then never again: opening
   *  somebody's link and pressing the gate open fights their exact match, and
   *  everything after that is freshly dealt. A room that replayed the same
   *  match every time the gate opened would be a room with one match in it. */
  const [asked] = useState(() => {
    const n = Number(params.get('s'))
    return Number.isInteger(n) && n > 0 && n <= SHUFFLE_CEILING ? n : 0
  })
  const spent = useRef(false)
  /** The shuffle the match on screen was dealt from — written into the link,
   *  never onto the page. */
  const [shuffle, setShuffle] = useState(asked)

  /** A match this room was already watching when the page was reloaded.
   *
   *  Read once, at the first render: after this the link is something the room
   *  *writes*, and re-reading it would have the room chasing its own tail. */
  const [resume] = useState(() => params.get('m') ?? '')
  /** Said out loud when the link named a match the arena no longer holds. */
  const [lost, setLost] = useState(false)

  /** Every bout the match has raised, in the order it raised them.
   *
   * The job's `partial` carries only the **newest** game and is cleared the
   * moment the match finishes, so a bout that lands while an earlier one is
   * still being told is gone from the wire before the room gets to it. That is
   * exactly what used to make a live series clip itself. Held here, the room
   * can finish telling one bout and then begin the next.
   *
   * **Bounded by the match rather than by a rule of thumb.** These are the
   * wire's own beats, not staged ones — the reel stages a single bout at a
   * time, however far behind it has fallen — and the gate caps a match at
   * twenty games. So the ceiling here is the same set of beats the finished
   * result carries a few seconds later anyway. Dropping a bout to save memory
   * would lose a fight somebody was queued up to watch, to save nothing worth
   * saving. */
  const [seen, setSeen] = useState<ForgeBeats[]>([])

  /** Take a tick from the match: hold the job, and keep any bout it brought.
   *
   *  Done on the tick rather than in an effect watching `partial`, because a
   *  bout arriving *is* an event — and an effect that writes state on every
   *  poll is a cascading render the room does not need. */
  const heed = useCallback((tick: Job) => {
    setJob(tick)
    const live = theaterBeats(tick.partial)
    if (!live) return
    setSeen((cur) =>
      cur.some((g) => g.game === live.game) ? cur : [...cur, live])
  }, [])

  // The record, fetched when somebody actually asks to see it — the same rule
  // the deck page's tabs follow. It is a read across the whole history and
  // most visits to this room never open it.
  //
  // Re-read when a match finishes while the record is the open place, because
  // the bout that just ended is now part of what the board is counting and a
  // stale board would be the room contradicting itself.
  useEffect(() => {
    if (view !== 'record') return
    let alive = true
    api.coliseumStandings()
      .then((b) => { if (alive) { setBoard(b); setBoardFailed(false) } })
      .catch(() => { if (alive) setBoardFailed(true) })
    return () => { alive = false }
  }, [view, forge])

  useEffect(() => {
    let alive = true
    api.coliseum()
      .then((d) => { if (alive) setData(d) })
      .catch(() => { if (alive) setFailed(true) })
    api.decks().then((d) => {
      if (!alive) return
      setDecks(d)
      // Both seats start occupied, because a form whose first state is
      // invalid scolds before anybody has touched it. A link that named the
      // fighters wins over the shelf's first two.
      const first = d[0]
      const second = d[1] ?? first
      if (first) setA((cur) => cur || `${first.owner}/${first.slug}`)
      if (second) setB((cur) => cur || `${second.owner}/${second.slug}`)
    }).catch((e) => { if (alive) setError(errorMessage(e)) })
    // Asked once, like `/api/claude`: a fact about the environment, and a
    // failed ask means the gates stay shut, which is the honest floor.
    api.forgeStatus()
      .then((st) => { if (alive) setForgeReady(st.available) })
      .catch(() => { if (alive) setForgeReady(false) })
    // **A reload does not cost you the fight** (Aaron, 2026-08-26: "When I
    // reload a page I lose the fight, anyway to avoid that and reload the
    // in-flight match?"). The match never went anywhere — it is being fought
    // on another machine and the arena has been holding it all along. What was
    // lost was only this room's handle on it, which lived in memory and died
    // with the page. It lives in the link now, so the room can ask for it back.
    if (resume) {
      api.job(resume)
        .then((held) => {
          if (!alive) return
          // A match that fell over is not a match to re-join. The room offers
          // to send them in again rather than replaying somebody's bad news.
          if (held.status === 'error') { setLost(true); return }
          heed(held)
          // Already over: the whole match is in the result, so the room walks
          // straight to the record and every bout is there to be watched back.
          if (held.status === 'done') {
            setForge(held.result as ForgeResult)
            return
          }
          const follower = followJob(resume, heed, undefined, held)
          cancelRef.current = follower.cancel
          follower.promise
            .then((done) => { if (alive) setForge(done.result as ForgeResult) })
            .catch((e) => { if (alive) setError(errorMessage(e)) })
        })
        // Evicted, never existed, or somebody else's — all one thing from
        // here, and all of them 404 by design (ADR 5). The room says so
        // kindly and holds the gates open; it never renders the refusal.
        .catch(() => { if (alive) setLost(true) })
    }
    return () => { alive = false; cancelRef.current?.() }
  }, [resume, heed])

  // The seats are in the URL, so a match somebody is watching is a link they
  // can send. Replaced rather than pushed: choosing an opponent is not a
  // navigation and should not need three Backs to undo.
  useEffect(() => {
    if (!a && !b) return
    setParams((cur) => {
      const next = new URLSearchParams(cur)
      if (a) next.set('a', a); else next.delete('a')
      if (b) next.set('b', b); else next.delete('b')
      return next
    }, { replace: true })
  }, [a, b, setParams])

  /** The handle on the match being watched, for as long as it is worth
   *  holding: the job's own once it exists, and the one the link arrived with
   *  until the arena answers about it either way. Without that second half a
   *  reload during the moment before the answer lands would throw away the
   *  very thing it was reloading to keep. */
  const handle = job?.id || (lost ? '' : resume)

  // The match and its shuffle ride in the link beside the seats, for two
  // reasons that happen to want the same thing. A reload can find its way back
  // to a fight in progress; and the link, sent to somebody else, seats the same
  // two decks and deals them the same shuffle, so pressing the gate open plays
  // out the very same bout. Replaced rather than pushed: watching a match is
  // not a navigation and should not need three Backs to leave.
  useEffect(() => {
    setParams((cur) => {
      const next = new URLSearchParams(cur)
      if (handle) next.set('m', handle); else next.delete('m')
      if (shuffle) next.set('s', String(shuffle)); else next.delete('s')
      return next
    }, { replace: true })
  }, [handle, shuffle, setParams])

  /** A deck the shelf knows, by its whole address. */
  const deckAt = useCallback((address: string) => {
    const cut = address.indexOf('/')
    const owner = cut < 0 ? '' : address.slice(0, cut)
    const slug = cut < 0 ? address : address.slice(cut + 1)
    return decks.find((d) => d.slug === slug && (!owner || d.owner === owner))
      ?? null
  }, [decks])

  const slugOf = (address: string) => {
    const cut = address.indexOf('/')
    return cut < 0 ? address : address.slice(cut + 1)
  }

  // What a match will leave out, before it is paid for — both seats, because
  // the bill here is minutes of real games. Asked only about a deck the shelf
  // has already said has errors, so the common case costs nothing.
  useEffect(() => {
    let live = true
    for (const address of [a, b]) {
      if (!address || checks[address]) continue
      const tile = deckAt(address)
      // `errors` is null when the pool never answered and the gate never ran,
      // which is not a pass and must not render as one — but it is also not
      // something this room can diagnose, so it asks nothing.
      if (!tile?.errors) continue
      api.validate({ owner: tile.owner, slug: tile.slug })
        .then((r) => { if (live) setChecks((c) => ({ ...c, [address]: r })) })
        .catch(() => undefined)   // a caution nobody can fetch is not an error
    }
    return () => { live = false }
  }, [a, b, decks, checks, deckAt])

  const arena: ColiseumArena | undefined = data?.arenas[chosen]
  const facts = useMemo(() => arena?.facts ?? [], [arena])

  // `nudge` is bumped by every deliberate click, which restarts both clocks
  // below. Without it a click could be followed a heartbeat later by the timer
  // firing anyway — the room walking off just as somebody chose to stand
  // still, which is the single most irritating thing a carousel does.
  const [nudge, setNudge] = useState(0)

  // Walking into a house begins at its first fact rather than halfway through
  // the last one's — done by moving both values together in `enter`, never by
  // an effect that watches `chosen` and calls `setSlide`. That shape schedules
  // a second render for every arena change and oxlint refuses it by name
  // ("calling setState synchronously within an effect can trigger cascading
  // renders"); the two pieces of state change for one reason, so they change
  // in one place.
  const enter = useCallback((next: number) => {
    setNudge((n) => n + 1)
    setChosen(next)
    setSlide(0)
  }, [])

  useEffect(() => {
    if (facts.length < 2) return
    const id = window.setInterval(
      () => setSlide((n) => (n + 1) % facts.length), SLIDE_MS)
    return () => window.clearInterval(id)
  }, [facts.length, chosen, nudge])

  // And the room itself walks on, so six arenas are seen rather than one.
  const arenaCount = data?.arenas.length ?? 0
  useEffect(() => {
    if (arenaCount < 2) return
    const id = window.setInterval(() => {
      setChosen((n) => (n + 1) % arenaCount)
      setSlide(0)
    }, ARENA_MS)
    return () => window.clearInterval(id)
  }, [arenaCount, chosen, nudge])

  const step = useCallback((by: number) => {
    if (facts.length === 0) return
    setNudge((n) => n + 1)
    setSlide((n) => (n + by + facts.length) % facts.length)
  }, [facts.length])

  // ------------------------------------------------------------- the match

  const running = submitting
    || job?.status === 'queued' || job?.status === 'running'
  /** Pressed, and the job has not reported back yet — the seconds a match
   *  spends starting a JVM on another machine, and where "did that click
   *  land?" lives. */
  const lighting = submitting && !job

  async function sendThemIn() {
    if (!a || !b) return
    cancelRef.current?.()
    setSubmitting(true)
    setError(null)
    setForge(null)
    setJob(null)
    setLost(false)
    // The bouts of the last match are not this one's. Cleared here rather than
    // when the next one lands, so the field is empty from the click.
    setSeen([])
    // The shuffle a link asked for is honoured once, and after that every
    // match is freshly dealt.
    const dealt = (!spent.current && asked) || drawShuffle()
    spent.current = true
    setShuffle(dealt)
    try {
      const submitted = await api.simForge({
        a_slug: slugOf(a), a_owner: a.slice(0, Math.max(0, a.indexOf('/'))),
        b_slug: slugOf(b), b_owner: b.slice(0, Math.max(0, b.indexOf('/'))),
        games, seed: dealt,
        // This room watches, so this room narrates. The measuring surfaces do
        // not ask, and Forge stays quiet for them.
        narrate: true,
      })
      heed(submitted)
      const follower = followJob(submitted.id, heed, undefined, submitted)
      cancelRef.current = follower.cancel
      const finished = await follower.promise
      setForge(finished.result as ForgeResult)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setSubmitting(false)
    }
  }

  const aDeck = deckAt(a)
  const bDeck = deckAt(b)
  const rows = forge ? forge.rows : theaterRows(job?.partial)

  /** Every bout the room can show, in order.
   *
   * Two sources, and the finished one wins. While the match runs these are the
   * bouts the room caught coming off the wire; once it is over the result
   * carries the whole match at once, including any bout that landed in a
   * moment nobody was listening. */
  const played = useMemo((): ForgeBeats[] =>
    forge?.beats?.length ? forge.beats : seen, [forge, seen])

  /** Which bouts exist, as plain numbers — the running order the reel walks
   *  and the chips the transport draws. */
  const bouts = useMemo(() => played.map((g) => g.game), [played])

  /**
   * One bout, turned into words.
   *
   * A resolver rather than a bag of beats, and that is what keeps a long
   * series bounded: the reel asks for the bout it is about to tell and holds
   * only that one staged. A twenty-game match falls behind by nineteen bouts
   * at a study pace, and nineteen bouts of English is not a thing to be
   * carrying around when nobody is reading eighteen of them.
   *
   * Translated here rather than on the field because turning a slug into a
   * name needs the shelf, and the shelf is this room's.
   */
  const stage = useCallback((game: number): Arriving | null => {
    const heard = played.find((g) => g.game === game)
    if (!heard) return null
    const name = (slug: string) => {
      const of = decks.find((d) => d.slug === slug)
      return shortName(of?.name ?? slug, of?.commander)
    }
    // Forge counts every player-turn; a person counts their own. See
    // `playerTurns` for the measurement that made this necessary.
    const turns = playerTurns(heard.beats)
    return {
      game: heard.game,
      truncated: heard.truncated,
      // The board rides with the beats it belongs to, so the picture and the
      // sentences can never be about different games — see `lib/reel.ts`.
      board: heard.board,
      beats: heard.beats.map((beat, i): StagedBeat => {
        const said = beatLine(beat, name)
        return {
          // The game and the beat's own place in it: a match replays row
          // numbers 1..n and a game replays beat 0..n, so neither alone is an
          // identity and both together are.
          key: `${heard.game}:${i}`,
          game: heard.game,
          turn: (beat.turn == null ? 0 : turns.get(beat.turn) ?? beat.turn),
          kind: beat.kind,
          who: said.who, text: said.text,
          // Carried alongside the sentence rather than dug back out of it:
          // the board marks the card a beat names, and Forge already said
          // which one.
          // ...and how a permanent arrived, for the same reason: the arena
          // opens a battlefield under a creature nothing cast, and this beat
          // is the only place that fact is said.
          // ...and the id beside the name, because a block has to be read
          // back to the exact attacker it stopped and two tokens share a
          // spelling. See `StagedBeat.id`.
          card: beat.card, id: beat.id, target: beat.target,
          entered: beat.entered,
        }
      }),
    }
  }, [played, decks])

  // **One clock for the room, and one running order.** The reel drains a
  // bout's beats at reading speed and says how many it has told; the board is
  // folded to exactly that many steps, because the server builds one step per
  // beat. And when a bout ends the reel walks on to the next one the match has
  // raised, rather than being clipped by it the moment it lands.
  // **How fast the room reads, which is not how fast the Forge plays.** The
  // match runs flat out and lands its results when it lands them; this only
  // governs the retelling. `Watch` is the natural pace — a game spread across
  // the time the next one takes to arrive.
  const [speed, setSpeed] = useState<Speed>('play')
  // The job is the match's identity: beats raised by the last match are not
  // this one's, and the room is not remounted between them.
  // `seek` moves the reel's own mark, so pressing play after a scrub carries
  // on from where the hand left off rather than snapping back.
  const [reel, seek, series] = useReel(job?.id ?? '', bouts, stage, speed)

  /** Where each of this bout's turns begins, for the transport's turn step.
   *
   *  **A player's turn, which is the unit somebody studying a game wants**
   *  (Aaron, 2026-08-26: "a player's turn at a time, not a full two player
   *  turn"). Forge prints a turn line per seat and alternates them, so
   *  consecutive `turn` beats are one player's turn apart and nothing has to
   *  be halved — the halving is exactly the trap `lib/theater.ts` records this
   *  project falling into once already.
   *
   *  Held as a count of beats *told* rather than an index into them, because
   *  that is what `seek` takes: landing on `i + 1` puts the turn's own
   *  announcement on the board as the last thing said, so stepping to a turn
   *  shows the turn beginning rather than the instant before it.
   *
   *  Told and untold together, so the step reaches a turn the room has not
   *  read out yet — that is the whole point of stepping rather than watching.
   *  It stops at this bout's end either way: the next bout is a different
   *  fight and the room tells each to its end. */
  const turnMarks = useMemo(
    () => turnMarksOf([...reel.shown, ...reel.queue]),
    [reel.shown, reel.queue])

  /** What the field calls a seat. The board carries slugs and Forge's own deck
   *  titles; only the room has the shelf that turns either into a name — and
   *  the shelf is also where the general comes from, which is what decides
   *  whether the title shortens at all. A `fallback` is Forge's own title for
   *  a deck the shelf does not have, so it is shortened against no commander
   *  and comes back whole, which is the right way to be wrong. */
  const seatName = useCallback((slug: string | null, fallback: string) => {
    const of = slug ? decks.find((d) => d.slug === slug) : undefined
    return shortName(of?.name ?? (slug || fallback), of?.commander)
  }, [decks])

  /** Which general a slug is sleeved behind, for the surfaces that shorten a
   *  deck's name and only have the match's own rows to go on. A `ForgeDeckRow`
   *  carries a slug and a title; the shelf is the only thing in the room that
   *  knows whether that title is the commander's name or a sentence somebody
   *  liked, and `shortName` cannot tell them apart without it. */
  const commanderOf = useCallback((slug: string) =>
    decks.find((d) => d.slug === slug)?.commander, [decks])

  // The play-by-play is offered from the moment a match starts and stays
  // offered after it ends, because the last game finishing is the moment
  // somebody most wants to read what just happened.
  const hasBeats = Boolean(job) && (running || Boolean(forge))


  return (
    <div className="mx-auto max-w-5xl px-4 pb-16">
      {/* Not `PageMasthead`, and that is a deliberate departure rather than
          an oversight. That component is a *nameplate* — the painting beside
          the title — and its rule exists to stop a 1.37:1 crop being flattened
          into a band. This room's subject IS a place, so the place is the
          page's first fact; the ratio is kept whole, which is what that rule
          was actually protecting. The h1 moves here with it. */}
      <Hero />

      <p className="mt-6 max-w-2xl text-[0.95rem] leading-relaxed text-[var(--text-muted)]">
        Six houses, and what Rome did in each of them. Send two decks in and
        {' '}<Term name="tier-3">the Forge</Term> plays real games — whole
        ones, with an opponent, told blow by blow. It takes minutes; this is
        what those minutes are for.
      </p>

      {/* **The room's two places.** Not the arena strip below — that one picks
          which house a match is watched in, and it stays where it is. This one
          picks between fighting and remembering.

          `.strip-tab` rather than `.btn` for the reason the arena strip uses
          it (commandment 17): these are places you go, not actions you take.
          Same class, same `is-active` ink, same shape `Library` and
          `DeckDetail` wear. */}
      <div role="tablist" aria-label="The coliseum"
           className="mt-6 flex flex-wrap border-b"
           style={{ borderColor: 'var(--hairline)' }}>
        <button type="button" role="tab" aria-selected={onSand}
                onClick={() => setView('sand')}
                className={`strip-tab -mb-px border-b-2 px-3 py-2 text-sm
                            font-medium${onSand ? ' is-active' : ''}`}>
          The sand
        </button>
        <button type="button" role="tab" aria-selected={!onSand}
                onClick={() => setView('record')}
                className={`strip-tab -mb-px border-b-2 px-3 py-2 text-sm
                            font-medium${!onSand ? ' is-active' : ''}`}>
          The record
        </button>
      </div>

      {/* The record. A whole-history read, so it says so while it is coming
          rather than flashing an empty room at somebody who has fought fifty
          bouts — an empty state shown by mistake is a lie about their record,
          and it is the one thing this surface must never get wrong. */}
      {!onSand && (
        <>
          {boardFailed && (
            <p className="mt-8 text-[var(--text-muted)]">
              The record keeper is not at his desk — the room could not open
              its books just now.
            </p>
          )}
          {!boardFailed && !board && (
            <p className="mt-8 text-[var(--text-muted)]">
              Opening the books…
            </p>
          )}
          {!boardFailed && board && <ColiseumRecord board={board} />}
        </>
      )}

      {/* The gates. Only where the gate said Forge is installed — absent,
          never greyed out with an excuse, which is the rule the Ask Claude
          surfaces set and this inherits. The room itself is worth walking
          through either way, so nothing else on the page depends on it. */}
      {onSand && forgeReady && (
        <div data-open={running ? 'true' : 'false'}
             className="card-surface gatehouse mt-5 flex flex-wrap items-end
                        gap-3 rounded-xl p-4">
          {/* **The selects carry a floor, and that is the whole fix.**
              Three rounds of this bug, and the first two treated a
              *breakpoint* as if it knew how wide the room was.

              `flex-1` is `flex: 1 1 0%`, and with `min-w-0` these two were
              the only items on the row that could give: the games field is a
              fixed `w-28` and the button holds its label. So they absorbed
              every deficit by shrinking toward nothing, and `flex-wrap` never
              saved them — an item with `min-width: 0` always fits, so the row
              has no reason to break. Measured on the deployed room with the
              `sm:` branch live and the container at phone widths: 32px at
              390, 19px at 520, 64px at 610, and one single row at every one
              of them.

              Round two hung the fix on `basis-full` below `sm`, which is
              correct only while the breakpoint agrees with the device. Aaron's
              phone reports a layout width at or above 640 — Safari's desktop
              mode does exactly that, and it is sticky per site — so it took
              the `sm:` branch at phone width and crushed, and the second fix
              never applied to the person who reported it.

              A minimum width does not care what the device claims to be. At
              `11rem` the pair cannot shrink into a smear; when the row runs
              out of space they *wrap*, which is what `flex-wrap` was there to
              do all along. `grow` rather than `flex-1` because grow leaves
              the basis alone, and the basis is what makes them take their own
              line on a real phone. The ellipsis on a long deck name survives:
              a `<select>` truncates its own option text at any width. */}
          <Select label="Champion" value={a} onChange={setA}
                  className="min-w-[11rem] grow basis-full
                             sm:basis-[13rem] sm:max-w-[15rem]"
                  options={decks.map(seatOption)} />
          <Select label="Challenger" value={b} onChange={setB}
                  className="min-w-[11rem] grow basis-full
                             sm:basis-[13rem] sm:max-w-[15rem]"
                  options={decks.map(seatOption)} />
          <NumberField label="Games" value={games} onChange={setGames}
                       min={1} max={20} help={help('sim.forge_games')} />
          {/* **The gate is a card, and it is why this row is tall.** It comes
              last and it is the one item here that is not a form control's
              shape: a whole `Arena`, portrait, standing about four times the
              height of a select. `items-end` above is what makes that work
              rather than a squash — the pickers, the games dial and the card
              all stand on the same floor, which is what a gate and its jambs
              do. Letting the panel grow was the choice; the alternative was
              flattening a Magic card to a control's height, and a card
              flattened is a card reframed. Every other argument — the
              material, the three labels, the printing, and why it is the
              whole card rather than a crop — lives on `SendThemIn` rather
              than here, so there is one place to read them and one to change
              them. */}
          <SendThemIn running={running} lighting={lighting}
                      disabled={running || !a || !b}
                      onPress={() => void sendThemIn()} />
        </div>
      )}

      {/* The card the control is cut from, said once, under it. See
          `SendThemIn` for why the fact is here at all. */}
      {onSand && forgeReady && <ArenaFootnote />}

      {/* What a match will leave out, before it is paid for — both seats. */}
      {onSand && <DeckCaution report={checks[a]} name={aDeck?.name ?? slugOf(a)} />}
      {onSand && b !== a && (
        <DeckCaution report={checks[b]} name={bDeck?.name ?? slugOf(b)} />
      )}

      {onSand && error && <ErrorNote>The match failed: {error}</ErrorNote>}

      {/* A link that named a match the arena is no longer holding. Not an
          error: a match lives as long as the arena keeps it, an old link
          outlives one, and somebody else's link was never this room's to
          open (ADR 5 answers all three the same way, and the room cannot
          tell them apart — which is the point). So it is said the way a
          doorman says it, and the gates are standing open right there
          (commandment 2). */}
      {onSand && lost && (
        <p className="mt-4 text-[0.9rem] text-[var(--text-muted)]">
          That bout has left the arena — the sand was raked and the gates
          closed behind it. Send them in again whenever you are ready.
        </p>
      )}

      {/* The score. One element across both phases — fed from the job's
          `partial` while the games are landing and from the result's own rows
          once they have all landed — so the pips that lit one at a time are
          still lit when the match is over and the win has somewhere to arrive.

          Keyed on the job id, which is the only remount anybody wants: a
          second match between the same two decks reuses the row numbers 1..n,
          so without it React would match the new match's rows to the old
          one's elements and the second run would play out with no strikes at
          all. */}
      {onSand && job && (running || forge) && (
        <div className="mt-6">
          <MatchTheater
            key={job.id}
            a={aDeck} b={bDeck}
            aSlug={slugOf(a)} bSlug={slugOf(b)}
            games={forge ? forge.games : (job.total || games)}
            rows={rows}
            running={running}
          />
        </div>
      )}

      {/* **The field, at full width, and that is the layout decision.** Two
          Commander battlefields, two hands, two land rows and two graveyards
          do not fit in a column beside a painting — a board squeezed into one
          is a board of thumbnails, and a graveyard reduced to a number is the
          thing this was built to stop being. So the arena's painting and the
          house's facts move below it while a match is on, and the thing you
          came to watch takes the room.

          Keyed on the job for the same reason the theater is: a second match
          begins on an empty field rather than on the last one's board. */}
      {onSand && hasBeats && (
        <div className="mt-6">
          {/* **What the room still owes you.** A series is told a bout at a
              time and the Forge raises them faster than anybody can watch
              them, so without this the only evidence that more were coming
              was a row of numbered chips that could equally have been a
              finished match. It is a count of fights, in the room's own
              words — never of anything that computes them (commandment 10) —
              and the chips under the field are how you get to one early. */}
          {series.waiting > 0 && (
            <p className="bouts-waiting">
              <span className="bouts-waiting-mark" aria-hidden="true" />
              {series.waiting === 1
                ? 'The next bout waits its turn.'
                : `${series.waiting} more bouts wait their turn.`}
            </p>
          )}
          <MatchBoard key={`board-${job?.id}`} board={reel.board}
                      zones={data?.zones ?? []}
                      shown={reel.told} game={reel.game}
                      name={seatName} running={running}
                      beat={reel.shown[reel.shown.length - 1] ?? null}
                      speed={speed} setSpeed={setSpeed}
                      of={reel.shown.length + reel.queue.length}
                      seek={seek} turns={turnMarks}
                      games={bouts}
                      playing={reel.game} chooseGame={series.pick} />
        </div>
      )}

      {/* **The result, directly under the sand** (Aaron, 2026-08-27: "our
          results block is wrong, it currently sits below the coliseum facts
          panel, it should sit right below the sandbox"). It used to render
          inside the arena's own block, after the painting, after the house's
          thirteen slides and after the champions — so the way to your own
          match's numbers ran the length of a room about Rome.

          Two things came free with the move and both were bugs. It no longer
          hangs off `data && arena`: the result is about the match, and a
          coliseum whose prose failed to load is no reason to withhold the
          score of a fight somebody just watched. And it sits with the things
          that are about *this* match — the score above it and the field above
          that — rather than downstream of the furniture. */}
      {onSand && forge && (
        <section aria-label="The result" className="mt-6 space-y-4">
          {/* Keyed on the job so a second match crowns again rather than
              reusing the element the last one was dismissed from. */}
          <MatchVerdict key={`verdict-${job?.id ?? 'x'}`} result={forge} />
          <TaleOfTheTape result={forge} homeSlug={slugOf(a)}
                         commanderOf={commanderOf} />

          {/* **The shuffle is written down, and it is not written here.**
              This line used to be a badge reading "shuffle 7" — a bare
              number, naming nothing, that a person could do nothing with
              and that commandment 10 exists to keep off the page. The
              shuffle still exists and is still recorded against this
              result; what it buys the watcher is *this*, said in the only
              terms that mean anything to them: the same two decks, dealt
              the same way, fighting the same fight again. */}
          <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
            This bout is sealed into the link to this page. Send it to
            somebody and the same two decks are dealt the same hands and
            fight it out exactly as they did here.
            {' '}<CopyTheLink />
          </p>
          <Caveat>{forge.caveat}</Caveat>
        </section>
      )}

      {onSand && failed && (
        <p className="mt-8 text-[var(--text-muted)]">
          The coliseum is dark tonight — its doors did not answer.
        </p>
      )}

      {onSand && data && arena && (
        <>
          {/* Places rather than actions, so `.strip-tab` rather than `.btn`
              (commandment 17). */}
          {/* Places rather than actions, so `.strip-tab` rather than `.btn`
              (commandment 17). The class carries colour and transition only —
              the border, padding and the `is-active` ink come from here, which
              is the shape `Library` and `DeckDetail` already use. */}
          <div role="tablist" aria-label="Arenas"
               className="mt-6 flex flex-wrap border-b"
               style={{ borderColor: 'var(--hairline)' }}>
            {data.arenas.map((a, i) => (
              <button
                key={a.key}
                role="tab"
                aria-selected={i === chosen}
                onClick={() => enter(i)}
                className={`strip-tab -mb-px border-b-2 px-3 py-2 text-sm font-medium${
                  i === chosen ? ' is-active' : ''}`}
              >
                {a.name}
              </button>
            ))}
          </div>

          <div className="mt-4 grid gap-6 lg:grid-cols-[1.15fr_1fr]">
            <div>
              <Stage arena={arena} />
              {/* Somebody painted this. Name them. */}
              <p className="mt-2 text-[0.75rem] text-[var(--text-muted)]">
                {arena.plane}
                {arena.backdrop && <> · <em>{arena.backdrop.name}</em></>}
                {' · '}{arena.art.printing}, art by {arena.art.artist}
              </p>
            </div>

            {/* **The house, and only the house.** This column used to be two
                places behind a pair of tabs — the arena's lore, and beside it
                "The account", a column of sentences retelling the game that
                the board above was already showing. The account is gone
                (Aaron, 2026-08-26: "I don't think that is useful at all
                anymore"). Worth naming precisely what left, because the label
                invited the wrong reading: it was never a log of the machinery,
                it was the narration — and the board still carries every beat
                of it, marks the permanent each one is about, and walks under
                the hand a beat at a time.

                So one place needs no tab strip to choose it with. A lone tab
                is a control that cannot do anything; a heading is what a
                column that is simply itself wears, and it is the same heading
                "Who fights here" below already uses. The facts stay put
                whether or not a match is on — they are the reason this room
                existed before it could run anything, and hiding them the
                moment a match begins would be the room forgetting itself. */}
            <section className="flex min-h-[14rem] flex-col">
              <h2 className="mb-3 border-b pb-2 text-sm font-semibold uppercase
                             tracking-[0.12em] text-[var(--text-muted)]"
                  style={{ borderColor: 'var(--hairline)' }}>
                The house
              </h2>
              <div className="flex min-h-0 flex-1 flex-col">
                  <div>
                    {facts[slide] && <Slide key={slide} fact={facts[slide]} />}
                  </div>
                  {/* **The slack, given a tenant.** The controls are pinned
                      to the bottom of this column on purpose — the painting
                      beside it is tall, the thirteen slides are not the same
                      length, and controls that move as the prose changes are
                      controls you have to find again every time. Measured on
                      the deployed room, that leaves between 162 and 274
                      pixels of nothing above them, which is a hole rather
                      than breathing room.

                      So the hole is where he stands, and it is *why* he can
                      stand: he takes exactly the space the slide did not, up
                      to a ceiling, and never less than his floor. On a phone
                      the two columns stack and there is little slack left —
                      the floor is what keeps him in the room there, and it
                      is the reason he is not simply sized to the gap.

                      Decorative, so `aria-hidden`: the facts beside him
                      already say what a secutor was, and a screen reader
                      reading a caption of the illustration of the thing it
                      just described is noise. */}
                  <div className="coliseum-yard">
                    <img className="coliseum-secutor" src={secutorArt}
                         alt="" aria-hidden="true" />
                  </div>
                  <div className="mt-4 flex items-center gap-2">
                    {/* `.btn` alone is a border-box with a transparent
                        border and no voice at all: no hover, no press,
                        nothing. These two carry the whole thirteen-slide
                        walk through an arena's lore and they answered the
                        hand with silence. `.btn-quiet` is the house's
                        patient voice and it was one word away. */}
                    <button type="button" className="btn btn-quiet btn-sm"
                            onClick={() => step(-1)}>Back</button>
                    <button type="button" className="btn btn-quiet btn-sm"
                            onClick={() => step(1)}>Next</button>
                    <span className="ml-1 text-[0.75rem] text-[var(--text-muted)]">
                      {facts.length === 0 ? 'nothing to tell'
                        : `${slide + 1} of ${facts.length}`}
                    </span>
                  </div>
              </div>
            </section>
          </div>

          <section aria-label={`Champions of ${arena.name}`} className="mt-8">
            <h2 className="text-sm font-semibold uppercase tracking-[0.12em]
                           text-[var(--text-muted)]">
              Who fights here
            </h2>
            {arena.champions.length === 0 ? (
              <p className="mt-3 text-[0.9rem] text-[var(--text-muted)]">
                The stable is empty until the card pool answers.
              </p>
            ) : (
              <ul className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {arena.champions.map((c) => (
                  <li key={c.name}>
                    {/* The card itself on hover: a gladiator named and not
                        shown is a name, and the whole point is who fights. */}
                    <CardHover card={c} className="block">
                      <div className="flex gap-3">
                        {c.art_crop && (
                          <img src={c.art_crop} alt="" loading="lazy"
                               className="h-16 w-24 shrink-0 rounded object-cover" />
                        )}
                        <div className="min-w-0">
                          <p className="text-[0.85rem] font-semibold text-[var(--ink)]">
                            {c.name}
                          </p>
                          <p className="mt-0.5 text-[0.78rem] leading-snug text-[var(--text-muted)]">
                            {c.role}
                          </p>
                        </div>
                      </div>
                    </CardHover>
                  </li>
                ))}
              </ul>
            )}
          </section>

          {!data.pool && (
            <p className="mt-8 text-[0.8rem] text-[var(--text-muted)]">
              No card pool has answered, so the arenas are showing without their
              paintings. Everything written here still stands.
            </p>
          )}
        </>
      )}
    </div>
  )
}
