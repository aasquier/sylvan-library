"""Per-account model tiers: the table, the column, and the precedence.

Which Claude answers whom (`claude/tiers.py`, schema v10). Aaron's call on
2026-08-18: a handful of seats may reach for a more capable model and everybody
else keeps the house answer, which is a different decision from the one ADR 14
deferred — not "is Sonnet enough for this project" but "somebody has to be able
to spend more on one seat than on another".

Four properties, and each is a decision rather than an implementation detail:

* **A tier is a name, never a model id.** The column holds `'opus'`, so the day
  a model id is superseded is not a migration.
* **An unknown tier resolves to the default rather than raising.** The column
  outlives any deploy; a rolled-back build reading a key it no longer knows
  must serve the ordinary model, not 500 every Claude surface.
* **Writing an unknown tier is refused.** The read path's tolerance must not
  reach the write path, or a typo grants a tier that does not exist and reads
  on the Admin page as the ordinary one.
* **The environment override outranks a tier.** `MTGLAB_CLAUDE_MODEL` is the
  A/B lever, and an A/B whose answer depended on which seat asked is not one.

No network and no key: nothing here makes a call.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

from mtglab import config
from mtglab.auth import db, users
from mtglab.claude import client, tiers


@pytest.fixture
def con(tmp_path):
    with (config.use_paths(data_dir=tmp_path / "data"),
          db.connection() as connection):
        yield connection


# ------------------------------------------------------------------ the table

def test_the_default_tier_is_the_house_model():
    """`tiers` and `client.MODEL` must not drift apart.

    Two spellings of "what an ordinary account is answered by" is one spelling
    too many: a default tier pointing somewhere else would mean every account
    silently moved the moment tiers shipped, which is exactly the drift the
    comment above `client.MODEL` forbids.
    """
    assert tiers.resolve(None) == client.MODEL
    assert tiers.get(None).key == tiers.DEFAULT_TIER


def test_every_tier_names_a_distinct_model():
    """Two keys pointing at one model would make the dossier cache's per-tier
    fragmentation a lie, and would make a grant mean nothing."""
    models = [tier.model for tier in tiers.TIERS]
    assert len(set(models)) == len(models), models


def test_an_unknown_tier_resolves_to_the_default_rather_than_raising():
    """The property that makes a rolled-back deploy survivable."""
    assert tiers.resolve("archaeopteryx") == client.MODEL
    assert tiers.get("archaeopteryx").key == tiers.DEFAULT_TIER
    assert not tiers.known("archaeopteryx")


def test_the_roster_carries_prose_and_never_a_model_id():
    """Commandment 10: no technology a user may see is ever named, and a model
    id is exactly that. The maintainer picks 'Opus'; the mapping stays here."""
    roster = tiers.roster()
    assert {t["key"] for t in roster} == {t.key for t in tiers.TIERS}
    blob = repr(roster)
    for tier in tiers.TIERS:
        assert tier.model not in blob, f"{tier.model} reached the wire"
    for entry in roster:
        assert entry["label"] and entry["blurb"]


# ------------------------------------------------------------- the precedence

def test_a_tier_chooses_the_model():
    assert client.model("opus") == tiers.get("opus").model
    assert client.model("opus") != client.MODEL


def test_no_tier_is_the_house_model():
    assert client.model() == client.MODEL
    assert client.model(None) == client.MODEL


def test_the_environment_override_outranks_every_tier(monkeypatch):
    """The A/B lever wins, deliberately.

    `MTGLAB_CLAUDE_MODEL` exists to put the whole instance on one model for a
    measurement. A comparison whose result depended on which seat happened to
    ask would not be a comparison, so a tier does not survive it — and setting
    the variable stays a deliberate operator action rather than something a
    grant can override from a database row.
    """
    monkeypatch.setenv("MTGLAB_CLAUDE_MODEL", "claude-haiku-4-5")
    assert client.model() == "claude-haiku-4-5"
    assert client.model("fable") == "claude-haiku-4-5"


# ----------------------------------------------------------------- the column

def test_a_new_account_has_no_grant(con):
    users.create(con, "friend", password="hunter2hunter2")
    account = users.get(con, "friend")
    assert account.model_tier is None
    assert client.model(account.model_tier) == client.MODEL


def test_a_grant_survives_a_round_trip(con):
    users.create(con, "sister", password="hunter2hunter2")
    users.set_model_tier(con, users.get(con, "sister").id, "fable")
    assert users.get(con, "sister").model_tier == "fable"


def test_granting_the_default_stores_nothing(con):
    """One representation for "nobody has chosen anything".

    Storing the default's key would work today and become wrong the day the
    default changes: every row that never asked for anything would be pinned
    to whatever the default used to be.
    """
    users.create(con, "friend", password="hunter2hunter2")
    uid = users.get(con, "friend").id
    users.set_model_tier(con, uid, "opus")
    users.set_model_tier(con, uid, tiers.DEFAULT_TIER)
    assert users.get(con, "friend").model_tier is None


def test_clearing_a_grant_restores_the_default(con):
    users.create(con, "friend", password="hunter2hunter2")
    uid = users.get(con, "friend").id
    users.set_model_tier(con, uid, "opus")
    users.set_model_tier(con, uid, None)
    assert users.get(con, "friend").model_tier is None


def test_writing_an_unknown_tier_is_refused(con):
    """The read path tolerates one; the write path must not.

    Reading is tolerant because the column outlives the code. Writing is strict
    because a tier nothing resolves is a grant that silently is not one — it
    would read as 'Opus' in the column and be answered by Sonnet forever.
    """
    users.create(con, "friend", password="hunter2hunter2")
    uid = users.get(con, "friend").id
    with pytest.raises(users.UnknownTier):
        users.set_model_tier(con, uid, "archaeopteryx")
    assert users.get(con, "friend").model_tier is None


def test_a_grant_for_nobody_is_refused(con):
    with pytest.raises(users.NoSuchUser):
        users.set_model_tier(con, 9999, "opus")


def test_a_stale_key_in_the_column_reports_the_tier_it_will_be_answered_by(con):
    """The bug this guards is a screen that lies.

    A row written by a build that knew a tier this one does not is answered by
    the default — so `as_dict` must say the default, not echo the column. An
    Admin page showing 'Opus' beside an account served Sonnet is the same class
    of untruth as quoting a cached simulation as a fresh one (ADR 18).
    """
    users.create(con, "friend", password="hunter2hunter2")
    uid = users.get(con, "friend").id
    # Straight past `set_model_tier`, which is what a future build's migration
    # or a hand-edited database looks like from here.
    con.execute("UPDATE users SET model_tier = ? WHERE id = ?",
                ("archaeopteryx", uid))
    con.commit()

    account = users.get(con, "friend")
    assert account.model_tier == "archaeopteryx"          # the column, verbatim
    assert account.as_dict()["model_tier"] == tiers.DEFAULT_TIER
    assert client.model(account.model_tier) == client.MODEL


def test_the_tier_is_serialised_without_asking_for_the_address(con):
    """Unlike `email`, a tier is not personal data and is not opt-in.

    It is a fact about the instance's spending. The assertion that matters is
    the other half: turning the tier on did not turn the address on with it.
    """
    users.create(con, "friend", email="friend@example.com",
                 password="hunter2hunter2")
    body = users.get(con, "friend").as_dict()
    assert body["model_tier"] == tiers.DEFAULT_TIER
    assert "email" not in body
