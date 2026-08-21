"""The stateless half of the goldens, and the check that none has gone stale.

One request, one record, compared against `golden/<family>.json`; see
`cases.py` for what is pinned and what is deliberately not.
"""

from __future__ import annotations

import json

import pytest

from contract import cases
from contract.checks import observed
from contract.goldens import GOLDEN_DIR


@pytest.mark.parametrize("case", cases.READS, ids=lambda c: c.id)
def test_wire_shape(instance, goldens, case):
    client = instance.as_user(case.user)
    response = client.request(case.method, case.path, json=case.json,
                              content=case.content, params=case.params)
    assert response.status_code in case.expect, (
        f"{case.id}: {case.method} {case.path} answered "
        f"{response.status_code}, not one of {case.expect}: "
        f"{response.text[:300]}")
    goldens.check(case.family, case.name,
                  observed(response, volatile=case.volatile))


def test_every_golden_record_is_still_produced():
    """A record nobody records any more is a shape nobody is checking.

    Compared against `cases.py` rather than against a run, so `-k` cannot
    make a neighbour look stale and the check costs nothing."""
    expected = cases.expected_records()
    on_disk = {path.stem: set(json.loads(path.read_text(encoding="utf-8")))
               for path in GOLDEN_DIR.glob("*.json")}
    stale = {family: sorted(names - expected.get(family, set()))
             for family, names in on_disk.items()
             if names - expected.get(family, set())}
    assert not stale, (
        f"golden records no case produces any more: {stale}. Remove them "
        f"from golden/, or name the case in cases.py.")
    unknown_families = set(on_disk) - set(expected)
    assert not unknown_families, f"golden files no family claims: {unknown_families}"


def test_every_expected_record_is_on_disk(goldens):
    """The other direction: a case whose golden was never recorded.

    The parametrised test above fails per case with the same message; this
    one says it once, for the whole table, which is the message a first
    checkout wants. Under `--update-golden` the files are being written as
    this runs, so there is nothing to compare yet."""
    if goldens.update:
        return
    expected = cases.expected_records()
    missing = {}
    for family, names in expected.items():
        path = GOLDEN_DIR / f"{family}.json"
        have = set(json.loads(path.read_text(encoding="utf-8"))) \
            if path.exists() else set()
        if names - have:
            missing[family] = sorted(names - have)
    assert not missing, (
        f"golden records not yet recorded: {missing}. Run "
        f"`pytest tests/contract --update-golden` and review the diff.")
