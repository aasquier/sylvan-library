# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

---

## Open — 2026-08-23

**1. The 95% coverage floor is enforced by nothing, and coverage is not 95%.**
CI runs `go test -race -count=1 -cover ./...`, which *prints* a number and
gates on nothing; no threshold exists in `ci.yml`, `.golangci.yml` or any doc.
Only the polish skill still asserts the floor. Measured 2026-08-23, and it
takes two numbers because Go reports two different things: **80.3%** of
statements covered by the whole suite (`-coverpkg=./...`, the number
comparable to a floor) and **74.1%** counting only each package's own tests
(plain `-cover`, the default). · *Cost of leaving it:* the pass keeps citing a
floor that does not exist, which is the exact drift it was written to hunt. ·
**Recommendation:** keep it a *watched* measurement rather than a gate —
record both numbers every White run and treat a fall as a finding — and delete
the "floor" language. A hard threshold set today would either fail CI
immediately or sit so low it certifies nothing. Say the word and it becomes a
gate instead.

**2. Adopt `gremlins` for mutation testing?** The in-repo harness died with
the old backend. Of the live options, `go-gremlins/gremlins` is the strongest
fit (391 stars, active, standalone binary, mutation-score thresholds);
`gtramontina/ooze` is more recently active but is a `go.mod` dependency that
runs inside `go test`. · *Cost of leaving it:* mutation sampling stays a hand
protocol, and the survivors the old ledger recorded stay unasked. ·
**Recommendation:** gremlins, installed on demand exactly as `go-licenses`
already is — no `go.mod` entry, no dependency surface. Run it one package at a
time on the determinism kernels first (`floats`, `mt19937`, `textutil`,
`yamlemit`, `gate`), never on `internal/api`, which is 63s per test run before
a single mutant.

**3. ADR 38 cites `docs/go-migration/`, twice, and the directory is gone.**
The zero-trace sweep deleted it; the ADR links it in its header and its
context. ADRs are immutable. · *Cost of leaving it:* an accepted record points
at nothing, and every future reader of ADR 38 hits it. · **Recommendation:** a
short superseding note recording that the directory was deliberately removed
and where its content went — cheaper than restoring it, and honest about why.

**4. The `deploy` job's `needs` list is checked by nothing.** The test that
derived the expected job set from `ci.yml`'s own job list died with the old
suite. A job added without `needs` now deploys off a partial suite, silently.
· *Cost of leaving it:* one forgotten line ships an unverified deploy. ·
**Recommendation:** rebuild it in Go as a test that parses `ci.yml` and
derives the set — it was a real guard and it is a small one.

**5. Does night work merge itself?** The skill's current rule, written
2026-08-23 and derived from commandments 14 and 16 rather than from a ruling:
anything a user can see stops at a green PR for Aaron's eye; everything else
may merge when the required checks are green. · *Cost of leaving it:* nothing
— it is already the conservative reading. · **Recommendation:** confirm it, or
tighten it to "a night run never merges" if a 3am deploy is unwelcome at all.

---

## Answered

*(Nothing yet. Answered items move here in one line with the ruling and the
date, then out entirely once the ledger carries them.)*
