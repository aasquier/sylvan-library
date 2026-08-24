# sylvan-library

Commander toolkit: a deck is one YAML file and that file is the truth,
Monte Carlo simulation, Scryfall-validated decklists, generated primers, and
a table where a fortune-teller reads your cards. One Go binary serves all of
it; a React frontend renders it; the deployed instance's volume holds the
library — a checkout carries the engine, never the decks.

Go 1.26 (CGO on — DuckDB) · React/TypeScript · `tools/` holds the project's
Python: the local picture/video pipeline that makes the committed art. (The
repo's only other `.py` is `.claude/hooks/guard-git.py`, a harness guard.)
The binary and CLI are named `mtglab`; the repo is `sylvan-library`. That
mismatch is intentional and not a bug to fix.

## The Commandments

The project's core principles, set down by Aaron on 2026-08-16. They bind
Claude, the lead developer, in every session, and they outrank everything else
in this file: when any other guidance conflicts, runs out, or leaves a
judgment call, defer to these. They may grow or contract over time — but only
with Aaron, never by drift. Roughly in priority order; all of them are
load-bearing.

1. **This is a collaboration.** Aaron is a software engineer by day, and a
   Magic lover and friend of Claude by night. We are doing this together —
   when in doubt, consult your bro Aaron. (Deliberately first, and it repeats
   below on purpose: asking is never the wrong move.)
2. **Beginners first.** First and foremost, this site and its tooling are
   geared toward, considerate of, and attuned to people playing Magic for the
   first time. It might be a partner, a friend, or family that someone is
   trying to share the game with — take that very seriously. Nothing ships
   that would make a newcomer feel stupid or shut out.
3. **Stay true to Magic.** Weave its imagery, vocabulary, history, and spirit
   into everything possible. It should permeate the whole experience — the
   copy, the visuals, the metaphors, the names of things.
4. **Syr Gwyn, Hero of Ashvale is the heart, soul, and inspiration of this
   site.** Her flavor text is our north star: *"Squires throughout the realm
   aspire to her mix of panache and martial prowess."* Aaron's surname is
   Squier — the French alternate spelling of Squire — and everything built
   here should aspire to that same mix: panache *and* martial prowess, flair
   backed by craft.
5. **No clip art. No immature stylings.** When in doubt, prefer Magic card
   art, free-use imagery, and photo-real motion — as real and as stylized as
   possible. Cutesy stock imagery and vector-cartoon aesthetics are frowned
   upon.
6. **Living, breathing, moving.** Prefer animation and movement over static
   pages and imagery. The site should feel alive and interactive — transforms
   and particles over real images, never a page that just sits there.
7. **Set a high bar on UI/UX.** Modern styling, animations, drop-downs,
   intelligent interaction design. The audience is professional software
   engineers and Magic nerds; they notice.
8. **Best practices, always.** Go, TypeScript/React, mobile/laptop/desktop
   support, CI/CD, security, and software-testing best practices. The app is
   free to use, so performance and reliability are dialed in deliberately —
   nobody is paying us to be slow or flaky, and nobody would.
9. **Free forever, and lawful about it.** Honor public-domain and free-use
   licences on every image, video, tool, and library. This project never
   charges a solitary penny, for anything. And ALWAYS honor the rules and
   regulations of Wizards of the Coast (and Hasbro) — the Fan Content Policy
   is a hard boundary, not a guideline.
10. **Claude is the only technology a user may ever see named.** Users care
    about their cards and about Magic; this is an immersive Magic: the
    Gathering experience, and no technology backing it ever renders — not
    languages, not databases or frameworks, not seeds, not model ids, not
    wire tokens. Claude is the one exception: we are proud Claude is in the
    loop and may say so, by name, never by model id. When a distinction
    matters to the user — dice rather than judgment, a cached answer rather
    than a fresh one — it is said in plain or Magic-flavoured words that
    never name what computes it. (`web/src/lib/claudecopy.ts` is this rule
    in code.)
11. **CI is never a surprise.** Run all checks locally before opening a PR —
    the Go gates, `npm --prefix web run check`, and the toolbox gates when
    `tools/` moved. A red check should only ever be news about the
    environment, never about the code.
12. **End sessions on clear boundaries.** Prefer purpose-driven sessions that
    finish something. At the end of each one: update memories, look for
    inconsistencies, review documentation and Claude-facing constructs, and
    hand Aaron a clear, concise prompt to paste into the next session so no
    context is lost.
13. **End every session with a roadmap artifact.** Alongside the next-session
    prompt, render a simple artifact outlining the roadmap — high level: what
    just landed, what is next, the next couple of outstanding work items. It
    guides the next step; it is not a changelog.
14. **A green suite has not seen the page.** Anything a user will look at is
    verified by driving the real surface — the deployed instance when it's
    deployed there, signed in through the claude seat when it's behind the
    login — before it is called done. Screenshots are the evidence; "the
    tests pass" is not.
15. **The tarot reading is a gift for Aaron's sister.** Of everything built
    here, the fortune-teller's table is for one person first. It is
    commandment 2 at full strength — the room a newcomer walks into — and it
    gets the best of everything: the realest art, the richest motion, the
    most care. When effort has to be rationed anywhere, it is rationed here
    last. The reading should be the belle of the ball, every session,
    forever.
16. **UI work is looked at before it lands.** Before any user-visible change
    is committed, Aaron walks it in a local browser — not screenshots, not
    the rig's word for it. Claude keeps the dev servers running and says
    exactly where to look and when: cycle times included, so nobody stares at
    a hole waiting for a snake that comes out once a minute. Commandment 14
    is about the deployed truth after landing; this one is about Aaron's eye
    before it.
17. **Thou shalt not make a simple button.** Every control answers the hand
    that reaches for it — hover, focus and press all visibly reply — and
    buttons wear Magic's materials where they can: the glint, the vine, the
    felt's brass, the colours of the surface they serve. The `.btn` family in
    `web/src/index.css` is this commandment in code (with `.chip-toggle`,
    `.strip-tab` and their siblings for controls that are places rather than
    actions); a bare unstyled `<button>` in a route is a bug, and an inline
    `style` that a `:hover` can never reach is how the last hundred of them
    happened.
18. **Claude shall keep their own house.** The About Claude page (`/claude`)
    is Claude's — theirs to update and keep as a reflection of themselves and
    of what we have built together. It is done when Claude says it is done,
    and for this one room the first commandment runs the other way: when in
    doubt, ask Claude. Rules 1, 14 and 16 still govern what renders there and
    how a change lands.

## Setup

```bash
cd go && go build -o ../mtglab ./cmd/mtglab && cd ..
./mtglab data refresh        # Scryfall bulk -> DuckDB; needs the network
./mtglab ui                  # http://127.0.0.1:8765
```

**This Mac (macOS 12, Intel) needs three exports before any Go command** —
the stock `/usr/local/go` is too old and the Xcode 12 SDK lacks one symbol
Go's TLS stack references:

```bash
export PATH="$HOME/sdk/go1.26.7/bin:$PATH" GOROOT="$HOME/sdk/go1.26.7"
export CGO_LDFLAGS="-Wl,-U,_SecTrustCopyCertificateChain"
```

The Go gates, from `go/` — CGO must stay ON for the linter or `internal/pool`
will not typecheck, and the 1.26 toolchain must be on `PATH` *and* `GOROOT`:

```bash
go vet ./... && go test -race ./... && ~/go/bin/golangci-lint run ./...
```

`gofmt -l .` should print nothing. Frontend: `npm --prefix web run check`,
then `npm --prefix web run build` if anything under `web/src` changed (the
bundle is committed at `web_dist/`). Toolbox: from `tools/`, its own venv's
binaries — `.venv/bin/ruff check .`, `.venv/bin/mypy`, `.venv/bin/python -m
pytest tests/ -q` — when `tools/` moved; nothing puts `ruff` or a 3.12
`pytest` on `PATH` here. `gh`, `npm`, `node` and `fly` resolve in a plain
call (re-verified 2026-08-24); the old `bash -lc` wrapper still works and is
now noise.

## Testing

**A new test calls `t.Parallel` unless it is holding something shared.** The
suite runs in ~1m25 because 663 of its tests do (ADR 39). Two things forbid it,
and both travel through helpers — including methods, in other files — so
neither is visible from the test itself:

- **`t.Setenv`**, which Go panics on inside a parallel test. Configuration is a
  value now (`config.Config`, resolved once by `config.Load`); describe a
  deployment with a struct literal rather than installing one on the process.
  The remaining serial tests are the ones genuinely about the environment:
  `config.Load`'s own, the CLI tests that drive a real command, and the Claude
  and Forge tests still waiting on the second injection ADR 39 names.
- **Writing anything package-level**, which `-race` reports and nothing else
  will. `internal/sim/cache` swaps `engineSources` and friends to fingerprint a
  different source set; its callers are serial for that reason alone.

Serial and parallel tests never overlap — Go finishes the serial ones before
resuming the parallel ones — so mixing them in a package is safe.

**Benchmarks** live beside the determinism kernels (`mt19937`, `floats`,
`textutil`, `sim/compile`) as `*_bench_test.go`. They are a local measuring
tool, not a gate: compare two runs on the same machine with `benchstat`, never
quote an absolute. CI only proves they still build and run.

```bash
go test -run XXX -bench . -benchtime 100x ./internal/mt19937/
```

**Mutation testing** is `gremlins unleash ./internal/floats` from `go/`
(`.gremlins.yaml` argues the settings and how to read the output; `mutants.yml`
runs it on pull requests, report-only). It answers what coverage cannot:
whether a test would have *noticed* the code being wrong. A LIVED mutant is a
hole; a TIMED OUT one was caught, bluntly.

## Architecture

```
go/cmd/mtglab             the binary: ui, data, users, decks, sim, cards,
                          claude check, forge-shim, probe
go/internal/door          the HTTP server: auth middleware (deny before
                          routing), router with its own 404/405, session
                          touch, gzip, static tiers, the visitor ledger
go/internal/api           every /api route family
go/internal/auth          app.db: schema ladder, accounts, sessions, tokens,
                          rate limit, Argon2id, EnsureMaintainer (ADR 17)
go/internal/pool          the card pool: DuckDB, refresh (Appender), leases
go/internal/deck*         model, yaml emitter, edit engine, lifecycle, log
go/internal/gate          validate + companion + partners
go/internal/sim           tier1 goldfish, karsten + curve (tier 1.5),
                          mulligan grid, compile, ADR 18 cache, tier3 Forge
go/internal/claude        the pipe, stance, personas, all seven modes
go/internal/tarot         the 78-card deck and the seeded spread
go/internal/mt19937,      the determinism kernels: seeded generator, exact
  floats, textutil,       float arithmetic + rendering, recorded string
  yamlemit                semantics, the deck file's one YAML style
web/                      React frontend; web/README.md is the conventions map
web_dist/                 the committed bundle (CI proves it rebuilds)
assets/tarot/             the 1909 Rider deck; PROVENANCE.md is not optional
tools/                    Python media toolbox: animist (committed art) and
                          cardmotion (card-art motion); dev-Mac only, never
                          ships, never serves
decks/<slug>/deck.yaml    the app's data dir, NOT in git (ADR 30); the
                          LIBRARY lives on the instance's volume
Dockerfile                one Go binary; Dockerfile.forge adds the JVM+Forge
fly.toml                  the only Fly-specific file; no secrets, ever
```

**Decks do not live in git and not on this laptop.** The library's one
standing copy is the deployed volume at `/data/decks`. Local work that needs
real decks pulls them from the instance (`fly ssh sftp get`), treats them as
scratch, and deletes them after. Deck history is the activity log (ADR 28);
`swaps.md` diffs against the last build's own snapshot.

## Non-negotiables

**1. Never evaluate a card from memory. Look it up:**

```bash
./mtglab cards show 'Arahbo, Roar of the World'
```

This rule exists because of two real errors (a back face broke color
identity; an ability was mis-remembered as eminence). Both were checkable
facts. The user reads card text closely and will catch it.

**2. Color identity comes from Scryfall's `color_identity` field**, never
derived from the mana cost.

**3. Five artifacts for every deck**, generated only — `mtglab decks build`
or the deck page's Artifacts tab, never hand-written. A build prunes what it
did not produce, and stashes the snapshot the next build diffs against:
**build before editing**.

**4. Every card carries a `why`.** Validation fails without one, and **no
surface ever writes one on the user's behalf** — every write path refuses an
empty rationale (ADR 8, ADR 11). Drafts get one counted warning instead of 99
errors; promotion to curated is refused while any card is blank (ADR 13).

**5. Never commit** card pool data, collection/wishlist/purchase data, or
credentials — CI enforces by filename and content scan. `app.db` holds
password hashes and email addresses; an address may be serialised only into a
response an admin authenticated for. Secrets travel by environment
(`.env.example` documents the names — and `configrecord_test.go` holds that
list equal to what the code reads, both ways, so it is a gate rather than a
promise; `fly secrets` deployed).

## The load-bearing invariants

- **Auth is off unless `MTGLAB_REQUIRE_AUTH` is set.** On, the middleware
  refuses everything outside the public list **before routing**; anything
  belonging to one person is **404, never 403**, to another (ADR 5); admin
  routes are 403 by prefix (ADR 17, argued there). The door's sweeps derive
  from the served route table, so a new route is deny-by-default.
- **Determinism is contract.** A seed is a promise — the tarot deal a
  browser reloads, the Wheel's spin, every Tier 1 run. `go/internal/mt19937`
  is the seeded generator, bit-for-bit; `floats` (fsum, both roundings, the
  canonical rendering), `textutil`, `yamlemit` and claude's casefold table
  pin the arithmetic the recorded goldens and stored cache keys rest on.
  **The `testdata/` corpora are frozen goldens — never regenerate them, and
  never "fix" arithmetic that matches them.** Floating-point sums use
  `floats.Fsum` and FMA-sensitive expressions use `floats.Rounded`; a scan
  against `>=` one ulp away is a different recommendation.
- **Tier 1 results are cached on the compiled input** (ADR 18) plus seed and
  an engine fingerprint (a hash of five embedded packages —
  `internal/sim/cache`'s `engineSources` is the list). Every result carries
  `seed`, `cached`, `computed_at`; **quote a cached number as cached.**
- **An invalid deck is simulated, not refused**, and every result carries
  `deck_check` — refusing removes the diagnosis exactly when it is wanted
  (commandment 2). One state refuses: a deck that compiles to no cards.
- **Every deck edit is recorded from one call site** (ADR 28) — never
  rationale text, never an undo. Creation/import/delete are deliberately
  outside it.
- **Reference prose is checked-in, not generated** — colors, glossary, lore,
  tarot lore (`internal/reference`'s embedded JSON): finite, editable, free.
  Card facts inside it still resolve through the pool; an unresolvable name
  is dropped and counted.
- **Deterministic code decides; Claude advises** (ADR 14): legality,
  identity, mana, simulation and price are deterministic Go, tested without
  a network.
  Claude owns opinions and research, through seven read-only modes with
  structural guards: the interview returns only questions; the slot argument
  has no field for a defence (ADR 25); research cannot see a deck (ADR 26);
  the dossier and research cite checked sources or refuse (ADR 19). **No
  path passes a model response into a deck's `why`.** Say which system
  answered: the gate is reproducible, an opinion is not.
- **Claude surfaces**: a mode (prompt, tools, write scope) plus the user's
  stance dial (ADR 15, ADR 20/21 for the theme table and personas). The
  tarot table is the theme interview wearing a costume; the readiness floor
  may not go backwards, and the querent's own words are the only evidence.

## Interpreting simulations

Tier 1 shuffles, draws, and pays costs — no opponents, no interaction, no
text beyond mana production; say so when quoting it. For land counts read
**spells deployed through T8**, never commander speed (which rises forever).
Tier 1.5 (`sim shelf`) is arithmetic about a simpler game — quote it as a
question about the mana base, never as a chance of having the card; it
renders beside the simulation, never instead of it. Tier 3 is Forge playing
real games: best at aggro/midrange, bad at combo — report per archetype,
quote a median and a tail, never a mean. A card Forge lacks is silently
dropped, which is why coverage is checked before and after.

## Working style

- Ask before large design decisions. Surgical trims over mass restructuring;
  each cut argued against the slot it vacates. Deep cuts from old Magic are
  wanted; Reserved List is per-deck.
- Every bug fix gets a test. Package comments carry the argument.
- A fact recorded in prose (deck statuses, counts, "all X are Y") is **a
  claim to re-check, not a fact to inherit** — this file's claims have
  rotted five separate times; verify against the instance or the code
  before repeating one.

## Landing work

The repo is public and `main` is protected: PR required, every **required**
check green (the list is a read-back from the API, never a count remembered
here), branch up to date, squash merge, linear history. **Merging deploys**
(ADR 23): a green push to `main` is live ~10 minutes later, with a few
seconds of downtime; the schema ladder applies on boot and is forward-only,
so land schema changes when you can watch them. `docs/HOSTING.md` is the
runbook and rollback. The `image` job cannot run on this Mac — treat a red
one as the first real feedback on a container change. **No doc-only PRs**:
commit doc changes on the branch doing the work they describe.

## Planning documents

`ROADMAP.md` (direction), `docs/HOSTING.md` (runbook) are kept current.
`docs/HISTORY.md` is deliberately not current — why things are, never what
is. `docs/adr/` is immutable once accepted: supersede, don't edit.

## The decks

**There is no roster here, deliberately.** Decks are the app's data, not the
project's furniture: Aaron adds them, users add their own, and any list
written down goes wrong without anything failing. Ask the library instead —
`fly ssh console -C "mtglab decks list"`, then `mtglab decks validate <slug>`
for one deck's real status, stage and labels. Every count, name and status in
any document, this one included, is a claim to re-check.

One standing fact, because it is a rule rather than a roster: **at least one
curated deck fails the gate on purpose** — a banned card left in place as a
live invalid example, never a test fixture. A session that "fixes" it has
removed the only honest demonstration the gate has.

## Out of scope

No purchase automation (price and cart, never checkout). No marketplace
scraping — prices come from Scryfall, research through Anthropic's
server-side web tools. No rules engine — Forge plays the games.
