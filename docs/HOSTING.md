# Hosting sylvan-library

The maintainer's runbook for the deployed instance: what is running, how to
deploy, refresh, back up, watch, and roll back. Written for one maintainer
hosting one instance that friends log into. History and design arguments
live in the ADRs and in git; this file is what you would open at a terminal.

## The shape

- **Fly app `sylvan-library`**, region `iad`, live at
  <https://sylvan-libraries.com>. Two machines: the **app** (one `mtglab`
  binary, the Dockerfile's image) and the **forge-worker**
  (`Dockerfile.forge`: the same binary as `mtglab forge-shim` beside a JVM
  and Forge). `fly.toml` is the only Fly-specific file and carries no
  secrets, ever.
- **One volume** (~3 GB), mounted on the app machine at `/data`:
  - `mtg.duckdb` — the card pool. Rebuildable in one command; never backed
    up, never committed.
  - `app.db` — accounts, sessions, tokens, the visitor and Claude ledgers,
    the sim cache. **Irreplaceable.**
  - `/data/decks` — the library. The only standing copy anywhere (ADR 30).
    **Irreplaceable.**
  - `/data/cache` — derived media (card-art motion). Regenerable from the
    dev machine.
- **Secrets travel by environment**: `.env.example` documents the names,
  `fly secrets set` carries them deployed. The binary reads the plain
  environment — there is no dotenv loading.
- One machine and one volume means a deploy stops the instance and starts
  its replacement: **every deploy is a few seconds of downtime.**
- **Stopping is a twenty-second budget the platform has to honour.** `serve`
  gives requests already in flight `shutdownGrace` — 20s — and only then runs
  `door.Close`, which flushes the visitor ledger and hands back `app.db` and
  the card pool. Fly's default `kill_timeout` is **5 seconds**, so until
  2026-08-27 the platform killed the app a quarter of the way into its drain
  and the close work never ran at all: a busy deploy silently lost whatever
  the ledger had counted since its last write. `fly.toml` sets `30s` now, and
  `TestTheStopHasLongerThanItNeeds` parses both numbers and holds them in
  order. It is a ceiling rather than a delay — the app exits the moment it has
  drained — so it does not lengthen an ordinary deploy.

## Deploying

**A push to `main` whose checks are green deploys itself** (ADR 23). The
`deploy` job needs every other job, so nothing red can ship. Expect ~10
minutes after a merge, most of it the `image` job. To redeploy without
merging (after a `fly secrets` change, say): Actions → *tests* → **Run
workflow** — it runs the whole suite first, deliberately.

The deploy is not done when `flyctl` exits: the job then requires the live
instance to answer `/api/health` with 200, `"pool": true` and a non-zero
deck count — a fresh or unmounted volume fails the last two, which are the
ones that matter.

**A failed smoke test does not roll back.** The schema ladder is
forward-only, so an automatic revert can leave things worse than what it
reverted from. Roll back by hand, eyes open:

```bash
fly releases --app sylvan-library
```

```bash
fly deploy --image <image-ref-from-above>
```

**A schema change deserves ceremony.** The ladder applies itself on the
first boot after the deploy, forward-only, with nobody watching. Land a
schema change on its own branch, merge it when you can watch it, and take
an `app.db` backup first (below).

### The deploy token

CI authenticates with the `FLY_API_TOKEN` repository secret — an
app-scoped deploy token, one-year expiry, named so `fly tokens list` means
something. **It expires 2027-08-14**; when it does, merges keep landing and
the deploy job goes red, so `main` and the instance diverge. Rotate with:

```bash
fly tokens create deploy -a sylvan-library -n github-actions-deploy -x 8760h
```

```bash
gh secret set FLY_API_TOKEN --repo aasquier/sylvan-library
```

## Refreshing the pool

Scryfall publishes daily; monthly by hand is plenty. `/api/health` says
when a refresh is *mandatory*: `pool_stale` is true when the volume's pool
predates a column the code now reads, and the affected surfaces degrade to
"we do not know" until somebody runs:

```bash
fly ssh console -C "mtglab data refresh"
```

**Or press the button.** `/admin` → **Upkeep** starts the same refresh as a
background job on the instance, which is ADR 6's own plan for this ("an
authenticated admin endpoint that starts a refresh as a background job") and
runs the same `pool.Refresh` sequence the command above does. It is the right
choice when you are away from a terminal and the wrong one when you want to
watch the phases go by — the page shows five beats and a bar, the terminal
shows the file names and the row counts.

Three differences worth knowing before you press it:

- **The page cannot pass `--oracle-only`.** The button always does the full
  refresh, printings included. Use the command when you want the small one.
- **The app shuts its own library first.** A refresh started from inside the
  serving process cannot open the writer while that process holds the file
  read-only — DuckDB refuses a same-process second open outright rather than
  making it wait — so the job seals the pool, waits up to a minute for
  readers already inside to finish, and hands the file back when it is done.
  Card lookups answer "no pool" for the duration, which is ADR 6's expected
  behaviour said out loud on the page before you commit.
- **One at a time, whoever asks.** A second press, a second tab or a second
  device is refused and pointed at the run already going; a page reloaded
  mid-refresh follows it rather than offering to start another. That guard is
  per process, which is the scope the write lock actually has.
- **The job is the instance's, not the tab's**, and it dies with the process
  rather than with the browser — jobs are in memory by design. Closing the tab
  is safe today because the app machine is held up (`auto_stop_machines` is
  off, `min_machines_running` is 1). **Re-read this bullet if that ever goes
  back to `suspend`**: an idle machine with nobody polling could suspend
  mid-gathering and resume it on the next request, which is a pause rather
  than a loss, but "leave the page" would stop being the whole truth.

**One reader does not raise `pool_stale`, and it is the newest one.** The
deck page's token section reads `oracle_cards.all_parts` — what each card
creates, which is how "what does this deck make" is answered from a chosen
printing rather than by looking a name up. A pool built before that column
cannot answer, and the section says so in its own words rather than claiming
the deck makes nothing. It is deliberately *not* wired into `Stale`: the
frozen refresh corpus records no `all_parts` on any card, so a freshly built
pool would read as stale for a column it had correctly loaded as empty. So
after any deploy that first shipped the column, run the refresh above once,
by hand — nothing else will prompt for it.

Facts about that command worth knowing at the terminal:

- **It is transactional.** The delete and the reload of each table share
  one transaction, so a killed refresh leaves the *old* pool intact, not an
  empty one.
- **It tidies the download shelf.** Once the rows are in, the refresh deletes
  the older dated copies of the kinds it just loaded and says what it took:
  `swept 5 older bulk files (243,142,880 bytes freed)`. Nothing did that
  before, so `/data/scryfall` grew by half a gigabyte a run. It sweeps nothing
  after a refresh that *failed* (the previous copy is the only local rollback
  for rows still in the pool), nothing of the printings after `--oracle-only`
  (that run never read them), and nothing that is not exactly
  `<kind>-<date><suffix>` — a `.part` in flight, a file you decompressed by
  hand, and anything else in that directory are left alone. **A run whose
  download was skipped still sweeps**, which is how a shelf that grew before
  this existed is reclaimed: run the refresh once, and the pile goes whether
  or not Scryfall published today.
- **It is fast now**: ~27 seconds end to end on a dev Mac. On the app
  machine's shared core, budget minutes — and update this sentence with a
  measured number the first time you run one there.
- **It needs the writer's lock.** The app holds a shared DuckDB lease that
  expires within ~10 s of nobody using the pool; health checks do not renew
  it. If the refresh reports the pool busy repeatedly, something is really
  using it.
- **`fly ssh` output lags badly.** Progress is on the volume: a growing
  `.wal` is work in progress; a `.wal` that collapses while the database
  jumps is a phase committing.

  ```bash
  fly ssh console -C "sh -c 'date -u +%T; ls -l /data/mtg.duckdb /data/mtg.duckdb.wal'"
  ```

- **A dead SSH session is not a dead job.** `auto_stop_machines` is
  `suspend`: a machine can suspend mid-command, break the transport, then
  resume the job from memory on the next request. Check `/api/health`
  before concluding anything.
- **Whatever ran over `fly ssh` ran as root.** The entrypoint re-chowns
  `/data` at boot; `fly machine restart <machine-id>` hands the volume
  back immediately.

`mtglab data snapshot` appends today's prices to the price history; the
refresh does not do it for you.

## Backups

Two things on the volume are irreplaceable: **`app.db`** (accounts,
password hashes, email addresses, both ledgers) and **`/data/decks`** (the
library, including rationales friends wrote, which rule 4 says nobody may
regenerate for them). The pool needs no backup — one command rebuilds it.

`app.db`, online and safe while the app serves:

```bash
fly ssh console -C "mtglab data backup /data/app-backup.db"
```

```bash
fly ssh sftp get /data/app-backup.db ./backups/app-$(date +%F).db
```

```bash
fly ssh console -C "rm /data/app-backup.db"
```

The third line is not tidiness: left behind, that file is a second complete
copy of every password hash sitting on the box indefinitely. The command
prints the schema version it copied — the number you want recorded when the
reason for the backup is a migration about to run. It refuses an existing
destination, and never plain-copies a live database (WAL can tear a `cp`).

The decks are plain YAML — no ceremony, just copies:

```bash
fly ssh sftp get /data/decks ./backups/decks-$(date +%F)
```

Keep `backups/` out of the repository; the copies hold password hashes and
email addresses (ADR 16).

**Fly separately snapshots the volume daily** — five-day retention, on a
clock that knows nothing about deploys, so the boot most at risk (a schema
migration after a merge) is the one guaranteed not to have a fresh
snapshot. The restore path (fork a volume from a snapshot, reattach the
machine) has never been exercised here; treat snapshots as a safety net of
unmeasured strength and keep taking the manual backups, which are the ones
with a proven restore.

```bash
fly volumes list --app sylvan-library
fly volumes snapshots list <volume-id>
```

## Decks, the laptop, and derived media

**The volume is the only library** (ADR 30). A laptop checkout keeps no
decks: local work pulls one, treats it as scratch, deletes it after.

```bash
fly ssh sftp get /data/decks/<slug>/deck.yaml ./decks/<slug>/deck.yaml
```

`sftp put` silently overwrites what the instance has — do it only when the
deck page's History tab (ADR 28) says nothing landed there meanwhile.

Card-art motion derivatives (ADR 32) are built on the dev machine by the
media toolbox (`tools/`): `cardmotion sync` sweeps every deck's commander
against the cache and builds what is missing, from the printing the deck
actually shows. Push the finished derivatives up; they arrive root-owned
and the next boot re-chowns them:

```bash
fly ssh sftp put -r data/cache/cardmotion /data/cache/cardmotion
```

Nothing under `/data/cache` needs backing up — a lost cache regenerates
from the same pool, and the deck pages simply show stills until it does.

**One entry is the exception, and it is the only one that cannot be
regenerated:** `cardmotion/grand-coliseum-daynight/` is the Coliseum's banner,
rendered outside the toolbox and pushed by hand. `cardmotion sync` will not
rebuild it; losing it costs the banner, and the room falls back to Critchlow's
still exactly as it did before the loop existed. Keep the source file
somewhere that is not this volume.

```bash
fly ssh sftp put -r data/cache/cardmotion/grand-coliseum-daynight \
  /data/cache/cardmotion/grand-coliseum-daynight
```

## Watching it

```bash
fly logs
fly status
fly machine list
```

Two health paths, and the difference is the point: **`/api/health`** is the
instance's own — pool present, tables counted, deck count, `pool_stale` —
and is what Fly's checks, the image's `HEALTHCHECK` (`mtglab probe`) and
the deploy's smoke test ask. **`/door/health`** answers `{"ok": true}`
whenever the process is up at all: liveness with no opinion about the
stores behind it.

Most of what the three commands answer is on **`/admin`**, signed in as an
admin, no terminal needed: the box's account of itself (process and machine
memory, load, the volume's free space, what every store weighs), the
visitor ledger (requests per day by status class and route template — a
census, never a surveillance), and Claude's ledger (tokens per mode,
honestly labelled a floor on the bill).

The far-seeing glass on `/admin` needs one secret — a read-only platform
token — and stays absent until it has one:

```bash
fly tokens create readonly
```

```bash
fly secrets set FLY_METRICS_TOKEN=<the token>
```

**Alerting lives in Grafana** (Fly auto-provisions it at
<https://fly-metrics.net>), because an alert must fire when nobody is
looking at a page. The two rules worth having: instance memory sustained
above ~85%, and a 5xx rate at the edge above a handful per minute — the
second catches a deploy that booted broken, including a schema migration
that failed on boot.

Memory is the number to watch on the app machine. If OOM kills appear, the
likely causes in order: a large game sweep, Argon2 memory during a login
burst, `data refresh` holding the bulk file. Scale memory before
optimising.

## Accounts

Auth is invite-only; there is no self-signup. The maintainer account is
reconciled at boot from `MTGLAB_ADMIN_EMAIL` (ADR 17), and everything else
— invites, resets, deletions — is the Admin page. `mtglab users` exists for
the same operations at a terminal.
