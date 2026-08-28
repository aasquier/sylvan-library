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

import { type CSSProperties, useEffect } from 'react'

import campusArt from '../assets/coliseum/campus.webp'
import certamenArt from '../assets/coliseum/certamen.webp'
import ossariumArt from '../assets/coliseum/ossarium.webp'
import cryptaArt from '../assets/coliseum/crypta.webp'
import fabricaArt from '../assets/coliseum/fabrica.webp'
import mementoArt from '../assets/coliseum/memento.webp'
import velatioArt from '../assets/coliseum/velatio.webp'
import viaArt from '../assets/coliseum/via.webp'
import type { ForgeBoard } from '../lib/api'
import type { Clash } from '../lib/board'
import type { Speed, StagedBeat } from '../lib/reel'
import { halfNamed } from '../lib/board'
import { halfGlassFor } from '../lib/halves'
import { ARCANA, boutAt, type BoutFighter, faceFor, mannerOf, plateNote,
  type Outcome, plateWord, sceneFor, type Staged, stagedBout, stagedMana,
  stageLife, type StagedBout, type StagedMana, useStaged, useStagedBout,
  useStagedMana } from '../lib/stage'
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
      <>
        <img className="stage-face" src={item.image} alt="" draggable={false} />
        {/* **The half of the card that is actually being cast, under glass.**

            A card with two names on one picture is two spells, and the room
            was showing neither: Forge renames the card the moment the second
            one goes on the stack, nothing in the match answered to *Stomp*,
            and the middle of the arena drew a black card with a title on it
            (Aaron, 2026-08-28). The card is found now — and finding it raises
            the second question, which is that a player who has just been told
            "Cast Instant" is looking at a Giant.

            So the whole card arrives, whole, and a magnifier stands over the
            half that was cast. That is Aaron's ask and it is also the only
            move the licence allows: Scryfall's imagery guidelines forbid
            cropping, distorting, desaturating and watermarking a card image,
            and ADR 32 makes that a wall. Nothing here cuts the picture — the
            card is laid out at its own aspect ratio, complete, and the glass
            is a *layer over it*, which is the same thing `.field-card-lens`
            has been doing on the battlefield since #312 and the same argument
            `.arena-gate` settled for the gate.

            **All four two-named layouts wear one**, which they did not: the
            glass belonged to an Adventure alone because an Adventure's box was
            the only one anybody had measured, and a split card and a flip card
            got the right picture and nothing over it. `lib/halves.ts` holds
            the measurements now — read off real faces at 488x680, several
            printings of each — and the arithmetic that turns a box into a
            lens.

            **The magnified copy is a second `<img>`, not a background**, and
            that is the fix rather than a preference. See `halfGlass`: a
            background percentage resolves against the glass rather than
            against the card, so the shipped `background-size: 155%` drew the
            card at seventy per cent — a magnifier that made things smaller —
            and scaled the two axes differently while it did, which is a
            *distortion* of a card image and one of the four words ADR 32 names
            outright. A copy laid over the card and scaled uniformly about the
            half's own centre cannot do either. */}
        {item.half && (
          <span className="stage-half" aria-hidden="true"
                style={{
                  '--glass-l': `${item.half.left}%`,
                  '--glass-t': `${item.half.top}%`,
                  '--glass-r': `${item.half.right}%`,
                  '--glass-b': `${item.half.bottom}%`,
                  '--card-x': `${item.half.cardLeft}%`,
                  '--card-y': `${item.half.cardTop}%`,
                  '--card-w': `${item.half.cardWidth}%`,
                  '--card-h': `${item.half.cardHeight}%`,
                  '--half-ox': `${item.half.originX}%`,
                  '--half-oy': `${item.half.originY}%`,
                } as CSSProperties}>
            <span className="stage-half-glass">
              <img className="stage-half-card" src={item.image} alt=""
                   draggable={false} />
            </span>
          </span>
        )}
      </>
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

/**
 * **The fourth scene, and the first that is not a place.**
 *
 * An instant and a sorcery never land. There is nowhere for them to arrive, so
 * there is nothing to open onto — the road, the vault and the valley are all
 * answers to *where has this gone* or *where has this come from*, and a
 * Lightning Bolt has been nowhere. What it has is a **shape**: something
 * sudden that happens to you, or something deliberate that you do.
 *
 * So the two of them stand in front of an arcanum instead, and that is the
 * vocabulary this room settled on (Aaron, 2026-08-28): a card that becomes an
 * object on the table opens onto a picture of a real place; a card that is only
 * ever an event is drawn from the deck of the fortune-teller's table. **The
 * Tower** for an instant — lightning through a crown, two figures falling,
 * which is what an instant does to somebody's plan — was his own suggestion.
 * **The Magician** for a sorcery is its answer from the other side: one hand
 * raised and one lowered over a table of prepared tools, on your own turn, at
 * your own pace. It happens to you, or you do it.
 *
 * **The composition is deliberately not the other three's.** Those are wide
 * bands laid across the sand, because a place is somewhere you look *into*.
 * This is a single card standing upright behind the spell, taller than it and
 * a little back, so the Magic card is held in front of the arcanum the way a
 * reader holds one card over another. Two portrait cards is the one thing the
 * three places could never draw, and it is the right thing here: nothing is
 * receding, because nothing is going anywhere.
 *
 * **Served from `/tarot/`, and that is why there is no import.** The 1909
 * Rider deck is package data on the server — 78 files with their provenance in
 * `assets/tarot/PROVENANCE.md`, public domain in both the US and the UK, and
 * already reachable at a path. Committing a second copy into the bundle would
 * be the same pictures twice, and the reading room's copy is the one with the
 * argument attached to it.
 */
/**
 * **The fourth place, and the one an object is made in.**
 *
 * Aaron, 2026-08-28: *"hopefully find a good image of a hammer hitting an anvil
 * with sparks for artifacts entering"*. Wright of Derby's `An Iron Forge` is
 * the picture and `fabrica.recipe.yaml` argues it at length, including why it
 * is not the Blacksmith's Shop he first chose — that one buries its own light
 * source under exactly where this stage draws a card, which a proof sheet said
 * and no amount of looking at the painting would have.
 *
 * **The sparks are the landing, and they are not in the painting.** The valley
 * learned this first: the road and the vault are places that were already
 * there and are simply opened, and a place with something *put down* in it has
 * just been disturbed. Dust is what a battlefield throws up; an anvil throws
 * sparks. Same element, same beat, different material — which is the whole of
 * what makes these four scenes one vocabulary rather than four decorations.
 *
 * It is also the half of Aaron's sentence the picture could not answer. He
 * asked for a hammer, an anvil **and sparks**; this painting has the first two
 * and a bar at white heat, and drawing the third as motion is better than
 * hunting for a painting that has all three and none of the depth.
 */
function StageForge() {
  return (
    <span className="stage-forge" aria-hidden="true">
      <img className="stage-forge-art" src={fabricaArt} alt=""
           draggable={false} />
      <span className="stage-forge-sparks" />
    </span>
  )
}

/**
 * **The fifth place, and the one an enchantment settles over.**
 *
 * Every other permanent in Magic is a thing you could pick up: a creature is a
 * body, an artifact is an object, an Equipment is a sword. An enchantment is
 * the one that is not — it is a condition laid on the *place*, and it stays
 * until somebody takes it off. So it gets the place: Piranesi's Pantheon
 * pronaos, sixteen granite monoliths holding up a roof that has held for
 * nineteen hundred years. `templum.recipe.yaml` argues it, including the three
 * subjects that were fetched and rejected first — among them the curse tablet
 * Aaron chose, which turned out to be a museum object on a cream mount and so
 * a *mark* rather than a place.
 *
 * No dust and no sparks. The two arrival scenes disturb their ground because
 * something was put down on it; nothing is put down here. An enchantment
 * arrives as a condition over a room that was already standing, and the room
 * simply opens.
 */
function StageTemple() {
  return (
    <span className="stage-temple" aria-hidden="true">
      {/* A `background-image` rather than an `<img>`, alone among the six.
          The picture is an etching and arrives grey; the stylesheet blends it
          `soft-light` into a warm ground the way the sand does with its own
          plate, so what it contributes is the structure and the colour stays
          the room's. A background is the only way to reach `background-blend-
          mode`, and the path lives in the stylesheet for the same reason the
          sand's does. */}
      <span className="stage-temple-art" />
    </span>
  )
}

/**
 * **The sixth, and it is a rite rather than a place.**
 *
 * An Aura is the one permanent that goes *on somebody*. It does not stand in a
 * row of its own — the board draws it tucked under the creature it is riding —
 * so the scene is not somewhere a card arrives, it is something being done to
 * a person. The `velatio` is the Roman name for exactly that: a bride was
 * veiled, and from that moment she was a different thing under the law. One
 * person, one rite, one condition that stays.
 *
 * **A frieze, and this is the one beat where a frieze is right.** Three of
 * these recipes rejected a frieze for having no depth to travel through. There
 * is no travel here: nothing is arriving from anywhere, and the picture is a
 * sentence read left to right — the water tested, the bride veiled, the
 * musicians waiting. `velatio.recipe.yaml` has the rest.
 */
function StageVeiling() {
  return (
    <span className="stage-veiling" aria-hidden="true">
      <img className="stage-veiling-art" src={velatioArt} alt=""
           draggable={false} />
    </span>
  )
}

/**
 * Fetch both arcana once, before either is wanted.
 *
 * **A scene that arrives after the card it is behind is not a scene.** The
 * three places are bundled bytes and are on screen the frame they are asked
 * for; these two are files on the server, and the first instant of a match
 * would otherwise draw a Lightning Bolt against nothing and paint The Tower in
 * behind it a moment later, once. `campus.recipe.yaml` records the same worry
 * about hotlinking and answers it by committing; this answers it by asking
 * early, which costs a hundred kilobytes to the browser cache and nothing to
 * the bundle.
 *
 * Once per mount and never again — the images live in the browser's cache
 * after the first ask, and this is the ask.
 */
function useArcana() {
  useEffect(() => {
    for (const src of Object.values(ARCANA)) {
      const warm = new Image()
      warm.src = src
    }
  }, [])
}

function StageArcanum({ scene }: { scene: 'tower' | 'magician' }) {
  return (
    <span className="stage-arcanum" aria-hidden="true">
      <img className="stage-arcanum-art" src={ARCANA[scene]} alt=""
           draggable={false} />
      <span className="stage-arcanum-glow" />
    </span>
  )
}

/** One card on the stage: the light behind it, the card, what is happening to
 *  it, and the plate saying so. */
function StageCard({ item, parting }: { item: Staged; parting?: boolean }) {
  const life = { '--stage-life': `${item.life}ms` } as CSSProperties
  return (
    // **Two classes, because there are two questions.** `is-{manner}` is what
    // happened — it colours the light and times the plate. `has-{scene}` is
    // what the arena opened onto, and every rule about a scene hangs off it:
    // the same valley has to be reachable by a cast creature, an uncast one
    // and a token, which is three manners and one picture.
    <span style={life} aria-hidden="true"
          className={`stage-card is-${item.manner}`
            + (item.scene ? ` has-${item.scene}` : '')
            + (parting ? ' is-parting' : '')}>
      {/* Light gathering behind the card, and only behind it. A rectangle of
          scrim over the whole arena would take the board away from somebody
          scrubbing the timeline; a pool centred on the card separates it from
          the sand and leaves the rows either side perfectly readable. */}
      <span className="stage-veil" aria-hidden="true" />
      {item.scene === 'road' && <StageRoad />}
      {item.scene === 'crypt' && <StageCrypt />}
      {item.scene === 'field' && <StageField />}
      {item.scene === 'forge' && <StageForge />}
      {item.scene === 'temple' && <StageTemple />}
      {item.scene === 'veiling' && <StageVeiling />}
      {(item.scene === 'tower' || item.scene === 'magician')
        && <StageArcanum scene={item.scene} />}
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
 * A fight, in the middle of the arena: one attacker, and the wall facing it.
 *
 * **The arrows this replaced were a line between two cards a person still had
 * to find.** They crossed the trench correctly and said nothing a newcomer
 * could use: on a gang block three of them started from the same creature, and
 * on any board with more than a few creatures a reader was tracing a stroke
 * from one thumbnail to another. Drawn here the fight is the biggest thing on
 * the screen for two seconds, and there is nothing to trace.
 *
 * **The attacker comes out of its own seat's edge**, which is the whole of why
 * this layout was chosen over the three drawn beside it. The board seats the
 * two players across a horizontal seam; a fight drawn on any other axis would
 * make somebody re-learn which way the table faces at exactly the moment they
 * most need to already know. `is-from-far` and `is-from-near` are that, and
 * the stylesheet flips the whole scene on the one class.
 *
 * **Everything is placed as a fraction of the stage, never in pixels.** The
 * arena is 940 wide on a laptop and much less on a phone, and a rank laid out
 * in pixels would be right on exactly one of them — the trap
 * `a-css-constant-is-measured-against-one-screen` records. `boutAt` returns
 * fractions and the stylesheet turns them into percentages.
 *
 * **A card already standing keeps its element.** Every blocker is keyed on its
 * board id, so when the second and third members of a gang arrive on their own
 * beats they mount alone and the cards beside them do not replay their
 * arrivals. That is the wall assembling, and it costs nothing but the key.
 */
function StageBout({ item }: { item: StagedBout }) {
  const n = item.blockers.length
  const life = { '--stage-life': `${item.life}ms` } as CSSProperties
  return (
    <span style={life} aria-hidden="true"
          className={`stage-bout is-from-${item.facing}`
        + (item.outcome ? ` is-${item.outcome}` : '')}>
      {/* The arena, which is a real place and the only scene in this room that
          is drawn behind more than one card. `certamen.recipe.yaml` argues the
          painting, the crop and why every other candidate lost. */}
      {/* **Three arenas, and which one opens says how the fight ended.** The
          declaration gets the arena itself; a fight that settled gets the
          verdict — the ossuary if the creature that swung was cut down, the
          arch if it went through. `certamen.recipe.yaml` and its two
          neighbours argue the pictures and why eleven others lost.

          The arch is a `background-image` rather than an `<img>` for the one
          reason `templum` is: it is an etching, and line on white paper
          arrives grey. `background-blend-mode` needs a background layer to
          blend with, and grey is the one thing this arena is not. */}
      <span className="stage-bout-scene" aria-hidden="true">
        {item.outcome === 'held' ? (
          <span className="stage-bout-art is-triumph" />
        ) : (
          <img className="stage-bout-art"
               src={item.outcome === 'fell' ? ossariumArt : certamenArt}
               alt="" draggable={false} />
        )}
      </span>
      {/* Dust along the line where the two ranks meet — the valley's own
          device, and it is here for the same reason: a place where something
          has just happened is a place that has been disturbed. It is drawn in
          the gap rather than on either card, because the fight is the gap. */}
      <span className="stage-bout-seam" aria-hidden="true" />
      <BoutCard fighter={item.attacker} role="att" />
      {item.blockers.map((b, i) => (
        <BoutCard key={b.id} fighter={b} role="blk"
                  at={boutAt(i, n)} order={i} />
      ))}
      <span className="stage-plate-strip">
        <span className="stage-plate-word">{item.word}</span>
        <span className="stage-plate-title">{item.attacker.name}</span>
        {item.note && (
          <span className="stage-plate-note">{item.note}</span>
        )}
      </span>
    </span>
  )
}

/** One fighter in the bout: the attacker, or one card of the rank.
 *
 *  **A card with no painting is still a card**, and it has to be — a match
 *  played before the pool knew a printing, or a token the dictionary never
 *  carried, still blocks. The frame is drawn either way and the name goes in
 *  it, which is the same fallback `StageFace` makes for the single-card
 *  stage. A gap in the rank would be a lie about how many creatures are in
 *  the fight. */
function BoutCard({ fighter, role, at, order }: {
  fighter: BoutFighter
  role: 'att' | 'blk'
  at?: number
  order?: number
}) {
  const where = {
    ...(at != null ? { '--at': `${at * 100}%` } : {}),
    // Each blocker arrives a beat behind the one before it, so a wall that
    // lands in one beat still reads as several creatures stepping up rather
    // than as one shape appearing. Only ever used by the first render of a
    // card; the ones already standing are never re-mounted.
    ...(order != null ? { '--order': order } : {}),
  } as CSSProperties
  return (
    <span className={`stage-bout-card is-${role}`} style={where}>
      {fighter.image ? (
        <img src={fighter.image} alt="" draggable={false} />
      ) : (
        <span className="stage-bout-blank">{fighter.name}</span>
      )}
      {fighter.count > 1 && (
        <span className="stage-bout-count tabular">{fighter.count}<span
          className="stage-times">×</span></span>
      )}
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
  at, clash, outcome }: {
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
  /** How that fight ended, when this beat is the fight settling rather than
   *  being declared. Picks which arena opens behind it. */
  outcome: Outcome | null
  /** The fight the board is showing, when this beat is a block or a death.
   *
   *  **Settled upstairs, because it cannot be settled here.** A clash is a
   *  fact about *both* seats — the attacker is on one side of the seam and the
   *  wall is on the other — and this component is handed the card dictionary
   *  rather than the folded state. `alignLanes` is decided in the same place
   *  and for the same reason; see `clashOf`. */
  clash: Clash | null
}) {
  // Both arcana asked for once, before the first spell of the match wants
  // one — see `useArcana`.
  useArcana()
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
  // **Which half's type line the plate should read.** A card with two names on
  // one picture has two type lines with it, and they are two different kinds of
  // spell: Locthwain Scorn is a Sorcery printed on an Enchantment, so a plate
  // reading the card's own type line says *"casts Enchantment"* over a sorcery
  // — a confident sentence about the wrong half. `halfNamed` says which half
  // the beat was about; the card's own line is the fallback and is right for
  // every card that has only one.
  const half = face && name ? halfNamed(face, name) : -1
  const kind = (half >= 0 ? face?.face_types?.[half] : undefined) ?? face?.types
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
      word: plateWord(manner, kind, beat.who),
      note: plateNote(manner, beat.target),
      count: times,
      image: face?.image ?? null,
      // **Which half of the card this is, when the card has two on one
      // picture.** `halfNamed` is 0 for the face the card is filed under and 1
      // for the other, and `lib/halves.ts` turns that pair into the glass —
      // null for a card with one half, an Adventure cast as its creature, or a
      // layout nobody has measured. See `Staged.half`.
      half: halfGlassFor(face?.layout, half),
      // **Which scene opens behind the card.** The manner alone decided this
      // until #375, which is why a creature nobody cast got a battlefield and
      // an identical creature somebody paid seven mana for got a glow. See
      // `sceneFor`: a departure is a departure whatever the card was, and an
      // arrival is about what arrived. `kind` rather than the card's own type
      // line, so a split card's half is judged as the half that was cast.
      scene: sceneFor(manner, kind),
      life: stageLife(manner, speed, dies),
    }
    : null
  const { showing, parting } = useStaged(next, beat?.key ?? '', game)
  // Two seats, two slots, two hooks — never one slot chosen between them. Both
  // pools can move on one beat, and a shared slot would make the second seat's
  // mana silently delete the first's.
  const key = `${at}`
  // **The fight rides the beat's own key and not the step's**, which is the
  // opposite of the mana beside it. Mana is keyed on the step because dragging
  // the scrubber through a turn should show the pool move again; a fight is
  // keyed on the beat because a gang arrives as several beats and each of them
  // has to reset the clock, or the third cat would land on a stage that was
  // already fading.
  const liveBout = stagedBout(clash, board, beat?.key ?? '', speed, outcome)
  const heldBout = useStagedBout(liveBout, beat?.key ?? '', game)
  // **Paused, the fight is the beat's own** — the marks' rule, and it is the
  // same argument. Stepping and scrubbing both pause first, and those are
  // exactly where holding a verdict past its beat is wrong: dragging through a
  // combat would leave *"Cut down: Arahbo"* standing over twenty later beats
  // that have nothing to do with it. Playing, the hold is what lets a fight be
  // read at all.
  const bout = speed === 'paused' ? liveBout : heldBout
  const farMana = useStagedMana(
    stagedMana(gained.far, 'far', key, speed), key, game)
  const nearMana = useStagedMana(
    stagedMana(gained.near, 'near', key, speed), key, game)
  if (!showing && !parting && !bout && !farMana && !nearMana) return null
  return (
    <div className="stage" aria-hidden="true">
      {/* **The fight goes under the spell, not over it.** An instant cast in
          the middle of a combat is a thing that happens constantly — a trick,
          a removal spell, a pump — and it is the more urgent of the two: the
          fight is a state the board is already in, and the spell is the thing
          that just happened. Keyed on the attacker so a gang assembling
          re-uses this element rather than rebuilding it; see `StagedBout`. */}
      {bout && <StageBout key={bout.attacker.id} item={bout} />}
      {parting && <StageCard key={parting.key} item={parting} parting />}
      {showing && <StageCard key={showing.key} item={showing} />}
      {farMana && <StageMana key={farMana.key} item={farMana} />}
      {nearMana && <StageMana key={nearMana.key} item={nearMana} />}
    </div>
  )
}
