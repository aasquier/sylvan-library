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
| Best-in-slot alternatives | **partial** | `decks/suggest.py` scores similarity; `mtglab decks suggest <slug>` and `GET /api/decks/{slug}/suggestions`. Aimed at the gate's offenders; `--card` points it anywhere |
| Upcoming spoilers for new decks | **partial** | `GET /api/sets/upcoming` is live; no card-level scan |
| Frugal alternatives | **partial** | price data loaded, shown in search; no "cheaper equivalent" logic |
| Pod simulation of real games | **not started** | Tier 3 (Forge) first; Tier 2 deferred behind it |

**Importing a list now works** — `mtglab decks import <slug> --from <file|->`,
`POST /api/decks/import`, and the app's Import page. It resolves names against
the corpus, files lands and nothing else, and writes a `stage: draft` deck with
an empty `why` on every card. Generating a deck from scratch is still not
started, though import subsumes most of it. See **The deck lifecycle** below.

## 2. Adversarial simulation between decks

**Not started, and re-sequenced on 2026-08-11.** This is also what produces
goal 7's tier list.

Two simulators could answer it, and they answer different questions. **Tier 2**
(the Python pod simulator) is four seats, each deck compiled to a policy
profile, archetype opponents, round-robin — a *statistical* model of Magic, not
a rules engine, right for bracket placement and matchup matrices and wrong for
"is this line correct". **Tier 3** is the Forge bridge: real games, a real rules
engine, a real AI, and card coverage that stops short of the whole format.

**Forge goes first, and Tier 2 waits behind it.** Tier 2 is a large build whose
output is a model whose fidelity nobody has yet had to defend; Forge is an
integration with an engine that already plays the game. If Forge turns out to
answer the bracket and matchup questions well enough, Tier 2 may never need
building — and if it does not, its measurements will say exactly what Tier 2
has to be better at. That is a cheaper way to find out than building the
simulator first.

Opponent decks sourced from EDHREC/Moxfield/Archidekt is an **open decision** —
see below.

## 3. Play real games against a real engine

**Partial, and re-aimed on 2026-08-11.** The UI exists (`mtglab ui`) with real
Scryfall art, but there is no play mode.

This goal used to read "play against Claude", with Claude in an opponent seat
reasoning over board JSON. That is the wrong shape and
[ADR 14](docs/adr/0014-python-decides-claude-advises.md) retires it: a language
model handed a board state is neither a rules engine nor a strong player, and
the one thing it would add over an engine — coverage of cards the engine does
not implement — is exactly where its answers would be least checkable.

**Forge plays the games instead.** It is a real rules engine with a real AI,
which is a decade of work this project is not going to repeat, and `CLAUDE.md`
already specifies how its results must be reported. That makes this goal the
Tier 3 bridge rather than a separate build.

The remaining work is a board-state manager for the UI — still explicitly *not*
a rules engine — plus the Forge bridge itself. **Whether Forge can be reached
from a hosted instance at all is an open decision**; see below.

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

**Not started.** All six decks are migrated now, so the remaining blocker is a
simulator that plays decks against each other — Forge first, per goal 2.

Note the caveat this inherits: `CLAUDE.md` requires Forge results to be reported
**per archetype, never as a single ranking**, because Forge's AI is good with
aggro and midrange and poor with control and most combo. Aaron's decks sit right
on that fault line — Dino and Cat are what Forge plays well, Tivit and Gyome are
what it plays badly. A tier list built from Forge output without that split
would be a confident ranking of how well Forge plays each deck, which is not the
question.

---

## Deck migration status

`decks/<slug>/deck.yaml` is the source of truth. The original markdown in
`~/Downloads` is historical — it should not be edited or re-imported, and
several of its claims were wrong (see below).

All six are now in `deck.yaml`. Four validate clean; two are blocked on a
single card each, both genuinely banned in Commander.

Separately from the gate, each deck declares whether it physically exists:
**Goreclaw and Tivit are `theoretical`** — lists Aaron is thinking about — and
the other four are `built`. The field defaults to `theoretical` when absent,
because a wrong "built" sends someone to a shelf with no deck on it while a
wrong "theoretical" costs nothing. The library filters and badges on it.

| Deck | Colours | Gate | Source |
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

**There is now a shortlist to argue with.** `mtglab decks suggest <slug>`, and
the same thing under the error on the deck page's validation tab, ranks legal
cards by measurable similarity to the one being removed — card type, mana
value, Scryfall's keywords, oracle text, with EDHREC rank only as a tiebreak.
It reports; it never edits. The decision is still yours, which is
[ADR 8](docs/adr/0008-the-gate-blocks.md) unchanged.

What it currently surfaces, for the record:

- **Goreclaw / Primeval Titan** — Regal Behemoth, **Cultivator Colossus**,
  Soul of the Harvest, Earthshaker Dreadmaw, Gruff Triplets.
- **Atla Palani / Emrakul, the Aeons Torn** — Earthquake Dragon,
  **Emrakul, the Promised End**, Autochthon Wurm, Draco.

Worth knowing before trusting the order: the scorer ranked Regal Behemoth above
Cultivator Colossus purely because mana value 7 vs 6 costs it on the curve
term, and Colossus is the closer fit to "fetches lands". Similarity is not
quality, the top of the list is not the answer, and tuning the weights until
one deck's preferred card comes first would be overfitting to a sample of one.

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

> **Next phase:** [docs/ENGINEERING.md](docs/ENGINEERING.md) — property-based
> and differential testing (**done**, §2), then ADRs, a deck-source seam,
> frontend tests and container hardening. A compiled rewrite is **deferred with
> a written trigger**; the measurements say Tier 1 would gain nothing, and Tier
> 2 gets built in Python and profiled before that call is re-made.

## The deck lifecycle

Planned 2026-08-11. Steps 1, 2 and 3 shipped the same day; 4 is next. Design
decisions live in
[ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md) (how a deck
is edited) and [ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md) (what an
imported deck is). This section is the order of work.

### Where it stands

| Path | Today | Wanted |
| --- | --- | --- |
| **Create** | copy `decks/_template/deck.yaml` by hand, or import a list with a commander and nothing else | a UI form; the machinery is import's |
| **Import** | **done** — `mtglab decks import`, `POST /api/decks/import`, the Import page | — |
| **Refactor** | **done** — add, remove, recategorise, requantify, rationalise and annotate, from the CLI or the deck page | — |
| **Promote** | hand-edit `stage: curated` once the rationales are written | the last hand-edit left in the loop; see below |
| **Export** | `moxfield.txt`, one of the five artifacts | unchanged; it already works |

### Order, and why

1. **Import.** ✅ **Done.** Highest value and it subsumes create — a new deck is
   an import of an empty list plus a commander. It is also the only one of the
   three that someone other than the maintainer needs on day one.

   `decks/decklist.py` is the grammar: quantities, set codes and collector
   numbers, `*CMDR*` and foil markers, Archidekt's `[Category]` annotations,
   Deckstats' `//Section` headers and leading `[SET]` codes, and section
   headers including card-type groupings. It is pure text → structure, so it
   tests exhaustively without a database. A line it cannot read comes back with
   its line number rather than vanishing.

   `decks/importer.py` resolves those names through `db.get_cards` — which
   already handles double-faced cards by face name — and writes the file.
   Unknown names are kept **verbatim** and reported, so the deck stays the size
   you pasted and the gate flags them as `unknown-card`; dropping them would
   hand back a 96-card deck silently. Category is inferred only for lands,
   because `is_land` is a corpus fact that is right about the double-faced
   cards a type line is wrong about. A sideboard or maybeboard becomes the swap
   board. Import refuses without a corpus rather than producing a deck whose
   facts were never checked.

   Two things it deliberately will not do: pick a commander when the list does
   not name one (it reports the candidates, including the sideboard Moxfield
   hides it in), and assume a card with a Companion ability is *this deck's*
   companion.

2. **The draft stage in the gate.** ✅ **Done.** `stage: draft | curated`,
   defaulting to curated so the six existing decks are never demoted. In a
   draft a missing `why` is a warning; in a curated deck it blocks. Promotion is
   refused while any card is blank, and `decks build` refuses a draft outright —
   not something `--force` overrides, because a draft is not *wrong*, it is
   unfinished, and the way out is to write the rationales.

   One thing changed shape in the building. A draft's missing rationales report
   as **one counted warning**, not one per card: 99 identical warnings is the
   wall ADR 13 set out to replace, and it buried the banned card the same run
   was meant to surface. The per-card list lives in the deck file (a blank
   `why:` on every line) and on the deck page.

3. **The rest of the edit operations.** ✅ **Done.** `add_card`, `remove_card`,
   `set_card_field`, `set_note`, each surgical and self-verifying per ADR 12,
   reachable from `mtglab decks add|remove|set|note`, four endpoints, and the
   deck page. Writing a rationale no longer means opening a text editor.

   Two things came out of the building. **Every operation now proves itself
   against an oracle**: it computes the document it ought to produce by mutating
   the parse — an ordinary dict — and refuses to return text that does not read
   back as exactly that. The naive parse-mutate-dump is used as the oracle it is
   good at being while the text surgery does the writing, which is the same move
   as [ADR 10](docs/adr/0010-correctness-against-independent-oracles.md). It
   earned its keep immediately: it caught that removing the last card from a
   list leaves `swap_board:` parsing as `None` rather than `[]`, which
   `Deck.from_text` would have iterated.

   And **insertion is category-aware**, because the deck files are grouped under
   section banners (`# ---- RAMP 14`). Appending a land to the end of the list
   would file it under whichever banner came last, so a new card goes after the
   last entry already in its category, and the banners — with the blank lines
   above them — are never inside any edit's reach.

4. **A create path in the UI**, once import and edit both exist, since it is the
   same machinery with an empty list.

5. **Promotion**, which step 3 turned into the last hand-edit in the loop. You
   can now fill in all 99 rationales in the app and are then told to open
   `deck.yaml` and change `stage: draft` to `curated`. That wants a
   `set_deck_field` operation — a fifth one, not in ADR 12's table, which is why
   it was left out rather than added quietly. It is small, and the gate already
   owns the hard part: it refuses the promotion while any card is blank.

### The question this settles

Rule 4 says every card carries a `why` or the gate fails. Import produces 99
cards with none. Generating rationales was rejected outright — a `why` written
by the tool is precisely the empty justification the rule exists to prevent — so
the answer is that an imported deck is honestly incomplete, says so, and counts
what it still owes. See ADR 13 for the full argument, including why `stage` is a
second field rather than another value of `status`.

---

## What Claude is for

Decided 2026-08-11 and recorded as
[ADR 14](docs/adr/0014-python-decides-claude-advises.md). Nothing is built:
there is no LLM SDK in `pyproject.toml`, and the only `ANTHROPIC_API_KEY` in
the repository belongs to the CI reviewer in `docs/ENGINEERING.md`.

**Python decides. Claude advises. Forge plays the games.** The split is by
whether the question has a right answer:

| | Owned by | Because |
| --- | --- | --- |
| Legality, colour identity, singleton, size, companion and partner rules | Deterministic Python | There is a correct answer and it must be the same tomorrow |
| Mana solving, Tier 1, category counts, similarity, price | Deterministic Python | Same — reproducible, tested without a network |
| The meta, whether a spoiled card earns a slot, what a ruling means in practice, whether a plan holds together | Claude | No corpus query answers these; they need an opinion or the open internet |
| Playing actual games | Forge | A real rules engine with a real AI, which took a decade to build |

### The three boundaries

1. **Rule 1 still binds Claude.** Card facts come from the corpus — not from
   the model's recall, and not from a web page. Research is for what the corpus
   does not contain: discussion, meta, rulings, cards spoiled ahead of the next
   bulk refresh.
2. **Claude may argue about a `why`; it may not write one.** It can
   interrogate, challenge and make the case against a card's slot — that is the
   conversation the curated six came out of. It must not author the text that
   lands in `deck.yaml`, and no surface may pre-fill that field. An
   edit-before-save gate was considered and rejected: it adds a click to the
   same failure.
3. **Provenance is always visible.** A user must be able to tell without asking
   whether an answer is the gate's (reproducible) or Claude's (an opinion).

### Modes, decided 2026-08-11

A Claude surface is a **mode**: a system prompt, a tool set, and — the part that
is code rather than prose — a declaration of what it may write.
[ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md) has the
argument. Four are worth building first, and every one of them may write
**nothing**:

| Mode | What it is for |
| --- | --- |
| Rationale interview | asks about a card so the user can write its `why`; import leaves 99 of them owing |
| Argue a slot | the case against a specific card, from corpus facts and category counts |
| Deck conversation | anything about a deck, with the gate's output and the corpus in reach |
| Research | the meta, rulings in practice, cards spoiled ahead of the next bulk refresh |

The interview is the mode that made this worth settling before writing code.
"Claude asks, the user answers, the answer lands in `why`" breaks no rule — the
keystrokes are the user's. "Tidy that up" is one button away and is a
machine-written rationale. So the boundary is drawn where it can be tested
rather than promised: **no code path passes a model response into the `why`
field**, and a mode may put a question beside the box but never text inside it.

### What building it looks like

The natural home is `api/service.py` — it is already the seam both the CLI and
the app call through, so there is nothing to prepare. The modes' tools are
functions that already exist there and in `cards/db.py`: `get_cards`,
`search_cards`, `validate`, `suggest`, `deck_stats`. That is also how rule 1 is
enforced structurally rather than by asking the model nicely — a mode that needs
to know what a card does calls the corpus and the tool result is the fact.

Research uses Anthropic's server-side web tooling rather than a crawler this
project maintains, which keeps `CLAUDE.md`'s no-scraping rule intact.

The rationale editor built in step 3 of the deck lifecycle is already the right
shape for the interview: the box sits beside a column showing the card as the
corpus has it, which is where a mode's questions go.

Two things to settle before it ships, both open decisions above: **what a hosted
Claude surface costs and who pays**, and — for the simulator half — **whether
Forge can run where the app runs**.

---

## Suggested order

1. **The rest of the deck lifecycle.** ✅ **Done 2026-08-11.** `add_card`,
   `remove_card`, `set_card_field`, `set_note` (ADR 12), and the rationale
   editor. What remains of the lifecycle is the create form and promotion.
2. **The Claude surface** — the modes in
   [ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md),
   starting with the rationale interview, which now has somewhere to put its
   output. Moved ahead of Forge on 2026-08-11: it is what makes the app useful
   for judgement rather than facts, and shipping the toolkit to someone else
   without it hands them a gate and a goldfish sim with no opinion in them.
   Local runs on the maintainer's own key; the hosted question stays open.
3. **Forge feasibility research** — can `forge.jar sim` be driven from here at
   all, and can it be reached from a hosted instance? Cheap to answer, and it
   gates goals 2, 3 and 7 together. See the open decision below.
4. **Spoiler scan** and **deals/carts** — both self-contained.

**Tier 2 is deliberately not on this list.** It waits behind Forge (goal 2).

---

## Open decisions

### Can Forge run where the app runs?

**Open, recorded 2026-08-11, and it gates goals 2, 3 and 7.**
[ADR 14](docs/adr/0014-python-decides-claude-advises.md) makes Forge the thing
that plays games. Forge is a JVM desktop application with its own card
database, and `forge.jar sim -d ... -f commander` is a headless mode of it —
not a library, not a service.

That sits awkwardly against the hosted instance. A Fly.io image carrying a JVM
plus Forge's card data is a large image with a real per-run CPU cost, and it
lands straight back on the Fly-versus-Hetzner sizing question below.

Three shapes, none chosen:

- **Local only.** Forge simulation is something you get when running `mtglab`
  on your own machine; the hosted instance has a documented feature gap. Keeps
  the deployment small and honest, and is the smallest thing that could work.
- **Server-side.** Anyone logged in can run Forge sims. Heavier container, and
  the CPU question reopens.
- **A separate worker.** The app queues a job; something else runs Forge. Most
  flexible, most moving parts, and probably premature.

**Answer this with the feasibility spike, not by guessing.** The prior question
is whether `forge.jar sim` can be driven from Python here at all, with the card
coverage pre-flight `CLAUDE.md` requires. If it cannot, none of the above
matters.

### What a hosted Claude surface costs, and who pays

**Open, recorded 2026-08-11.** ADR 14 puts conversation and research on the
Claude API. Locally that is the maintainer's own key and own spend. Hosted, it
means **the maintainer pays for other people's questions**, and research turns
are not cheap — web search and long context are where the cost is.

That is a real constraint on "shareable with friends" and it wants deciding
before the surface opens up, not after an invoice. Options range from a
per-user budget, to bring-your-own-key, to keeping the conversational surface
local-only while the read-only app is what gets shared. Not decided.

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
2. **`data refresh` is run on demand against the volume, never as a build or
   boot step.** It needs several minutes, which blows any build budget and —
   with scale-to-zero putting boot on the request path — would turn a wake into
   an outage. Cron does not work either: Fly volumes attach to exactly one
   machine, so a scheduled second Machine cannot mount the corpus. Run it by
   hand, about monthly. Scryfall publishes daily, but deck tooling does not need
   day-fresh data, and prices only matter to `price deck`. See
   [ADR 6](docs/adr/0006-never-redistribute-scryfall-bulk-data.md).
3. **Long sims must stay off the request path.** Already true — `api/jobs.py`
   and `api/simruns.py` run them as background jobs and the UI polls. Nothing
   to change.

**Shortlist, real monthly numbers:**

| Option | Cost/mo | Why / why not |
| --- | --- | --- |
| **Fly.io** (recommended) | **~$6-8** | `shared-cpu-1x` with 1 GB RAM ≈ $5.70, plus a 3 GB volume at $0.15/GB ≈ $0.45. Persistent volumes, scale-to-zero with fast wake, scheduled Machines for the refresh cron. Best fit without running a server. |
| **Hetzner CX22 VPS** | **~€4** | 2 vCPU / 4 GB / 40 GB. By far the most CPU per euro, which is what a simulator will want — and the only one of these with room for a JVM plus Forge, if that lands server-side. Cost: you own OS updates, TLS and deploys. Pick this if the simulator is the point. |
| **Railway / Render** | ~$5-7 | Simplest deploys, persistent volumes on paid tiers. Render's free tier has no persistent disk and spins down, so it is not an option here. |
| Vercel / Netlify / Workers | n/a | Frontend would be free and trivial, but the backend needs a 63 MB local DB and minutes-long CPU. Only viable split: static frontend on Cloudflare Pages (free) + API elsewhere. Not worth the extra moving part at this size. |

**Recommendation: Fly.io**, moving to a Hetzner box if a simulator turns out to
need real cores. 1 GB RAM is the number to watch — DuckDB plus numpy plus a
25,000 game sweep is the memory high-water mark, and 512 MB is too tight.

**Forge changes this sizing question, and the answer is not yet known.** ADR 14
makes Forge the thing that plays games, and a JVM plus Forge's card database
server-side is a different class of image and a different CPU profile from
anything measured here. That is the open decision above; until the feasibility
spike answers it, this recommendation covers the app *without* server-side
Forge.

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

**A heavy simulator is where this decides itself**, and as of 2026-08-11 that
is no longer certain to be Tier 2. The trigger in
[ADR 3](docs/adr/0003-tier-1-stays-python.md) was written against Tier 2's
measurements: a pod simulator is four seats making real decisions over more
turns — plausibly 50-100x the work per game — so if it took minutes per matchup
in Python, the inner loop would move to a compiled language and the rest would
stay Python.

Tier 2 now waits behind Forge (goal 2), so **the trigger waits on whichever
simulator gets built first**. If that is Forge, the compiled-rewrite question
may not arise at all: the expensive loop would be inside a JVM this project
does not maintain, and the Python side would be orchestration and parsing. ADR
3's shape — a written, measured threshold rather than a guess — is unchanged,
which is why this re-points the trigger rather than superseding the decision.

Do not port Tier 1 pre-emptively. `mana.py` and `sim/tier1/` are deliberately
stdlib-plus-numpy precisely so they *could* move later; the boundary already
exists. Measure before porting anything.

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

- **Phyrexian mana was dropped by the cost parser**, found 2026-08-10 by the
  new property tests. `{U/P}` parsed to mana value 0 with no colours, so the
  curve in `decks/analyze.py` filed Mental Misstep as a 0-drop and Phyrexian
  Metamorph as a 3-drop, and reported Tivit's average mana value as **1.90
  instead of 1.93**. Only Tivit runs Phyrexian cards, and the generated
  artifacts do not carry the curve, so nothing shipped wrong — but the UI and
  the API did show it. The distinction the fix encodes: a Phyrexian symbol
  places no demand on the mana base, and is still a symbol with a mana value
  and a colour.

All six are fixed and pinned by tests. The lesson worth keeping: logic in
tested code gets caught, logic in conversation does not. 250 tests, CI runs
them on 3.11 and 3.12, typechecks and builds the frontend, and fails if the
committed bundle drifts from source.

Since 2026-08-10 the mana solver is also checked against two independent
reference implementations on every run, and Tier 1's seeded output is pinned to
a digest verified identical on 3.11 and 3.12 — so a change in any simulation
number is now a decision someone has to write down. See
[docs/ENGINEERING.md](docs/ENGINEERING.md) §2.

Two smaller fixes from the same pass: the card-search text input was
`flex-1` with the default `basis-0` in a wrapping row, so it collapsed to
~14px next to the fixed-width selects; and `GET /api/decks` now carries the
gate's error and warning counts so the Library can flag a deck that does not
validate, instead of rendering a banned card exactly like a clean list.
