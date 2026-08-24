# Red — Speed & Alarum

Four facets: the CI/CD pipeline, alerting & self-healing, the hot-spot
patrol, and the controls — commandment 17 made checkable. Red is speed, the
fire alarm, and the answered impulse: the pipeline that answers fast, the
bell that rings before Aaron's friends notice the site is down, the patrol
that finds where the time goes while it is still smoke, and the button that
replies the instant a hand reaches for it. A control belongs to Red for the
same reason lightning does — its whole virtue is the speed of its reply.

## Facet: the hot-spot patrol

Black profiles when a number looks wrong; until this facet, nobody profiled
when nothing did. The patrol is the proactive half — find where the time
actually goes while it is still nobody's complaint — and it sits beside the
fire alarm because that is what it is: a hot spot is smoke, and smoke found
early is a fix chosen calmly instead of a Saturday debugging session.

**The division of labor is one sentence: Red finds *where*; Black owns
*whether and how*.** The patrol produces a ranking and never an optimisation.
Anything worth chasing is handed to Black's discipline — benchstat
before/after, the alloc analysis, the fingerprint caveat that stops a 2% win
from dumping the Tier 1 cache — because an optimisation taken without that
discipline is how the expensive mistakes in Black's ledger got made.

The method, with the tools that exist today:

- **Profile the hot packages under test-shaped load.** The serving process
  has no profiling mount (checked 2026-08-23 — no pprof anywhere in the
  tree), so the patrol profiles at the package seam: `-cpuprofile` and
  `-memprofile` over the suites of the packages the routes lean on
  (`library`, `deckread`, `gate`, `sim/tier1`, `api` itself), then
  `go tool pprof -top -nodecount=25` on each. Test-shaped load is not
  request-shaped load — say so in the ledger line — but a function fat in
  both is fat.
- **Clock every route a session actually hits, from outside, warm and cold**
  — shared with the alerting facet's probe, one measurement recorded once
  (the rule Green and Black already follow). The route clock is what catches
  the flat-profile case: a request spending its time in cgo or in waiting
  shows a quiet profile and a loud clock, and the difference *is* the
  finding.
- **The record is a ranking, and the trend is the alarm.** Top-10 CPU and
  top-10 alloc into the ledger each patrol, beside last patrol's. A function
  that climbed three places is news even when the wall time has not moved —
  that is smoke before flame, the whole reason this lives in Red.
- **Two standing blind spots, so the patrol never files a false alarm**: cgo
  (pool time lands under `runtime.cgocall` with no shape — clock the
  database at the query, per Black's shelf) and the sampling floor (a
  sampled profile cannot see a function that runs two milliseconds per
  request; the route clock catches what the profiler cannot).

The upgrade that would let the patrol profile the *serving process itself* —
a pprof mount, dev-local and perhaps admin-gated live — is a door change and
therefore Aaron's, queued in `DAYBREAK.md` with the security argument that
makes it his call.

## Facet: CI/CD

**Never write the required checks down. Read them back, every run:**

```bash
gh api repos/aasquier/sylvan-library/branches/main/protection \
  --jq .required_status_checks.contexts
```

This file named six for months and the answer is now eight; the list changed
under it twice without a word of prose noticing, which is precisely why
CLAUDE.md calls it a read-back rather than a count. Two structural facts about
that list *are* worth holding, because they are what a count hides:

- **A matrix leg gates separately.** `go` is a two-architecture matrix, so it
  supplies `go (amd64)` and `go (arm64)` as distinct contexts, and `image` has
  an arm64 sibling. Adding an architecture adds a required check.
- **CodeQL is not among them, deliberately.** It runs on every pull request
  and on a schedule, and `codeql.yml`'s own header argues the case: a scanner
  that blocks merges on a query-pack update is a gate that gets disabled in
  anger. Treat "green" as the required list *plus* a CodeQL run somebody
  actually read.

A green `main` deploys itself (ADR 23). The audit is runtime trend,
robustness, and whether the free tier has grown capabilities worth adopting.

- **Measure runtimes first**: `bash -lc 'gh run list --workflow ci.yml
  --limit 20 --json databaseId,conclusion,createdAt,updatedAt'` and per-job
  timings from `gh api` on recent runs. Record medians per job in the
  ledger; the question is the trend, not today's number. A job that grew 40%
  since last run has a cause — new tests, cold caches, runner changes — name
  it.
- Cache health: the Go module and build caches (`setup-go` keys on
  `go/go.sum`) and npm's actually hitting — read a recent run's log, do not
  assume — and keys not invalidating on every run. The Go build cache is the
  one that pays: a cold one rebuilds the DuckDB bindings on both architectures.
- Concurrency: superseded runs on the same branch should cancel
  (`concurrency` groups) — pushing three fixups should not queue three full
  suites. Check it is configured; add it if not (safe fix).
- Actions hygiene: third-party actions pinned (by SHA for anything
  non-GitHub-authored), permissions blocks minimal (`contents: read` unless
  a job needs more), no `pull_request_target` foot-guns. This repo is
  public; its workflows are an attack surface.
- The pinned invariants stay pinned: `image` and `image-arm64` as the only
  container builds anywhere (neither runnable on this Mac), the `go` matrix as
  the only arm64 compiler this project has, and **`deploy` naming every other
  job in `ci.yml` in its `needs`.** Verify that last one by reading both lists
  in the file, because *nothing checks it any more* — it used to be a test
  that derived the expected set from the file's own job list, and that test
  died with the old suite. An unguarded invariant is this pass's own standing
  lesson: rebuilding it in Go is a queued item, and until it lands, a job
  added without `needs` deploys off a partial suite and nothing says so.
- **Free-tier feature audit**: check GitHub's changelog for features now free
  for public repos — merge queue, artifact attestations, better caching,
  required workflows. Adoptions that change contributor workflow are queued;
  pure-win config (a better cache key) is a safe fix.
- Local/CI parity: everything CI checks must be runnable locally except
  **four** — `image` and `image-arm64` (no container runtime on this Mac),
  `dependency-review` (it diffs a base against a head, and a laptop has no
  base), and **CodeQL**, which wants the CLI and its query packs and is in no
  runbook. The arm64 leg is a half-exception worth naming: the tests
  themselves run locally, but only on amd64, so an arm64-specific failure is
  CI's to find. CodeQL is the one that bites: White's first run spent four
  commits teaching a correct guard to satisfy a path-injection query, which is
  "CI is a surprise" happening. It is tolerable only because CodeQL does not
  gate merging — budget for it on a security fix and never let it become
  required without the local story first. A *fifth* such check violates the
  rule outright — flag it.

## Facet: alerting & self-healing

The goal state: something breaks — the machine, the volume, a deploy, a
migration — and Aaron's *phone* knows before a friend does, with no human in
the loop on detection. Text over email is his stated preference.

Most of this facet is infrastructure that costs a decision or a dollar, so
the run's job is usually to **measure the current posture, keep the gap list
current, and sharpen the queued proposals** — not to build monitoring nobody
approved.

- Posture inventory, every run: what watches the site right now? Fly's own
  health checks (`fly.toml` — are HTTP checks even configured, and does a
  failing check restart the machine?), Fly's status emails, GitHub's deploy
  job failure emails, and anything adopted since last run. Write the list in
  the ledger; the deltas are the story.
- Probe the instance from outside during the run (uptime, TLS expiry,
  response time of `/` and an API route) and record the numbers — a manual
  probe now is the baseline for the automated one later.
- The standing proposal set to keep sharp (all queued-class, each needs
  Aaron's yes and possibly dollars):
  - **External uptime monitoring** — a free checker (UptimeRobot,
    healthchecks.io) hitting a health endpoint; or a scheduled GitHub
    Action doing the same for zero new accounts, with the caveat that
    Actions cron is best-effort.
  - **Text delivery** — true SMS costs money (Twilio); the free-adjacent
    paths are carrier email-to-SMS gateways (fragile), ntfy.sh push
    (free, self-hostable), or Pushover (one-time $5). A recommendation
    should compare reliability, not just price.
  - **A health endpoint worth probing** — one that checks the volume is
    mounted, `app.db` opens, and disk headroom exists, so a probe
    detects sickness and not just liveness.
  - **Admin-page surfacing** — Green's facet owns resource *numbers*; the
    alerting tie-in is thresholds: the admin page showing a number is
    Aaron checking; a threshold firing a push is nobody needing to.
- **The expiry calendar.** A probe watches the site; nothing watches a date
  unless a run does, and a lapse is the outage no monitor explains. Enumerate
  everything that expires and record the nearest three in the ledger: the
  domain registration for sylvan-libraries.com (the site going dark with DNS
  intact-looking), TLS (Fly renews it — verify the date, never assume the
  renewal), the Anthropic key's rotation cycle (it is short-lived by policy;
  a 401 on every Claude surface is what expiry looks like), and the Fly
  payment method. Anything inside sixty days is a daybreak line with the
  renewal step attached.
- Self-healing posture: Fly restart policy on the machine, what happens on
  OOM, and the known hard edge — **schema migrations apply on boot,
  forward-only, unwatched**. Any proposal that increases automatic restarts
  must reckon with that edge, and say so when queued.
- Failure-tolerance review: single machine, single volume is the accepted
  design (held awake deliberately). Don't re-litigate it; do keep the
  recovery runbook honest — snapshots current, and **the restore drill dated
  in the ledger**: the last time the documented restore path was actually
  walked end to end, against a scratch target, and what it produced. A drill
  older than the newest schema migration is due, because the ladder is
  forward-only and a restore crosses it. The volume is the library's one
  standing copy (ADR 30); an untested backup of the only copy is a hope
  wearing a procedure's name.

## Facet: controls — commandment 17, made checkable

Commandment 17 says every control answers the hand that reaches for it. It is
the commandment most often satisfied *in the abstract* and missed on the
actual element, because a control can look finished while doing none of it.
Aaron's standing complaint, 2026-08-23: dull buttons, buttons that keep
accepting clicks after the first one, and links doing a toggle's job.

**Walk the surfaces in a real browser and press things.** jsdom cannot see a
hover state, a focus ring, or a second click landing. Then work the list —
each item is a grep or a press, not a judgment call:

- **Does it reply to hover, focus *and* press?** Three separate states, and
  focus is the one that gets skipped: keyboard users get no hover, so a
  control with `:hover` styling and no `:focus-visible` is invisible to them.
  A control that changes on hover alone is two-thirds done.
- **Is it in the shared control vocabulary?** `web/src/index.css` holds the
  `.btn` family for actions and `.chip-toggle` / `.strip-tab` and their
  siblings for controls that are *places* rather than actions. Measured
  2026-08-23: **131 button tags outside tests, 21 of them wearing no class
  from that vocabulary.** Not all 21 are bugs — the tarot reader tiles, the
  art picker and the wheel are deliberately bespoke and carry their own named
  classes — so the test is not "does it have a `.btn`" but **"is there one
  named place where this control's three states are defined?"** A control
  styled only by inline `style={{…}}` fails that by construction, because
  **a `:hover` can never reach an inline style** — which is how the last
  hundred dull buttons happened, and there are 648 inline style props under
  `web/src` for the sweep to work through.
- **Does a click that starts work stop accepting clicks?** Measured
  2026-08-23: **19 buttons start async work on click and 6 of them never
  disable** — including `save()` and the deck page's return-a-card control,
  which are *writes*, so the failure mode is a double edit rather than a
  double read. The pattern is a busy flag driving `disabled` **and** a visible
  pending state (the shared `Spinner`, or the button's own label changing).
  Disabling with no visible change reads as broken; a spinner with no
  `disabled` still double-submits. Both halves or neither.
- **Is it the right element for the job?** A link navigates; a button acts; a
  thing with an on and an off state is a toggle and should say so with
  `aria-pressed` or `role="switch"`, not be a link that happens to change
  colour. Aaron named this one specifically. An `<a>` with an `onClick` and no
  `href` is the tell.
- **Is the disabled reason legible?** A control disabled with no explanation
  is a dead end, and commandment 2 makes that a real cost — a newcomer
  assumes they broke it. A `title`, a helper line, or a tooltip saying *what
  would enable this* is part of the control.
- **Does it survive the keyboard and the phone?** Tab to it, press Enter and
  Space; then check it at a touch size (Green owns the 44px floor, this facet
  owns whether the hit target is the visible thing).

Record which surfaces were walked and which controls were fixed, and — the
part that keeps this from restarting every cycle — **which were examined and
deliberately left bespoke, with the reason.**
