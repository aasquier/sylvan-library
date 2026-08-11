# 6. Never redistribute Scryfall bulk data; refresh it in place, on demand

**Status:** Accepted · **Recorded:** 2026-08-10

## Context

The corpus is ~63 MB of DuckDB built from ~98 MB of compressed Scryfall bulk
(~500 MB uncompressed, several minutes to download and load). Scryfall publishes
those files daily and explicitly licenses them for this use, and asks that the
bulk data not be redistributed. It is regenerable by anyone in one command:
`mtglab data refresh`.

So the corpus is simultaneously essential to the tool and something this project
must not hand out. Where it lives, and when it gets rebuilt, follows from that.

## Options considered — where it lives

**Commit it to the repo.** Rejected: redistribution, and a 63 MB binary in git
history forever. CI enforces this with a job that greps the file list for
`data/*.duckdb` and friends.

**Bake it into the container image.** Rejected for the same reason — an image
someone can pull is redistribution — and because it belongs on a volume where it
survives a deploy instead of being rebuilt into every one.

**Volume, gitignored, rebuilt by the operator.** Chosen.

## Options considered — when it gets rebuilt

This is where three planning documents had drifted apart. Recording the
resolution is the main reason this ADR exists.

**As a build step.** No. It needs several minutes and blows any build budget.

**At boot.** No — and this is the one that reads plausible until you follow it
through. The deployment target uses scale-to-zero, so boot is *on the request
path*: a wake that takes minutes is a broken site. It would also re-download
~500 MB on every restart, which is precisely the traffic Scryfall's guidelines
ask you not to generate.

**On a cron schedule.** No, not on Fly. **Fly volumes attach to exactly one
machine**, so a scheduled second Machine cannot mount the volume the corpus
lives on. The obvious approach does not work, and discovering that during a
deploy is worse than writing it down here.

**By hand, on demand.** Chosen. `fly ssh console -C "mtglab data refresh"`,
about monthly. Scryfall publishes daily; deck tooling does not need day-fresh
data, and prices only matter to `price deck`.

## Decision

The corpus is never committed and never baked into an image. It lives on the
volume and is rebuilt by an explicit operator-run command — not at build, not at
boot, not by cron.

Alongside it, the constraints that come from the same source:

- **Hot-link card images** from `cards.scryfall.io` rather than proxying or
  rehosting them.
- **Send a descriptive User-Agent** — `mtg-lab/0.1 (local personal deckbuilding
  tool)` — and pull one bulk file rather than hammering the per-card API.
- **The Fan Content Policy is noncommercial.** Whatever this runs on stays free
  to use: no ads, no subscription, no donations tied to it. The disclaimer in
  the UI footer stays.

This supersedes "run it weekly by cron" in `ROADMAP.md` and "refresh at boot
instead" in the seed list in `docs/ENGINEERING.md` §6, both of which predate the
volume-affinity finding. Both have been corrected.

## Consequences

- A fresh deploy starts with **no corpus**, and that is expected rather than
  broken: `/api/health` reports corpus state, and `service._connect()` returns
  `None` rather than 500ing. This is the same fresh-clone case the API tests
  already cover, which is why it works.
- Card lookups are briefly unavailable *during* a refresh, because the refresh
  holds DuckDB's exclusive write lock — see
  [ADR 2](0002-duckdb-for-the-card-corpus.md). By design.
- The corpus needs no backup at all. That is the whole reason it is gitignored,
  and it is what makes the backup story in
  [ADR 4](0004-two-embedded-databases.md) small enough to actually do.
- If refreshing by hand turns out to be forgotten in practice, the next step is
  an authenticated admin endpoint that starts a refresh as a background job,
  called on a schedule from outside. Build that only after forgetting it twice.
