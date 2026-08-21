"""The golden store: `golden/<family>.json`, one record per named case.

A record is what `checks.observed` returns -- status, content type, a few
headers, the body's shape -- and the store either compares a fresh one
against the file or, under `--update-golden`, writes it in. Writes are
read-merge-write so a partial run (`-k`) never drops a neighbour's record;
the check for records nobody produces any more is `test_contract_golden.py`'s
ghost test, which compares the files against `cases.py` without running
anything.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from contract.checks import check_golden

GOLDEN_DIR = Path(__file__).resolve().parent / "golden"


class Goldens:
    def __init__(self, directory: Path = GOLDEN_DIR, *, update: bool = False):
        self.directory = directory
        self.update = update
        self.seen: dict[str, set[str]] = {}

    def path(self, family: str) -> Path:
        return self.directory / f"{family}.json"

    def load(self, family: str) -> dict[str, Any]:
        path = self.path(family)
        if not path.exists():
            return {}
        data = json.loads(path.read_text(encoding="utf-8"))
        assert isinstance(data, dict), f"{path} must hold an object"
        return data

    def check(self, family: str, name: str, actual: dict[str, Any]) -> None:
        """Compare `actual` with the record, or record it under --update-golden."""
        self.seen.setdefault(family, set()).add(name)
        if self.update:
            table = self.load(family)
            table[name] = actual
            self.directory.mkdir(parents=True, exist_ok=True)
            self.path(family).write_text(
                json.dumps(table, indent=2, sort_keys=True) + "\n",
                encoding="utf-8")
            return
        recorded = self.load(family).get(name)
        assert recorded is not None, (
            f"{family}/{name} has no golden record yet; run "
            f"`pytest tests/contract --update-golden` and review the diff")
        check_golden(actual, recorded, f"{family}/{name}")
