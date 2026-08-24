# White — Law & Protection

Three facets: free-use and licensing compliance (triple-checked), security and
user isolation, and testing discipline. White is the color of rules held for
the good of everyone at the table — the licence that lets this project exist,
the isolation that lets a friend trust it with their cards, and the suite that
lets every other run move fast.

## Facet: free-use & licensing (triple-check rigor)

The stakes, plainly: this project exists because Wizards of the Coast's Fan
Content Policy permits free fan projects. One violation — one committed piece
of their art, one monetized corner — and the standing to run the site at all
is gone. Commandment 9 makes this a hard boundary; Aaron has named this facet
the one to triple-check.

Triple-check means: for every asset and dependency you examine, (1) find the
claim of compliance, (2) verify it against the primary source — the licence
text, the recipe, the PROVENANCE entry — not a summary, and (3) confirm the
enforcement mechanism that keeps it true is still in place and still has no
override. Uncertain after that? Queue it for Aaron and treat it as
non-compliant until he rules.

Work the list:

- `mtglab animist verify` passes: every committed asset matches its recipe
  (ADR 29). Then sweep for binaries that *bypassed* the pipeline: compare
  `git ls-files` image/font/media files against what the recipes and
  `PROVENANCE.md` files account for. A hand-placed binary is a finding even
  if its licence turns out fine — the pipeline exists so nobody has to trust
  a memory.
- The licence gate (`mtglab animist licence`) still has no `--force` and no
  code path around it. Check the code, not the docs.
- Wizards' art is runtime-only, always: `PERSONA_ART` hotlinks with credit
  and nothing under `git ls-files` is a Wizards image. The tarot art is the
  1909 Rider printing — the 1971 recolouring is still in copyright, so any
  new tarot-adjacent asset needs its edition argued per file.
- ADR 6: no Scryfall bulk data, price data, or any redistribution of their
  files in the repo, the image, or an artifact. Scryfall attribution appears
  where their data renders.
- No monetization surface exists, even vestigially: no payment code, no
  donation links, no ad slots, nothing that takes a penny. Check the frontend
  too — a well-meaning "buy me a coffee" is a violation here.
- Dependency licences: sweep the Go module graph (`go-licenses report ./...`
  from `go/`, installed on demand), the tools/ toolbox's Python metadata,
  and `npm --prefix web ls` trees for licences incompatible with a free
  public project (AGPL in a dependency is a finding to queue, not necessarily fatal
  — Aaron rules). Record the sweep date in the ledger.
- Fonts, CSS, and anything served: each has a named free licence. If the
  provenance argument lives nowhere, that is the finding.

## Facet: security & user isolation

The design intent: isolation is the first thought. Anyone keeping cards
private on this site must actually have them private — from other users and
from accidents, not just from attackers.

- Every route is classified by the door's own sweep tests (derived from the served route table), and the sweep is
  live: try adding a fake unclassified route locally and confirm the suite
  fails (then remove it). The middleware refuses before routing — verify any
  new prefix landed in the right list.
- The 403/404 law: another person's things are **404** (ADR 5); an admin
  route to a non-admin is **403** (ADR 17, argued); deck writes are
  owner-only (ADR 22, #80). Check any route added since the last run against
  all three.
- Email addresses: `User.as_dict()` omits the address unless asked; exactly
  two callers may ask (`mtglab users list`, the admin routes). Grep for new
  serialisation paths, log lines, or tool results that could carry one.
- Session hygiene: cookie flags (HttpOnly, Secure, SameSite), Argon2id
  parameters against current OWASP guidance, rate limiting on login/reset
  still answering 429 with Retry-After, reset responses still uniform for
  existing and non-existing addresses.
- Tokens: invite/reset links are single-use, hashed at rest, and arrive in
  the URL fragment — never the query string, which would log a live
  credential. Confirm no new surface reintroduced a query-string token.
- Secrets: CI's filename and content scans still cover the tracked tree
  including `web_dist/`; `.env` gitignored; `fly.toml` carries no secrets;
  the Anthropic key reaches code only via environment.
- Supply chain and static analysis: read the latest CodeQL and
  dependency-review results rather than assuming green means examined; note
  anything dismissed and why.
- SQL: everything through parameterized queries behind `internal/auth` and
  `internal/pool`; string-built SQL anywhere is a finding.

### Fixing a security finding — the hard-won protocol

A real fix landed here (the SPA catch-all path traversal, PR #126) and cost
four commits to get green. The lessons are worth more than the fix:

- **Two jobs, not one: close the hole *and* satisfy the scanner.** The bug is
  fixed when the vulnerability is gone; the PR merges when CodeQL is also
  green. These are different, because CodeQL's model may not recognise a
  perfectly correct guard. Both `Path.is_relative_to` and a `startswith` on
  the resolved paths *contained* the traversal correctly — the test proved it
  — and CodeQL flagged both anyway, because it does not model them as
  barriers on this query.
- **When a guard isn't recognised, break the taint provenance instead of
  hunting for a guard form the scanner likes.** Do not build the sensitive
  value out of user input at all. The traversal fix stopped resolving
  `WEB_DIST / full_path` and made `full_path` a pure dict key, so the served
  `Path` comes from a trusted directory listing and no user input reaches the
  filesystem call — nothing for the taint tracker to follow. This is both
  safer *and* legible to the scanner, and it is the move to reach for first,
  not fourth.
- **Mutation-verify every security test.** Revert the guard, watch the test
  fail, restore it. A security test that passes against the *broken* code is
  worse than none — it certifies a hole as shut.
- **Verify on the live instance after deploy.** A merged fix auto-deploys
  (ADR 23); drive the real surface to confirm the hole is actually closed in
  production, because the whole class of deployment-only bugs lives in the gap
  between the local tree and the running instance.
- Each CI round on a fix like this is a ~5-minute image build. Diagnose from
  the *actual* alert (`gh api .../code-scanning/alerts`) — which sink, which
  line, new-on-this-PR vs pre-existing-on-main — rather than guessing and
  re-pushing; guessing is what made this four commits instead of two.

## Facet: testing discipline

The 95% floor exists to make regressions loud, but Aaron's bar is the *right*
tests, not coverage tests — and a suite that stays fast enough that adding
tests never feels expensive.

- **Check the environment before believing the run.** Compare the local
  test count against CI's — a passing suite that ran *fewer tests than CI*
  is the failure mode this facet exists for, and it reads exactly like
  success. It has happened: a missing extra once silently skipped the
  entire HTTP layer, isolation sweep included. A green local suite is
  evidence only once you know it is the same suite; on this Mac that also
  means the three exports (toolchain PATH+GOROOT, the CGO ldflag) are set,
  because without CGO the linter silently typechecks less.
- **Once per cycle, follow the documented setup from a clean checkout.**
  `git worktree add` to a scratch path, follow CLAUDE.md's Setup block
  *verbatim* — no extras nobody wrote down — and compare the test count
  with CI's. The documented instructions and the working environment drift
  apart indefinitely unless someone deliberately stands where a new
  contributor stands.
- Measure first: `go test -count=1 ./... 2>&1 | tail` for wall time, and
  `go test -json` piped through a duration sort for the slow tail. Record
  both in the ledger. A test that got slower has a reason; find it.
- Hunt duplicated setup: fixtures and helpers belong in the shared test
  helpers (`internal/pool/pooltest`, `internal/auth`'s authtest fixtures) —
  three tests hand-rolling the same scaffolding is a finding.
- Skips are a budget, not a convenience: every `t.Skip` in the tree is
  conditional on a real absence (a live instance, a Forge install, a full
  pool), and a drift in the skip census is a finding even when CI is green.
- No test sends mail, spends a token, or touches the network — confirm the
  seams (the mail sender, faked Claude turns, faked subprocesses) still
  hold for anything added since last run.
- Verify new guard tests by mutation, not by greenness: a test written to
  hold a boundary gets the boundary broken locally once to prove it fires
  (the conftest-hides-the-deployed-branch lesson).
- **Mutation sampling is suspended, not retired as a practice** (Aaron's
  standing ask, 2026-08-16): the harness died with the old backend and its
  Go rebuild is the ledger's open item. Until it lands, this facet's
  sampling is the hand protocol on a *throwaway copy* of a package — never
  the working tree — and the survivors the old ledger recorded stay listed
  there so the rebuilt tool can re-ask them by name on its first run.
- After the suite, **`git status data/` proves nothing** — `app.db` is
  gitignored, so a test that writes the developer's real database leaves
  the status clean. Use `ls -la data/` and treat a fresh mtime on
  `data/app.db` as the finding; a test reaching past its scratch directory
  gets fixed, never accommodated.
- Coverage: read the report for *meaningless* coverage too — a module at 100%
  through tests that assert nothing is worse than an honest gap, because it
  reads as done.
