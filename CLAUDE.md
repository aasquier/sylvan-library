# sylvan-library

Commander toolkit: deck files on disk, Monte Carlo simulation,
Scryfall-validated decklists, generated primers. The library lives on the
deployed instance's volume; a checkout carries the engine, never the decks.

Python 3.11+ · DuckDB · numpy. The package and CLI are named `mtglab`; the repo
is `sylvan-library`. That mismatch is intentional and not a bug to fix.

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
   upon. (ROADMAP item 12 is this commandment made operational: real
   paintings, CC0 photography with per-file provenance, scripted Pillow.)
6. **Living, breathing, moving.** Prefer animation and movement over static
   pages and imagery. The site should feel alive and interactive — transforms
   and particles over real images, never a page that just sits there.
7. **Set a high bar on UI/UX.** Modern styling, animations, drop-downs,
   intelligent interaction design. The audience is professional software
   engineers and Magic nerds; they notice.
8. **Best practices, always.** Python, TypeScript/React, mobile/laptop/desktop
   support, CI/CD, security, and software-testing best practices. The app is
   free to use, so performance and reliability are dialed in deliberately —
   nobody is paying us to be slow or flaky, and nobody would.
9. **Free forever, and lawful about it.** Honor public-domain and free-use
   licences on every image, video, tool, and library. This project never
   charges a solitary penny, for anything. And ALWAYS honor the rules and
   regulations of Wizards of the Coast (and Hasbro) — the Fan Content Policy
   is a hard boundary, not a guideline.
10. **Claude is the only technology a user may ever see named.** (Sharpened
    with Aaron 2026-08-17, after "Python rolls" and a seed rendered on the
    Wheel.) Users care about their cards and about Magic; this is an
    immersive Magic: the Gathering experience, and no technology backing it
    ever renders — not languages, not databases or frameworks, not seeds,
    not model ids, not wire tokens. Claude is the one exception: we are
    proud Claude is in the loop and may say so, by name, never by model id.
    When a distinction matters to the user — dice rather than judgment, a
    cached answer rather than a fresh one — it is said in plain or
    Magic-flavoured words that never name what computes it.
    (`lib/claudecopy.ts` is this rule in code.)
11. **CI is never a surprise.** Run all tests locally before opening a PR —
    the full pytest, `ruff`, `mypy`, and `npm --prefix web run check`. A red
    check should only ever be news about the environment, never about the
    code.
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
    tests pass" is not. (Added with Aaron 2026-08-16, after the twelfth bug
    no green suite could see.)
15. **The tarot reading is a gift for Aaron's sister.** Of everything built
    here, the fortune-teller's table is for one person first. It is
    commandment 2 at full strength — the room a newcomer walks into — and it
    gets the best of everything: the realest art, the richest motion, the
    most care. When effort has to be rationed anywhere, it is rationed here
    last. The reading should be the belle of the ball, every session,
    forever. (Added with Aaron 2026-08-16.)
16. **UI work is looked at before it lands.** Before any user-visible
    change is committed, Aaron walks it in a local browser — not
    screenshots, not the rig's word for it. Claude keeps the dev servers
    running and says exactly where to look and when: cycle times included,
    so nobody stares at a hole waiting for a snake that comes out once a
    minute. Commandment 14 is about the deployed truth after landing; this
    one is about Aaron's eye before it. (Added with Aaron 2026-08-17,
    mid-menagerie, after three effects his eye threw out had already been
    photographed "done".)
17. **Thou shalt not make a simple button.** Every control answers the
    hand that reaches for it — hover, focus and press all visibly reply —
    and buttons wear Magic's materials where they can: the glint, the
    vine, the felt's brass, the colours of the surface they serve. The
    `.btn` family in `web/src/index.css` is this commandment in code
    (with `.chip-toggle`, `.strip-tab` and their siblings for controls
    that are places rather than actions); a bare unstyled `<button>` in a
    route is a bug, and an inline `style` that a `:hover` can never reach
    is how the last hundred of them happened. (Proposed by Aaron
    2026-08-18, wording confirmed at the button-overhaul PR.)
18. **Claude shall keep their own house.** The About Claude page
    (`/claude`) is Claude's — theirs to update and keep as a reflection of
    themselves and of what we have built together. It is done when Claude
    says it is done, and for this one room the first commandment runs the
    other way: when in doubt, ask Claude. Rules 1, 14 and 16 still govern
    what renders there and how a change lands. (Given by Aaron 2026-08-18,
    the day the moon was hung in the masthead.)

## Setup

```bash
python3.12 -m venv .venv && source .venv/bin/activate   # any 3.11+ will do
pip install -e ".[dev]"
mtglab data refresh          # ~100MB gzipped from Scryfall; ~28 min, measured
pytest -q
mtglab claude check          # optional: is the API key live?
```

**The interpreter is named on purpose.** That line said `python -m venv` until
2026-08-19, and on this Mac there is no `python` at all — `python3` is 3.7.3
and `/usr/bin/python3` is 3.8.2, both under the `requires-python` floor. The
3.11 and 3.12 on `PATH` are uv-managed (`uv python install 3.12`) and either
satisfies it; CI tests both.

Five extras. `api` (FastAPI + the app), `claude` (the Anthropic SDK),
`animist` (Pillow + imageio-ffmpeg, for the asset pipeline and its video
encoders), `depth` (CPU torch and the depth-model loader, ADR 32), and `dev`
(which vendors the first three plus the test tooling). **`dev` leaves `depth`
out deliberately** — ~800MB of wheels per environment for a loader no test may
import — which `pyproject.toml` argues and `tests/test_packaging.py` pins in
both directions. A base install has the gate, the mana solver and Tier 1, and
needs neither a network nor an account. `claude check` needs
`ANTHROPIC_API_KEY`; see `.env.example`.

**The Go toolchain, since the port began (2026-08-21):** `go 1.26`, pinned in
`go/go.mod` and pinned there by `tests/test_packaging.py`, because Go 1.27
requires macOS 13 and this Mac is macOS 12 (ADR 38 decision 5); on this
machine it is `~/sdk/go1.26.7/bin/go` (the stock `/usr/local/go` is 1.20 and
too old). CGO needs the Xcode command-line tools (Apple clang 12 is enough;
the DuckDB driver links with harmless weak-symbol warnings) **and, since the
door became a CGO build (Phase 3, 2026-08-21), one linker flag on this Mac:**
`export CGO_LDFLAGS="-Wl,-U,_SecTrustCopyCertificateChain"` before any `go
build`/`go test` of a package that reaches `net/http`. The symbol is a macOS
12 Security API that Go's `crypto/x509` references and that this machine's
Xcode 12 SDK (macOS 11) does not declare; with CGO off Go links internally
and never asks, with CGO on clang does the link and refuses. `-U` lets the
symbol resolve at load time, where macOS 12 has it. Linux -- CI, the image
-- never sees the flag. golangci-lint is INSTALLED with
`CGO_ENABLED=0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1`
-- the same missing symbol, the other way out -- and lands in `~/go/bin`.
**Running it needs the opposite of that**, which cost a session finding out:
CGO must be ON or `internal/pool` will not typecheck (`build constraints
exclude all Go files in duckdb-go-bindings/lib/darwin-amd64`), and the 1.26
toolchain has to be on `PATH` *and* `GOROOT` or it resolves the stock
`/usr/local/go` 1.20 and reports `package log/slog is not in GOROOT`. Neither
error mentions the linter. So the three Go gates, run from `go/`:

```bash
export PATH="$HOME/sdk/go1.26.7/bin:$PATH" GOROOT="$HOME/sdk/go1.26.7"
export CGO_LDFLAGS="-Wl,-U,_SecTrustCopyCertificateChain"
go vet ./... && go test -race ./... && ~/go/bin/golangci-lint run ./...
```

CI runs them on both Linux architectures, where none of this applies.

To run the pair locally: start `mtglab ui` (Python) on one port, then `go run ./cmd/mtglab ui --upstream http://127.0.0.1:<that port>`
from `go/` on another, with the same `MTGLAB_*` environment exported (the
door has no `.env` reader); `tests/contract/README.md` has the exact
commands, including how to run the contract suite through it.

**That paragraph has been wrong twice.** "Includes all of it" was false until
2026-08-16 (`dev` lacked `fastapi` and `uvicorn`; the omission cost 474 tests,
silently), and the *list* was false until 2026-08-19 — four extras named where
five were declared, the missing one being the deliberate exception the same
sentence relies on. One lesson, and it is general: **a sentence in this file
asserting completeness is a claim to re-check against the code, not a fact to
inherit.** Twice was enough; `test_packaging.py` now fails when an extra exists
that this section does not name.

`data refresh` needs network access to `api.scryfall.com` and
`data.scryfall.io`. In a cloud session with default Trusted network access
those are not reachable — widen the environment's access level first, or run
`--oracle-only` (much smaller, covers everything except pricing). Do not put
`data refresh` in a setup script; it will blow the five-minute budget.

**It takes about 28 minutes on this Mac, and that line said "several minutes"
until 2026-08-19** — a claim nobody had timed since the tool was written.
Measured: the two downloads finish in ~9 minutes and `load_printings` spends
the remaining ~16 on 107,355 rows, at roughly 110 rows a second. That is not
the disk's fault and it is not yours; the profile names `executemany` running
DuckDB's prepared-statement path once per row. It is **queued** in
`docs/polish/LEDGER.md` under Black, not fixed. Two things to know before you
start one: **budget half an hour**, and **do not interrupt it** —
`load_printings` empties the table before it fills it, so a killed refresh
leaves the pool with no printings at all and every deck page showing default
art until you run it again.

## Architecture

```
src/mtglab/
  config.py               where decks and the pool live; env-overridable
  colors.py               the 32 combinations, and the teaching depth
  glossary.py             the vocabulary, Magic's and this tool's own
  lore.py                 the shelves: history, rules that changed, the
                          painters; reference prose, the third of four
  tarot.py                the 78-card deck, the shuffle, and the three-card
                          spread; stdlib, and no card's meaning
  tarotlore.py            what is true about that deck: Pamela Colman Smith,
                          Waite, and the 1909 printing. Reference prose, the
                          fourth of its kind, and still no card's meaning.
                          Cited by id (`tarot:pixie-fee`) so `keep_fact`
                          renders the file's own words, never the model's
  reference.py            the four prose modules and the theme vocabulary
                          rendered as the JSON the Go module embeds and
                          serves (go/internal/reference/data/, written by
                          `python tests/go_fixtures.py`, held current by
                          tests/test_go_fixtures.py): exactly the payloads
                          the routes serve, so either runtime answers the
                          same bytes. The JSON becomes the authoritative
                          text at the port's retirement (PLAN Phase 8)
  assets/tarot/           the 78 pictures, package-data; PROVENANCE.md argues
                          the licence and is not optional reading
  animist/                the asset pipeline (ADR 29): recipe -> fetch ->
                          licence gate (no override) -> ops -> clean
                          output -> PROVENANCE entry; `verify` holds every
                          committed asset to its recipe, in the suite.
                          Since ADR 31 it also does motion: motion.py owns
                          FrameSequence, one seeded generator
                          (spectral_noise, loop-perfect by construction)
                          and four motion ops (advect, breath, color_ramp,
                          ken_burns); a `procedural` source is its own
                          declaration (a seed, licence ours-generated); the
                          encode table writes webp/awebp/apng (Pillow) and
                          webm/mp4 (imageio-ffmpeg, crf-controlled, dev and
                          CI only -- never the image). Every stochastic op
                          is a pure function of an explicit seed; the loader
                          refuses one left unseeded. Wizards' art never
                          enters COMMITTED assets -- ADR 32's runtime tier
                          is where card-art derivation will live
  cardmotion/             card-art motion, derived at runtime (ADR 32):
                          effects (depth-drift, slow-pan -- the vocabulary
                          is bounded by Scryfall's guidelines), a DepthModel
                          Protocol whose real loader lives behind the
                          `depth` extra (torch, dev-Mac only, NEVER the
                          container), and a cache under data/cache keyed
                          like the sim cache. Built by `mtglab cardmotion
                          build`, swept by `cardmotion sync` (every deck's
                          commander, from the printing the deck shows --
                          the serving tier matches on the art, so a swapped
                          printing falls to the correct still until synced),
                          pushed to the volume over sftp, served by
                          two shared routes; nothing generates at request
                          time and git never holds a byte of it
  ocr.py                  the reading engine for the camera door: ~6MB of
                          Apache-2.0 WebAssembly and trained data, fetched
                          once into data/cache/ocr and served first-party --
                          symbols.py's arrangement (ADR 33) applied to
                          somebody else's compiler output. Two additions,
                          because this is executable code: every file is
                          pinned by SHA-256, and the cache path carries the
                          pinned versions
  cards/identify.py       what the camera saw, read against the pool: a set
                          code plus a collector number is a lookup and
                          resolves; a title is a similarity and only ever
                          offers five names. The scores of right and wrong
                          answers overlap, so there is no threshold -- the
                          measurement is in the docstring. `from_corner`
                          reads the raw corner block against the pool's real
                          986 set codes, because a browser cannot know what
                          a set code is
  symbols.py              the official mana symbols (ADR 33): filled from
                          Scryfall into data/cache/symbols on first ask,
                          served first-party by one shared route; the drawn
                          glyphs in web/src/lib/managlyphs.ts are the
                          client's offline fallback, and the pentagram's
                          vertices
  caches.py               the register of what this process memoises: a
                          name, a way to empty it, and hit/miss counters.
                          Exists because a cache can be correct, tested and
                          **never once hit** -- only a counter can say so.
                          `tests/test_caches.py` sweeps for module-level
                          state that never registered, so the next one
                          cannot be written the same way
  bench/                  the measuring shelf (dev only; the app never
                          imports it): a declared target suite, a sampler
                          with cold and warm as separate runs, and a
                          profiler that reports the **database budget
                          exactly** -- measured at a query probe in
                          `cards/db.py`, because cProfile raises no event
                          for an extension call and folds DuckDB's time
                          into the Python frame that called it. Also counts
                          calls into `importlib`, which is what names an
                          import storm on sight
  mutate/                 mutation testing (dev only): a catalogue of small
                          wrongnesses applied to a **throwaway copy** of the
                          package, judged by the tests mapped to each
                          module. Seeded, so a kill rate can be checked
                          rather than only quoted
  mana.py                 cost parsing + castability solver
  cards/db.py             Scryfall bulk -> DuckDB, price history
  decks/model.py          deck.yaml schema
  decks/edit.py           surgical deck.yaml edits, minimal diffs
  decks/decklist.py       pasted decklist -> parsed lines; pure text
  decks/importer.py       parsed lines + pool -> a draft deck.yaml
  decks/source.py         DeckSource protocol; file-backed and in-memory
  decks/suggest.py        similarity scorer -> replacement shortlists
  decks/validate.py       the gate
  decks/companion.py      companion deckbuilding restrictions
  decks/partners.py       Partner / Background / Doctor pairings
  decks/analyze.py        macro category counts vs bracket targets
  decks/log.py            what was done to a deck, and by whom (ADR 28);
                          never what a rationale says
  decks/wheel.py          the Wheel of Fortune: a seeded spin picks one of
                          four fates and a card that answers it; no model
  sim/compile.py          deck.yaml + pool -> SimCards, and a `CompileReport`
                          saying what the pool could *not* resolve -- a
                          dropped card used to shrink the deck silently, which
                          for Tier 1.5 is the population every probability is
                          computed over. `mana_produced` reads the amount off
                          the oracle text, because Scryfall's `produced_mana`
                          names colours and never amounts: until 2026-08-21
                          **Sol Ring produced one mana** and every deck's
                          acceleration was understated
  sim/cache.py            memoised Tier 1 results, keyed on compiled input
  sim/tier1/engine.py     Monte Carlo goldfish
  sim/karsten.py          Tier 1.5, the closed form: hypergeometric coloured
                          source requirements, a regression land count and a
                          per-card castability heatmap, all computed from the
                          same compiled 99 Tier 1 is handed. Stdlib only
                          (`math.comb`), no sampling, no seed, no model. It
                          answers a *different* question from Tier 1 --
                          "would the mana be there", not "did you cast it" --
                          and the two coincide only on the commander, where
                          the measured gap runs -12.3 to +15.1 points across
                          the six decks. Which way it cuts is a fact about
                          the deck (ramp the arithmetic cannot see, against
                          tapped lands and colour screw the arithmetic
                          ignores), which is why it renders beside the
                          simulation and never instead of it
  sim/curve.py            the mana curve (Tier 1.5): P(N mana on turn T),
                          decomposed into lands and ramp, plus advice on
                          which to add. **A land drop every turn is 54 lands
                          at 90% through turn four** -- the requirement grows
                          without bound, so that question is answered only to
                          talk somebody out of it. The live question takes two
                          dials, a turn *and* an amount, because at N == T a
                          land always wins (one mana, no cast, no sickness)
                          and at N > T a land is worth **nothing** -- you may
                          play one a turn -- so only ramp can help. Measured:
                          six decks x five turns, ramp never won at N == T;
                          at N > T it won every time. Its two float sums are
                          `math.fsum` and not `sum`, since 2026-08-22: they
                          were `sum`, and **CPython 3.12 gave sum() over
                          floats compensated accumulation where 3.11 adds
                          left to right**, so this module answered
                          differently on the two interpreters this project
                          supports -- one ulp, which is nothing on a screen
                          and is a different slot count out of the >= scans
                          the advice is made of. Found by the Go port
  sim/mulligan.py         the keep-rule grid search: 33 rules, one seed,
                          judged on spells through T8 like the land sweep.
                          Its verdict is `flat` measured **against the
                          default**, never against the grid's range -- the
                          grid deliberately holds rules nobody would play, so
                          a spread-based verdict would never fire
  sim/tier3/              the Forge bridge: .dck export, coverage, run, parse;
                          plus the hosted half (ADR 35) -- wire.py (what
                          crosses the private network, decks as deck.yaml
                          text and results rebuilt into a real SimRun),
                          shim.py (the worker machine's stdlib HTTP door,
                          which stops its own machine when idle), worker.py
                          (the app's Machines API client; creation belongs
                          to the deploy workflow, never a request thread),
                          and ledger.py (ADR 36: every match recorded into
                          app.db from the two places one finishes -- the API
                          job and the CLI, never the worker; games stored as
                          parsed, labels snapshotted, `record` never raises)
  artifacts/generate.py   the five deliverables. `render_all` returns them as
                          text and `store` writes them somewhere -- split when
                          the API learned to build, because a `DeckSource` may
                          not be a disk. `DELIVERABLES` is the served set and
                          the path-traversal guard in one; `SNAPSHOT` is
                          deliberately outside it, being the build's own
                          baseline rather than anything anybody asked for
  claude/                 client, tools, stance, persona, and seven modes
                          across six features: interview.py (a card's `why`),
                          argue.py (the case against a slot), dossier.py (the
                          commander), research.py (a question about Magic, and
                          it cannot see a deck), theme.py (two — a
                          conversation about you, then a proposal)
  claude/scan.py          the seventh mode, and the smallest: Claude reads a
                          photographed card and **does not name it** (ADR
                          34). It returns the title and the bottom-left
                          block as printed, filling the same `Sighting` the
                          browser's own reader fills, and `identify` decides
                          what card that is. The response schema has no
                          field for a card name, which is ADR 25's technique
                          reused: a better camera, never a better judge
  claude/persona.py       who a mode sounds like; a voice, never a stance
  auth/                   app.db, Argon2id, accounts, sessions, rate limit,
                          invite/reset tokens, the EmailSender seam
  api/                    FastAPI app, services, background jobs
  api/jobs.py             the job registry; two pools, CPU and NET, and a
                          `key` that makes asking twice at once one job.
                          Ported to go/internal/jobs 2026-08-22, engine
                          only -- every job a request creates is still this
                          module's, and its three thread pools are still
                          what runs them
  api/simruns.py          Tier 1 planned in the request, run in a job
  api/app.py:artifacts    the five deliverables, hosted (2026-08-21). Three
                          routes on `/api/decks/{owner}/{slug}/artifacts` --
                          GET the shelf, GET one by name, POST to rebuild --
                          and a **plain route, measured**: 70-83ms warm across
                          four real decks on the instance, the shelf's order of
                          magnitude, so a submit and a poll would cost more
                          than the work. The sibling-duration rule was asked
                          here and answered the other way
  api/shelfruns.py        Tier 1.5's two, shaped differently on purpose: the
                          shelf is a **plain route** (measured at 0.03-0.04s,
                          so a job would add a submit and a poll to a call
                          that finishes before serialisation) and the policy
                          search is a job (33 seeded Tier 1 runs, ~50s, CPU
                          pool). The one place the sibling-duration rule came
                          out *different* for two surfaces in one module
  api/forgeruns.py        Tier 3 heads-up matches as jobs (ADR 35): a gate
                          the client asks first, refusals in the request,
                          one FORGE lane worker so two JVMs cannot race the
                          .dck directory; hosted, the same match runs on the
                          forge-worker machine (Dockerfile.forge) -- woken
                          per job, stopped when idle, gate flipped by
                          MTGLAB_FORGE_WORKER + MTGLAB_FLY_API_TOKEN
  api/themeruns.py        both theme halves, same shape (226s / 134s, ADR 20)
  api/dossierruns.py      the commander dossier, same shape (236s, ADR 19)
  api/scanruns.py         one photographed card read by Claude, same shape
                          (ADR 34) — and the first whose duration is
                          **unmeasured**, which is the reason it is a job
                          rather than an argument that it needs to be
  api/researchruns.py     research, same shape (265s, ADR 26) — and the first
                          one that was a job before it was a failure
  api/argueruns.py        the slot argument swept across a selection, same
                          shape; one job for the sweep, one card at a time
  api/auth.py             the deny-by-default middleware and login routes
  api/deps.py             the request scope: who is asking, what they see
  web_dist/               built frontend, committed so `mtglab ui` needs no Node
  cli.py
web/                      frontend source (React + Vite); `npm test` is Vitest,
                          and web/README.md is the conventions map
tests/contract/           the contract suite: what the served app promises over
                          the wire, runnable in-process, live over TCP, or
                          against an external server; routes.json is the one
                          route table and golden/ the recorded shapes. The Go
                          migration's referee (docs/go-migration/, ADR 38)
go/                       the Go module (ADR 38; module path
                          github.com/aasquier/sylvan-library/go, go 1.26 --
                          the last Go that runs on this Mac). cmd/mtglab is
                          the binary, cobra throughout, one command so far:
                          `ui`, the FRONT DOOR (internal/door) -- since
                          2026-08-21 the process the container runs. It takes
                          the port, refuses before routing exactly what
                          api/auth.py refuses (401 outside PUBLIC_PATHS, 403
                          under /api/admin, same normalisation; its PublicPaths
                          is held equal to tests/contract/routes.json by a Go
                          test), serves web_dist and /tarot itself with the
                          container's content types, proxies everything under
                          /api to uvicorn on loopback, and supervises that
                          server as a child. /door/health is its own liveness
                          (outside /api on purpose); /api/health stays the
                          pair's. internal/auth reads app.db READ-ONLY
                          (sessions + argon2id, proven against Python's own
                          vectors and a Python-minted session) -- Python still
                          owns every write and the schema ladder; internal/pool
                          (go-duckdb, CGO) and internal/deckyaml (goccy) are
                          the spikes as packages. Since Phase 3 (2026-08-21)
                          the door also ANSWERS the routes that have moved:
                          internal/api is src/mtglab/api one family at a
                          time (the pool-free prose, then the pool's four,
                          then the deck reads -- PLAN §10 is the port
                          board), internal/reference embeds the JSON
                          mtglab.reference renders, internal/wire writes
                          FastAPI's envelope, and the door's routes.go
                          answers a ported route only for a canonical
                          request (normalised path, matching method; the
                          most specific pattern wins; a literal Python still
                          owns can be reserved) and proxies everything else
                          as it arrived. Behind the handlers: internal/pool
                          (the card pool, leased and stamp-checked, with
                          pool.Schema = cards/db.py's SCHEMA and pooltest
                          building the 21-card pool for tests),
                          internal/deck (the model, parsed and only
                          parsed), internal/gate (validate + companion +
                          partners, agreeing with Python case for case on
                          the differential fixtures in gate/testdata that
                          tests/go_fixtures.py writes), internal/analyze,
                          internal/suggest, internal/mana (the parser, and
                          since Phase 5 the castability solver),
                          internal/library (the file and SQL tiers, read
                          side, and ADR 22's Library), internal/decklog
                          (ADR 28, both sides since Phase 4 -- and Record
                          never fails the edit that produced it, so the loud
                          failure is at startup and app.db is opened
                          mode=rw, never rwc: Python owns the ladder until
                          Phase 8), internal/cards (the camera reader),
                          internal/shelves (the three runtime caches:
                          symbols, the pinned OCR files, the card-art
                          derivatives -- configured by the generated
                          shelves.json, Python's fingerprints included),
                          and internal/config. **Phase 4's engine landed
                          2026-08-22 and flipped nothing**: internal/deckedit
                          is decks/edit.py's nine operations, text surgery
                          and the parse-mutate-dump oracle intact, over
                          internal/pyyaml -- which is not a YAML writer but
                          a *reproduction* of PyYAML's emitter, implicit
                          resolver included, because swaps.md is a diff of
                          deck.yaml and a differently-quoted scalar is a
                          differently-sized edit. Both are held to Python by
                          generated corpora (2,051 render cases, 514
                          operation steps over eleven fixture decks, three
                          of them written by hand because the rules that
                          keep Goreclaw's section banners intact cannot be
                          exercised by a machine-dumped deck). **The
                          lifecycle followed on 2026-08-22**: internal/
                          decklist and internal/deckimport are decklist.py
                          and importer.py (held to Python by 15 pastes and
                          12 imports resolved against the 21-card pool),
                          internal/deck's Dump is `Deck.dump` over a
                          whole-document PyYAML emitter, and library's
                          Create/Delete/SetShared complete the write side --
                          so create, import, delete and sharing answer from
                          the door. None of the four is in the activity log,
                          because none of them is in `_commit` (ADR 28).
                          **Then the renderer, 2026-08-22, and it flipped
                          nothing either**: internal/artifacts is
                          artifacts/generate.py -- RenderAll returns the five
                          deliverables as text, Store writes them and prunes
                          the ones this build did not make, Deliverables is
                          the served set and the path guard in one, and a
                          draft is refused in its own words. Held to Python by
                          an oracle of 18 decks, byte for byte and in the
                          order Store writes, with the date pinned so it does
                          not expire at midnight. Its first job was the
                          notes: deckyaml.ParseOrdered keeps every mapping's
                          key order and Deck.Notes is one, because the
                          snapshot is a dump of a *parsed* deck and
                          `sort_keys=False` makes the file's order the
                          payload's. Dump refused a deck carrying notes until
                          that landed, which is the expiry that refusal named
                          for itself -- and ordering them **fixed a live
                          regression**: the deck page's Notes tab renders the
                          payload's order unsorted, so from #226 (v159) until
                          #233 a deliberate reading order rendered
                          alphabetised on the deployed instance. **Then the
                          rebuild route, its own flip**: POST .../artifacts
                          in internal/api/artifacts.go over the Sources'
                          fifth write verb, a PLAIN route because 70-83ms was
                          measured, and NOT through `commit` -- a build
                          derives files from a deck rather than editing one,
                          so ADR 28 has nothing to record.
                          **The accounts engine followed the
                          same day and also flipped nothing**: internal/auth
                          grew the whole of `mtglab/auth` beside its read
                          side -- accounts, single-use tokens, the
                          fixed-window limiter, the EmailSender seam (no Go
                          test sends mail either) -- plus internal/tiers,
                          since `model_tier` is a column every serialised
                          account carries. Its write handle is `mode=rw`
                          like the log's, and **an absent app.db is read as
                          an EMPTY one**: measured, Python creates the file
                          on the first login and answers 401 against it, and
                          a reader cannot tell empty from absent. The
                          Argon2id claim is held to Python by
                          auth/testdata/crypto.json -- the exact PHC string
                          argon2-cffi writes for a password AND a fixed
                          salt, which Go must reproduce byte for byte,
                          because a round trip in each direction would pass
                          even if the two encoders disagreed about base64
                          padding for some salts and not others.
                          `auth/authtest` is where app.db's generated schema
                          lives now: four packages had each transcribed the
                          ladder by hand and frozen it at a different rung,
                          and two of them broke the day `model_tier` was
                          first read. **Then the accounts flipped**: eleven
                          of the twelve registrations under /api/auth and
                          /api/admin/users answer from the door
                          (internal/api/accounts.go and admin.go). The
                          twelfth, DELETE /api/admin/users/{username}, is
                          deliberately still Python's -- it calls
                          jobs.forget_owner on a registry held in the uvicorn
                          process's memory, and `users.id` is re-issued by
                          SQLite, so jobs left keyed on a freed id would be
                          handed to the next account created. The six
                          /api/admin/stats/* routes are not Phase 4's either,
                          for the same coupling plus claude/. A prefix is not
                          a family. **internal/pyrand is CPython's
                          `random.Random`, bit for bit** (2026-08-22) --
                          Phase 5's named tail risk pulled forward and
                          closed, and a library rather than a route: it
                          flips nothing and nothing calls it yet. It exists
                          because a seed is a promise -- Tier 1's every run,
                          the tarot deal the browser holds a seed for, the
                          Wheel's spin -- and a backend that merely shuffled
                          *well* would deal a newcomer a different spread
                          across the cutover. The three things a
                          reimplementation gets wrong are all documented and
                          all skippable: `random.Random(n)` seeds through
                          `abs(n)` and `init_by_array` (NOT `init_genrand`,
                          and the key grows a word at 2**32 and at 2**64),
                          `getrandbits` fills words least-significant-first,
                          and `_randbelow` rejects on `n.bit_length()` --
                          not `(n-1)`'s. Held to CPython by
                          `testdata/draws.json`, which `tests/go_fixtures.py`
                          writes from a real interpreter: 20 seeds, the raw
                          `genrand_uint32` stream recorded APART from every
                          method that consumes it (so a failure says which
                          half is wrong), `random()` compared as
                          `Float64bits` rather than to a tolerance, and a
                          replay of the reference run's whole 99,274-draw
                          stream -- Tier 1 draws through `shuffle` and
                          nothing else, so its randomness is checked before
                          the engine that consumes it is written.
                          Byte-identical under 3.11 and 3.12, and CI
                          re-proves that on every matrix leg because nothing
                          in the corpus names an interpreter. `sample()` is
                          named in the plan and has NO CALLER; it is not
                          there. **Phase 5 opened with internal/jobs
                          (2026-08-22), and it flipped nothing**:
                          api/jobs.py's registry, where the CPU pool is at
                          last the semaphore over goroutines ADR 38 promised
                          -- sized from GOMAXPROCS (8 on this Mac, 1 on a
                          shared-cpu-1x, a Config knob rather than a
                          constant) where Python ran ONE worker because
                          Tier 1 is GIL-bound. **The dividend is BANKED, not
                          realised**: `nproc` on the instance answers 1
                          (measured 2026-08-22), so the deployed lane is
                          exactly as wide as Python's one worker until the
                          machine is scaled -- and the comment that used to
                          explain the sizing was wrong, crediting Go 1.25's
                          cgroup-quota reader for a number it never
                          computes. A Fly machine is a 1-vCPU Firecracker
                          microVM on cgroup v1 with every controller at the
                          root, so there is no `cpu.max` to read, GOMAXPROCS
                          falls back to NumCPU, and the two agree. NET stays
                          at two and FORGE at
                          one, because neither of those bounds is a fact
                          about the machine: two is what a Claude call costs
                          per run, one is the shared `.dck` directory. The
                          `key` dedupe is the same one locked step (per
                          owner, ADR 5; live jobs only, so a failure stays
                          retryable), a cache hit is still a job born `done`,
                          and there is still NO cancellation -- `ForgetOwner`
                          drops a running job without stopping it, exactly as
                          Python does, and guards owner zero because that is
                          how this port spells the local user. Two
                          differences are Go's rather than the design's: the
                          mutable half of a Job is **guarded** (Python's
                          worker writes `status` and `done` unlocked and the
                          GIL absorbs it; `-race` does not), and a
                          **panicking worker is a failed job**, not a dead
                          process. Held to Python by jobs/testdata/jobs.json,
                          which caught three divergences a careful port would
                          have shipped: `percent` **rounds half to even** (1
                          of 8 is 12.5 -- Python 12, `math.Round` 13),
                          `created_at` **loses its fraction entirely** at a
                          zero microsecond and is spelled `+00:00` never `Z`
                          -- and it is the sort key, as text -- and the lane
                          refusal quotes with `repr`, which prefers single
                          quotes. One rule for every family still to cross
                          falls out of it: **a job result must be a struct
                          with its fields in Python's order, never a
                          `map[string]any`**, because encoding/json sorts map
                          keys and a dict does not.
                          **Tier 1.5 followed, 2026-08-22, and flipped
                          nothing**: internal/sim/karsten and
                          internal/sim/curve are the closed forms, over a
                          shared internal/sim that holds the compiled card
                          and the three CPython float behaviours a port has
                          to reproduce (math.fsum, both rounds). PLAN 5.4
                          asked for "an epsilon pinned per function"; every
                          one is pinned at ZERO and the corpora compare
                          Float64bits, because exact was affordable --
                          math/big binomials where Python has math.comb, one
                          correctly-rounded big.Rat division, and Shewchuk's
                          summation transcribed. Exact is also *wanted*:
                          required_sources, reliable_turn and
                          _slots_to_target all scan a float against >=, so
                          one ulp is a different land count or a different
                          recommendation, never a different last decimal.
                          Two divergences the port FOUND rather than
                          inherited, both fixed in both runtimes at once.
                          **arm64 fuses `t += a*b` into one FMADDD** and
                          rounds once where CPython rounds twice -- on the
                          architecture the image ships; sim.Rounded is the
                          explicit conversion the Go spec blesses for
                          exactly this, and the disassembly was read to
                          confirm it survives inlining. And **sum() is not
                          the same function on every interpreter**: CPython
                          3.12 gave sum() over floats compensated
                          accumulation where 3.11 adds left to right, so
                          `curve.expected_lands_in_play` and
                          `curve.on_curve_odds` answered differently
                          depending on the Python underneath -- both are
                          `fsum` now, and the corpus was rendered under 3.11
                          and 3.12 and diffed to prove the fix.
                          **Tier 1 followed the same day, and
                          `REFERENCE_DIGEST` is reproduced**:
                          internal/sim/tier1 is the goldfish -- the London
                          mulligan, the land heuristic, `_consume`'s Kuhn
                          matching, the timing table and its sort -- over the
                          same shared `internal/sim` the closed forms brought,
                          which gained one thing for it: `Card.Equal`. That is
                          not decoration. `list.remove` takes out the first
                          card **equal** to its argument rather than the one it
                          was handed, and a compiled deck repeats one object
                          per `qty`, so which basic leaves a hand reorders the
                          rest and picks the next land. It flipped no route;
                          Python still serves every simulation. The gate is
                          `tests/test_determinism.py`'s sha256 over `repr()` of
                          one game, one 300-game run and a three-point sweep,
                          and Go computes the same one -- which meant
                          reproducing CPython's `repr` too (repr.go), since the
                          digest hashes text: `100.0` is not `100`, and
                          `median_commander_turn` is an **int** for an
                          odd-length list because `statistics.median` returns
                          one, its `float | None` annotation notwithstanding.
                          The digest matched on the first run, which is
                          pyrand's doing -- the one half that could not have
                          been debugged from outside was already proved. But
                          **the digest is one deck and one seed**, and its
                          blind spots are real: `build_golgari`'s 99 names are
                          all distinct, so nothing in it exercises
                          `list.remove` taking out an *equal* card, and its
                          policy mulligans at most three times. A second corpus
                          covers both (a deck of repeated cards, policies that
                          mulligan to nine) and **caught the port's one real
                          bug**: Go re-reads a `for` condition every pass where
                          `range(min(mulligans, len(hand) - 1))` is computed
                          once, so at four or more mulligans it bottomed a card
                          too few. Nine of ten deliberate mutations die against
                          the corpus; the tenth is an equivalent mutant and
                          says so in the code. Two absences are deliberate:
                          `SimSummary.report()` is `cli.py`'s text table, which
                          no route reads; and `spells_through` is **not** in
                          the corpus, because it sums floats and CPython's
                          `sum` is compensated from 3.12 and naive before it --
                          so the value is a fact about the interpreter, Go
                          answers as 3.12 does (what the image runs), and the
                          corpus stays byte-identical on both legs of the
                          matrix. That is the same trap the closed forms hit in
                          `curve.py`, found twice in one day from opposite
                          directions.
                          **The two generic job routes did NOT follow the
                          registry** (examined 2026-08-22): GET /api/jobs and
                          GET /api/jobs/{job_id} own no state -- they are the
                          VIEW over a registry the eight job-submitting
                          families still write from the uvicorn process, and
                          a registry is per-process, so a Go handler would
                          answer an empty list and a 404 for every id the app
                          hands out. The rule that fell out: **a route can
                          only flip when the state it reads has flipped, so a
                          view flips last, not first** -- "it is only a read"
                          is exactly backwards here. api_test.go's
                          TestTheGenericJobRoutesAreStillPythons is the
                          tripwire.
                          **internal/mana grew the SOLVER beside its
                          parser** (2026-08-22) -- `can_pay`, `expand_units`
                          and Kuhn's augmenting-path matching -- and it is a
                          library like pyrand: no route flipped, and `CanPay`
                          has NO CALLER at all, because the goldfish carries
                          its own private `canPay` over `sim.Source` exactly
                          as `engine._consume` re-solves this in Python.
                          `tier1.go` says in a comment that its own becomes a
                          call to this one; until then **the two Go solvers
                          are each held to Python and neither to the other**,
                          where Python pins its pair with
                          `test_consume_agrees_with_can_pay`. `mana.Source`
                          is deliberately field-for-field `sim.Source`, in
                          order, so the conversion at that seam is free; they
                          stay separate types because `mana` sits BELOW `sim`
                          and must not import it -- the same split
                          `sim.Cost` beside `mana.Cost` already makes. The package
                          takes plain records and imports nothing outside the
                          standard library, which is `mana.py`'s own boundary
                          and the reason the most correctness-critical
                          function here can be asked ten thousand questions
                          in a few milliseconds. It is held to Python by
                          `testdata/castability.json`, which carries the
                          **enumeration** rather than 13,944 dumped rows: Go
                          rebuilds the case set from the same alphabets in
                          the same order and reads only the answers, because
                          `mana_oracle.py`'s claim is that its cases come out
                          identical "in any language, on any machine,
                          forever", and replaying a dump would leave that
                          claim untested. Two digests, so a failure localises
                          -- the case NAMES alone, then the names with their
                          answers -- and `tests/go_fixtures.py` **compares**
                          `CASES_ANSWER_DIGEST` rather than writing it,
                          refusing to render a corpus that would move a pin.
                          Both of `mana_oracle.py`'s references are rewritten
                          in Go from their theorems (a backtracking search
                          over injective assignments; Hall's condition over
                          subset bitmasks), never transliterated -- a second
                          clever algorithm may be wrong the same way, and a
                          copied one certainly is. They keep their OWN unit
                          expansion and colour comparison rather than calling
                          the solver's, so `ExpandUnits` and the six-bit
                          packing are judged against them -- over amounts 0,
                          2 and negative, because **every pool in the case
                          set is a single-mana source** and all 13,944 cases
                          together say nothing whatever about Sol Ring.
                          **And the 13,944 cases are
                          not enough, which is the thing to carry away**:
                          `case_costs` draws pips with
                          `combinations_with_replacement`, so every cost in
                          the set has its pips in non-decreasing width order,
                          and NO case ever offers a wide pip before a narrow
                          one. Deleting one line -- the `seen` reset between
                          pips, the classic Kuhn's mistake -- passes all
                          13,944, both oracles, and every hand-pinned trap,
                          while answering `{W/U}{W} <- [W U]` wrongly. Python
                          was never exposed (Hypothesis asserts exactly that
                          property); the hole was in what the documents
                          *claimed* the case set proved. It is pinned as a
                          limit by a named test now, the case is a permanent
                          trap in the Go table, and the fuzz target found it
                          by the order-reversal property rather than by an
                          oracle. The contract
                          suite runs through the door locally and in CI
decks/<slug>/deck.yaml    the app's data dir, NOT in git (ADR 30); the LIBRARY
                          lives on the instance's volume, and a checkout —
                          this one included — normally holds no decks at all
decks/<slug>/artifacts/   GENERATED — never edit by hand
Dockerfile                two stages, no Node; app runs non-root
docker-entrypoint.sh      fixes the volume's ownership, then drops privileges
fly.toml                  the only Fly-specific file; no secrets, ever
```

**Decks do not live in git** (ADR 30), **and since 2026-08-21 they do not
live on the laptop either** (Aaron's ruling, the day ADR 37's relabels made
the divergence visible: labels applied to the laptop copies had never reached
the volume, and only a hand diff caught it). The library's one standing copy
is the deployed volume at `/data/decks`, where the app's editing routes write
`deck.yaml` — a second standing copy anywhere else is a fork waiting to be
discovered. `decks/` in a checkout is just the app's gitignored data
directory, normally empty: `mtglab ui` works on whatever is put there, and
work that needs the real decks — an overnight Forge round-robin, a migration
rehearsal — pulls them from the instance first (`fly ssh sftp get`), treats
them as scratch, and deletes them after. The last laptop copies were pushed
to the volume, tarballed to `~/decks-laptop-final-2026-08-21.tar.gz`, and
deleted. The image carries no decks and no pool at all; `docs/HOSTING.md` §4
step 6 says how a fresh instance's library fills (a backup or an import).
Deck history is the activity log (ADR 28), and `swaps.md` diffs against the
last build's own snapshot (`artifacts/deck.last-built.yaml`), not against a
git revision.

Layering: `api/` must not import from `cli.py`. Anything both need lives in
`config.py` or the relevant package — that rule is why `deck_paths` and the
deck compiler are where they are.

Deck-facing endpoints never read the filesystem. They take a `DeckSource` from
the request scope (`api/deps.py`), so a second deck tier is one dependency to
swap rather than thirteen handlers to edit. A `DeckSource` is a **locator, not
a connection**: background jobs capture one and outlive the request.

Paths come from `config.py` and honour `MTGLAB_DATA_DIR` and
`MTGLAB_DECKS_DIR`, defaulting to `data/` and `decks/`. Tests point them at a
scratch directory with `config.use_paths()`; never reassign the globals.
`app.db` — the SQLite half of ADR 4, holding users and sessions — is derived
from `MTGLAB_DATA_DIR` and moves with it.

**Auth is off unless `MTGLAB_REQUIRE_AUTH` is set.** The local app is one
person on a laptop and a login in front of it would be a regression; the
deployment turns it on. When it is on, `api/auth.py`'s middleware refuses
every path that is not in `PUBLIC_PATHS` — **before routing**, so an endpoint
nobody remembered to protect is refused rather than served. Adding a route
means classifying it in `tests/contract/routes.json` — the one table
`tests/test_isolation.py`, the contract suite and (from Phase 2 of the Go
migration) the Go module all read; the suite fails until you do. Anything that belongs to one person is reported as **404, never 403**, to
another (ADR 5). Nothing in `src/mtglab/auth/` imports FastAPI, which is what
lets `mtglab users` work on a box with no web server.

Invites and password resets are built (ADR 16): `auth/tokens.py` issues
single-use hashed links for both, `auth/mail.py` is the `EmailSender` seam, and
**no test sends mail.** An invite is an *unclaimed* account
(`password_hash IS NULL`), never a disabled one — `disabled_at` is the
maintainer's revocation lever and redeeming a link must not undo it.

Admin authorization is built (ADR 17). **Admin routes live under `/api/admin`,
and the middleware refuses that prefix to a non-admin before routing** — the
same mechanism as `PUBLIC_PATHS`, so a route is protected by where it is
mounted. `deps.Admin` is the second check on the handlers. Adding an admin
route means classifying it as `admin` in `tests/contract/routes.json`, which
is checked against the prefix in *both* directions; the sweep then requires **403**
from a logged-in non-admin. 403 and not ADR 5's 404 is deliberate and argued in
ADR 17 — an admin route's existence is published in a public repository.

`MTGLAB_ADMIN_EMAIL` names the maintainer, and `auth/bootstrap.py` reconciles
that account to admin-and-enabled at every start of the app and of every
`mtglab users` command, creating it unclaimed if absent —
`MTGLAB_ADMIN_USERNAME` gives it a handle, and is the one thing *not*
reconciled, since renaming somebody at boot is a surprise. Unset, none of it
happens, which is what a laptop wants. **There is no `MTGLAB_ADMIN_PASSWORD`**
and there will not be one; ADR 16 stands. Separately, `users.set_admin` and `set_disabled`
raise `LastAdmin` rather than removing the last admin **who can sign in** —
enabled and holding a password, because an instance whose only admin is an
unclaimed invite is locked out just as thoroughly.

The browser side is built (`docs/HISTORY.md` §6 step 5c), so auth is complete
code-side and what is left before a deployment is infrastructure. **Auth is a
gate, not a route**: with `MTGLAB_REQUIRE_AUTH` on and nobody signed in,
`App.tsx` renders `routes/Login.tsx` in place of the header, the nav and the
router — the client's version of a middleware that refuses everything outside
`PUBLIC_PATHS` before routing. With auth off the gate is never reached and the
app is unchanged: no login, no sign-out button, nothing. That is what
`auth_required` and `authenticated` are two fields for, and `App.test.tsx`
pins it.

`routes/Claim.tsx` is the page the emailed link lands on. The token arrives in
the URL *fragment*, so it reads `location.hash` and never the query string —
which would put a live credential in every access log — and it is cleared from
the address bar only once spent, because a refused attempt has to be
retryable. **Claiming sets no session**; it hands the username to the login
form. A 401 from any fetch is handled once, in `lib/api.ts`, which announces a
lost session so the shell can re-ask `/api/auth/me` rather than each screen
guessing. Login and reset are rate limited and answer 429 with `Retry-After`,
which the forms count down. The reset answer is rendered **verbatim** and
never as a confirmation: the endpoint says the same thing whether or not the
address exists, and a cheerful "check your inbox" would leak from the client
what ADR 16 built the server not to say.

**Tier 1 results are cached** (ADR 18), and the key is what makes that safe:
a hash of the **compiled** deck — the SimCards the engine is handed — plus the
clamped parameters, the seed, and a fingerprint of `engine.py` and `mana.py`'s
source. Not a hash of `deck.yaml`: card facts come from the pool, so a
`data refresh` can change a simulation while the deck file does not move. A hit
is a job that was born `done`, so no client changed shape, and every result now
carries `seed`, `cached` and `computed_at` — **quote a cached number as
cached.** Runs are seeded by default (`simruns.DEFAULT_SEED`); an unseeded
sample was what the app used to show and is not reproducible. Land sweeps cache
per count, so an overlapping range reuses rows. `mtglab sim cache [--clear]`.

**Every deck edit is recorded, from one call site** ([ADR 28](docs/adr/0028-the-activity-log-records-edits-and-never-rationales.md),
schema v8). `service._commit` is where every deck write already goes and it
had always assembled the per-operation description and thrown it away;
`decks/log.py` keeps it. One call site is the whole design — the tenth edit
operation is the one somebody adds in a year, and it inherits the log the same
way it inherits the gate's verdict. Read it with `mtglab decks log <slug>` or
the deck page's History tab.

Four rules, each of which is a decision rather than a default. **No rationale
text ever lands in it** — `describe` builds its sentence from card names,
categories and field *names*, and drops the `why` where `swap_card` passes
one; the log says a rationale changed and never what it says, because rule 4's
text lives in `deck.yaml` and a second copy is both stale and a place it could
be mined from. **A deck is `owner_id` + `slug`, and the file tier is
`owner_id IS NULL`** — not the URL's owner segment, which is `local` on a
laptop and a username deployed, so a history keyed on it would split in two
the day `MTGLAB_ADMIN_EMAIL` was set. **Who may read one is decided by where
the route is mounted**, through `Library` like every other per-deck route, so
there is no second visibility rule to keep in step. And **`record` never
raises**: the deck write has already happened, so a failure there is a logged
warning, exactly as in `sim/cache.py` and `claude/ledger.py`.

**It is not an undo, and must not become one.** ADR 27 rejected a server-side
journal because an undo buffer only the database knows about is deck state the
deck file does not show; that argument stands and the graveyard in `deck.yaml`
is still the undo. This records that something happened, never enough to put
it back. Creation, import and deletion are outside it because they are outside
`_commit` — adding them means a second call site, which is a decision to take
deliberately rather than by drift.

**`colors.py`, `glossary.py`, `lore.py` and `tarotlore.py` are reference prose,
and that was argued rather than assumed.** They are the four modules that
deliberately know things Scryfall did not say — what a guild is, what happened to it, what a
mulligan is, whose brush painted the card — and the alternative was a Claude
surface. It lost on four counts, written
out in `colors.py`'s docstring and ROADMAP item 3 branch 4: `/api/colors`,
`/api/glossary` and `/api/lore` work with **no card pool and no network**, the
set is finite and
written once, ADR 20 already classed `colors.py` as a fourth source that is
free, and *bland prose is fixed by editing, which only checked-in text
allows*. Claude answers the unbounded per-deck question about a **commander**
— that is ADR 19, and it stays there.

Card facts inside that prose still come from the pool. `champions` and
`signature` hold **names**; `/api/colors/{key}` resolves them through
`get_cards` and a name that does not resolve is **dropped and counted**, the
same instrument ADR 19 built for the dossier's rivals. `signature` carries no
prose at all — what it asserts is that the card's identity is *exactly* that
combination, which a test checks — so the only editorial sentence attached to
a card is a champion's story role, and the card's own oracle text renders
beside it. Adding a name means the full-pool test in `tests/test_colors.py`
has to pass; it is one test covering all three lists, because a second
`needs_full_pool` marker would move CI's skip gate off two.

The simulator's parameters and reported figures are glossary entries too
(`sim.*` for what you set, `stat.*` for what you are given), which is why they
live next to the `KeepRule` that defines them rather than in the React form.
`SIMULATOR_KEYS` in `tests/test_glossary.py` is the seam: TypeScript cannot
check a string against a Python table, so a renamed key fails there instead of
silently emptying a tooltip.

Keep `mana.py` and `sim/` dependency-light (stdlib + numpy). DuckDB stays
behind `cards/db.py`. `sim/cache.py` imports `auth/db.py` for one reason and it
is not auth: that module is the `app.db` connection helper, and a second
migration ladder for the same file would be worse than the import. That boundary is what keeps the simulation core fast to
test: the solver, the gate and the simulator all take plain records, so most of
the suite needs no database at all.

The tests that *do* need one build it. `tests/tiny_pool.py` loads 21 real
cards into a scratch DuckDB in about a second, and `mono_green_deck()` is a
legal 99 built only from those cards. That is what the card-fact tests use —
swap, add, suggestions, search, the Tier 1 endpoints, the Claude tools. It is
**not** the ~500MB Scryfall pool, which stays out of git and out of CI
(ADR 6). Before it existed those 29 tests skipped on every pull request and
passed only on this machine; `ci.yml` now fails if the skip count moves off the
two tests that genuinely need the full download.

## Non-negotiables

**1. Never evaluate a card from memory. Look it up.**

```python
from mtglab.cards import db
con = db.connect('data/mtg.duckdb')
for n, r in db.get_cards(con, ['Arahbo, Roar of the World']).items():
    print(r.name, r.mana_cost, r.type_line, sorted(r.color_identity))
    print(r.oracle_text)
```

This rule exists because of two real errors. *Ajani, Nacatl Pariah* was
proposed for a G/W deck — its back face is R/W, so its color identity is
illegal. And *Arahbo's* {1}{G}{W} doubling ability was described as eminence;
it is not, eminence is only the +3/+3 and the doubling requires him on the
battlefield. Both are checkable facts that were missed by reasoning from
memory. The user reads card text closely and will catch this.

**2. Color identity comes from Scryfall's `color_identity` field**, never
derived from the mana cost. It already accounts for back faces, reminder text,
and land types.

**3. Five artifacts for every new deck or refactor**, no exceptions:
`primer-quick.md`, `primer-advanced.md`, `decklist-annotated.md`,
`moxfield.txt`, and `swaps.md` when anything changed. Generate them with
`mtglab decks build <slug>`, or from the deck page's Artifacts tab — the same
`render_all`, so the two cannot produce different files. Never hand-write them.

**The deck page is how a deployed deck gets rebuilt** (2026-08-21). Until then
the only builder was the CLI, so refreshing the library's artifacts meant
`fly ssh console` — the laptop coupling the volume ruling ended, and a real gap
once `mtglab ui` became a development harness rather than the product. The tab
also answers the question nobody could ask before: **every artifact on the
volume was eight days older than its deck**, and nothing said so. `baseline` is
that answer — `current`, `different`, or `unknown` — computed by comparing the
stored snapshot against the deck, never a file timestamp, so reverting an edit
correctly makes the artifacts current again.

**4. Every card carries a `why`.** Validation fails without one. A card that
cannot justify its slot is a card to cut. **Never write one on the user's
behalf** — every write path refuses an empty rationale rather than inventing
one, which is what keeps [ADR 8](docs/adr/0008-the-gate-blocks.md) intact now
that the tool can edit decks. See
[ADR 11](docs/adr/0011-the-api-may-apply-a-swap.md). The rationale editor in
the app is the same rule in a UI: the box opens empty, its placeholder is a
question rather than a draft, and a test pins that.

The one bend, and it does not bend the rule: a deck declares a `stage` as well
as a `status`. In a **draft** — what `decks import` writes — a missing `why` is
a single counted warning rather than 99 errors, so the deck's *facts* get
checked on day one while the thinking is still owed. Promotion to **curated** is
refused while any card is blank, and the five artifacts refuse a draft outright.
Absent means `curated`, the opposite default from `status`, so the six existing
decks are never silently demoted. [ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md).

**5. Never commit** card pool data, collection/wishlist/purchase data, or
credentials. CI enforces this — by filename, and by scanning the contents of
every tracked file (the built frontend bundle included) for an API key. A
public inventory of expensive cards tied to a real identity is a targeting
list. Secrets reach the app through the environment: a gitignored `.env`
locally, `fly secrets` deployed, and `.env.example` documents the names.

`app.db` is gitignored for the same reason and one more: it holds password
hashes and, since ADR 16, **email addresses — the first personal data this
project stores.** An address must never reach a log line, an artifact, or a
Claude tool result. `User.as_dict()` leaves it out unless asked, and ADR 17
states the rule for who may ask: **an address may be serialised only into a
response an admin authenticated for.** Two callers — `mtglab users list`,
printing to the maintainer's own terminal, and `api/admin.py`. A third needs the
argument made again; `tests/test_isolation.py` is where it is pinned.

## Workflow

```bash
mtglab decks import <slug> --from list.txt --commander 'X'   # -> a draft
mtglab decks validate <slug>      # gate — fix errors before anything else
mtglab decks suggest <slug>       # shortlist replacements for what it flagged
mtglab claude argue <slug> --card X   # the case against a slot; never for it
mtglab decks swap <slug> --out X --in Y --why '...'   # apply your choice
mtglab sim mana <slug>            # baseline consistency
mtglab sim lands <slug> 30 40     # is the land count right?
mtglab sim shelf <slug>           # Tier 1.5 -- the closed form, no shuffling
mtglab sim mulligan <slug>        # search keep rules; best policy for a deck
mtglab sim cache                  # what Tier 1 results are memoised; --clear
mtglab sim forge <a> <b> [c] [d]  # Tier 3 — Forge plays real games
mtglab sim matches                # the match ledger: every Forge match recorded
mtglab decks build <slug>         # before a refactor, so swaps.md can diff it
```

**A local `decks import` writes a scratch deck, never a library entry.** The
library is the deployed volume, and `decks/` in a checkout is the gitignored
data directory the app happens to read -- so a deck imported here exists on
this laptop and nowhere else. That is the right place to rehearse an import,
ask the gate what it makes of a list, or drive the UI against something
disposable. It is not a way to add a deck to the library, and leaving one
there is the second standing copy the 2026-08-21 ruling ended. Delete it when
the rehearsal is over. A deck joins the library through the instance --
`POST /api/decks/import`, which the app's import screen drives and which runs
the same `service.import_deck` the CLI does.

Site imagery goes through the animist (ADR 29) — never hand-place a binary:

```bash
mtglab animist build <recipe>     # fetch, licence-gate, transform, encode,
                                  # and write the PROVENANCE entry itself
mtglab animist verify             # every committed asset vs its recipe
mtglab animist licence <recipe>   # the gate alone; per file, dated, no --force
mtglab animist measure <recipe> --output X   # the size curve and its knee
```

Editing, all surgical and self-verifying ([ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md)),
each also a route and a control on the deck page:

`set_shared` is a surgical edit too, since 2026-08-22. It was a
`Deck.load` / `Deck.dump` round trip -- the only thing on the write path that
called `dump` on an existing file -- so one press of the deck page's share
toggle rewrote the whole file, taking a hand-written deck's section banners,
its trailing comments, its folded blocks and its `swap_board: []` with it.
Found by the Go port, which had to reproduce the bytes and so had to ask what
they were; ruled by Aaron and fixed in both runtimes at once. It is
`edit.set_shared`, the editor's tenth operation, and deliberately not a
`SETTABLE_DECK_FIELDS` entry -- that tuple is what the PATCH beside it
publishes, and `shared` has its own route because the two tiers keep the fact
in different places.

```bash
mtglab decks add <slug> --card X --category ramp --why '...'  # pool-checked
mtglab decks remove <slug> --card X   # entombs: 99 -> graveyard, why intact
mtglab decks return <slug> --card X   # graveyard -> 99, exactly as it left
mtglab decks exile <slug> --card X    # the only permanent delete (ADR 27)
mtglab decks set <slug> --card X --why '...'        # or --category / --qty
mtglab decks set <slug> --status built              # no --card: a deck field
mtglab decks note <slug> --key mulligan --value '...'
mtglab decks set <slug> --art <set-code>   # which printing's art the deck shows
mtglab decks promote <slug>       # draft -> curated, once every card is justified
mtglab decks delete <slug>        # confirm by typing the slug; moves to decks/.trash/
mtglab decks log <slug>           what has been done to it, and by whom
mtglab decks build <slug>         # diffs against the last build's snapshot
```

A rebuild now **prunes the deliverables it did not produce**, which it did not
until 2026-08-21: a build with no baseline writes no `swaps.md`, so the
previous build's swap list used to sit in the directory describing a diff that
no longer existed — stale in the one way that is indistinguishable from
current. Found by a parity test across the deck sources, since the two tiers
disagreed about it, and fixed in the one place they share.

Measuring, before optimising or before trusting the suite:

```bash
mtglab bench run [--cold]         # the declared suite; medians and p95, never
                                  # a mean, and cold/warm are two numbers
mtglab bench profile <target>     # database budget, import calls, hot frames
mtglab bench caches               # hit rates; a dead cache is visible as dead
mtglab mutate run --sample N --seed S   # does the suite hold what it claims?
mtglab mutate list                # every site, and which tests defend it
```

Two rules these encode, both bought the hard way. **A large number is a
question, not a datum** -- `bench run` profiles anything over 25ms unasked,
because a run once recorded `/api/decks` at 224ms and moved on while 162ms of
failed `import pandas` sat inside it. And **a probe finds *which*, only a
profile finds *why*** -- the same episode was misattributed to YAML for three
days by a plausible guess. Record the numbers in `docs/polish/LEDGER.md`; the
polish skill's Black and White facets say when.

`swaps.md` diffs the deck against `artifacts/deck.last-built.yaml`, which
every build stashes (ADR 30 — decks are not in git, so there is no revision
to diff). **Build before editing** or the next build has no baseline;
`--against <path>` still accepts an explicit one.

## Python decides, Claude advises

The split, decided 2026-08-11 and argued in
[ADR 14](docs/adr/0014-python-decides-claude-advises.md): **anything with a
right answer belongs in deterministic Python; Claude is for opinions and
research.**

**Started, not finished.** `src/mtglab/claude/` is the pipe — a client on
`ANTHROPIC_API_KEY` and seven read-only tool schemas over `api/service.py` —
plus the stance (`stance.py`, three axes, off by default) and **seven** modes
across six features. The **rationale interview** (`interview.py`) asks about
a card's slot so you can write its `why`. The **slot argument** (`argue.py`,
[ADR 25](docs/adr/0025-argue-a-slot-argues-one-direction.md)) makes the case
against that slot. **Research** (`research.py`,
[ADR 26](docs/adr/0026-research-answers-about-magic-not-about-your-deck.md))
answers a question about Magic from pages it read — and **cannot see a deck**.
The **commander dossier**
(`dossier.py`, [ADR 19](docs/adr/0019-the-dossier-cites-three-sources.md)) says
who a deck's commander is, what archetype they define, who their rivals are and
where they sit in Magic's history. The **theme interview** (`theme.py`,
[ADR 20](docs/adr/0020-the-theme-interview-reads-a-person.md)) is two modes and
the create flow's first door: a conversation whose questions are **not about
Magic** — a film, a period, your sign, how you are at game night — and then a
proposal of two colour combinations with three pool-checked commanders each.
The door opens on a persona tile grid (seven voices, ADR 21) and the
fortune-teller tile deals the tarot spread.
`mtglab claude check` proves the key;
`mtglab claude interview <slug> --card X`, `mtglab claude argue <slug> --card X`
and `mtglab claude dossier <slug>` run the first three, and the deck page runs
all three. `mtglab claude research "<question>"` and the `/research` page run
the fourth — note it takes **no slug**, which is the feature and not an
omission. **Deck conversation does not exist** — check what is actually there
before assuming either way, and read ADR 26 before adding a deck to any
surface that does not already have one. The **activity log** it waits on does
exist now (ADR 28, below); the other three prerequisites do not.

**Research answers about Magic, never about your deck**
([ADR 26](docs/adr/0026-research-answers-about-magic-not-about-your-deck.md)).
The meta, a ruling in practice, a card spoiled ahead of the next bulk refresh —
the three questions the pool cannot hold. Its contract is an **absence**: no
`DeckSource`, no slug, no deck tool, and a route outside `/api/decks`. That
puts rule 4 out of reach (no rationale to read, no 99 to be asked what to cut
from) and stops **deck conversation being built by accident**, which was the
real risk, since "should I cut X" is what somebody types first and answering it
well *is* that mode under another name.

Three consequences. **Every finding cites a page the search actually returned**
— `keep_sources` reused from `dossier.py`, not copied — and a finding whose
citations all failed the check is **dropped and counted**, one step past what
the dossier does to a section, because a dossier passage may rest on its brief
and research has no brief. **If no source survives, the answer is refused.**
And **a card the pool lacks is labelled `in_pool: false`, not dropped** — the
deliberate opposite of the dossier's rivals, because a spoiled card is the
question rather than an error; `cards_unresolved` is reported apart from the
dropped counts, since above zero is the *right* answer for a spoiler. Nothing
is cached (the subject is the part of Magic that moves) but two identical
questions in flight are one job.

**The slot argument argues one direction, and that is the whole design**
([ADR 25](docs/adr/0025-argue-a-slot-argues-one-direction.md)). The interview
holds rule 4 with a predicate anybody can read — everything it returns must end
in a question mark — and this mode's output is *all* declarative sentences
about a card's merit, so that guard would delete the feature. The replacement:
**it makes the case against a slot and has no way to make the case for one.**
The response schema has no `defence`, `verdict` or `summary` field and forbids
extra properties, so a balanced answer has nowhere to go. That matters because
the balanced version is the attractive one and it is a rationale generator: a
paragraph explaining why a card earns its place, grounded in the user's own
deck, is a `why` in everything but authorship. Guarding it in the UI would not
be guarding it — the CLI renders the same payload and the endpoint is public.

Three consequences. **Every charge must cite a fact or it is dropped and
counted** — the predicate moved from "is it a question" to "does it rest on
anything", since every item here is declarative by design. **Alternatives are
bare names and Python judges them**: each is resolved through the pool and
dropped if it does not exist, is banned, or falls outside the deck's colour
identity, counted separately in each case, because "you invented that card" and
"that card is off-colour" are different failures. That check is rule 2 made
executable — *Ajani, Nacatl Pariah* is in `tiny_pool` for it. And **a weak case
is reported as weak** via `strength`, because removing the counter-case must
not create pressure to invent a case.

**The stance dial is built** (2026-08-14), and since 2026-08-15 it is a
**one header control plus per-surface readouts** rather than a fieldset
repeated on three screens. That control is now the **stance slider inside the
settings gear** (`components/settings.tsx`, which also holds theme, ambience
and table sound); it began as a `stancemenu.tsx` of its own and was folded in
when the gear landed, so the file no longer exists and the pin was always one
global value in `lib/stance.ts`. `components/stance.tsx` is the
line each Claude panel keeps saying what that setting resolved to *here*, and
`lib/claudecopy.ts` is the only place a wire token (`second-opinion`,
`on-request`) becomes a user-facing label — no raw enum or model id renders
anywhere. Three things about it survive the move unchanged. **"Follow the deck"
is a position, and the default one** — `default_for` reads the deck's `status`,
so a theoretical deck opens wider than a built one, and a control whose bottom
setting was `off` would throw that away the first time it was touched. **The
axes are a readout of the server's resolved answer**, never recomputed here; a
second copy of `clamp` in TypeScript would disagree silently. And **a pin the
server refuses is dropped and the call retried bare**, because every Claude
panel gates on `/api/claude` — a renamed preset would not show an error, it
would remove the menu, which is the only control that can clear the pin.

`/api/claude` takes a **`surface`** because of what building the dial found:
the create flow has no deck, so the endpoint resolved `off` while
`theme.stance_for` was about to run that conversation at `second-opinion`. All
42 tests on that endpoint passed, because every one of them asked about a deck.
A surface's default is asked of the module that owns it, never copied.

**A mode also has a voice, and a voice is not a stance**
([ADR 21](docs/adr/0021-a-persona-is-a-voice-and-the-spread-is-the-slots.md)).
`stance.py`'s three axes are all about *how much the model does*; a
**persona** (`claude/persona.py`) is *who it sounds like*, which is
orthogonal, so it is its own field on the wire and inherits ADR 15's
constraint verbatim — same tools, same write scope, same schema. The voice is
**appended** to `CONVERSATION_INSTRUCTIONS`, never substituted, which is what
keeps the interview's own rules out of a persona's reach; a parametrised test
asserts each of them still appears in *every* persona's prompt.
`CONVERSATION_MODES["plain"] is THEME_CONVERSATION` — identity, not equality,
because that block is what `converse` caches. **Seven voices are built**
(2026-08-15): `plain`, `fortune-teller`, and five costumed ones — therapist,
scientist, chef, storyteller, barkeep — each a `Persona` and a prompt with
nothing else to move, which was ADR 21's claim and held when tested. Only the
fortune-teller deals; the roster ships `{key, label, blurb, deals}` and never
a prompt. Each costumed voice has a Scryfall art crop on its tile
(`PERSONA_ART` in `components/tarot.tsx`, hotlinked with credit, never
committed); `plain` is deliberately artless — the tile with no costume.

**"Start a deck" has three doors now, and the persona grid is the first.**
The theme door and the tarot door merged (2026-08-15): "Help me decide" opens
the tile grid — pick who you talk to — and the fortune-teller tile is where
the old "Read my cards" door went, dealing exactly as before. The grid, the
table and the resume-from-stash logic all live in `components/tarot.tsx`,
and a stashed `{persona, seed, turned}` walks back to its table past the grid.

**The readiness count may not go backwards, and the conversation may not end
in a wall** (2026-08-18, after Aaron and his sister sat down at the table).
`_closing_for` asks the model to re-state every slot it is confident of each
turn and nothing checked that it did, so a turn that spoke to one kind deleted
the others: driven with the short answers a first-timer actually gives, the
count went 0, 1, 0, 1, 0 and never reached three. `theme.carry` is the
enforcement — the previous reading is the floor a turn builds on, a kind this
turn spoke to replaces what was there, and both halves have been through
`ground` so this widens what is *remembered*, never what counts as evidence.
Then the ceiling: at `MAX_EXCHANGES` with the floor unmet the screen was a
live answer box returning the same closing sentence forever beside a disabled
proposal button, and only "start over" worked. `finished` in
`components/theme.tsx` closes the box and opens the guided door instead —
commandment 2, because the alternative tells a newcomer they answered wrong.
Two related fixes went with them: `followJob` now rides out a handful of
dropped polls (a 226-second proposal used to die on one blip while the server
finished work nobody was listening for), and the proposal's clock renders in
the conversation column as well as the sidebar, which stacks *below* it on
anything narrower than a laptop.

**The tarot table is the theme interview wearing a costume.**
`tarot.py` is stdlib, holds all 78 cards and
**no card's meaning** — Python shuffles, the reader reads. `tarotlore.py` is
what she may say while she reads: a checked-in corpus about the 1909 deck and
the woman who painted it (Aaron's choice of well, 2026-08-18), offered in the
frame message and **cited by id**, so `theme.keep_fact` renders the corpus's
own sentence and a paraphrase is discarded. The deck tier is true of every
spread, which is why a reading of three minors is never empty. **All 78 cards
carry their own facts** (377 in total), every minor at five or better,
and every picture fact was checked against the committed 1909 plate rather than
recalled. The load-bearing
decision is that `tarot.SPREAD`'s three positions **are** `SLOT_KINDS[:3]`
(taste, temperament, posture) with `len(SPREAD) == FLOOR`: a card is dealt
*for* a slot, so ADR 20's grounded-quote readiness works untouched and **the
querent's own words stay the only evidence — a card is not something they
said.** A test pins the coupling because its failure is silent: drift, and the
proposal button simply never lights up. The deal is seeded and returns its
seed, so the client carries one integer and a reload deals the same three
cards; the dealt cards ride in the frame **message**, never the system prompt,
which is what `converse` caches on. `persona` and `seed` are client-held
exactly as the transcript is, and **a persona is fixed for a conversation** —
`components/tarot.tsx` remounts the interview on its key rather than warning
about it. The art is the 1909 Rider printing and the licence was checked per
file; the 1971 recolouring everybody pictures is still in copyright and is not
this. Because the pictures are package-data, the `image` CI job is the only
place a packaging mistake is visible, and it counts them.

**The theme proposal is a background job** (`api/themeruns.py`), because it was
measured at 226 seconds and no hosted proxy holds a POST open that long. The
division is the one `api/simruns.py` already makes and it is load-bearing here:
**everything refusable is refused in the request** — a malformed transcript
(422), a floor not yet reached (409), no key (503) — and only the Anthropic call
is queued, because three distinct answers delivered as a job in state `error`
are one string and no status code. `jobs.py` grew a second pool for it: Tier 1
keeps its single CPU worker (pure Python, GIL-bound), and anything waiting on a
socket goes in `NET`, so a thirty-second sweep never queues behind four minutes
of somebody's conversation. Nothing is cached — a proposal is not reproducible
and its subject does not outlive the conversation — but the **client keeps the
job id**, so a reload reattaches rather than paying twice.

**And so is the theme conversation turn**, which is the same module's
`plan_ask` and the weakest case of the three on its own numbers: 4.3–37.7s
across eleven measured turns, with one at 133.8s that did not reproduce. It
moved anyway, because the docstring keeping it synchronous said *"it is a few
seconds"* — the sentence that left the dossier synchronous until it broke — and
because the transport ceiling is known only as **at or below 236s**, with
133.8s inside the unmeasured region. **A duration measured for one surface is a
question to ask of every sibling surface**; this was the sibling nobody asked.
Two differences from the proposal, both deliberate: a no-call turn (stance
`off`, or past `MAX_EXCHANGES`) is a **job born finished**, so the client passes
it to `followJob` as `initial` and the cheap case still costs one request; and
it takes **`key=None`**, because two turns in flight are two conversations
rather than one question asked twice.

**So is the dossier** (`api/dossierruns.py`), and the reason it took a second
session to get there is worth keeping. It was measured at **236 seconds on the
deployed instance** — *longer* than the proposal that pattern was built for —
and stayed a synchronous POST because nobody re-measured it when ADR 20 was
written. Deployed, it presented as a spinner and then Safari's `Load failed`: a
**transport** error, so no status code reached the client, and no access-log
line was written either, because uvicorn writes one when a response completes
and this one never did. The work itself was fine and sat in `dossier_cache`
while the page showed a failure. Two lessons rather than one — **a duration
measured for one surface is a question to ask of every sibling surface**, and
**the HTTP surface had no tests at all**, which is how it shipped: the 42 tests
matching "dossier" all exercised the module and none asked what the route did.
Unlike the proposal it *is* cached (ADR 19, on the commander's `oracle_id`), so
a hit is a job born finished.

And it is **deduplicated in flight**, which is the same argument one step
earlier in time: the cache covers "somebody asked before", `Plan.key` covers
"somebody is asking right now". Both were needed and only the first was built
at first — two paid runs for the same commander went concurrently on the
instance because a second click inside the four-minute window had nothing to
collide with. `jobs.submit(key=…)` does the lookup and the insert in one locked
step, matches per **owner** as well as per key (two accounts sharing an id would
give the second a 404 for a job it had just been handed — ADR 5), and joins only
a **live** job, because a finished one is the cache's business and a failed one
must stay retryable. `key=None` is the default and opts out, which is right for
a theme proposal: two at once are two different conversations.

**The dossier is the first mode whose facts are not all the pool's**, so it
carries rules the interview did not need. Card facts still come from the pool,
always. The meta and the history come from **Anthropic-hosted web search**
(`web_search_20260209`) — not a crawler, and not a way around the ban: it reads
at request time, for one commander, and shows the link. Claude supplies voice
and carries no factual weight. Three things enforce that and none is the prompt:
the schema keeps prose and sources in different fields; **every cited page is
checked against what the search actually returned** (a response schema
suppresses the API's own citations, so a URL in the payload is otherwise just a
string the model typed); and every rival is resolved through `get_cards` or
dropped. **If no source survives, the dossier is refused rather than shown.**
Cached on the commander's `oracle_id` — a dossier is about a character, so every
deck that commander leads shares one — and stamped with the date it was written,
which is the honest substitute for a freshness guarantee it cannot make.

Two things about `converse` that only bite with a server tool: the dated search
filters inside a code-execution container, so the container id must ride along
on every turn after the first, and a `pause_turn` must be **resumed**, never
returned — it carries text that reads finished, which is the Forge-with-96-cards
failure wearing a different hat.

[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md) says
what a surface *is*: a **mode** (a system prompt, a tool set, and what it may
write) plus a **stance** (the user's dial over initiative, scope, and write
autonomy). A stance may widen what a mode does, never what it is allowed to do.
Card facts reach a mode through pool tools rather than recall, which is how
rule 1 below becomes structural instead of a request. Target model is
**Claude Sonnet 5** to begin with — the user's call, not a default to override;
**load the `claude-api` skill before writing any integration code.**

Deterministic Python owns legality, colour identity, singleton, deck size,
companion and partner rules, mana solving, Tier 1, category counts, similarity
and price. Reproducible, tested without a network, no model consulted. Claude
owns conversation about a deck and the questions the pool cannot answer — the
meta, whether a spoiled card earns a slot, what a ruling means in practice.

Three boundaries, all of which apply to you in this session as much as to
anything built later:

1. **Rule 1 binds Claude too.** Card facts come from the pool, not from
   recall and not from a web page. Research is for what the pool lacks —
   discussion, meta, rulings, cards spoiled ahead of the next bulk refresh.
2. **Argue about a `why`; never write one.** Interrogating a card's slot and
   making the case against it is the conversation the curated six came out of.
   Authoring the text that lands in `deck.yaml` is not, and no surface may
   pre-fill that field. Rule 4 above is the rule; this is where its edge is.
   In code the line is **no path passes a model response into
   `set_card_field(field="why")`** — an interview supplies questions, the user
   supplies the words. Enforced structurally rather than by prompt: nothing
   under `src/mtglab/claude/` may name a deck-write function at all, checked
   over the package's syntax tree by `tests/test_claude_boundary.py`, so the
   helpful-looking commit that adds one fails before a prompt is written.
3. **Say which system answered.** The gate's output is reproducible and
   checkable; an opinion is neither. Never present one as the other.

## Interpreting simulations

**Tier 1** shuffles, draws, and pays costs. It does not model opponents,
interaction, tutors, cost reduction, or card text beyond mana production.
State that caveat when quoting its numbers.

**Choosing a land count: read "spells deployed through T8", not commander
speed.** Commander speed rises monotonically with land count, so optimising it
alone always recommends more lands. Deployment peaks and then falls as flood
sets in. That peak is the answer.

**An invalid deck is simulated, not refused -- and every result says so.**
Decided 2026-08-21 with Aaron. Refusing was the obvious call and is the wrong
one: two of the six decks here deliberately fail the gate on a banned card, a
deck mid-import fails it by construction, and the simulator is the tool
somebody reaches for to *fix* a deck, so refusing removes the diagnosis at
exactly the moment it is wanted (commandment 2). Instead every Tier 1 and
Tier 1.5 result carries `deck_check` -- the gate's verdict, attached after the
cache because the numbers are keyed on the compiled deck and the verdict is
not. `MOVES_THE_NUMBERS` in `api/simruns.py` splits failures that change what
was computed (a banned card is in the 99 being shuffled; an unresolved card
was dropped, so the deck shrank) from failures that do not (a missing
rationale blocks a curated deck and has nothing to do with mana), because the
screen says something different about each.

**One state is refused**: a deck that compiles to no cards raises
`NothingToSimulate`. An empty deck used to answer with a 100% mulligan rate,
zero spells through turn 8, and a shelf demanding coloured sources against a
library of nought -- every figure arithmetically correct and none of them
about anything. `adrix-and-nev-twincasters` was in that state on the deployed
instance.

**Tier 1.5** (`sim/karsten.py`, `mtglab sim shelf`) answers with arithmetic
what Tier 1 answers by sampling -- exactly, and about a simpler game. It
assumes you keep your seven, hit every land drop, and have every drawn source
in play, and it cannot see ramp at all. **Quote it as a question about the
mana base, never as a chance of having the card**: a 96.7% turn-one card is
cast in 4.3% of games, and both figures are correct because they answer
different questions. Its coloured requirements run a shade stricter than
Karsten's published table, which models the London mulligan and this does not;
the direction is known, documented and pinned by a test.

**Tier 2** (pod simulator, not yet built, and **deferred behind Tier 3** as of
2026-08-11) is a model of Magic. Right for bracket placement and matchup
matrices, wrong for "is this line correct." Forge goes first; Tier 2 gets built
only if Forge cannot answer those questions. See ROADMAP goal 2.

**Tier 3** (Forge bridge — the spike landed 2026-08-11; `mtglab sim forge`)
runs `forge.jar sim -d ... -f Commander`. Forge's AI is best with aggro and
midrange, poor with control, bad with most combo. The user's decks sit right on
that fault line — Dino and Cat are what Forge plays well; Tivit and Gyome are
what it plays badly. **Report Forge results per archetype with that caveat,
never as a single ranking.** Also required, and all now in `sim/tier3/`: a
pre-flight card-coverage check, a raised `-c` clock (the default is 300 here),
and draws reported separately rather than folded into losses — with a clock-out
separate again from a real draw.

The coverage check is not a formality. **A card Forge does not implement does
not stop the game**: it prints a warning and plays on with 96 cards, reporting
a winner and a turn count that look entirely normal. That is why coverage is
checked twice, before and after, and why `run_games` raises instead of
returning a flag. See [docs/FORGE.md](docs/FORGE.md).

**Quote a median and a tail, never a mean.** Game length is heavily
right-skewed: heads-up medians sit at 4.6–6.8s, but one Trostani game took
134s. A mean hides that, and the tail is what a timeout has to be set against.

## Working style

- Ask before large design decisions rather than guessing.
- Prefer surgical trims over mass restructuring. The user pushes back on
  aggressive cut lists and expects each cut argued against the specific slot
  it vacates.
- Deep cuts from old Magic are actively wanted. Query the whole pool.
- Price is not usually an object, but prefer the cheaper option when a genuine
  functional equivalent exists.
- Reserved List is allowed or forbidden **per deck** — check the deck file.
- Every bug fix gets a test. `mana.py` is subtle; `tests/test_mana.py` pins the
  cases where naive source-counting gives the wrong answer.
- Go, from `go/`: `go vet`, `go test -race` and `golangci-lint run` before
  pushing — the three checks CI requires, and on this Mac all three want the
  environment the Setup section spells out (CGO on, the 1.26 toolchain on
  `PATH` and `GOROOT`). `gofmt -l .` should print nothing. Package comments
  carry the argument, like docstrings do here.
- `ruff check src tests` and `mypy` before pushing. mypy is strict by default
  with one named exception in `pyproject.toml` (`cli.py`, since `cards/db.py`
  graduated 2026-08-16); that list is meant to shrink,
  so a new module is strict from the day it is written.
- Frontend: `npm --prefix web run check` runs the typecheck, oxlint and Vitest
  in one; then rebuild the committed bundle with `npm --prefix web run build`
  if anything under `web/src` changed. CI checks all four, and runs the first
  three as separate steps on purpose so a type error reports as a type error
  rather than as an opaque build failure.

## Landing work

The repo is public and `main` is protected: pull request required, **every
required CI check** green, branch up to date, enforced for admins. A direct
push to `main` is rejected — branch first, then open a PR. Squash merge; linear
history is required.

**The count is deliberately not written here.** It said "all six" from
2026-08-12 until 2026-08-22 and was wrong for the last three days of that,
twice over — `image-arm64` joined, then the three Go jobs, then `contract`.
The list lives in ENGINEERING §5, which records it as a **read-back from the
API** rather than as something a session remembered; a number in this file is
a claim to re-check, exactly like the ones about the decks.

The fifth is `image`, added 2026-08-12 with containerisation. **It cannot be
run locally** — this Mac is macOS 12 on Intel, where Docker Desktop will not
install and Homebrew is too stale to build Colima, so CI is the only place the
`Dockerfile` is ever built. Treat a red `image` job as the first real feedback
on a container change rather than as a surprise.

The sixth is `dependency-review`, required since 2026-08-14 and the only one
that is **not** a `ci.yml` job. It runs on `pull_request` only — it diffs the
dependency graph between base and head, and a push has no base — so it gates
merging and takes no part in the deploy, whose `needs` list is `ci.yml`'s five.
It also cannot be run locally in any useful form. See ENGINEERING §5.

`image-arm64`, added 2026-08-19 when the arm64 build moved off QEMU onto a
native runner, is required too — read back from the API on 2026-08-21, having
been described here as optional until somebody asked.

**`contract`**, added 2026-08-21 with the Go migration's Phase 1, runs
`tests/contract` with `--live` — the harness seeds a scratch, starts
`mtglab ui` on it and drives it over TCP — and, since Phase 2, runs the same
suite once more **through the Go front door**, built from `go/` in the same
job and stood in front of a second Python server. It gates the deploy through
`needs` **and is required on `main`** since 2026-08-22.

It was written as "owed to Aaron" for three sessions, which was the wrong
word: the setting is a `gh api` call and the `gh` CLI is Aaron's own, so
nothing but a confirmation was ever missing. **Anything a session can do with
his credentials and defers on is a question to ask, not a line to write in a
document** — a note saying somebody else must do it reads like a permission
boundary and outlives the moment it was true.

**And three Go jobs, `go (amd64)`, `go (arm64)` and `go-lint`**, added with
Phase 2 (2026-08-21): `go vet`, the door build as the image builds it (CGO since Phase 3), race-detected tests
and a tidy check on both architectures the image is built for, and
golangci-lint. All three gate the deploy and **are required on `main`** —
both steps taken in the same change, read back the same day.

**Merging deploys.** Since 2026-08-14 a push to `main` whose `ci.yml` checks
are green deploys itself ([ADR 23](docs/adr/0023-a-green-main-deploys-itself.md));
the `deploy` job `needs` all five, so it cannot run on a red suite. Expect the
instance to be live about ten minutes after a merge, and note the two
consequences: **every merge is a few seconds of downtime** (one machine, one
volume, so Fly cannot roll), and **a schema migration in `auth/db.py` applies
on boot without anybody watching** — that ladder is forward-only, so rolling
the code back does not roll the schema back. Land a schema change on its own
branch and merge it when you can watch it. There is a manual button
(Actions → *tests* → Run workflow) which runs the whole suite and then
deploys; the runbook and the rollback are `docs/HOSTING.md` §5.

**Do not open a documentation-only pull request.** Updating `ROADMAP.md` when
direction changes is required and the rule above still holds — but a PR whose
whole diff is a few paragraphs costs seven CI jobs, a review round trip and a
squash commit to land prose nobody was blocked on. Three of them went in over
2026-08-12 alone ([#54](https://github.com/aasquier/sylvan-library/pull/54),
[#56](https://github.com/aasquier/sylvan-library/pull/56), and #58, which was
closed rather than merged once the pattern was named).

So: **commit the doc change on the branch that does the work it describes, and
let it ride along.** Write it *when the decision is made* rather than
afterwards — the point of these files is that they survive a fresh session, and
a paragraph sitting uncommitted on a branch still survives one. A doc change
earns its own PR only when nothing else is coming: a correction to something
already merged and wrong, or a decision recorded at the end of a phase with no
next branch to carry it.

## Planning documents

`ROADMAP.md` (goals vs reality, what is next, open decisions),
`docs/ENGINEERING.md` (the next phase: compiled backend, testing rigor, CI/CD),
`docs/HOSTING.md` (the runbook for the deployed instance), and since
2026-08-21 `docs/go-migration/` (the Go port: PLAN, the append-only
BASELINE, and the port board once Phase 2 starts — **the main line** until
the port retires Python, ADR 38). These are kept
current deliberately — read them before proposing direction, and update them
when direction changes.

**`docs/HISTORY.md` is the one that is deliberately not current.** Split out
of the other two on 2026-08-21 (Aaron's ruling), it holds what they had
accumulated underneath their live heads: HOSTING's auth, data-model and cost
analyses and its build order and readiness list, and ROADMAP's sixteen-phase
account of how this was built. Read it for *why* something is the way it is;
never for what is true now. Two things about the split are load-bearing.
**The section numbers travelled with the content** — HOSTING §§1, 2, 3, 6 and
7 are HISTORY §§1, 2, 3, 6 and 7, and HOSTING keeps a stub at each number —
because roughly fifty citations point at those numbers and fifteen of them are
inside ADRs, which are immutable and were not edited. And **nothing was
rewritten in the move**: where a moved section narrates a plan in the future
tense, that is the era it was written in.

`docs/adr/` records the decisions themselves — context, options considered,
decision, consequences. Unlike the three above, **ADRs are immutable once
accepted**: do not edit a decision, write a new one that supersedes it. Read
`docs/adr/README.md` before arguing for a change to something already decided.

## Out of scope

No purchase automation — the shopping tooling prices decks, watches for deals,
and builds carts, but never enters payment details and never checks out. No
marketplace scraping; prices come from Scryfall. No rules engine — the play UI
manages board state, it does not enforce rules, and Forge plays the games.
No web crawler either: research goes through Anthropic's server-side web
tooling, which is not a way around the scraping ban.

## The decks

Six curated Commander decks: Arahbo cats (Selesnya, Kaheera companion, cats
only), Atla Palani dinosaurs (Naya), Goreclaw mono-green stompy (bracket 4),
Tivit (Esper cEDH, bracket 5), Gyome food (Golgari, bracket 4), and Trostani
tokens (Selesnya — an older token deck retooled into this list).

**There is a seventh directory**, `adrix-and-nev-twincasters`, and it is not
one of the six: it is an empty draft — no cards at all — left over from an
import. It is why `compile_report` refuses a deck with nothing in it (a
simulation of it answered with a 100% mulligan rate and a shelf demanding
coloured sources against a library of nought). Fill it, or delete it; until
then, "the six" below means the six named above and the count of directories
is seven.

All six live on the instance's volume as `/data/decks/<slug>/deck.yaml` —
Aaron's app data, **not in git** (ADR 30) **and not on the laptop** (ruling
2026-08-21: the last laptop copies were pushed to the volume, tarballed, and
deleted), so a checkout has none of them and nothing in the suite may assume
otherwise. Local work that needs them pulls from the instance and cleans up
after itself; checking one of the facts below means asking the instance
(`fly ssh console -C "mtglab decks validate <slug>"`), not the laptop. The
original markdown in `~/Downloads` is historical and should not be edited or
re-imported. `ROADMAP.md` records what the migration turned up. The facts
below (statuses, stages, the two banned cards) are recorded here as prose
precisely because no test can read the files to check them.

Each deck declares `status: built | theoretical`. **Goreclaw and Tivit are
theoretical** — lists under consideration, not boxes of cards; the other four
are built. Absent means theoretical, so nothing is ever silently claimed as
owned. (Adrix is theoretical too, being empty.)

Separately, each declares `stage: draft | curated` — whether it has been
reasoned about, as opposed to whether it exists. **All six are curated**, and
Adrix is the only draft. A deck brought in with `decks import` starts as a
draft; see rule 4.

**Verified 2026-08-21** against `validate` and the deck files, not inherited:
built = arahbo, atla, gyome, trostani; theoretical = goreclaw, tivit, adrix;
the only non-curated deck is adrix; the only decks failing the gate are
goreclaw (banned card) and adrix (no cards). Re-run that check rather than
trusting this sentence — it is exactly the kind that has rotted twice before.

A deck may also declare its labels (ADR 37, superseding ADR 36's second
axis): one open `themes` list — identity, several per deck, strategy words
included, from the hand-curated vocabulary in `model.THEMES`, which grows
only by somebody *reading* and editing it, never by scraping (EDHREC's own
Terms of Use forbid automated queries, read 2026-08-21, so that door is shut
twice). The `archetype` the rating boards group by is a **reading** of the
declared themes, not a second declaration: among the four class words
declared (aggro | midrange | control | combo), the worst-Forge-piloted wins,
so a control deck with a combo finish can finally say both and wears combo's
caveat. Deriving a class from the *decklist* is still banned — that would
launder Forge's pilot bias into the boards; the themes are the human's own
words. A legacy `archetype:` key still answers while a file's themes name no
class word, and the next write drops it once shadowed. The match ledger
snapshots the reading at match time, so relabelling a deck changes its next
match and never its history. Absent means unlabelled.

**The deck page edits them** (2026-08-21), which closed the follow-up the
migration exposed: the only label editor had been the CLI, so relabelling the
deployed library meant `fly ssh console` — the laptop coupling the volume
ruling ended. `components/labels.tsx` is the control and `GET /api/themes`
serves the vocabulary, a route rather than a tuple copied into TypeScript for
the reason `SIMULATOR_KEYS` exists: a copy drifts silently and would offer a
label `set_deck_field` then refuses. Two things about it are load-bearing.
**The archetype is never predicted while editing** — it is a readout of what
the server resolved, the stance dial's rule applied again, because a second
copy of worst-piloted-wins living in TypeScript would disagree with the
Python one and nobody would learn which was right; the editor names which
ticked words are *class* words and stops. And **the four class words render
in `ARCHETYPES` order, easiest to hardest to pilot**, not alphabetically —
that gradient is the only thing their order carries, and the copy beside them
("the board reads the hardest of them to pilot") is unreadable without it.

**All six decks are labelled**, verified against the volume 2026-08-21:
arahbo (cats, aggro), atla (dinosaurs, sacrifice, tokens, midrange), goreclaw
(stompy, big-mana, ramp, midrange), gyome (food, aristocrats, sacrifice,
midrange), tivit (treasure, clues, politics, control, combo, cedh), trostani
(tokens, lifegain, midrange). This sentence said they were where "all six
start" — unlabelled — until the editor was built and the volume was actually
read. Re-check it there rather than here; it is the same class of claim as
the two above it.

**One** deck currently fails the gate, and it is Goreclaw, on Primeval Titan.
Picking the replacement is Aaron's call; the deck is `theoretical`, so nothing
physical is blocked by it.

This paragraph said *two* until 2026-08-21, naming Atla Palani for running
Emrakul, the Aeons Torn. **Atla runs Emrakul, the Promised End, which is legal
in Commander**, and the gate has been passing that deck for however long the
claim has been wrong. It is the third time a completeness claim in this file
has been false — see the `dev` extra above — and the lesson is the one already
written there: *a sentence in this file asserting a fact about the decks is a
claim to re-check against `mtglab decks validate`, not a fact to inherit.*
These are recorded as prose precisely because no test can read the deck files
(ADR 30), which is exactly why they rot.

**Goreclaw's banned card is not a test fixture and cannot be one.** `decks/`
is not in git, so CI has never seen that deck; the invalid-deck path is
covered by in-git tests over `tiny_pool`'s deck instead. Keeping the Titan
buys a convenient live example when walking the deployed UI, and nothing
else — do not let a test depend on it.
