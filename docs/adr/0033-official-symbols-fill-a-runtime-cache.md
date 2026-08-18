# 33. The official mana symbols fill a runtime cache, and the drawn five become the fallback

**Status:** Accepted · **Decided:** 2026-08-18 with Aaron · Supersedes the
"drawn, not fetched" decision of PR #61 (which predated the ADR series'
habit of writing these down); applies ADR 32's runtime tier to iconography.

## Context

PR #61 gave the app hand-drawn SVGs of the five colour symbols, and the
argument was written into `lib/managlyphs.ts` in three parts: a pip is
inline in prose and must not wait on a third party; `/api/colors` renders
pips with no pool and no network, a property CDN pips would spend; and
checking Scryfall's own path data into a public repository would be
redistribution rather than hotlinking (rule 5, ADR 6). All three points
were sound, and the decision they produced still had two costs. The
drawings are approximations — panache within the reach of a pen, not the
symbols players have read since Alpha — and the vocabulary stopped at five:
tap, untap, colourless, hybrids and Phyrexian all rendered as letters in a
grey disc, though the prose parser already recognised every one of them.

Aaron asked for the real thing: *"much more realistic icons for the five
colors — is that not something we can get somewhere official from magic?"*

Since PR #61, ADR 32 built the third place that dissolves the dilemma: a
runtime tier under the gitignored `data/` tree — the volume when deployed —
where Wizards-derived imagery may live because a private server-side cache
is not the bulk redistribution a public repository would be. Card-art
motion derivatives already work exactly this way.

## Options considered

**Keep drawing, at higher fidelity.** Extend the glyph set by hand — tap
arrow, untap, colourless diamond, split hybrid discs, phi. Preserves every
PR #61 property, but the ceiling is permanent: our ink never *is* the
official symbol, and each new mechanic's symbol is another drawing session.
Rejected as the primary, kept as the fallback.

**Hotlink `svgs.scryfall.io` directly.** The official art with no backend
work, but prose pips — the app's most load-bearing text — would depend on a
third-party CDN at render time, which is the exact regression PR #61
refused. Rejected.

**Serve the official symbols from our own origin, out of a runtime cache.**
`GET /api/symbols/{code}.svg` fetches a symbol from Scryfall the first time
anybody asks, caches it under `data/cache/symbols/`, and serves the local
file ever after with a week of browser caching. Chosen.

## Decision

The third option. Its properties, against PR #61's three points:

- **Prose still depends on nobody at render time.** After first fill, a pip
  is a local file read from the app's own origin. The client's drawn glyphs
  remain in the bundle as the fallback — a cold cache with no network shows
  the sun, drop, skull, flame and tree exactly as before, and letters for
  the rest. Degradation, not breakage.
- **Nothing enters git.** The cache lives with the pool, the sim cache and
  the motion derivatives: gitignored locally, on the volume deployed, never
  in the image. Rule 5 and ADR 6 stand untouched.
- **The full vocabulary arrives for free.** Every symbol Scryfall draws —
  numerals in their proper circles, `{T}`, `{Q}`, `{C}`, `{W/U}` split
  discs, `{2/W}`, Phyrexian phi, snow — is one URL away, and new sets'
  symbols need no drawing session.

Mechanics: the code in the URL is the braced symbol with punctuation
dropped (`{W/U}` → `WU.svg`), a shape check (`[0-9A-Z]{1,10}`) is the
path-traversal guard on both server and client, downloads are bounded and
sanity-checked so nothing that is not an SVG is ever cached, and a
Scryfall 404 is remembered in memory so a typo'd note cannot re-ask on
every render. Because every pip is a drawing now, every pip carries an
accessible name (`symbolName`), the numerals included.

## Consequences

- `Pip`, `ColorRing` and `ColorPips` render the official art everywhere;
  the coloured disc underneath doubles as loading placeholder and fallback
  canvas. The pentagram keeps the drawn glyphs at its vertices — they are
  part of one hand-drawn diagram, not pips.
- A numeral in prose no longer contributes its character to the page; it
  contributes a name ("Generic 2"). Tests that asserted on pip text assert
  on names now.
- The route is classified SHARED in the isolation suite: a shared cache
  over public symbol art, nothing per-account.
- A fresh instance's first render of a symbol pays one outbound fetch per
  symbol; a instance that can never reach Scryfall keeps the PR #61
  experience wholesale.
