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

# MTGLAB_DECKS_DIR points at the **volume**, not at the copy baked in below.
# `deck.yaml` is the source of truth and every editing route in the app writes
# it — swap, add, remove, set, note, import, delete — so decks living on the
# image's read-only-in-spirit layer would mean every edit made in the hosted
# app vanished at the next deploy, silently and with no error to notice.
#
# PYTHONDONTWRITEBYTECODE because the venv is root-owned and the app is not:
# every import would otherwise attempt a `.pyc` write it is not allowed to make.
# pip already compiled them in the builder, so there is nothing to gain anyway.
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PATH="/opt/venv/bin:$PATH" \
    MTGLAB_DATA_DIR=/data \
    MTGLAB_DECKS_DIR=/data/decks

# A fixed uid, so the volume's ownership survives a rebuild that would
# otherwise renumber the account.
RUN useradd --system --uid 10001 --create-home --shell /usr/sbin/nologin mtglab

COPY --from=builder /opt/venv /opt/venv

WORKDIR /app

# The repository's decks, as a **seed** and not as the live directory. Copying
# them into place is one line of the volume-seeding run in HOSTING §4 step 6,
# deliberately manual: which of these two copies is authoritative is a question
# with a real answer per instance, and a boot-time sync would pick one silently.
COPY decks /app/decks-seed

COPY docker-entrypoint.sh /usr/local/bin/
# `setpriv` is how the entrypoint drops privileges; it ships in util-linux,
# which is Priority: required in Debian and so present in -slim. Asserted at
# build time rather than discovered at boot: if a future base image drops it,
# that should be a red build here, not a container that starts as root or
# crash-loops on a customer's first wake.
RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
 && command -v setpriv >/dev/null

# **The corpus is never in this image.** Scryfall asks that bulk data not be
# redistributed; it is ~63MB built from ~98MB of downloads; and it belongs on
# the volume where it survives a deploy. CI fails the build if a corpus file is
# ever tracked, and `.dockerignore` keeps `data/` out of the build context so a
# local corpus cannot reach a layer by accident either.

EXPOSE 8080

# stdlib rather than curl: python is in this image by definition, and adding a
# package for one HTTP request is a package to patch forever.
#
# `/api/health` is on `PUBLIC_PATHS`, so this answers with `MTGLAB_REQUIRE_AUTH`
# on. It reports corpus state rather than merely being 200, but *liveness* is
# what a health check is for here — an instance with no corpus yet is a correct
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
CMD ["mtglab", "ui", "--no-open", "--host", "0.0.0.0", "--port", "8080"]
