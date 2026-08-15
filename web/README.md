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
- **The stance dial is a readout, not a computer.** `lib/stance.ts` pins a
  preset *name* only; the axes shown come from the server's resolved answer,
  never recomputed here, and a refused pin is dropped and the call retried
  bare. See ADR 15 and the dial notes in `ROADMAP.md`.
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
- **Both themes, every time.** Light/dark live in `index.css` on
  `data-theme` with a `prefers-color-scheme` fallback; decorative art needs
  *opposite* treatment per theme (dark mode brightens, light mode dims — the
  reasoning is written next to `.hero-art`). A new visual belongs in
  `index.css` when the two themes need different treatment, and inline
  otherwise.
- **No non-null assertions outside test files.** `noUncheckedIndexedAccess`
  is on and a tuple type does not satisfy it — only a literal index does —
  so walk rotated copies in lockstep rather than indexing `[(i + 1) % 5]`.

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
