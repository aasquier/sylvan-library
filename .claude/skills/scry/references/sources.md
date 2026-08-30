# Sources — the council's supply lines

Every external door the advisors use, with the exact shapes verified on
2026-08-29. Fetch with `curl`, parse with `python3` **on stdin** — the
default `python3` here is a certless 3.7 that cannot open HTTPS itself:

```bash
curl -sS --max-time 20 '<url>' | python3 -c "import json,sys; d=json.load(sys.stdin); ..."
```

Be a good guest everywhere: one request at a time, a beat between them,
and cache every fetch into the session's `edhrec/` / `spellbook.json`
files so a page is pulled once per scry.

## EDHRec (JSON mirror)

**Slug recipe:** lowercase the commander's name, drop punctuation
(apostrophes, commas, periods), spaces to single hyphens. *Arahbo, Roar of
the World* → `arahbo-roar-of-the-world`. Partner pairs join both slugs
with a hyphen. Verify with `curl -sS -o /dev/null -w '%{http_code}'` — a
404 means the slug guess was wrong, not that the page is missing.

| Page | URL |
| --- | --- |
| Commander | `https://json.edhrec.com/pages/commanders/<slug>.json` |
| Theme cut | `https://json.edhrec.com/pages/commanders/<slug>/<theme>.json` (e.g. `cats`) |
| Average deck | `https://json.edhrec.com/pages/average-decks/<slug>.json` |
| Combos | `https://json.edhrec.com/pages/combos/<slug>.json` |
| One card | `https://json.edhrec.com/pages/cards/<card-slug>.json` |

**Commander-page structure:** `container.json_dict.cardlists[]`, each with
`header` (*New Cards, High Synergy Cards, Top Cards, Game Changers,
Creatures, … Lands*) and `cardviews[]`. A cardview carries `name`,
`synergy` (a fraction: `0.84` renders as +84%), `num_decks`,
`potential_decks`. Combos-page cardlist headers are the pairings
themselves with deck counts (`"Heliod, Sun-Crowned + Walking Ballista
(49977 decks)"`).

```bash
curl -sS 'https://json.edhrec.com/pages/commanders/<slug>.json' | python3 -c "
import json,sys
d=json.load(sys.stdin)['container']['json_dict']['cardlists']
for l in d:
    for cv in l.get('cardviews',[]):
        print('%s\t%.3f\t%d/%d\t%s' % (l['header'], cv.get('synergy') or 0,
              cv.get('num_decks') or 0, cv.get('potential_decks') or 0, cv['name']))"
```

## Commander Spellbook

One POST answers the whole combo question:

```bash
python3 - <<'EOF' > spellbook-body.json
import json
main = [ ... the 99 as names ... ]
print(json.dumps({
  "commanders": [{"card": "<commander>", "quantity": 1}],
  "main": [{"card": n, "quantity": 1} for n in main]}))
EOF
curl -sS -X POST 'https://backend.commanderspellbook.com/find-my-combos' \
  -H 'Content-Type: application/json' -d @spellbook-body.json > spellbook.json
```

Entries **must** be `{"card": name, "quantity": n}` objects — bare strings
are refused. The answer's `results` carries: `included` (combos already in
the deck), `almostIncluded` (one card away — the Artificer's gold),
`almostIncludedByAddingColors`, `includedByChangingCommanders`,
`almostIncludedByChangingCommanders` (the Kingmaker's), and `identity`.
Each combo: `uses[].card.name` (the pieces), `produces[].feature.name`
(what it does), `description`, `manaNeeded`, `notablePrerequisites`,
`bracketTag`, `popularity`, `legalities`, `prices`.

## Scryfall search API

The Archaeologist's trowel and the Quartermaster's catalogue. Courtesy:
50–100ms between requests, a descriptive `User-Agent` (the generic one can
be 403'd), paginate via the answer's `next_page`.

```bash
curl -sS -A 'sylvan-library-scry/1.0' \
  'https://api.scryfall.com/cards/search?q=<urlencoded query>&order=released&dir=asc'
```

Query atoms that matter here — always include the first two:

- `legal:commander` — the banned list, enforced at the source.
- `id<=WUG` — inside the commander's color identity (letters as needed).
- `otag:ramp`, `otag:card-advantage`, `otag:removal`, `otag:board-wipe` —
  community function tags; how the Quartermaster finds fills for a hole.
- `year<=2003`, `frame:1993`, `set:leg`, `set:arn`, `set:ptk` — strata.
- `is:reserved` / `-is:reserved` — per the interview's ruling.
- `usd<=5` — per the interview's budget.
- `oracle:/regex/` for effect archaeology.

One card, with prices: `https://api.scryfall.com/cards/named?exact=<name>`
(`prices.usd` in the answer). Prices for the swap list come from here and
nowhere else.

**The recent-sets list** (for `recent-sets.md` and the recency mandate) —
discovered at runtime, never recalled; compute the cutoff from today:

```bash
curl -sS -A 'sylvan-library-scry/1.0' 'https://api.scryfall.com/sets' | python3 -c "
import json,sys
d=json.load(sys.stdin)['data']
keep=[s for s in d if s.get('set_type') in ('core','expansion','commander',
      'draft_innovation','masters') and s.get('released_at','') >= '<two years ago>'
      and not s.get('digital') and s.get('released_at','') <= '<today>']
keep.sort(key=lambda s: s['released_at'], reverse=True)
for s in keep: print(s['released_at'], s['code'].upper(), '-', s['name'])"
```

Bound searches to the window with `date>=<two years ago>` (a Scryfall
query atom), or to one set with `set:<code>`. Unreleased sets come back
from `/sets` too — the `<= today` filter keeps spoiler-season ghosts out
of a census, and the pool will not have them anyway.

## The local instruments

- `./mtglab cards show '<a>' '<b>' …` — cost, types, color identity,
  oracle text from the pool; several names per call, ~15–20 per batch. A
  refusal names what the pool lacks. **This is the ground truth every
  advisor confirms against — external data proposes, the pool disposes.**
- The seeded simulators and the gate — the SKILL.md End-step recipe.

## Moxfield

Bot-walled (Cloudflare 403 on the API). The pasted text export is the one
true input; there is nothing to fetch, and nothing should try.
