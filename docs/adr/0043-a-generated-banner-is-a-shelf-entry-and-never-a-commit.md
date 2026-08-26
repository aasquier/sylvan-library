# 43. A generated banner is a shelf entry, and never a commit

**Status:** Accepted · **Decided:** 2026-08-25 · Widens the effect vocabulary
of [ADR 32](0032-card-art-motion-is-derived-at-runtime-and-never-committed.md)
without disturbing its storage rule.

## Context

Aaron generated a fifteen-second video from *Grand Coliseum* (Onslaught, Carl
Critchlow) — the painting the Coliseum's banner already shows — and wanted it
as the banner, with the title dropped because the picture says "coliseum"
better than the word does.

Three of this project's own rules were read against it, and one external one:

- **ADR 29 rule 3** — *"Wizards' art is animated at runtime only, never baked
  into a committed file. A sprite sheet, video loop or displacement map
  derived from a card painting is a committed derivative."* This is exactly
  that shape, and the repository is public, so committing it is
  redistribution.
- **ADR 29's licence gate** — public domain and CC0 only, confirmed through a
  provider API, no override. A derivative of a copyrighted painting cannot
  pass it, and nothing about this asset was fetched by the animist anyway.
- **ADR 32's effect vocabulary** — bounded to *"motion and parallax
  (`depth-drift`, `slow-pan`), never distortion, blur or colour-shift of the
  artwork"*, because Scryfall's imagery guidelines constrain how their
  imagery is presented. ADR 32 also says: *"Adding an effect means re-reading
  that list."* This ADR is that reading.
- **The Fan Content Policy**, which was first read here too strictly. It
  *permits* free non-commercial fan content built on Wizards' IP — which is
  what this site is, and what ADR 32 already said. What it forbids is
  "verbatim copying and reposting": bulk redistribution of Wizards' products,
  not fan-made derivative work. The policy was not the obstacle; the public
  repository was.

## Options considered

**Commit the mp4 beside the other assets.** The obvious move and the one
rejected first: it is a derivative of Wizards' art in a public repository,
which ADR 29 rule 3 forbids by name, and it would sit in an asset directory
whose whole contract is that everything in it has a recipe and a licence
confirmation. Six and a half megabytes of unverifiable binary is the exact
thing that pipeline was built to refuse.

**Rebuild the effect as runtime CSS or WebGL over the hotlinked still.** This
was offered first and is genuinely achievable — a darkening sky, a moon, warm
window lights — and it would have needed no new storage rule at all. Rejected
because it is not the same artefact: what Aaron has is a rendered frame with
crowds filling the stands and light moving through the gatehouses, and a
gradient over a JPEG is a different and lesser thing pretending to be it.
Worth keeping in mind for arenas that never get a loop.

**Serve it from the volume as a cardmotion entry.** Taken. The storage rule,
the two routes, the fingerprint keying, the attribution file and the
`fly ssh sftp put` runbook all already existed for precisely this class of
object, and none of them needed changing. The only thing that had to move was
the effect vocabulary, which ADR 32 anticipated being re-read.

**Drop it.** The honest fourth option, and the one that was on the table while
the Fan Content Policy was being read too strictly. Once that reading was
corrected — the policy permits free fan content and forbids bulk
redistribution of products — the objection that remained was about the public
repository, and the option above answers that completely.

## Decision

**The banner is a cardmotion shelf entry.** It lives at
`data/cache/cardmotion/grand-coliseum-daynight/` — gitignored, on the volume,
served by the two routes ADR 32 already built, keyed on the painting's
`oracle_id` and the effect fingerprint like every other derivative. Nothing
about a *generated* derivative needed new machinery; ADR 32's storage rule was
already the answer, and it keeps ADR 29 rule 3 satisfied to the letter: no
derivative of Wizards' art is committed.

**`daynight` joins the effect vocabulary, and it is honestly outside the old
line.** A day-to-night pass is a colour-shift, which the bounded list excluded.
Two things separate it from what that bound was protecting against. The bound
comes from Scryfall's guidelines about presenting *their* imagery — this is not
Scryfall's imagery being re-presented, it is a new frame generated from the
painting, and the still it replaces is still the untouched hotlink underneath.
And the effect is a whole rendering rather than a filter over the plate, so
there is no card image being distorted in place. It is a widening, and it is
recorded as one rather than assimilated quietly.

**The credit changes with what is on screen, and that is not decoration.**
Over the plate the footnote is a credit — *"Grand Coliseum, Onslaught — art by
Carl Critchlow"*. Over the loop it is an acknowledgement — *"Motion inspired by
Grand Coliseum, Onslaught — art by Carl Critchlow"* — because what is playing
was generated from his painting and is not his work, and "art by" alone over it
would put his name on something he did not make. The same sentence rides inside
the entry's `attribution.json` as `inspired_by`, so the acknowledgement travels
with the file rather than living only in a component.

**One derivative is not regenerable, and the runbook says so.** Every other
shelf entry rebuilds from the pool with `cardmotion sync`; this one was
rendered outside the toolbox. Losing it costs the banner and the room falls
back to the still, which is the same floor every motion surface has.

## Consequences

- The Coliseum's visible `<h1>` is gone; an `sr-only` one stays, because a
  screen reader has no picture to be spoken for by and a page whose outline
  starts nowhere is a page nobody can navigate.
- `daynight` is a real effect key, so a second arena could get a banner the
  same way — and the same widening argument would have to be made again, not
  inherited.
- Reduced motion gets the painting rather than a poster frame: a fifteen-second
  day-to-night pass is motion by anybody's definition, and the plate is the
  better still because a person made it.
- The animist and its licence gate are untouched. This asset never enters that
  pipeline, has no recipe, and is not committed — the three properties that
  make ADR 29's gate meaningful are all preserved by keeping it out.
