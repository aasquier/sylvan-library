# web/ — the frontend, and the rules it follows

React + Vite + Tailwind, TypeScript `strict` **plus
`noUncheckedIndexedAccess`**. This file is the conventions map for a fresh
session; the argument for each rule lives where the rule is enforced, and
`CLAUDE.md` at the repo root still governs everything here.

## What serves what

The app you see at `mtglab ui` is the **committed bundle** in
`web_dist/` — a change under `web/src` is invisible until
`npm --prefix web run build`, and CI fails if the committed bundle drifts from
source. For live iteration use the `web-dev` entry in `.claude/launch.json`
(Vite with HMR, proxying `/api` — and *only* `/api`, which is why package-data
assets like the tarot art 404 in dev and only in dev).

`npm --prefix web run check` = typecheck + oxlint + Vitest in one. CI runs
them as separate steps on purpose, so a type error reports as a type error.

`web/public/` is the one directory Vite copies **verbatim to the bundle root**,
which is where a file has to be for the door to serve it by its own name and
for a home screen to find it: the web app manifest and the four icons live
there and nowhere else. Two consequences worth knowing before adding anything.
Every byte in it is committed twice, here and in `web_dist/`, so it is for
small files that must have a fixed URL and for nothing else. And it is
*published* — an explanatory `PROVENANCE.md` dropped in here would be served
to the world at `/PROVENANCE.md`, which is why this paragraph is here instead.

The icons are the app's own `LibraryMark` (`components/forest.tsx`), whose
geometry is repeated in three places already: the component, the inlined
favicon in `index.html`, and these. They are flat PNGs because iOS accepts
nothing else for a home-screen icon, they were rendered by flattening that
SVG's beziers and filling them at 8x, and re-rendering them is a matter of
following the same geometry — there is no build step to run and nothing in
`tools/` to keep alive for four files that change when the logo does.

## Conventions that are load-bearing

- **The browser floor is Safari 16.4** (declared 2026-08-19), and it is
  enforced by `go/cmd/mtglab/browserfloor_test.go`, whose comment carries the
  argument. Two independent things hold it — Tailwind v4's `@property` and
  `color-mix(in lab, …)`, and the camera door's SIMD wasm core — so lowering it
  means answering both, and the test pins each separately. **It reads the
  committed bundle, not `web/src`**, and that is the lesson worth keeping:
  every feature that ever moved this floor arrived through a **dependency**,
  so a grep of what we wrote could never have caught one and did not.
- **Every animation has to be reachable by a reduced-motion guard**, and
  `go/cmd/mtglab/reducedmotion_test.go` holds that against the same artifact,
  for the same reason: the two that once escaped were Tailwind utilities that
  exist only in the built stylesheet.
- **Routes are lazy.** Every non-landing screen is a `React.lazy` line in
  `App.tsx`; a new screen wants one, not a top-level import. Three are
  deliberately eager — `Library`, `Login`, `Claim`, the screens you arrive on.
  The entry chunk (`web_dist/assets/app.js`) is **285 kB raw / 91 kB
  gzipped**, measured 2026-08-24; it was written here as ~266 kB and nothing
  re-measures it, so treat the figure as a claim to check rather than a
  budget that is enforced.
- **A deck is addressed by `DeckRef`** — `{owner, slug}` as an object, never
  two positional strings (transposed strings are a runtime 404 against
  somebody else's library; named fields are a compile error). `deckUrl` in
  `lib/api.ts` is the single place an in-app deck link is built, and
  `lib/api.test.ts` asserts the URL shape directly.
- **A 401 is handled once**, in `lib/api.ts`, which announces a lost session
  so `App.tsx` re-asks `/api/auth/me`. Screens do not catch it themselves.
- **Every preference about *you* is one gear; the panels keep a readout.**
  `components/settings.tsx` is that gear — theme, ambience, table sound, and
  the Claude stance slider, which is the single stance control (the pin was
  always one global value in `lib/stance.ts`). `components/stance.tsx` is the
  per-surface line that says what it resolved to, and `lib/claudecopy.ts` is
  the only place a wire token (`second-opinion`, `on-request`) becomes a
  label. The readout is still not a computer: the axes shown come from the
  server's resolved answer, never recomputed here, and a refused pin is
  dropped and the call retried bare. A new preference belongs in this panel
  and in `lib/prefs.ts`, not in a second control somewhere else.
  See ADR 15 and the dial notes in `ROADMAP.md`.
- **Only the five colours get a drawn glyph** (`components/manasymbol.tsx`,
  the `hasGlyph` branch). A numeral is a numeral, `{X}` is a letter, a hybrid
  is two colours no single glyph states. A drawn pip carries `role="img"` and
  the colour's name, because a drawing contributes nothing to the
  accessibility tree on its own.
- **A cost is a row of pips, and so is a pool; what a permanent *taps for*
  would be one mark.** `ManaPip` (`components/manasymbol.tsx`) is one mana that
  *exists* — a pool beside a hand, a flash on the sand — and a pool holding a
  green and a white is two things you can spend, so it is two pips. The mark
  for what a printing taps for is the opposite claim: `{G}{W}` means two mana
  and a Temple Garden makes *one*, which is why that was a single hybrid glyph
  and never a pair of pips. It had one caller, the bead on a turned permanent,
  and #341 took the bead off the card — so `ManaProduced` went with it rather
  than staying exported and unused. `producedSymbol`/`producedName` in
  `lib/mtg.ts` still say the same thing in words, which is what the card's
  own `title` carries.
- **The board says mana arrived. It never says what made it.** Forge's mana
  event carries a seat and a pool, the tap event carries a card, and no key
  joins them, so an attribution would be the guess ADR 44 forbids. What the
  Coliseum draws instead is the pool itself, which is per-seat and needs no
  attribution at all: `BoardSide.pool` is where it came to rest,
  `BoardSide.raised` is what there was to spend across the beat, and
  `BoardSide.gained` is what arrived (`lib/board.ts`'s fold, `lib/mana.ts`'s
  arithmetic). `FieldPool` fills and drains beside each hand; `StageMana`
  flashes the arrival on the sand.
- **Forge taps one land and spends it before tapping the next.** Measured on a
  real match: a five-mana turn crosses as `G, '', G, '', G, '', G, '', G, ''`,
  so a pool's *instantaneous* peak is one mana for every spell in the game.
  Anything drawn from that peak strobes. `poolRaised` sums the rises instead,
  which is why the row shows five pips and not one pip five times.
- **Glossary keys are pinned to the served table.** A `Term` or `HelpTip` name
  must exist in `go/internal/reference/data/glossary.json`, which the app
  fetches at runtime (`lib/glossary.ts`, and a missing entry costs a tooltip
  and nothing else — deliberately). The consequence is that a typo'd key fails
  **silently**: TypeScript cannot check a string against a JSON table, and a
  component test cannot either, because the glossary is mocked away there. The
  check that failed when a simulator control had no entry is gone; rebuilding
  it over the Go table is a queued item, so until then a new `Term` gets its
  key confirmed against that file by hand.
- **Page nameplates are `PageMasthead`** (`components/ui.tsx`): the painting
  whole at its own ratio beside the title, never a cropped band behind it —
  `art_crop` is 1.37:1 and a full-bleed band keeps less than half of it, a
  lesson this project has now learned four times. The component owns the
  page's `h1`. Art is always a Scryfall hotlink chosen by naming a printing,
  with the provenance in a comment at the call site and the credit rendered
  under the title; the non-library pages draw on the Strixhaven Mystical
  Archive cycle (the argument is in `CardSearch.tsx`). Never commit an image
  (rule 5, ADR 6).
- **Light goes ON a card, never through it** (commandment 19, ADR 48). No
  `filter` may land on an element that is a Scryfall image or an ancestor of
  one — not `brightness`, not `saturate`, not `blur`, not a `url(#…)`, and not
  through a `@keyframes` the rule merely names. Scryfall's guidelines say
  *"Do not blur, sharpen, desaturate, or color-shift card images"* and *"Do
  not distort, skew, or stretch"*, and fifteen violations of that reached the
  live site under an ADR that already forbade them. Draw the effect as a
  layer instead: an `::after` on the box the card is already in for a shade,
  `.art-lift` for a screened sheet of light, `.art-dimmed` when the card has
  no box of its own. `.field-card-leaf::after` is the reference, and
  index.css's "light lands ON a card" block carries the arithmetic for
  converting an old `brightness(k)` into an alpha.
  `go/cmd/mtglab/cardimagery_test.go` enforces it against the committed
  bundle — **its stylesheet and its script**, so an inline `style={{ filter }}`
  in JSX is caught too, and banned outright wherever it lands. Adding a class
  to that file's `artBearing` with nothing else done fails immediately, which
  is the right way round.
- **A committed asset comes from a recipe.** The exception to the rule above
  is CC0/public-domain imagery that must be ours (the ivy under
  `src/assets/ambience/`), and it arrives only through `animist`
  (ADR 29): a `*.recipe.yaml` beside the assets records source, per-file
  API-confirmed licence, and every transform; the tool writes the
  PROVENANCE.md entry, and the suite verifies the committed files against
  the recipe. Never hand-place a `.webp` — and mind Vite's
  `assets/[name].[ext]` rule: asset basenames must be unique across the app
  or the committed bundle diverges between macOS and CI Linux.
- **Motion assets are the same rule with more formats** (ADR 31). A video
  loop or animated still is committed only through a recipe — procedural
  ones (the mist) declare a seed instead of an upstream and rebuild
  identically from the recipe alone. Play them through `VideoBackdrop`
  (`components/videofx.tsx`) and pick the mode deliberately: `ambience`
  removes the element under reduced motion or the ambience pref ("frozen
  weather is a smudge"); `art` falls back to the still it replaced. Card-art
  motion is never a committed asset at all — it arrives from the runtime
  tier (ADR 32) through `CommanderMotion`, and `ready: false` renders
  yesterday's page.
- **Both themes, every time.** Light/dark live in `index.css` on
  `data-theme` with a `prefers-color-scheme` fallback; decorative art needs
  *opposite* treatment per theme (dark mode brightens, light mode dims — the
  reasoning is written next to `.hero-art`). A new visual belongs in
  `index.css` when the two themes need different treatment, and inline
  otherwise.
- **No non-null assertions outside test files**, and since 2026-08-16 oxlint
  says so rather than this file alone: `typescript/no-non-null-assertion` is
  an error in `.oxlintrc.json`, switched off for `*.test.ts(x)` only. It was
  prose for months and drifted four times — three `map.get(k)!.push(…)`
  group-bys and the root mount — which is the argument for the rule.
  `noUncheckedIndexedAccess` is on and a tuple type does not satisfy it —
  only a literal index does — so walk rotated copies in lockstep rather than
  indexing `[(i + 1) % 5]`, and reach for get-or-create (`const bucket =
  m.get(k) ?? []`) rather than an assertion.
- **The forest layer is drawn, gated, and removable.** `components/forest.tsx`
  and the whisper sprout are inline SVG on `var(--vine)` — no assets, ever —
  and the two themes get different weather (fireflies at night, falling
  leaves by day), display-gated in `index.css` with the same two-selector
  dance as every other theme rule. `prefers-reduced-motion` removes the
  ambience outright rather than freezing it: frozen weather is a smudge.
  Decoration is `aria-hidden`, `pointer-events: none`, and never load-bearing.

## Testing habits with a history behind them

- **Mock `api`, but know what that proves.** A screen mocking `api` passes
  while the real client asks for a route that no longer exists — that is why
  `lib/api.test.ts` pins URL shapes, and why a renderer test must not assert
  the server's own payload text back at itself (a relabelled heading once
  passed exactly that way; mutate the code to check the test).
- **Async UI: wait for the state, not the tick.** The tarot stash test read
  state the instant a name rendered, before the writing effect flushed, and
  was the suite's one flake (#95). `waitFor` the observable, however cheap
  the render looks.
- **Drive the real thing before calling a branch done.** Nine branches in a
  row shipped a bug only the running app showed (`ROADMAP.md` keeps the
  list). A green Vitest run is necessary, never sufficient — open the app,
  click the thing, screenshot it.
