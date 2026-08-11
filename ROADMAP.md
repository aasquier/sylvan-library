# Roadmap

The original goals, what actually works today, and what comes next. This file
is the durable plan — it survives a fresh session, unlike a conversation.

Status keys: **done** · **partial** · **not started**

---

## 1. Analyse or generate decks with simulation

| Sub-goal | Status | Where |
| --- | --- | --- |
| Mana base analysis | **done** | `sim/tier1/engine.py`, `mtglab sim mana` |
| Commander strategy / speed to online | **done** | commander-by-turn curve, median turn |
| Macro categories covered | **done** | `decks/analyze.py` — counts vs bracket targets |
| Colour identity confirmation | **done** | `decks/validate.py`, from Scryfall `color_identity` |
| Deep hits from all of Magic | **partial** | `mtglab ui` card search queries all 35k oracle cards; no "suggest for this deck" scoring yet |
| Best-in-slot alternatives | **not started** | needs a similarity/upgrade scorer |
| Upcoming spoilers for new decks | **partial** | `GET /api/sets/upcoming` is live; no card-level scan |
| Frugal alternatives | **partial** | price data loaded, shown in search; no "cheaper equivalent" logic |
| Pod simulation of real games | **not started** | Tier 2 |

Generating a deck from scratch is **not started**. Everything so far analyses a
list you supply.

## 2. Adversarial simulation between decks

**Not started.** Needs Tier 2 (pod simulator): four seats, each deck compiled to
a policy profile, archetype opponents, round-robin. This is also what produces
goal 7's tier list.

Opponent decks sourced from EDHREC/Moxfield/Archidekt is an **open decision** —
see below.

## 3. Play against Claude in a local UI

**Partial.** The UI exists (`mtglab ui`) with real Scryfall art, but there is no
play mode. Needs a board-state manager and Claude in an opponent seat reasoning
over board JSON. Explicitly *not* a rules engine — building one is what took
Forge and XMage a decade each.

## 4. Shopping, swaps, deals

**Partial.** `mtglab price deck` works and 107k printings with prices are
loaded, plus a `price_history` table for deal detection. No `deals` command, no
cart generation, no wishlist.

Hard boundary, unchanged: never enters payment details, never completes a
purchase. Carts are staged for a human to confirm.

## 5. Five artifacts per deck or refactor

**done** — `artifacts/generate.py`, via `mtglab decks build <slug>`:
`primer-quick.md`, `primer-advanced.md`, `decklist-annotated.md`,
`moxfield.txt`, and `swaps.md` when something changed.

Run for the four decks that pass the gate; Goreclaw and Atla Palani are
blocked on their banned card. `swaps.md` is the exception — it is a git diff,
so it only appears once a deck changes against a committed baseline, which has
not happened yet.

## 6. Scan upcoming sets against curated decks

**Partial.** Upcoming set list is live from Scryfall. The card-level scan —
pull spoiled cards, filter to each deck's identity, score against current
slots — is not built.

## 7. Tier list of curated decks

**Not started.** All six decks are migrated now, so the remaining blocker is
Tier 2.

---

## Deck migration status

`decks/<slug>/deck.yaml` is the source of truth. The original markdown in
`~/Downloads` is historical — it should not be edited or re-imported, and
several of its claims were wrong (see below).

All six are now in `deck.yaml`. Four validate clean; two are blocked on a
single card each, both genuinely banned in Commander.

| Deck | Colours | Status | Source |
| --- | --- | --- | --- |
| Gyome, Master Chef — Food | Golgari, B4 | **migrated**, 0 errors | `02-the-99-annotated_1.md` |
| Arahbo — Cats | Selesnya (Kaheera companion) | **migrated**, 0 errors | `arahbo-cats-decklist_4.md` |
| Trostani, Selesnya's Voice — Tokens | Selesnya, B4 | **migrated**, 0 errors | `trostani-tokens-FINAL-decklist_2.md` |
| Tivit — cEDH | Esper, B5 | **migrated**, 0 errors / 1 warn | `tivit-cedh-bracket5.md` |
| Atla Palani — Dinos | Naya, B4 | **migrated**, 1 error — see below | `Atla-Palani-FINAL-Decklist.txt` + annotated |
| Goreclaw — Mono-green stompy | Green, B4 | **migrated**, 1 error — see below | `goreclaw-mono-green-stompy_2.md` |

### Open: two banned cards need a replacement chosen

Both confirmed against Scryfall `legalities.commander`, on a corpus current to
2026-11-20. Neither is a transcription slip; both lists genuinely contain them.

- **Goreclaw** runs **Primeval Titan** (banned). The slot it fills is "6/6
  trample, fetches two lands on ETB and on attack — ramp and threat in one
  card."
- **Atla Palani** runs **Emrakul, the Aeons Torn** (banned). She was the top of
  the titan module: 15/15 flying, annihilator 6, protection from coloured
  spells.

Until a replacement is picked, both decks sit at 99 cards with one illegal
slot and the gate blocks artifact generation. This is the gate working, not a
bug to route around.

### What the migration turned up

Beyond the two bans, the gate and a corpus cross-check caught:

- `Captain America's Aid` in the Arahbo list is not a card. The source's own
  parenthetical, **Sigarda's Aid**, is the real name.
- The Arahbo source describes the {1}{G}{W} doubling as something you activate
  with spare mana. Oracle text is *"Whenever another Cat you control attacks,
  you may pay {1}{G}{W}"* — a triggered ability, once per attacking Cat, so six
  mana doubles two attackers. Neither of Arahbo's abilities can target himself.
- The Arahbo source's curve (8/15/17/14/6/3, avg 3.06) was counted by hand and
  is wrong. From the corpus it is **9/13/20/13/4/4, avg 3.03**.
- The Arahbo source claims the commander is castable by T5 in 67% of games.
  `sim mana` at 20,000 games says **57.2%**.
- Kaheera's companion condition was **not checked by the gate** — verified by
  hand at the time (all 27 creature cards are Cats). **Now fixed**, see below.

Every one of these was a checkable fact that prose got wrong, which is the
same lesson as the section below.

### Companion restrictions are now enforced

`decks/companion.py` checks the deckbuilding restriction itself, not just that
the named card has a Companion ability. All **10 commander-legal companions**
are covered:

| Companion | Restriction | Check |
| --- | --- | --- |
| Gyruda | even mana values | exact |
| Obosh | odd mana values, lands exempt | exact |
| Keruga | mana value 3+, lands exempt | exact |
| Lurrus | permanents mana value 2 or less | exact |
| Kaheera | creature types, read from her own oracle text | exact |
| Jegantha | no repeated mana symbol in a cost | exact |
| Lutri | nonland names all different | exact |
| Umori | nonland cards share a card type | exact |
| Yorion | deck size +20, wired into the size check | exact |
| Zirda | permanents have an activated ability | **heuristic → warning** |

Three further companions exist in the corpus and are *deliberately* reported as
unchecked: Lutri, Pauper Otter; Treizeci, Sun of Serra; and The Companion of
the Wilds. Their conditions reference expansion symbols, retro frames and
specific sets — properties of a *printing*, not of an oracle card. None is
legal in Commander, so none can legitimately appear anyway.

The design rule: **an unevaluated restriction warns loudly and is never
reported as satisfied.** An unrecognised companion produces
`companion-unchecked` rather than a silent pass. Zirda's activated-ability test
is a colon-plus-keyword heuristic, so it reports at warning level rather than
blocking generation on a guess.

The same pass closed three other holes the companion had: it was never checked
for **Commander legality**, never checked for **colour identity** against the
commander, and never checked for being **listed in the 99** as well. Also,
`is_companion` now tests the `Companion —` ability marker rather than the mere
presence of the word "companion", which appears in ordinary rules text.

Note "your starting deck" includes your commander, so the commander is part of
the check — Arahbo, Roar of the World is a Cat Avatar, so the cats list stays
legal.

### Two-commander pairings are enforced too

`decks/partners.py` covers every way a deck can have two commanders. The gate
previously assumed one, and was wrong about legal decks in three ways:

- **A Background was rejected outright.** It is a `Legendary Enchantment —
  Background` whose text never says it can be your commander, so Jaheira +
  Raised by Giants failed with `not-a-commander`.
- **Deck size was always 99.** Two commanders share the command zone, so the
  deck holds **98**. Any legal partner deck failed `deck-size`. Note the
  contrast with a companion, which is "effectively a 101st card" and therefore
  does *not* change the deck size — commanders are inside the 100, companions
  are not.

**A rule I got wrong first time, recorded so nobody repeats it.** Battlebond
printed ten *non-legendary* creatures with `Partner with` (Lore Weaver, Ley
Weaver, Chakram Slinger and friends) for Two-Headed Giant limited. I assumed
the ability granted commander eligibility the way "Choose a Background" does.
It does not — the official ruling on those cards is blunt: *"A nonlegendary
creature can't be your commander, even if it has a 'partner with' ability."*
The gate now rejects them and says exactly that, because "does not say it can
be your commander" reads like a data problem rather than a rule. The Background
exemption is the only real one: it is legal despite not being a creature
because "Choose a Background" makes it a second commander.

Mechanics covered, all enumerated from the corpus: plain **Partner**, **Partner
with `<name>`** (pairs only with that card), **Partner—`<label>`**, **Choose a
Background** + **Background**, and **Doctor's companion** + a `Time Lord
Doctor`. `Partner—<label>` is a generalised template and the corpus already
carries four labels — Friends forever, Survivors, Character select, Father &
son — so the check matches on the label rather than hardcoding one, and a new
set adds labels for free.

Also added: more than two commanders is an error, and an illegal pairing says
precisely why ("Lore Weaver has Partner with Ley Weaver, so it can only pair
with that card") rather than just refusing.

---

> **Next phase:** [docs/ENGINEERING.md](docs/ENGINEERING.md) — compiled
> backend (Tier 2 in Rust, with the measurements for why *not* Tier 1),
> property-based and differential testing, container hardening, and Claude
> review on PRs. Written for the case where peers review this repo.

## Suggested order

1. **Migrate the remaining decks.** Highest value: the Library screen is built
   for a shelf, a tier list is meaningless with one deck, and each migration
   exercises the gate against a new list.
2. **Play mode in the UI** — the fun one, and it needs no new engine work
   beyond board state.
3. **Tier 2 pod simulator** — unlocks adversarial sims and the tier list
   together.
4. **Spoiler scan** and **deals/carts** — both self-contained.

---

## Open decisions

### Hosting — plan

> Full maintainer setup guide, auth design, per-user data model and measured
> compute analysis now live in [docs/HOSTING.md](docs/HOSTING.md). Summary below.

Wanted: follow along remotely, and eventually point friends at it. Budget is
not the binding constraint; a few dollars a month is fine.

**The constraint is data and CPU, not code.** The corpus is ~63 MB of DuckDB
built from ~98 MB of compressed Scryfall bulk, gitignored on purpose — Scryfall
asks that bulk data not be redistributed, and it is re-downloadable in one
command. And Tier 1 is genuinely CPU-bound: `sim mana` at 20,000 games is ~30s,
a land sweep is ~5 minutes. That rules out most serverless platforms on two
counts (no persistent disk for the DB, and request timeouts far below a sweep).

Three things follow, and they decide the shortlist:

1. **Persistent disk is required.** Rebuilding a 63 MB DuckDB from a ~500 MB
   download on every cold start is unacceptable, so the platform must keep a
   volume between restarts. This is the single hardest filter.
2. **`data refresh` is a scheduled job, never a build step.** It needs several
   minutes and blows any build budget. Run it weekly by cron against the
   volume — Scryfall publishes daily, but deck tooling does not need
   day-fresh data, and prices only matter to `price deck`.
3. **Long sims must stay off the request path.** Already true — `api/jobs.py`
   and `api/simruns.py` run them as background jobs and the UI polls. Nothing
   to change.

**Shortlist, real monthly numbers:**

| Option | Cost/mo | Why / why not |
| --- | --- | --- |
| **Fly.io** (recommended) | **~$6-8** | `shared-cpu-1x` with 1 GB RAM ≈ $5.70, plus a 3 GB volume at $0.15/GB ≈ $0.45. Persistent volumes, scale-to-zero with fast wake, scheduled Machines for the refresh cron. Best fit without running a server. |
| **Hetzner CX22 VPS** | **~€4** | 2 vCPU / 4 GB / 40 GB. By far the most CPU per euro, which is what Tier 2 will want. Cost: you own OS updates, TLS and deploys. Pick this if the simulator is the point. |
| **Railway / Render** | ~$5-7 | Simplest deploys, persistent volumes on paid tiers. Render's free tier has no persistent disk and spins down, so it is not an option here. |
| Vercel / Netlify / Workers | n/a | Frontend would be free and trivial, but the backend needs a 63 MB local DB and minutes-long CPU. Only viable split: static frontend on Cloudflare Pages (free) + API elsewhere. Not worth the extra moving part at this size. |

**Recommendation: Fly.io**, moving to a Hetzner box if Tier 2 turns out to need
real cores. 1 GB RAM is the number to watch — DuckDB plus numpy plus a 25,000
game sweep is the memory high-water mark, and 512 MB is too tight.

**Constraints the deployment has to respect:**

- **Fan Content Policy is noncommercial.** Whatever this runs on stays free to
  use — no ads, no subscription, no donations tied to it. The disclaimer is
  already in the UI footer and must stay.
- **Do not redistribute Scryfall bulk data.** The instance downloads its own
  copy; the volume is not a public mirror. Keep hot-linking card images from
  `cards.scryfall.io` rather than proxying or rehosting them, send a
  descriptive User-Agent, and keep the request rate polite.
- **Put auth in front before any collection feature ships.** The app has no
  auth today, which is fine for decks and public card data. But CLAUDE.md rule
  5 exists because a public inventory of expensive cards tied to a real
  identity is a targeting list — and that reasoning does not stop at `git`.
  Cloudflare Access is free for up to 50 users and needs no application
  changes; that is the cheapest way to let friends in without opening it to
  everyone.

**Not yet done:** there is no Dockerfile, no `fly.toml`, and no refresh cron.
Nothing in the architecture blocks any of it — the API is a normal FastAPI app
and the frontend is prebuilt static files served by it.

### Rust or Go for the simulation core

Measured on this machine: `sim mana` at 20,000 games takes ~30s; a land sweep
across 11 counts at 25,000 games each takes ~5 minutes. Tier 1 is tolerable.

**Tier 2 is where this decides itself.** A pod simulator is four seats making
real decisions over more turns — plausibly 50-100x the work per game. If Tier 2
in Python turns out to take minutes per matchup, the inner loop moves to a
compiled language and the rest stays Python.

Do not port Tier 1 pre-emptively. `mana.py` and `sim/tier1/` are deliberately
stdlib-plus-numpy precisely so they *could* move later; the boundary already
exists. Measure Tier 2 first.

### Reaching outside Scryfall

CLAUDE.md currently forbids marketplace scraping and purchase automation. Goal
2 wants opponent decklists from EDHREC/Moxfield/Archidekt, and goal 4 mentions
acting as a user on TCGplayer.

Unresolved. Note that a shared repo spreads whatever is chosen to everyone who
runs it, and that entering payment details or completing a purchase stays off
the table regardless.

---

## What is solid underneath

Worth knowing before trusting any number: several bugs found this session were
producing confident, wrong answers for *every* deck, not just one.

- `qty` was ignored when compiling a deck for simulation, so a 99-card deck
  simulated as ~83 cards with 20 lands instead of 34.
- Tapland detection matched Scryfall's old wording, so every modern tapland
  compiled as untapped.
- Land-fetch ramp compiled to blank cards.
- `get_cards` matched only Scryfall's combined `Front // Back` name, so every
  modal DFC and adventure card was reported as unknown.
- **`is_land` tested `"Land" in type_line`** against Scryfall's *combined*
  type line, so every card whose **back** face is a land counted as a land.
  Tier 1 uses `is_land` to decide what a land is, so Trostani simulated with
  **37 lands instead of 35** (Ojer Taq, Growing Rites of Itlimoc) and Atla with
  37 instead of 36 (Welcome to . . .) — wrong mulligan rates and a wrong
  land-count recommendation, from decks that looked fine. Fixed by reading the
  front face and consulting Scryfall's `layout`: a `modal_dfc` lets you choose
  which face to play, so a land back face is a real land drop; a `transform`
  permanent is cast as its front face and the back only ever arrives by
  flipping.

All five are fixed and pinned by tests. The lesson worth keeping: logic in
tested code gets caught, logic in conversation does not. 216 tests, CI runs
them on 3.11 and 3.12, typechecks and builds the frontend, and fails if the
committed bundle drifts from source.

Two smaller fixes from the same pass: the card-search text input was
`flex-1` with the default `basis-0` in a wrapping row, so it collapsed to
~14px next to the fixed-width selects; and `GET /api/decks` now carries the
gate's error and warning counts so the Library can flag a deck that does not
validate, instead of rendering a banned card exactly like a clean list.
