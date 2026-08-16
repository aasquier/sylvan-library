"""Hypothesis configuration, and the guards that keep a test out of real state.

Four autouse fixtures now, and they divide in two. Three are *stubs*, each
closing a door through which a test was found writing the developer's own
`app.db`; the fourth, `_real_app_db_untouched`, is a *detector*, and it exists
because the first three were all found by hand, one incident at a time.

The environment guard first, because it is the shorter story. `MTGLAB_ADMIN_EMAIL` and
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

from mtglab import config

#: The developer's own `app.db`, resolved once from the ambient environment --
#: before any test can call `config.use_paths` -- so the guard below watches
#: the real database rather than whichever scratch copy is currently pointed at.
_REAL_APP_DB = config.APP_DB_PATH.resolve()


def _real_app_db_state() -> tuple[int, int] | None:
    """`None` if it is absent, else enough to notice any write to it."""
    try:
        stat = _REAL_APP_DB.stat()
    except OSError:
        return None
    return (stat.st_mtime_ns, stat.st_size)


@pytest.fixture(autouse=True)
def _real_app_db_untouched():
    """No test opens the developer's real `app.db` -- and now it is checked.

    The third door, after `_no_usage_ledger` and `_no_deck_log` below. Those
    two say they were found "by running the suite and then looking at the
    file", which is a discovery method that only works when somebody thinks to
    look; this fixture is that look, taken automatically after every test.

    It closes a real one. `tests/test_deck_source.py::test_library_resolution_edges`
    called `Library.source_for` for an unknown owner while authenticated, which
    falls through to `Library._owner_id` -- and `library.py` says at that very
    branch that opening `app.db` on a laptop means *creating* it. So the suite
    reached the maintainer's actual database, holding his account, his sessions
    and the only personal data this project stores, and ran `auth/db.py`'s
    migration ladder over it. That ladder is forward-only (ADR 23's note about
    unwatched migrations), so running the suite on an older checkout would have
    quietly carried the real schema forward with no way back.

    Neither of the two fixtures below could have caught it: both work by
    replacing a *writer* the code calls through, and this arrived as a direct
    connection with no seam to stub. Hence a detector rather than a fourth stub
    -- the next door does not have to be one anybody predicted.

    Absence is not the test. A maintainer running this has a real `app.db`
    sitting there, so the check is that the file's size and mtime are exactly
    what they were before the test ran; on a fresh checkout, or in CI, that
    reduces to "it was not created". `git status data/` cannot do this job and
    never could -- `app.db` is gitignored, so it is invisible to a status.
    """
    before = _real_app_db_state()
    yield
    after = _real_app_db_state()
    if after != before:
        what = "created" if before is None else "written to"
        pytest.fail(
            f"this test {what} the real {_REAL_APP_DB}. Point `config` at a "
            f"scratch directory with `config.use_paths(data_dir=tmp_path / "
            f"'data')`, or stub the writer, as the fixtures beside this one do."
        )


@pytest.fixture(autouse=True)
def _no_maintainer_configured(monkeypatch):
    """No maintainer is configured unless a test says so. See the docstring."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    monkeypatch.delenv("MTGLAB_ADMIN_USERNAME", raising=False)


@pytest.fixture(autouse=True)
def _no_usage_ledger(monkeypatch):
    """No test writes usage rows into the developer's real app.db.

    `modes.converse` records every conversation's token accounting through
    `claude/ledger.py`, which resolves its database from `config` -- and in a
    test process that is the repository's real data directory. Left alone,
    every scripted-conversation test in the suite would deposit junk rows in
    the laptop's actual ledger. So the attribute `modes.py` holds is replaced
    with a no-op here; the ledger module itself stays real, which is what
    `test_claude_ledger.py` exercises against a scratch path, and one test in
    `test_claude_modes.py` re-points a conversation at the real module to
    prove the seam end to end -- a stub nothing ever removes is how a broken
    seam stays green.
    """
    from types import SimpleNamespace

    from mtglab.claude import modes
    monkeypatch.setattr(modes, "ledger",
                        SimpleNamespace(record=lambda **kwargs: None))

@pytest.fixture(autouse=True)
def _no_deck_log(monkeypatch):
    """No test writes activity-log rows into the developer's real app.db.

    The same hazard as `_no_usage_ledger` above, arriving through a different
    door and found the same way — by running the suite and then looking at the
    file. `service._commit` records every deck edit through `decks/log.py`,
    which resolves its database from `config`; and the edit tests deliberately
    run against a `MemoryDeckSource` **without** overriding the data directory,
    because until now nothing about an edit touched a database at all. So the
    decks were safe and one row landed in the laptop's real history anyway,
    attributing a promotion of `gyome-food` to a test.

    Only `record` is replaced, and the module stays real behind it: `entries`
    and `describe` are pure reads a test may want, and `tests/test_deck_log.py`
    exercises the writer directly against a scratch path. One test there also
    puts the real module back in front of `service`, because a stub nothing
    ever removes is how a broken seam stays green.
    """
    from mtglab.api import service
    from mtglab.decks import log

    class _Silent:
        """`decks.log`, with a `record` that does not write."""

        def __getattr__(self, name):
            # Everything else — `entries`, `describe`, `DEFAULT_LIMIT` — comes
            # off the real module, so this cannot go stale when one is added.
            return getattr(log, name)

        @staticmethod
        def record(**_kwargs):
            pass

    monkeypatch.setattr(service, "log", _Silent())


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
