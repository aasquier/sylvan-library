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
import { faceFor, mannerOf, PLATE, type Staged, stageLife, useStaged }
  from '../lib/stage'

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
export function CenterStage({ board, beat, speed, game, dies, seat }: {
  board: ForgeBoard | null
  beat: StagedBeat | null
  speed: Speed
  game: number
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
  if (!showing && !parting) return null
  return (
    <div className="stage" aria-hidden="true">
      {parting && <StageCard key={parting.key} item={parting} parting />}
      {showing && <StageCard key={showing.key} item={showing} />}
    </div>
  )
}
