# Red — Speed & Alarum

Two facets: the CI/CD pipeline, and alerting & self-healing. Red is speed and
the fire alarm — the pipeline that answers fast, and the bell that rings
before Aaron's friends notice the site is down.

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
- Self-healing posture: Fly restart policy on the machine, what happens on
  OOM, and the known hard edge — **schema migrations apply on boot,
  forward-only, unwatched**. Any proposal that increases automatic restarts
  must reckon with that edge, and say so when queued.
- Failure-tolerance review: single machine, single volume is the accepted
  design (held awake deliberately). Don't re-litigate it; do keep the
  recovery runbook honest — snapshots current, restore path documented in
  HOSTING.md and actually tried at least once since it last changed.
