# 7. Card facts come from the corpus, never from memory

**Status:** Accepted · **Recorded:** 2026-08-10

## Context

This is the decision the rest of the project is arranged around, and it was made
in response to real errors rather than in the abstract.

- **Ajani, Nacatl Pariah** was proposed for a G/W deck. Its back face is R/W, so
  its colour identity is illegal. Reasoning from the front face gets this wrong
  every time.
- **Arahbo's** {1}{G}{W} doubling was described as eminence. It is not — eminence
  is only the +3/+3, and the doubling is a triggered ability that requires him on
  the battlefield, once per attacking Cat. Six mana doubles two attackers, not
  one attacker three times.
- **`Captain America's Aid`** was cited as a card in the Arahbo list. It is not
  one; the source's own parenthetical named the real card, Sigarda's Aid.

Every one of those is a checkable fact that was missed by recalling instead of
checking. The user reads card text closely and catches them, which is the only
reason they were caught at all.

## Decision

**Never evaluate a card from memory. Look it up.**

```python
from mtglab.cards import db
con = db.connect('data/mtg.duckdb')
for n, r in db.get_cards(con, ['Arahbo, Roar of the World']).items():
    print(r.name, r.mana_cost, r.type_line, sorted(r.color_identity))
    print(r.oracle_text)
```

**Colour identity comes from Scryfall's `color_identity` field**, never derived
from the mana cost. It already accounts for back faces, reminder text and land
types. `color_identity_of()` prefers an explicit value and only falls back to
deriving one for cards not yet in the corpus — freshly spoiled previews.

The generalisation, which is what makes this an architectural decision rather
than a style note: **a claim about a card is a query, and anything that cannot
be expressed as a query is not yet a claim.** That is why the corpus is local
([ADR 2](0002-duckdb-for-the-card-corpus.md)), why decks are data
([ADR 1](0001-deck-yaml-in-git-is-the-source-of-truth.md)), and why generated
documents are generated rather than written
([ADR 8](0008-the-gate-blocks.md)).

## Consequences

- Colour-identity, legality, companion and partner checks are all mechanical,
  and all of them found errors in lists that looked fine.
- The corpus is a hard dependency for deck work, and a soft one for everything
  else: the simulation core and the whole test suite run without it.
- **It applies to code, not just prose.** The Phyrexian mana-value bug fixed on
  2026-08-10 was found because a property test compared the parser's mana value
  against a symbol table checked against the rules, and the fix was written
  against symbol shapes enumerated from the corpus (`{G/U/P}` and `{C/P}` both
  exist, and both count 1) rather than against what Phyrexian mana was assumed
  to look like.
- The rule extends to derived numbers. The Arahbo source claimed a 67% turn-five
  commander rate; the simulator says 57.2%. Both are checkable, so neither gets
  asserted.
- Cost: a card question needs a ~500 MB download first. `--oracle-only` covers
  everything except pricing and is much smaller, which is the answer for a
  constrained environment.
