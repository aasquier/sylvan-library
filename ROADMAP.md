# Roadmap

The original goals, what actually works today, and what comes next. This file
is the durable plan — it survives a fresh session, unlike a conversation.

Status keys: **done** · **partial** · **not started**

---

## The near-term TODO

Decided 2026-08-12, in order. Everything below it in this file is the longer
arc; this is what the next few sessions actually do.

1. ~~**The best-practices and cleanup pass.**~~ Landed 2026-08-12: the API
   catch-all refuses `/api` misses as JSON, dead code out, mode tool sets
   enforced at dispatch, four-colour names in, `api/service.py` and
   `api/simruns.py` strict under mypy, route-level code splitting (262 kB
   entry, Recharts lazy), actions SHA-pinned with dependency review and
   Dependabot, the mode prefix prompt-cached, version 0.2.0. Still open from
   that pass, deliberately deferred: mutation testing, golden artifact
   snapshots, Playwright/axe, SBOM and image signing, and making
   `dependency review` a required check.
2. ~~**Manual UI testing**~~ — done 2026-08-12, a person driving the app end
   to end rather than a suite. Before it, all seven screens and the catch-all
   were smoke-tested clean (every lazy route mounts, no stuck Suspense
   spinner, no console errors), so what the tour found is UI/UX rather than
   breakage. The auth-on configuration (`MTGLAB_REQUIRE_AUTH=1`, the
   `mtglab-ui-auth` launch entry) is still worth a pass of its own, since it
   is the configuration Fly actually runs.
3. **UI/UX polish** — the punchlist from that tour, and the phase that now
   stands between here and deploy. The list came from the maintainer on
   2026-08-12 and is written out below so it survives a fresh session; it is
   his list, so **an item is done when he says it is, not when it compiles.**
   Five branches, each green before the next starts. Branch 2 was not on the
   original list — it came out of reviewing branch 1's deck page and displaced
   the rest by one.

   **The order is now 1, 2, 5, 3, 4**, changed 2026-08-12 after the branch 2
   review; 1, 2 and 5 have landed. The numbers are identities, not positions, so nothing renumbers.
   What moved 5 to the front is worth recording because it is a finding rather
   than a preference: asked to test branch 2, the maintainer went looking for
   an *interactive* Claude assist in the deck builder and found none. He was
   right — "Start a deck" has no Claude in it at all, and the one interactive
   surface that does exist, the rationale interview, is reachable only by
   opening a deck, clicking *Edit why* on a card, and then *Ask for questions*.
   Nothing announces it. So the four modes built or planned so far are a read
   surface and a hidden one, and the thing that would make the app feel like it
   has an assistant in it is branch 5. Visual identity and teaching are worth
   doing and neither of them changes that.

   **1 — Bugs and quick wins.** Landed 2026-08-12 in
   [#55](https://github.com/aasquier/sylvan-library/pull/55).
   - *Delete was unusable.* The confirm label was styled `uppercase` while the
     check was case-sensitive against the lowercase slug, so typing what was
     on screen left the button disabled and said nothing about why. The
     confirmation is now the word `bury` (`service.DELETE_WORD`, and the slug
     still works), matched case-insensitively, with the reason shown when it
     does not match. Regression-tested on both sides.
   - *Deck notes read like source code.* 17 `TODO —` markers across six decks
     became prose or a bare `—`. Artifacts regenerated for the four decks the
     gate lets through; Atla and Goreclaw still refuse on their banned card.
   - *The commander was cropped out of its own page.* `art_crop` is 1.37:1 and
     the hero band was 4.6:1, so it kept a third of the painting's height from
     the middle. The band is atmosphere now and the commander is the whole
     card, uncropped, with the usual hover. Library tiles went to `art_crop`'s
     own ratio for the same reason.
   - *Dead carousel controls* on Colourless and All five, which have one
     member each.
   - *Tier context lived inside the first combination of each tier* — Bant
     explained shards, Abzan explained wedges, Artifice explained four-colour
     identities — so it was invisible to anyone who arrowed past them.
     `colors.TIER_BLURBS` is where it lives now, one per tier including the
     four that never had one. A blurb is mechanical and never names a plane;
     an era is a setting and never restates the mechanics.

   **2 — The commander dossier, and alternative arts.** Landed 2026-08-12 in
   [#57](https://github.com/aasquier/sylvan-library/pull/57), with
   [ADR 19](docs/adr/0019-the-dossier-cites-three-sources.md) written first. Branch 1 answered "what does this card do" with corpus counts; this
   is the *interesting* half — who this character is, what archetype they
   define and where it came from, their rivals, where they sit in Magic's
   history. The second Claude mode, and the first whose facts do not all come
   from the corpus.

   What the build settled, beyond the decisions below:

   - **Web search and structured outputs coexist, and that is what makes the
     source check load-bearing.** A response schema *suppresses* the API's own
     citations, so a URL in the payload is a string the model typed and nothing
     more. `dossier.keep_sources()` intersects the cited pages with the ones
     `Turn.searched` recorded and drops the rest, counting what went — the
     answer to "how do you know it read that" is now a set intersection rather
     than a promise. Measured on Gyome: 54 pages read, 4 cited, 0 dropped.
   - **Two bugs that only appear with a server tool *and* a second turn**, both
     found by running the mode rather than reading the request shapes. The
     dated search filters its results inside a code-execution container, so a
     follow-up request carrying that turn's blocks is a 400 unless the
     container id comes with it. And a server-side tool loop that hits its own
     limit stops with `pause_turn` carrying text that reads finished — the same
     shape as a Forge game that plays on with 96 cards, and now resumed rather
     than returned.
   - **A rule-1 leak in the one sentence most likely to have one.** A first run
     described Trostani Discordant as making Food tokens; she makes 1/1
     Soldiers. Comparing two commanders is exactly the sentence that wants a
     half-remembered ability, so the mode now must call `get_cards` on every
     rival before writing about it, and the rival's real corpus text rides in
     the payload so the card sits next to the sentence. The second half is what
     does not depend on the model complying.
   - **Cost:** about 800 uncached input tokens and 2,100 out per commander,
     with ~57k served from the prompt cache. Once per commander, ever, because
     the key is the `oracle_id`.

   Decided with the maintainer on 2026-08-12, so a session does not re-open
   these:

   - **Three sources, and a rule about which may support which claim.** Card
     facts — cost, type, text, legality, identity — come from the corpus,
     always; never from web search and never from recall. The meta, archetype
     history and "where does this sit in Magic" come from **server-side web
     search with its sources shown** (`web_search_20260209`; Anthropic-hosted,
     so the no-crawler rule is intact). Claude supplies voice and framing and
     carries no factual weight. The UI shows the seams: branch 1's counted
     strip stays, Claude's prose sits below it labelled, web claims keep their
     link. That is ADR 14 boundary 3 made visible.
   - **It writes nothing to `deck.yaml`.** `may_write` stays empty and ADR 15's
     invariant is untouched. The result is cached like Tier 1 results, keyed on
     the commander's `oracle_id`, so it is generated once and shared by every
     deck that commander leads — including across users on a hosted instance.
   - **Generated automatically for new and imported decks**, on a button for
     the existing six. At stance `off` the button does not appear, because off
     means no calls (ADR 15).
   - **Any card the model names is validated against the corpus**, the way the
     interview drops anything that is not a question.
   - **Alternative arts**: a `commander_art` field on `deck.yaml` holding a
     printing id, a picker showing every **non-digital** printing newest first
     (Goreclaw has 12, including a Secret Lair; Gyome has 3), and
     `mtglab decks set <slug> --art <set>`. A deck property rather than a
     per-viewer preference — `deck.yaml` is the source of truth and the choice
     should travel with the deck through git. Note `printings` has
     `image_normal` but no `image_art_crop`, so the hero band needs one or the
     other resolved.

     *Built.* The crop is **derived** rather than stored: Scryfall's image URLs
     differ only in the size segment, which `oracle_cards` proves by carrying
     both for the same printing id, so `service.art_crop_from` swaps `normal`
     for `art_crop` and returns None on any other shape. That avoids blocking a
     UI branch on a 500MB re-ingest, and a column can still be added later
     without changing a caller. Two checks in two layers: `edit.py` refuses a
     value that is not a UUID (it is text surgery and has no database), and
     `service.py` refuses a printing that is not **this commander's** (only a
     query can know). A set code with several printings — `MUL` has four
     Goreclaws — lists them and refuses rather than picking one.

   **3 — Visual identity.** After 5. The splash and the Sylvan Library art (which is
   only rendered on an *empty* library today, so the maintainer has never
   seen it), an interactive colour pentagram for the mono tier, and the
   builder's tier headers, which are plain grey panels. Decided: the card art
   stays a Scryfall hotlink and everything else is **drawn in SVG/CSS** — no
   new binary assets, no licensing question, and the pentagram is a diagram
   anyway.

   **4 — Teaching.** A vocabulary section for beginners; hover help in the
   simulator, whose parameters are words and numbers divorced from meaning;
   and real depth behind the guilds, shards, clans and colours — champions,
   plot lines, classic cards.

   **5 — Claude in the builder.** Built 2026-08-12, with
   [ADR 20](docs/adr/0020-the-theme-interview-reads-a-person.md) written first.
   Moved ahead of 3 and 4 for the reason recorded above. A guided, adaptive
   interview that helps pick a theme and a commander, plus the discoverability
   fix below. The refactor pass stayed out and remains goal 10. Rule 4 is
   untouched: no mode writes a `why`.

   What the build settled, beyond the decisions below:

   - **The questions are not about Magic, and that is the whole feature.** The
     first draft asked "when you picture yourself winning, what is on the
     table" — a Magic question wearing a friendly hat, unanswerable by exactly
     the person this is for. It asks about a film, a period, a star sign, how
     somebody is at game night, and translates. That works because the colour
     pie is a personality taxonomy before it is a set of mechanics, which is
     also why `colors.py` is a **fourth source** alongside ADR 19's three:
     checked in, carrying `verified_by`, and free.
   - **Readiness is a grounded-slot count.** Every reading the mode takes
     carries a quote, Python checks the quote against the user's own turns, and
     three surviving kinds opens the proposal. Third instrument after
     `only_questions()` and `keep_sources()`, pointed at a model reporting back
     a preference nobody expressed.
   - **Three bugs that only appear when you run it**, none visible from reading
     the shapes. The interviewer speaks first, so the transcript starts with an
     assistant turn and the request needs a synthetic user frame or every
     answer is a 400. Alternation was enforced on a false premise — the API
     combines consecutive same-role turns — and enforcing it wedged any
     conversation where a turn came back without a usable question. And the
     proposal ran **zero searches** on its first outing, resting archetype
     claims on nothing, until the prompt was made prescriptive about when to
     call the tool.
   - **A dropped commander can cost a whole suggestion.** A legend of a
     *subset* identity is legal in those colours and does not make a deck that
     fills that slot, so it is dropped — and when all three go, the combination
     goes with them. Observed live: one run returned two combinations and the
     next returned one. Counted and surfaced now rather than silently thin.
   - **Cost and time:** a conversation turn is a few seconds and heavily
     prompt-cached (~48k cached tokens by turn three). The proposal is the
     expensive half — **measured at 226 seconds** end to end with `max_uses: 4`,
     ~79k input / 8k output, since it reads a dozen-odd pages and checks every
     legend. Trimmed to three searches, and the UI says it takes a few minutes.
     That was **the deploy blocker**, and it is fixed — see 5b below.

   **5b — The proposal is a background job.** Landed 2026-08-13, on the branch
   that also carries this paragraph. No ADR: nothing ADR 20 settled moved. The
   transcript is still client-held and resent, the server still stores no
   conversation, readiness is still recomputed rather than carried, and the
   wire format is still the mode's own. What changed is only how the answer is
   delivered.

   - **Checking happens in the request; calling happens in the job.** The same
     division `plan_mana` makes, and for a sharper reason: three things refuse a
     proposal without a network call — a malformed transcript (422), a floor not
     yet reached (409), no key (503) — and each is a distinct answer the UI acts
     on. Carried into a worker they would all arrive as *a job in state `error`*,
     which is one string for three cases and a status code for none. So
     `theme.check_proposal` runs in the route and `api/themeruns.py` queues only
     what needs Anthropic. A stance of `off` is a job born finished, the shape
     `jobs.completed` already existed for.
   - **There are two job pools now, and the split is about what the work waits
     on.** Tier 1 is CPU-bound pure Python and keeps its single worker, because
     a second thread would contend on the GIL. A Claude call is a socket wait
     that releases it for minutes, so sharing one queue would stall a
     thirty-second sweep behind four minutes of somebody else's conversation.
     `jobs.CPU` and `jobs.NET`; the lane rides on the `Plan` because it is a
     property of the work rather than of the route.
   - **Nothing is cached, deliberately.** ADR 18 caches a simulation because it
     is reproducible; a proposal is not, and the dossier is cached because its
     subject is a character that outlives any conversation. Caching here would
     mean the one moment somebody wants a different answer — clicking again on
     an unchanged transcript — is the moment they cannot have one. The client
     keeps the job *id* instead, so a reload reattaches to the run in flight
     rather than paying for a second.
   - **Two things only the live run showed**, which is now the fourth branch
     running where that has been true. A four-minute job reporting nothing is
     indistinguishable from a wedged one, so `converse` gained an `on_turn`
     hook and the job reports turn *n* of 8 (a ceiling it usually does not
     reach, so the UI shows seconds rather than a bar that would sit at 38% and
     jump). And the first reattach after a reload showed **0s against a job
     already 70 seconds old** — the clock now reads the job's own `created_at`,
     which is the run's age rather than this tab's.
   - **Measured on the real surface, twice:** 15 pages read, 4 cited, 0 sources
     dropped, 0 commanders dropped, ~72k in / 5.8k out with 53k served from the
     prompt cache. A reload mid-run reattached to the same job both times.

   Three things are already settled and should not be re-opened:

   - **It proposes; the user creates.** Nothing under `src/mtglab/claude/` can
     reach a write path, and `create_deck` is on the write surface
     `tests/test_claude_boundary.py` forbids naming. So the interview's output
     is a *proposal* — colours, then commanders — and the existing create flow
     is what makes a deck. That is the same shape the rationale interview has,
     arrived at from the other direction, and it is a feature: the deck is
     made by the person whose deck it is.
   - **Every commander it names comes from the corpus.** The theme half is
     opinion and is exactly what Claude is for; the moment it starts naming
     cards, rule 1 binds. `search_cards` with `commanders_only` and an identity
     filter is the tool, and a name that does not resolve gets dropped and
     counted — the instrument the dossier's rivals already use.
   - **A theme is not a `why`.** Asking what historical period somebody relates
     to engages no part of rule 4, and the mode still may not pre-fill a
     rationale for any card it suggests.

   **Conversational, not one-shot** — decided 2026-08-12. A multi-turn
   interview that adapts to the answers, rather than a form of fixed questions
   followed by a proposal. It is the more expensive thing to build and it is
   the reason the feature is interesting: a form could have been a form
   without a language model in it.

   That choice is what makes this mode genuinely new rather than the rationale
   interview with different words, and it is the part the ADR has to think
   about hardest. Three consequences that are not obvious:

   - **The interview holds state across turns, and `converse` currently does
     not.** Every mode so far is one question and one answer; this one is a
     conversation whose history has to survive between HTTP requests. Where
     that history lives — client-held and resent, or server-side and keyed —
     is a real decision with a cost either way, and it is the first thing to
     settle.
   - **A multi-turn mode has no natural stopping point**, which is exactly
     where `MAX_TOOL_TURNS` came from for the single-shot ones. It needs a
     ceiling that is about the *conversation*, not the tool loop, and a way
     to say "I have enough to propose now" that is checkable in Python rather
     than trusted from the model.
   - **The proposal is a schema, the conversation is prose.** Those want
     different response shapes, so a mode that does both is either two modes
     or one mode with a mode switch — and ADR 15 says a mode is a prompt, a
     tool set and a capability declaration, so two is probably the honest
     answer.

   **The refactor pass stays out**, confirmed 2026-08-12, and the reason is now
   a design one rather than only sequencing: it is a *critique* surface over an
   existing deck, and this branch's whole argument is that the deckbuilding
   surface must not reach a deck. Shipping both together would blur the
   boundary on the branch that draws it. It also still inherits the rationale
   interview's answer to who writes the `why`, and the pod measurement still
   decides whether Forge can contribute to it honestly. Goal 10.

   **Also in scope, and cheap:** the rationale interview was undiscoverable —
   it worked, and nothing on the deck page said so. *Done:* every card carries
   an **Ask Claude** control beside *Write why*, which opens the editor already
   asking rather than revealing a second button that asks; the cards tab says
   the feature exists and states rule 4 in the same breath; and all of it is
   honestly absent when the surface is off, unconfigured or uninstalled.

   Two things from the cleanup pass are worth knowing while working through
   it: the four-colour names come from `colors.py`'s taxonomy (Artifice,
   Chaos, Aggression, Altruism, Growth) and any new copy of that table has to
   agree with it, and the six non-landing routes are lazy, so a new screen
   wants a `React.lazy` line rather than a top-level import.

   **Added 2026-08-13, from play-testing branch 5.**
   Eleven more from the maintainer, unprompted, after he drove the theme
   interview. **None of it is started, none of it is scheduled**, and it does
   not displace branches 3 and 4 — it is written here rather than left in a
   conversation because that is the whole point of this file. Three of them
   already have a home elsewhere and say so.

   *Deckbuilding surface:*

   - **An opening-hand randomiser/visualiser for a built deck** — "pretty
     standard and a fun addition to help people get a feel for opening hands."
     Bonus: randomise which printing's art each card shows. Further bonus: a
     **mulligan-confidence suggestion**, which is the part to be careful with —
     confidence about a keep is a claim, and Tier 1 is the only thing here that
     could back one. Either put a real simulation behind it or do not call it
     confidence.
   - **Two-sided cards show one face.** Scryfall renders a small flip control;
     this app does not, anywhere. The most concrete item in the list — the
     corpus already carries both faces, and `CardRecord.front_type_line` exists
     because the commander dossier already had to care.
   - **"Entomb" as the delete button's label for commanders.** The label only:
     the typed confirmation **stays `bury`** (`service.DELETE_WORD`, branch 1),
     confirmed by him — "still fine to ask for them to type 'bury' to be sure."

   *Content depth:*

   - **The guild, clan and shard descriptions are bland**, at the macro level
     too — "the guilds of Ravnica are pretty famous. We can do better."
     **This is branch 4** (teaching) and needs no new slot; it is recorded here
     as evidence for what branch 4 is actually for.
   - **Lore rivals on the commander dossier.** He likes ADR 19's Rivals, and
     reads them as *strategic* rivals; he also wants **story** rivals — "like
     Bolas and Ugin, for instance." A second, separately-labelled kind rather
     than a replacement, and it inherits ADR 19's rules: a rival that is a card
     resolves through `get_cards` or is dropped, and a claim about the story
     rests on a cited page.
   - **Searchable infinite combos**, linked to a deck's wincons or its
     breakdown — "that is good info to know and I think there are websites
     devoted to it." There are, and **the no-crawler rule is what shapes this**:
     hosted web search per question, or a small hand-curated set of our own.
     Not an ingest of somebody's combo database.

   *Storage:*

   - **"Are we ready for multi-user deck storage? Seems like we just throw
     things in `/decks`."** Not yet, and the answer is already designed: that
     is `user_decks`, [docs/HOSTING.md](docs/HOSTING.md) §6 step 6, and
     `decks/source.py`'s `DeckSource` protocol plus `api/deps.py` exist so it
     is one dependency to swap rather than thirteen handlers to edit.

   *The theme interview, which is where most of his thinking went:*

   - **Personas instead of a fixed question battery.** "I don't want to get too
     locked into our question battery. Books, art, tv, movies, star signs, all
     good stuff, but we almost want personas" — a **storyteller**, a **tarot
     reader**, a **confessor**, as characters the interviewer adopts. In ADR 15
     terms this is a mode's prompt varying while its tools and its write scope
     do not, which is the cheapest possible version of it.
   - **A tarot reading as a door of its own.** The Rider–Waite deck's original
     art is public domain: deal somebody a hand, let Claude be the oracle, and
     interpret the spread into a colour identity and a commander — "It could be
     fun with animations, crystal balls, etc." Note this is the one item that
     wants **binary assets**, which branch 3 deliberately avoided; the licence
     is the reason it is possible at all and should be checked rather than
     assumed.
   - **An interview for somebody who already has a theme.** The current one
     discovers a theme; this one would take a given theme and follow it. "That
     would be a fun alternative interview style."
   - **Claude's suggestions should go past interpretation.** Today it reads you
     and describes; he wants it advising specific commanders and **tied into
     the rest of the analysis** the tool already does. That is the item most
     likely to collide with ADR 20's "it proposes, you create" and with rule 4,
     so it wants reading against both before it is designed.

   **The register is the requirement, not decoration.** The theme interview
   exists because he rejected a first draft that asked Magic questions in a
   friendly voice; the tarot and persona ideas are that same instinct. A
   version of these that arrives sensible and dull has missed them.
4. **Deploy** — [docs/HOSTING.md](docs/HOSTING.md) §7 is the checklist. What
   remains is an account, a card, a DNS record and the seeding run: the Fly
   app + volume, the Resend account and verified sending domain (start the
   DNS early), `fly secrets`, seed the corpus and decks, then the refresh
   runbook.
5. **After deploy, next build work in order:** re-price automated PR review
   (ENGINEERING §5, parked), the stance dial UI, then the remaining Claude
   modes ADR 15 names and branch 5 does not build (argue a slot, deck
   conversation, research).

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
auth (the core exists as of 2026-08-12; the email half does not), a per-user
data model (still does not — `user_decks` is build-order step 6), consent and
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

Beyond the two bans, the gate and a corpus cross-check caught five errors in
the source prose — a card name that does not exist (the real one was Sigarda's
Aid), Arahbo's doubling described as an activation when it is a per-attacker
trigger, a hand-counted curve off by a few cards per bucket, a claimed 67%
T5 commander rate that sampling puts at 57.2%, and Kaheera's companion
condition never checked by the gate at all (now enforced, below). Every one
was a checkable fact that prose got wrong.

### Companion and partner rules are enforced

Condensed 2026-08-12 — the detail lives in `decks/companion.py`,
`decks/partners.py` and their tests, which are the authority.

**Companions** (`decks/companion.py`): all 10 commander-legal companions'
deckbuilding restrictions are checked — nine exactly, Zirda's
activated-ability test as a heuristic that warns rather than blocks. The
design rule worth keeping: **an unevaluated restriction warns loudly and is
never reported as satisfied** — an unrecognised companion produces
`companion-unchecked`, not a silent pass. Three printing-dependent companions
(expansion symbols, retro frames) are deliberately unchecked; none is
Commander-legal anyway. The companion is also checked for legality, colour
identity, and not being listed in the 99, and "your starting deck" includes
the commander — Arahbo is a Cat Avatar, so the cats list stays legal.

**Two-commander pairings** (`decks/partners.py`): plain Partner, Partner
with `<name>`, `Partner—<label>` (matched on the label, so new sets add
labels for free), Choose a Background + Background, and Doctor's companion +
a Time Lord Doctor — all enumerated from the corpus, with deck size correctly
98 when two commanders share the zone (companions, by contrast, sit outside
the 100). **One rule got wrong the first time and recorded so nobody repeats
it:** Battlebond's non-legendary `Partner with` creatures do *not* gain
commander eligibility — the official ruling is blunt about it — and the gate
rejects them saying exactly that, because "does not say it can be your
commander" reads like a data problem rather than a rule.

---

> **Next phase:** [docs/ENGINEERING.md](docs/ENGINEERING.md) — property-based
> and differential testing (**done**, §2), then ADRs, a deck-source seam,
> frontend tests and container hardening. A compiled rewrite is **deferred with
> a written trigger**; the measurements say Tier 1 would gain nothing, and Tier
> 2 gets built in Python and profiled before that call is re-made.

## The deck lifecycle

**Complete as of 2026-08-11** — create (`POST /api/decks` and the New Deck
page, which teaches the colour combinations on the way in), import, the
surgical edits, promotion, deletion (confirm by typing the slug; moves to
`decks/.trash/`), and export. Design decisions live in
[ADR 12](docs/adr/0012-decks-are-edited-by-surgical-operations.md) (how a deck
is edited) and [ADR 13](docs/adr/0013-an-imported-deck-is-a-draft.md) (what an
imported deck is). The build notes below are kept for what they settled, not
as a plan.

### What the build settled, kept as findings

Condensed 2026-08-12; the code and its tests are the authority.

- **Import** (`decks/decklist.py` grammar, `decks/importer.py` resolution):
  unknown names are kept verbatim and reported rather than dropped — dropping
  them would hand back a 96-card deck silently. It refuses without a corpus,
  and deliberately will not pick a commander the list did not name or assume
  a Companion-ability card is *this deck's* companion.
- **The draft stage**: a draft's missing rationales report as **one counted
  warning**, not 99 — the per-card wall was burying the banned card the same
  run was meant to surface. `decks build` refuses a draft outright, with no
  `--force`, because a draft is not wrong, it is unfinished.
- **The surgical edits** prove themselves against an oracle — the naive
  parse-mutate-dump computes the document each edit ought to produce, and the
  text surgery refuses to return anything that does not read back as exactly
  that (the ADR 10 move; it immediately caught an empty `swap_board:` parsing
  as `None`). Insertion is category-aware so a new card lands under its own
  section banner.
- **Promotion is refused before the write, not after it** — the deck is never
  written into a state its author has to undo, and the refusal names the
  cards still owing.

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

Since then: the **stance** (three axes, off by default, deck-derived default,
deployment ceiling) and the **first mode**, the rationale interview — with a UI
beside the rationale box. Both are detailed below.

Then the **commander dossier** (2026-08-12, ADR 19) — the second mode, and the
first to reach past the corpus. It is also where research through server-side
web tooling turned from a plan into code: `web_search_20260209`, with every
cited page checked against what the search actually returned.

What is *not* built: the other three modes ADR 15 names, a UI for the stance
dial itself, the activity log the top of the write axis needs, and the Forge
half.

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
| **Commander dossier** | who the character is, the archetype and its history, rivals, standing — ADR 19, built 2026-08-12 |

Four was a guess and the dossier is the guess being wrong in the direction ADR
15 predicted. What held is the structure: a mode is a prompt, a tool set and a
capability declaration, and it absorbed a **server-side tool** and a second
class of evidence without the write column moving.

The interview is the mode that made this worth settling before writing code.
"Claude asks, the user answers, the answer lands in `why`" breaks no rule — the
keystrokes are the user's. "Tidy that up" is one button away and is a
machine-written rationale. So the boundary is drawn where it can be tested
rather than promised: **no code path passes a model response into the `why`
field**, and a mode may put a question beside the box but never text inside it.

#### The rationale interview, built 2026-08-11

The first of the four. `mtglab claude interview <slug> --card X`,
`POST /api/decks/{slug}/interview`, and a panel in the column ADR 12's
rationale editor left empty for exactly this. The other three are not built.

What is worth carrying forward is **where the boundary ended up living**, since
none of it is the system prompt:

- **The response schema has no field for a rationale**, and forbids extra
  properties. A model that wanted to hand over a draft has nowhere to put one.
- **Every item is checked to be a question** — it must end in a question mark,
  and what does not is dropped and *counted*. A mode that starts editorialising
  shows up as a number rather than as help.
- **The corpus facts arrive before the model does.** `interview.brief()`
  assembles the oracle text, the gate's verdict, the category counts and the
  neighbouring cards' rationales in deterministic Python. Rule 1 therefore does
  not depend on the model choosing to call `get_cards` — which is the same
  failure mode as the hole above, one level up.

Run against the two decks that deliberately fail the gate, it opened both times
on the ban and quoted the real oracle text of both banned cards — the case that
caught the earlier hole, now passing by construction. It cost about 4,900 input
tokens a card and made **no tool calls at all**, because the brief already had
what it needed.

**It also found something.** Asked about Primeval Titan, whose rationale claims
"6/6 trample", it asked whether that was recalled from memory — because the
corpus does not store power or toughness at all. It does not: no `power` or
`toughness` column exists. That is a real rule-1 gap for every creature claim in
every deck, it needs a loader change and a re-ingest, and it is its own piece of
work. Worth recording *how* it surfaced: by running the surface against a real
deck, which is the second time that has been the thing that found the gap.

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
since it returns 99 cards with full oracle text. **The caching half landed
2026-08-12:** `converse` puts a cache breakpoint on the system block, which
caches the tools and system prompt together (the interview's prefix is ~1.5k
tokens, above the model's cacheable minimum), and `Turn.cache_read_tokens`
reports what the cache actually served — the number to watch, because a zero
across repeated calls means the prefix is drifting.

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
- **Put auth in front before any collection feature ships.** CLAUDE.md rule 5
  exists because a public inventory of expensive cards tied to a real identity
  is a targeting list — and that reasoning does not stop at `git`.
  **The auth core landed 2026-08-12** (`src/mtglab/auth/`, ADR 5 and ADR 16):
  accounts, Argon2id, sessions, the scoped accessor and the adversarial
  isolation test, all off unless `MTGLAB_REQUIRE_AUTH` is set. Cloudflare
  Access remains the recorded exit if the remaining half sprawls.

**Done 2026-08-12:** the `Dockerfile`, `fly.toml`, `.dockerignore` and
`docker-entrypoint.sh` are in the repository, CI builds and exercises the image
on every PR, and the corpus-and-decks seeding run is documented in HOSTING §4
step 6. A refresh cron deliberately does not exist — the refresh is monthly and
by hand, for the volume-attachment reasons in ADR 6; the runbook is the one
item §7 still lists as prose to write.

**Auth's server side is finished as of 2026-08-12** — the core (step 5), the
email half (step 5b: invites, password resets, tokens stored hashed and
single-use, the `EmailSender` seam), and admin authorization (step 5d, ADR 17).
That last one turned `is_admin` from a flag with no privileges attached into
enforcement: admin routes live under `/api/admin` and the middleware refuses
that prefix to anybody else before routing, `tests/test_isolation.py` has a
fourth classification checked against the prefix in both directions, and
`MTGLAB_ADMIN_EMAIL` reconciles the maintainer to admin at every start so the
standing requirement is a property rather than a setup step. The core also now
refuses to demote or disable the last admin who can sign in.

**The browser side landed the same day** (step 5c), so nothing code-side is
left between here and a deployment with auth on. The login screen and the claim
page are `web/src/routes/Login.tsx` and `Claim.tsx`, and the decision that
shaped both is in `App.tsx`: auth is a **gate** that replaces the whole shell
rather than a route beside the others, mirroring a server that refuses
everything outside `PUBLIC_PATHS` before routing. With auth off the gate is
never reached and the app is exactly what it was — no login, no sign-out
button, nothing — which is what `auth_required` and `authenticated` being two
fields has been for since the auth core. A 401 from any fetch is handled once,
in `lib/api.ts`, for the same reason the server's check is middleware. The
claim page reads `location.hash` and never the query string, and does not sign
anybody in; it hands the username to the login form, which is all
`POST /api/auth/claim` gives it.

The `mtglab users` CLI stays now that the admin UI has shipped rather than being
replaced by it: it is the bootstrap path — the first account on a fresh
instance predates anybody who could log in to create it — and the recovery path
when mail or the frontend is broken. `promote` and `demote` were added with
ADR 17, because `users.set_admin` had had no caller at all.

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
