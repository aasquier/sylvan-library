"""Fixtures for the contract suite; the options are in `tests/conftest.py`.

`instance` is the seeded app, reached the way the command line asked
(`contract.harness`). Its scope is **dynamic**: module-scoped in-process, so
a contract module's `config.use_paths` is torn down before the next module
in an ordinary full run; session-scoped when a real server is in play, where
only this package runs and one server should answer all of it. Every test
is written to be safe under either -- nothing assumes an empty server, and
anything a test creates carries a per-run name.
"""

from __future__ import annotations

import os
import uuid
from collections.abc import Iterator
from pathlib import Path

import pytest

from contract import harness
from contract.goldens import Goldens


def contract_mode(config: pytest.Config) -> str:
    if config.getoption("--base-url"):
        return "external"
    if config.getoption("--live"):
        return "live"
    return "in-process"


def _scope(fixture_name: str, config: pytest.Config) -> str:
    return "module" if contract_mode(config) == "in-process" else "session"


@pytest.fixture(scope=_scope)
def instance(request: pytest.FixtureRequest,
             tmp_path_factory: pytest.TempPathFactory
             ) -> Iterator[harness.Instance]:
    config = request.config
    mode = contract_mode(config)
    root = tmp_path_factory.mktemp("contract")
    if mode == "external":
        data_dir, decks_dir = (config.getoption("--data-dir"),
                               config.getoption("--decks-dir"))
        if not data_dir or not decks_dir:
            raise pytest.UsageError(
                "--base-url needs --data-dir and --decks-dir: the harness "
                "seeds the directories the server was started with")
        scratch = harness.make_scratch(root, data_dir=Path(data_dir),
                                       decks_dir=Path(decks_dir))
    else:
        scratch = harness.make_scratch(root)
    mutation = os.environ.get("MTGLAB_CONTRACT_MUTATE") or None
    with harness.open_instance(mode, scratch,
                               base_url=config.getoption("--base-url"),
                               mutation=mutation) as inst:
        yield inst


@pytest.fixture(scope="session")
def goldens(request: pytest.FixtureRequest) -> Goldens:
    return Goldens(update=bool(request.config.getoption("--update-golden")))


@pytest.fixture
def run_id() -> str:
    """A short token that makes anything a test creates unique to this run."""
    return uuid.uuid4().hex[:8]


@pytest.fixture(autouse=True)
def _real_writers(instance: harness.Instance, monkeypatch: pytest.MonkeyPatch
                  ) -> None:
    """In-process, put back every writer the root conftest stubs.

    `tests/conftest.py` replaces the deck log, the traffic counter, the
    usage ledger and the match ledger with no-ops for every test, so that no
    test can write the developer's real `app.db`. Here they would write the
    contract scratch -- `instance` holds `config.use_paths` at it for the
    module -- and stubbing them makes the in-process app *differ* from a
    real server: an edit that leaves no history row, a request nobody
    counted. The contract is what a server does, so the writers go back.
    Over a socket there is nothing to restore; the server is its own
    process.
    """
    if instance.mode != "in-process":
        return
    from mtglab.api import forgeruns, service, traffic
    from mtglab.claude import ledger as usage_ledger
    from mtglab.claude import modes
    from mtglab.decks import log
    from mtglab.sim.tier3 import ledger as match_ledger
    monkeypatch.setattr(service, "log", log)
    monkeypatch.setattr(traffic, "record", traffic._REAL_RECORD)
    monkeypatch.setattr(traffic, "flush", traffic._REAL_FLUSH)
    monkeypatch.setattr(modes, "ledger", usage_ledger)
    monkeypatch.setattr(forgeruns, "ledger", match_ledger)
