# sylvan-library

Local-first Commander toolkit: deck files on disk, Monte Carlo simulation,
Scryfall-validated decklists, generated primers.

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
mtglab data refresh          # ~500MB from Scryfall, several minutes
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
  sim/compile.py          deck.yaml + pool -> SimCards
  sim/cache.py            memoised Tier 1 results, keyed on compiled input
  sim/tier1/engine.py     Monte Carlo goldfish
  sim/tier3/              the Forge bridge: .dck export, coverage, run, parse
  artifacts/generate.py   the five deliverables
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
                          `key` that makes asking twice at once one job
  api/simruns.py          Tier 1 planned in the request, run in a job
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
decks/<slug>/deck.yaml    SOURCE OF TRUTH — live app data, NOT in git (ADR 30)
decks/<slug>/artifacts/   GENERATED — never edit by hand
Dockerfile                two stages, no Node; app runs non-root
docker-entrypoint.sh      fixes the volume's ownership, then drops privileges
fly.toml                  the only Fly-specific file; no secrets, ever
```

**Decks do not live in git** (ADR 30). `decks/` is the local app's data
directory — gitignored, like the pool and `app.db` — and deployed, decks live
on the volume at `/data/decks`, which is the only copy that instance has: the
app's editing routes write `deck.yaml`, so decks baked into a layer would lose
every edit at the next deploy. The image carries no decks and no pool at all;
`docs/HOSTING.md` §4 step 6 says how a fresh instance's library fills (a
backup, your laptop, or an import). Deck history is the activity log (ADR 28),
and `swaps.md` diffs against the last build's own snapshot
(`artifacts/deck.last-built.yaml`), not against a git revision.

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
means classifying it in `tests/test_isolation.py`; the suite fails until you
do. Anything that belongs to one person is reported as **404, never 403**, to
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
route means classifying it as `admin` in `tests/test_isolation.py`, which is
checked against the prefix in *both* directions; the sweep then requires **403**
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

The browser side is built (`docs/HOSTING.md` §6 step 5c), so auth is complete
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
`mtglab decks build <slug>`. Never hand-write them.

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
mtglab sim cache                  # what Tier 1 results are memoised; --clear
mtglab sim forge <a> <b> [c] [d]  # Tier 3 — Forge plays real games
mtglab decks build <slug>         # before a refactor, so swaps.md can diff it
```

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

The repo is public and `main` is protected: pull request required, **all six**
CI checks green, branch up to date, enforced for admins. A direct push to
`main` is rejected — branch first, then open a PR. Squash merge; linear history
is required.

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

There is a **seventh job**, `image-arm64`, added 2026-08-19 when the arm64
build moved off QEMU onto a native runner — and it is deliberately described
here as a job rather than as a seventh required check, because it **is not
one**. It gates the *deploy* (`deploy` needs it, and `tests/test_packaging.py`
forces that), so a red `image-arm64` stops a release; it does not stop a
merge until somebody adds it to the required list, which is a repository
setting and not a file in this repo. ENGINEERING §5 is where that list lives
and why writing an aspirational one there is worse than writing none.

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

`ROADMAP.md` (goals vs reality, open decisions), `docs/ENGINEERING.md` (the
next phase: compiled backend, testing rigor, CI/CD) and `docs/HOSTING.md`
(deploying a shared instance). These are kept current deliberately — read them
before proposing direction, and update them when direction changes.

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

All six live as `decks/<slug>/deck.yaml` — Aaron's app data on this machine
and on the instance's volume, **not in git** (ADR 30), so a fresh checkout has
none of them and nothing in the suite may assume otherwise. The original
markdown in `~/Downloads` is historical and should not be edited or
re-imported. `ROADMAP.md` records what the migration turned up. The facts
below (statuses, stages, the two banned cards) are recorded here as prose
precisely because no test can read the files to check them.

Each deck declares `status: built | theoretical`. **Goreclaw and Tivit are
theoretical** — lists under consideration, not boxes of cards; the other four
are built. Absent means theoretical, so nothing is ever silently claimed as
owned.

Separately, each declares `stage: draft | curated` — whether it has been
reasoned about, as opposed to whether it exists. **All six are curated.** A
deck brought in with `decks import` starts as a draft; see rule 4.

Two decks currently fail the gate on one card each, deliberately and not as a
bug to route around: **Goreclaw** runs Primeval Titan and **Atla Palani** runs
Emrakul, the Aeons Torn, both banned in Commander. Picking the replacement is
the user's call.
