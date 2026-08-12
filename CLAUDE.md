# sylvan-library

Local-first Commander toolkit: deck files in git, Monte Carlo simulation,
Scryfall-validated decklists, generated primers.

Python 3.11+ · DuckDB · numpy. The package and CLI are named `mtglab`; the repo
is `sylvan-library`. That mismatch is intentional and not a bug to fix.

## Setup

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
mtglab data refresh          # ~500MB from Scryfall, several minutes
pytest -q
mtglab claude check          # optional: is the API key live?
```

Extras: `api` (FastAPI + the app), `claude` (the Anthropic SDK), `dev` (which
includes both plus the test tooling). A base install has the gate, the mana
solver and Tier 1, and needs neither a network nor an account. `claude check`
needs `ANTHROPIC_API_KEY`; see `.env.example`.

`data refresh` needs network access to `api.scryfall.com` and
`data.scryfall.io`. In a cloud session with default Trusted network access
those are not reachable — widen the environment's access level first, or run
`--oracle-only` (much smaller, covers everything except pricing). Do not put
`data refresh` in a setup script; it will blow the five-minute budget.

## Architecture

```
src/mtglab/
  config.py               where decks and the corpus live; env-overridable
  mana.py                 cost parsing + castability solver
  cards/db.py             Scryfall bulk -> DuckDB, price history
  decks/model.py          deck.yaml schema
  decks/edit.py           surgical deck.yaml edits, minimal diffs
  decks/decklist.py       pasted decklist -> parsed lines; pure text
  decks/importer.py       parsed lines + corpus -> a draft deck.yaml
  decks/source.py         DeckSource protocol; file-backed and in-memory
  decks/suggest.py        similarity scorer -> replacement shortlists
  decks/validate.py       the gate
  decks/companion.py      companion deckbuilding restrictions
  decks/partners.py       Partner / Background / Doctor pairings
  decks/analyze.py        macro category counts vs bracket targets
  sim/compile.py          deck.yaml + corpus -> SimCards
  sim/cache.py            memoised Tier 1 results, keyed on compiled input
  sim/tier1/engine.py     Monte Carlo goldfish
  sim/tier3/              the Forge bridge: .dck export, coverage, run, parse
  artifacts/generate.py   the five deliverables
  auth/                   app.db, Argon2id, accounts, sessions, rate limit,
                          invite/reset tokens, the EmailSender seam
  api/                    FastAPI app, services, background sim jobs
  api/auth.py             the deny-by-default middleware and login routes
  api/deps.py             the request scope: who is asking, what they see
  web_dist/               built frontend, committed so `mtglab ui` needs no Node
  cli.py
web/                      frontend source (React + Vite); `npm test` is Vitest
decks/<slug>/deck.yaml    SOURCE OF TRUTH
decks/<slug>/artifacts/   GENERATED — never edit by hand
Dockerfile                two stages, no Node; app runs non-root
docker-entrypoint.sh      fixes the volume's ownership, then drops privileges
fly.toml                  the only Fly-specific file; no secrets, ever
```

Deployed, **decks live on the volume at `/data/decks`, not in the image** — the
app's editing routes write `deck.yaml`, so decks baked into a layer would lose
every edit at the next deploy. The image carries them at `/app/decks-seed` and
`docs/HOSTING.md` §4 step 6 copies them across once. The corpus is never in the
image at all.

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
source. Not a hash of `deck.yaml`: card facts come from the corpus, so a
`data refresh` can change a simulation while the deck file does not move. A hit
is a job that was born `done`, so no client changed shape, and every result now
carries `seed`, `cached` and `computed_at` — **quote a cached number as
cached.** Runs are seeded by default (`simruns.DEFAULT_SEED`); an unseeded
sample was what the app used to show and is not reproducible. Land sweeps cache
per count, so an overlapping range reuses rows. `mtglab sim cache [--clear]`.

Keep `mana.py` and `sim/` dependency-light (stdlib + numpy). DuckDB stays
behind `cards/db.py`. `sim/cache.py` imports `auth/db.py` for one reason and it
is not auth: that module is the `app.db` connection helper, and a second
migration ladder for the same file would be worse than the import. That boundary is what keeps the simulation core fast to
test: the solver, the gate and the simulator all take plain records, so most of
the suite needs no database at all.

The tests that *do* need one build it. `tests/tiny_corpus.py` loads 21 real
cards into a scratch DuckDB in about a second, and `mono_green_deck()` is a
legal 99 built only from those cards. That is what the card-fact tests use —
swap, add, suggestions, search, the Tier 1 endpoints, the Claude tools. It is
**not** the ~500MB Scryfall corpus, which stays out of git and out of CI
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

**5. Never commit** card corpus data, collection/wishlist/purchase data, or
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
mtglab decks swap <slug> --out X --in Y --why '...'   # apply your choice
mtglab sim mana <slug>            # baseline consistency
mtglab sim lands <slug> 30 40     # is the land count right?
mtglab sim cache                  # what Tier 1 results are memoised; --clear
mtglab sim forge <a> <b> [c] [d]  # Tier 3 — Forge plays real games
git commit -am "before refactor"  # so swaps.md has something to diff
```

Editing, all surgical and self-verifying ([ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md)),
each also a route and a control on the deck page:

```bash
mtglab decks add <slug> --card X --category ramp --why '...'  # corpus-checked
mtglab decks remove <slug> --card X
mtglab decks set <slug> --card X --why '...'        # or --category / --qty
mtglab decks set <slug> --status built              # no --card: a deck field
mtglab decks note <slug> --key mulligan --value '...'
mtglab decks promote <slug>       # draft -> curated, once every card is justified
mtglab decks delete <slug>        # confirm by typing the slug; moves to decks/.trash/
mtglab decks build <slug> --against <(git show HEAD:decks/<slug>/deck.yaml)
```

`swaps.md` is a **git diff**. Commit before editing or you won't get one.

## Python decides, Claude advises

The split, decided 2026-08-11 and argued in
[ADR 14](docs/adr/0014-python-decides-claude-advises.md): **anything with a
right answer belongs in deterministic Python; Claude is for opinions and
research.**

**Started, not finished.** `src/mtglab/claude/` is the pipe — a client on
`ANTHROPIC_API_KEY` and seven read-only tool schemas over `api/service.py` —
plus the stance (`stance.py`, three axes, off by default) and one mode: the
**rationale interview** (`modes.py`, `interview.py`), which asks about a card's
slot so you can write its `why`. `mtglab claude check` proves the key;
`mtglab claude interview <slug> --card X` runs the mode, and the deck page runs
it beside the rationale box. The other three modes ADR 15 names, the activity
log, and any UI for the stance dial do not exist — check what is actually there
before assuming either way.

[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md) says
what a surface *is*: a **mode** (a system prompt, a tool set, and what it may
write) plus a **stance** (the user's dial over initiative, scope, and write
autonomy). A stance may widen what a mode does, never what it is allowed to do.
Card facts reach a mode through corpus tools rather than recall, which is how
rule 1 below becomes structural instead of a request. Target model is
**Claude Sonnet 5** to begin with — the user's call, not a default to override;
**load the `claude-api` skill before writing any integration code.**

Deterministic Python owns legality, colour identity, singleton, deck size,
companion and partner rules, mana solving, Tier 1, category counts, similarity
and price. Reproducible, tested without a network, no model consulted. Claude
owns conversation about a deck and the questions the corpus cannot answer — the
meta, whether a spoiled card earns a slot, what a ruling means in practice.

Three boundaries, all of which apply to you in this session as much as to
anything built later:

1. **Rule 1 binds Claude too.** Card facts come from the corpus, not from
   recall and not from a web page. Research is for what the corpus lacks —
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
- Deep cuts from old Magic are actively wanted. Query the whole corpus.
- Price is not usually an object, but prefer the cheaper option when a genuine
  functional equivalent exists.
- Reserved List is allowed or forbidden **per deck** — check the deck file.
- Every bug fix gets a test. `mana.py` is subtle; `tests/test_mana.py` pins the
  cases where naive source-counting gives the wrong answer.
- `ruff check src tests` and `mypy` before pushing. mypy is strict by default
  with ten named exceptions in `pyproject.toml`; that list is meant to shrink,
  so a new module is strict from the day it is written.
- Frontend: `npx tsc -b`, `npx oxlint --deny-warnings` and `npm test` in
  `web/`, then rebuild the committed bundle with `npm --prefix web run build`
  if anything under `web/src` changed. CI checks all four.

## Landing work

The repo is public and `main` is protected: pull request required, **all five**
CI checks green, branch up to date, enforced for admins. A direct push to
`main` is rejected — branch first, then open a PR. Squash merge; linear history
is required.

The fifth is `image`, added 2026-08-12 with containerisation. **It cannot be
run locally** — this Mac is macOS 12 on Intel, where Docker Desktop will not
install and Homebrew is too stale to build Colima, so CI is the only place the
`Dockerfile` is ever built. Treat a red `image` job as the first real feedback
on a container change rather than as a surprise.

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

All six are migrated into `decks/<slug>/deck.yaml`, which is now the only
source of truth — the original markdown in `~/Downloads` is historical and
should not be edited or re-imported. `ROADMAP.md` records what the migration
turned up.

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
