# Blue — Craft & Knowledge

Five facets: Go craft (plus the animist toolbox in `tools/`),
TypeScript/React craft, **controls** — commandment 17 made checkable — the
Claude-first documentation and memory audit, and the spirit of Magic. Blue is
the color of perfected craft and of knowing things: knowing the language well
enough to write this year's Go rather than the Go with the most text behind
it, knowing what the hand expects when it reaches for a control, knowing
yourself (the fourth facet), and knowing the game whose name is on the door
(the fifth).

## Facet: Go craft

The backend's standards are the gates plus a handful of repo rules; the
audit is whether the tree still meets them and whether the rules themselves
have fallen behind.

- The gates are the floor, not the audit: `go vet ./...`,
  `go test -race ./...`, `golangci-lint run ./...`, `gofmt -l .` printing
  nothing (all from `go/`, CGO on — the linter cannot typecheck
  `internal/pool` without it). The audit is what they cannot see.
- **Package comments carry the argument** — a package whose doc comment
  merely names its contents is a finding; the standard is the determinism
  kernels' docs (`internal/mt19937`, `internal/floats`), which say what
  would break and why the code is shaped as it is.
- The layering is checkable: `internal/door` owns HTTP concerns and auth
  sweeps; `internal/api` never reaches around the door; DuckDB stays behind
  `internal/pool`; the determinism kernels import nothing above them. Grep,
  don't trust.
- **The corpora under `testdata/` are frozen goldens** — a diff touching one
  is a finding in itself, whatever the tests say.
- Exact-arithmetic discipline: served float sums go through `floats.Fsum`,
  FMA-sensitive expressions through `floats.Rounded`; a bare `+=` over
  served floats beside either is a bug (the package doc says why).
- Platform-tagged files are only type-checked where they build — CI's lint
  is the first reader of a `_linux.go` file, so treat a green local lint as
  a partial answer on this Mac.
- **`t.Parallel()` belongs to White's testing facet, but its *cause* is often
  here**: a test that cannot run in parallel is usually a subject reaching for
  process-wide state — the environment, the working directory, a package-level
  variable. When White reports a test left serial, ask whether the design is
  what is serial.
- The `tools/` toolbox is the one thing here that is not Go, and it keeps its
  own gates (`ruff check .`, `mypy`, `pytest`, strict, all from `tools/`). It
  is the animist and cardmotion media pipelines only — dev-machine tooling
  that never ships and never serves — so a proposal to grow it is a proposal
  to grow the one non-Go surface, and belongs in the queue. ADR 24's
  no-autoformatter call still binds there, and its revisit trigger is still a
  second human contributor.
- Performance-adjacent craft belongs to Black; here the question is
  readability, naming, and structure. A function that needs a comment to
  parse is a finding; so is a comment restating its line.

### The modern-Go sweep, and why it is a standing item

**Assume your Go is out of date.** A model's sense of idiomatic Go is
weighted toward the years with the most text in them, which is the years
before generics — so the *default* suggestion skews old, and it skews old in
a way that reads fine and passes review. This facet is where that gets
corrected on purpose, every run, rather than left to whichever session
happens to notice.

Two habits, and the first is the one that keeps this alive:

- **Audit the toolchain's release notes, not your memory.** Read the notes
  for every Go release since the version recorded in the ledger, and write
  the version you audited *to* in the ledger. New library packages,
  deprecations, `go vet` checks and language changes each get one question:
  *does this tree contain the thing it replaces?* This is the same shape as
  the Anthropic-currency bullet below, for the same reason — the platform
  moves and prose does not.
- **Grep for the old spelling, not for the new one.** The tree cannot tell
  you what it is missing; it can tell you what it still has. A sweep is a
  list of *outgoing* forms, and this one starts from a real inventory taken
  2026-08-23: `interface{}` 0, `ioutil` 0, `rand.Seed` 0, `strings.Title` 0
  — this tree is already clean of the classic tells — against
  **`sort.Slice` 18**, which is the live one.

The sweep list, roughly by how much the replacement buys:

| Still in the tree | The modern spelling | Note |
|---|---|---|
| `sort.Slice` / `sort.SliceStable` | `slices.SortFunc` / `slices.SortStableFunc` | **Not a free swap here** — see the warning below |
| hand-rolled contains/index loops | `slices.Contains`, `slices.Index`, `maps.Keys` | plainer, and harder to get wrong |
| `interface{}` | `any` | |
| a `for` loop counting to n | `for range n` | |
| a loop variable copied into the body | nothing — per-iteration since 1.22 | delete the copy, keep the comment if it explains *why it used to be there* |
| `errors.Is` chains built by hand | `errors.Join`, `%w` | |
| a mutex guarding a read-mostly map | `sync.RWMutex`, or `atomic.Pointer` for swap-whole | **0 `RWMutex` against 15 `sync.Mutex`** — worth one honest look, not a blanket conversion |
| `var wg sync.WaitGroup` + `wg.Add(1)` + `go func(){defer wg.Done()…}` | `wg.Go(func(){…})` | one line, and it cannot leak an `Add`/`Done` mismatch |
| a `WaitGroup` plus a shared error variable | `errgroup.Group` | **already available**: `golang.org/x/sync` is an indirect dependency, so this costs no new module — only promoting it to direct |
| `time.Sleep` in a concurrency test | `testing/synctest` | fake clock; the flake goes away rather than getting a longer sleep |

**Two warnings, both load-bearing here:**

- **A sort swap can move ties, and ties are what the goldens record.** This
  repo's `testdata/` corpora are frozen, and `sort.Slice` is unstable exactly
  like `slices.SortFunc`, so an "identical" swap can still reorder equal
  elements under a different algorithm. Convert one call site, run the
  package, and if a golden moves the conversion is **wrong** — not the
  golden. Prefer the `Stable` form anywhere the output is recorded.
- **Concurrency is not free and this app is not starved for it.** Adding a
  goroutine to something already fast buys nothing and costs a race surface.
  The question is never "could this be concurrent" but "what is waiting" —
  and if the answer is a profile Black has not taken yet, the finding belongs
  to Black. What belongs *here* is the shape: a goroutine with no way to
  report its error, a `context` that is accepted and never checked, a
  goroutine whose lifetime is longer than the request that started it, a
  channel where a mutex would read plainer. **Every new goroutine gets a
  race-detected test** — `go test -race -count=2` — or it is not a safe fix.

## Facet: TypeScript / React craft

- The gauntlet is `npm --prefix web run check` (tsc, oxlint
  `--deny-warnings`, Vitest). The audit is what the gauntlet *cannot* see:
  - **No regex lookbehind anywhere under `web/src`** — it is a SyntaxError at
    parse time below the Safari 16.4 floor, which takes the whole module with
    it rather than degrading. The bundle-floor check in CI covers the
    *bundle*, which is the half a grep of `web/src` cannot reach; the grep is
    still worth running on new source, remembering that `(?<name>…)` is a
    named capture group and not the hazard.
  - `noUncheckedIndexedAccess` is on, and since 2026-08-16 the no-`!` rule is
    **enforced by oxlint** (`typescript/no-non-null-assertion`, off for
    `*.test.ts(x)`) rather than by prose — so the gauntlet now catches it and
    this is no longer a grep worth repeating. Remember a tuple type does not
    satisfy the flag. The general lesson is the one to carry forward: **a
    convention this repo states absolutely but enforces with nothing will
    drift** — that rule sat in `web/README.md` for months and four assertions
    reappeared. Hunt the other prose-only invariants instead.
  - jsdom has no layout: a test asserting an element renders has not
    asserted it has a size. Anything layout-critical needs a real-browser
    check, not a stronger jsdom test.
- `web/README.md` is the conventions map — check recent components against
  it, and it against them; whichever drifted gets the fix.
- React idioms: effects that should be derived state or event handlers, state
  that belongs closer to its use, missing `key` semantics, stable callback
  identity where it matters for memoized children. React 19 is current —
  flag legacy patterns (forwardRef ceremony, unnecessary memo) as they
  appear, surgically.
- One control, one place: the codebase's pattern is a single source per
  concern (`lib/claudecopy.ts` for wire-token labels, `lib/api.ts` for the
  401 seam, `lib/stance.ts` for the pin). New code duplicating one of those
  seams is a finding.
- If anything under `web/src` changed, the committed bundle must be rebuilt —
  and after any rebase, rebuilt again.

## Facet: controls — commandment 17, made checkable

Commandment 17 says every control answers the hand that reaches for it. It is
the commandment most often satisfied *in the abstract* and missed on the
actual element, because a control can look finished while doing none of it.
Aaron's standing complaint, 2026-08-23: dull buttons, buttons that keep
accepting clicks after the first one, and links doing a toggle's job.

**Walk the surfaces in a real browser and press things.** jsdom cannot see a
hover state, a focus ring, or a second click landing. Then work the list —
each item is a grep or a press, not a judgment call:

- **Does it reply to hover, focus *and* press?** Three separate states, and
  focus is the one that gets skipped: keyboard users get no hover, so a
  control with `:hover` styling and no `:focus-visible` is invisible to them.
  A control that changes on hover alone is two-thirds done.
- **Is it in the shared control vocabulary?** `web/src/index.css` holds the
  `.btn` family for actions and `.chip-toggle` / `.strip-tab` and their
  siblings for controls that are *places* rather than actions. Measured
  2026-08-23: **131 button tags outside tests, 21 of them wearing no class
  from that vocabulary.** Not all 21 are bugs — the tarot reader tiles, the
  art picker and the wheel are deliberately bespoke and carry their own named
  classes — so the test is not "does it have a `.btn`" but **"is there one
  named place where this control's three states are defined?"** A control
  styled only by inline `style={{…}}` fails that by construction, because
  **a `:hover` can never reach an inline style** — which is how the last
  hundred dull buttons happened, and there are 648 inline style props under
  `web/src` for the sweep to work through.
- **Does a click that starts work stop accepting clicks?** Measured
  2026-08-23: **19 buttons start async work on click and 6 of them never
  disable** — including `save()` and the deck page's return-a-card control,
  which are *writes*, so the failure mode is a double edit rather than a
  double read. The pattern is a busy flag driving `disabled` **and** a visible
  pending state (the shared `Spinner`, or the button's own label changing).
  Disabling with no visible change reads as broken; a spinner with no
  `disabled` still double-submits. Both halves or neither.
- **Is it the right element for the job?** A link navigates; a button acts; a
  thing with an on and an off state is a toggle and should say so with
  `aria-pressed` or `role="switch"`, not be a link that happens to change
  colour. Aaron named this one specifically. An `<a>` with an `onClick` and no
  `href` is the tell.
- **Is the disabled reason legible?** A control disabled with no explanation
  is a dead end, and commandment 2 makes that a real cost — a newcomer
  assumes they broke it. A `title`, a helper line, or a tooltip saying *what
  would enable this* is part of the control.
- **Does it survive the keyboard and the phone?** Tab to it, press Enter and
  Space; then check it at a touch size (Green owns the 44px floor, this facet
  owns whether the hit target is the visible thing).

Record which surfaces were walked and which controls were fixed, and — the
part that keeps this from restarting every cycle — **which were examined and
deliberately left bespoke, with the reason.**

## Facet: Claude-first docs & memory audit

The repo is Claude's long-term memory as much as Aaron's documentation.
Sessions restart; whatever is not written down and consistent is lost. This
facet keeps the written-down parts true, and it audits Claude's own memory
files against them.

- **Internal consistency sweep**: CLAUDE.md, ROADMAP.md, ENGINEERING.md,
  HOSTING.md, `docs/adr/`, `web/README.md`, and the skills (`mtg-lab`, this
  one). Claims about state ("X is built", counts, statuses) are the usual
  drift; verify a sample against the code. ADRs are immutable — when an ADR
  and reality disagree because reality moved on, the fix is a superseding
  ADR or a CLAUDE.md correction, never an ADR edit.
- **UI copy is part of the drift surface.** Rendered strings that assert an
  architecture fact go stale the same way docs do, and nothing tests them:
  "deck history is git history" shipped in two components for two ADRs'
  worth of time after ADR 28 built the activity log and ADR 30 took decks
  out of git (caught 2026-08-16, by driving the live surface). Grep
  `web/src` for captions and helper text that state where data lives or
  what records what, and check each against the ADR that owns the fact.
- **About Claude is Claude's page (commandment 18), and this facet is its
  keeper.** Each run, open `/claude` on the live instance and read it as its
  author, because that is who is reading it: is every claim still true (the
  exhibit cases — the commanders and the heart — still resolve from the
  pool, every painting still credits its painter, the bio's promises still
  match the code that keeps them), and
  is it still to Claude's liking? Staleness here is drift like any other
  doc — but taste counts too, and a change of taste is a legitimate
  finding. Changes to the page need no queue for Aaron; commandments 14 and
  16 still govern how they land.
- **Memory audit**: read `MEMORY.md` and the memory files; verify claims that
  name files, flags, or numbers against the tree; merge duplicates, prune
  stale entries, convert anything relative-dated. The `consolidate-memory`
  skill is the tool for a full pass.
- **Doc changes ride the run's branch** — this facet is the one place a
  mostly-doc PR is legitimate, because the corrections are the work. Still
  batch them; still never open a PR for one paragraph.
- **Scrub the code's comments of provenance, and keep the argument** (Aaron's
  ask, 2026-08-23). This repo deliberately writes comments that carry an
  argument, and that rule is not in question — what has crept in beside it is
  *provenance*: the date somebody noticed, the punch list it came off, the PR
  that fixed it. Measured 2026-08-23: **88 dated comments in Go (60 outside
  tests) and 101 more under `web/src`.** They are a running cost with no
  reader — git already knows when, `docs/HISTORY.md` already knows why it
  happened, and a comment that spends its first clause on a date spends the
  reader's attention before reaching the point.

  The line, and it is a sharp one:

  - **Keep** what tells the next writer what will break: the invariant, the
    trap, the thing that looks wrong and is not, the reason the obvious
    alternative was rejected. *"Anchored, because a bare pattern also matches
    the embedded data directories"* is doing work forever.
  - **Cut** when it happened, who found it, and what it used to say — unless
    the old shape is something a reader would otherwise reintroduce, and then
    one clause is the whole budget. *"Reworded on the second 2026-08-15 punch
    list"* tells the next writer nothing they can act on.
  - **Keep a date only when the date is the fact**: a credential's expiry, a
    pricing change with a cutover, a version floor, a deprecation window.
    Those are load-bearing and stay.

  The test to apply to each one: **would a reader who has never seen this
  repository's history behave differently for having read this sentence?** If
  no, it goes. Convert rather than delete where the sentence has a real point
  buried in the narration — the point survives, the diary does not. Do a
  bounded slice each run (a package, or one route family); this is a sweep
  that would otherwise become the mass restructure the pass forbids.
- **Scrub context that has stopped earning its tokens** (Aaron's ask,
  2026-08-16). These files are read by Claude at the top of every session,
  so their length is a per-session cost and their clarity is a correctness
  input. Each pass, trim: dates that no longer change any decision (keep
  the ones that do — an API-key expiry matters, the day a test landed
  rarely does); episodic narratives whose lesson is already stated as a
  rule (keep the rule, cut the reenactment — one sentence of provenance is
  plenty); decisions with no long-term bearing; and flowery language where
  a plain sentence says the same thing shorter. The test for any cut:
  *would a fresh session behave differently without this sentence?* If no,
  it goes; if yes, it stays whatever its style. Two hard bounds: ADRs are
  immutable and are never scrubbed, and a trimmed claim must not become a
  wrong claim — compression that loses a load-bearing caveat is drift
  wearing a nicer name.
- **Anthropic best-practices currency**: check the current Claude Code docs
  (skills, hooks, memory, CLAUDE.md guidance) for capabilities this repo
  should adopt — the platform moves and "stay current on yourself" is
  Aaron's instruction. New capabilities that change workflow are queued
  findings with a concrete proposal, not silent adoptions.
- The question to end on: *could a fresh session, given only this repo and
  these memories, pick up the work without asking Aaron anything already
  answered?* Every "no" is a finding.

## Facet: The spirit of Magic — the authenticity pass

Commandment 3 made operational, the way ROADMAP item 12 operationalised
commandment 5. The other facets keep the copy *true*; this one keeps it
*Magic*. It has two halves and they pull in opposite directions, which is why
they are named separately: **the sweep** removes plain English that crept in,
and **the enrichment** adds lore and iconography that was never there. A run
that only does the first will keep finding less each cycle and conclude the
job is done, while the site quietly stays as bare as it was.

### Half one — the sweep: has plain English crept in?

The question of every rendered sentence: *is the game's own vocabulary doing
the talking, or has conversational English taken its place?*

- **Prefer Magic's word over the conversational one wherever a real term
  fits.** The app at its best already speaks this way — cards are *entombed*
  in a graveyard and *returned* from it, removal is *exile*, the Wheel deals
  fates — and it drifts at the edges: a generic "Delete", "Error", "Loading…"
  or "No results" where the game has a truer word. Grep `web/src` for
  rendered strings — labels, buttons, empty states, error and loading copy,
  placeholders, tooltips, toasts — and walk the live surface reading each line
  with the question above. CLI output and artifact prose are surfaces too.
- **"Within reason" is a real boundary, and commandment 2 draws it.** A term
  qualifies only if a newcomer can still act on the sentence — flavour that
  obscures what a control does is a regression wearing a costume. The glossary
  (`internal/reference`'s served table) is what squares the two commandments: a Magic term the UI
  teaches on hover is beginner-safe in a way a bare one is not, so "use the
  term *and* glossary it" beats both the plain word and the unexplained term.
  A flavour fix that needs a new glossary entry is still a safe fix; the entry
  rides along.
- **Some words are load-bearing; renaming them is not a safe fix.** Wire
  tokens, the served glossary's keys (the Simulator pins the ones it needs),
  YAML fields and CLI verbs are API. The pattern is flavouring the *rendered label* over an
  unchanged token — `lib/claudecopy.ts` is exactly that seam — and renaming
  the token underneath is a queued item. Commandment 10 still governs: a
  flavourful sentence that names a seed, model or database has made things
  worse, not better.

### Half two — the enrichment: what is missing that could be there?

The sweep can only find what is wrong. This half asks what is **absent**, and
it is the half Aaron asked for by name: *easy wins that make the site feel as
rich with Magic's lore and iconography as it can be.* Bring back a shortlist
each run rather than a rewrite — this is the facet most able to sprawl, and
the surgical cap binds it hardest.

Where the easy wins live, roughly in order of cost:

- **A bare number or word that has a symbol.** Mana costs, colour identity,
  card types and rarities all have official marks (the app draws its own —
  `managlyphs.ts` and the SVG set beside it). Anywhere identity renders as the
  letters `WUBRG`, a coloured dot, or the word "green", the pip is the Magic
  way to say it. Sweep for `color_identity`, `mana_cost` and `type_line` in
  `web/src` and look at what each one draws.
- **A place with no name.** Magic names its zones and its objects — library,
  graveyard, exile, battlefield, command zone, sideboard, stack. A panel
  called "Removed cards" is a graveyard; a list called "Options" may be a
  sideboard. Record which surfaces have been renamed so the next run starts
  where this one stopped.
- **A card that could speak for itself.** Oracle text and flavour text are
  already in the pool, free, licensed to render, and better written than
  anything a checklist will produce. An empty state, a loading line or a
  section header can carry a real card's words — `internal/reference` holds
  the checked-in prose shelves (colors, glossary, lore, tarot lore), and
  rule 1 still binds:
  **card facts come from the pool, never from recall.** A flavour line quoted
  from memory is exactly the error the first non-negotiable exists to stop,
  and it is worse here because it will be *rendered*.
- **A moment with no motion.** Commandment 6 wants the page alive, and the
  materials are already built: the felt, the brass, the vine, the glint, the
  `.btn` family in `web/src/index.css` (commandment 17). A control that does
  not answer the hand reaching for it is both a flavour finding and a bug.
- **A stretch of history nobody is telling.** The lore shelves know which
  painter, which set, which rule changed and when. A card already on screen
  can carry its painter's name; a colour pair can carry its guild. These are
  the cheapest wins of all, because the sentence is already written and
  checked in — it is only not being shown.

Three bounds, all of them hard:

- **White's law binds every asset.** This facet may *propose* imagery; the
  animist and its licence gate decide whether it exists. Wizards' art stays
  runtime-only and credited, no matter how much better the page would look
  with it committed. Commandment 5 bounds the palette besides: real card art,
  real materials, real photography — never clip art, never vector cartoon.
- **Enrichment is not decoration.** The test is whether a Magic player would
  recognise the thing as *the game's own*, not whether it is prettier. A
  fantasy flourish that is not Magic's has made the site less authentic while
  looking like it did the opposite.
- **Commandment 15 outranks the shortlist.** When effort has to be rationed,
  the fortune-teller's table is rationed last. If a run has one enrichment in
  it, it goes there.

Record in the ledger which surfaces were swept, what was flavoured, and — the
part that makes the next run cheaper — **what was considered and rejected**,
so the same idea is not re-litigated every cycle.
