"""The root conftest's app.db detector: armed everywhere except external mode.

`_real_app_db_untouched` watches the app.db that `config` resolved from the
ambient environment. In the contract suite's external mode the shell that
runs pytest exports the *server-under-test's* `MTGLAB_DATA_DIR` (the CI door
leg and the local pair recipe both do), so the watched file is the server's
own database, which it writes on its own clock -- sessions, the activity
log, the door's visitor ledger, WAL checkpoints. The guard must stand down
exactly there and nowhere else.

The wiring -- fixture consults predicate -- is proven by the CI door leg
itself, which went red the day the door grew a traffic recorder. These tests
hold the predicate's judgment. `conftest` is importable here the same way
`tiny_pool` is: pytest puts each test file's directory on `sys.path`.
"""

from pathlib import Path

import conftest


class _Options:
    """Just enough of `pytest.Config` for the predicate: `getoption`."""

    def __init__(self, base_url: str | None, data_dir: str | None) -> None:
        self._options = {"--base-url": base_url, "--data-dir": data_dir}

    def getoption(self, name: str) -> str | None:
        return self._options[name]


def test_ordinary_runs_keep_the_guard_armed(pytestconfig) -> None:
    """This very run: no --base-url, so the predicate says the guard stays.

    Driven through the real `pytest.Config` rather than the stub, so the
    option names the predicate asks for are proven registered -- a renamed
    option would fail here, not silently disarm nothing.
    """
    if pytestconfig.getoption("--base-url"):
        # An external contract run of this file is exactly the stand-down
        # case; the stub cases below still judge the predicate.
        return
    assert not conftest._watched_db_is_the_external_servers(pytestconfig)


def test_external_mode_on_the_servers_data_dir_stands_down() -> None:
    data_dir = str(conftest._REAL_APP_DB.parent)
    options = _Options("http://127.0.0.1:8765", data_dir)
    assert conftest._watched_db_is_the_external_servers(options)


def test_external_mode_on_a_different_data_dir_stays_armed(tmp_path: Path) -> None:
    """Ambient environment still pointing at the real db: the guard holds."""
    options = _Options("http://127.0.0.1:8765", str(tmp_path))
    assert not conftest._watched_db_is_the_external_servers(options)


def test_a_data_dir_without_a_base_url_stays_armed() -> None:
    """--data-dir alone is not external mode; the harness refuses it too."""
    options = _Options(None, str(conftest._REAL_APP_DB.parent))
    assert not conftest._watched_db_is_the_external_servers(options)
