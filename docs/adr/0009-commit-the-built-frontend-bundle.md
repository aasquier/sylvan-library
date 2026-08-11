# 9. Commit the built frontend bundle

**Status:** Accepted · **Recorded:** 2026-08-10

## Context

The UI is a React + Vite app under `web/`. The backend is a FastAPI app that
serves it as static files. The tool is installed with `pip install -e .` and run
with `mtglab ui`.

Committing build output is normally a mistake, so this one needs an argument.

## Options considered

**Build the frontend at install time.** Rejected. It puts a Node toolchain and
an `npm ci` on the critical path of `pip install`, for a Python package. Anyone
who wants to run the CLI and never opens the UI pays for it anyway.

**Ship the frontend separately.** Rejected. Two deployables, a CORS
configuration and a version-skew problem, for a single-user local tool.

**Build in CI and attach to a release.** A reasonable option, and the one to
revisit if this ever gets awkward. Rejected for now because it makes a git clone
insufficient: you would have to fetch a release artifact before the UI worked,
which is worse for the "clone it and run it" case that matters most.

**Commit `src/mtglab/web_dist/`.** Chosen.

## Decision

The built bundle is committed and shipped inside the package, so that a clone
plus `pip install -e .` gives a working `mtglab ui` with **no Node installed at
all**. `.gitignore` ignores `web/dist/` and explicitly does *not* ignore
`src/mtglab/web_dist/`, with a comment saying why.

The obvious hazard — someone edits `web/src`, pushes, and the app quietly serves
the old bundle — is closed by a CI job. It runs `npm ci`, `tsc -b` and
`npm run build`, then fails if `git diff --quiet -- src/mtglab/web_dist` reports
a change. Output filenames are stable and the build is byte-reproducible, so a
clean diff is a real check rather than a hopeful one.

## Consequences

- `mtglab ui` works from a clone with only Python installed. That is the point.
- **The committed bundle cannot silently drift from source**, because CI rebuilds
  it and compares. The failure message says exactly what to run:
  `npm --prefix web run build`.
- Diffs on frontend PRs include minified output. Noisy, and the price of the
  above. A reviewer reads `web/src`, not `web_dist`.
- The build must stay byte-reproducible for the drift check to work. If a
  dependency ever introduces a timestamp or a random chunk hash, the check turns
  into a false alarm and the decision has to be revisited — probably by moving
  to build-in-CI-and-attach.
- The container image needs no Node stage, which is a straightforward win for
  the hardening work in `docs/ENGINEERING.md` §3. The image build should still
  prove the bundle can be rebuilt from source.
