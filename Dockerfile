# sylvan-library, containerised. `docs/HOSTING.md` §4 is the deployment guide;
# this file is the thing that guide used to only describe.
#
# Two stages, and deliberately **no Node stage**. `web_dist/` is
# committed precisely so the image needs no Node toolchain. CI's `frontend`
# job proves the committed bundle can be rebuilt from source on every pull
# request — running the real `npm run build` and failing on any diff — so the
# split is: CI proves the bundle is current, the image ships it.

# ------------------------------------------------------------------ builder
#
# `golang:1.26-trixie`: 1.26 because go.mod pins it (the last Go that runs
# on the maintainer's macOS 12), trixie because the runtime stage is
# `debian:trixie-slim` — and that match is load-bearing rather than tidy:
# the binary links the builder's glibc and must find the same one at run.
FROM golang:1.26-trixie AS builder

WORKDIR /build

# Modules first, so the download layer caches across source edits.
COPY go/go.mod go/go.sum ./
RUN go mod download

COPY go ./

# CGO **on**: the server reads the card pool, and the DuckDB driver links a
# prebuilt libduckdb per platform — a static archive, so the C toolchain is
# needed here and nowhere else. The binary comes out linked against glibc
# and libstdc++; the runtime stage installs libstdc++6 for it explicitly,
# exactly as the Forge worker image does. CI proves the same build on both
# architectures (`The server builds as the image builds it` in ci.yml).
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/mtglab ./cmd/mtglab

# ------------------------------------------------------------------ runtime

FROM debian:trixie-slim AS runtime

LABEL org.opencontainers.image.title="sylvan-library" \
      org.opencontainers.image.description="Local-first Commander toolkit" \
      org.opencontainers.image.source="https://github.com/aasquier/sylvan-library" \
      org.opencontainers.image.licenses="MIT"

# MTGLAB_DECKS_DIR points at the **volume**, which is the only copy there is
# (ADR 30). `deck.yaml` is the source of truth and every editing route in the
# app writes it — swap, add, remove, set, note, import, delete — so decks in
# an image layer would mean every edit made in the hosted app vanished at the
# next deploy. A fresh instance starts with zero decks and is populated the
# way the pool is — a documented run (HOSTING §4 step 6): restore a backup
# over sftp, or import through the app.
ENV MTGLAB_DATA_DIR=/data \
    MTGLAB_DECKS_DIR=/data/decks

# Take Debian's security updates rather than waiting for Docker Hub to
# republish the base: the image otherwise inherits exactly whatever
# `debian:trixie-slim` last shipped, and a Debian security fix reaches it
# only when the base is rebuilt. `ignore-unfixed` in ci.yml means that gate
# only ever fires when a fixed version exists.
#
# `libstdc++6` is for the DuckDB static archive the binary links; the slim
# base does not promise it.
#
# **`SECURITY_EPOCH` is what makes the line below actually run.** `cache-from:
# type=gha` will happily serve this layer forever: nothing above it changes
# when Debian ships a fix, so the layer stays warm and the upgrade never
# happens. That is not a hypothetical — it has now skipped a deploy twice,
# CVE-2026-53615 and CVE-2026-14456, and both times the remedy written down
# here was "a deliberate cache bust", which is a remedy that requires somebody
# to notice. Nobody noticed either time; the first sign was a red required
# check on `main` and a site that had silently stopped deploying.
#
# So the cache key carries a date. CI passes today's, the layer is rebuilt at
# most once a day, and the image is never more than a day behind Debian's
# security archive without anybody having to think about it. The default is a
# constant so a local `docker build` with no arguments still builds and still
# caches; CI is the only place freshness is a promise, and CI is the only
# place the image is built for a deploy.
#
# The cost is the layers below this one rebuilding daily: a `useradd`, the
# binary, and two directory copies. The Go build is a separate stage above and
# does not notice.
ARG SECURITY_EPOCH=0
RUN apt-get update \
 && apt-get upgrade -y --no-install-recommends \
 && apt-get install -y --no-install-recommends libstdc++6 ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# A fixed uid, so the volume's ownership survives a rebuild that would
# otherwise renumber the account.
RUN useradd --system --uid 10001 --create-home --shell /usr/sbin/nologin mtglab

WORKDIR /app

# The one binary, on PATH under the name the CLI has always had: the same
# `mtglab` answers the port and the runbook (`fly ssh console -C "mtglab
# data refresh"`, `mtglab users list`, `mtglab decks validate ...`).
COPY --from=builder /out/mtglab /usr/local/bin/mtglab

# What the server serves: the committed bundle and the 78 tarot pictures,
# copied from the build context — the same files CI's `frontend` job holds
# current. Asserted here so a moved directory fails the build rather than
# the first page load.
COPY web_dist /app/web_dist
COPY assets/tarot /app/tarot
RUN test -f /app/web_dist/index.html && test -d /app/tarot
ENV MTGLAB_WEB_DIST=/app/web_dist \
    MTGLAB_TAROT_DIR=/app/tarot

COPY docker-entrypoint.sh /usr/local/bin/
# `setpriv` is how the entrypoint drops privileges; it ships in util-linux,
# which is Priority: required in Debian and so present in -slim. Asserted at
# build time rather than discovered at boot.
RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
 && command -v setpriv >/dev/null

# **The pool is never in this image.** Scryfall asks that bulk data not be
# redistributed; it is ~63MB built from ~98MB of downloads; and it belongs on
# the volume where it survives a deploy. CI fails the build if a card pool
# file is ever tracked, and `.dockerignore` keeps `data/` out of the build
# context so a local pool cannot reach a layer by accident either.

EXPOSE 8080

# The binary probes itself: the image carries no curl and no interpreter.
# `/api/health` is on `PUBLIC_PATHS`, so this answers with
# `MTGLAB_REQUIRE_AUTH` on. It reports pool state rather than merely being
# 200, but *liveness* is what a health check is for here — an instance with
# no card pool yet is a correct state between deploy and seeding, not an
# unhealthy one.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["mtglab", "probe", "http://127.0.0.1:8080/api/health"]

ENTRYPOINT ["docker-entrypoint.sh"]

# The entrypoint fixes the volume's ownership, drops privileges and `exec`s,
# so the process on the port is `mtglab` (uid 10001), as CI asserts. One
# process; the job registry lives in it, which is why there is exactly one.
CMD ["mtglab", "ui", "--host", "0.0.0.0", "--port", "8080"]
