/**
 * One colour combination, with a room of its own.
 *
 * All thirty-two of them — the ten guilds, the five shards, the five clans,
 * the five colours, the five four-colour sets, WUBRG and colourless — and
 * they are pages rather than a panel because the panel had run out of room.
 * Learn's colours tab carried a name, two mana symbols, a creed plate, a
 * tagline, a history, champions, signature cards and a Build control inside
 * one bordered box, and the next thing anybody wanted to add had nowhere to
 * go. So the box became the index and each combination became a place.
 *
 * ## What wears what, and why there are exactly two answers
 *
 * **Twenty of the 32 wear their own heraldry.** Three blocks each printed a
 * faction-emblem cycle and between them they cover every multicolour faction
 * exactly once — Ravnica's ten Signets, Alara's five Obelisks, Tarkir's five
 * Banners. The painting, the artist, the printing and the flavour text are
 * pinned together in the served data so they cannot drift apart, and the
 * credit renders in the same room as the picture (commandment 19).
 *
 * **The other twelve wear their own mana symbols, set large.** Colourless,
 * the five colours, the five four-colour sets and WUBRG are not factions and
 * no such cycle exists for them; inventing a fourth would put something in
 * front of a newcomer that Magic never printed. The symbols are the official
 * ones, through `/api/symbols/{code}.svg` (ADR 33) — the same mark every mana
 * cost in the app draws, so the emblem on White's page is the pip on White's
 * cards.
 *
 * ## The creed and the sigil's caption are not two pull-quotes
 *
 * Both are printed flavour text and putting them near each other as two
 * blockquotes would read as the page saying the same thing twice. They are
 * doing different jobs and the page gives them different ones:
 *
 * - **The creed is a person speaking.** It is set as a carved plate in the
 *   faction's own two colours, with the guild's seal pressed into it and a
 *   speaker's name under it. Jarad said this.
 * - **The sigil's flavour is a museum label.** It captions the painting
 *   directly above it, quiet, in the page's own ink, attributed to the *card*
 *   rather than to a person, beside the artist and the printing it was read
 *   from. Nobody said it; it is what the device means.
 *
 * ## Nothing here fills a field
 *
 * A page renders what its combination has. The four-colour pages and
 * colourless are genuinely short — no faction, no story, no champions, and
 * for Artifice a signature list of two cards because two is the whole set —
 * and they are allowed to be short. `champions` and `signature` are also
 * expected to grow, so nothing is laid out around a count: three signature
 * cards and eight both have to look deliberate, which is why every card list
 * is one wrapping grid rather than a row sized for what is in it today.
 */

import { useEffect, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { Link, Navigate, useParams } from 'react-router-dom'
import {
  api,
  type ColorTaxonomy,
  type Combination,
  type CombinationDetail,
  type Creed,
  type ReferenceCard,
  type Sigil,
} from '../lib/api'
import { COLOR_NAMES, COLOR_VAR } from '../lib/mtg'
import { colorPath, resolveSlug, useColorTaxonomy } from '../lib/colors'
import {
  CardHover, ColorRing, ErrorNote, ManaCost, ManaText, Spinner,
} from '../components/ui'
import { ManaGlyph, OfficialSymbol } from '../components/manasymbol'
import { hasGlyph } from '../lib/managlyphs'
import { SceneBackdrop } from '../components/forest'
import { useAmbience } from '../lib/prefs'

/** The era whose story named a tier. The same three the create flow knows. */
const TIER_ERA: Record<string, string> = {
  guild: 'Ravnica',
  shard: 'Alara',
  wedge: 'Tarkir',
}

/**
 * "a Golgari deck", "an Azorius deck".
 *
 * Eight of the 32 begin with a vowel — Abzan, Aggression, Altruism, Artifice,
 * Azorius, Esper, Izzet, Orzhov — and "Build a Artifice deck" is the kind of
 * sentence that tells a newcomer nobody read this page. Decided on the letter
 * rather than on a list, because the list would be another copy of the
 * taxonomy; none of the 32 is one of English's exceptions (no *a European*,
 * no *an hour*), and a name that became one would want a rule of its own
 * rather than a longer condition here.
 */
function article(name: string): string {
  return /^[aeiou]/i.test(name) ? 'an' : 'a'
}

/* ------------------------------------------------------------- the emblem */

/**
 * How the symbols on an emblem plate are arranged, by how many there are.
 *
 * Rows rather than one wrapping line, because a wrap is decided by the box
 * and a coat of arms is decided by the herald: four symbols should be two
 * over two on every screen there is, not two over two on a phone and four
 * across on a laptop. The numbers are the row lengths; the sizes below are
 * what fits the plate at 375px with those rows.
 */
const EMBLEM_ROWS: Record<number, number[]> = {
  1: [1],
  2: [2],
  3: [3],
  4: [2, 2],
  5: [3, 2],
}

/** Symbol diameter for a plate of `n`. One colour gets the biggest mark on
 *  the page; five share the same box and are drawn smaller so the cluster
 *  stays inside it at a phone's width. */
const EMBLEM_SIZE: Record<number, number> = {
  1: 148, 2: 104, 3: 74, 4: 84, 5: 76,
}

/**
 * The mana symbols of a combination that has no painting, set large.
 *
 * `OfficialSymbol` draws the real thing and falls back to the app's own inked
 * glyph when the symbol cache is cold, which is what it does everywhere else;
 * a plate that showed a broken image would be worse than one showing a hand
 * drawing. The disc behind each symbol is the colour's own wash, so the plate
 * reads as heraldry rather than as a row of icons floating on the surface.
 */
function EmblemPlate({ combo }: { combo: Combination }) {
  const symbols = combo.colors.length ? combo.colors : ['C']
  const rows = EMBLEM_ROWS[symbols.length] ?? [symbols.length]
  const size = EMBLEM_SIZE[symbols.length] ?? 74
  // Each row's start is the sum of the rows before it, computed rather than
  // carried in a running total: a variable reassigned inside a `map` during a
  // render is a variable that survives into the next one.
  const laid = rows.map((count, i) => {
    const start = rows.slice(0, i).reduce((n, c) => n + c, 0)
    return symbols.slice(start, start + count)
  })
  return (
    <span
      className="combo-emblem"
      style={{
        // The plate's own light: the combination's colours across it, at the
        // strength a background can take. Colourless falls back to its wash,
        // which is the one case with nothing to make a gradient out of.
        '--emblem-a': COLOR_VAR[symbols[0] ?? 'C'] ?? 'var(--mtg-c)',
        '--emblem-b': COLOR_VAR[symbols[symbols.length - 1] ?? 'C'] ?? 'var(--mtg-c)',
      } as CSSProperties}
    >
      <span className="combo-emblem-cluster">
        {laid.map((row, i) => (
          <span className="combo-emblem-row" key={i}>
            {row.map((code, j) => (
              <span
                key={code}
                className="combo-emblem-mark"
                // The stagger is per mark and in seconds, so five symbols
                // breathe out of step rather than pulsing as one block.
                style={{
                  width: size, height: size,
                  background: COLOR_VAR[code] ?? 'var(--mtg-c)',
                  animationDelay: `${(i * 3 + j) * 0.55}s`,
                } as CSSProperties}
              >
                {/* The app's own inked glyph behind the official one, which
                    is what every pip and every colour ring already does: a
                    cold symbol cache should show a sun, not a broken square
                    where the page's whole subject ought to be. */}
                <OfficialSymbol
                  symbol={code} size={size}
                  fallback={hasGlyph(code)
                    ? <ManaGlyph symbol={code} size={size * 0.68} />
                    : <span className="sr-only">{COLOR_NAMES[code] ?? code}</span>} />
              </span>
            ))}
          </span>
        ))}
      </span>
    </span>
  )
}

/**
 * The room the twelve without a painting stand in.
 *
 * `SceneBackdrop` washes every mastheaded route in the colours of the
 * painting it is showing, and these twelve have no painting — so the choice
 * was a bare page or a room lit by something. It is lit by the deck's own
 * colours, which is the one honest source available: `--mtg-*` are Magic's
 * fixed semantics rather than a palette somebody picked, and the same five
 * values light the pips, the creed's stone and the drop cap. Nothing here
 * claims to have been sampled from anything.
 *
 * It reuses `.scene-backdrop` and `.scene-backdrop-wash` rather than
 * declaring a second full-viewport layer, so the mask, the two themes'
 * opacities and the z-index are one rule for every room in the app.
 *
 * **Portalled to the body, and that is not a preference.** `App`'s page
 * wrapper animates a `transform`, which makes it the containing block for
 * any `position: fixed` descendant — a backdrop rendered in place would be
 * fixed to the article rather than to the window. `SceneBackdrop`'s own
 * docstring records the same thing.
 */
function ColorRoom({ colors }: { colors: string[] }) {
  const [ambience] = useAmbience()
  if (!ambience) return null
  const lit = colors.length ? colors : ['C']
  // Lobes rather than a linear sweep, because that is the shape `useArtWash`
  // hands the same rule and the mask above is cut for it: light pools in a
  // room, it does not run across one in a band.
  const spots = [
    { x: '18%', y: '26%', r: '58%' },
    { x: '82%', y: '34%', r: '54%' },
    { x: '50%', y: '78%', r: '62%' },
    { x: '8%', y: '70%', r: '46%' },
    { x: '92%', y: '84%', r: '44%' },
  ]
  const wash = spots.map((s, i) => {
    const colour = COLOR_VAR[lit[i % lit.length] ?? 'C'] ?? 'var(--mtg-c)'
    return `radial-gradient(${s.r} ${s.r} at ${s.x} ${s.y}, ${colour} 0%, `
      + 'transparent 100%)'
  }).join(', ')
  return createPortal(
    <div className="scene-backdrop" aria-hidden="true">
      <div className="scene-backdrop-wash"
           style={{ '--scene-wash': wash } as CSSProperties} />
    </div>,
    document.body,
  )
}

/* ------------------------------------------------------------ the nameplate */

/**
 * The head of the page: the emblem, the name, what kind of thing it is, and
 * the one line that says what the deck is for.
 *
 * Not `PageMasthead`, which is the right component for a route that has one
 * painting and one credit. Half of these have no painting at all, and the
 * twenty that do owe their painting a caption as well as a credit — so this
 * is the same silhouette (a plate beside the words, on a card surface) with
 * the two halves it needs. The `h1` lives here.
 *
 * `SceneBackdrop` is called directly for the twenty with art, which is what
 * `PageMasthead` does with its own: the room is washed in the colours of the
 * painting it is showing. The other twelve get no wash, because there is no
 * painting to take one from and a room lit by an invented colour is a room
 * that lies about where it got it.
 */
function Nameplate({ combo, tierLabel }: { combo: Combination; tierLabel?: string }) {
  const sigil = combo.sigil
  return (
    <section className="card-surface overflow-hidden rounded-xl">
      <div className="flex flex-col sm:flex-row">
        <span className="combo-plate">
          {sigil ? (
            <>
              <img src={sigil.art} className="combo-art"
                   alt={`The device of ${combo.name}, painted by ${sigil.artist} `
                      + `for ${sigil.card}.`} />
              {/* Dark mode's lift, laid over the painting rather than mixed
                  into it — the shared layer every card image in this app is
                  brightened by (commandment 19). */}
              <span className="art-lift" aria-hidden="true" />
            </>
          ) : (
            <EmblemPlate combo={combo} />
          )}
        </span>
        <div className="flex min-w-0 flex-1 flex-col justify-center gap-2 px-5 py-5 sm:px-6">
          <div className="flex flex-wrap items-center gap-3">
            <ColorRing colors={combo.colors} size={28} />
            <h1 className="text-3xl font-semibold tracking-tight">{combo.name}</h1>
          </div>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {tierLabel}
            {combo.aliases.length > 0 && ` · also called ${combo.aliases.join(', ')}`}
          </p>
          <p className="text-lg leading-snug" style={{ color: 'var(--text-primary)' }}>
            {combo.tagline}
          </p>
          <div className="mt-1 flex flex-wrap items-center gap-3">
            {/* The one control this page is for, wearing the combination it
                is for. `.btn-sigil` in index.css argues the colours; the two
                ends are this row's own, which is the same expression the
                creed plate and the drop cap are built on. */}
            <Link to={`/new?c=${combo.key}`} className="btn btn-sigil"
                  style={{
                    '--sigil-a': COLOR_VAR[combo.colors[0] ?? 'C'] ?? 'var(--mtg-c)',
                    '--sigil-b': COLOR_VAR[combo.colors[combo.colors.length - 1] ?? 'C']
                      ?? 'var(--mtg-c)',
                  } as CSSProperties}>
              Build {article(combo.name)} {combo.name} deck
            </Link>
            <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
              Colour identity checked against <em>{combo.verified_by}</em>.
            </span>
          </div>
        </div>
      </div>
      {/* Every room is lit; only the source differs. Twenty take the light
          off their own painting, twelve off their own colours. */}
      {sigil
        ? <SceneBackdrop art={sigil.art} />
        : <ColorRoom colors={combo.colors} />}
    </section>
  )
}

/* ---------------------------------------------------------- the museum label */

/**
 * What the device means, captioning the painting it belongs to.
 *
 * Wizards wrote this line about the emblem itself — *"Made of bone and boiled
 * in blood, a Rakdos signet is not considered finished until it has been used
 * as a murder weapon"* — and every one of the twenty happens to be flavoured
 * the same way, which is why the field is worth rendering at all. It is set
 * as a caption and not as a quotation: the same width as the picture's own
 * room, the page's ordinary ink, attributed to the card. The creed below
 * gets the coloured stone, because a creed has somebody saying it.
 *
 * The artist and printing are here as well as on nothing else, and that is
 * the requirement rather than a nicety — a surface that draws card art names
 * the painter in the same room.
 */
function DeviceLabel({ sigil, name }: { sigil: Sigil; name: string }) {
  return (
    // An `aside` and not a `figcaption`: a caption has to be inside the
    // `figure` it captions, and the picture this one belongs to is up in the
    // nameplate. It is the label beside the frame rather than the plaque on
    // it — which is also what it reads as.
    <aside className="combo-label">
      <span className="combo-label-kind">The device of {name}</span>
      <p className="combo-label-words">{sigil.flavor}</p>
      <p className="combo-label-hand">
        <cite>{sigil.card}</cite> — painted by {sigil.artist}, {sigil.printing}
      </p>
    </aside>
  )
}

/* ------------------------------------------------------ the guild's words */

/**
 * A guild's creed, set as an inscription in its own two colours.
 *
 * Moved here from Learn's panel unchanged, because the panel it lived in is
 * now an index and a creed is not an index entry. The line is printed flavour
 * text read off a real card — never written here, never paraphrased — so the
 * plate carries its citation. Six of the ten come off the guild Charm cycle
 * and four do not: Izzet's, Orzhov's and Selesnya's Charms were printed with
 * no flavour text at all in any printing, and Boros's has a line but Aurelia
 * says a better one elsewhere. That is why the card is a field rather than
 * something this component could work out from the guild.
 *
 * **The colours are light, not ink.** `--mtg-*` are washes — pale by design,
 * and pale type on a pale page is the bug a whole route once shipped. So the
 * guild's pair tints the stone and inks the edge (mixed toward
 * `--text-primary`, which darkens it on paper and brightens it at night in
 * one expression), and the words themselves stay the page's own high-contrast
 * ink in both themes.
 */
function GuildCreed({ creed, colors }: { creed: Creed; colors: string[] }) {
  const first = COLOR_VAR[colors[0] ?? 'C'] ?? 'var(--mtg-c)'
  const last = COLOR_VAR[colors[colors.length - 1] ?? 'C'] ?? 'var(--mtg-c)'
  return (
    <figure className="guild-creed"
            style={{ '--creed-a': first, '--creed-b': last } as CSSProperties}>
      {/* The guild's seal, pressed into the stone and running off its edge.
          Decoration: the nameplate above already names these colours out
          loud, and a screen reader that heard them a second time here would
          be hearing furniture. */}
      <span className="guild-creed-seal" aria-hidden="true">
        <ManaCost cost={colors.map((c) => `{${c}}`).join('')} size={82} />
      </span>
      <blockquote className="guild-creed-words">
        {/* Curly quotes as real characters rather than generated content: it
            is a quotation, and a reader listening to the page should hear
            that it is one. */}
        &ldquo;{creed.words}&rdquo;
      </blockquote>
      <figcaption className="guild-creed-hand">
        <span className="guild-creed-speaker">{creed.speaker}</span>
        <span className="guild-creed-source">
          printed on <cite>{creed.card}</cite>, {creed.printing}
        </span>
      </figcaption>
    </figure>
  )
}

/* ------------------------------------------------------------------ a card */

/**
 * A real card, rendered from the pool.
 *
 * `note` is the champion's story role and is the only sentence attached to a
 * card anywhere on this page. The signature list passes none at all, which is
 * deliberate — what that list claims is that the card's identity is exactly
 * this combination, and that is checkable rather than editorial.
 */
function RefCard({ card, note }: { card: ReferenceCard; note?: string }) {
  return (
    <CardHover card={card} className="block">
      <article className="card-surface flex h-full flex-col overflow-hidden rounded-xl text-left">
        {card.art_crop && (
          <img src={card.art_crop} alt="" loading="lazy"
               className="h-20 w-full object-cover" style={{ objectPosition: 'center 30%' }} />
        )}
        <div className="flex flex-1 flex-col gap-1 px-3 py-2.5">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <span className="text-sm font-semibold">{card.name}</span>
            <ManaCost cost={card.mana_cost} size={13} />
          </div>
          <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            {card.type_line}
          </p>
          {note && (
            <p className="mt-0.5 text-xs leading-relaxed"
               style={{ color: 'var(--text-primary)' }}>
              {note}
            </p>
          )}
          {/* `whitespace-pre-line`, like the deck page and the rationale
              editor: oracle text is newline-separated and collapsing it runs
              a keyword line into the ability below it. */}
          {card.oracle_text && (
            <p className="mt-auto whitespace-pre-line pt-1 text-[11px] leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              <ManaText size={11}>{card.oracle_text}</ManaText>
            </p>
          )}
        </div>
      </article>
    </CardHover>
  )
}

/** A heading with the same voice everywhere on the page. */
function Chapter({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-xs uppercase tracking-[0.12em]"
        style={{ color: 'var(--text-muted)' }}>
      {children}
    </h2>
  )
}

/* ----------------------------------------------------- what a colour wants */

/**
 * The colours in this deck, each with what it is after and what it is afraid
 * of.
 *
 * Served by `/api/colors` since long before this page and rendered by nothing
 * until now, which is a waste of the best two sentences in the table. It is
 * also the section that makes the pages people asked for last actually worth
 * visiting: Mono-Red's page has no faction, no story and no champions, and
 * *"Freedom through action… fears being told to wait"* is the whole reason
 * somebody would build one.
 *
 * On a four-colour page it renders the colour that is **missing** as well,
 * because that tier's own blurb says the missing one says more about the deck
 * than the four present ones do — and the absent colour's wants and fears are
 * exactly what it is saying. Nothing is invented to do it: the colour is
 * WUBRG minus the four, and the two sentences are the served ones.
 */
function ColorVoices({ combo, taxonomy }: {
  combo: Combination
  taxonomy: ColorTaxonomy
}) {
  const present = taxonomy.colors.filter((c) => combo.colors.includes(c.code))
  const missing = combo.tier === 'quad'
    ? taxonomy.colors.filter((c) => !combo.colors.includes(c.code))
    : []
  if (!present.length) return null
  return (
    <section className="space-y-3">
      <Chapter>
        {present.length === 1 ? 'What this colour wants' : 'The colours in it'}
      </Chapter>
      {/* Two columns from two voices up, one from one: a mono-colour page has
          exactly one, and a two-column grid holding a single card leaves a
          hole beside the most important paragraph on the page. */}
      <ul className={`grid gap-3${present.length > 1 ? ' sm:grid-cols-2' : ''}`}>
        {present.map((c) => (
          <li key={c.code} className="combo-voice"
              style={{ '--voice': COLOR_VAR[c.code] } as CSSProperties}>
            <div className="flex items-center gap-2">
              <ColorRing colors={[c.code]} size={22} />
              <h3 className="text-sm font-semibold">{c.name}</h3>
            </div>
            <p className="mt-1.5 text-sm leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              {c.wants}
            </p>
            <p className="mt-1.5 text-xs leading-relaxed"
               style={{ color: 'var(--text-muted)' }}>
              <span className="combo-voice-fear">Afraid of</span> {c.fears}
            </p>
          </li>
        ))}
      </ul>
      {missing.map((c) => (
        <div key={c.code} className="combo-voice combo-voice-absent"
             style={{ '--voice': COLOR_VAR[c.code] } as CSSProperties}>
          <div className="flex items-center gap-2">
            <ColorRing colors={[c.code]} size={22} />
            <h3 className="text-sm font-semibold">
              …and the one it refuses: {c.name}
            </h3>
          </div>
          <p className="mt-1.5 text-sm leading-relaxed"
             style={{ color: 'var(--text-secondary)' }}>
            {c.wants}
          </p>
        </div>
      ))}
    </section>
  )
}

/* -------------------------------------------------------------- the page */

function CombinationRoom({ combo, taxonomy }: {
  combo: Combination
  taxonomy: ColorTaxonomy
}) {
  const [detail, setDetail] = useState<CombinationDetail | null>(null)
  const [failed, setFailed] = useState(false)

  // Nothing to clear: the route keys this component on the combination, so
  // arriving at another one builds a new room whose detail starts empty.
  // Clearing in the effect instead painted the previous combination's cards
  // under the new one's name for a frame.
  useEffect(() => {
    let live = true
    api.combination(combo.key)
      .then((d) => { if (live) setDetail(d) })
      .catch(() => { if (live) setFailed(true) })
    return () => { live = false }
  }, [combo.key])

  const era = taxonomy.eras.find((e) => e.name === TIER_ERA[combo.tier])
  const tier = taxonomy.tiers.find((t) => t.key === combo.tier)
  const siblings = taxonomy.combinations.filter(
    (c) => c.tier === combo.tier && c.key !== combo.key)

  return (
    <div className="space-y-6">
      <nav className="flex flex-wrap items-center gap-1.5 text-xs"
           style={{ color: 'var(--text-muted)' }}>
        <Link to="/learn?tab=colors" className="btn btn-xs btn-ghost">
          All thirty-two
        </Link>
        <span aria-hidden="true">›</span>
        <span>{tier?.label}</span>
      </nav>

      <Nameplate combo={combo} tierLabel={tier?.label} />

      {/* The label belongs to the picture above it, so it renders whether or
          not anything else on the page does. */}
      {combo.sigil && <DeviceLabel sigil={combo.sigil} name={combo.name} />}

      {/* The faction's own voice, before ours. Only the ten guilds have one. */}
      {combo.creed && <GuildCreed creed={combo.creed} colors={combo.colors} />}

      {/* The history, illuminated. Aaron asked for this paragraph to be
          "reprinted in some stylized text" and this is that: the site's own
          serif at reading size on a measure that fits it, opening on a
          drop cap the colours of the combination itself. */}
      <section className="combo-history"
               style={{
                 '--hist-a': COLOR_VAR[combo.colors[0] ?? 'C'] ?? 'var(--mtg-c)',
                 '--hist-b': COLOR_VAR[combo.colors[combo.colors.length - 1] ?? 'C']
                   ?? 'var(--mtg-c)',
               } as CSSProperties}>
        <p className="combo-history-words">
          <ManaText size={17}>{combo.history}</ManaText>
        </p>
      </section>

      {tier && (
        <section className="space-y-2">
          {/* The tier's own label as the heading, rather than a sentence built
              round it: the seven read "Guild — two colours", "Four colours",
              "Colourless", and every phrasing that tried to wrap those in
              English was ungrammatical for at least two of them. */}
          <Chapter>{tier.label}</Chapter>
          <p className="max-w-3xl border-l-2 pl-4 text-sm leading-relaxed"
             style={{ borderColor: 'var(--baseline)', color: 'var(--text-secondary)' }}>
            {tier.blurb}
          </p>
        </section>
      )}

      {/* The story beat. Only the twenty slots that are an actual faction have
          one; the other twelve do not render a heading with nothing under it. */}
      {combo.lore && (
        <section className="space-y-2">
          <Chapter>What happened</Chapter>
          <p className="max-w-3xl text-sm leading-relaxed"
             style={{ color: 'var(--text-secondary)' }}>
            {combo.lore}
          </p>
          {era && (
            <p className="max-w-3xl text-xs leading-relaxed"
               style={{ color: 'var(--text-muted)' }}>
              <strong style={{ color: 'var(--text-secondary)' }}>{era.name}</strong>
              {' '}— {era.setting}. {era.story}
            </p>
          )}
        </section>
      )}

      <ColorVoices combo={combo} taxonomy={taxonomy} />

      {failed && (
        <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
          Could not load the cards for this combination.
        </p>
      )}

      {/* Names without a card pool, cards with one. The champion list comes
          from the served taxonomy either way, so a fresh clone still learns
          who Trostani is — it just does not get her card. */}
      {combo.champions.length > 0 && (
        <section className="space-y-2">
          <Chapter>Who they are</Chapter>
          {detail?.pool ? (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {detail.champions.map((c) => (
                <RefCard key={c.name} card={c} note={c.role} />
              ))}
            </div>
          ) : (
            <ul className="space-y-1.5">
              {combo.champions.map((c) => (
                <li key={c.card} className="text-sm"
                    style={{ color: 'var(--text-secondary)' }}>
                  <strong style={{ color: 'var(--text-primary)' }}>{c.card}</strong>
                  {' '}— {c.role}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      <section className="space-y-2">
        <Chapter>Exactly these colours</Chapter>
        <p className="max-w-3xl text-xs leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          Cards whose colour identity is precisely {combo.name} — they can go in
          this deck and in no narrower one.
          {/* Counted over the pool rather than stored, and it is the sharpest
              sentence available about a four-colour slot: two cards, in the
              entire game. */}
          {detail?.exact_total != null && (
            <> The library holds <strong style={{ color: 'var(--text-secondary)' }}>
              {detail.exact_total.toLocaleString()}</strong> of them
              {detail.exact_total <= 5 && ' — the whole set'}.
            </>
          )}
        </p>
        {detail?.pool ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {detail.signature.map((c) => <RefCard key={c.name} card={c} />)}
          </div>
        ) : (
          <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            {combo.signature.join(' · ')}
            <span className="block text-xs" style={{ color: 'var(--text-muted)' }}>
              The cards themselves appear once the library&rsquo;s shelves are
              stocked.
            </span>
          </p>
        )}
        {detail && detail.dropped > 0 && (
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {detail.dropped} named card{detail.dropped === 1 ? '' : 's'} could
            not be found on the shelves and {detail.dropped === 1 ? 'is' : 'are'}
            {' '}not shown.
          </p>
        )}
      </section>

      {siblings.length > 0 && (
        <section className="space-y-2">
          <Chapter>Others like it</Chapter>
          <div className="flex flex-wrap gap-1.5">
            {siblings.map((c) => (
              <Link key={c.key} to={colorPath(c)}
                    className="chip-toggle flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm">
                <ColorRing colors={c.colors} size={14} />
                {c.name}
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

/**
 * The route. Resolves the address before it renders anything, because the
 * address is a name and the names live in the served table.
 *
 * Three answers: the room, a redirect to the room's own address when the
 * segment was one of the other spellings it answers to, and a note for a
 * segment that names nothing. The last one is written here rather than left
 * to the app's catch-all because this page knows what the reader was probably
 * looking for and can hand them the index.
 */
export default function ColorPage() {
  const { slug = '' } = useParams()
  const { taxonomy, failed } = useColorTaxonomy()

  if (failed) {
    return (
      <ErrorNote>
        Could not open the colour guide. The thirty-two are checked-in prose and
        need neither the shelves nor the outside world, so this is the guide
        itself failing rather than missing data — try again in a moment.
      </ErrorNote>
    )
  }
  if (!taxonomy) return <Spinner label="Finding the colour…" />

  const found = resolveSlug(taxonomy.combinations, slug)
  if (!found) {
    return (
      <div className="card-surface rounded-xl px-6 py-10 text-center">
        <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
          There are thirty-two colour combinations in Magic and none of them is
          called &ldquo;{slug}&rdquo;.
        </p>
        <Link to="/learn?tab=colors" className="btn btn-sm btn-quiet mt-4">
          See all thirty-two
        </Link>
      </div>
    )
  }
  // A redirect rather than rendering under the wrong address: `/colors/bg`
  // and `/colors/golgari` are the same room, and only one of them is where it
  // lives. **Replacing** rather than pushing, so the back button leaves the
  // way the reader came in instead of bouncing off the address they typed.
  // It happens here rather than in the router because resolving a name needs
  // the served table, and the router does not have it.
  if (!found.canonical) {
    return <Navigate to={`/colors/${found.slug}`} replace />
  }
  // Keyed on the combination, so crossing from Golgari to Simic builds a new
  // room rather than swapping the contents of this one.
  return <CombinationRoom key={found.combo.key} combo={found.combo} taxonomy={taxonomy} />
}
