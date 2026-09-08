/**
 * A card in a list, with the other side of it one press away.
 *
 * **The 99 could only ever show a card's front.** A transforming card and a
 * modal double-faced card are two paintings on one piece of cardboard, and the
 * deck page drew the first and offered no way at all to the second — so a
 * Pathway land in Hylda's deck was a picture of its front and a promise you
 * had to take on trust (Aaron, 2026-09-07: "we should have a flip icon on the
 * image you can click to flip the card").
 *
 * **Which cards get the control is the server's ruling, not a layout list
 * here.** Six layouts carry an `A // B` name and only two of them paint each
 * face separately; an Adventure, a split card and a flip card put both halves
 * on one painting, and offering to turn one over would be the interface
 * claiming a second picture that does not exist. The server sends `faces` only
 * when there is genuinely something to turn to, so the test here is simply
 * whether it did — `paintedTwice` in `deckread.go` carries that argument, and
 * `records.go` carries the Scryfall encoding it reads it off.
 *
 * **Every event the control fires is stopped from bubbling, and that is
 * load-bearing.** These plates stand inside the 99's rows, which carry their
 * own `onClick`, `onDoubleClick` and Enter/Space handlers whenever the action
 * bar is armed — so without this, turning a card over would also entomb it, or
 * open the art picker, depending on what was armed. `FieldHint` learned the
 * identical lesson one component across.
 *
 * The face resets when the card does, because the plate is keyed by name in
 * every list that draws it and React gives a new key a new component.
 */

import { useState } from 'react'
import type { Card } from '../lib/api'
import { TurnCardGlyph } from './glyphs'
import { CardArt, CardHover } from './ui'

export function CardFacePlate({ card, className = '', artClassName = 'w-16' }: {
  card: Card
  /** The plate's own classes. It is the positioning context for the mark. */
  className?: string
  /** The painting's classes — the width every caller actually varies. */
  artClassName?: string
}) {
  const [turned, setTurned] = useState(0)

  // **Undefined is "one face", never "the faces failed to arrive."** A deck
  // payload from before these keys existed lands in a browser that has them,
  // and the reading has to be the same either way. The lengths are compared
  // rather than trusted: three arrays that disagree are an index answering
  // confidently for the wrong painting, which is the one failure worse than
  // no control at all.
  const faces = card.faces ?? []
  const images = card.face_images ?? []
  const crops = card.face_art_crops ?? []
  const twoSided = faces.length > 1
    && images.length === faces.length
    && crops.length === faces.length

  // Indexed reads are `string | undefined` under this project's TypeScript,
  // and rightly: the three arrays agreeing is checked above rather than
  // assumed, so the fallbacks here are the same belief written once more where
  // the compiler can see it.
  const at = twoSided ? turned % faces.length : 0
  const name = (twoSided ? faces[at] : card.name) ?? card.name
  const crop = twoSided ? crops[at] : card.art_crop
  const image = twoSided ? images[at] : card.image
  const next = (twoSided ? faces[(at + 1) % faces.length] : '') ?? ''

  // A mouse press, a double press and a key press all reach the row behind
  // this. Stopped here rather than by the row not listening, because the row's
  // handlers are right for the ninety-five per cent of itself that is not this
  // mark.
  const alone = (e: { stopPropagation: () => void }) => e.stopPropagation()

  return (
    <span className={`card-plate ${className}`}>
      <CardHover card={{ name, image }}>
        <CardArt src={crop} alt={name} ratio="aspect-[626/457]"
                 className={`${artClassName} shrink-0 cursor-help`} />
      </CardHover>
      {twoSided && (
        <button
          type="button"
          className="card-flip"
          // The sentence a glyph this size cannot say. `glyphs.tsx` holds the
          // rule that a mark goes beside a label and never instead of one;
          // this is that label, said to a screen reader in full and shown to
          // an eye on hover and on focus.
          aria-label={`Turn ${name} over to ${next}`}
          onPointerDown={alone}
          onDoubleClick={alone}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') alone(e)
          }}
          onClick={(e) => { alone(e); setTurned((n) => n + 1) }}
        >
          <TurnCardGlyph size={13} />
          <span className="card-flip-says">{next}</span>
        </button>
      )}
    </span>
  )
}
