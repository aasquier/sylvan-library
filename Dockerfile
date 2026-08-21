# sylvan-library, containerised. `docs/HOSTING.md` §4 is the deployment guide;
# this file is the thing that guide used to only describe.
#
# Two stages, and deliberately **no Node stage**. `src/mtglab/web_dist/` is
# committed precisely so the image needs no Node toolchain, and
# `docs/ENGINEERING.md` §3 asks for that property to be kept. It also asks that
# the build prove the bundle can be rebuilt from source — which the `frontend`
# job in `.github/workflows/ci.yml` already does, and does better: it runs the
# real `npm run build` and fails on any diff against the committed bundle, on
# every pull request. Rebuilding it here as well would be a slower duplicate of
# a gate that already exists, bought by making the image depend on Node and the
# npm registry. So the split is: CI proves the bundle is current, the image
# ships it.
#
# What the builder stage buys instead is that pip, its build backend and any
# compiler a dependency might one day need stay out of the runtime image.
# Nothing compiles today — every dependency publishes manylinux wheels — but
# the day one stops, it compiles over there rather than here.

# ------------------------------------------------------------------- door
#
# The Go front door (ADR 38; docs/go-migration/PLAN.md section 4). From
# Phase 2 of the migration the process the container runs is this binary:
# it takes :8080, refuses before routing what `api/auth.py` refuses, serves
# the bundle and the tarot art itself, proxies everything under /api to the
# Python server on loopback, and supervises that server as a child. Three
# stages now, and the image is larger for the duration -- the plan says so
# out loud so the interim is never read as the outcome.
#
# `golang:1.26-trixie`: 1.26 because go.mod pins it (the last Go that runs
# on the maintainer's macOS 12, ADR 38 decision 5), trixie because that is
# the Debian the runtime stage's `python:3.12-slim` is built on. It does not
# matter much: the binary is built with CGO off and is static.
FROM golang:1.26-trixie AS door

WORKDIR /build

# Modules first, so the download layer caches across source edits.
COPY go/go.mod go/go.sum ./
RUN go mod download

COPY go ./

# CGO off, deliberately, and it is asserted in CI too (`The front door
# builds without CGO` in ci.yml). The door's dependencies are pure Go --
# modernc.org/sqlite reads `app.db` -- and the DuckDB driver, the module's one
# CGO dependency, is not imported by this binary until Phase 3 lands the read
# spine. The day it is, this line changes with a C toolchain beside it, and
# this comment is where the decision gets made rather than discovered.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mtglab ./cmd/mtglab

# ------------------------------------------------------------------ builder

FROM python:3.12-slim AS builder

ENV PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_ROOT_USER_ACTION=ignore

WORKDIR /build

# `pyproject.toml` declares no `readme`, so the build backend never reads
# README.md and copying it would only widen the layer's cache key.
COPY pyproject.toml ./
COPY src ./src

# A venv rather than `--prefix`, so the runtime stage can copy one directory
# and put it on PATH without caring where the interpreter keeps site-packages.
#
# `.[api,claude]` and not `.[dev]`: fastapi, uvicorn, python-dotenv, argon2-cffi
# and the Anthropic SDK. No pytest, ruff or mypy.
#
# The `claude` extra was left out when this file was written, and the reason
# given was that "ADR 15's modes are not built, so an unused SDK is dependency
# surface with no caller." That was true on 2026-08-12 and is not true now:
# four modes are built, and the deployed instance had `ANTHROPIC_API_KEY` set
# as a secret while the image had nothing that could read it. `mtglab claude
# check` on the live machine answered `unavailable`, and the dossier and both
# theme-interview modes were 503s behind buttons the UI shows.
#
# #60 is the argument in one line: the theme proposal was made a background job
# *because* no hosted proxy holds a 226-second POST open. That work was for a
# deployment, so the deployment has to be able to run it.
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
RUN pip install --no-cache-dir ".[api,claude]"

# ------------------------------------------------------------------ runtime

FROM python:3.12-slim AS runtime

LABEL org.opencontainers.image.title="sylvan-library" \
      org.opencontainers.image.description="Local-first Commander toolkit" \
      org.opencontainers.image.source="https://github.com/aasquier/sylvan-library" \
      org.opencontainers.image.licenses="MIT"

# MTGLAB_DECKS_DIR points at the **volume**, which is the only copy there is
# (ADR 30). `deck.yaml` is the source of truth and every editing route in the
# app writes it — swap, add, remove, set, note, import, delete — so decks in
# an image layer would mean every edit made in the hosted app vanished at the
# next deploy. The image used to carry a seed copy at /app/decks-seed; decks
# left the repository entirely, so a fresh instance now starts with zero decks
# and is populated the way the pool is — a documented run (HOSTING §4 step 6):
# restore a backup over sftp, or import through the app.
#
# PYTHONDONTWRITEBYTECODE because the venv is root-owned and the app is not:
# every import would otherwise attempt a `.pyc` write it is not allowed to make.
# pip already compiled them in the builder, so there is nothing to gain anyway.
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PATH="/opt/venv/bin:$PATH" \
    MTGLAB_DATA_DIR=/data \
    MTGLAB_DECKS_DIR=/data/decks

# Take Debian's security updates rather than waiting for Docker Hub to
# republish the base, which is what broke the deploy on 2026-08-17.
#
# The image had no `apt-get upgrade` at all: it inherited exactly whatever
# `python:3.12-slim` last shipped, so a Debian security fix reached us only
# when the base image was rebuilt. Debian fixed CVE-2026-53615 — an integer
# overflow in `libblkid/src/partitions/dos.c`, HIGH — in util-linux
# `2.41.5-0+deb13u1`; the base was still on `2.41-5`; Trivy's database learned
# about it between #151's pull-request run and the `main` run of the very same
# commit, so a merge that had been green went red on push and `deploy` was
# skipped. **The scan was right and the image was stale**, which is why the
# answer here is to take the fix rather than to add a `.trivyignore`:
# `ignore-unfixed: true` in `ci.yml` means the gate only ever fires when a
# fixed version actually exists.
#
# Runtime stage only — the builder's packages never ship. And a known edge,
# written down rather than discovered later: `cache-from: type=gha` can serve
# this layer from cache, so it goes stale exactly while the base image digest
# is unchanged AND Debian has shipped something new. A base-image rebuild
# busts it automatically (FROM is the parent), and the Trivy gate catches the
# window in between — but if that window is ever hit, this line needs a
# deliberate cache bust rather than a re-run.
RUN apt-get update \
 && apt-get upgrade -y --no-install-recommends \
 && rm -rf /var/lib/apt/lists/*

# A fixed uid, so the volume's ownership survives a rebuild that would
# otherwise renumber the account.
RUN useradd --system --uid 10001 --create-home --shell /usr/sbin/nologin mtglab

COPY --from=builder /opt/venv /opt/venv

WORKDIR /app

# The door, **off PATH on purpose**. The venv's `mtglab` stays the one a
# `fly ssh console -C "mtglab decks validate ..."` finds, because during
# coexistence the runbook surface is Python's; the Go binary takes the name
# on PATH at Phase 8, when it carries those commands itself. Until then it is
# reached by absolute path from CMD and nowhere else.
COPY --from=door /out/mtglab /opt/door/mtglab

# What the door serves itself, pointed at the *installed package's* copies
# rather than a second COPY: one source for the bundle and the 78 tarot
# pictures, so the door and the Python server cannot disagree about a byte.
# Resolved through Python at build time because the site-packages path
# carries the interpreter version, and asserted so a rename fails the build
# rather than the first page load.
RUN ln -s "$(python -c 'import mtglab, pathlib; print(pathlib.Path(mtglab.__file__).parent / "web_dist")')" /app/web_dist \
 && ln -s "$(python -c 'import mtglab, pathlib; print(pathlib.Path(mtglab.__file__).parent / "assets" / "tarot")')" /app/tarot \
 && test -f /app/web_dist/index.html \
 && test -d /app/tarot
ENV MTGLAB_WEB_DIST=/app/web_dist \
    MTGLAB_TAROT_DIR=/app/tarot

COPY docker-entrypoint.sh /usr/local/bin/
# `setpriv` is how the entrypoint drops privileges; it ships in util-linux,
# which is Priority: required in Debian and so present in -slim. Asserted at
# build time rather than discovered at boot: if a future base image drops it,
# that should be a red build here, not a container that starts as root or
# crash-loops on a customer's first wake.
RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
 && command -v setpriv >/dev/null

# **The pool is never in this image.** Scryfall asks that bulk data not be
# redistributed; it is ~63MB built from ~98MB of downloads; and it belongs on
# the volume where it survives a deploy. CI fails the build if a card pool file is
# ever tracked, and `.dockerignore` keeps `data/` out of the build context so a
# local pool cannot reach a layer by accident either.

EXPOSE 8080

# stdlib rather than curl: python is in this image by definition, and adding a
# package for one HTTP request is a package to patch forever.
#
# `/api/health` is on `PUBLIC_PATHS`, so this answers with `MTGLAB_REQUIRE_AUTH`
# on. It reports pool state rather than merely being 200, but *liveness* is
# what a health check is for here — an instance with no card pool yet is a correct
# state to be in between deploy and seeding, not an unhealthy one.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/api/health', timeout=4)"]

ENTRYPOINT ["docker-entrypoint.sh"]

# **One worker, and the reason is not the one §4 used to give.** That note
# pointed at DuckDB, and §3 has since corrected itself: read-only handles are
# safe across processes, so serving would be fine on two.
#
# `api/jobs.py` is what actually binds. The job registry is a module-level dict
# in one process, so a sim submitted to worker A is invisible to worker B —
# and `get()` reports a job it cannot see as absent, which the route turns into
# a 404 (ADR 5, never a 403). The failure is a running simulation reported as
# gone, at random, half the time. Sessions and the login rate limiter are both
# in `app.db` and would be fine; the jobs are not.
#
# **The door is PID 1 and the Python server is its child** (ADR 38). The
# entrypoint still drops privileges and `exec`s -- into the door now -- so
# the process on the port is `mtglab` (uid 10001), as CI asserts; the door
# starts the command after `--`, relays SIGTERM to it, and exits when it
# exits, so a crashed half is a restart rather than a door answering 502 to
# everyone forever. `--upstream` and the child's `--port` name the same
# loopback port (a test holds them equal); the one-worker rule above is
# unchanged, and so is the HEALTHCHECK: `/api/health` is answered by the
# Python half *through* the door, which is what makes it the pair's health
# rather than the door's own.
CMD ["/opt/door/mtglab", "ui", "--host", "0.0.0.0", "--port", "8080", \
     "--upstream", "http://127.0.0.1:8765", \
     "--", "mtglab", "ui", "--no-open", "--host", "127.0.0.1", "--port", "8765"]
