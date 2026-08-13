"""The commander dossier: everything about it that does not need a model.

No network, no key, no tokens — same rule as `test_claude_interview.py`, and
for the same reason. What is here is the part that has to hold whatever the
model says, and for this mode that is a larger surface than for the interview,
because the dossier is the first mode allowed to cite something other than the
corpus.

Three checks carry [ADR 19](../docs/adr/0019-the-dossier-cites-three-sources.md):

* **A cited page must be one the search actually returned.** This is the one to
  understand before the rest. With a response schema in play the API attaches
  no citations, so a URL in the payload is a string the model typed and nothing
  more. `keep_sources` is what puts something behind it, and the tests below
  include the case that matters — a real-looking URL that was never fetched.
* **Every rival is a corpus row or it is not there.** Rule 1, in the sentence
  most likely to break it.
* **No source survived means no dossier.** Rendering one anyway would be the
  unattributed paragraph the ADR rejected, reached by accident.

The write boundary that outranks all of it lives in `test_claude_boundary.py`,
which picks `dossier.py` up automatically.
"""

import json
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import tiny_corpus  # noqa: E402
from mtglab import config  # noqa: E402
from mtglab.claude import client, dossier, modes, stance, tools  # noqa: E402
from mtglab.decks.model import Deck  # noqa: E402
from mtglab.decks.source import MemoryDeckSource  # noqa: E402

DECK_YAML = """\
slug: mini
name: Mini Deck
status: theoretical
stage: curated
commander:
  - Gyome, Master Chef
cards:
  - name: Swamp
    category: land
    qty: 99
    why: Black mana.
"""


@pytest.fixture
def source(tmp_path):
    path = tmp_path / "deck.yaml"
    path.write_text(DECK_YAML, encoding="utf-8")
    return MemoryDeckSource([Deck.load(path)])


@pytest.fixture
def corpus(tmp_path):
    """A real DuckDB corpus, and a scratch `app.db` for the dossier store."""
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_corpus.build(config.DB_PATH)


@pytest.fixture
def no_network(monkeypatch):
    """Any attempt to reach the API is a test failure rather than a bill."""
    def refuse():
        raise AssertionError("a Claude call was made; this test forbids one")
    monkeypatch.setattr(client, "connect", refuse)


# --------------------------------------------------------------- the mode

def test_the_mode_declares_no_write_capability():
    """ADR 19 leaves ADR 15's invariant alone, as an assertion."""
    assert dossier.COMMANDER_DOSSIER.may_write == ()


def test_the_tool_set_is_inside_the_read_only_registry():
    assert set(dossier.COMMANDER_DOSSIER.tool_names) <= set(tools.READ_ONLY)


def test_the_dossier_cannot_read_the_decklist():
    """`get_deck` is absent on purpose.

    A dossier is about the commander, not the 99. Handing it the decklist —
    and every rationale in it — would put a mode that must never comment on
    deck contents in front of exactly that, and the prompt would be the only
    thing standing in the way.
    """
    named = set(dossier.COMMANDER_DOSSIER.tool_names)
    assert "get_deck" not in named
    assert "suggest_replacements" not in named


def test_the_only_server_tool_is_a_hosted_search():
    """`CLAUDE.md` bans a crawler. This is the assertion that the mode reaches
    the web through Anthropic's hosted search and through nothing else."""
    server = dossier.COMMANDER_DOSSIER.server_tools
    assert [t["name"] for t in server] == ["web_search"]
    assert server[0]["type"] == "web_search_20260209"


def test_code_execution_is_not_declared_alongside_the_search():
    """The dated search runs its own filtering in a container. Declaring a
    second execution environment confuses the model, and the docs say so."""
    types = {t["type"] for t in dossier.COMMANDER_DOSSIER.server_tools}
    assert not any(t.startswith("code_execution") for t in types)


def test_the_request_tool_list_is_stable_across_calls():
    """Tools render first in the prompt, so an unstable order would invalidate
    the prompt cache on every call — for free, and invisibly."""
    once = json.dumps(dossier.COMMANDER_DOSSIER.schemas())
    twice = json.dumps(dossier.COMMANDER_DOSSIER.schemas())
    assert once == twice


def test_the_schema_forbids_an_unattributed_section():
    """Every passage declares what it rests on, and cannot grow a field that
    would let it say something else."""
    props = dossier.RESPONSE_SCHEMA["properties"]
    assert dossier.RESPONSE_SCHEMA["additionalProperties"] is False
    for name in ("who", "standing"):
        assert set(props[name]["required"]) == {"prose", "source_ids"}
        assert props[name]["additionalProperties"] is False


def test_the_schema_has_nowhere_to_rate_the_deck():
    """The dossier is about a character. A rating, a recommendation or a
    verdict on the list belongs to a different surface with the user's hands
    on it, and there is no field here to smuggle one through."""
    blob = json.dumps(dossier.RESPONSE_SCHEMA)
    for word in ("rating", "power_level", "recommend", "verdict", "swap"):
        assert word not in blob


# ------------------------------------------------- a source must be fetched

SEARCHED = [
    {"url": "https://edhrec.com/commanders/gyome-master-chef",
     "title": "Gyome, Master Chef | EDHREC"},
    {"url": "https://articles.starcitygames.com/cooking-with-gyome/",
     "title": "Cooking With Gyome"},
]


def test_a_page_the_search_returned_is_kept():
    kept, dropped = dossier.keep_sources(
        [{"id": "s1", "title": "whatever", "url": SEARCHED[0]["url"]}], SEARCHED)
    assert dropped == 0
    assert kept[0]["url"] == SEARCHED[0]["url"]


def test_a_plausible_url_that_was_never_fetched_is_dropped():
    """The failure this whole mechanism exists for.

    The URL is real-looking, on a real site, about the right card — and the
    search never returned it. Nothing downstream could tell; the citation
    would render beside a claim and look exactly like the checked ones.
    """
    kept, dropped = dossier.keep_sources(
        [{"id": "s1", "title": "Gyome deck guide",
          "url": "https://edhrec.com/commanders/gyome-master-chef/budget"}],
        SEARCHED)
    assert kept == []
    assert dropped == 1


def test_the_title_comes_from_the_search_not_the_model():
    """One is a fact about the page; the other is a description of it."""
    kept, _ = dossier.keep_sources(
        [{"id": "s1", "title": "The Definitive Gyome Primer",
          "url": SEARCHED[0]["url"]}], SEARCHED)
    assert kept[0]["title"] == "Gyome, Master Chef | EDHREC"


def test_matching_ignores_a_trailing_slash_and_host_case():
    kept, dropped = dossier.keep_sources(
        [{"id": "s1", "title": "t",
          "url": "https://EDHREC.com/commanders/gyome-master-chef/"}], SEARCHED)
    assert dropped == 0 and len(kept) == 1


def test_matching_does_not_ignore_the_path():
    """Treating a site as a source is how a citation stops meaning anything —
    'edhrec.com said so' is not a claim anybody can check."""
    _, dropped = dossier.keep_sources(
        [{"id": "s1", "title": "t", "url": "https://edhrec.com/"}], SEARCHED)
    assert dropped == 1


def test_nothing_searched_means_nothing_cited():
    _, dropped = dossier.keep_sources(
        [{"id": "s1", "title": "t", "url": SEARCHED[0]["url"]}], [])
    assert dropped == 1


def test_a_section_cannot_cite_a_source_that_was_dropped():
    """Dropping the source is only half of it: a passage still pointing at its
    id would render a dangling citation marker, which reads as evidence."""
    section = dossier._section({"prose": "A claim.", "source_ids": ["s1", "s2"]},
                               allowed={"s2"})
    assert section["source_ids"] == ["s2"]


# ----------------------------------------------------- rivals are real cards

def test_a_rival_the_corpus_does_not_have_is_dropped(corpus):
    """Rule 1 in the sentence most likely to break it.

    'Gyome's obvious rival is X' is exactly the shape of claim a model
    produces fluently about a card that does not exist.
    """
    rivals, dropped = dossier._rivals(
        [{"card": "Sol Ring", "prose": "Fast mana.", "source_ids": []},
         {"card": "Gyome, Sous Chef", "prose": "Invented.", "source_ids": []}],
        allowed=set())
    assert [r["name"] for r in rivals] == ["Sol Ring"]
    assert dropped == 1


def test_a_kept_rival_carries_the_corpus_row(corpus):
    """The prose sits next to the real card, which is what lets a reader catch
    a wrong claim about it. A first live run described a rival as making Food
    tokens when it makes Soldiers; the card being right there is the half of
    the fix that does not depend on the model complying."""
    rivals, _ = dossier._rivals(
        [{"card": "Craterhoof Behemoth", "prose": "Ends games.",
          "source_ids": []}], allowed=set())
    assert rivals[0]["mana_cost"] == "{5}{G}{G}{G}"
    assert "trample" in (rivals[0]["oracle_text"] or "").lower()


def test_rivals_survive_a_corpus_that_is_not_there(tmp_path):
    """A fresh clone has no corpus. That is an empty rivals list, not a 500.

    The scratch data dir is not optional. Without it this reads whatever
    `data/mtg.duckdb` happens to hold on the machine running it — green on a
    laptop with the 500MB download, and testing nothing.
    """
    with config.use_paths(data_dir=tmp_path / "empty"):
        rivals, dropped = dossier._rivals(
            [{"card": "Sol Ring", "prose": "x", "source_ids": []}], allowed=set())
    assert rivals == [] and dropped == 1


# ----------------------------------------------------------- refusing to lie

def test_no_surviving_source_means_no_dossier(corpus, source, monkeypatch):
    """ADR 19: an unsourced dossier is the blended paragraph the design
    rejected, arrived at by accident. It is refused instead."""
    invented = json.dumps({
        "who": {"prose": "A troll.", "source_ids": ["s1"]},
        "archetype": {"name": "Food", "prose": "Food.", "source_ids": ["s1"]},
        "rivals": [], "standing": {"prose": "Niche.", "source_ids": ["s1"]},
        "sources": [{"id": "s1", "title": "Made up",
                     "url": "https://example.com/never-fetched"}],
    })
    monkeypatch.setattr(dossier, "converse", lambda *a, **k: modes.Turn(
        mode="commander-dossier", model="m", stop_reason="end_turn",
        text=invented, tool_calls=[], input_tokens=1, output_tokens=1,
        searched=[{"url": "https://edhrec.com/x", "title": "t"}]))

    report = dossier.ask("mini", requested="consultant", source=source)
    assert report["dossier"] == {}
    assert "No source survived" in report["reason"]


def test_the_dropped_counts_are_reported(corpus, source, monkeypatch):
    """A number that climbs is a prompt inventing citations. Nobody checks a
    number they cannot see, so it is in the payload."""
    payload = json.dumps({
        "who": {"prose": "A troll.", "source_ids": ["s1", "s2"]},
        "archetype": {"name": "Food", "prose": "Food.", "source_ids": ["s1"]},
        "rivals": [{"card": "Not A Real Card", "prose": "x",
                    "source_ids": ["s1"]}],
        "standing": {"prose": "Niche.", "source_ids": []},
        "sources": [
            {"id": "s1", "title": "t", "url": "https://edhrec.com/real"},
            {"id": "s2", "title": "t", "url": "https://example.com/invented"},
        ],
    })
    monkeypatch.setattr(dossier, "converse", lambda *a, **k: modes.Turn(
        mode="commander-dossier", model="m", stop_reason="end_turn",
        text=payload, tool_calls=[], input_tokens=1, output_tokens=1,
        searched=[{"url": "https://edhrec.com/real", "title": "t"}]))

    body = dossier.ask("mini", requested="consultant", source=source)["dossier"]
    assert body["sources_dropped"] == 1
    assert body["rivals_dropped"] == 1
    assert [s["id"] for s in body["sources"]] == ["s1"]
    # And the passage that cited the dropped source no longer points at it.
    assert body["who"]["source_ids"] == ["s1"]


def test_an_answer_that_does_not_parse_stores_nothing(corpus, source, monkeypatch):
    monkeypatch.setattr(dossier, "converse", lambda *a, **k: modes.Turn(
        mode="commander-dossier", model="m", stop_reason="max_tokens",
        text="{\"who\": {\"prose\": \"trunc", tool_calls=[],
        input_tokens=1, output_tokens=1))
    report = dossier.ask("mini", requested="consultant", source=source)
    assert report["dossier"] == {}
    assert "did not parse" in report["reason"]
    assert dossier.stored() == []


# ------------------------------------------------------------- the stance

def test_the_stance_being_off_makes_no_call(corpus, source, no_network):
    """`no_network` is the assertion: reaching the API here fails the test."""
    report = dossier.ask("mini", requested="off", source=source)
    assert report["asked"] is False
    assert "stance is off" in report["reason"]
    assert report["dossier"] == {}


def test_a_stored_dossier_is_served_even_at_stance_off(corpus, source,
                                                       no_network):
    """Off means no calls, not a feature that hides what already exists.

    Reading a row somebody else's call produced costs nothing and reaches
    nothing, so refusing it would be a different rule than the one ADR 15 set.
    """
    oracle_id = dossier.brief("mini", source=source)["card"]["oracle_id"]
    assert oracle_id, "the tiny corpus should carry Gyome's oracle id"
    dossier.put(dossier.cache_key(oracle_id), oracle_id=oracle_id,
                commander="Gyome, Master Chef",
                result={"who": {"prose": "stored", "source_ids": []}})

    report = dossier.ask("mini", requested="off", source=source)
    assert report["cached"] is True
    assert report["asked"] is False, "a cache hit made no call and must say so"
    assert report["dossier"]["who"]["prose"] == "stored"


# --------------------------------------------------------------- the cache

def test_the_key_is_the_commander_not_the_deck():
    """Two decks led by Gyome are two lists and one Gyome."""
    assert dossier.cache_key("abc") == dossier.cache_key("abc")
    assert dossier.cache_key("abc") != dossier.cache_key("def")


def test_no_oracle_id_disables_caching_rather_than_colliding():
    """An empty key is a miss without touching the database. The alternative
    is every uncatalogued commander sharing one row."""
    assert dossier.cache_key("") == ""
    assert dossier.get("") is None


def test_editing_the_prompt_invalidates_stored_dossiers(monkeypatch):
    """A stored dossier was written under a particular prompt and schema.
    Serving it after either moves is serving text written to answer a
    different question."""
    before = dossier.cache_key("abc")
    monkeypatch.setattr(dossier, "INSTRUCTIONS",
                        dossier.INSTRUCTIONS + "\n- And one more thing.")
    assert dossier.cache_key("abc") != before


def test_changing_the_model_invalidates_them_too(monkeypatch):
    before = dossier.cache_key("abc")
    monkeypatch.setattr(client, "model", lambda: "claude-opus-5")
    assert dossier.cache_key("abc") != before


def test_a_dossier_round_trips_through_the_store(corpus):
    key = dossier.cache_key("oracle-1")
    dossier.put(key, oracle_id="oracle-1", commander="Gyome, Master Chef",
                result={"who": {"prose": "hello", "source_ids": []}})
    hit = dossier.get(key)
    assert hit["result"]["who"]["prose"] == "hello"
    assert hit["created_at"]
    assert [r["commander"] for r in dossier.stored()] == ["Gyome, Master Chef"]
    assert dossier.clear() == 1
    assert dossier.get(key) is None


def test_a_broken_store_is_a_miss_and_never_an_exception(corpus, monkeypatch):
    """A cache that can fail the feature is worse than no cache — the same
    rule `sim/cache.py` holds, for the same reason."""
    import sqlite3

    def explode(*a, **k):
        raise sqlite3.OperationalError("database is locked")
    monkeypatch.setattr("mtglab.auth.db.connection", explode)
    assert dossier.get(dossier.cache_key("x")) is None
    dossier.put(dossier.cache_key("x"), oracle_id="x", commander="c", result={})
    assert dossier.stored() == []


# ------------------------------------------------------------ no commander

def test_a_deck_with_no_commander_is_refused_clearly(tmp_path, corpus):
    yaml = DECK_YAML.replace("commander:\n  - Gyome, Master Chef\n",
                             "commander: []\n")
    path = tmp_path / "headless" / "deck.yaml"
    path.parent.mkdir()
    path.write_text(yaml, encoding="utf-8")
    # The slug comes from the file, not the directory: `mini`, as declared.
    src = MemoryDeckSource([Deck.load(path)])
    with pytest.raises(dossier.NoCommander):
        dossier.ask("mini", source=src)


def test_the_scope_axis_changes_the_prompt_and_the_tools_never_move():
    """ADR 15 again: a stance widens what a mode does, never what it may do."""
    mode = dossier.COMMANDER_DOSSIER
    prompts = {s: mode.system(stance.Stance("on-request", s, "none"))
               for s in stance.SCOPE}
    assert len(set(prompts.values())) == 3
    assert len({tuple(mode.tool_names) for _ in stance.SCOPE}) == 1
