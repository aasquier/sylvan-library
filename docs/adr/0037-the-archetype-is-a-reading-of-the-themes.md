# 37. The archetype is a reading of the themes, not a second declaration

**Status:** Accepted · **Decided:** 2026-08-22 with Aaron · Supersedes the
labelling half of ADR 36 (the ledger's snapshot rule and everything else in
that ADR stand); informed by a survey of EDHREC's tag taxonomy
(https://edhrec.com/tags/themes, read 2026-08-22) and by the six decks' own
primers.

## Context

ADR 36 gave a deck two declared labelling axes: an open `themes` list
(identity) and a closed single-slot `archetype` (aggro | midrange | control |
combo — the class the rating boards group by). Preparing the rating boards
(the next-phase sequence's step 4) put the labels the six decks actually
carry next to the decks' own primers, and the single slot failed on both of
its jobs at once:

- **It contradicted the decks' own pages.** Tivit's primer opens "Esper
  midrange-control at cEDH speed" and wins through "a compact two-card
  combo"; the slot held `combo` and threw the control half away. Whichever
  one it kept, the deck page called it wrong. Trostani — a token-
  multiplication deck with a Craterhoof finish — wore `midrange`, the
  default bucket that means "not clearly the other three".
- **The coarseness never bought what it was chosen for.** The class was kept
  small so each board would hold enough games to mean anything
  (`decks/model.py`'s own argument). Measured against the actual library:
  aggro ×1, combo ×1, midrange ×4 — one board of four decks and two boards
  of one, rating nothing. The fidelity was paid and the density never
  arrived. This is the same lesson the mana-curve work bought a day earlier:
  a branch nobody checked never fires.

A survey of EDHREC's taxonomy — one of the two sources named when the
archetype reference was first scoped, and the page Aaron pointed back to —
showed the model the community actually uses:

- **There is no closed archetype slot anywhere in it.** Aggro (98k decks),
  Combo (116k), Control (68k), Midrange (53k), cEDH (43k), Tempo (12k) are
  *tags in the themes list*, beside Landfall and Food, and decks carry
  several at once. Tivit's own page wears four at near-equal weight:
  Politics, Artifacts, Combo, Control — combo and control side by side,
  which is exactly what his primer says and exactly what our slot forbade.
- **Typal is its own kind and the tribe is the tag.** There is no generic
  "tribal" tag at all; Cats *is* the theme. Our vocabulary carried both
  `tribal` and `cats`, one tag too many.
- **Their own FAQ concedes derived tagging is unscientific** — per-theme
  count-fiddling with no unified formula. Evidence *for* ADR 36's
  declared-never-derived rule, not against it.

Our own tuple already agreed without saying so: `stax`, `voltron`, `mill`,
`reanimator` and `spellslinger` — strategy words all — were themes, while
`control` was exiled to a different axis. The quartet was singled out for no
reason the vocabulary itself could state.

## Options considered

1. **Widen `ARCHETYPES`** — more classes, still one per deck. Rejected: the
   Tivit contradiction is not about granularity, it is about arity. Any
   single slot forces a multi-strategy deck to lie, and finer classes make
   the boards sparser, the opposite of what grouping needs.
2. **Two declared fields** — keep a declared coarse class under an honest
   name plus strategy words in themes. Rejected: two declarations of one
   fact drift apart, and the six decks just demonstrated that nobody
   re-reads the coarse one against the fine one.
3. **Derive the class from the declared strategy themes by a fixed rule.**
   Accepted, below.

## Decision

**The four class words join the themes vocabulary, and the archetype becomes
a derived reading of them.**

- `aggro`, `midrange`, `control`, `combo` join `THEMES`, alongside `cedh`
  and `tempo` from the same survey. A deck declares as many strategy words
  as are true of it, exactly as it declares `food` or `politics`. Tivit can
  finally say `control, combo` — what his primer has said all along.
- `Deck.archetype` is now a **property**: among the four class words present
  in the deck's declared themes, the **worst-Forge-piloted wins** — latest
  in the gradient aggro > midrange > control > combo that `ARCHETYPES` has
  always recorded. Control + combo reads combo. A deck declaring no class
  word has no board to sit on, unchanged from ADR 36's "absent means
  unlabelled".
- **Worst-wins is the whole rule and it is deliberate.** The class exists to
  carry a caveat about Forge's pilot; deriving the *most* pessimistic
  applicable caveat means the boards can only err toward more warning, never
  toward a rosier grouping. Only the four class words feed the rule — `cedh`
  and `tempo` are identity, not board classes, so Tivit's board comes from
  `combo`, not from `cedh`.
- **This is not the laundering ADR 36 banned.** That argument was against
  deriving a class from the *decklist*, where whatever signal picked the
  class would correlate with how well Forge pilots the deck. The input here
  is the human's own declaration — deriving a coarse view from a declared
  fine one is projection, not inference, and the rule is one line anybody
  can read.
- `tribal` leaves the vocabulary. The tribe is the tag — `cats` and
  `dinosaurs` already say it better, and the survey found no community
  precedent for the generic word. The two decks declaring it get a
  `unknown-theme` warning until relabelled, never an error.

**Migration is a fallback, not a rewrite.** A legacy `archetype:` key in
`deck.yaml` is still read: when the themes name no class word, the property
answers with the legacy value (if it is one of the four), so every existing
deck — the local six and whatever a deployed volume holds — keeps its board
with zero edits. `validate` counts a `legacy-archetype` warning saying where
the label should now live. `dump` writes the key back only while it is
load-bearing; once a class word lands in themes it is shadowed and the next
write drops it, the same self-cleaning round-trip `commander_art` uses.
`decks set --archetype` is removed; `--themes` is the one control.

**The ledger is untouched.** `forge_seats.archetype` keeps its name and its
snapshot rule — it now records the derived reading at match time, which is
the same promise ADR 36 made: the boards group by the class a deck wore when
it played, and relabelling never rewrites history. No schema migration; the
column comment says what the value now is.

## The fetch question, answered while the survey was open

Aaron asked whether EDHREC's tag data could be fetched at deck build or
import, or by a future refresh feature. **No — their own Terms of Use settle
it** (read 2026-08-22): the licence is personal and noncommercial, content
may not be republished, and the Acceptable Use Policy expressly forbids
using "software or automated agents or scripts … to generate automated
searches, requests, or queries to the Site". Commandment 9 makes that
binding regardless of our own crawler ban, which also stands.

What stays lawful, and composes — the same three shapes the first archetype
scoping recorded, now with the terms actually read:

1. **Link out.** A deck page may link its commander's EDHREC page; a link is
   not a scrape.
2. **The vocabulary grows by reading.** A human (or Claude in session, in a
   browser, as reconnaissance) surveys the page and edits `THEMES` as a
   reviewed diff. This ADR is that shape's first product.
3. **Label suggestions at import or build** can be built later on the
   dossier's existing pattern (ADR 19): one Anthropic-hosted web-search
   question per commander, cited sources, output constrained to our own
   `THEMES`, and the human confirms before anything lands in `deck.yaml`.
   Suggestion-then-confirmation keeps ADR 36's declared-never-derived rule
   intact — the human declares by accepting.

## Consequences

- The six decks should be relabelled with the strategy words their primers
  already use — Aaron's call, deck by deck, as label declarations always
  are. Until then the legacy fallback keeps every board where it was.
- Per-archetype reporting stays law. Nothing in this ADR changes how results
  are grouped or captioned — only how honestly a deck comes to wear its
  class.
- The boards' density problem is now visible instead of hidden: with six
  decks, however classed, some boards hold one deck. That is a fact about
  the library, not the labels, and the rating design (next-phase step 4)
  has to face it rather than assume the classes fixed it.
- A future deck whose themes name no class word quietly has no board. The
  `validate` gate does not demand one — a board is an opt-in to Forge
  comparison, not a legality fact — but the rating surfaces must say
  "unclassed" rather than silently omitting the deck.
