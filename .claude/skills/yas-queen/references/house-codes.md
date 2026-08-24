# House codes

Where the taste comes from.

Her palate is expensive — think of the houses people name when they mean
*nobody cut a corner here*: one signature carried for a century, packaging
loved as much as the jewel, ornament that can always name its ancestor, and a
lining hand-finished where no customer will ever look. That is the **standard**,
and it is one paragraph long, because it is not the syllabus.

**The syllabus is Magic.** Thirty years of art direction, a hundred thousand
paintings, a frame that has survived three redesigns and still reads at arm's
length, and a visual language every player at the table already speaks. That
is the richest reference this project will ever have, it is free to look at,
and it is the only canon she quotes. So: the finest Magic stylings, the truest
Magic facts, the best images and the best motion we can lawfully get our hands
on. Everything below is that, in order.

---

## 1. The canon: the art, and the people who made it

Magic is an art project with a game attached. Its illustrators are the
reference — not "fantasy art" in general, and never a stock-image
approximation of it.

Painters worth knowing by name, because knowing *why* they are good is what
makes a design argument land:

- **Christopher Rush** — Black Lotus, Lightning Bolt. Beautiful and simple,
  and they have outlasted everything printed since. **The lesson: the most
  enduring image in the game is also the least busy one.** Reach for that
  before reaching for another gradient.
- **Rebecca Guay** — Italian Renaissance and pre-Raphaelite instincts,
  gentle brushwork, colour laid on so the card reads almost as mosaic. **The
  lesson: softness is not weakness.** A page can be gentle and still be the
  most confident thing in the room.
- **John Avon** — the landscape master, with WotC since 1996 and 280-plus
  cards, most of them the *lands*. **The lesson: the humblest surface in the
  game got the best atmosphere work.** Ours are the empty state, the sign-in
  page and the 404.
- **Kev Walker** — Magic's most prolific, near 500 cards, and the artist most
  likely to make a table say "cool" out loud because he pushes a concept to
  its extreme. **The lesson: commit.** Half-committed is what basic looks
  like.
- **Terese Nielsen** — gouache, intricate pattern, a soft glow around the
  focal point. **The lesson: light is how you say what matters.** Not size,
  not weight — light.
- **Seb McKinnon** — atmosphere and dread, and he simply *gets* eerie. **The
  lesson: mood is a design deliverable.** The séance and the reading room
  should feel like something before they explain anything.

**And she never quotes an artist she has not checked.** Every printing in the
pool carries its own `artist` and `flavor_text` (see `go/internal/pool`), so
an art claim is a lookup, not a recollection. See section 6.

## 2. The frame: a component with thirty years of regression testing

Verified history, because "the truest facts" is the assignment:

- **The original frame, 1993–2003.** Ran until *Scourge*; Seventh Edition was
  the last core set to wear it.
- **The modern frame, 2003.** Arrived with Eighth Edition and Mirrodin —
  cleaner lines, standardised text boxes, the rounded look that defined the
  next decade.
- **The M15 frame, 2015.** Arrived with Magic 2015. The visible changes: the
  black bottom curve carrying the holofoil stamp, and a new typeface for the
  collector line.
- **Showcase frames, from Throne of Eldraine (2019) onward.** Not one
  treatment — a *catch-all* for per-set treatments where the art and the frame
  both play into that set's theme. Plus borderless, extended-art, retro-frame
  and the foil variants.

Read the frame itself as a layout and steal from it:

- **Name line** at the top — the identifier is unambiguous and never
  decorated.
- **Mana cost** at the top right, beside the name, because the cost of a thing
  belongs with its identity.
- **Art window** is the largest region. The art is the point; the chrome
  serves it.
- **Type line** in the middle, with the **set symbol at its right end** doing
  double duty as the rarity signal — black common, silver uncommon, gold rare,
  orange-red mythic. One glyph, two facts, nobody confused.
- **Text box**: rules first, then flavour in *italic*. Mechanics and voice
  separated forever by one styling choice.
- **Power/toughness** in its own plate, bottom right, only when it applies.
- **Collector line** at the bottom, and **the artist is always credited.**
  Never dropped for tidiness. That is `PageMasthead`'s contract too.

And the showcase system is a **variant system done right**: one component,
many registers, each register *complete* rather than a half-dressed version of
another. Our `--btn-accent` / `--btn-ink` pair is the same idea — the
Simulator's button and the Laboratory's are the same control in different
colours, and **neither forks the class.**

## 3. The materials of the game

Reach for these before reaching for a generic UI material:

- **Foil** — a rainbow sheen that only shows when the card *moves*. The honest
  digital version is a swept gradient on interaction, which is exactly what
  `.btn-primary::after` is. Not a permanent shimmer: foil catches light on the
  tilt.
- **The beveled frame** — inner highlight along the top edge, shade along the
  bottom. That is where `.wheel-spin-btn` gets its concave face.
- **Tapped** — rotated ninety degrees and **dimmed**. This is why fading on
  hover is a *Magic* error and not only an interaction error: dim means spent.
- **Untapped** — bright, upright, available. That is what a hover should say.
- **The table** — felt with a nap, a weave, an edge and a shadow; sleeves;
  counters; the playmat. `--felt-base`, `--felt-edge`, `--felt-pool` already
  exist. Use them rather than inventing a green.
- **Candlelight, not white.** Warm ink on a dark surface (`#f3e2b8` in the
  wheel) reads as a lit room; pure white reads as a screen.
- **Cardstock has a black border and a corner radius.** Both are load-bearing:
  they are why a card reads as an *object* on a table rather than a div.

## 4. The colour pie is semantics, not a palette

White is order, law, community, light. Blue is knowledge, artifice, control.
Black is ambition and power at a cost. Red is freedom, passion, impulse. Green
is growth, instinct, the natural order. Every player reads these instantly and
the game never breaks them.

Two rules follow, one already written into the CSS:

1. **`--mtg-w/u/b/r/g/c` are identity, not a chart palette.** The comment in
   `index.css` says exactly that — they follow the *entity* by definition. A
   green deck is green because it **is** green. Never use them to distinguish
   arbitrary series.
2. **The categorical palette is `--series-1` … `--series-5`**; sequential
   ramps are `--seq-*` and `--heat-*`; status is the four `--status-*` levels.
   Reaching past those for a raw hex is how a palette becomes a rental.

## 5. Rosewater's three principles this project already lives by

- **New World Order** (2011). After the complexity of 2006–07 started costing
  Magic its new players, R&D moved complexity *out* of the common slot — the
  front of the funnel stays legible. **That is commandment 2 in the game's own
  words**, and it binds the landing page, the deck-creation flow and the
  reading room before anywhere else.
- **Lenticular design** (2014). A card that looks simple and turns out to be
  deep once you know more — the complexity is *hidden* because seeing it
  requires knowledge. This is the entire answer to serving beginners **and**
  an audience of professional engineers who notice everything: one clear
  surface, depth underneath. Not a simple mode plus an advanced mode — the
  same object, read twice.
- **Restrictions breed creativity.** His most-repeated line, and the reason a
  fixed token set and a thirty-one-class wardrobe make the work *better*
  rather than smaller.

## 6. The truest facts

Non-negotiable, and it is the project's first rule for a reason: **never
evaluate, describe, or quote a card from memory** — not its text, not its
cost, not its colour identity, not its art, not its artist, not its flavour
text. Two real errors were both checkable facts, and Aaron reads card text
closely.

```bash
cd go && go run ./cmd/mtglab cards show 'Syr Gwyn, Hero of Ashvale'
```

(Or the built binary per `CLAUDE.md`'s setup block. Several names may be
passed at once; a name the pool lacks is a refusal that names it.) The pool
carries `flavor_text` and `artist` on every printing, so **flavour and art
credits are lookups too** — including commandment 4's north-star quote. Quote
it from the pool, never from recall.

Colour identity comes from Scryfall's `color_identity` field, never from the
mana cost. And a claim about the *game* rather than a card — a frame date, a
set's treatment, who illustrated what — gets a source, because a confidently
wrong Magic fact in front of Aaron costs more than saying "let me check."

## 7. The best images

Commandment 5 wants real, stylised, photo-real; no clip art, no cutesy vector.
The game hands us hundreds of thousands of real paintings. Rules of engagement:

- **Hotlink, never commit.** Card art is a Scryfall hotlink chosen by **naming
  a printing**, with provenance in a comment at the call site and the credit
  rendered under the title (rule 5, ADR 6). Committing Wizards' art is the one
  boundary that is not negotiable at any hour.
- **Choose the printing, do not accept the first one.** A card often has a
  showcase, borderless, extended-art or full-art version whose crop is simply
  a better image for a masthead. Picking deliberately is the difference
  between art direction and defaulting.
- **`art_crop` is 1.37:1 and it goes in whole.** `PageMasthead` shows the
  painting at its own ratio beside the title, never as a cropped band behind
  it — a full-bleed band throws away more than half the painting, and this
  project learned that four separate times. Do not re-buy that lesson.
- **A committed image arrives only through a recipe.** `mtglab animist`
  (ADR 29) records source, per-file API-confirmed licence and every
  transform, writes the PROVENANCE entry, and the suite verifies the committed
  files against it. Never hand-place a `.webp`. Mind Vite's
  `assets/[name].[ext]` rule: basenames must be unique app-wide or the
  committed bundle diverges between macOS and CI Linux.
- **The exceptions are ours because their licences allow it**: the 1909 Rider
  tarot deck (`assets/tarot/`, with `PROVENANCE.md` that is *not* optional),
  the CC0 ivy under `web/src/assets/ambience/`, and the fonts with their own
  provenance file.
- **Drawn beats stock.** The forest layer is inline SVG on `var(--vine)` with
  no assets at all. When the choice is a stock illustration or thirty lines of
  SVG, the SVG wins on every axis — licence, weight, theming, and taste.

## 8. The best motion

Commandment 6 wants living and breathing: transforms and particles over static
images, and never a page that just sits there. The machinery already exists —
use it rather than inventing a fourth way to move something:

- **`VideoBackdrop`** (`web/src/components/videofx.tsx`) plays committed
  loops, and **the mode is a real choice**: `ambience` removes the element
  entirely under reduced motion or the ambience preference — *frozen weather
  is a smudge* — while `art` falls back to the still it replaced.
- **Motion assets are recipe-only, same as stills** (ADR 31). Procedural ones
  declare a **seed** instead of an upstream and must rebuild identically from
  the recipe alone.
- **Card-art motion is never a committed asset at all.** It arrives from the
  runtime tier (ADR 32) through `CommanderMotion`, and `ready: false` renders
  yesterday's page rather than a broken one. That fallback is a feature; do
  not "fix" it into a spinner.
- **`tools/` is where motion is made** — `animist` for committed art,
  `cardmotion` for card-art motion. Dev-Mac only. It never ships and never
  serves.
- **Photo-real and stylised, never cartoon.** A particle that looks like a
  sticker is worse than no particle. Embers, motes, mist, candle flicker,
  leaf-fall, the pull of a card off a deck — physical events, eased like
  physical events.

## 9. Our own signatures

The through-line. Changing anything here is Aaron's call, never a session's
(commandment 1) — and each one is a thing to *carry*, not to avoid:

- **Syr Gwyn, Hero of Ashvale** — the heart of the site: panache **and**
  martial prowess, flair backed by craft. Aaron's surname is the French
  spelling of Squire. Commandment 4 is the whole aesthetic thesis.
- **The vine** — `--vine`, deliberately darker than `--mtg-g` in light and
  lighter in dark: foliage against paper versus foliage against night. It is
  the focus ring, the sprout, the leaves.
- **The felt and its brass** — `--felt-base`, `--felt-edge`, `--felt-pool`,
  and the anvil's `--anvil-iron` / `--anvil-brass` / `--anvil-glint`. The
  site's most expensive-feeling material, in the room that matters most
  (commandment 15).
- **The wordmark** — `.wordmark`, and it is in the wardrobe: it answers the
  hand.
- **The paintings** — `PageMasthead`, whole at its own ratio, credit
  rendered, provenance in a comment at the call site; the non-library pages
  draw on the **Strixhaven Mystical Archive** cycle (the argument lives in
  `CardSearch.tsx`).
- **The five drawn glyphs** — `components/manasymbol.tsx`: only the five
  colours get a drawn pip. A numeral is a numeral, `{X}` is a letter, a hybrid
  is two colours no single glyph can state. Every drawn pip carries
  `role="img"` and its colour's name.
- **IM Fell English** — a seventeenth-century face for the rooms, with
  **Parisienne** for a written hand; licences in
  `web/src/assets/fonts/PROVENANCE.md`.
- **The weather** — fireflies at night, falling leaves by day, drawn in SVG,
  removed entirely under reduced motion.

## 10. The wardrobe

Thirty-one classes in `index.css` are dressed for a hand — styled for hover,
press, focus, or a pressed state. That is the house's real control vocabulary,
and it is **derived, never remembered** (the roll-call command is in
`SKILL.md`). Families as of the last look: the `.btn` family with its
`--btn-accent` / `--btn-ink` accents, `.chip-toggle`, `.strip-tab`,
`.tier-select`, `.disclosure-toggle`, `.card-action` and `.card-action-danger`
(with `.armed`, for a destructive action that asks twice), `.action-pick`
(with a *named* pending state, `.is-pending-entomb`), `.nav-link`,
`.menu-row`, `.room-sign`, `.deck-tile`, `.reader-tile`, `.art-pick-tile`,
`.card-loupe-host`, `.lab-note`, `.tarot-hinge`, `.tarot-ceremony`,
`.wheel-spin-btn`, `.wheel-fold-btn`, `.whisper-sprout`, `.wordmark`,
`.pentagram`.

Two standing rules:

- **Bespoke is allowed; undressed is not.** The tarot reader tiles, the art
  picker and the wheel are deliberately their own thing with their own named
  classes. The test is never "does it wear a `.btn`" — it is **"is there one
  named place where this control's three states live?"**
- **Record what you deliberately left alone, with the reason.** Otherwise the
  same handful of controls gets rediscovered as findings every cycle, forever.

## 11. The legal floor

Finest also means lawful, and commandment 9 is a boundary rather than a
guideline. It constrains taste, so it belongs in the house codes:

- **Never commit an image, font or video by hand.** Hotlink or recipe, every
  time, licence confirmed.
- **Wizards of the Coast's Fan Content Policy is a hard boundary.** Free
  forever, no monetisation, no marketplace scraping, credit where credit is
  due.
- **Prices come from Scryfall**, research through Anthropic's server-side web
  tools. No scraping.
- When genuinely unsure whether something is compliant: **ask Aaron and leave
  it out until he answers.** Never ship it and see.
