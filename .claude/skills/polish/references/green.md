# Green — Growth & Resilience

Three facets: browser and mobile compatibility, the cloud resource watch, and
scalability. Green is the color that adapts to every terrain and grows
without breaking — the site working on a phone in someone's kitchen, the
volume never silently filling, the architecture bending instead of snapping
when ten users become a hundred.

## Facet: browser & mobile compatibility

Commandment 2 makes this concrete: the newcomer someone shares the site with
is opening it on *their* phone, in *their* browser. It has to just work.

- The compatibility floor is real and personal: **Safari 15 on macOS 12 is
  the dev machine's own browser** — but check the ledger before repeating that
  number, because the 2026-08-16 run found the shipped bundle needs 16.4 and
  queued the question of which way to resolve it. Check newer JS/CSS features
  against whatever the floor actually is — `:has()`, container queries,
  `structuredClone`, top-level await in served code. When Vite's target and
  reality disagree, reality is the phone that renders white.
- **Audit `src/mtglab/web_dist/assets/`, not `web/src`, and run
  `tests/test_browser_floor.py` rather than grepping.** This is the correction
  that run earned the hard way: the floor moved from 15 to 16.4 the day
  Tailwind v4 landed, and no source file changed — v4 emits `@property` and
  `color-mix(in lab, …)` into the *bundle*. A grep of `web/src` cannot see
  what a phone parses. Two further traps in the old instruction: below the
  floor the failure is **quiet** (unsupported `@property` declarations are
  dropped, so shadows, transforms, filters and rings vanish while the layout
  stays correct), and `grep '(?<'` has a built-in false positive, since a
  named capture group `(?<name>…)` is not a lookbehind.
- Mobile Safari's quirks are the usual suspects; audit the surfaces changed
  since last run for them: viewport height (`100vh` vs dynamic toolbars —
  prefer `dvh`/`svh` with fallback), `env(safe-area-inset-*)` on notched
  phones, hover states that trap touch users (a menu needing hover has no
  phone story), 300ms-tap and double-tap-zoom on interactive elements,
  fixed-position elements over the keyboard.
- Touch targets: interactive elements at ≥44px effective size on the deck
  page's dense controls. Measure in a real mobile viewport
  (`resize_window` mobile preset drives a true touch profile), not jsdom.
- Responsive sweep of anything new since last run at phone, tablet, laptop
  widths — and both themes. A screenshot at each width is the evidence;
  "the classes look right" is not.
- Motion accessibility: Commandment 6 wants a living page *and*
  `prefers-reduced-motion` is a promise to users who get motion-sick.
  Animations should respect it — reduced, not necessarily removed.
- Cross-browser: Chrome, Firefox, Safari on desktop; Safari and Chrome on
  mobile. The practical method is feature-floor discipline plus real-device
  spot checks when Aaron can; flag anything that needs his physical phone.

## Facet: cloud resource watch

The failure Aaron named: the database fills, nobody noticed, service
degrades. Proactive means the numbers are recorded *every run* and the
trend is read.

- Volume: `bash -lc 'fly ssh console -C "df -h /data"'` — used, available,
  percent. Then the breakdown: `app.db` size, `mtg.duckdb` size, decks
  tree, anything unexpected growing. Record all of it.
- Machine: memory and CPU headroom (`fly status`, machine metrics), OOM
  events since last run, restart count.
- Snapshots: retention and recency — is the newest snapshot newer than the
  newest schema migration?
- Response times from outside, cold and warm (Black measures for speed; this
  facet watches for *degradation* — same numbers, different question, so
  share the measurement and record it once).
- Fly free-tier/plan posture: what the project is on, what it is near the
  edge of (machine count, volume GB, bandwidth). A limit within one growth
  step is a queued finding with the price of the next tier attached.
- Thresholds, not vigilance: the standing direction is to surface these on
  the Admin page — centralised, per Aaron — with Red's alerting facet owning
  the "then it texts me" half. Keep the admin-surfacing proposal sharp in
  the queue: which numbers, which thresholds, which endpoint serves them
  (admin-mounted, 403 to non-admins, per ADR 17).

## Facet: scalability & user adaptability

The design point is **100 accounts, 10 concurrent** — chosen, not emergent.
The facet's job is to keep that number a *setting* rather than an
assumption, so the day it changes is a config edit and a re-measure, not an
archaeology dig.

- Inventory where the design point lives in code: rate-limit constants, job
  pool sizes (CPU=1 deliberate and GIL-bound; NET pool width), uvicorn
  worker count, SQLite's single-writer posture, session/token table growth,
  invite volume. Anything hard-coded that would need to move at 10× belongs
  in one documented place — a finding if scattered, a safe fix if
  centralising it is surgical.
- SQLite is the accepted store (ADR 4) and fine at this scale — do not
  propose Postgres; do check the pragmas serve concurrent reads (WAL mode,
  busy timeout) and that write paths hold transactions briefly.
- Find the actual first bottleneck, with evidence: is it the single CPU sim
  worker queueing under concurrent sim requests? SQLite writes? The
  machine's memory? A cheap local load probe (a dozen concurrent requests
  against a local instance) tells more than speculation; record what breaks
  first and at what N.
- Growth levers, kept documented rather than pulled: second Fly machine
  (blocked by single-volume design — say so honestly), bigger machine,
  NET/CPU pool widening, per-user quotas on expensive surfaces (Claude
  modes are the costly ones — tie to Black's spend numbers).
- The adaptability test: could the design point move to 500/50 with a
  config change and a re-run of this facet? Every "no, because X" is either
  a finding to fix or a documented lever with its trigger.
