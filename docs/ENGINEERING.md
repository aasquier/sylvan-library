# Engineering direction

The standing engineering calls, one section per area. CI's comments cite §3
and §5 by number; the numbering is load-bearing. History and the arguments
that led here live in git and in `docs/adr/`.

## 1. The backend

One Go binary (`mtglab`), CGO on for DuckDB, serving everything.
Performance is measured rather than assumed — `docs/polish/LEDGER.md` keeps
the baselines — and the deliberate floor is that the app is free to use, so
nobody is paying us to be slow. The measuring shelf itself — a bench command,
cache-hit counters — does not exist yet, and building it is an open ledger
item (`go test -bench` and pprof serve in the meantime).

## 2. Testing

- **Every bug fix gets a test**, written to drive the trigger — the code
  path that failed — rather than the mechanism by hand.
- **The corpora under `testdata/` are frozen goldens.** They pin recorded
  behavior (seeds, wire shapes, cache-key bytes, emitted YAML). Never
  regenerate one, and never "fix" arithmetic that matches one.
- **Property-based tests and fuzzing** cover the math-shaped code (the
  gate, the solver, the generator's `fuzz_test.go`), and **mutation testing
  is `gremlins`** — a standalone binary installed on demand
  (`go install github.com/go-gremlins/gremlins/cmd/gremlins@latest`), run one
  package at a time, no `go.mod` entry and no CI gate on it yet. A suite's
  strength is still checked the manual way wherever that is quicker: break
  the thing, watch the test fail, restore it.
- The Go gates, from `go/`: `go vet ./...`, `go test -race ./...`,
  `golangci-lint run ./...`, and `gofmt -l .` printing nothing.
- Frontend tests run through `npm --prefix web run check` — vitest invoked
  from the repo root loads no config and manufactures flakes.

## 3. Containerisation

`Dockerfile`, `docker-entrypoint.sh`, `.dockerignore` and `fly.toml` are in
the repository; the `image` job builds and exercises the image on every PR.
The properties, each asserted against a running container:

- **Multi-stage**: a Go builder stage; the runtime is `debian:trixie-slim`
  plus `libstdc++6`, carrying one binary and the committed assets — no
  toolchain, no package manager use at runtime.
- **No Node stage, deliberately.** The `frontend` job runs the real
  `npm run build` and fails on any diff against the committed `web_dist/`.
  CI proves the bundle is current; the image ships it.
- **Non-root.** PID 1 starts as root, fixes `/data`'s ownership (Fly mounts
  volumes `root:root`), and `exec`s the app as `mtglab` (uid 10001) under
  `setpriv`. CI asserts the owner of the running process.
- **Read-only root proven by test**: a second container runs with
  `--read-only --tmpfs /tmp` and must answer health. The deployed instance
  does not run read-only; the check exists for the invariant underneath —
  **everything this app writes lives under `/data`** — whose failure mode
  (a write into the image layer) works perfectly until a deploy discards
  it. `/tmp` is genuinely written, so the tmpfs is load-bearing.
- **`HEALTHCHECK` is `mtglab probe`** against `/api/health`, a public path,
  and CI pins that a fresh volume reports `"pool": false` rather than
  failing — unseeded is a correct state, and a 500 there would have the
  platform restarting a healthy machine forever.
- **Multi-arch** (`linux/amd64` required, `linux/arm64` as `image-arm64`)
  via buildx.
- **Trivy scanning**, failing on HIGH/CRITICAL, `ignore-unfixed` so a CVE
  with no patch does not turn the gate into ignored noise.
- **Never bake the pool in.** Scryfall's bulk data is not redistributed:
  the `no-secrets-or-card-data` job checks what is tracked, and the `image`
  job greps the built image — a different question the moment a
  `.dockerignore` line is deleted.
- **Committed assets match their recipes** (ADR 29): the `tools` job runs
  `animist verify` against every committed asset.

**Why CI carries all of this**: the dev Mac (macOS 12, Intel) cannot build
containers at all — Docker Desktop will not install and Homebrew is too
stale for Colima. The `image` job is the only place the Dockerfile is ever
built, so a red one on a container change is the first real feedback, not a
flake.

## 4. Frontend

React + TypeScript under `web/`, bundle committed at `web_dist/` (CI proves
it rebuilds byte-identically). `noUncheckedIndexedAccess` is on. No regex
lookbehind under `web/src` — the supported-browser floor includes WebKit
without it. The check gate is `npm --prefix web run check`; run
`npm --prefix web run build` whenever `web/src` changes.

## 5. CI/CD

The repository is public and `main` is protected. The settings, recorded so
they can be rebuilt — and **the check list is a read-back, never an
inheritance**; it has been stale in this file more than once:

| Setting | Value |
| --- | --- |
| Pull request required | yes, 0 approvals (a solo maintainer cannot approve their own) |
| Required checks | `frontend`, `image`, `no-secrets-or-card-data`, `dependency-review`, `image-arm64`, `go (amd64)`, `go (arm64)`, `go-lint` — **eight, read back from the API 2026-08-24** |
| Strict (branch up to date) | yes |
| Enforce for admins | yes — off, it would not apply to the only contributor |
| Force pushes, deletions | blocked |
| Linear history | required (squash merges) |

Re-read the list rather than trusting the row:

```bash
gh api repos/aasquier/sylvan-library/branches/main/protection --jq .required_status_checks.checks
```

**Adding a CI job is two steps** — writing it, and requiring it — and the
second has no artifact in the repository, so it is the one that gets
forgotten. Append and remove required contexts with:

```bash
gh api -X POST repos/aasquier/sylvan-library/branches/main/protection/required_status_checks/contexts -f 'contexts[]=<job-id>'
```

(the same path with `-X DELETE` removes one). A required context matches the
**check-run name** — the job's `name:` if set, its id otherwise — not the
workflow's `name:`.

`dependency-review` runs only on `pull_request` (a push to `main` has no
base to diff against), which is why the `deploy` job's `needs` list and the
protection list differ on purpose: protection governs merging,
`needs` governs the deploy, and requiring a check that never fires on a
push would deadlock every deploy.

**Merging deploys** (ADR 23): a green push to `main` is live ~10 minutes
later. `docs/HOSTING.md` is the runbook, including rollback and the deploy
token's expiry.
