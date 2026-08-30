---
name: scry
description: "The intake council. Turns one of Aaron's existing Commander decklists (a Moxfield or any text export, pasted) into a sylvan-library-ready import: six adversarial advisors — commander fit, combos, the macro census, deep Magic history, cold numbers, and mana — research the list against EDHRec, Commander Spellbook, Scryfall, the local card pool and the seeded simulators; Aaron rules the contested slots; and the session hands back import.txt with a drafted why on every card, a swap list, deck name candidates, a dossier artifact and a pilot's quick-start. Triggers on 'scry', 'scry this deck', 'intake', 'intake a deck', 'convene the council', 'import my Moxfield deck', 'bring this deck into the library', or Aaron pasting a decklist he wants worked into the library. NOT for tuning a deck already in the library — that is mtg-lab's work — and not for the site's own import page, which this skill's output feeds."
---

# /scry — the intake council

Look at the top of your library. Decide what stays, and what goes to the
bottom. That is the whole skill: one of Aaron's existing decks arrives as
pasted text, six advisors who do not agree with each other go digging, Aaron
rules where they clash, and what leaves the table is a meaner, truer 99 in
the exact grammar the import page blesses — every card carrying a `why`,
every claim carrying a seed or a source.

## Before anything else

Read `CLAUDE.md` in full if you have not this session. **The Commandments
outrank everything in this skill.** This is commandment 1 in miniature — a
collaboration, with Aaron in the loop at every hinge — and commandment 3 at
full volume: the council speaks Magic, not project management.

Two memories to honor without re-deriving: the default `python3` here is a
certless 3.7 that cannot make HTTPS calls (fetch with `curl`, parse on
stdin), and `gh`/`npm`/`node`/`fly` resolve plainly.

## Non-negotiables

1. **No card is ever judged from memory.** `./mtglab cards show '<name>'`
   (several names per call) before any advisor, any why, any swap mentions
   a card. Two real errors made this rule; Aaron reads card text closely.
2. **Color identity is the pool's word** — `cards show` prints it — and
   legality is the gate's word (`decks validate` reports `banned` and
   `color-identity` as errors). No advisor overrules either.
3. **A `why` describes what the card does in this deck. It never argues the
   card deserves the slot.** That distinction is ADR 41's, it protects
   ADR 25, and it is the voice every drafted rationale uses.
4. **Every quoted number carries its seed**, and a tier-1 quote says what
   tier 1 is: goldfishing, no opponents, no interaction, no text beyond mana
   production. `sim shelf` is arithmetic about a simpler game — quote it as a
   question about the mana base, never as a chance of having the card.
5. **Nothing here writes to the real library, ever.** The proving grounds
   are a scratch library in the session scratchpad, burned at cleanup. The
   final import is Aaron's paste, on the site, by his hand (ADR 49 rulings,
   2026-08-29).
6. **Prices come from Scryfall; carts are built, never checked out.**
   Reserved List is allowed or not per deck — ask, never assume.

## The turn, in order

Run the whole turn in one session per deck (commandment 12). Announce each
phase as it opens, and say what is running in the background while it runs.

### Upkeep — the interview

Ask Aaron, in chat (a decklist does not fit a question widget — ask him to
paste it as a message or give a file path):

- **The list.** Moxfield text export or anything like it. The parser the
  site uses reads `1 Card Name (SET) 123` lines, `Commander`/`Sideboard`
  section headers, and `*CMDR*` markers — take what he pastes as-is and
  resolve it, never retype it.
- **Scouting reports.** Cards already on his mind for this deck. They enter
  the docket as first-class candidates every advisor must weigh, not as
  afterthoughts.
- **The bracket** he wants the deck to land at (1–5, the site's own field).
  This calibrates every advisor: a combo that ends the game on turn 4 is a
  finding at bracket 4 and a fun-police citation at bracket 2.
- **Sacred cows.** Cards the council may not cut. Recorded, honored,
  and passed to every advisor.
- **When the deck was last touched.** Most of these lists predate the last
  two years of sets, and those years have been dense. The answer sets how
  hard the recency sweep leans; when in doubt, assume it predates them.
- **Budget and Reserved List stance**, per deck.
- **The vibe** — what piloting this deck should feel like at the table, in
  his words. The flavor pass and the name ceremony both feed on this.

Confirm the read-back before spending research: commander as resolved,
count, anything that did not resolve against the pool, and the scouting
reports listed by their real names.

### Draw — ground truth

Build the dossier in `<scratchpad>/scry/<slug>/` before any advisor runs:

1. `moxfield.txt` — the paste, untouched.
2. `pool-facts.md` — `./mtglab cards show` over every name in the list plus
   every scouting report, batched 15–20 names per call. A name the pool
   refuses is surfaced to Aaron now, not discovered by an advisor later.
3. `edhrec/` and `spellbook.json` — fetched per `references/sources.md`:
   the commander's EDHRec page (synergy scores), theme and average-deck
   pages, and Commander Spellbook's `find-my-combos` over the exact list.
4. `recent-sets.md` — the last two years of sets from Scryfall's `/sets`
   (recipe in `sources.md`), newest first, **discovered at runtime and
   never recalled** — sets ship faster than any memory of them. Confirm
   the pool has the newest one (`cards show` a card from it); a pool that
   predates it means `data refresh` before the council sits, with Aaron's
   nod (it is a large download).
5. If the binary is missing, build it first (`cd go && go build -o ../mtglab
   ./cmd/mtglab`) with the three exports from CLAUDE.md.

### Main phase — the council

Fan out five advisors as parallel background subagents (`general-purpose`).
Each gets: its full brief from `references/<name>.md`, the dossier paths,
the interview terms (bracket, sacred cows, budget, vibe, scouting reports),
**the recency mandate**, and the output contract below. **They do not see
each other's work** — the adversarial value is five independent digs,
reconciled later, in the open.

**The recency mandate, verbatim in every fan-out prompt:** *this deck was
built before the last two years of sets, and those years were dense. Sweep
them especially hard — `recent-sets.md` names them, newest first, through
the current set — and weigh every new printing against the sitting slots.
The deck should leave the council knowing what the format learned since it
was sleeved.* (Aaron's standing order, 2026-08-29.)

- `references/kingmaker.md` — is the crown on the right head?
- `references/artificer.md` — engines, loops, infinities (2–3 pieces).
- `references/quartermaster.md` — the macro census, from oracle text.
- `references/archaeologist.md` — the deep cuts, Alpha to yesterday.
- `references/oddsmaker.md` — synergy numbers, curves, best-in-slot.

**The output contract, for every advisor.** A markdown report with exactly
these sections, so synthesis is reconciliation rather than re-reading:
`## Verdict` (a paragraph, in voice) · `## Keeps` (only slots someone else
would question) · `## Cuts` (each: card, one-line reason, confidence 1–5)
· `## Adds` (each: card, one-line reason, source, confidence 1–5) ·
`## Flags` (anything for another advisor or for Aaron). Advisors cite what
they measured — a synergy score, a combo id, a printed year, an oracle line
— never what they recall. Save each under `council/<name>.md`.

### Combat — the docket

Reconcile in the main session, in the open:

- Where advisors **agree**, the call is made; note who carried it.
- Where they **clash** — the Oddsmaker's best-in-slot against the
  Archaeologist's deep cut, a sacred cow the Kingmaker eyes sideways — the
  contested slots go to Aaron in batches (AskUserQuestion, up to four slots
  per round, each option naming its advisor and its argument in one line).
  Blowouts are settled by the room; close calls are his by right.
- Every scouting report gets an explicit verdict here, whatever the
  advisors thought of it.

The output is `draft-99.md`: the draft list, each card tagged with which
advisor(s) carried it and what it displaced.

### Second main — Rainbow's lap

The one advisor who judges the *draft*, not the raw list. Spawn Rainbow
(`references/rainbow.md`) with `draft-99.md` and the dossier. Rainbow audits
sources against pip demands, land count against the deployment plateau, and
holds a veto: mana findings force revisions to the draft before anything is
called final. One revision round is normal; two means the draft was greedy —
say so.

### End step — the proving grounds

Materialize the final draft as a scratch library and measure it with the
real instruments, headless:

```bash
mkdir -p "$SCRY/decks/<slug>"
# write deck.yaml: name, commander, stage: draft, status: theoretical,
# cards as {name, why, qty} entries — the same list import.txt carries
export MTGLAB_DECKS_DIR="$SCRY/decks"
./mtglab decks validate <slug>            # the gate: banned, identity, size
./mtglab sim mana <slug> --seed 7 --games 20000
./mtglab sim lands <slug> 30 40 --seed 7  # read where deployment plateaus
./mtglab sim shelf <slug>                 # colored sources, the closed form
./mtglab sim mulligan <slug> --seed 7
```

`--seed` must be passed explicitly — `sim mana` unseeded is unreproducible.
Gate errors are findings, not failures: fix banned/identity now, report the
rest. Land count reads **spells deployed through T8**, never commander
speed. Record every number, with its seed, in the dossier. Forge (tier 3)
is deliberately not part of a scry — play-tuning comes later, on the site.
When the lap is read, `rm -rf "$SCRY/decks"` and `unset MTGLAB_DECKS_DIR`:
no deck lingers on this laptop.

### Cleanup — the name ceremony and the handoff

**Names first.** Five candidates, each with a one-line story rooted in the
deck's cards, its commander's lore, or Aaron's vibe words — no puns that
would embarrass the deck at a real table unless the vibe asked for them.
AskUserQuestion; his "Other" is always a fine answer.

**Then the deliverables**, sent as files as they finish (SendUserFile) with
the dossier published as an artifact:

1. **`import.txt`** — see "The blob" below. The one that matters.
2. **`swaps.md`** — every out/in against the current list, each cut argued
   against the specific slot it vacates (mtg-lab rule 5), with rough
   Scryfall prices on the ins. Cuts are low-stakes by decree: Aaron owns
   the cards and can always put them back.
3. **The dossier** (artifact) — the deck's name and story, a high-level
   description, the pilot's quick-start (first three turns, the main line,
   the panic button), the sim numbers with seeds, and the council's tally.
4. **`minutes.md`** — each advisor's verdict, in voice. The keepsake.

Close the session by the standing rules: memory updates, the next-session
prompt, the roadmap artifact.

## The blob — import.txt's exact shape

The site's parser (`go/internal/decklist`) is the audience. One card per
line, no category sections for the 99 — **the import logic handles
categories; never pre-sort them** (Aaron's ruling, 2026-08-29).

```
1 Arahbo, Roar of the World (C17) 27 *CMDR* "the whole deck, and the reason the 99 purrs"
1 Access Tunnel (MKC) 247 "Taps for colorless but also lets small creatures through"
1 Sol Ring "fast mana, and it never gets cut"

Maybeboard:
1 Beast Within "flexible answer waiting its turn on the swap board"
```

- Quantity, name, optional `(SET) collector`, the commander marked `*CMDR*`,
  and the `why` in **straight double quotes, last on the line**.
- **A why may not contain any quotation mark, straight or curly** — the
  parser's quoted run ends at the first one. Apostrophes are fine.
- Lines stay under 512 runes; a card whose *name* ends in a quoted epithet
  (Kongming) must always be followed by a rationale, or the parser has two
  readings.
- **Word budget:** a role-player's why runs ~8–20 words; a card pivotal to
  the strategy earns 25–45. Every why in the descriptive voice
  (non-negotiable 3), and none of them identical boilerplate — a reader
  should feel a person thought about each slot.
- The `Maybeboard:` section carries the near-misses onto the deck's swap
  board, ready for the first tune-up.
- Tell Aaron, when handing it over: paste on the site's import page and
  **tick "Claude drafted these reasons"** so every why lands signed
  `why_by: claude` (ADR 49). The mark is the truth-teller; skipping it
  makes the file lie in six months.

## What a scry never does

- Never quotes a card's text, cost, or identity it did not just look up.
- Never writes a why that argues for the slot, and never leaves one blank.
- Never touches the repo's `decks/` directory, the live library, or any
  deck already imported — the tune-up of an imported deck is future work
  through the bulk door, and today it belongs to mtg-lab.
- Never quotes an unseeded or cached number as fresh.
- Never checks out a cart, scrapes a marketplace, or prices anywhere but
  Scryfall.
- Never ships a combo above the bracket Aaron dialed without flagging it in
  combat, and never "fixes" the library's deliberately-invalid deck if it
  somehow appears in a paste.
- Never ends without the four deliverables or an explicit note of which are
  missing and why.
