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

**The Forge bridge exists as of 2026-08-11**: `mtglab sim forge <a> <b>` plays
real Commander games headless and reports them per archetype. `sim/tier3/`,
setup in [docs/FORGE.md](docs/FORGE.md).

The remaining work is a board-state manager for the UI — still explicitly *not*
a rules engine. **Whether Forge can be reached from a hosted instance is still
an open decision**; the spike measured what it would cost, but did not pick a
shape. See below.

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

**That caveat is now measured, not predicted.** From the spike, ten games each:

| Matchup | Result |
| --- | --- |
| Arahbo cats vs Atla Palani dinos | **10–0** cats |
| Tivit cEDH vs Atla Palani dinos | **2–8** dinos |

Tivit is the bracket 5 cEDH list — the most powerful deck of the six by a wide
margin — and Forge's AI lost with it 8–2 to a casual dinosaur deck. Any tier
list that sorted on those numbers would put the dinosaurs above the cEDH deck
and look authoritative doing it. **Keep these numbers to hand whenever a
ranking is proposed**; they are the cheapest available argument against one.

## 8. Onboarding for someone new to Commander

**Not started, recorded 2026-08-11** from Aaron's design notes. Every goal above
assumes a player who already knows what they want to build. This one does not,
and it is the goal most aligned with pointing this at friends — they will not
all be twenty-year players.

Four pieces. Three of them share one dataset, which is the main finding here.

### The colour taxonomy *is* the 32 Deck Challenge

The [32 Deck Challenge](https://archidekt.com/folders/384512) — build a deck for
every colour combination — decomposes as **1 colourless + 5 mono + 10 pairs +
10 three-colour (5 shards + 5 wedges) + 5 four-colour + 1 five-colour = 32.**
Those are exactly the nodes a colour-wheel diagram draws. So the Ravnica guild
pentagram, the Alara shards, the Tarkir wedges, the four-colour groups and
challenge progress tracking are **one dataset with several views**, not five
features:

> 32 rows keyed by colour identity → name, tier (mono/guild/shard/wedge/…),
> and a short philosophy blurb.

Progress tracking then costs nothing: group a user's decks by `color_identity`
and see which slots are empty. That field already comes from Scryfall and rule 2
already makes it the authority, so there is no new source of truth.

The pentagram's geometry is worth stating because it carries the lesson: five
vertices in WUBRG order, the five **perimeter edges** are the allied guilds, the
five **chords across the middle** are the enemy guilds. The shape teaches the
colour pie by itself.

Three things to settle before building it:

- **Four-colour naming has competing conventions.** Look it up; do not assert
  one from memory. This is a rule 1 habit applied to something that is not a
  card.
- **Mana symbols are Wizards' artwork.** Scryfall serves symbol SVGs, so the
  rule that already governs card images applies unchanged: hotlink, never
  commit, never rehost.
- **The blurbs are editorial prose** and somebody has to write them. Note this
  is *not* blocked by rule 4 — a guild's philosophy is general reference, not a
  card's `why` in a `deck.yaml` — but it is prose whose accuracy is checkable,
  so it should be checked.

### Archetype reference, and the wall it runs into

Aaron named two sources: [the MTG wiki's Archetype
page](https://mtg.fandom.com/wiki/Archetype) and [EDHREC's
themes](https://edhrec.com/tags/themes). **Neither can be ingested.** `CLAUDE.md`
bans a web crawler outright and says in as many words that server-side web
tooling "is not a way around the scraping ban". EDHREC's themes are also derived
from their own aggregated data, so their terms would need reading before any
bulk use even if we were willing.

Three options that stay inside the rules, and they compose:

1. **Link out.** Costs nothing, and a link is not a scrape.
2. **Hand-curate a small archetype taxonomy of our own**, which composes with
   the `tags` field already on `CardEntry` and with the macro categories in
   `decks/analyze.py`.
3. **Let a Claude research mode answer archetype questions live.** This is
   exactly ADR 14's half — "the questions the corpus cannot answer" — and it is
   the only one of the three that scales past what we are willing to type.

### A theme interview

*"What is your favourite zodiac sign? What historical period do you relate to?"*
→ a colour combination → a commander. This is a different product from every
tool that opens by asking which commander you have already chosen, and it is the
most differentiated idea in this section.

Mechanically it is an **ADR 15 mode** — a system prompt, a tool set, and a write
scope — and it queues behind the rationale interview, which is already next. It
also chains into the taxonomy above: theme → colours → an empty slot in the 32.

The boundary holds, and it is worth writing down why so nobody has to re-derive
it: **a theme is not a card's `why`**, so rule 4 is not engaged by asking these
questions. The moment the mode starts naming cards, **rule 1 is** — those come
from corpus tools, never from recall. And it still may not pre-fill a `why`.

## 9. Shared decks and a simulation leaderboard

**Parked 2026-08-11, deliberately and not for lack of interest.** The idea:
users opt into having their decks used in simulations, and the winners form a
leaderboard for people to play against.

**The ranking is the problem, and goal 7 above has the measurement.** Forge's AI
lost 8–2 with the bracket 5 cEDH deck against a casual dinosaur list. A
leaderboard sorted on Forge results would rank the dinosaurs higher, present the
number as authoritative, and — the part that makes it worse than a bad
statistic — **people would build toward it.** That inverts the rule `CLAUDE.md`
already sets: per archetype, never one ranking.

**Two shapes could survive**, and neither is ruled out:

- Separate boards **per archetype**, so aggro is never ranked against combo.
- One board, honestly relabelled as *"how well does Forge's AI pilot this
  deck"* — a real question, and a different one from "is this deck good".

**The prerequisite stack is the other reason to wait.** Opt-in sharing needs
auth (does not exist), a per-user data model (does not exist), consent and
withdrawal semantics for shared decks, and moderation of user content. The
leaderboard on top of that needs *continuous* simulation compute — on a
self-funded box, with four-player pod timings still unmeasured and a
134-second game already observed.

**What would unpark it — and what the measurement said.** The pod timings landed
2026-08-11 (see the Forge open decision below) and they argue *against* a
leaderboard rather than for one: a 5-game pod is ~18 minutes of near-saturated
CPU, and **40% of pod games hit the 300s clock**, so a meaningful sample would
be hours of compute producing a table where a large share of rows are the
measurement giving up. Combined with the archetype bias, a leaderboard would be
expensive *and* misleading. **Still parked**, now for a measured reason rather
than an unknown one; what remains is a decision on which of the two shapes
above is worth having at all.

## 10. Assisted refactor — swap recommendations from three sources

**Not started, recorded 2026-08-11.** The feature Aaron described: point it at a
deck and get recommended card swaps, backed by Claude's analysis, Tier 1
simulation, and possibly Forge games against control decks.

This is the first thing that would compose all three systems, and that is both
why it is valuable and where all of its difficulty is.

### What already exists

More than it looks. The mechanics of a refactor are **done** — `decks swap`,
`add`, `remove`, `set` are surgical, self-verifying, and available from the CLI,
the API and the deck page ([ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md)).
`mtglab decks suggest` already produces a ranked replacement shortlist, and
`decks/suggest.py` is explicit that it is *a measurement, not a recommendation*.
Tier 1 answers consistency and land count. Forge plays games. The stance decides
how much opinion the user wants.

**So this goal is not new machinery. It is the layer that decides what to
recommend, and the discipline about who said what.**

### Three sources, three different epistemic statuses

ADR 14's third boundary — *say which system answered* — stops being a style note
here and becomes the central design constraint. A recommendation that blends
these without labelling them is the failure mode:

| Source | What it can settle | What it cannot |
| --- | --- | --- |
| **Deterministic Python** (`suggest.py`, the gate, `mana.py`, `analyze.py`) | legality, colour identity, similarity, curve, category balance, price | whether the deck *wants* the card |
| **Tier 1** | consistency, land count, castability | anything about opponents, interaction, tutors, or card text beyond mana |
| **Forge** | whether a line actually works in a real rules engine | see the bias problem below |
| **Claude** | the meta, why a slot exists, what a card is *for*, whether a spoiler earns a place | any card fact — those come from corpus tools, never recall |

**Every recommendation must carry its provenance**, not as a footnote but as
part of the object: "the gate says this is illegal" and "Claude thinks this slot
is weak" are different claims and a user must be able to act on them
differently.

### The three problems to solve before building

1. **A swap needs a `why`, and Claude cannot write one.** This is the sharpest
   constraint and it is a feature, not an obstacle. `replace_card` requires a
   rationale unconditionally ([ADR 15](docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)),
   and no stance lifts that. So the output of an assisted refactor is a
   **proposal**, and the user accepting it *is* them writing why — which is the
   rationale interview's shape, arriving from the other direction. Build the
   interview first and this goal inherits its answer.
2. **Forge results against control decks are not a ranking.** Goal 7's measured
   numbers apply directly: Forge's AI lost 8–2 with the bracket 5 cEDH deck
   against a casual dinosaur list. A swap recommended because "Forge won more
   games with it" is a swap recommended by a judge that plays aggro well and
   combo badly. If Forge is used here at all it must be **within archetype**,
   with the caveat attached to the recommendation, and never as the deciding
   vote. Control decks would also have to be chosen and justified — an arbitrary
   gauntlet silently defines what "better" means.
3. **Tier 1 cannot see most of what a swap changes.** It models mana and
   nothing else, so it can compare two cards' effect on castability and is
   silent on everything a deckbuilder actually swaps for. Quoting a Tier 1
   delta as evidence a swap is *good* would be the most authoritative-looking
   wrong number this project could produce.

### The shape that follows

A recommendation is a **proposal object**, not a diff applied: the card out, the
candidates in, and per candidate the evidence from each source that has an
opinion, each labelled. The user picks, writes the rationale, and the existing
surgical edit applies it — so the write path is unchanged and the gate still
runs on the result.

**Sequencing:** it sits behind the rationale interview (which solves problem 1)
and behind the pod measurement (which decides whether problem 2 is answerable at
all). Nothing about it is blocked *today* except that building it before those
two would mean guessing at both.

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

Planned 2026-08-11. Steps 1, 2, 3 and 5 shipped the same day; the create form
is what is left. Design decisions live in
[ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md) (how a deck
is edited) and [ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md) (what an
imported deck is). This section is the order of work.

### Where it stands

| Path | Today | Wanted |
| --- | --- | --- |
| **Create** | copy `decks/_template/deck.yaml` by hand, or import a list with a commander and nothing else | a UI form; the machinery is import's |
| **Import** | **done** — `mtglab decks import`, `POST /api/decks/import`, the Import page | — |
| **Refactor** | **done** — add, remove, recategorise, requantify, rationalise and annotate, from the CLI or the deck page | — |
| **Promote** | **done** — `mtglab decks promote`, `PATCH /api/decks/<slug>`, a button on the deck page once nothing is outstanding | — |
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

5. **Promotion.** ✅ **Done.** `set_deck_field` — a fifth operation, not in
   ADR 12's original table — writes the deck's own scalars: `stage`, `status`
   and `bracket`. `mtglab decks promote <slug>` is the ergonomic form, and the
   deck page grows a button once nothing is outstanding.

   **Promotion is refused before the write, not after it.** The gate would catch
   a premature one either way — a curated deck reports one `missing-rationale`
   per card — but refusing up front means the deck is never written into a state
   its author has to undo, and the refusal names the cards still owing. That is
   the same shape as refusing a swap with no rationale rather than writing one
   and failing it afterwards.

   Two details the real files forced. A trailing comment survives the edit:
   `status: built  # built: the cards are sleeved up` is the author's note about
   the vocabulary, not about the value. And a key the file does not have yet —
   `stage` is absent from every deck written before ADR 13 — is inserted where
   `Deck.dump` would put it rather than appended to the bottom.

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
[ADR 14](docs/adr/0014-python-decides-claude-advises.md). **The pipe is open as
of 2026-08-11** — `src/mtglab/claude/` has the `anthropic` dependency behind a
`claude` extra, a client built off `ANTHROPIC_API_KEY`, six tool schemas over
the read-only half of `api/service.py`, and `mtglab claude check` to make one
real call and say whether the key is live. A first turn against
`claude-sonnet-5` answered correctly over live decks, calling `list_decks`,
`validate_deck` and `get_cards` unprompted.

Seven tools now, after `get_cards` was added to close a measured hole in
rule 1 — see below.

What is *not* built: modes, the stance, any UI, research through server-side
web tooling, and the Forge half.

The plumbing was already in place: an API key reaches the app from a gitignored
`.env` or `fly secrets`, named in `.env.example` and in the CI reviewer workflow
in `docs/ENGINEERING.md`, and CI fails the build on a key committed to any
tracked file.

### The hole in rule 1 — found and closed, 2026-08-11

**Closed.** Recorded here because how it was found is the transferable part.

`search_cards` filters to `legalities.commander = 'legal'`, which is correct for
finding cards to play and wrong for looking one up: **a banned card could not be
described at all.** Asked which decks fail the gate and what the flagged cards
do, the first turn got the gate's answer right and then could not look up either
Emrakul, the Aeons Torn or Primeval Titan — the two deliberate failures in
`atla-palani-dinos` and `goreclaw-stompy`. It said so and labelled the fallback
as unverified recall, which is boundary 3 working exactly as designed. It still
answered from memory, which is boundary 1 not working, on precisely the two
cards this project most needs to discuss.

`service.cards_named()` closes it: exact names through `db.get_cards`, **no
filters at all**, with `legal_commander` reported per card — so a banned card
now returns its real oracle text *and* its ban status, which is strictly more
useful than absence. Unresolved names come back in `not_found` rather than being
dropped, because a lookup that silently returns four cards for five names is how
a confident claim gets made about the fifth. Exposed as the `get_cards` tool
ADR 15's table always named.

The same turn re-run against it calls `get_cards` for both cards and quotes the
corpus. It also costs **less** — 19,130 input tokens against 25,142 — because a
model that can look a card up stops making speculative searches.

**The lesson, which is the reason this stays in the document:** the hole was
invisible from the code and obvious from one real turn. Boundary 1 is not
provable by reading the tool list. Run the surface against the awkward case —
here, the cards the library is deliberately wrong about — before trusting it.

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

### How much of it you want is yours to set

Also 2026-08-11, and it is why ADR 15 has a fourth element. Some people want a
deckbuilding tool that never speaks unless spoken to; some want the thing that
dreams up an axis they had not considered. A **stance** is the user's dial over
three axes — initiative, scope, and write autonomy — with named presets, because
"never interrupt me, but go wild when I ask" is a real setting that a single
slider cannot express. Off is a real position: no calls at all.

The stance may widen what a mode does. It may never widen what a mode is
*allowed* to do, and `why` is off limits at every position.

At the top of the write axis, Claude may apply reversible edits without asking —
git and `swaps.md` are the undo. What that turns out to permit is narrower than
it sounds, and narrowed by the editor rather than by a rule about models:

| Operation | Autonomous? |
| --- | --- |
| `remove_card`, `set_card_field` (category, qty) | yes — no rationale needed |
| `add_card` to a draft | yes — a blank `why` there is counted work |
| `add_card` to a curated deck, `replace_card` | **no** — the operation refuses a blank `why`, and Claude cannot supply one |
| `set_note` | **no** — deck prose is the same kind of thing as a `why` |

So the most attractive thing to automate, a twelve-card swap, is blocked. The
way through is the interview: Claude proposes, the user says why they accept,
and the user's sentence is the rationale. The write stops being autonomous
exactly where a human judgement enters.

Two things this adds to the build: an **activity log**, since "what did it
change while I was not looking" cannot be answered with "read the git diff" by
someone on a hosted instance, and a default that comes from the deck —
`status: built | theoretical` already separates lists under consideration from
sleeved cardboard. The stance itself starts as per-conversation state, not
persisted, so what people actually reach for is known before a default is
written into anything.

### What building it looks like

The natural home is `api/service.py` — it is already the seam both the CLI and
the app call through, so there is nothing to prepare. The modes' tools are
functions that already exist there and in `cards/db.py`: `get_cards`,
`search_cards`, `validate`, `suggest`, `deck_stats`. That is also how rule 1 is
enforced structurally rather than by asking the model nicely — a mode that needs
to know what a card does calls the corpus and the tool result is the fact.

**Done.** `mtglab.claude.tools` wraps seven: `list_decks`, `get_deck`,
`validate_deck`, `deck_stats`, `suggest_replacements`, `get_cards` and
`search_cards`. The last two are deliberately separate and their descriptions
say why — `get_cards` looks a named card up and filters on nothing;
`search_cards` finds candidates and is Commander-legal only. Treating them as
interchangeable is exactly the hole above.

`READ_ONLY` is the whole capability set, and a mode subsets it rather than
extending it. That is what makes ADR 15's rule cheap to keep: the package has
no write door, so a mode written next month cannot open one without editing
the registry, which is where the test is looking.

Research uses Anthropic's server-side web tooling rather than a crawler this
project maintains, which keeps `CLAUDE.md`'s no-scraping rule intact.

The rationale editor built in step 3 of the deck lifecycle is already the right
shape for the interview: the box sits beside a column showing the card as the
corpus has it, which is where a mode's questions go.

### The account, the model, and the order of work

Settled 2026-08-11, when the build stopped being hypothetical.

**A separate API account, not the Claude Max subscription.** The developer
platform bills independently of consumer Claude; Max carries no API credits.
The account is being set up with its own workspace and spend limit, so this
project's usage is a line item rather than a share of something larger — which
is also what will make the hosted-cost decision below answerable with a number
instead of a guess. The key reaches the app as `ANTHROPIC_API_KEY`, and CI's
no-secrets check is what keeps it out of the repository.

**Start on Sonnet, and find out whether it is enough.** Claude Sonnet 5
(`claude-sonnet-5`) rather than the Opus default — Aaron's call, and the
question it answers is worth answering early: most of what the modes do is
conversation over tool results the corpus already computed, which is not
obviously Opus-shaped work. Moving up is a model-ID change and a re-measure, so
the cheap experiment runs first. Note Sonnet 5's request surface differs from
its predecessor's in ways that will bite a from-memory implementation —
adaptive thinking is on by default, sampling parameters are rejected, effort
defaults to `high`. **Load the `claude-api` skill before writing any of it**
rather than recalling the shapes.

**Local first, hosted before too long.** The first surface runs against the
maintainer's own key on his own machine, which needs none of the open decisions
resolved. Hosting comes after, and by then the local run will have produced
real per-conversation numbers to size it with.

*First real numbers, 2026-08-11.* A health check is 18 in / 6 out. A four-tool
turn over the live library — "which decks fail the gate, on what card, and what
does that card do" — cost **19,130 in / 851 out**, about $0.05 at Sonnet 5's
introductory rate. (The same turn cost 25,142 in before `get_cards` existed:
better grounding was also cheaper, because a model that can look a card up
stops guessing at searches.)

Input dominates by 20:1, which is the shape to expect: tool results are large
and answers are short. Two consequences worth carrying into the mode work —
prompt caching is the lever that matters, and `get_deck` is the expensive tool,
since it returns 99 cards with full oracle text.

Two things to settle before it ships, both open decisions above: **what a hosted
Claude surface costs and who pays**, and — for the simulator half — **whether
Forge can run where the app runs**.

---

## Suggested order

1. **The rest of the deck lifecycle.** ✅ **Done 2026-08-11.** `add_card`,
   `remove_card`, `set_card_field`, `set_note` (ADR 12), and the rationale
   editor. What remains of the lifecycle is the create form and promotion.
2. **The Claude surface** — **in progress; the pipe is open, grounded, and now
   has a dial.** The client, the tools, the no-`why` boundary and `get_cards`
   landed 2026-08-11, and **the stance landed the same day** — three axes, four
   presets, `off` by default, the default derived from the deck's `status`, and
   a deployment ceiling. `GET /api/claude` answers whether the surface is
   installed, configured and switched on, as three separate questions.
   **The stance moved ahead of the interview deliberately**: it is the frame
   every mode plugs into, and retrofitting a gate around modes that already
   exist is how the gate ends up with holes. Next, in order: **(a)** the
   rationale interview, the mode that made ADR 15 worth writing; **(b)** a UI
   for the dial, which is what makes any of it reachable by someone who is not
   at a terminal. Moved ahead of Forge on 2026-08-11: it is what makes the app useful
   for judgement rather than facts, and shipping the toolkit to someone else
   without it hands them a gate and a goldfish sim with no opinion in them.
   Sonnet 5, on a separate API account, running locally first — see *The
   account, the model, and the order of work* above.
3. ~~**Forge feasibility research**~~ — ✅ **done 2026-08-11.** `forge.jar sim`
   is driven from Python, all six decks are fully covered, and the timings are
   measured. `mtglab sim forge` works locally today. What is left of this item
   is the *deployment shape*, which the numbers inform but do not decide — see
   the open decision below.
4. **Spoiler scan** and **deals/carts** — both self-contained.
5. **The colour taxonomy** (goal 8). Cheapest thing on this list that a new
   player would notice: one 32-row dataset gives the guild/shard/wedge diagrams,
   the colour-pie lesson and 32 Deck Challenge tracking at once. No Claude, no
   auth, no network.
6. **Assisted refactor** (goal 10) — swap recommendations from all three
   sources. Deliberately last: it inherits the rationale interview's answer to
   "who writes the `why`", and the pod measurement decides whether Forge can
   contribute to it honestly. Building it before those two means guessing at
   both.

**Tier 2 is deliberately not on this list.** It waits behind Forge (goal 2).
**Nor is the leaderboard** (goal 9), which is parked behind the pod measurement
and a decision about what a ranking would even mean.

---

## Open decisions

### Can Forge run where the app runs?

**The prior question is answered; the deployment shape is still open.** Recorded
2026-08-11, gates goals 2, 3 and 7.
[ADR 14](docs/adr/0014-python-decides-claude-advises.md) makes Forge the thing
that plays games. Forge is a JVM desktop application with its own card
database, and `forge.jar sim -d ... -f commander` is a headless mode of it —
not a library, not a service.

**The feasibility spike ran on 2026-08-11 and Forge works here.** All four
deliverables landed: a `.dck` exporter, headless Commander games whose results
parse, the card-coverage pre-flight, and per-game timings. `mtglab sim forge`
is the surface; `sim/tier3/` is the code; [docs/FORGE.md](docs/FORGE.md) is the
setup and the workarounds. What it found:

- **All six curated decks are fully covered.** Forge implements every card in
  every one of them — 87, 89, 76, 85, 86 and 100 distinct names checked against
  its own card scripts.
- **A card Forge does not implement does not stop the game.** It prints a
  warning and plays on. A deck with three bogus names produced a 96-card game,
  a winner, and a turn count, with nothing in the result line saying anything
  was wrong. This is the single most important finding of the spike and the
  reason coverage is now checked twice, before and after every run.
- **`brew install openjdk@21` does not work on this machine**, contrary to the
  prerequisite recorded below. There is no bottle for it on the pinned
  Homebrew, so it is a source build, and the build refuses: Xcode 12.4, needs
  14.2. A prebuilt Temurin tarball needs no compiler and works.
- **`-D` is a lie for single matches.** It is only wired into tournament mode.
  Decks reach Forge through `forge.profile.properties` instead.

**Timings, on the 2015 MBP, 8 logical CPUs.** Ten games per row, one JVM per
row, `-c 300`:

| Deck (heads-up vs a fixed opponent) | Median | Mean | Max | Wall for 10 |
| --- | --- | --- | --- | --- |
| Goreclaw stompy | 4.6s | 5.7s | 12.1s | 67s |
| Atla Palani dinos | 4.8s | 6.3s | 17.8s | 72s |
| Arahbo cats | 5.0s | 5.8s | 11.9s | 67s |
| Gyome food | 5.8s | 10.2s | 37.8s | 110s |
| Trostani tokens | 6.7s | **25.3s** | **134.5s** | 262s |
| Tivit cEDH | 6.8s | 11.0s | 28.1s | 119s |

**The median is not the number that matters; the tail is.** Medians cluster in
a boring 4.6–6.8s band across every archetype. The means do not: Trostani's is
four times its median because one game took 134 seconds, and a wide token board
is combinatorially expensive for the AI to evaluate. Nothing hit the 300s clock,
so 300 is currently a real ceiling rather than a source of fake draws — but
120s, Forge's default, would have turned that Trostani game into a draw and
quietly corrupted the row. **Quote medians and tails, never means.**

JVM boot plus the card database is **~9s, flat**, and it amortises: it is paid
once per `sim` invocation regardless of `-n`. That is the number that decides
process shape more than per-game cost does.

**Four-player pods, measured 2026-08-11** — the shape Commander actually plays,
and therefore the shape a hosted instance would be paying for. Same machine,
`-c 300`:

| Pod | Games | Median | Mean | Max | **Clocked out** | Wall |
| --- | --- | --- | --- | --- | --- | --- |
| A — Cats / Dinos / Goreclaw / Trostani | 5 | **283s** | 210s | 300s | **2 of 5** | 17.7 min |
| B — Tivit / Gyome / Cats / Dinos | 4 of 5 † | 126s | 158s | 300s | 1 of 4 | — |

† Pod B was stopped after four games; treat its row as indicative, not a
measurement. Pod A is complete.

**The runtime is not the finding. The clock-out rate is.** Heads-up, nothing
came within 100s of the 300s clock. In a pod, **40% of games hit it** — and a
clocked game is the measurement giving up, not a result. They are recorded as
`timed_out` and reported separately from draws, which is exactly why
`CLAUDE.md` insisted on that distinction.

That creates a bind: raising the clock to make pod games honest makes runs
proportionally longer. A 600s clock plausibly puts one 5-game pod past half an
hour. **There is no setting at which pod simulation is both honest and quick on
this hardware.**

**Why pods are slow, from Forge's own diagnostic.** When the clock trips Forge
dumps the AI thread's stack under `AI eval thread at timeout:`, and it lands in
`ComputerUtilCard.shouldPumpCard` / `PumpAllAi` — the AI evaluating a mass pump
across a wide board, now with three opponents' boards to weigh instead of one.
The same mechanism as the 134s Trostani heads-up outlier, multiplied by the
table.

Both runs were clean: **0 unsupported cards, 0 abandoned games.**

One observation worth flagging rather than concluding from: in pod B's four
games, Tivit and Gyome — the two decks Forge plays badly — won **none**, while
the dinosaurs won three. Four games proves nothing, but it points the same way
as the 8–2 heads-up result in goal 7.

**What this does and does not settle.** It sizes the hosted question: a pod run
is tens of minutes of near-saturated CPU, which is a background job on a
dedicated box, not a request on a shared one. It does **not** choose the
deployment shape — that stays open, and stays Aaron's call.

Three shapes, still none chosen — but now with numbers against them:

- **Local only.** Forge simulation is something you get when running `mtglab`
  on your own machine; the hosted instance has a documented feature gap. Keeps
  the deployment small and honest, and is the smallest thing that could work.
  *Supported by:* nothing in the spike argues against it, and it is the only
  shape that needs no new infrastructure at all.
- **Server-side.** Anyone logged in can run Forge sims. *Supported by:* the
  ~9s startup amortising over a batch, and by the fact that the run is already
  a subprocess with a timeout. *Argued against by:* a 470 MB image plus a JVM
  on a 1 GB Fly instance that also runs DuckDB and numpy, and by 134-second
  games at 100–200% CPU. This is the Fly-versus-Hetzner sizing question below,
  and the tail says Hetzner.
- **A separate worker.** The app queues a job; something else runs Forge.
  *Supported by:* the tail. A minutes-long run with an unpredictable ceiling is
  exactly what a queue is for, and `api/jobs.py` already has the shape.
  *Argued against by:* it is the most moving parts, for a feature no one has
  asked for yet.

Two things a hosted shape would have to solve that a local one does not:
`ensure_profile` writes `forge.profile.properties` into the Forge install
(fine in an image, baked at build time; not fine on a read-only mount decided
later), and generated `.dck` files are named for the deck slug in one shared
directory, which two concurrent runs would race on.

#### The spike brief, researched 2026-08-11

Written down so the next session starts from evidence rather than re-deriving
it. **Forge remains the only candidate** and the question "should we use
something else instead" is closed:

| Option | Verdict |
| --- | --- |
| [Forge](https://github.com/Card-Forge/forge) | **The one to use.** Documented headless mode, actively maintained, Commander is a first-class format. |
| [XMage](https://github.com/magefree/mage) | No. Excellent rules coverage (~19k unique cards) but it is a networked play server — no headless batch mode, not built for running hundreds of games for statistics. |
| Cockatrice | No. **No rules enforcement at all** — a virtual tabletop, not an engine. Frequently suggested; cannot simulate anything. |
| Arena / MTGO | No. Closed, no API, automation against ToS, and Arena has no Commander. |
| Deck sites (MTGGoldfish et al.) | Not engines. Prices and lists. Also already out of scope — `CLAUDE.md` bans marketplace scraping. |

The invocation, from Forge's own AI wiki — note it matches `CLAUDE.md`'s Tier 3
requirements exactly, which were written against the real flag list:

```bash
forge sim -d <deck.dck> ... -f Commander -n 100 -c 300 -q
```

`-f Commander` selects the format · `-n` game count · `-m` matches (best-of) ·
`-c` seconds before a draw is declared, **default 120, which is the number
`CLAUDE.md` says to raise for Tivit** · `-q` results only · `-D` an absolute
deck directory · `-t` tournament type. Games end with an announcement of the
winner and the match status, so the output is line-oriented text to parse — not
JSON, and that parser is part of the spike.

**Prerequisite, checked on this machine:** Forge needs **Java 17+**; the Mac has
10.0.1 and 1.8. ~~`brew install openjdk@21` resolves cleanly on the pinned
Homebrew, so this is an install rather than a blocker~~ — **wrong, corrected
2026-08-11 by the spike.** The formula resolves but has **no bottle**, so it is
a source build, and the build refuses on Xcode 12.4 when it wants 14.2. The
conclusion survives the correction: it needs no OS upgrade, because a prebuilt
Temurin 21 tarball needs no compiler. See [docs/FORGE.md](docs/FORGE.md).

**What the spike has to produce**, in order, stopping at the first thing that
fails — **all four done 2026-08-11**:

1. ✅ A `.dck` exporter from `deck.yaml` — Forge's own deck format, and the first
   place a mismatch will show up. `sim/tier3/dck.py`; format read off the 13,994
   `.dck` files Forge ships rather than guessed.
2. ✅ One headless Commander game that completes and whose result parses.
   `sim/tier3/parse.py`, matching the literal format strings in
   `forge.view.SimulateMatch`.
3. ✅ **The card-coverage pre-flight.** Non-negotiable per `CLAUDE.md`: Forge does
   not implement every card, and silently dropping cards would poison every
   number that follows. Establish how a dropped card is reported *before*
   trusting any result. **Established, and it is worse than assumed: a dropped
   card is reported only as a log warning, and the game plays on.** Hence two
   checks, `sim/tier3/coverage.py` before and `parse.py` after.
4. ✅ A timing measurement per game, which is what makes the local-vs-hosted
   question above answerable with a number. Table above.

Only then does the deployment shape get chosen — which is where this now sits.

### What a hosted Claude surface costs, and who pays

**Open, recorded 2026-08-11 and narrowed the same day.** ADR 14 puts
conversation and research on the Claude API. Locally that is the maintainer's
own key and own spend. Hosted, it means **the maintainer pays for other
people's questions**.

**It is smaller than it felt, at least for the interview.** Estimated from a
turn shape of roughly 12K tokens in and 800 out — the deck plus one card's
corpus facts, a question back — with the deck and system prompt sitting in a
cached prefix that reads at a tenth of input price:

| | First turn | Cached turn | A full 99-card draft |
| --- | --- | --- | --- |
| Sonnet 5 | ~$0.03 | ~$0.01 | **~$1.00–1.50** |
| Opus 5 | ~$0.08 | ~$0.03 | **~$2.50–3.00** |

A dollar or two to interview a whole deck does not need a funding model. These
are estimates, not measurements — re-derive them with `count_tokens` against
the real prompt once the surface exists, since the turn shape is the assumption
doing all the work here.

**Research is still the expensive half** — web search and long context are
where the cost is, and that half is not estimated above. So the decision
narrows rather than closing: the interview and deck conversation look
shareable at friends scale, and research is the mode that may need a per-user
budget, bring-your-own-key, or staying local. The stance dial in ADR 15 is
also a cost control — off means no calls at all, which is a reasonable hosted
default. Batch pricing (half rate) covers anything not latency-sensitive, like
a spoiler scan across six decks.

### Hosting — plan

> Full maintainer setup guide, auth design, per-user data model and measured
> compute analysis now live in [docs/HOSTING.md](docs/HOSTING.md). Summary below.
>
> **The running list of what is still missing is
> [§7, Deployment readiness](docs/HOSTING.md#7-deployment-readiness--the-running-list)**,
> started 2026-08-11 when hosting stopped being hypothetical. Tick items off
> there rather than rewriting the plan here.

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
