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
   review; **all five have landed as of 2026-08-13**, and what stands between
   here and deploy is item 4 below. The numbers are identities, not positions, so nothing renumbers.
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
   [ADR 19](docs/adr/0019-the-dossier-cites-three-sources.md) written first. Branch 1 answered "what does this card do" with pool counts; this
   is the *interesting* half — who this character is, what archetype they
   define and where it came from, their rivals, where they sit in Magic's
   history. The second Claude mode, and the first whose facts do not all come
   from the pool.

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
     rival before writing about it, and the rival's real pool text rides in
     the payload so the card sits next to the sentence. The second half is what
     does not depend on the model complying.
   - **Cost:** about 800 uncached input tokens and 2,100 out per commander,
     with ~57k served from the prompt cache. Once per commander, ever, because
     the key is the `oracle_id`.

   Decided with the maintainer on 2026-08-12, so a session does not re-open
   these:

   - **Three sources, and a rule about which may support which claim.** Card
     facts — cost, type, text, legality, identity — come from the pool,
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
   - **Any card the model names is validated against the pool**, the way the
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

   **3 — Visual identity.** Built 2026-08-13. No ADR: the one decision that
   needed making had already been made — card art stays a Scryfall hotlink and
   everything else is **drawn in SVG/CSS**, no new binary assets, no licensing
   question — and nothing in the build moved it. The scope was the splash and
   the Sylvan Library art (rendered only on an *empty* library, so the
   maintainer had never seen it), an interactive colour pentagram for the mono
   tier, and the builder's tier headers, which were plain grey panels.

   What the build settled:

   - **The five mana symbols are drawn now**, which was not on the list — the
     maintainer raised it mid-branch, and it is the change that touches the
     most screens because `Pip` is shared by every cost, every prose pip and
     every identity ring. A lettered disc was a placeholder that had stopped
     looking like one. Drawn rather than hotlinked from Scryfall, and the
     reason is not licensing: **a pip is inline in a sentence**. The deck files
     carry 174 across `why`, `strategy` and `notes`, the gate's own errors are
     the densest of them, and `/api/colors` is the one page that works with no
     pool and no network. Card art is decorative and lazy; prose is neither.
     Checking Scryfall's own path data in would also be redistributing their
     asset rather than hotlinking it, which is the line rule 5 draws. ~2 kB,
     no requests, works offline.
   - **Only the five colours get an icon.** A numeral is a numeral, `{X}` is a
     letter, a hybrid is two colours no single glyph states — so the branch is
     `hasGlyph` and the text path is untouched. `{C}` is the one that is merely
     not done rather than argued against.
   - **A drawn pip had to be given a name.** A lettered one reached the
     accessibility tree as the character "G"; a drawing contributes nothing, so
     `role="img"` and the colour's name are explicit. Caught by an existing
     test that read the letters out of `textContent` — the failure was the
     feature working.
   - **The pentagram's geometry is derived, not tabulated.** Five vertices in
     WUBRG order; adjacent is allied, two apart is enemy. That yields exactly
     the ten guilds, once each, and the chords are what draw the star. Every
     name comes from the taxonomy the page already fetched, so there is no
     second copy of `colors.py` — pinned from both sides, by
     `tests/test_colors.py` against the table and by a frontend test that
     renames a guild in the data and watches the diagram rename it.
   - **The tier badges are the same wheel at 26px**, each with its own shape
     lit, which makes the two tiers people confuse self-explaining: a shard is
     an arc and mostly solid, a wedge is a span and mostly dashed. Same three
     dots, opposite texture.
   - **The `art_crop` ratio lesson has now been learned three times.** The
     hero band on the empty library is 3.08:1 over a 1.36:1 painting, so it
     kept **44% of the height from the middle** — bare wall, with the sky, the
     path and the three figures that give the canyon scale all cropped away.
     Branch 1 fixed exactly this on the deck hero and the library tiles and
     could not have caught this one, because the only screen that renders it is
     one an instance with decks on it never shows. The nameplate therefore
     shows the painting **whole, beside the title**, which is branch 1's own
     answer; the empty-library band keeps its crop but is anchored low, where
     the subject is.
   - **A dark-mode filter tuned on the wrong sample.** `brightness(1.75)`
     assumes card paintings are dark and vanish against a near-black panel.
     *Sylvan Library* is a bright yellow-green forest and it bleached — the
     app's own art was the one image the rule was never checked against. 1.3
     now, and the nameplate's copy gets 1.12 because it is the picture rather
     than a wash behind text.
   - **Moving the title into the nameplate took the empty library's `h1` with
     it**, found by running the app rather than by the suite. Fixed and pinned:
     exactly one `h1` and exactly one copy of the painting in both states.
   - **Cost:** entry chunk 262 kB → 266 kB (84 kB gzipped). The pentagram rides
     in the lazy `NewDeck` chunk; only the glyphs are in the entry.

   **4 — Teaching.** Built 2026-08-13. A vocabulary section for beginners;
   hover help in the simulator, whose parameters are words and numbers
   divorced from meaning; and real depth behind the guilds, shards, clans and
   colours — champions, plot lines, classic cards. No ADR: the one decision
   that needed making is recorded here and in `colors.py`'s own docstring,
   and nothing in the build moved it.

   The decision, made before any code: **the depth is checked-in reference
   prose, not a Claude surface.** A guild is exactly the kind of question
   ADR 19's dossier answers well, so it is worth writing down why not.
   `/api/colors` is the one page in the app that works with no card pool and no
   network — a stated property, pinned in `tests/test_isolation.py` — and a
   model call would spend it on the screen a brand-new player meets first. The
   set is finite: ten guilds, five shards, five wedges, written once, ever, so
   a per-view call pays repeatedly for content with no variance in it. ADR 20
   had already classed `colors.py` as a **fourth source** alongside the
   dossier's three — checked in, carrying `verified_by`, and free. And the
   complaint that produced the work was that the prose was *bland*; bland is
   fixed by editing, and only checked-in text can be edited. What Claude
   answers is the unbounded, per-deck question about a **commander**, which is
   ADR 19 and stays there.

   What the build settled:

   - **The teaching content moved out of a wizard.** The whole colour taxonomy
     rendered only inside "Start a deck" — a screen you pass through on the
     way to something else. Reference material reachable only mid-task is
     reference material nobody reads twice. `/learn` is a sixth nav item and
     the home of all three pieces; the create flow keeps a **short version**
     (the champions named, and a link across) so it stays a chooser.
   - **A champion is a character; the card is the evidence.** `Champion(card,
     role)` holds a card *name* and one sentence about who they are in the
     story — the only thing the card cannot say about itself. The page
     resolves the name through `get_cards` and renders the real card's cost,
     type and oracle text directly beneath the sentence, so a role that
     drifted from the card is visible next to what would disprove it. A name
     that does not resolve is **dropped and counted**, which is ADR 19's
     rivals instrument pointed at reference data.
   - **`signature` carries no prose at all, and that is the design.** Three or
     four cards per combination whose colour identity is **exactly** that
     combination — so the list asserts a checkable property rather than an
     opinion, and there is no sentence in it for a card fact to be wrong in.
     "Exactly these colours" is also the most direct available answer to what
     a combination is *for*.
   - **`exact_total` is counted live and teaches more than the paragraph does.**
     Exactly **two** cards in the pool have the Artifice identity, and the
     page says so. No four-colour blurb about refusing green lands as hard.
   - **144 card names, every one verified against the pool before it landed**
     — 51 champions and 93 signature slots. Two rules, and they are not the
     same rule: signature and `verified_by` must be *exactly* the
     combination's identity, a champion need only be a *subset*, because a
     faction is a story and the story owes the colour pie no exact match.
     Folded into the existing `needs_full_pool` test rather than added as a
     second one, so CI's skip gate stays pinned at two.
   - **The vocabulary is one table with two kinds of entry**, deliberately:
     Magic words (commander, colour identity, ramp, goldfish) and *this
     tool's own* controls and measures (`sim.min_pieces`,
     `stat.deployment_spread`). Both are words and numbers divorced from
     meaning to a newcomer. Keeping the simulator's half in `glossary.py` next
     to the `KeepRule` that defines it is what makes it checkable —
     `SIMULATOR_KEYS` in `tests/test_glossary.py` fails if a control on the
     screen has no entry, which TypeScript cannot do against a Python table.
   - **Twenty of the 32 get a story and twelve do not.** Mono-Red is not from
     anywhere. The test asserts that in both directions, because a non-faction
     with lore is a paragraph invented to fill a field.
   - **Two bugs only the running app showed**, which is now six branches for
     six. The colour wheel captions whatever `selected` it is handed, and on
     Learn that is any of the 32 — a four-colour key is neither a vertex nor a
     chord, so it found no edge, fell through, and described **Artifice as an
     "enemy pair — opposite on the wheel"** with a button offering to cross to
     the guilds. And the help popover is a descendant of the field label,
     which on the simulator is `uppercase tracking-wide`, so every sentence of
     help arrived SHOUTED AND LETTER-SPACED. Both are pinned now.
   - **Cost:** entry chunk unchanged at 266 kB (84 kB gzipped). `Learn` is
     lazy at 10.9 kB and the pentagram became a shared chunk, since two lazy
     routes now draw it.

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
   - **Cost and time:** a conversation turn is heavily prompt-cached (~48k
     cached tokens by turn three). It was described here as "a few seconds"
     until somebody measured it: **4.3–37.7s across eleven turns on the
     instance, with one at 133.8s**, and 27.7s for an ordinary turn driven
     locally. That sentence is why 5c exists. The proposal is the expensive
     half — **measured at 226 seconds** end to end with `max_uses: 4`, ~79k
     input / 8k output, since it reads a dozen-odd pages and checks every
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

   **5c — The conversation turn is a background job too.** Landed 2026-08-13,
   the third surface to make the same move and the one that had the weakest
   case on its own numbers. No ADR: as with 5b, nothing ADR 20 settled moved.

   - **The reason is not the outlier.** One turn in eleven at 133.8s did not
     reproduce, and restructuring a chat box on a single data point would be
     wrong. The reason is that `api/app.py` justified keeping it synchronous
     with *"it is a few seconds"* — **word for word the sentence that left the
     dossier synchronous until it broke deployed at 236s.** A duration measured
     for one surface is a question to ask of every sibling surface, and this
     was the sibling nobody asked.
   - **The ceiling is unknown, and that is the argument.** All anybody knows is
     that it is *at or below* 236s, because that is where the dossier failed.
     133.8s sits inside the unmeasured region below it. Measuring it properly
     would take a throwaway endpoint that holds a response open, a deploy to
     put it there, a binary search, and a deploy to remove it — for a number
     that is multi-hop (Fly's proxy, then Safari, then whatever network), that
     Fly can change without telling anybody, and that would not change the
     decision unless it came back above ~240s. Considered and rejected on
     2026-08-13; the fix costs less than the measurement.
   - **The failure being avoided is the bad kind.** A transport error carries
     no status code, writes no access-log line — uvicorn logs a response when
     it *completes* — and discards work that finished fine. That is exactly
     what the dossier looked like from a browser: a spinner, then `Load failed`.
   - **The cheap case stays one request.** A turn that reaches nobody — stance
     `off`, or a conversation past `MAX_EXCHANGES` — comes back as a job
     already `done`, and the client hands it straight to `followJob` as
     `initial`, which resolves without a single poll. Only a turn that actually
     calls Anthropic pays the 400ms poll.
   - **`key=None`, and that is the opposite of the dossier's call.**
     `jobs.submit(key=…)` collapses concurrent duplicates, which is right when
     two clicks inside four minutes are one question asked twice. A transcript
     is client-held, so two turns in flight are two *conversations*, and
     joining them would hand one of them the other's question.
   - **The route had no tests, which is how the dossier shipped and was very
     nearly how this did.** The proposal route had five; `/api/claude/theme`
     had zero, and all 259 tests matching "theme" passed against a module. Nine
     now cover the HTTP surface. Worth recording separately: the first draft of
     the born-finished test **passed against a mutation that removed the
     short-circuit**, because it asserted `status == "done"` on the response
     and a queued job satisfies that whenever the worker wins the race — which
     it always does when the work makes no call. The honest seam is
     `jobs.submit` never being reached at all, and the `no_worker` fixture is
     that. The identical weakness in the *proposal's* equivalent test was
     inherited and is fixed with it.

   Three things are already settled and should not be re-opened:

   - **It proposes; the user creates.** Nothing under `src/mtglab/claude/` can
     reach a write path, and `create_deck` is on the write surface
     `tests/test_claude_boundary.py` forbids naming. So the interview's output
     is a *proposal* — colours, then commanders — and the existing create flow
     is what makes a deck. That is the same shape the rationale interview has,
     arrived at from the other direction, and it is a feature: the deck is
     made by the person whose deck it is.
   - **Every commander it names comes from the pool.** The theme half is
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
     pool already carries both faces, and `CardRecord.front_type_line` exists
     because the commander dossier already had to care.
   - ~~**"Entomb" as the delete button's label for commanders.**~~ **Done
     2026-08-15, and it grew into
     [ADR 27](docs/adr/0027-entomb-is-the-delete-and-the-graveyard-is-the-undo.md)**
     after a live drive showed the card-level delete firing on one unconfirmed
     click — a handful of Gyome's cards died in an afternoon, unrecoverable on
     the instance where deck edits have no git history. Every delete label is
     Entomb now, red, and armed (first click cocks the button, second acts,
     four seconds disarms); a removed 99-card goes to a `graveyard:` list in
     `deck.yaml` with its `why` intact, with **Return** and **Exile** as the
     two ways out and a bulk sweep that is one all-or-nothing write. The typed
     confirmation for a whole deck **stays `bury`**, as he confirmed. The
     card-row buttons also stopped looking disabled (`card-action` classes —
     hover states, which inline styles could never say).

   *Content depth:*

   - ~~**The guild, clan and shard descriptions are bland**, at the macro level
     too — "the guilds of Ravnica are pretty famous. We can do better."~~
     **Done in branch 4**, which is what it turned out to be the brief for:
     every faction has what happened to it, two or three named champions with
     their real cards, and the cards that are exactly its colours.
   - ~~**Lore rivals on the commander dossier.**~~ **Done 2026-08-15**, after
     the first Gyome dossier showed why it mattered: the single "rivals" list
     answered the deckbuilding question while wearing the story's name, and
     the `who` section — the only one whose prompt gave no guidance at all —
     regressed to a mechanical description. The split: **Competitors** is the
     old list (pool-resolved cards, `get_cards` or dropped, unchanged
     machinery), **Rivals** is the story's — cited prose like `who` and
     `standing`, because a plot line is not a pool row, with the prompt
     explicit that a minor character honestly has none rather than an invented
     feud. `who` got its brief back: character first, mechanics belong to
     archetype. `DOSSIER_VERSION` bumped to 2, and the prompt fingerprint in
     the cache key means every stored dossier regenerates on next request —
     one call per commander, which is the price of the fix reaching Gyome.
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

   - ~~**Personas instead of a fixed question battery.**~~ **Started
     2026-08-13, [ADR 21](docs/adr/0021-a-persona-is-a-voice-and-the-spread-is-the-slots.md).**
     The complaint turned out to be half right and the half that was wrong is
     the useful part: `SLOT_KINDS` is a taxonomy handed to the model, not a
     script, so every sentence anybody reads was already generated. What was
     fixed was the *register*. So a **persona** is a voice and explicitly not a
     fourth stance axis — stance is how much the model does, persona is who it
     sounds like — and the voice is *appended* to the interview's instructions
     rather than replacing them, so its rules stay out of a persona's reach.
     `plain` and `fortune-teller` were built first; **the costumed five
     followed on 2026-08-15** — therapist, scientist, chef, storyteller,
     barkeep — and ADR 21's "a `Persona` and a prompt with nothing else to
     move" held exactly: the client changes for that PR were tiles and art,
     not plumbing. The same change merged the theme and tarot doors into one
     persona-grid door ("Help me decide"), so the reader picker is now the
     first thing the create flow's Claude door shows and the fortune-teller
     tile is where "Read my cards" went.
   - ~~**A tarot reading as a door of its own.**~~ **Built 2026-08-13** —
     backend first, then the door: a fourth entry on "Start a deck" that picks
     a reader from the roster, shuffles, deals three cards face down, and turns
     them over. The decision that made it possible rather
     than a rewrite: **the spread's three positions are `SLOT_KINDS[:3]`**, so
     a card is dealt *for* a slot and ADR 20's grounded-quote readiness works
     untouched — a card is not something the querent said, and the cards colour
     the questions rather than replacing the evidence. `tarot.py` holds all 78
     cards and no card's meaning; Python shuffles, the reader reads.

     **The licence was checked rather than assumed, and it matters.** The
     original 1909 Rider "Roses & Lilies" printing is public domain in both the
     US and the UK — but **US Games Systems' 1971 recolouring, which is the
     deck everybody pictures, is not.** All 78 files were verified per file
     through the Commons API; `src/mtglab/assets/tarot/PROVENANCE.md` is the
     argument. 4.6MB of WebP, shipped as package-data rather than through the
     committed bundle, which is why the `image` CI job now counts the cards.

     The door's own decisions, all small and all load-bearing. **The reader
     roster is fetched, never written in the client** — `/api/claude/personas`
     is free and needs no key, so the first screen of the most expensive door
     costs nothing, and the three unbuilt voices will appear there with no
     frontend change (a test pins that by putting a reader in the mock that
     exists nowhere in the app's source). **The card back is drawn**, for the
     reason the mana symbols are: 78 faces have a provenance file behind them
     and a back lifted off the internet would be a 79th image with no argument
     attached. **Persona is fixed for a conversation**, enforced by remounting
     the interview on its key rather than by a warning nobody reads, and a
     stash left by a different reader is discarded rather than adopted.
     **A reversal is a rotation on the `<img>`, never on the face** — the face
     is already rotated 180° about Y to hide behind the back, so putting both
     on one element makes a reversed card spend the flip un-reversing itself
     and land upright, which looks exactly like nothing going wrong. And the
     reveal gets a beat before the spread folds away, because the first version
     shrank the cards 840ms into a 1.6s stagger and resized the climax.

     Driven in a browser and screenshotted rather than described, per the
     habit that has caught eight bugs in eight branches. What it turned up:
     Vite proxies `/api` and nothing else, so package-data art 404s in dev and
     only in dev; a card back needs a hairline of light in dark mode, where a
     drop shadow against `#0d0d0d` says nothing; and "Your spread" was labelling
     an empty table for the reader who deals no cards.
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
4. ~~**Deploy**~~ — **done 2026-08-13. Live at https://sylvan-libraries.com.**
   [docs/HOSTING.md](docs/HOSTING.md) is the guide and was corrected the same
   day against what actually happened (#65).
5. **Testing the live instance** — started 2026-08-13, **not finished.** Email
   is proven end to end: a real invite, a new sending domain, Gmail, the claim
   link, a sign-in. Getting there turned up two bugs that only a deployment
   could show, both fixed and deployed in #66 — Cloudflare in front of
   `api.resend.com` refusing Python's default User-Agent, and the image not
   carrying the Anthropic SDK while the instance held the key.

   **Both lived below a test seam**, which is the durable lesson: mail is faked
   through `Transport` and the SDK is stubbed in every Claude test, so the
   whole suite passed while neither worked. `tests/test_packaging.py` is the
   first check that reads the *image* rather than the code.

   What is left: driving the app itself — the Learn page, the theme interview,
   the dossier, a deck edit surviving a restart. **The Claude surfaces became
   reachable on 2026-08-13** (`mtglab claude check` answers `pipe open` on the
   machine) and are still entirely unexercised there.

   Two punchlist items came out of the first real claim, both now built:
   choosing your own username at sign-up (#67), and **deleting an account**,
   which the first one turned up rather than predicted. `disable` was the whole
   revocation story and it does not release anything: `username` and `email` are
   `UNIQUE`, so a disabled row keeps both and an address cannot be invited twice.
   `users.delete` is the third door to a lockout and carries the same
   `LastAdmin` guard as `disable` and `demote`, with nothing to walk it back.

   The find worth keeping is not the feature. `users.id` is `INTEGER PRIMARY
   KEY` **without `AUTOINCREMENT`, so SQLite reissues a deleted account's rowid**
   — and jobs are held in memory keyed on exactly that integer. Delete the
   newest account, invite a replacement, and the new holder of the id inherits
   the dead account's jobs. `jobs.forget_owner` is the fix, and the class is
   worth naming: an isolation filter that is written correctly and defeated by
   arithmetic underneath it. Anything future keyed on a user id inherits the
   same trap; the alternative considered and not taken was a migration to
   `AUTOINCREMENT`.

   **The reset path is now proven end to end too** — a real reset mail, Gmail,
   the link, a new password, sessions revoked — but proving it turned up a third
   deployment-only failure. **A mail app can drop the URL fragment when you
   click.** The message left the server whole and the *visible* URL was whole;
   the click arrived at `/auth/claim` with an empty hash, which the server
   cannot see, because keeping the token out of every access log is exactly what
   the fragment is for (ADR 16).

   That made the failure terminal rather than annoying: a stripped link is
   indistinguishable from no link, so **"ask for a new one" produces one that
   fails identically, forever.** The fix is a paste field on the claim screen
   and a sentence in both messages pointing at it. The token still never rides
   in a URL — a paste is read on the client and posted in the body, which is
   the same rule the fragment was serving rather than an exception to it.

   The pattern across all three is worth naming: **every one lived below a seam
   the suite cannot reach** — a faked mail `Transport`, a stubbed SDK, and now a
   mail client's linkifier, which no test anywhere can exercise. What each
   needed was somebody using the thing for real.

   Also of note: the instance's host went unreachable for several minutes that
   day (machine suspended, volume on the same host, `no snapshots available`)
   and came back intact. **One machine, one volume, no snapshot** is the shape
   that was exposed — worth a decision before it matters.

   **The evening of 2026-08-13 put the first Claude output on the instance.**
   A dossier ran there — `claude-sonnet-5`, 77 pages searched, 5 cited, ~236
   seconds — and finding that out cost two more deployment-only bugs, merged as
   [#71](https://github.com/aasquier/sylvan-library/pull/71) and
   [#72](https://github.com/aasquier/sylvan-library/pull/72): a synchronous POST
   nothing had re-measured since ADR 20, and a bundle a browser cached
   heuristically and then black-screened on. Six dossiers are cached now;
   the reattach-after-reload contract is proven in Safari against the access
   log, which is the instrument for it — what you are looking for is one job id
   continuing across a document request, and a screenshot cannot show that.

   **A fourth deployment-only fault, found the same way, 2026-08-13.** Hours
   after the tarot art shipped, every one of the 78 pictures was being served
   as `application/octet-stream` — on the instance and only there. Starlette
   asks `mimetypes`, `mimetypes` asks the operating system, and the slim image
   has no `/etc/mime.types`; macOS and CI's ubuntu both know `.webp`, so no
   local check could see it, and browsers sniff and render anyway, so no remote
   *page* could either. It took reading the response headers. `api/app.py`
   names the type itself now and the `image` job asks the container.

   The shape is the point and it is now four for four: a faked mail
   `Transport`, a stubbed SDK, a mail client's linkifier, and the host's mime
   database. **Every one is a fact about the environment rather than about the
   code**, which is precisely what a test seam stands in for.

   ### The punchlist, 2026-08-13

   Five items, written down here rather than left in a session's memory. The
   first is fixed on the branch that carries this paragraph; the rest are open.

   1. ~~**No in-flight dedupe on the dossier.**~~ **Fixed 2026-08-13.**
      `plan_dossier` answered a *stored* dossier as a job born done, but nothing
      checked for a run already going for that `oracle_id` — so a second click
      inside the four-minute window started a second paid job with a second web
      search, and two ran concurrently on the instance that day. `Plan.key` and
      `jobs.submit(key=…)` are the fix: the lookup and the insert are one locked
      step, matching is per owner as well as per key (ADR 5 — two accounts
      sharing an id would give the second a 404 for a job it had just been
      handed), and only a *live* job is joined, because a finished one is what
      the cache is for and a failed one has to stay retryable. **This is the
      robust half of the reattach story** — the localStorage id only ever
      covered one tab; this covers a reload, a second tab, another device and a
      cleared cache, because the server is the thing that knows.
   2. ~~**Learn/Vocabulary renders `long` only.**~~ **Fixed 2026-08-13**, by
      rendering `short` as a lead line — the maintainer's call between that and
      rewriting the two offending paragraphs.

      **The count in the original note was wrong, and it is what decided it.**
      "35 of the 37 longs stand alone" does not survive reading all 37: around a
      third open as sentence *two*. The entire `stat.*` block does it as a house
      style — "The tail the median hides", "The cost of flooding, made into a
      number", "The sweep's answer, and only as good as the spread it sits in" —
      each commenting on a measure that only `short` ever names. **Commander
      tax** and **Mana base** were not two exceptions; they were the two the
      maintainer happened to open. So rewriting them would have fixed two
      symptoms and left a dozen, and left the next entry free to acquire the
      same defect.

      The rendering fix also closes a smaller hole: the search box at `:395` has
      always matched `short`, so it was possible to find an entry by text the
      page then refused to display. `glossary.py`'s docstring now states the
      contract the data always had — definition in `short`, argument in `long`,
      the page renders both in that order — so a new entry inherits it.
   3. **iOS Safari private tab lost the dossier reattach.** Unexplained — zero
      polls after the reload, while the theme key survived a reload in that same
      tab and planting a job id locally proved the read half works. P3, and (1)
      covers it in practice.
   4. **Fly volume snapshots still unconfirmed.** Volume created 13 Aug 14:11
      UTC with `snapshot_retention: 5` and `Scheduled snapshots: true`; the list
      was empty at 19:45Z. **Re-check any time after 14:11 UTC on 2026-08-14** —
      before that an empty list proves nothing, which is the same trap as the
      morning of the 13th.
   5. ~~**`users.id` → `AUTOINCREMENT`** migration still owed.~~ **Done
      2026-08-13**, as schema version 5. #68 fixed the instance;
      `jobs.forget_owner` covers the one caller that exists today, and this
      closes the class, so the next thing keyed on a user id does not have to
      rediscover it.

      AUTOINCREMENT cannot be added by `ALTER TABLE`, so it is SQLite's
      documented table rebuild — and **the rebuild is more dangerous than the
      thing it fixes.** `sessions` and `auth_tokens` reference `users`
      `ON DELETE CASCADE`, and `connect()` turns `foreign_keys` on *before* the
      ladder runs, so the obvious migration signs every account out and voids
      every unspent invite. Silently: a cascade is not an error. On the instance
      as it stands that is five live sessions and one outstanding invite.

      So `_apply_migrations` now turns the pragma off around the whole ladder,
      runs `PRAGMA foreign_key_check` before giving it back, and raises
      `MigrationFailed` rather than serving requests on a file that did not pass
      — the pragma is a no-op inside a transaction, which is why it cannot live
      in the migration script that needs it. The migration carries its own
      `BEGIN`/`COMMIT` for a second reason: `executescript` performs no
      transaction control, so a failure between the `DROP` and the `RENAME`
      would leave a half-built table and a version still at 4, and the next
      start would fail on a table that already exists — an app that never boots.

      Worth recording that the first draft of the *test* destroyed the rows it
      was written to protect, in its own setup, for exactly this reason.

      What it does not do is repair ids already handed out: the high-water mark
      is `max(id)` over rows that exist, and a deleted id is not one of them. It
      stops the next reissue, not the last one.

   6. **The curated library was writable by every invited account.** Found
      2026-08-14, minutes after the first non-admin claimed their invite and
      became a real second person on the instance. `deps.deck_source` handed
      the same `FileDeckSource()` to everybody and `FileDeckSource.writable`
      was hardcoded `True`, so `mitch` could swap cards in, or delete, any of
      the curated six. Recoverable — a delete moves the directory to `.trash/`
      and the decks are in git — but an edit to `/data/decks` is the live
      source of truth and nothing else records it.

      **Nothing was wrong with the classification; it was answering a
      different question.** `tests/test_isolation.py` files every deck route as
      *shared*, with reasons like "edits a shared deck", and that is correct
      about **reading**. Before there was a second account, "everyone sees the
      same decks" and "everyone may edit them" were indistinguishable
      statements. An invite made them different and nothing in the suite was
      watching the seam: the read-only path *existed* in `service.py` and was
      dead code, because no source ever reported `writable=False`.

      Fixed by deriving writability from the caller in the one place that
      already decides what a request may see, and by collapsing four bespoke
      refusals — `EditRejected`, `CreateRejected`, `DeleteRejected`,
      `ImportRejected`, each chosen to match whatever its own route caught —
      into `ReadOnlySource`, handled once as a **403**. It answered 422 before,
      which was defensible while read-only was a property of the *source* and
      is not once it is a property of the *caller*: nothing is wrong with the
      request except the person making it. No client had ever seen the 422,
      since the path was unreachable.

      Two things worth carrying. **The test that would have caught it is the
      one nobody writes**: `test_api.py` covered read-only sources through
      `dependency_overrides`, proving what a route does *when handed* one, and
      never that anybody is handed one. Same shape as the dossier's missing
      HTTP tests. And **`/api/decks` is no longer byte-identical per caller** —
      it carries `writable`, which is about the viewer — so the "shared really
      is shared" assertion had to get sharper rather than looser: every field
      but that one is identical, and that one must actually differ.

      **The consequence is deliberate and is not the end state.** There is
      nowhere for a non-admin's decks to live, so the app is read-only for
      them, the three "start a deck" doors included. That is the correct
      interim answer — the alternative is their decks landing in somebody
      else's library — and it is the argument for doing the ownership tier
      next rather than eventually.

   7. **Deck ownership and sharing — built, both halves
      ([ADR 22](docs/adr/0022-decks-have-owners-and-sharing-is-a-flag.md)), and
      not yet exercised on the instance.** Asked for 2026-08-14: people should be able to
      show each other their decks, the maintainer's should always be visible,
      it should be a tab somebody opts into rather than something in the way,
      and other players' decks should be organised **by username**. Leaderboards
      and macro deck stats are named as later work on top of it.

      **Decided, on the branch `deck-ownership-and-sharing`:**

      - **Paths are owner-qualified — `/api/decks/{owner}/{slug}`.** Slugs are
        unique *per owner*, never globally, which is what stops "is this slug
        free" from being a question about everybody's private decks at once. A
        global namespace was rejected for exactly that leak; an opaque deck id
        was rejected for breaking the slug/directory correspondence ADR 1 keeps
        permanently.
      - **Sharing is a per-deck flag.** Curated six shared by default (absent
        means shared, so they are never silently hidden); a deck in the SQL
        tier is **private** by default, because `decks import` writes 99 empty
        `why` fields and publishing that instantly is nobody's intent.
      - **The 403/404 split resolves as one sentence: *403 is only ever an
        answer about a deck the caller can already read.*** A private deck is
        absent from the source, so every verb answers 404 — writes included,
        because a 403 there confirms it exists. A shared deck answers 403 to a
        write, which is item 6's answer unchanged.
      - **The file tier's owner is a rule, not a column.** `MTGLAB_ADMIN_EMAIL`
        names them; unset, the six fall back to `local` and stay visible, since
        the alternative is an instance whose showcase nobody owns and therefore
        nobody sees.

      **What that cost, and what it caught.** Two bugs the sweep found rather
      than the design: the sim routes take their slug in the *payload*, so they
      resolved a deck by name with nobody asked whose it was; and
      `_for_writing` refused on writability before resolving the deck, so a
      write to somebody's private deck answered 403 and confirmed it. Both are
      the same shape as item 6 — a check that was correct while there was one
      library. `tests/test_isolation.py` files every per-deck route as
      **user-scoped** now, with ten new adversarial tests.

      **The browser half, and the two things it needed that the server half did
      not have.** Every deck call takes a `DeckRef` — `{owner, slug}` as an
      object rather than two positional strings, because transposing two
      strings is a runtime 404 against somebody else's library and named fields
      make it a compile error. `deckUrl` is the single place an in-app deck link
      is built, and `lib/api.test.ts` asserts the **URL shape** directly:
      a screen mocking `api` passes its tests while the real client asks for a
      route that no longer exists, which is precisely the failure this half
      exists to prevent.

      - **`GET /api/decks` gained one field, `showcase`.** The browse tab needs
        three groups out of one flat list — yours, the showcase, everybody
        else's — and could only work out two. `writable` identifies the
        caller's own decks; *nothing* identified the maintainer's, because the
        client is never told who that is. Inferring it from the response's
        order was the alternative, and ordering is not a contract.
      - **`/decks/:slug` survives as a resolver, not as a deck route.** That
        was every deck's address for the life of the app and the instance has
        been driven for days, so a bookmark or a link sent to a friend still
        works: it looks the slug up and redirects, first match winning, which
        is your own deck before the showcase before a stranger's because that
        is the order the library is listed in.
      - **The authoring doors are no longer gated on `is_admin`.** That gate
        said it would disappear rather than move when decks got owners, and it
        did — everybody has a library to put a deck in now.
      - **The sharing toggle is the deck page's, owner-only.** Without it a
        SQL-tier deck is private forever and the browse tab can never have
        anything in it.

      **Owed: exercising it on the instance.** A non-admin account driving the
      write gate, ADR 5's 404 and ADR 17's 403 against the deployed app, which
      is where every fault in this project has actually lived.

   Still owed from the test list itself: **the theme interview on the
   instance** (both modes, and now both readers) — the deployed React half,
   specifically; the environment itself is proven.

   **Done since:** a deck edit surviving a machine restart (2026-08-13), and
   **delete → re-invite → claim with a chosen username**, completed
   2026-08-14T00:25Z. The claim is worth a sentence because it was open for
   three sessions on a wrong theory: a stripped URL fragment was suspected, and
   `POST /api/auth/claim/preview` answering 200 disproved it — that call reads
   the token and spends nothing, so its success means the fragment had been
   arriving the whole time and **the claim had simply never been attempted.**
   An absent request is the proof of a stripped link *and* of a thing nobody
   did; the log cannot tell them apart, and only a live tail during a real
   attempt could.
6. ~~**A second quality pass.**~~ Landed 2026-08-14. Not a feature branch —
   vocabulary, documentation, lint reach and workflow hygiene, done while the
   instance was already live.

   - **"Corpus" is now "the card pool"**, across 1,016 occurrences in 106
     files: prose, comments, identifiers, UI strings, the pytest marker, and
     the `/api/health` and deck-response **wire fields**. The word came from
     linguistics rather than from Magic, and "card pool" is a term the game
     already uses for the set of cards you may build from. **ADRs 2 and 7 keep
     the old word** — they are records of what was decided and how it was said,
     and `docs/adr/README.md` carries a note saying both names mean one thing.
     One place resisted the rename and was more interesting than the rest:
     `tests/mana_oracle.py` used "corpus" for its enumerated set of
     differential test cases, a different sense entirely, and the file already
     used `pool` for a pool of mana sources — a blind sweep produced
     `pool_pools()`. Those are `all_cases()` and a **case set** now.
   - **The README is a README**, not a tutorial: 297 lines to 180, leading with
     the idea rather than with `pip install`. The setup path, the full command
     reference and the deck workflow moved to `CONTRIBUTING.md`, which is where
     somebody who has decided to use the thing actually looks.
   - **Stale claims across the docs**, nearly all of one kind: they were
     written before the deploy and still said so. The ADR index stopped at 20
     with 21 and 22 on disk, described 14 and 15's modes and stances as
     unbuilt, and called 4 and 5 decisions about "a deployment that does not
     exist". HOSTING §7 framed itself as a pre-deploy checklist. Three
     documents disagreed about the size of the mypy exemption list; two said
     "ten", `pyproject.toml` said eight, and eight was right.
   - **Ruff's rule set widened** to add C4, RET, PTH, TID, PIE and RUF, chosen
     by measuring each against the tree rather than by taste — three reported
     nothing at all. RUF earns its place on RUF100 alone: **71 `# noqa`
     directives were suppressing rules that no longer fire**, and a dead
     suppression reads exactly like a live one.
   - **Workflow hygiene**: `permissions: contents: read`, a `concurrency` group,
     `timeout-minutes` on all four jobs, and pip caching. See ENGINEERING §5.
   - **Five new tests** on the three background-job error translations, which
     had none. That is the code deciding whether an expired key reads as "your
     key may have expired" or as a stack trace in a job's error field — and the
     key has a fixed lifetime, so it is a question of when. Verified by
     mutation, not by going green: stripping `explain()` fails all three.
   - **Deferred, with the measurement written down:** `noUncheckedIndexedAccess`
     (51 errors across 15 files, ENGINEERING §4) and `ruff format` (101 of 111
     files, ~15,000 lines, and it would fight the deliberate argparse table).
     Both are their own change for the same reason `strict` was.

     **Both decided 2026-08-14.** `ruff format` is a **no**, recorded as
     [ADR 24](docs/adr/0024-no-python-autoformatter.md) — the first rejection in
     the directory, because "why is there no formatter?" is a question that will
     be asked again and an answer nobody wrote down gets relitigated. The
     deciding measurement was not the diff size but the line-length one: 117
     lines of 39,823 over 88 characters, and 60 of the 61 over 100 are oracle
     text in `tests/tiny_pool.py` that a formatter cannot split. The discipline
     it would impose is already there. `noUncheckedIndexedAccess` is a **yes**,
     on its own branch — re-measuring found the 51 errors cluster into ~15
     distinct sites, several of whose fixes are strictly better code.

     **Both landed 2026-08-14.** The flag is on, all 51 are fixed, and no
     non-null assertion was added under `web/src` outside test files. The one
     finding worth carrying: **a tuple type does not satisfy the flag** — only
     a literal index escapes it, so `WUBRG[(i + 1) % 5]` is `string | undefined`
     whether `WUBRG` is an array or a five-element tuple. The pentagram's edge
     list is built by walking a rotated copy in lockstep instead.

7. **After that, next build work in order:** ~~re-price automated PR review~~
   (done 2026-08-14 — **still parked**, and now for a measured reason: 87 PRs
   in five days is 17.4 a day, and a Sonnet 5 review of a median PR costs $0.50,
   so **$262/month against the $10/month Copilot Pro already rejected on
   price**. Not close, and not fixable by model choice. ENGINEERING §5 has the
   table.), the stance dial UI, then the remaining Claude modes ADR 15 names and
   branch 5 does not build (argue a slot, deck conversation, research).

   The re-price's **useful** finding was incidental to it: `web_dist/` is **75%
   of the median review's input and 88% of the worst** — PR #87 is 305,735
   tokens whole and 15,216 without the bundle. That is a bill being paid today,
   because `/code-review ultra` is billed and does get run, and #81's 865,448
   tokens is close enough to the 1M window to lose the diff. Exclude the bundle
   from any review diff.

   **The stance dial landed 2026-08-14** and was a prerequisite rather than a
   peer, which only became clear once the code was read: ADR 15 gives **deck
   conversation** its reversible edits *"at the top stance only"*, and until
   this no client could ask for a stance at all — every surface sent none and
   took the deck-derived default, which caps at `SECOND_OPINION` (`write:
   none`). That mode's defining capability was unreachable. It is reachable
   now.

   Building it found a bug forty-two tests had missed, and the shape of the
   miss is the transferable part. The create flow has no deck, so
   `/api/claude` resolved through `stance.resolve(None, None)` and answered
   `off` — while `theme.stance_for` was about to run that conversation at
   `second-opinion`. Every test of that endpoint passed, because every one of
   them named a deck; the case with no deck was the case nobody wrote.
   **Rendering a value is what audits it.** The number had been served since
   ADR 20 and nothing had ever had cause to look at it, and it took putting it
   on screen next to the thing it describes. `/api/claude` takes a `surface`
   now, and each surface's default is asked of the module that owns it.

   **Argue a slot landed 2026-08-14**, and the ordering was argued rather than
   taken in table order — the reasoning is worth keeping because it corrects
   the paragraph above. The dial made `write: proposes` selectable; ADR 15's
   phrase for deck conversation is *"the reversible edits, at the top stance
   only"*, and `stance.py`'s own table maps *reversible edits* to
   `write: applies`, which **is not a preset**. `COLLABORATOR` is the top
   preset and it is `proposes`; `lib/stance.ts` pins a preset *name* and
   nothing else, so no client can express an axis. Below that sit four locks
   that each say in their own words that moving them needs a superseding ADR:
   no write tool in `tools.READ_ONLY`, `Mode.__post_init__` refusing a
   non-empty `may_write`, `test_claude_boundary.py` forbidding the *mention*
   of a write function anywhere under `src/mtglab/claude/` — including
   `remove_card` and `set_card_field`, the two ADR 15 says are
   autonomous-safe. Plus a sim-results tool that `service.py` does not have.
   **Deck conversation is the largest of the three, not the unblocked one.**

   The fifth item on that list — the activity log — landed 2026-08-16 as
   ADR 28, which leaves the four locks and the missing tool. Each lock names
   the ADR that would have to supersede it, so none of them is work; they are
   arguments somebody has to make.

   So argue a slot went first, and not only because it was cheapest. It is
   the mode nearest the boundary while the stakes are lowest: its whole output
   is declarative prose about a card's merit, which is exactly what
   `only_questions()` exists to delete from the interview, so it forced the
   question of what guards that. The answer is
   [ADR 25](docs/adr/0025-argue-a-slot-argues-one-direction.md) — **it argues
   one direction**, and the schema has no field for the case in favour. A
   balanced version would return a finished `why` grounded in the user's own
   deck, and a UI that declined to render it would not be a guard, because the
   CLI renders the same payload and the endpoint is public.

   Three things the build added that the ADR did not need to name. The
   alternatives it offers are **bare names judged by Python** — resolved
   through the pool and dropped if invented, banned, or outside the colour
   identity, counted separately in each case — which makes the *Ajani, Nacatl
   Pariah* error in CLAUDE.md an assertion that runs on every PR rather than a
   story. Writing that filter found a real bug: a double-faced card comes back
   under its full `A // B` name, so an index keyed on the pool's spelling
   dropped every DFC named by its front face, **silently**, which is the one
   thing the function exists not to do. And the UI test for "this is the case
   against" passed against a heading relabelled *Assessment*, because it was
   matching the `never` sentence in the payload — **a test asserting the
   server's own text back at itself is not testing the renderer**, and it was
   only caught by mutating the code rather than by going green.

   **Research landed 2026-08-14** — `src/mtglab/claude/research.py`,
   `mtglab claude research "<question>"`, `POST /api/claude/research`, and a
   `/research` page of its own in the nav, with
   [ADR 26](docs/adr/0026-research-answers-about-magic-not-about-your-deck.md)
   written first. Sixth mode, fourth feature, ADR 15's fourth table row.

   The unsolved problem this entry used to name — *it has no narrow contract to
   check against* — has an answer, and the answer turned out not to be about
   facts at all. **The contract is that the mode cannot reach a deck.** No
   `DeckSource`, no slug, no deck tool, and the route sits at
   `/api/claude/research` rather than under `/api/decks/{owner}/{slug}`. That
   does two jobs with one absence: rule 4 is out of reach because there is no
   rationale to read and no 99 to be asked what to cut from, and **deck
   conversation cannot be built by accident**, which was the real risk — the
   question "should I cut X from my deck" is what somebody types on day one,
   and answering it well is deck conversation under another name, with none of
   the five things ADR 15 still owes it settled.

   Three things the build settled that the ADR did not have to name:

   - **A card the pool lacks is labelled, not dropped, and that is the one
     place research must differ from the dossier.** ADR 19 drops an unresolved
     rival because a rival that does not exist is an error; a card *spoiled
     since the last `data refresh`* does not exist either and is one of the
     three things this surface is for. So both are kept and marked
     `in_pool: false`, counted separately from the dropped counts because that
     number is not a fault. Applying the dossier's instrument here would have
     made the mode silently worst at its own best use.
   - **A finding whose citations all failed the check is dropped, not
     narrowed.** One step past what `dossier._section` does, and the reason is
     that a dossier passage may rest on the brief it was handed while research
     has no brief — its subject is whatever was asked, so `get_cards` is the
     only pool door rather than a second one.
   - **265 seconds, measured on the first real question.** Longer than the
     dossier's 236s, which is the duration that broke deployed. It was a
     background job from its first commit rather than after an incident —
     the first Claude surface here of which that is true — and the route had
     tests before it had a deploy, which is the other half of that lesson.

   What is left of item 7 is one mode: **deck conversation**, and it is now
   *harder* to build rather than closer, deliberately. Anything that wants a
   deck inside a Claude surface has to supersede ADR 26 and say what it does
   about the five things listed under the stance dial above.

8. **An efficiency pass against a stated load** — landed 2026-08-14. The
   target was named rather than assumed: 100 accounts, 10 concurrent, one
   `shared-cpu-1x` machine. Measured first, on the real pool and the real six
   decks, and the findings were not where intuition pointed:

   - **PyYAML was the shelf.** `yaml.safe_load` takes the pure-Python path
     even with libyaml compiled in — the C loader is opt-in per call — so each
     deck file cost ~36ms to parse and the shelf spent more time in YAML than
     in DuckDB. `model.load_yaml` is the one entry point now, `edit.py`
     included: 36ms → 7ms per deck, the shelf 430ms → 245ms, the deck page
     228ms → 124ms, with a pure-Python fallback where libyaml is absent.
   - **"Nothing on the wire compressed" — half wrong, and a skipped deploy
     proved it.** The premise was that Fly's proxy passes bodies through; a
     flaky frontend test skipped the deploy, which left the old code live long
     enough to catch it answering `Content-Encoding: gzip` already — Fly's
     edge compresses on its own, undocumented. `GZipMiddleware` stays for what
     was measured once the deploy landed: the app's level-9 gzip puts the
     bundle at 84.5 kB where the edge's compressor sent 119.6 kB, the
     behaviour is owned rather than inherited from one host, and `mtglab ui`
     on a laptop has no edge. Registered *innermost* because `minimum_size`
     reads Content-Length and the decorator-style middlewares re-wrap every
     response as a stream without one — registered outermost it compressed
     two-byte job polls. Two tests pin both sides, verified by mutation. The
     flaky test read the tarot stash the instant 'The Root' rendered, before
     the writing effect flushed, and waits now.
   - **The session lookup could block the event loop.** `sessions.lookup`
     writes — the five-minute `last_seen_at` touch, the delete of an expired
     row — and a write that finds the file locked waits up to `busy_timeout`,
     five seconds, on the loop, stalling every request in flight rather than
     this one. It runs in the threadpool now. The docstring that kept it
     inline priced the hop against the read alone, which is the "it is a few
     seconds" shape at a smaller scale.
   - **`jobs.MAX_JOBS` was sized for a laptop.** Fifty global slots shared by
     100 accounts evicts a finished job somebody's tab is still polling —
     cache hits are born finished, so ordinary use fills the registry with
     exactly the jobs eviction takes first. 200 now.
   - **Measured and deliberately left alone:** the per-request DuckDB connect
     (~15ms, but holding one open would lock `mtglab data refresh` out of the
     volume for the life of the process — the transient handle is what keeps
     that workflow possible), and `get_cards`' query shape (an array-bind
     variant saved nothing; the ~200ms shelf union is the scan itself, and
     the 2026-08-14 CTE rewrite already took the cheap half).

9. **Coverage to ~96%, floor to 95** — landed 2026-08-14. The suite stood at
   90% (both CI and local; the old two-point gap between them is gone) and the
   floor had been 90 since 2026-08-12. A deliberate pass took it to ~96 — the
   ground gained is itemised in `pyproject.toml`'s `fail_under` comment, and
   the largest single piece was the CLI's three Claude renderers, which had
   *no* output tests at all: the argue, dossier and research printing is where
   ADR 14's "say which system answered" lives in a terminal, and none of it
   was pinned. Also new: the theme modes' full call path faked at `Turn`, the
   Forge run faked at `subprocess` (seat mapping, the dropped-card refusal,
   the no-games refusal), the Scryfall ingest against fake bulk files, and
   ADR 22's `SqlDeckSource` exercised directly — create/read/update/delete,
   the freed slug, and the private-is-absent rule, which had only ever been
   tested through the routes. The floor sits a point under the suite so a
   change that costs a full point is loud and ordinary churn is not.

10. **A shore-up pass** — landed 2026-08-15. A full audit first (every screen
    driven in the browser, desktop and phone, both themes; suites, ruff, mypy
    all green; no console errors anywhere), then the gaps it found:

    - **The page nameplates.** The library's masthead — a whole painting at
      its own ratio beside the title, credited — was the app's best screen
      and four screens were plain grey next to it. `PageMasthead` in
      `components/ui.tsx` is that layout made shared (the library uses it
      too now), and Card search, Simulator, Research and Import each carry a
      painting from the **Strixhaven Mystical Archive** cycle — an archive
      of the game's definitive spells, for an app named after a library:
      *Demonic Tutor* (search your library for a card), *Strategic Planning*
      (look at the top three, keep what the plan needs), *Compulsive
      Research* (draw three, keep what survives scrutiny), *Cultivate*
      (search for two, keep both). All hotlinked and credited, chosen by
      printing id resolved through the pool, none committed — the branch-3
      decision unchanged, just applied to more screens. Each attribution
      clause was checked against the card's oracle text, because rule 1
      does not stop applying when the card fact is in a caption.
    - **CodeQL** (`codeql.yml`) — see ENGINEERING §5. The one scanner that
      reads the source; free on a public repo; not a required check until
      its signal-to-noise has been watched for a few weeks.
    - **A preconnect to `cards.scryfall.io`** in `index.html` — every screen
      hotlinks card art from that one host, and the handshake now happens
      while the bundle parses instead of in front of the first painting.
    - **Stale doc claims**: ENGINEERING §5 still said the coverage floor was
      90 (it is 95, item 9) and the mypy exemption list was eight (it is
      two, #90); CONTRIBUTING said five required checks (six) and described
      a local-vs-CI coverage gap that item 9 closed.
    - **`web/README.md`** — the frontend conventions map for a fresh
      session: what serves what, the load-bearing conventions (lazy routes,
      `DeckRef`, the stance readout, `hasGlyph`, glossary keys, the
      masthead rules, both-themes), and the testing habits with a history
      behind them.

11. **A Claude cost pass** — landed 2026-08-15, from an audit of the client
    against current API guidance. The audit's headline was that the
    integration was already clean (no deprecated parameters, `pause_turn`
    resumed, refusals handled, the system-block cache breakpoint in place);
    what it found were the two things below, built together:

    - **The tool loop now caches its own history.** `converse` kept one
      breakpoint, on the system block — so turn six of a dossier re-bought
      turns one through five, search results included, at full input price.
      A second, *moving* marker now rides the newest tool-result block each
      turn (moved, not accumulated: the API allows four markers and the
      theme flow already spends one inside `messages`). Cache reads bill at
      ~a tenth of the input rate, so this is the searching modes' largest
      single saving.
    - **A usage ledger** (`claude/ledger.py`, `claude_usage` in app.db,
      schema v7). Every mode counted its tokens and the CLI printed them,
      but the hosted instance — where the spending happens — discarded them
      with the job payload, so "what did this month's dossiers cost" had no
      answer. `converse` now records every conversation on every way out
      (answer, refusal, and the turn-ceiling exception, whose burned tokens
      are exactly the ones worth seeing); `mtglab claude usage` is the
      roll-up, per mode, most expensive first. Deliberately aggregate —
      counters, a mode name, a model id; no user id and no question text —
      so ADR 17's who-may-read-what argument never has to be made for it.
      Tokens and not dollars, because prices move (Sonnet 5's introductory
      rate ends 2026-08-31) and a stale price table is a wrong invoice.

    Deferred until the ledger has numbers to argue from: an effort A/B
    (`medium` on the brief-fed modes), a per-mode model field (Haiku on the
    mechanical modes via the `MTGLAB_CLAUDE_MODEL` machinery), and a
    Batch-API warm command for post-`DOSSIER_VERSION`-bump regeneration at
    half price. Rejected outright: doing our own web searches and pasting
    results into prompts — it would dismantle the `keep_sources` check
    (ADRs 19/26 verify citations against pages the search *actually
    returned*, in-band) and it is the crawler CLAUDE.md already bans, for
    savings measured in cents.

12. **The photo-real pass** — started 2026-08-15, from the maintainer's second
    punch list of that day, whose through-line was "no more clip art." The
    tooling question ("do we need third-party software for animation and
    photo-real imaging?") was asked three times in that list and is answered
    once, here, so no session re-litigates it:

    - **No third-party animation software.** Lottie and friends produce
      vector output — the exact aesthetic being evicted. The material is
      **real images**: Magic's own paintings (hotlinked from Scryfall with
      credit, the posture the app has always had) and **CC0 photography
      committed as assets** when a thing must be ours — found via Openverse
      with a licence filter, checked per file, recorded in a `PROVENANCE.md`
      per asset directory (the tarot deck's rule).
    - **The pipeline is scripted Pillow** (a dev-only dependency): fetch,
      matte, tile, measure, encode WebP. Measurements against a painting
      (the wheel's circle, the Lab's glassware) are fitted numerically and
      recorded in the component that uses them.
    - **Motion is transforms and particles over real images** — CSS masks
      and rotations of the artwork itself (the Wheel of Fortune spins the
      painted wheel), Ken Burns drift on gallery lanes, and a canvas
      particle layer (`components/vapor.tsx`) for volumetric steam/smoke.
      Video loops stay on the table for later, but nothing so far has needed
      one; if one ever does, it is a committed asset with a PROVENANCE
      entry like any other.

    Landed so far under this heading: the painted wheel spin (#116), and the
    photo ivy canopy + Experimental Lab bench + gallery lanes (this branch).

    **The room was invisible from #118 until 2026-08-16, and the reason is
    worth keeping.** Driving the deployed instance, the maintainer reported
    every backdrop as "just a bland background… not present in the least."
    Three faults, stacked, none of which any test could see:

    - **The gallery lanes never rendered at any width.** `.scene-lane` is an
      `<img>` — a *replaced* element — given `top: 0; bottom: 0` and no
      explicit height. `height: auto` resolves from the intrinsic aspect
      ratio, the box is over-constrained, and `bottom` is the declaration
      dropped. Measured at 250x145 where 250x1000 was intended: painted,
      present, and far too small to read as anything.
    - **Every fixed backdrop was trapped for the first 300ms of each page
      view.** The routed page is wrapped in `.page-enter`, which animates a
      `transform`, and a transformed ancestor becomes the containing block
      for `position: fixed` descendants. `SceneBackdrop` portals to
      `document.body` now; anything fixed added under that wrapper needs the
      same treatment.
    - **The wash was tuned below visibility** — 0.13 opacity behind an 18px
      blur behind a mask fully transparent for its top 26%, an effective peak
      near 0.09. The sunbeam was worse and for a different reason: pale
      yellow on a near-white page is light-on-light, so more alpha could
      never have fixed it. It is amber now; the warmth is the signal.

    Two pages had no room at all (`/learn`, `/new`) and now wear mastheads
    like their four siblings. The lesson generalises past this feature: **a
    test that asserts an element renders has not asserted that it has a
    size**, and jsdom cannot close that gap — only a browser measuring a real
    box can.

    **The pipeline is real now — `mtglab animist`, 2026-08-16
    ([ADR 29](docs/adr/0029-an-asset-is-committed-only-with-a-recipe.md)).**
    "Scripted Pillow" had been a description of scripts that were never
    committed; it is now a package (`src/mtglab/animist/`, Pillow behind the
    `animist` extra) driven by a `*.recipe.yaml` beside each asset directory:
    fetch from Openverse/Commons, **licence gate per file through the
    provider's API with no override**, transform (matte, feather, tile,
    resize), encode WebP with metadata stripped, write the PROVENANCE entry,
    and `verify` holds every committed asset to its recipe's `expect` block
    in the test suite. Both founding pipelines (ivy, tarot) are reconstructed
    as committed recipes. Wizards' art stays runtime-animated only — the
    pipeline deliberately has no provider that takes a Scryfall URL.

    The first two later phases landed 2026-08-16 as **ADR 31**: procedural
    motion (a seeded, loop-perfect `spectral_noise` generator plus `advect`,
    `color_ramp` and `ken_burns`, with a `procedural` source whose
    declaration — its seed — is the source) and the animated formats
    (`awebp`/`apng` through Pillow, `webm`/`mp4` through `imageio-ffmpeg`,
    crf-controlled, dual-shipped for the Safari floor, never in the image).
    `verify` reads video through the same bundled ffmpeg that wrote it, and
    `measure` sweeps crf where a video output is the subject.

    The whole chain is live as of the same day. The first procedural asset
    shipped — `mist.recipe.yaml`, seed 6161, the forest mist every room's
    floor now carries through `SceneBackdrop`, budgeted at `measure`'s crf
    knees (webm 40, mp4 30) — and **2.5D depth parallax runs for real**:
    ADR 32's runtime tier derived depth-drift loops for all six commanders
    with Depth-Anything-V2-Small on this machine, the browser plays them
    through `CommanderMotion` (WebGL tilt where a depth map ships, the
    baked loop elsewhere, the still as the floor), and the derivatives
    travel to the instance over sftp, never through git.

    One trap worth the paragraph: torch 2.2.2 is the **last** torch with
    macOS x86_64 wheels and it predates numpy 2, so the `depth` extra pins
    `numpy<2` and lives in **its own venv** on this machine
    (`.venv-depth`); the main venv never downgrades. The extra's comment in
    `pyproject.toml` records the whole argument.

    The committed painting landed 2026-08-16, chosen with Aaron over the
    alternatives (Rembrandt's Philosopher, Böcklin's Isle, Friedrich's
    Wanderer): **Spitzweg's *Der Bücherwurm*** (bookworm.recipe.yaml — a
    wikimedia source through the gate, resize, then a `ken_burns` breath
    with `bounce`), hanging framed at the foot of the Learn page. Two more
    procedural loops joined the mist the same day — **mana wisps** (seed
    1909, the fortune-teller's table and room; `color_ramp` grew a `gamma`
    for them, because a wisp is mostly the dark around it) and **candlelight
    embers** (seed 1666, the Laboratory) — behind a `mood` prop on
    `SceneBackdrop`, and the mist itself was softened (falloff 2.7, advect
    5) after its chewed edges read as moss. Sprite sheets and the runtime
    sprite-sheet player remain open; a committed depth map for the painting
    (true parallax rather than the breath) is the natural next step and
    needs the where-does-a-depth-map-live story from ADR 31.

    From Aaron's 2026-08-16 eye pass: **motion derivatives are a
    dev-machine artifact, and the deployed instance has no way to grow
    one.** `mtglab cardmotion sync` (2026-08-16) is the dev-side answer —
    every deck's commander, from the printing the deck shows, art swaps and
    imports both — but a deck imported *on the instance* shows a still
    until somebody runs the sync here and pushes.

    **Decided 2026-08-16 (at Aaron's direction, in session): the still is
    the intake-time story.** A deck imported on the instance shows its
    commander's still — the browser ladder's designed floor, not a
    degraded state — until the next dev-machine `cardmotion sync` + push,
    which joins the end-of-session ritual rather than a schedule. The
    other two options lost on their own terms: in-container `slow-pan`
    needs ffmpeg in the image, which ADR 31 deliberately keeps out
    ("dev and CI only, never the image"), and spends shared-machine CPU
    upgrading a decorative layer a few hours earlier; a scheduled
    unattended sweep is automation on a laptop that sleeps, and a
    schedule that silently doesn't run is worse than a ritual that
    visibly does. Three named triggers reopen this, and the shape it
    would take is a NET job (never the request, per ADR 32): another
    pilot's instance-imported deck visibly living on a still long enough
    for a human to mind; a second Fly machine appearing; or ffmpeg
    entering the image for any other reason.

13. **The tarot overhaul** — started 2026-08-16, under the new commandment
    15: the reading is a gift for Aaron's sister, and it gets the best of
    everything. Aaron's brief is eight items; the phase runs them as five
    PRs, each verified by eye through the Playwright WebKit rig before it
    lands (the pane screenshots black on this machine; headless WebKit
    does not).

    **Landed in the first PR (this branch):** the Magic crossovers wear a
    drawn 1909 Rider frame (ivory ground, numeral band, Fell-set caption,
    per-card cover focus — and the `cqw` unit is banned from the flipped
    card face, where WebKit 17.4 resolves it against the viewport); the
    fun facts can no longer repeat (the told list rides the wire like the
    transcript, is quoted back in the closing instruction, and a repeat is
    dropped and counted — the rule had been enforced by nothing); the
    fortune-teller writes on aged parchment (the Fell Types, OFL, with
    per-directory provenance; a quill-tracked word-by-word ink reveal,
    reduced-motion safe); and the deck grew an **echoes tier** (three deep
    dives, 2026-08-16): real cards whose name, art and rules carry a tarot
    card — every one of the 22 trumps answered (Gelon's Alpha Wheel of
    Fortune above all, the same painting the site's Wheel spins) and the
    minors opened — on top of the three printed tarot cards, whose cycle a
    Scryfall sweep confirmed complete. Two disciplines hold the tier:
    original imagery outranks every other classifier, and every Magic card
    carries a `note` justifying its slot in checkable pool facts (Flubs
    has 0 power, Homer is a 0/9, Apatzec's rules text says 4 four times)
    or it is cut — rendered under the turned card as the fun fact and
    handed to the reader.

    **Aaron judged the roster on 2026-08-17** and it is now thirty-eight
    echoes, 119 cards. Eight trumps changed hands: The High Priestess to
    Willow Priestess, The Lovers to True Love's Kiss, The Hanged Man to
    Suspension Field, Temperance to Chalice of Life // Chalice of Death,
    The Devil to Asmodeus the Archfiend (type line: Devil God, 6/6 for
    six, ability called Binding Contract), The Tower to Command Tower
    (Evan Shipard's struck-and-burning painting, on the land written for
    this format), The Star to Ephara, God of the Polis, and Death to
    Murderous Rider // Swift End. Nine minors opened: the Two, Five, Page
    and Knight of Wands; the Three, Ten and Queen of Cups; the Three, Nine
    and Ten of Swords. Two rules came out of that session. The rubric
    **widened past art alone** — a slot can be won on name, rules text or
    the card's place in the game, so long as the `note`'s facts are still
    checked against the pool. And the **printing is a choice**: the pool's
    default is not always the right painting, so Command Tower, Young
    Pyromancer, Thassa and Murder each name theirs. The search method that
    found them is worth keeping: Scryfall's Tagger vocabulary (`art:` /
    `otag:`) over the API, then Pillow contact sheets of every candidate
    beside the 1909 scan, judged by eye. A Magic card landing is called an omen with
    precedence in the frame message, and an original landing beside its
    Magic answer (trumps and minors both) is named as the stars aligning —
    all Python-detected facts, never prompt hopes.

    **Round two of the minors, and three rulings, the same day.** Aaron
    settled the three questions round one left open: **Temperance keeps
    Chalice of Life** over Angel of Serenity (the better fact, and the
    monochrome suits 1909 stock — the earlier "unreadable grey blob"
    report was a screenshot of the folded 96px strip, not of the card);
    **suit colour is a tiebreaker and not a law**; and **the pale horse
    stays, as the Knight of Swords** — Pale Rider of Trostad, cut from
    Death in round one, takes the charging knight instead, which is the
    better slot for it, since the Knight's horse gallops and Death's
    walks. Seventeen more minors then filled in the same method
    (tag-search, contact sheets beside the 1909 scans, ruled card by
    card): the Six, Seven, Eight, Ten and Queen of Wands; the Two, Eight,
    Nine and King of Cups; the Four, Six and Knight of Swords; and the
    Ace, Seven, Eight, Nine and Ten of Pentacles. **Fifty-five echoes,
    136 cards**, thirty-six of the fifty-six minors now answered.
    `ECHO_WEIGHT` came down 0.2 → 0.14, and that number was **measured
    rather than reasoned**: 38 echoes at 0.20 put a Magic card in 35.5%
    of spreads and 55 at 0.14 put one in 35.7%, over 40,000 seeded deals.
    The landing rate is the constant; the weight is what moves.

    The colour tiebreaker is worth recording as **a rule that did almost
    no work**: round two seated four white cards in the fire suit and no
    white card at all in the suit of air, because in every one of those
    slots the painting and the rules text pointed where the colour pie
    did not. It breaks a genuine tie and nothing more — if colour is the
    best argument a candidate has, it is not a good enough candidate.

    **Murderous Rider changed printing** in the same pass, and it is the
    clearest case the printing rule has: the pool's default (Josh Hass)
    is a colour painting of a green-faced Zombie Knight, while Ravenna
    Tran's Eldraine showcase is pen and ink and, under the ageing filter,
    reads as though it came off the same press as the 1909 plate next to
    it. Five echoes now name their printing.

    **PR 2 landed the photo-real crystal ball.** The old one was inline
    SVG and it was good SVG — fresnel ring, caustic pool, a hand-turned
    brass cradle — and it still read as a rendering, because what a real
    sphere has that geometry does not is *dirt*: veils, fractures, internal
    cloud that no gradient stack proposes. The glass is now a CC0
    photograph of a smoky-quartz sphere (Ervins Strauhmanis, via
    Openverse), fetched, licence-gated, matted and committed through the
    animist rather than hand-placed, and the room behind it is two ADR 31
    procedural loops of our own — smoke at seed 1848, the year the Fox
    sisters started the spiritualist craze, and candlelight at 1909, for
    the printing.

    Three things are worth keeping from building it. **The composite is
    four layers because the card has to be *inside* the ball** — candle and
    smoke behind, the depths, the vision, then the photograph twice on top
    (once at `soft-light` to lay the glass over the card, once crushed to
    its highlights and screened so the specular arc sits above everything).
    Flatten any of it and the card is in front of a ball instead of in one.
    **The brass stays drawn**, because a cradle is turned geometry — a
    stack of profiles each catching its own light — which is what SVG is
    good at and what a photograph would have bolted a stranger's taste
    onto. And **moving it to the centre deleted a problem rather than
    managing one**: pinned to the felt's right edge it closed on the
    centred spread as the page narrowed, sat across the third card below
    about a thousand pixels, and had to be shrunk and then hidden below
    `lg` to cope. Standing it above the cards means nothing is beside it,
    so it shows at every width and can be the size the thing deserves.

    The animist grew one op for it, `mask_circle`, and the reason it is
    geometric rather than perceptual is the point: `matte_green` keys on
    colour because foliage has no edge a number can name, while a glass
    sphere is the one subject a chroma matte *cannot* cut out, since the
    whole subject is the background seen through it.

    **PR 3 is [#151](https://github.com/aasquier/sylvan-library/pull/151), and
    it grew past its brief into the whole table.** Aaron's verdict on the
    drawn brass was blunt and correct — asked whether it looked photo-real,
    the answer was no, and the argument for it ("a cradle is turned
    geometry, which is what SVG is good at") was a rationalisation for not
    doing the harder half. A photographed sphere on a vector stand is worse
    than an all-drawn ball, because the sphere sets a standard of realism
    the stand cannot meet and the eye goes to the seam.

    So the stand is now **the Met's own crystal ball on a bronze stand in
    the shape of a fish** — a carp leaping through waves with the sphere
    held in a spray of foam, museum open access, CC0. Aaron picked it off a
    board of six and chose the **sepia** grade off a board of four. The
    decisive property is that sphere and stand come from ONE photograph:
    one light, one shadow, one set of material responses, and no
    compositing mismatch to manage. It also deletes the problem three
    passes went into — the ball does not need to be made to *sit in* the
    ring, because in the photograph it already does.

    Two animist ops were built for it, both tested and committed.
    **`matte_backdrop`** cuts a subject off a studio ground by flooding
    inward from the frame edges, because on these plates the bronze is
    darker than the ground while the crystal is brighter and no single
    threshold keeps both; what separates subject from ground is
    connectivity, not brightness. Its `soft` ramp exists because what
    survived a hard cut was not backdrop but the object's own **cast
    shadow** (143-167 against a ground of 192), and `enclosed: drop` runs
    the flood first and only then removes unreachable pockets — the studio
    grey framed inside the arch of the wave, which on a table reads as a
    hole cut in the felt. **`duotone`** maps luminance onto a three-stop
    ramp, the still sibling of `color_ramp`; a monochrome museum plate has
    no hue to preserve, and bronze is close to the ideal subject, being
    genuinely one hue with luminance variation.

    The assets are built and verified: `crystal-fish.webp` and
    `crystal-shell-sepia.webp` (the same quartz through the fish's own
    ramp — two photographs on one table have to share a plate, or the glass
    reads as pasted onto the bronze, which it visibly did). **The geometry
    is recorded in the recipe** because no image can carry it: in the
    700x950 asset the foam closes at y=290 and its centre is x=529, and the
    ball is drawn at r=265 with its centre **0.88 radii above the claw
    line** — Aaron's number, off a board of four, because at 0.62 the claws
    crossed the ball's belly and it read as impaled.

    The plate is the **Met's own**, not the aggregator's, and that is an
    animist provider rather than a URL: Openverse's record for this object
    points at rawpixel's `editor_1024` derivative, 763x1024, while the
    museum serves 2982x4000. Irrelevant while the stand was an ornament and
    decisive the moment it became the largest thing in the room. The `met`
    provider's gate reads a **boolean** rather than a licence string —
    `isPublicDomain`, mapped onto CC0 — so the check is `is True` and not
    truthiness, and a test pins that a string `"true"` is refused; one hop to
    the institution that owns the object is also the better provenance.

    **Aaron's two nits were one bug and one bug's cousin.** The "dark navy
    fresnel rim" was never a colour: the shell photograph's sphere is
    `mask_circle: {r: 0.482}` of its file, so the glass ends at 96.4% of the
    orb's box while `border-radius: 50%` clips at 100%, and that 3.6%
    annulus of bare `.crystal-depths` was hard-cut by the radius — a drawn
    stroke by any other name. Warming the palette only made it a warmer
    stroke; ending the depths where the glass ends removes it. Its cousin:
    `filter: drop-shadow` follows an element's **clip**, so the orb's perfect
    circle cast a perfect ring of shadow in the same place. A third of the
    family sat next door — the vision's mask ramp reached zero *at* the box
    edge, and under a permanent `scale` animation WebKit squares it off
    there. **A mask that ends at the edge is one you are trusting the
    compositor to round off.**

    **Then the table became a room**, which is Aaron's composition and not a
    decoration on the old one: black above, green below, one horizontal line
    between them, and that line is the back edge of the table. A rack of
    CC0 church candles burns along it on either side — one crop mirrored, so
    it reads as one rack seen from both ends rather than two racks to
    compare — and the carp stands in the gap with its base bridging them,
    which is what puts the mirror's seam behind a foot of bronze. The
    horizon needs no number: the stage is exactly as tall as the ball and
    the stand occupies 17.98%–100% of that box, so `--seance-horizon` is
    measured up from the carp's own foot and used by the dark and both
    racks. The candles are **screened, not matted** — their ground is black
    and black is already transparent under `screen`, so the flames' halos
    survive as photographed; `matte_backdrop` is for the opposite case, the
    bronze, whose studio grey is *lighter* than its subject.

    The cloth is printed, too, which is the item's "card positions laid out
    around the ball": three named places stamped into the felt on an arc
    that radiates from the ball, with the cards landing in them. The place
    carries `--arc-rot` and the card carries `--arc-rot` *plus*
    `--settle-rot`, so every card lies a degree or two off its own place —
    the cloth was printed square and the deal was a hand. And turned cards
    zoom under the pointer (1.35x on the felt, 2.1x in the 96px strip),
    gated on `(hover: hover)` because on a touch screen `:hover` latches.

    **Three rig facts came out of it, and each looked like a product bug.**
    An **element** screenshot is not what a person sees — the first scale
    board cropped to `.crystal-ball` and would have shipped a ball whose
    tail is cut off by the viewport. A **narrow viewport is not a phone**:
    headless WebKit at 390px reports `hover: hover`, so the latched-hover bug
    rendered as correct behaviour until the context got `hasTouch: true,
    isMobile: true`. And headless WebKit **ignores `backface-visibility:
    hidden`** (proved in isolation, not inferred), so a face-down card paints
    its own face, mirrored — the card back cannot be photographed through
    this rig at all. Same ledger as the black Browser pane and the missing
    media codecs: the rig is not the browser, and where it differs it
    differs silently.

    **The two researched fronts and Aaron's ten-item polish landed
    2026-08-17 evening** (PRs #153 and #154, both ruled live off boards).
    The parchment is a photograph now — the Met's Qur'an folio 448369
    through `parchment.recipe.yaml`, its own hue kept (a duotone
    monochromes skin; the new `levels` op floors the shadows at a stated
    colour instead, 6.4:1 against the ink), a seeded deckle mask for a
    silhouette, candlelight breathing on the sheet. The hand writes in
    **Parisienne** at 52ms/char with pen-lift and punctuation pauses, no
    cap, click-to-dry — and the trap worth remembering: per-character
    spans break OpenType shaping in a joined face, so the timing is per
    character while the markup stays per word. The ceremony **fits one
    screen**: the page's chrome steps aside (`onCeremony`), the ball
    sizes against the viewport's height, the controls ride the room's
    dark corners, and the spread pulls up with its outer cards on the
    wings of the trimmed, widened racks. The table **lingers** once all
    three are up until the querent knocks twice on the glass (or takes
    the visible button). The card backs wear Magic's colour wheel, drawn.
    **The dripping-wax feature is scrapped** — five prototype mechanisms
    (falling beads, screened runs, curved SVG paths, photographed
    self-clones) all failed Aaron's eye; the photographed frozen runs
    stand, and the motion budget went to the flicker instead.

    **All of the above about the room was superseded on 2026-08-18**, and
    the paragraphs stay because the reasoning is worth keeping, not because
    the code is. Aaron generated a photo-real séance table (Seedance 2.0,
    his own machine, second take against a written brief: 16:9, eight
    seconds, clear felt in the near third, dark upper corners for the
    controls) and it replaced **the whole composite** — the Met's bronze
    carp and its quartz sphere, the mirrored candle plate and its
    seventeen measured flames, the three smoke loops, the drawn gradient
    room, the light-spill. Roughly 690 lines of CSS and 400 of TSX went
    with them, plus six assets and the `crystal.recipe.yaml` that built
    them.

    The argument is that **every one of those numbers existed to hide a
    seam between two things never photographed together** — the rack's
    mirror line, the sphere against its stand, the horizon where black met
    felt — and one photograph of one table has no such seams to hide. What
    is left is four rules and four measured numbers: where the sphere sits
    in the frame, as percentages of the room's own 16:9 box, so nothing is
    cropped and nothing drifts as the window resizes.

    The turned card now surfaces **inside the footage's own glass**: a dark
    disc multiplies the interior down, the card is screened over it, and a
    seeded turbulence filter ripples its edges. That order is forced —
    the sphere is lit from within, and a picture screened onto a near-white
    ball adds nothing an eye can find. The card backs are Magic's now, a
    plate Aaron painted, and the cards themselves are small: in a
    photographed room they are objects lying on a table rather than a
    composition competing with a drawn one.

    Three things came out of it worth keeping. **A duration measured for
    one surface is a question to ask of every sibling** — the phone was
    broken by this change and nothing said so, because the controls' "dark
    upper corners with nothing in them" are a fact about a wide window,
    not about the design; at 375px the room is 184px tall and three rows
    of wrapped buttons covered the candles. **`mtglab animist verify` is
    enforced by nothing**: it sweeps all eleven recipes and CI never runs
    it, while `tests/test_animist_recipes_repo.py` pins only two of them —
    so the nine broken outputs this change created passed the full suite
    and were caught by hand. And **the three new assets sat outside the
    gate**, because `sources.py` has no kind for a file the maintainer
    authored; `web/src/assets/seance/PROVENANCE.md` says so in as many
    words and names the `authored` source kind that is owed.

    **Still to come, in order:** the
    reader-as-artist proposal (the spread, flavor text and art crops as
    real evidence in the commander pick, pool-resolved as ever); an ADR
    and the 99 ceremony (category by category, a card dealt per category,
    ending in a draft deck through the import path — rule 4 intact); and
    the Wheel of Fortune's wildcard slots in that ceremony plus its
    visual love.

14. **The six-feature batch** — Aaron's list of 2026-08-18, planned as seven
    PRs (the full carve-up lives in the session plan and the PRs
    themselves). Landed so far, each walked by eye before commit: **the
    official mana symbols** served first-party from a runtime cache with
    the drawn five as offline fallback (ADR 33 — supersedes PR #61's
    drawn-only stance; tap, untap, colourless, hybrids, Phyrexian and
    numerals all draw now, every pip named for a reader); **the button
    overhaul** (commandment 17, Aaron's own words: thou shalt not make a
    simple button — one `.btn` family plus chip/tab/menu/felt cuts, every
    control answering hover, focus and press, the felt warming to brass
    rather than vine) with the replay and hand-fan glyphs on the rerun and
    reshuffle verbs; and **the 99 rollup** (each category folds behind a
    header wearing a drawn glyph, folds remembered per deck, plus the
    CardHover fix — the ~200 capture-phase scroll listeners the cards tab
    used to register are now at most one).

    **Still to come, in order:** the **About Claude** page (a fresh
    session by design — the bio deserves a clean context; nav tab, the
    Keeper's structural pattern, pool-checked favourites, credited art);
    the **Admin tab** (rename from Accounts + an on-box dashboard:
    system/storage/claude/activity stats endpoints under `/api/admin/stats`,
    the ledger's tokens honestly labelled a floor — the Anthropic dollar
    widget was dropped with Aaron 2026-08-18, his Console account is
    individual and the Usage & Cost Admin API does not exist for those);
    the **visitor ledger** (schema v9, `request_log` of route templates
    only — no IPs, no UAs — on its own watched branch); and **Fly metrics**
    (`FLY_METRICS_TOKEN` → managed Prometheus for machine + edge stats,
    Grafana link-out for alerting).

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
the pool, files lands and nothing else, and writes a `stage: draft` deck with
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
blocked on their banned card. `swaps.md` is the exception — it is a diff, so
it only appears once a deck changes against a baseline. Since ADR 30 the
baseline is the last build's own snapshot (`artifacts/deck.last-built.yaml`)
rather than a git revision, because decks are live app data and not in git.

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
   exactly ADR 14's half — "the questions the pool cannot answer" — and it is
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
from pool tools, never from recall. And it still may not pre-fill a `why`.

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
| **Claude** | the meta, why a slot exists, what a card is *for*, whether a spoiler earns a place | any card fact — those come from pool tools, never recall |

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

Both confirmed against Scryfall `legalities.commander`, on a card pool current to
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

Beyond the two bans, the gate and a card pool cross-check caught five errors in
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
a Time Lord Doctor — all enumerated from the pool, with deck size correctly
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
  them would hand back a 96-card deck silently. It refuses without a card pool,
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
first to reach past the pool. It is also where research through server-side
web tooling turned from a plan into code: `web_search_20260209`, with every
cited page checked against what the search actually returned.

Then the **theme interview** (2026-08-13, ADR 20 — two modes, conversation and
proposal, plus the tarot door of ADR 21), the **stance dial UI** (2026-08-14,
#88), the **slot argument** (2026-08-14, ADR 25, #89), and **research**
(2026-08-14, ADR 26) — six modes across five features.

Then the **activity log** (2026-08-16, ADR 28) — not a mode, but the
prerequisite ADR 15 lists for the top of the write axis, and the last one that
was cheap. Every deck edit is now recorded from `service._commit`, with who
made it, and **never with what a rationale says**; `mtglab decks log <slug>`
and the deck page's History tab read it. It answers "what did it change while
I was not looking" for the two cases git cannot: ADR 22's SQL tier, which has
no git history at all, and the deployed instance, where `/data/decks` is the
live source of truth and nothing commits it.

What is *not* built: the one mode ADR 15 names that remains — **deck
conversation** — and the Forge half. Note that ADR 26 made deck conversation
*harder* rather than nearer: research is deck-blind by construction precisely
so that "a Claude surface that can see your list" stays a decision somebody
has to argue for.

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
pool. It also costs **less** — 19,130 input tokens against 25,142 — because a
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
| The meta, whether a spoiled card earns a slot, what a ruling means in practice, whether a plan holds together | Claude | No card-pool query answers these; they need an opinion or the open internet |
| Playing actual games | Forge | A real rules engine with a real AI, which took a decade to build |

### The three boundaries

1. **Rule 1 still binds Claude.** Card facts come from the pool — not from
   the model's recall, and not from a web page. Research is for what the pool
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
| **Argue a slot** | the case against a specific card, from pool facts and category counts — ADR 25, built 2026-08-14 |
| Deck conversation | anything about a deck, with the gate's output and the pool in reach — the one still unbuilt |
| **Research** | the meta, rulings in practice, cards spoiled ahead of the next bulk refresh — ADR 26, built 2026-08-14, and it cannot see a deck |
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
rationale editor left empty for exactly this. **Argue a slot followed on
2026-08-14 (ADR 25) and research the same day (ADR 26); deck conversation is
not built.**

What is worth carrying forward is **where the boundary ended up living**, since
none of it is the system prompt:

- **The response schema has no field for a rationale**, and forbids extra
  properties. A model that wanted to hand over a draft has nowhere to put one.
- **Every item is checked to be a question** — it must end in a question mark,
  and what does not is dropped and *counted*. A mode that starts editorialising
  shows up as a number rather than as help.
- **The pool facts arrive before the model does.** `interview.brief()`
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
pool does not store power or toughness at all. It does not: no `power` or
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
someone on a hosted instance — **built 2026-08-16 as ADR 28**, with an `actor`
column that says NULL today and is there so nothing has to be migrated the day
something autonomous writes — and a default that comes from the deck —
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
to know what a card does calls the pool and the tool result is the fact.

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
pool has it, which is where a mode's questions go.

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
conversation over tool results the pool already computed, which is not
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
pool facts, a question back — with the deck and system prompt sitting in a
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

**The constraint is data and CPU, not code.** The pool is ~63 MB of DuckDB
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
   machine, so a scheduled second Machine cannot mount the pool. Run it by
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
on every PR, and the pool seeding run is documented in HOSTING §4 step 6
(which since ADR 30 also says where a fresh instance's decks come from — a
backup, the laptop, or an import; the image carries none). A refresh cron deliberately does not exist — the refresh is monthly and
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
