/**
 * The centre stage: the card, big, for the moment it is happening.
 *
 * The drawing only. Every decision about *what* goes in the middle of the
 * arena and *for how long* is in `lib/stage.ts`, which carries the argument —
 * including the one that matters most here, which is that a visual timed by
 * the beat is a visual cut off before it is half told. One number is both the
 * animation's duration and the element's life in the tree, and it reaches the
 * stylesheet as `--stage-life` rather than being written down there a second
 * time, so the two cannot drift.
 *
 * `index.css`'s "the centre of the arena" block is the other half of this
 * file; neither is readable without the other.
 */

import type { CSSProperties } from 'react'

import mementoArt from '../assets/coliseum/memento.webp'
import type { ForgeBoard } from '../lib/api'
import type { Speed, StagedBeat } from '../lib/reel'
import { faceFor, mannerOf, PLATE, type Staged, stagedMana, stageLife,
  type StagedMana, useStaged, useStagedMana } from '../lib/stage'
import { ManaPip } from './manasymbol'

/** The card itself, or the card set in type when there is no picture of it.
 *
 *  **A missing painting is not a missing card.** The board only draws what it
 *  was handed art for, which is right for a floor of forty cards and wrong
 *  here: this whole surface exists so that nothing in the game happens unseen,
 *  and falling back to nothing would put the hole back exactly where it was.
 *  So a card with no face is set in type instead — named, framed and rimmed in
 *  the arena's own brass, the way a card looks before the art is printed onto
 *  it. On a phone it is arguably the more legible of the two: a name in words
 *  at fifteen point beats the same name printed at four point across the top
 *  of a picture. */
function StageFace({ item }: { item: Staged }) {
  if (item.image) {
    return (
      <img className="stage-face" src={item.image} alt="" draggable={false} />
    )
  }
  return (
    <span className="stage-face stage-plate">
      <span className="stage-plate-rule" aria-hidden="true" />
      <span className="stage-plate-name">{item.name}</span>
      <span className="stage-plate-rule" aria-hidden="true" />
    </span>
  )
}

/** One card on the stage: the light behind it, the card, what is happening to
 *  it, and the plate saying so. */
function StageCard({ item, parting }: { item: Staged; parting?: boolean }) {
  const life = { '--stage-life': `${item.life}ms` } as CSSProperties
  return (
    <span style={life} aria-hidden="true"
          className={`stage-card is-${item.manner}`
            + (parting ? ' is-parting' : '')}>
      {/* Light gathering behind the card, and only behind it. A rectangle of
          scrim over the whole arena would take the board away from somebody
          scrubbing the timeline; a pool centred on the card separates it from
          the sand and leaves the rows either side perfectly readable. */}
      <span className="stage-veil" aria-hidden="true" />
      <span className="stage-frame">
        <StageFace item={item} />
        {/* The skull, falling onto the card as it dies. A **child of the
            frame**, so it travels down with the card when the card sinks —
            which is the whole difference between one continuous death and a
            skull and a card that happen to be on screen together. */}
        {item.manner === 'dies' && (
          <img className="stage-skull" src={mementoArt} alt=""
               draggable={false} />
        )}
      </span>
      <span className="stage-plate-strip">
        <span className="stage-plate-word">{PLATE[item.manner]}</span>
        <span className="stage-plate-title">{item.name}</span>
      </span>
    </span>
  )
}

/** **The mana that just arrived**, in the half of the arena it arrived for.
 *
 *  A row of pips coming up out of the sand: one per mana, because a pool
 *  holding two green is two things you can spend — which is the opposite of
 *  the rule for what a permanent *taps for*, where two colours are one mana
 *  with a choice attached — which is why this is `ManaPip` and not the mark
 *  that used to ride a turned card.
 *
 *  Staggered, and that is what makes three mana read as three. Landing
 *  together they are one shape the eye has to stop and count; landing a
 *  fraction apart they are counted on the way past, the way pips land on a
 *  die. The index reaches the stylesheet as `--pip-i`, so the whole cascade is
 *  one number per pip and no timers at all. */
function StageMana({ item }: { item: StagedMana }) {
  const life = { '--stage-life': `${item.life}ms` } as CSSProperties
  return (
    <span style={life} aria-hidden="true"
          className={`stage-mana is-${item.facing}`}>
      <span className="stage-mana-glow" aria-hidden="true" />
      {item.pips.map((pip) => (
        <span key={`${pip.symbol}-${pip.at}`} className="stage-mana-pip"
              style={{ '--pip-i': pip.at } as CSSProperties}>
          <ManaPip symbol={pip.symbol} size={22} />
        </span>
      ))}
    </span>
  )
}

/**
 * The middle of the arena, and what is happening in it.
 *
 * Mounted inside `.field` rather than over the whole stage, which is three
 * decisions at once: it is clipped to the sand, so nothing spills onto the
 * zone rails; it is centred on the *arena* rather than on the arena plus a
 * column of graveyards; and it inherits `--mark-life-dies` from `.field-stage`,
 * which is how a death drawn here and the skull on the card in its own row
 * stay one event rather than two.
 *
 * **It never takes a pointer event, and it says nothing to a screen reader.**
 * Somebody dragging the scrubber through a match is dragging it through sixty
 * casts, and a spell that swallowed the drag would make the timeline unusable
 * at the moment it is most wanted. The words are already in the play-by-play
 * beside it, read from the same beat — announcing them twice would be the room
 * talking over itself.
 */
export function CenterStage({ board, beat, speed, game, dies, seat, gained,
  at }: {
  board: ForgeBoard | null
  beat: StagedBeat | null
  speed: Speed
  game: number
  /** What each seat's pool *gained* on the beat being drawn, from the fold —
   *  `''` for the seats and the beats where nothing arrived, which is most of
   *  them. Two seats rather than one because an opponent can hold up mana on
   *  somebody else's turn, and a room that only drew the active player's would
   *  go quiet at exactly the moment a counterspell was being paid for. */
  gained: { far: string; near: string }
  /** How far into the game the room is showing, which is what a flash is keyed
   *  on. **The step count and not the beat's own key**, because scrubbing the
   *  timeline moves this while the beat under the pointer may not change at
   *  all — and a person dragging back through a turn should see the mana move
   *  again, not see it stuck on whatever it was when they grabbed it. */
  at: number
  /** How long a death is watched for at this pace — the marks' own number,
   *  passed in rather than worked out a second time. One event, one clock. */
  dies: number | null
  /** The seat the current beat belongs to, when the room knows it. Used only
   *  to prefer the caster's own copy of a card both seats run. */
  seat: number | null
}) {
  const manner = beat ? mannerOf(beat.kind) : null
  const name = beat?.card ?? null
  const next: Staged | null = manner && name && beat
    ? {
      key: beat.key,
      manner,
      name,
      image: faceFor(board, name, seat)?.image ?? null,
      life: stageLife(manner, speed, dies),
    }
    : null
  const { showing, parting } = useStaged(next, beat?.key ?? '', game)
  // Two seats, two slots, two hooks — never one slot chosen between them. Both
  // pools can move on one beat, and a shared slot would make the second seat's
  // mana silently delete the first's.
  const key = `${at}`
  const farMana = useStagedMana(
    stagedMana(gained.far, 'far', key, speed), key, game)
  const nearMana = useStagedMana(
    stagedMana(gained.near, 'near', key, speed), key, game)
  if (!showing && !parting && !farMana && !nearMana) return null
  return (
    <div className="stage" aria-hidden="true">
      {parting && <StageCard key={parting.key} item={parting} parting />}
      {showing && <StageCard key={showing.key} item={showing} />}
      {farMana && <StageMana key={farMana.key} item={farMana} />}
      {nearMana && <StageMana key={nearMana.key} item={nearMana} />}
    </div>
  )
}
