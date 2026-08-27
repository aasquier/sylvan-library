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

import campusArt from '../assets/coliseum/campus.webp'
import cryptaArt from '../assets/coliseum/crypta.webp'
import mementoArt from '../assets/coliseum/memento.webp'
import viaArt from '../assets/coliseum/via.webp'
import type { ForgeBoard } from '../lib/api'
import type { Speed, StagedBeat } from '../lib/reel'
import { faceFor, mannerOf, plateNote, plateWord, type Staged, stagedMana,
  stageLife, type StagedMana, useStaged, useStagedMana } from '../lib/stage'
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

/**
 * **Several of one card, drawn as the pile it is** — the board's own language,
 * spoken here rather than a second one invented for the middle of the arena.
 *
 * A row of Clue Tokens on the sand is one card with leaves fanned behind it
 * and a count in its corner (`.field-card-pile`, `.field-card-count`), and the
 * moment three of them are conjured is the same fact a second earlier. Drawing
 * it any other way would teach a person two notations for one thing.
 *
 * **Fanned from the first frame, and that is the one departure.** On the board
 * the fan opens under a thumb, because a pile standing there is a thing you go
 * and count. This is over in a second and a quarter and nobody is going to
 * hover it — so the leaves arrive already spread, which is what the board's
 * pile looks like at the moment somebody is actually looking at it.
 *
 * Four leaves at four or more and two below that: the same cap, and the same
 * reason. The arc is a texture saying *there are several*; the number in the
 * corner is the tally, and a fan twelve deep would cover the arena to say a
 * thing already written down.
 */
function StagePile({ item }: { item: Staged }) {
  return (
    <span className="stage-pile" aria-hidden="true"
          style={item.image
            ? ({ '--leaf-art': `url(${item.image})` } as CSSProperties)
            : undefined}>
      {(item.count >= 4 ? [-1, -0.42, 0.42, 1] : [-1, 1]).map((leaf) => (
        <span key={leaf} className="stage-leaf"
              style={{ '--leaf': leaf } as CSSProperties} />
      ))}
    </span>
  )
}

/**
 * **The road**, drawn behind a card that is leaving the game or arriving from
 * outside it, and behind nothing else.
 *
 * Aaron, 2026-08-27: *"We need a good animation for the exile mechanic, a
 * photo-real winding road leading into the distance of some kind?"* This is
 * the road — the Via Appia, which `via.recipe.yaml` argues at length, including
 * why it is that road and not another and why it is a photograph rather than
 * an engraving.
 *
 * **It runs both ways, and that is not a saving.** An exile is a card walking
 * away down it; a companion is a card walking *up* it — bought in from outside
 * the game, which is the one other moment in a match where the arena opens
 * onto somewhere that is not the table. Drawing a second picture for the
 * second direction would say those were two different elsewheres, and Magic
 * says they are one: the place a game keeps what is not in it. The stylesheet
 * reverses the journey; the road is the same road.
 *
 * **It goes underneath, because an exile happens to the card's *place*.** It
 * is still whole; it is simply not here any more. So the arena opens onto
 * somewhere else and the card walks off into it, rather than anything being
 * laid over the picture.
 *
 * That paragraph used to draw the line one step further over — it said a
 * *death* takes layers on the card because a death happens to the card, and
 * that a scene underneath was exile's alone. Half of that was right and the
 * other half was a sentence written before there was a crypt to put under a
 * death. A death is both things at once: the creature is changed (the pall,
 * the stone, the sinking) **and** it goes somewhere, and Magic's own wording
 * says so — rule 700.4 defines "dies" as *put into a graveyard from the
 * battlefield*, which is a zone change with a destination in it, exactly as
 * exile is. See `StageCrypt`, which is that destination.
 *
 * The road's own vanishing point is about four fifths across and just under
 * half way down, measured off the finished crop; the stylesheet sends the card
 * there. Those two numbers live in `index.css` beside the keyframes that use
 * them and are named in the recipe, which is the only coupling between a
 * picture and a stylesheet in this room.
 */
function StageRoad() {
  return (
    <span className="stage-road" aria-hidden="true">
      <img className="stage-road-art" src={viaArt} alt="" draggable={false} />
      <span className="stage-road-haze" />
    </span>
  )
}

/**
 * **The vault below**, drawn behind a card that is dying and nowhere else.
 *
 * Aaron, 2026-08-27, having watched the road land: *"I want something to show
 * when something dies and is going to the graveyard... A good crypt, tomb, or
 * gothic graveyard would work."* This is the crypt — a columbarium passage on
 * the Via Appia, which `crypta.recipe.yaml` argues at length, including why a
 * Roman burial chamber rather than the gothic one he named, and why it is the
 * same road the exile scene already runs down.
 *
 * **The skull stays, and that was the call this needed most.** Two objects
 * saying "dead" over one card is a real risk and it is the reason the question
 * was asked at all — but these two are not saying the same thing twice. The
 * stone is *on the card*, small, bright and sharp-edged, and it is what
 * happens to this creature. The vault is *behind* it, large, dark and soft,
 * and it is where the creature is going. Figure and ground, at different
 * depths and different values.
 *
 * Two concrete reasons beyond the argument, either of which would have carried
 * it alone. On a phone the card fills most of the arena and what shows around
 * it is mostly shadow, so a death without the stone is a card getting darker;
 * the stone is what makes the beat read at that size. And the skull is the one
 * object tying this to the same death drawn on the card in its own row and on
 * the graveyard pile — one event in three places, which is `lib/stage.ts`'s
 * whole argument about the clock, and dropping it here would leave the middle
 * of the arena and the board disagreeing about what just happened.
 *
 * **The card sinks and the vault comes forward**, which is the one place this
 * deliberately reads differently from the road while using the same motion.
 * The road creeps toward its own vanishing point behind a card travelling the
 * same way, so the ground moves *with* it. Here the card goes down and away
 * while the chamber comes toward the eye, and two things moving apart is what
 * makes one of them look left behind.
 */
function StageCrypt() {
  return (
    <span className="stage-crypt" aria-hidden="true">
      <img className="stage-crypt-art" src={cryptaArt} alt=""
           draggable={false} />
      <span className="stage-crypt-damp" />
    </span>
  )
}

/**
 * **The field below**, drawn behind a card that arrived on the battlefield
 * with nothing having cast it, and nowhere else.
 *
 * Aaron, 2026-08-27, in the same breath as the crypt: *"Same thing for 'Enters
 * the Battelfield', we should be able to find something cool. A free use
 * painting or picture of a battle before us, like down in a valley or
 * something stylized like our exile one?"* This is that valley — Jan
 * Brueghel's `Battle of Issus`, which `campus.recipe.yaml` argues at length,
 * including why the room's one non-Roman picture is the honest choice for the
 * one subject nobody can photograph.
 *
 * **The third scene, and the only one a card arrives *out of*.** The road
 * takes a card away down it and the vault takes one down into it; both are
 * departures, and both animate the card *shrinking*. This one runs the other
 * way. A permanent put onto the battlefield is not going anywhere — a second
 * later it is standing in a row on the board — so the card comes **toward**
 * the eye, out of the fight, and settles. Nothing here recedes, and that is
 * the whole difference between "it left" and "it is here now", drawn rather
 * than captioned.
 *
 * **The dust is the landing.** A second element over the photograph, blooming
 * outward from under the card on the beat it lands and gone before the plate
 * has been read. It is the one thing in the three scenes that is not a fact
 * about the picture: the road and the vault are places that were already
 * there, and a field with something dropped into it is a place that has just
 * been disturbed. Without it the card simply appears in front of a painting;
 * with it, something happened to the ground.
 *
 * **It goes underneath, for the road's reason.** An arrival happens to the
 * card's *place*, not to the card — it is unchanged, it is simply here now —
 * so the arena opens onto somewhere else and the card comes out of it, with
 * nothing laid over the picture.
 */
function StageField() {
  return (
    <span className="stage-field" aria-hidden="true">
      <img className="stage-field-art" src={campusArt} alt=""
           draggable={false} />
      <span className="stage-field-dust" />
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
      {(item.manner === 'exiled' || item.manner === 'companion')
        && <StageRoad />}
      {item.manner === 'dies' && <StageCrypt />}
      {item.manner === 'put' && <StageField />}
      <span className="stage-frame">
        {item.count > 1 && <StagePile item={item} />}
        <StageFace item={item} />
        {/* A death, in two layers over the card and never *through* it.

            Both are **children of the frame**, so they travel down with the
            card when it sinks — which is the whole difference between one
            continuous death and a skull and a card that happen to be on
            screen together.

            The pall is the grave coming up over the picture, and it is an
            element rather than a `filter` for a reason that is licence rather
            than taste: Scryfall's imagery guidelines forbid desaturating card
            imagery, and ADR 32 bounds this room to motion and parallax over
            the artwork and nothing else. Until tonight the light went out of
            a dying card through `grayscale()` on this very face. It is laid
            *on* the card now, the way the token materials lay light on an
            object rather than on the painting under it. `index.css`'s "the
            grave, coming up over it" argues the whole of it. */}
        {item.manner === 'dies' && (
          <>
            <span className="stage-pall" aria-hidden="true" />
            <img className="stage-skull" src={mementoArt} alt=""
                 draggable={false} />
          </>
        )}
        {/* The tally, in the corner the board puts it in and in the same
            brass. "3×" rather than "3": a bare number beside a card reads as
            a stat, which is a thing this room prints in a different corner of
            a different card. */}
        {item.count > 1 && (
          <span className="stage-count tabular">{item.count}<span
            className="stage-times">×</span></span>
        )}
      </span>
      {/* **One engraved line, then the name, then the rest of it.** The deed
          and the player are one string rather than two spans on purpose: a
          museum plate is a label, not a layout, and splitting "Gyome casts"
          into a name and a verb to give them different inks would make the
          eye assemble a sentence it should simply read. `lib/stage.ts` builds
          the whole phrase, so the one place that knows the card's type line
          is the one place that reads it. */}
      <span className="stage-plate-strip">
        <span className="stage-plate-word">{item.word}</span>
        <span className="stage-plate-title">{item.name}</span>
        {item.note && (
          <span className="stage-plate-note">{item.note}</span>
        )}
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
 * at the moment it is most wanted.
 *
 * The silence used to be justified here by the play-by-play beside it reading
 * the same beats out, and **that panel is gone** — #328 removed it, so the
 * words on this plate are currently the only place several of these moments
 * are said at all. The silence is still right for what this *is*: a hundred
 * and thirty announcements a game, arriving on a timer nobody controls, is a
 * live region that talks over everything else on the page. It is not right
 * forever, and the honest shape of the fix is the account coming back as
 * something a person can read at their own pace — `lib/theater.ts` keeps
 * `beatLine` correct against exactly that day.
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
  const name = beat?.card ?? null
  // **One lookup, three answers, and it now happens before the manner rather
  // than after it**: the painting to draw, the type line the plate reads, and
  // whether this card is a token — which is the thing `mannerOf` cannot decide
  // without, because a permanent entering play is only worth the middle of the
  // arena when nothing cast it. Asking the same list the same question twice
  // would be the waste; asking it once, first, is not.
  const face = name ? faceFor(board, name, seat) : null
  // **And the beat's own word for how a permanent arrived**, which the card
  // cannot answer: `face.token` says a thing was conjured, and nothing on a
  // card says whether the real spell standing on the sand was cast or put
  // there. `mannerOf` argues why an absent word draws nothing at all.
  const manner = beat ? mannerOf(beat.kind, face, beat.entered) : null
  // **A beat that only repeats the one before it takes nothing.** Four tokens
  // conjured at once arrive as four identical beats, and `countRuns` marks the
  // followers `0` so this moment is drawn once, with a four on it, instead of
  // being replayed under three more keys. A beat the reel never counted has no
  // `run` at all, and that is a one rather than a nothing.
  const times = beat?.run ?? 1
  const next: Staged | null = manner && name && beat && times > 0
    ? {
      key: beat.key,
      manner,
      name,
      word: plateWord(manner, face?.types, beat.who),
      note: plateNote(manner, beat.target),
      count: times,
      image: face?.image ?? null,
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
