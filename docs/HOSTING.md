# Hosting sylvan-library

A maintainer's runbook for the deployed instance: the architectural decision
everything rests on, the step-by-step infrastructure setup, and how to deploy,
refresh, back up, watch and roll back what is running.

Written for **you as the sole maintainer**, hosting one instance that friends
log into — not a forked repo per person. That choice is right: a fork gives
every friend a 500 MB Scryfall download, a Python toolchain, and their own
stale copy of the code. One instance means one upgrade path.

**Five sections moved to [`docs/HISTORY.md`](HISTORY.md) on 2026-08-21** —
§§1, 2 and 3 (the auth and isolation design, the data model, the cost
analysis) and §§6 and 7 (the build order and the readiness list for a deploy
day that happened 2026-08-13). They are landed narrative: true, and no longer
instructions. **Their numbers did not change**, so a docstring or an ADR
citing "§1" still resolves — each number below keeps a stub pointing at where
its content went. What stays here is what you would actually open at a
terminal: §0, §4 and §5.

---
## 0. The architectural decision you have to make first

Everything else depends on this, so it goes first.

**The deployed instance holds the library.** `decks/<slug>/deck.yaml` is the
source of truth. It lived in git when this section was first written; ADR 30
made decks **live app data**, and Aaron's 2026-08-21 ruling finished the
thought: the volume at `/data/decks` is the one standing copy, gitignored
everywhere, with the activity log (ADR 28) as the edit record and the build
snapshot as `swaps.md`'s baseline. A laptop checkout keeps no decks — local
work pulls them from the instance and deletes them after. What follows kept
its shape through both changes: there are still two tiers, because friends'
decks still need per-user storage.

### Recommended: two tiers, not one

| Tier | Storage | Who writes it | Keeps |
| --- | --- | --- | --- |
| **Curated decks** (your six) | `deck.yaml` files under `MTGLAB_DECKS_DIR` | You, via CLI or UI | The file-based model: the gate, the five artifacts, `swaps.md` |
| **User decks** | SQLite on the volume, one row per deck | Logged-in users, via UI | Per-user isolation |

Your six live on the volume and everyone can view them — that is the
showcase, and it is what makes the site worth logging into. Users get a
UI-backed deck store.

**Why not give each user a git repo on the volume?** Tempting — it would
preserve `swaps.md` for everyone. But it means running git operations per
request, handling concurrent writes to repos, and a much larger failure
surface, for a feature friends have not asked for. Revisit if they do.

### Prerequisite changes in the app

These are needed before any deployment works at all:

1. ~~**Paths are hardcoded and relative.**~~ **Done.** `config.py` reads
   `MTGLAB_DATA_DIR` and `MTGLAB_DECKS_DIR`, defaulting to `data/` and
   `decks/` so local use is unchanged. The `fly.toml` below sets both, and the
   same change is what made the CLI testable against a scratch directory.
2. **A user/session/deck store.** SQLite, on the volume. Details in §2.
3. **Auth middleware and a user-scoped query layer.** Details in §1.

---


## 1. Login and per-user isolation

**Moved to [`docs/HISTORY.md`](HISTORY.md) §1.** The auth design — Argon2id,
sessions over JWTs, no self-signup, and isolation enforced in one place — was
settled before deploy day and is built. ADR 4, ADR 5, ADR 16 and ADR 17 are
the decisions; `src/mtglab/auth/` and `src/mtglab/api/auth.py` are the code;
CLAUDE.md's auth section is the current summary. Account operations you would
actually run are §4 step 8 and the Admin page in §5.

---

## 2. Data model

**Moved to [`docs/HISTORY.md`](HISTORY.md) §2.** The two-tier store — curated
decks as files, user decks as rows — as it was designed. ADR 30 and Aaron's
2026-08-21 volume ruling moved the file tier onto the volume; §0 above carries
the current shape, and `src/mtglab/auth/db.py` holds the live schema and its
forward-only migration ladder.

---

## 3. Cost and compute — measured, not guessed

**Moved to [`docs/HISTORY.md`](HISTORY.md) §3.** The pre-deploy sizing, the
DuckDB locking rule, and the argument against rewriting the simulator in Rust
or Go. What replaced it operationally: `docs/polish/LEDGER.md` holds measured
baselines, `mtglab bench` takes new ones, and §5 below has the live resource
picture.

---
## 4. Infrastructure setup guide

**Recommendation: Fly.io.** Persistent volumes, scale-to-zero with fast wake,
a single deploy command, and no server to patch. Roughly **$6–8/month**, less
with scale-to-zero. If a simulator later needs real cores, move to a Hetzner
CX22 (2 vCPU / 4 GB, ~€4/mo) — more CPU per euro, at the cost of owning OS
updates and TLS.

**Two things could change this sizing, both open decisions in `ROADMAP.md`.**
Server-side Forge means a JVM plus a card database in the image, which is a
different class of container than anything costed here — **measured on
2026-08-11, and the numbers are in §7 below; the short version is that it does
not fit on the 1 GB instance this section prices.** And a Claude surface
(ADR 14) adds a per-request cost that is *not* CPU — it is somebody's API bill,
and on a shared instance it is the maintainer's. The numbers above cover the
app without either.

On the Claude half there is now at least an estimate. A rationale interview
turn is roughly 12K tokens in and 800 out, and the deck sits in a cached prefix
that reads at a tenth of input price — so interviewing a whole 99-card draft
lands near **$1–1.50 on Sonnet 5** and **$2.50–3 on Opus 5**. That is small
enough not to need a funding model at friends scale, and it is a separate
account from the app's hosting bill either way. **Research is the mode that is
not estimated** — web search and long context are where the cost actually is,
and it is the one plausibly worth gating per user. ADR 15's stance dial doubles
as the control: off means no API calls at all, which is a defensible hosted
default.

### Prerequisites

**Not `brew install flyctl` on this machine.** The only development machine
this project has is macOS 12 on Intel, where Homebrew is too stale to build
anything from source — so the package-manager instruction every Fly guide opens
with is a dead end here, and it is worth saying that in the guide rather than
rediscovering it on deploy day. Fly ships a shell installer that needs no
package manager and no compiler:

```bash
curl -L https://fly.io/install.sh | sh
fly auth login
```

It installs a static binary to `~/.fly/bin` and appends a `PATH` line to your
shell rc — `~/.bashrc` here, which `~/.bash_profile` sources, so it resolves in
a login shell. Verified on macOS 12.7.6 / x86_64 with flyctl v0.4.82: the
binary runs, which was the real risk on an OS this old.

**`flyctl` is not optional, and the browser is not a substitute.** Fly's web
dashboard can create the app, set secrets and deploy — but steps 6 and 8 below
are commands *inside* the machine, and there is no browser path to either. An
instance built entirely from the dashboard has no card pool, no decks, and no way
to give the maintainer a password.

You will also need a credit card on file; the machine sizes below are inside
Fly's paid tier.

### Step 1 — Dockerfile

**Both files now exist in the repository** — [`Dockerfile`](../Dockerfile),
[`docker-entrypoint.sh`](../docker-entrypoint.sh),
[`.dockerignore`](../.dockerignore) and [`fly.toml`](../fly.toml), landed
2026-08-12. They carry their reasoning inline; what follows is what the drafts
this section used to hold got wrong, so the reasoning is not lost with them.

The pool is **not** in the image. It is ~63 MB built from ~98 MB of Scryfall
bulk, Scryfall asks that bulk data not be redistributed, and it belongs on the
volume where it survives deploys. `.dockerignore` keeps `data/` out of the
build context so a local pool cannot reach a layer by accident, and the
`image` job in CI greps the built image for card pool files and fails on a hit —
the tracked-file check is about the repository, this one is about the artifact.
The frontend bundle *is* committed to `src/mtglab/web_dist`, so the image needs
no Node toolchain.

Five things differ from the draft, each for a reason:

- **Two stages, and still no Node.** `docs/ENGINEERING.md` §3 asks both that
  the no-Node property be kept and that the build prove the bundle rebuilds
  from source. Those pull opposite ways, and the second is already satisfied by
  the `frontend` job in CI, which runs the real `npm run build` and fails on any
  diff against the committed bundle. So CI proves the bundle is current and the
  image ships it; the builder stage exists to keep pip and any future compiler
  out of the runtime image, not to touch the frontend.
- **`MTGLAB_DECKS_DIR` is `/data/decks`, not `/app/decks`.** The draft would
  have lost data. `deck.yaml` is the source of truth and every editing route
  writes it, so decks inside the image meant a rationale written in the UI
  vanished at the next deploy — silently, with nothing to notice. See the
  deck-drift note in §5.
- **A non-root app process, reached through an entrypoint.** Fly attaches the
  volume owned by `root:root` and the mount shadows whatever the image had at
  that path, so a `chown` in the Dockerfile is invisible by the time it matters.
  PID 1 starts as root, fixes ownership, and `exec`s the app as `mtglab` via
  `setpriv`. A bare `USER mtglab` would look stricter and leave the app unable
  to write its own volume.
- **A `HEALTHCHECK` on `/api/health`**, using stdlib `urllib` rather than
  adding `curl` — a package installed for one HTTP request is a package to
  patch forever. That path is on `PUBLIC_PATHS`, so it answers with auth on.
- **No `README.md` in the build context.** `pyproject.toml` declares no
  `readme`, so the build backend never reads it.

> **Single worker on purpose — but not for the reason this section used to
> give.** The old note pointed at the DuckDB locking rule, and §3 has since
> corrected itself: read-only handles are safe across processes, so serving on
> two workers would be fine. What actually binds is `api/jobs.py`, whose
> registry is a module-level dict in one process. A sim submitted to worker A
> is invisible to worker B, and `get()` reports what it cannot see as absent —
> which the route turns into a 404 (ADR 5, never a 403). The symptom would be a
> running simulation reported as gone, at random, half the time. Sessions and
> the login rate limiter live in `app.db` and would have been fine.
>
> **That got sharper on 2026-08-13**, when the theme proposal became a job too
> (`api/themeruns.py`). It is a *four-minute* job that costs a real Anthropic
> call, so on two workers the failure would not be a lost simulation somebody
> resubmits for free — it would be a proposal that vanishes halfway through and
> has to be paid for twice. One worker, and the reason is now in two places
> rather than one.

### Step 2 — fly.toml

[`fly.toml`](../fly.toml) is in the repository. Two things to know before the
first deploy:

- **Four `[env]` values are placeholders** and are marked as such in the file:
  `MTGLAB_ADMIN_EMAIL`, `MTGLAB_ADMIN_USERNAME`, `MTGLAB_BASE_URL` and
  `MTGLAB_EMAIL_FROM`. The address ones are placeholders on purpose — this
  repository is public, and an email address is the one piece of personal data
  the project handles. Either edit the file or, to keep the address out of git
  entirely, `fly secrets set MTGLAB_ADMIN_EMAIL=...`; a secret is injected as an
  environment variable and takes precedence over `[env]`. Neither is a
  credential, but the second is a private place to put configuration.
- **`MTGLAB_REQUIRE_AUTH = "1"` is set**, and the local default stays off. One
  person on a laptop does not need a login; a shared instance is what §1 was
  written for.

Nothing secret is in that file and nothing secret may go in it — CI scans every
tracked file's contents for an API key, the committed frontend bundle included.

### Step 3 — create the app and volume

```bash
fly launch --no-deploy --name sylvan-library
```

```bash
fly volumes create mtglab_data --size 3 --region iad
```

3 GB leaves room for the 63 MB pool, the raw Scryfall download during a
refresh, `app.db`, the decks, and backups. It costs about $0.45/month.
`fly.toml` also carries `initial_size = "3gb"`, so a volume Fly creates for you
is the same size as one you create here.

That sizing was only half true until 2026-08-12: `cards.db.download_bulk`
defaulted to a *relative* `data/scryfall` and `cmd_data_refresh` passed nothing,
so the pool went to the volume and the ~98 MB of JSON it is built from went to
the container's working directory — an ephemeral layer, and not the thing this
3 GB was sized for. `config.SCRYFALL_DIR` is derived from `MTGLAB_DATA_DIR` now,
with the same "derived, never set independently" rule as `DB_PATH`.

### Step 4 — secrets

```bash
fly secrets set RESEND_API_KEY="paste-it-here"
fly secrets set ANTHROPIC_API_KEY="paste-it-here"   # four modes exist; see below
```

**If you set them in the dashboard instead, they are `Staged` and not live.**
Found the hard way on 2026-08-13. Fly's web UI stages a secret and waits for an
explicit `fly secrets deploy`; `fly secrets list` says so in a column nobody
reads until something is wrong. Until that runs, the app is up, healthy, and
running on `fly.toml`'s **placeholders** — which means `MTGLAB_ADMIN_EMAIL` is
`you@example.com` and the admin account that exists is called `you`. The
symptom is `mtglab users passwd <your-handle>` reporting no such user, which
points at everything except the cause.

```bash
fly secrets list          # STATUS must read Deployed, not Staged
fly secrets deploy
```

**And `fly secrets deploy` does not pick up `fly.toml` changes.** It restarts
the machine with new secrets against the *image and `[env]` of the last real
`fly deploy`, which is a half-state worth naming: new credentials, old
configuration. On the first deploy that showed up as a correct
`RESEND_API_KEY` next to a placeholder `MTGLAB_EMAIL_FROM` on an unverified
domain — so mail was configured, authorised, and refused by Resend on every
send. Run `fly deploy` after any `fly.toml` edit, and treat a `[env]` change
and a secret change as two different operations.

**There is no `SESSION_SECRET`.** This step used to open with one; the code was
then written and did not need it. Sessions are opaque random tokens stored as
their SHA-256, so there is nothing to sign and no key to hold — one fewer
secret to rotate. §7 records the same correction rather than quietly dropping
the line, because a checklist that loses entries is a checklist nobody trusts.

`RESEND_API_KEY` is the one that is genuinely required: with
`MTGLAB_REQUIRE_AUTH` on, `sender_from_env()` refuses to fall back to the
console sender, because that fallback would print recipients' email addresses
into Fly's logs and ADR 16 forbids it. It is read at call time rather than at
import, so the app still *starts* without it — what fails is the first invite or
password reset, which is to say the first thing you will try to do.

**If a send is refused with a bare `HTTP 403`, suspect the request before you
suspect the account.** `api.resend.com` is behind Cloudflare, and the first
real invite ever sent from this instance (2026-08-13) was refused with 403 and
Cloudflare's error code 1010 — the banned-browser-signature page — because
`ResendSender` was sending Python's default `Python-urllib/3.12` User-Agent.
The domain was verified, the key was valid, and the same request from
`http.client`, which sets no User-Agent at all, was answered 200. `mail.py`
now sends its own agent and a test pins it.

What is worth carrying forward is the diagnostic: **a 403 whose body is not
Resend's JSON did not come from Resend.** Their refusals carry
`{"name": "...", "message": "..."}`; a WAF's do not, so the error message now
says which kind it got. The two want completely different fixes, and an hour
went into the domain and the API key — both healthy — before that distinction
existed.

`MTGLAB_ADMIN_EMAIL` is **not** a secret and belongs in `[env]` — it is an
address, not a credential. (It may still be set with `fly secrets` to keep it
out of a public repository, as step 2 notes; that is a privacy choice, not a
security one.) It is worth guarding either way: whoever can change it is an
admin on the next boot (ADR 17). That is already true of anybody who can run
`fly secrets` or `fly deploy`, so it grants nothing new, and every change the
reconciliation makes is written to the log.

Never put either of the secrets in `fly.toml` or the repo. Fly stores them
encrypted, injects
them as environment variables at runtime, and setting one triggers a redeploy —
so they are never in the image either. Rotating `SESSION_SECRET` logs everyone
out, which is the correct emergency response to a suspected session leak.

`ANTHROPIC_API_KEY` is read by the Anthropic SDK directly; the app never binds
it to a variable, and asks only whether it is set. Locally the same variable
comes from a gitignored `.env` (see `.env.example`) rather than from Fly.

**The key alone is not enough, and the failure is silent.** The image must also
carry the SDK, which is the `claude` extra. On the first deployment it did not:
the Dockerfile installed `.[api]` on the then-true grounds that no ADR 15 mode
was built, so the instance had the secret set and nothing that could read it.
Nothing looked broken from outside — the app was healthy, the UI rendered the
dossier and theme-interview controls, and every one of them was a 503.
`mtglab claude check` on the machine is what says so:

```bash
fly ssh console -C "mtglab claude check"     # status: available
```

`tests/test_packaging.py` now pins the image's extras against the surfaces that
need them, because no other test can see this: they all stub the SDK, so they
pass whether or not it is installed.

#### Why a static key here, and when to stop using one

Anthropic offers three authentication methods, and their documented fit is
worth quoting rather than guessing at, because the answer for this app is not
the answer for every app:

| Method | Documented best for |
| --- | --- |
| **API key** | "Local development, prototyping, scripts, and **single-tenant servers where you control secret storage**" |
| **Workload Identity Federation** | "Production workloads on cloud platforms (AWS, Google Cloud, Azure), CI/CD pipelines, and Kubernetes, **where you want to eliminate static secrets**" |
| App Attest | iOS/macOS apps calling the API directly with no backend — not us |

A single-tenant Fly app with secrets in `fly secrets` is the first row
verbatim, so the key is the right instrument here and not a shortcut. The
guidance for moving is conditional rather than aspirational: adopt federation
"when your workload already has a platform-issued identity you can federate."

**WIF exchanges an OIDC JWT from an identity provider you already trust for a
short-lived token the SDK refreshes itself — there is no `sk-ant-api...` string
to mint, distribute, or rotate.** Setup is three Console resources (a service
account, a federation issuer, and a federation rule) plus an IdP that issues
OIDC tokens; the named ones are AWS IAM, Google Cloud, Azure/Entra, Kubernetes
service accounts, GitHub Actions, SPIFFE, and Okta. Two consequences for us:

- **Fly is not on that list.** Whether Fly's machine OIDC tokens can back a
  federation rule is a question for Fly's own docs before betting on it, and
  the Hetzner alternative in §3 has no platform identity at all. Until one of
  those resolves, a key in `fly secrets` is the endpoint, not a waypoint.
- **CI is the better first candidate.** GitHub Actions is an OIDC issuer
  Anthropic supports, and the reviewer workflow in `docs/ENGINEERING.md`
  already requests `id-token: write`. That is the place where a static
  repository secret could actually be removed.

Federation is not a free upgrade either — Anthropic's own caveat is that it
"does not, on its own, guarantee end-to-end security: the trust chain is only
as strong as your identity provider's configuration, and a long-lived secret
one hop upstream ... can still undermine it."

#### Key expiration, chosen once

A key's expiration is set **at creation and cannot be changed afterwards** —
3 hours, 1 day, 7 days, 30 days, a custom duration, or **Never**. "Never" is
the documented choice "for keys you store in a secrets manager and rotate
yourself", which is what `fly secrets` is. A short-lived key is the better
choice while the key lives mainly on a laptop, where the blast radius of a leak
is a stolen file rather than a breached host.

Anthropic emails the key's creator before expiry — 7 days ahead for a key with
a lifetime of at least 14 days — and an **expired key returns `401` with no way
to reactivate it**. Rotating means creating a new key and replacing it
everywhere it lives.

**This project runs a single environment**, so there is one key rather than one
per stage. That is the right call at this size — two keys is two things to keep
in sync for a benefit that only appears once other people have accounts — but
it has two consequences worth stating plainly:

- Once deployed, one key lives in **two places**: the gitignored `.env` on the
  maintainer's machine and `fly secrets` on the host. A rotation is not done
  until both are updated, and the second one is the easy one to forget.
- There is no staging key to fail first. The expiry date is therefore an
  operational date, not a background detail, and a short-lived key wants either
  a calendar reminder or a switch to **Never** once `fly secrets` is holding it.

**Make the failure legible in code.** When the Claude surface lands, a `401`
from the API should say *the key was rejected and may have expired* rather than
surfacing as a generic error — that message is worth writing on the day the
integration is built, because it will be read a month later by someone who has
forgotten the key had a lifetime at all.

Three ways this leaks that are worth naming, because two of them are specific
to this app:

- **Never with a `VITE_` prefix, and never in `web/.env`.** Vite bakes those
  into the bundle, and `src/mtglab/web_dist/` is committed to a public
  repository *and* served to every browser. Every Claude call goes through
  FastAPI; the frontend must never hold the key. CI scans the committed bundle
  for exactly this.
- **Never in a prompt or a message.** Session history persists, so a key placed
  there is durably stored and readable back for the life of the session.
- **A spend limit on the API workspace is the backstop**, because storage
  hygiene eventually fails and a cap bounds what that costs. Rotating the key
  is a console click.

### Step 5 — first deploy

```bash
fly deploy
```

This is the **first** deploy, and the manual path generally. Routine deploys
are automatic as of 2026-08-14: a push to `main` whose `ci.yml` checks are green
deploys itself, and there is a manual button in the Actions tab that runs the
whole workflow and then deploys. See
[ADR 23](adr/0023-a-green-main-deploys-itself.md) and §5 below for the runbook.
`fly deploy` from a laptop still works and is the rollback path.

The app will start with **no card pool and no decks**. Both are expected, and both
are fixed by step 6 — `/api/health` reports pool state rather than crashing,
which is exactly the fresh-clone case the API tests already cover, and an empty
`MTGLAB_DECKS_DIR` yields an empty library rather than an error. The CI `image`
job pins that: it starts this image with an empty volume and requires
`/api/health` to answer 200 with `"pool": false`.

You should be able to sign in at this point, before seeding anything.

### Step 6 — seed the volume

Two things live on the volume and neither arrives on its own. **This is a
documented run, not a build step and not a boot step** — the pool half needs
several minutes and a ~500 MB download, and with scale-to-zero putting boot on
the request path, doing it at startup would turn a visit into an outage.

**The decks**, first, because it is instant. The image carries none (ADR 30:
decks are live app data, not repository content), so a fresh instance's
library fills the way the pool does — from outside. Either restore a backup
pulled off a previous instance:

```bash
fly ssh sftp put ./backups/decks-<date>/<slug>/deck.yaml /data/decks/<slug>/deck.yaml
```

or push your local working decks up, one `deck.yaml` per deck, or simply
import through the app once you can sign in. A brand-new instance with zero
decks is a legitimate state, not a broken one.

Files put over sftp arrive owned by root, so hand them back afterwards — the
entrypoint does this at every boot, and a restart would fix it, but not
before the first write fails:

```bash
fly ssh console -C "chown -R mtglab:mtglab /data"
```

**The pool**, second, and this is the slow one:

```bash
fly ssh console -C "mtglab data refresh"
```

Both halves of that download land on the volume, which was not true before
2026-08-12 — see step 3. If the machine's connection or your shell drops
part-way, re-run it: `download_bulk` writes to a `.part` file and renames only
on completion, so an interrupted download is never mistaken for a finished one.

Verify, and note this checks both halves at once — `validate` needs the pool
to check card facts and a deck to have something to check (use whichever slug
you restored or imported):

```bash
fly ssh console -C "mtglab decks validate gyome-food"
```

On the maintainer's own library, expect **Goreclaw and Atla Palani to fail
the gate on one banned card each**. That is a known, deliberate state
recorded in CLAUDE.md, not a bad deploy.

#### Reading the mail DNS, and the mistake worth not repeating

Resend's records span three names and it is easy to declare one missing by
querying the wrong one. For a sending domain of `send.sylvan-libraries.com`:

| Name | Record | What it is |
| --- | --- | --- |
| `send.sylvan-libraries.com` | `MX inbound-smtp...` | receiving |
| `send.sylvan-libraries.com` | DKIM at `resend._domainkey.` | signs outbound mail |
| `send.send.sylvan-libraries.com` | `TXT v=spf1 include:amazonses.com` | the custom MAIL FROM |
| `send.send.sylvan-libraries.com` | `MX feedback-smtp...` | bounces |

**SPF is evaluated against the envelope sender, not the `From` header.** SES
sets a custom MAIL FROM on a further subdomain, so the SPF record correctly
lives at `send.send.` and its absence from `send.` is not a gap. On
2026-08-13 this was reported as a missing SPF record on exactly that reasoning;
the tell that it was wrong was already in the output and read past —
`inbound-smtp` is a *receiving* endpoint, so the sending records were always
going to be somewhere else.

Both alignments hold: DKIM matches the `From` domain exactly, and SPF matches
under relaxed alignment through the shared organisational domain.

**Porkbun ships two ALIAS records on a new domain** — one at the apex and a
`*` wildcard — both pointing at a parking page. Both have to go: the apex one
conflicts with the `A` record outright, and the wildcard otherwise answers for
every name that has no record of its own, including the `_acme-challenge` and
`_fly-ownership` names Fly falls back to. Deleting them does not disturb the
mail records, because a wildcard never synthesises for a name that exists —
verified before and after on this instance rather than assumed.

### Step 7 — domain and TLS

The domain is `sylvan-libraries.com`, registered 2026-08-13, and the app
answers on the **root**:

```bash
fly certs add sylvan-libraries.com
```

Add the records Fly prints, then confirm:

```bash
fly certs show sylvan-libraries.com
```

An apex is A/AAAA rather than a CNAME, which Fly prints for you — there is no
extra work, but it is the one place the instructions differ from a subdomain.
`force_https` in `fly.toml` handles the redirect. TLS is automatic and free.

**The `A` record points at Fly's *shared* IPv4 and that is correct.** It looks
wrong next to a dedicated IPv6 and it is not: Fly routes shared-IPv4 traffic by
the hostname in the TLS handshake, so a dedicated IPv4 buys nothing here and
costs ~$2/month. Validation runs against the `AAAA` — Fly needs one of an AAAA
pointing at the app, an `_acme-challenge` CNAME, or a `_fly-ownership` TXT, and
the dedicated IPv6 satisfies the first without any extra record.

**Done 2026-08-13.** Issued by Let's Encrypt about three minutes after the
records landed; `https://sylvan-libraries.com` serves the app, plain HTTP 301s
to it, and `sylvan-library.fly.dev` keeps working alongside.

**`send.sylvan-libraries.com` is the mail subdomain and is not this.** It is
verified with Resend and carries the sending records; nothing is served from
it. `MTGLAB_EMAIL_FROM` is on `send.`, `MTGLAB_BASE_URL` is the root, and
mixing them up produces mail Resend refuses or links that go nowhere.

### Step 8 — create your account

**Usually you do not.** `MTGLAB_ADMIN_EMAIL` from step 4 has already created it:
the app reconciles that address to an enabled admin every time it starts
(ADR 17), so a deployed instance comes up with your account in place and no
password on it. Claim it from the sign-in page's "forgot password" link — an
unclaimed account gets a reset link deliberately — and you are in without ever
opening a shell.

The shell path is the break-glass one, for a misconfigured mail provider or a
`MTGLAB_ADMIN_EMAIL` you got wrong. **It has to be an interactive console**,
and this is the correction worth reading before you need it: the password is
read with `getpass`, which needs a TTY, so `fly ssh console -C "..."` — which
runs one command with no terminal attached — cannot work. Open the shell
first:

```bash
fly ssh console
```

then, **before anything else**, put the venv on `PATH`:

```bash
export PATH="/opt/venv/bin:$PATH"
```

This is the second half of the same correction and it is the combination that
actually bites. The image sets `ENV PATH="/opt/venv/bin:$PATH"`, and
`fly ssh console -C "mtglab ..."` inherits it — every command in step 6 works
unmodified. An *interactive* console does not: it starts a login shell, and
Debian's `/etc/profile` overwrites `PATH` for root. So the one form that needs
a TTY is the one form where `mtglab` is not found, and the error says
`command not found` rather than anything about `PATH`. `/opt/venv/bin/mtglab`
in full works just as well for a single command.

Then, whichever of these applies:

```bash
mtglab users passwd gyome                       # the bootstrapped account exists
mtglab users add aaron --email you@example.com --admin   # it does not
```

Running as root is fine here, and deliberately so: `docker-entrypoint.sh`
does a recursive `chown` of the volume at every boot *because* this step and
step 6 both arrive over `fly ssh console` as root. Anything you leave
root-owned is repaired at the next restart.

`users passwd` is usually the one you want. `MTGLAB_ADMIN_EMAIL` has already
created the account and left it unclaimed, so there is nothing to *add* — what
is missing is a password, and that is the command that sets one. It ends every
session on that account as a side effect, which is the right behaviour for a
credential reset.

Do this from a terminal you trust. Both prompt twice; there is no way to pass a
password as an argument, because command-line arguments land in shell history
and in the process table.

**`RESEND_API_KEY` is not optional for the email path**, and it is worth being
explicit because the fallback people expect is deliberately absent: locally, a
missing key makes `mtglab users invite` print the link it would have sent, but
with `MTGLAB_REQUIRE_AUTH` on that fallback is *refused* rather than used —
printing recipients into Fly's logs is what ADR 16 forbids. So on a deployed
instance there is no "just read the link off the console" escape hatch. Either
Resend works, or you use the shell.

**Clean up the placeholder account if one was created.** If the instance ever
booted with `MTGLAB_ADMIN_EMAIL` unset or still reading `you@example.com`, an
admin account exists for it. Disabling is the right lever here and `delete`
would be the wrong one: the placeholder holds no address anybody wants back,
and a row that stays is a record of what the bootstrap did. Do it only *after*
your real account has a password, or the last-admin guard refuses:

```bash
mtglab users disable you
```

(`mtglab users delete` does exist, and sessions and tokens are not the obstacle
they once looked like — both cascade. It is for releasing a `username` or an
`email` so it can be invited again, which is the one thing disabling cannot do.)

Everyone else gets an invite rather than an account you made a password for
(ADR 16) — `mtglab users invite <email>`, or the Admin page once you are in.
Note that neither the CLI nor that page will demote or disable the last admin
who can sign in; to hand the instance over, promote the successor first.

**If somebody says the link "didn't work" and dropped them on the sign-in
screen, this is almost certainly it: their mail app cut the link short.** The
token travels in the URL fragment so it stays out of every access log, and the
cost of that is that some clients drop it when you click — the address they can
*see* is whole, and the one the browser opens is not. Nothing about it reaches
the server, so there is no log line to find, and re-sending does not help
because the next link fails identically.

The claim screen takes a pasted address for exactly this, and both messages say
so. Tell them to copy the whole address out of the email — including the part
after the `#` — and paste it into the box on that page. Their link is
untouched: a stripped click spends nothing.

---

## 5. Running it

### Deploying

**A push to `main` whose `ci.yml` checks are green deploys itself**, since
2026-08-14 — [ADR 23](adr/0023-a-green-main-deploys-itself.md). The `deploy`
job in `ci.yml` `needs` every other job, so it cannot start unless they passed,
and it runs only for a push to `main` or an explicit `workflow_dispatch`.
Expect it about ten minutes after a merge; most of that is the `image` job.

**To deploy without merging anything** — a redeploy after a `fly secrets`
change, say — use the manual button: Actions → *tests* → **Run workflow**. It
runs the whole suite and then deploys, deliberately: a button that skips the
suite is a button that eventually ships something red.

#### The deploy token

CI authenticates with a `FLY_API_TOKEN` repository secret. Fly is opinionated
about this and worth following: use **the token with the narrowest access that
will work**, which for deploying one app is an app-scoped deploy token rather
than the org-wide auth token.

```bash
fly tokens create deploy -a sylvan-library -n github-actions-deploy -x 8760h
```

```bash
gh secret set FLY_API_TOKEN --repo aasquier/sylvan-library
```

Both flags matter and neither is the default:

- **`-x 8760h` — one year.** `fly tokens create deploy` issues a **20-year**
  token when you omit this (175200h), and flyctl's own help recommends against
  it. A credential that outlives the project is how one ends up live in an old
  fork.
- **`-n github-actions-deploy`** — the default name is `flyctl deploy token`,
  which tells you nothing in `fly tokens list` once there is more than one.

**This token expires 2027-08-14.** When it does, merges will keep landing and
the deploy job will go red, which means `main` and the instance diverge — the
exact state [ADR 23](adr/0023-a-green-main-deploys-itself.md) exists to
prevent, just with a red check beside it. The deploy job prints the rotation
command on failure for that reason.

There is no way to avoid a stored secret here: Fly's OIDC support is
**outbound** — Machines authenticating to AWS, GCP and so on — and there is no
inbound trust from GitHub Actions to Fly. Checked 2026-08-14.

```bash
fly tokens list                            # what exists, and when it lapses
fly tokens revoke <id>                     # after rotating
```

**The deploy is not done when `flyctl` exits.** The job then requires the live
instance to answer `/api/health` with 200, `"pool": true` and a non-zero deck
count. The last two are the ones worth having — a fresh or unmounted volume
answers `"pool": false`, and a deploy that lost the volume loses deck edits
that exist in no repository.

**A failed smoke test does not roll back.** It fails loudly and prints
`fly releases`. That is deliberate: the schema ladder is forward-only, so an
automatic revert can leave things worse than the state it reverted from, and
an unmounted volume is not fixed by redeploying the previous image.

To roll back by hand:

```bash
fly releases --app sylvan-library          # find the last good version
fly deploy --image <image-ref-from-above>  # or: fly releases rollback
```

**A schema change deserves the treatment every deploy used to get.** Migrations
in `auth/db.py` run on boot and do not run backwards, so rolling the code back
does not roll the schema back. Land a schema change on its own branch, merge
it when you can watch it, and take a backup first (see *Backups* below).

One machine and one volume means Fly cannot roll — a deploy stops the instance
and starts the replacement, so every deploy is a few seconds of downtime. That
is inherent to the shape rather than something automation introduced; it simply
happens more often now.

### Refreshing the pool

Scryfall publishes daily; deck tooling does not need day-fresh data. Monthly is
plenty unless you are watching prices.

**A pool schema change makes this mandatory, and `/api/health` says so.**
`pool_stale` is true when the volume's pool predates a column the code now
reads, and the app degrades to "we do not know" rather than to a wrong answer —
so the instance keeps serving, with the affected field simply absent, until
somebody runs the command below. Two have shipped: the printed stats
(2026-08-16) and the **painter and flavour text on `printings`**
(2026-08-19, so a deck showing a chosen printing renders no credit line until
the refresh, where before it rendered the wrong one). Nothing else is affected
and no deploy is needed afterwards.

**Budget half an hour and do not interrupt it.** Measured on the maintainer's
Mac, 2026-08-19: **~28 minutes**, of which ~9 are the two downloads and ~16 are
`load_printings` alone, at roughly 110 rows a second for 107,355 rows. That is
a known inefficiency, profiled and queued in `docs/polish/LEDGER.md` under
Black, not a symptom of the machine. The reason not to interrupt is separate
and sharper: **`load_printings` empties the table before it fills it**, so a
killed refresh leaves the pool with no printings at all — every deck page
showing default art, the art picker offering nothing, and no error to say why.
Re-running it is the only fix. On the instance this compounds with the timings
below, so scale up first.

```bash
fly ssh console -C "mtglab data refresh"
```

**That line could not work at all until 2026-08-19, and the reason is worth
the paragraph.** `data refresh` opens the pool read-write and needs DuckDB's
**exclusive** lock. The app holds a *shared* one through the pool keeper
(`service._KEEPER`), which is a lease meant to expire after `_KEEPER_IDLE` of
nobody wanting the pool -- and `_KEEPER_IDLE` was 30.0 while the
`[[http_service.checks]]` block above calls `/api/health` every **30s**, an
endpoint that counts both tables and asks `pool_stale`. The lease was renewed
by the one caller that never stops asking, exactly as often as it came due, so
the refresh was refused **forty times over five minutes**, always at `connect`,
always by the same holder. The lease is 10.0s now and a test derives that
ceiling from `fly.toml`; step 6 works on a populated volume for the first time.

It read as working before only because the one run that succeeded --
2026-08-13, the measurements below -- was the **first** load, when there was
no pool file to hold a lock on. A runbook step verified once on an empty
volume is a runbook step verified in the state you will never be in again.

**One line, and it does not fit on the machine that runs the app.** Measured on
the first real run, 2026-08-13. The job is five phases and only the first
prints before it starts:

1. download `oracle_cards` (~24 MB gzipped)
2. `load_oracle` — 35,390 rows, **silent until it finishes**
3. print `loaded N oracle cards`
4. download `default_cards` — several times larger
5. `load_printings` — 107,338 rows, the longest phase

On `shared-cpu-1x` phase 2 alone took **over thirty minutes** and phase 5 is
three times the rows. So scale up for the run and back down after:

```bash
fly scale vm performance-1x                       # ~7x faster, measured
fly ssh console -C "mtglab data refresh"
fly scale vm shared-cpu-1x --vm-memory 1024       # or let the next deploy do it
```

**`performance-1x` rather than `shared-cpu-4x`, despite a quarter the cores.**
The load is a single-threaded Python loop — `json.loads` per line, then batched
`executemany` — so more cores buy nothing and an unthrottled one buys
everything. Measured on the volume: ~0.65 MB/min of WAL growth on the shared
slice against ~4.5 MB/min on the dedicated core.

`fly.toml` pins `shared-cpu-1x` in `[[vm]]`, so **the next `fly deploy` scales
you back down whether or not you remember to.** That is a safety net, and it is
also why a permanently larger machine has to be a change to the file rather
than a `fly scale` command.

#### Watch the filesystem, not stdout

Two things about a long job over `fly ssh console` that cost an hour of
confusion on the first run, and that generalise to every command in step 6:

**The output lags badly.** `fly ssh` delivers stdout in chunks, so the terminal
sat on `downloading oracle_cards ...` while the work was demonstrably three
phases further on. Progress is on the volume:

```bash
fly ssh console -C "sh -c 'date -u +%T; ls -l /data/mtg.duckdb /data/mtg.duckdb.wal'"
```

A growing `.wal` is work in progress; a `.wal` that collapses while the
database jumps is a commit landing — that is how you see a phase boundary go
by without a line being printed.

**And the session dying does not mean the job died.** `auto_stop_machines` is
`suspend`, which snapshots memory rather than killing processes: when the
machine suspended mid-refresh, the SSH session broke with
`remote command exited without exit status or exit signal` — a message about
the transport that reads like a message about the job — and the refresh
*resumed from memory* on the next start and ran to completion headless. The
apparent failure was reported, believed, and wrong. **Check
`/api/health` before concluding anything about a job that appeared to die.**

**Whatever ran over `fly ssh` ran as root**, so hand the volume back
afterwards. A restart does it, since that is what the entrypoint's
`chown -R` is for:

```bash
fly machine restart <machine-id>
```

**Why there is no cron here.** Fly volumes attach to exactly one machine, so a
scheduled second machine cannot mount the same volume — the obvious approach
does not work. Your options, in order of how much I would recommend them:

1. **Run it by hand monthly.** One command, zero code, and you will notice if
   it breaks. At this cadence this is genuinely the right answer.
2. An authenticated admin endpoint that kicks off a refresh as a background
   job, called by GitHub Actions on a schedule.
3. An in-process timer thread in the app, guarded by a lock file.

Start with (1). Only build (2) if you find yourself forgetting.

### Decks on the volume, and the laptop

This section used to be about keeping `/data/decks` in step with a copy
tracked in git, and it opened with the admission that neither was
automatically authoritative. ADR 30 resolved that by removal: decks are live
app data, git holds none, and **each instance's `MTGLAB_DECKS_DIR` is the
only copy that instance has.** The laptop's `decks/` and the volume's
`/data/decks` are two different libraries that happen to share some slugs,
the same way two laptops would be. Copy between them deliberately, in
whichever direction you mean:

```bash
fly ssh sftp get /data/decks/<slug>/deck.yaml ./decks/<slug>/deck.yaml
```

`sftp put` silently overwrites whatever was written on the instance, so do it
only when you know nothing was — the History tab (ADR 28) is how you know.

### Card-art motion derivatives (ADR 32)

Generated on the dev machine (`mtglab cardmotion build --deck <slug>
--effect depth-drift`), never in git and never in the image; the instance
only serves what sits in its cache. `mtglab cardmotion sync` is the sweep
form: every deck's commander checked against the cache, and whatever is
missing built from the printing that deck actually shows — which is how an
imported deck's commander first breathes, and how a swapped art choice gets
its new painting derived (the serving tier matches on the art, so until the
sync runs the page shows the correct still rather than the old loop). Push
a finished derivative up the same way everything else reaches the volume:

```bash
fly ssh sftp put -r data/cache/cardmotion /data/cache/cardmotion
```

Files arrive root-owned; the entrypoint re-chowns `/data` at the next boot,
or run the chown line from §4 step 6 to fix it immediately. Nothing here is
irreplaceable — a lost cache regenerates from the same seeds and pool — so
`backups/` need not carry it; the deck pages simply show stills until the
push is redone, which is the app as it was before the tier existed.

### Backups

The pool needs no backup — `data refresh` rebuilds it in one command. That is
the whole reason it is gitignored.

Two things on the volume *are* irreplaceable. **`app.db`** holds users,
sessions and password hashes. **`/data/decks`** holds the instance's whole
library — since ADR 30 there is no git copy behind it — including the
rationales your friends wrote, which by rule 4 nobody may regenerate on
their behalf.

Back `app.db` up with SQLite's online backup, which is safe to run against a
live database. **There is no `sqlite3` binary in the image** — the same class of
absence as `curl`, and for the same reason: nothing in the runtime needs one.
Python's `sqlite3` module has the identical online-backup API and is present by
definition, so that is what the procedure uses:

```bash
fly ssh console -C "python3 -c \"import sqlite3; s = sqlite3.connect('/data/app.db'); d = sqlite3.connect('/data/app-backup.db'); s.backup(d); d.close(); s.close()\""
fly ssh sftp get /data/app-backup.db ./backups/app-$(date +%F).db
fly ssh console -C "rm /data/app-backup.db"
```

`python3 -m sqlite3` looks like a drop-in for the missing binary and is not
one: Python 3.12 does ship a `sqlite3` CLI, but it only executes SQL. It has no
`.backup` dot-command and answers `near ".": syntax error`.

**The third line is not tidiness.** Left behind, `/data/app-backup.db` is a
second complete copy of every password hash and every email address, sitting on
the volume indefinitely. Take the backup, pull it down, remove it.

`fly ssh sftp get` works. It is `put` that Fly's permission classifier refuses
— worth knowing when you want a script *on* the machine, where inline
`python3 -c` is the way in.

Do not simply `cp` a live SQLite file — with WAL enabled you can capture a torn
copy. Keep these backups private: they contain password hashes **and email
addresses**, which is the same reason `app.db` is gitignored (ADR 16). A
backup directory that ends up in git is the leak this whole rule exists to
prevent, so keep `backups/` out of the repository.

This procedure was executed against the live instance on 2026-08-13 and
verified end to end: online backup, `integrity_check: ok`, pulled down with
`sftp get`, restored into a scratch `MTGLAB_DATA_DIR`, and opened by the app's
own `auth/db.connect()` — with `foreign_keys` on, so ADR 16's
`ON DELETE CASCADE` survives a restore.

**Take one before a deploy that carries a schema migration.** `app.db` migrates
itself on the first connection after a deploy, which is usually invisible and
occasionally not: schema version 5 *rebuilds the `users` table* to add
`AUTOINCREMENT`, because SQLite cannot `ALTER` a column into it. The rebuild is
guarded — `foreign_keys` off around the ladder, `PRAGMA foreign_key_check`
before it is given back, and a refusal to serve from a file that fails it — but
"the migration is careful" and "there is a copy of the file from before it ran"
are not the same assurance, and only one of them is yours. `PRAGMA user_version`
tells you where a file is:

```bash
fly ssh console -C "python3 -c \"import sqlite3; print(sqlite3.connect('/data/app.db').execute('PRAGMA user_version').fetchone()[0])\""
```

The decks need no such ceremony — they are plain YAML — but they do need
copying, and since ADR 30 this pull is the instance's whole recovery story
alongside Fly's own snapshots:

```bash
fly ssh sftp get /data/decks ./backups/decks-$(date +%F)
```

#### The snapshots Fly takes on its own, and what is not yet known about them

Everything above is a backup *you* run. Fly is separately snapshotting the
volume daily without being asked, and this runbook said nothing about them
until 2026-08-16, which is the kind of silence that reads as "there are none"
at exactly the wrong moment. Observed that day:

```bash
fly volumes list --app sylvan-library
fly volumes snapshots list vol_vwnqxewn1y00oy9v
```

Four snapshots, one a day, the newest six hours old, **five-day retention**,
286 MiB stored against a 3 GB volume. Five days is Fly's default and it is the
number to know: a corruption nobody notices within five days is a corruption
with no snapshot behind it, and `app.db` is the file where that matters,
because a bad row can sit unnoticed far longer than a missing one.

**A deploy does not take one, and the timing is exactly backwards.** Four
deploys landed on 2026-08-16 — machine v61 through v65 — and the newest
snapshot still predated all four, because Fly snapshots on a daily clock that
knows nothing about when the volume is at risk. The moment it is *most* at
risk is the boot after a merge: ADR 23 means merging deploys, and `auth/db.py`
migrates forward-only on the first connection afterwards with nobody watching.
So the deploy that would most want a rollback point is the one guaranteed not
to have a fresh one. That is what the manual `app.db` backup above is for, and
why "take one before a deploy that carries a schema migration" is written as an
instruction rather than left to the schedule.

**The restore path has never been exercised, and this file will not pretend
otherwise.** Fly restores a snapshot by creating a *new* volume from it
(`fly volume fork` / `fly volumes create --snapshot-id`) rather than by
rewinding the one in place, so a real restore also means detaching the running
machine from the current volume and attaching the new one — which is downtime,
a machine edit, and a step nobody has walked here. Contrast the `app.db`
procedure above, which was executed end to end on 2026-08-13 and is written
down because it was. Until somebody does the same for a snapshot, treat these
as a safety net of unmeasured strength and keep taking the manual `app.db`
backup, which is the one with a proven restore.

The two are not redundant, either: the snapshot holds the whole volume
including the pool, and the manual backup holds the two irreplaceable things at
a moment you chose — which is the one you want before a schema migration.

### Watching it

```bash
fly logs
fly status
fly machine list
```

Memory is the number to watch. If you see OOM kills, the likely causes in order
are: a 25,000-game sweep, Argon2 memory during a login burst, or `data refresh`
holding the bulk file. Bump to 2 GB (~$11/mo) before you start optimising.

### The Admin page, and what it can tell you without SSH

Most of what those three commands answer is on **`/admin`** now, signed in as
an admin, and none of it needs a terminal or a token:

- **the box's own account of itself** — process memory, machine memory, load,
  the volume's free space, and what every store on it weighs (pool, `app.db`,
  the caches, the decks);
- **the visitor ledger** (schema v9) — requests per day by status class and the
  most-asked-for **route templates**. Never a concrete path, an address or an
  agent; it is a census, not surveillance;
- **Claude's ledger** — tokens per mode, honestly labelled a floor on the bill.

**The far-seeing glass is the one part that needs a secret**, and it stays
absent until it has one. Mint a read-only token and set it:

```bash
fly tokens create readonly
```

```bash
fly secrets set FLY_METRICS_TOKEN=<the token>
```

That switches on the platform's own view — instance memory as Fly accounts for
it, and **edge** responses by status class over 24 hours. The edge counts what
reached the platform; the visitor ledger counts what the app answered, and the
gap between the two is the requests the app never saw (a proxy refusal, a TLS
failure, an outage). Answers are cached five minutes, and a token that stops
working clouds the panel rather than breaking the page.

**Alerting lives in Grafana, not in this app.** Fly auto-provisions managed
Grafana per organisation at <https://fly-metrics.net>, which is where alert
rules belong — it can notify when nobody is looking at a dashboard, which is
the entire point of an alert and something a page cannot do. The two rules
worth having first:

- **instance memory above ~85% of the machine's total**, sustained a few
  minutes — the OOM warning the paragraph above describes, arriving before the
  kill rather than after;
- **a 5xx rate above a handful per minute at the edge**, which catches a
  deploy that booted broken (including a schema migration that failed on boot
  — those apply unwatched, see §5's deploy notes).

---


## 6. Build order

**Moved to [`docs/HISTORY.md`](HISTORY.md) §6.** The order the pre-deploy work
was done in, every step now struck through. Cited by ADR 5, ADR 13 and ADR 23,
and by `config.py`, `auth/__init__.py` and `api/deps.py`, all of which point at
step 5 or step 6; both resolve in HISTORY at the same numbers.

---

## 7. Deployment readiness — the running list

**Moved to [`docs/HISTORY.md`](HISTORY.md) §7.** The checklist for deploy day,
2026-08-13, plus the Forge worker provisioning notes ADR 35 left in it. Cited
by ADR 16, ADR 17, ADR 32, `docs/FORGE.md` and `docs/ENGINEERING.md`. **The
live equivalent is `docs/polish/LEDGER.md`**, which is re-checked against the
tree rather than carried forward — a readiness list for a day that has passed
cannot be the open-work list, which is most of why this file was split.

---
