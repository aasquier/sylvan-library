#!/bin/sh
# Fix the volume's ownership, then drop to the unprivileged account.
#
# Fly attaches a volume owned by root:root, and the mount shadows whatever the
# image had at that path — so a `chown` in the Dockerfile is invisible by the
# time it would matter. The app has to write there: `app.db`, the corpus, and
# (since decks moved to the volume) every deck edit made in the UI.
#
# So PID 1 starts as root, does the one thing that needs root, and execs the
# app as `mtglab`. The application process — the thing exposed to the network —
# never runs as root, which is the property `docs/ENGINEERING.md` §3 asks for.
# A plain `USER mtglab` in the Dockerfile would look stricter and would leave
# the app unable to write to its own volume.
set -eu

DATA_DIR="${MTGLAB_DATA_DIR:-/data}"

if [ "$(id -u)" = "0" ]; then
    mkdir -p "$DATA_DIR"
    # Recursive, and cheap here specifically: this tree is a handful of large
    # files (one DuckDB, one or two Scryfall downloads, `app.db`) plus a few
    # dozen small deck and artifact files. It is not a package tree, so the
    # metadata pass does not show up on the wake path that scale-to-zero puts
    # in front of a waiting visitor.
    #
    # Recursive rather than just the mount point because the seeding run in
    # HOSTING §4 step 6 happens over `fly ssh console`, which is root: without
    # this, a correctly seeded volume would be one the app cannot write to, and
    # the symptom would be a 500 on the first deck edit rather than anything
    # near the cause.
    chown -R mtglab:mtglab "$DATA_DIR"
    exec setpriv --reuid=mtglab --regid=mtglab --init-groups "$@"
fi

# Already unprivileged: nothing to drop, and no chown is possible. This is the
# path a local `docker run --user` takes.
exec "$@"
