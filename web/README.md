# web/ — the frontend, and the rules it follows

React + Vite + Tailwind, TypeScript `strict` **plus
`noUncheckedIndexedAccess`**. This file is the conventions map for a fresh
session; the argument for each rule lives where the rule is enforced, and
`CLAUDE.md` at the repo root still governs everything here.

## What serves what

The app you see at `mtglab ui` is the **committed bundle** in
`src/mtglab/web_dist/` — a change under `web/src` is invisible until
`npm --prefix web run build`, and CI fails if the committed bundle drifts from
source. For live iteration use the `web-dev` entry in `.claude/launch.json`
(Vite with HMR, proxying `/api` — and *only* `/api`, which is why package-data
assets like the tarot art 404 in dev and only in dev).

`npm --prefix web run check` = typecheck + oxlint + Vitest in one. CI runs
them as separate steps on purpose, so a type error reports as a type error.

## Conventions that are load-bearing

- **Routes are lazy.** Every non-landing screen is a `React.lazy` line in
  `App.tsx`; a new screen wants one, not a top-level import. The entry chunk
  is ~266 kB and stays that way by this rule.
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
- **Glossary keys are pinned from Python.** `Term`/`HelpTip` names must exist
  in `glossary.py`; `SIMULATOR_KEYS` in `tests/test_glossary.py` fails when a
  control on the simulator has no entry, because TypeScript cannot check a
  string against a Python table.
- **Page nameplates are `PageMasthead`** (`components/ui.tsx`): the painting
  whole at its own ratio beside the title, never a cropped band behind it —
  `art_crop` is 1.37:1 and a full-bleed band keeps less than half of it, a
  lesson this project has now learned four times. The component owns the
  page's `h1`. Art is always a Scryfall hotlink chosen by naming a printing,
  with the provenance in a comment at the call site and the credit rendered
  under the title; the non-library pages draw on the Strixhaven Mystical
  Archive cycle (the argument is in `CardSearch.tsx`). Never commit an image
  (rule 5, ADR 6).
- **A committed asset comes from a recipe.** The exception to the rule above
  is CC0/public-domain imagery that must be ours (the ivy under
  `src/assets/ambience/`), and it arrives only through `mtglab animist`
  (ADR 29): a `*.recipe.yaml` beside the assets records source, per-file
  API-confirmed licence, and every transform; the tool writes the
  PROVENANCE.md entry, and the suite verifies the committed files against
  the recipe. Never hand-place a `.webp` — and mind Vite's
  `assets/[name].[ext]` rule: asset basenames must be unique across the app
  or the committed bundle diverges between macOS and CI Linux.
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
