# Red — Speed & Alarum

Two facets: the CI/CD pipeline, and alerting & self-healing. Red is speed and
the fire alarm — the pipeline that answers fast, and the bell that rings
before Aaron's friends notice the site is down.

## Facet: CI/CD

Six checks are **required** on `main`, and they are not the six this file
used to name. Read from the protection setting rather than from memory
(2026-08-16): `test (3.11)`, `test (3.12)`, `frontend`, `image`,
`no-secrets-or-card-data`, `dependency-review`. So `ci.yml` supplies five
contexts and not four — `test` is a matrix and each leg gates separately —
and **CodeQL is not among them**. It runs on every pull request and on a
schedule, and `codeql.yml`'s own header says why it is deliberately advisory:
a scanner that blocks merges on a query-pack update is a gate that gets
disabled in anger. Treat "green" as those six plus a CodeQL run somebody
actually read. A green main deploys itself (ADR 23). The audit is runtime
trend, robustness, and whether the free tier has grown new capabilities worth
adopting.

- **Measure runtimes first**: `bash -lc 'gh run list --workflow ci.yml
  --limit 20 --json databaseId,conclusion,createdAt,updatedAt'` and per-job
  timings from `gh api` on recent runs. Record medians per job in the
  ledger; the question is the trend, not today's number. A job that grew 40%
  since last run has a cause — new tests, cold caches, runner changes — name
  it.
- Cache health: pip and npm caches actually hitting (read a recent run's
  log, don't assume), keys not invalidating on every run.
- Concurrency: superseded runs on the same branch should cancel
  (`concurrency` groups) — pushing three fixups should not queue three full
  suites. Check it is configured; add it if not (safe fix).
- Actions hygiene: third-party actions pinned (by SHA for anything
  non-GitHub-authored), permissions blocks minimal (`contents: read` unless
  a job needs more), no `pull_request_target` foot-guns. This repo is
  public; its workflows are an attack surface.
- The pinned invariants stay pinned: the skip gate at 2 — **that is CI's
  number, and the local suite skips 0**, because this Mac has the pool and
  both `needs_full_pool` tests run — the `image` job as the only container
  build (never runnable on this Mac), and deploy `needs` every other job in
  `ci.yml`. The last of those is a test now
  (`test_the_deploy_job_waits_for_every_other_job_in_the_file`), and it
  **derives** the expected set from the file's own job list rather than
  restating it, so adding a job and forgetting `needs` fails locally instead
  of shipping a red suite. Verify the rest, don't assume.
- **Free-tier feature audit**: check GitHub's changelog for features now free
  for public repos — merge queue, artifact attestations, better caching,
  required workflows. Adoptions that change contributor workflow are queued;
  pure-win config (a better cache key) is a safe fix.
- Local/CI parity: everything CI checks must be runnable locally except
  **three** — `image` (no container runtime on this Mac), `dependency-review`
  (it diffs a base against a head, and a laptop has no base), and **CodeQL**,
  which wants the CodeQL CLI and its query packs and is in neither the
  `[dev]` extra nor the runbook. The third is the one that bites: White's
  first run spent four commits teaching a correct guard to satisfy
  `py/path-injection`, which is "CI is a surprise" happening. It is tolerable
  only because CodeQL does not gate merging — budget for it on a security fix
  and never let it become required without the local story first. A *fourth*
  such check violates the rule outright — flag it.

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
