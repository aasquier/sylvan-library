"""Hypothesis configuration, and one environment guard.

The guard first, because it is the shorter story. `MTGLAB_ADMIN_EMAIL` and
`MTGLAB_ADMIN_USERNAME` (ADR 17) make the app create an account as it starts
serving, and the maintainer's own `.env` sets them — so a suite that inherited
the environment would pass in CI, where nothing is set, and fail on the laptop
of the person who configured their own instance. That is the worst shape a test
failure can have: it depends on a gitignored file, so nobody else can reproduce
it.

`_no_maintainer_configured` clears both for every test. The handful that are
*about* the bootstrap set them back with `monkeypatch.setenv`, which is the
right way round: reconciling an account is a thing a test opts into, not a
thing it inherits.

Two Hypothesis profiles, because a local run and a CI run want different things
from the same tests.

Locally, randomness is the point: every run should get a fresh shot at finding
something, and the on-disk example database should remember failures so a
shrunk counterexample is replayed first next time.

In CI, a test that fails on one pull request and passes on the next because
Hypothesis rolled different examples is worse than useless -- it trains people
to re-run the job. So CI derandomises. The deterministic coverage that a
randomised search would have provided comes instead from the enumerated pool
in `mana_oracle.py`, which is exhaustive over its alphabet and identical on
every run.

`deadline=None` in both: the per-example time limit measures the machine, not
the code, and a shared runner will trip it on a solver that is doing nothing
wrong.

`database=None` in CI: the example database is a cache of previously-failing
inputs, which has no meaning on a runner that is destroyed afterwards.

Note what that does *not* do. Hypothesis still keeps a small unicode and
constants cache under `.hypothesis/` in the working directory regardless of the
database setting -- measured, not assumed. The knob that moves it is the
`HYPOTHESIS_STORAGE_DIRECTORY` environment variable; with it set, nothing is
written into the repository at all. That is what the read-only root filesystem
in docs/ENGINEERING.md 3 will need, and it is a container setting rather than
something this file can decide, because the directory is resolved at import.
"""

import os

import pytest
from hypothesis import HealthCheck, settings


@pytest.fixture(autouse=True)
def _no_maintainer_configured(monkeypatch):
    """No maintainer is configured unless a test says so. See the docstring."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    monkeypatch.delenv("MTGLAB_ADMIN_USERNAME", raising=False)

settings.register_profile("dev", max_examples=200, deadline=None)
settings.register_profile(
    "ci",
    max_examples=500,
    deadline=None,
    database=None,
    derandomize=True,
    # The brute-force oracle is factorial by design; "this data generation is
    # slow" is a fact about the reference implementation, not a problem.
    suppress_health_check=[HealthCheck.too_slow],
)

settings.load_profile(
    os.environ.get("HYPOTHESIS_PROFILE") or ("ci" if os.environ.get("CI") else "dev")
)
