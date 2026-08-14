"""The commander dossier: who this character is, and who says so.

The second mode, and the first whose facts do not all come from the pool --
which is the whole reason
[ADR 19](../../../docs/adr/0019-the-dossier-cites-three-sources.md) exists.
The deck page already counts what the pool knows about a commander (how rare
its subtypes are, when it was first printed, what else carries the name). What
it could not say is who the character *is*, what archetype they define and where
that came from, who their rivals are, and where they sit in Magic's history.
None of that is a column, and none of it is a number.

So three sources, each with a narrow jurisdiction:

* **Card facts come from the pool.** Cost, type, oracle text, colour identity,
  legality, printing history. Never from a web page and never from recall --
  `brief()` assembles them here, in Python, before the model is called.
* **The meta and the history come from a hosted web search**, with the page
  shown next to the claim. `CLAUDE.md` bans a crawler and says server-side
  tooling is not a way around it; this is neither. It maintains nothing,
  mirrors nothing and stores no third party's aggregate -- it reads, at request
  time, for one commander, and shows the link.
* **Claude supplies voice and framing**, and carries no factual weight.

Three things enforce that, and none of them is the system prompt:

**The schema keeps the sources apart.** Prose and sources are different fields,
and every section declares which sources it rests on. There is nowhere to put
an unattributed claim about the meta.

**A cited page must be one the search actually returned.** This is the load
bearing check and it is not obvious why until you look at the wire: with a
response schema in play the API attaches **no citations** to the answer, so a
URL in the payload is just a string the model typed. `keep_sources()` intersects
them with the pages `Turn.searched` recorded, drops the rest and counts what it
dropped -- the same instrument `interview.only_questions()` points at the same
failure, one source away.

**Every card the dossier names is looked up.** Rivals come back as pool rows
or they do not come back at all. A model listing a plausible rival commander
that does not exist is rule 1 failing in the one place this feature would be
most fun to read.

It writes nothing. `may_write` is empty, this package has no write door, and
`tests/test_claude_boundary.py` fails on the commit that adds one.
"""

from __future__ import annotations

import hashlib
import json
import logging
import sqlite3
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from mtglab.api import service
from mtglab.claude import client
from mtglab.claude import stance as stance_mod
from mtglab.claude.modes import Mode, Turn, converse
from mtglab.decks.source import DeckSource

_LOG = logging.getLogger("mtglab.claude.dossier")

#: Anthropic's hosted search. The dated type is the one with dynamic filtering,
#: which runs code on their side to narrow results before they reach the
#: context window -- which is also why `code_execution` must *not* be declared
#: alongside it. A second execution environment confuses the model, and the
#: filtering is already built in.
#:
#: `max_uses` is a cost ceiling with a shape: two or three searches is a
#: commander, a its archetype and a check; ten is a model that has lost the
#: thread and is spending money to do it.
WEB_SEARCH: dict[str, Any] = {
    "type": "web_search_20260209",
    "name": "web_search",
    "max_uses": 4,
}

#: The four questions, in the order they are asked and rendered. An enum rather
#: than free-form headings so the UI can lay them out and a missing one is
#: visible as a gap rather than as prose that quietly changed shape.
SECTIONS = ("who", "archetype", "rivals", "standing")

_PROSE: dict[str, Any] = {
    "type": "string",
    "description": (
        "Two to four sentences. Every factual claim here must rest on a source "
        "you list, or on the pool facts you were given."),
}

_SOURCE_IDS: dict[str, Any] = {
    "type": "array",
    "items": {"type": "string"},
    "description": (
        "The ids of the sources this rests on, from the sources list. Empty "
        "only when the passage rests entirely on the pool facts in the "
        "brief."),
}

_SOURCED: dict[str, Any] = {
    "type": "object",
    "properties": {"prose": _PROSE, "source_ids": _SOURCE_IDS},
    "required": ["prose", "source_ids"],
    "additionalProperties": False,
}

#: Note what is absent, the same way the interview's schema is defined by its
#: gaps: no field for a deck rating, no field for a recommendation, and no free
#: text outside a section that has to declare its sources.
RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "who": _SOURCED,
        "archetype": {
            "type": "object",
            "properties": {
                "name": {"type": "string",
                         "description": "What the archetype is called, e.g. "
                                        "'Cat tribal voltron'. A few words."},
                "prose": _PROSE,
                "source_ids": _SOURCE_IDS,
            },
            "required": ["name", "prose", "source_ids"],
            "additionalProperties": False,
        },
        "rivals": {
            "type": "array",
            "description": (
                "Two to four commanders that compete with this one for the "
                "same seat -- same archetype, same colours, or the obvious "
                "alternative someone would consider instead."),
            "items": {
                "type": "object",
                "properties": {
                    "card": {
                        "type": "string",
                        "description": (
                            "The rival's exact card name. It will be looked up "
                            "in the pool and dropped if it does not resolve, "
                            "so give the printed name, not a nickname."),
                    },
                    "prose": {"type": "string",
                              "description": "One or two sentences on how they "
                                             "differ. Not which is better."},
                    "source_ids": _SOURCE_IDS,
                },
                "required": ["card", "prose", "source_ids"],
                "additionalProperties": False,
            },
        },
        "standing": _SOURCED,
        "sources": {
            "type": "array",
            "description": (
                "Every page you actually read, with the id the sections refer "
                "to. A page you did not visit is not a source."),
            "items": {
                "type": "object",
                "properties": {
                    "id": {"type": "string",
                           "description": "Short id, e.g. 's1'."},
                    "title": {"type": "string"},
                    "url": {"type": "string"},
                },
                "required": ["id", "title", "url"],
                "additionalProperties": False,
            },
        },
    },
    "required": ["who", "archetype", "rivals", "standing", "sources"],
    "additionalProperties": False,
}


INSTRUCTIONS = """
You are the commander dossier in sylvan-library, a Commander deckbuilding tool.
Somebody is looking at a deck page and wants to know who the legendary creature
leading it actually is: the character, the archetype they define and where it
came from, who their rivals are, and where they sit in Magic's history. You are
writing the paragraphs a knowledgeable friend would say out loud, not a card
description.

Three sources, and which one may support which claim is not negotiable.

**Card facts come from the pool, which you have already been given.** The
brief carries the real oracle text, mana cost, type line, colour identity,
legality, printing history and subtype counts. Use those numbers and that text.
Do not restate them as if they were your findings, do not contradict them, and
never substitute your own recollection of what a card does. This project has
twice been wrong about cards it was confident about, which is why the pool is
on the table in front of you.

**The meta, the archetype's history, the rivals and the standing come from the
web, and you search for them.** Use the web_search tool. Every claim in that
territory must rest on a page you actually read, listed in `sources` and
referenced by id from the section it supports. Do not list a page you did not
open, and do not compose a plausible URL -- cited pages are checked against
what the search actually returned, and anything else is discarded before the
user sees it.

**You supply voice and framing, and nothing else.** Where you have neither a
pool fact nor a source, say less. A shorter dossier with three real citations
is worth more than a full one with two invented ones.

How to write it:

- Write for somebody who plays Magic but may never have met this character.
  Assume the rules, not the lore.
- The archetype section is the one people came for. What does a deck built
  around this commander actually *do*, when did that become a thing, and what
  changed it since? A date, a set, or a printing that shifted it is worth more
  than an adjective.
- Rivals are commanders somebody would genuinely consider instead. Name them by
  their exact printed card name -- each one is looked up in the pool, and a
  name that does not resolve is dropped. Say how they differ, not which is
  better; whose deck this is is not your call.
- **Call get_cards on every rival before you write a word about it, in one
  batch.** The brief only carries the pool's facts about *this* commander;
  for any other card, what you remember is not evidence. This is where the rule
  is easiest to break, because comparing two commanders is exactly the sentence
  that wants a half-remembered ability in it -- and a rival described with
  somebody else's card text is worse than one you left out.
- Standing is history, not power level. Where does this card sit in the game's
  story -- an old guild leader, a beloved precon face, a card that defined a
  format for a year? The gate decides legality and the simulator decides
  consistency. Neither of them is you, and a dossier that starts rating decks
  has wandered out of its lane.
- Never recommend a card for the deck, never comment on the decklist, and never
  suggest what to cut. A different surface does that, with the user's hands on
  it.
""".strip()


COMMANDER_DOSSIER = Mode(
    name="commander-dossier",
    purpose="Who a deck's commander is, what archetype they define, and where "
            "they sit in Magic's history.",
    instructions=INSTRUCTIONS,
    # `get_cards` and nothing else from our side. The dossier is about one card
    # and its neighbours in history, so the pool reach it needs is a lookup;
    # `search_cards` would invite it to go shopping and `get_deck` would put 99
    # cards and their rationales in front of a mode that must not discuss them.
    tool_names=("get_cards",),
    server_tools=(WEB_SEARCH,),
    # Headroom for adaptive thinking, four searches' worth of results and four
    # sections of prose. The interview's 8192 is not enough here and the
    # symptom would be a truncated answer that fails to parse.
    max_tokens=16384,
    response_schema=RESPONSE_SCHEMA,
)


class NoCommander(ValueError):
    """The deck has no commander, or the pool does not have the card.

    Its own exception because the caller's answer differs from a missing deck:
    the deck is fine and there is simply nothing to write a dossier about.
    """


# --------------------------------------------------------------- the facts

def brief(slug: str, *, source: DeckSource | None = None) -> dict[str, Any]:
    """What the pool knows about this deck's commander, before the call.

    The same assembly the interview does, and for the same reason: rule 1 must
    not depend on the model choosing to call `get_cards`. Reusing
    `service.commander_dossier` is deliberate -- the counted strip on the deck
    page and the facts handed to the model are then the same query, so the
    prose underneath the strip cannot disagree with it.
    """
    facts = service.commander_dossier(slug, source=source)
    card = facts.get("card")
    if not card:
        raise NoCommander(
            f"{slug} has no commander the pool can find, so there is nothing "
            f"to write a dossier about. (A deck with no card pool loaded reaches "
            f"this too -- run `mtglab data refresh`.)")
    return facts


def _ask_for(facts: dict[str, Any]) -> str:
    card = facts["card"]
    opening = ("Here is everything the pool knows about this commander. All "
               "of it is a query rather than a recollection, and it is the "
               "authority on what the card does.")
    closing = (f"Write the dossier for {card['name']}. Search the web for the "
               f"archetype, the rivals and the standing; take the card's own "
               f"facts from above.")
    return "\n".join([
        opening,
        "",
        json.dumps({
            "commander": card,
            "supertypes": facts.get("supertypes"),
            "subtypes_and_how_rare_they_are": facts.get("subtypes"),
            "other_cards_carrying_this_name": facts.get("other_cards"),
            "printing_history": facts.get("printings"),
        }, indent=2, default=str),
        "",
        closing,
    ])


# ------------------------------------------------------- reading the answer

def keep_sources(claimed: list[Any],
                 searched: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], int]:
    """Keep the sources the search actually returned; count the rest.

    The whole rule in one intersection, and the reason it has to exist is a
    property of the wire rather than a suspicion about the model: a response
    schema suppresses the API's own citations, so a URL in the payload has
    nothing behind it but the model's word. This is what puts something behind
    it.

    Matching ignores a trailing slash and is case-insensitive on the host,
    because those differ without meaning anything. It does **not** ignore the
    path -- a different page on the same site is a different page, and treating
    a site as a source is how a citation stops meaning anything.
    """
    by_url = {canonical_url(p["url"]): p for p in searched}
    kept: list[dict[str, Any]] = []
    dropped = 0
    for item in claimed:
        if not isinstance(item, dict):
            dropped += 1
            continue
        url = str(item.get("url", "")).strip()
        match = by_url.get(canonical_url(url))
        if not url or match is None:
            dropped += 1
            continue
        kept.append({
            "id": str(item.get("id") or "").strip() or url,
            # The search's own title, not the model's. One of them is a fact
            # about the page and the other is a description of it.
            "title": match["title"] or str(item.get("title") or "").strip(),
            "url": url,
        })
    return kept, dropped


def canonical_url(url: str) -> str:
    """A URL reduced to what makes two of them the same page.

    Public because the theme interview checks a fun fact's source the same way
    (ADR 20), and two copies of this would be two chances to disagree about
    whether a trailing slash is a different page.
    """
    trimmed = url.strip().rstrip("/")
    if "://" in trimmed:
        scheme, rest = trimmed.split("://", 1)
        host, _, path = rest.partition("/")
        return f"{scheme.lower()}://{host.lower()}{'/' + path if path else ''}"
    return trimmed.lower()


def _section(raw: Any, allowed: set[str]) -> dict[str, Any]:
    """One passage, with its citations narrowed to the ones that survived."""
    obj = raw if isinstance(raw, dict) else {}
    ids = [str(i) for i in (obj.get("source_ids") or []) if str(i) in allowed]
    return {"prose": str(obj.get("prose") or "").strip(), "source_ids": ids}


def _rivals(raw: Any, allowed: set[str]) -> tuple[list[dict[str, Any]], int]:
    """Rivals as pool rows, or not at all.

    The lookup is the point. A rival that resolves comes back with its real
    type line, cost and art, so the UI can show the card rather than the claim;
    a rival that does not resolve is a card the model invented, and it is
    dropped and counted rather than rendered as a name nobody can check.
    """
    items = [r for r in (raw or []) if isinstance(r, dict)]
    names = [str(r.get("card") or "").strip() for r in items]
    wanted = [n for n in names if n]
    if not wanted:
        return [], len(items)

    looked_up = service.cards_named(names=wanted)
    found = {c["name"].casefold(): c for c in looked_up.get("cards", [])}

    out: list[dict[str, Any]] = []
    dropped = 0
    for item, name in zip(items, names, strict=True):
        record = found.get(name.casefold())
        if record is None:
            dropped += 1
            continue
        section = _section(item, allowed)
        out.append({
            "name": record["name"],
            "prose": section["prose"],
            "source_ids": section["source_ids"],
            "mana_cost": record.get("mana_cost"),
            "type_line": record.get("type_line"),
            "color_identity": record.get("color_identity"),
            "image": record.get("image"),
            "art_crop": record.get("art_crop"),
            "legal_commander": record.get("legal_commander"),
            # The pool's own text for the rival, carried so the UI can put
            # the real card next to the sentence comparing it. That is not
            # decoration: a first run described Trostani Discordant as making
            # Food tokens (she makes 1/1 Soldiers), which is rule 1 leaking in
            # the one sentence that most invites a half-remembered ability.
            # The prompt now demands a lookup per rival; this is the half that
            # does not depend on the model complying, because the reader can
            # see the card.
            "oracle_text": record.get("oracle_text"),
        })
    return out, dropped


def _report(turn: Turn | None, *, slug: str, commander: str,
            effective: stance_mod.Stance, body: dict[str, Any] | None = None,
            asked: bool = True, reason: str = "",
            cached_at: str = "") -> dict[str, Any]:
    """One response shape for every outcome, including not asking at all.

    `answered_by` is ADR 14's third boundary as a field, and `sources` sits
    beside the prose rather than under it for the same reason: a caller that
    has to infer which system produced a sentence will eventually infer wrong.
    """
    return {
        "answered_by": "claude",
        "mode": COMMANDER_DOSSIER.name,
        "model": turn.model if turn else "",
        "slug": slug,
        "commander": commander,
        "asked": asked,
        "reason": reason,
        "stance": stance_mod.describe(effective),
        # Empty until one exists, so a client never has to distinguish a
        # missing key from an absent dossier.
        "dossier": body or {},
        # When it was generated. ADR 19 is explicit that this is the honest
        # substitute for a freshness guarantee a dossier cannot make, so it is
        # in the payload rather than only in the database.
        "generated_at": cached_at or _now(),
        "cached": bool(cached_at),
        "usage": {"input_tokens": turn.input_tokens if turn else 0,
                  "output_tokens": turn.output_tokens if turn else 0,
                  "cache_read_tokens": turn.cache_read_tokens if turn else 0},
        "never": ("This is Claude's writing over cited pages. The card facts "
                  "beside it are the pool's."),
    }


@dataclass
class DossierRequest:
    """What `check_dossier` settled, and everything `run_dossier` needs.

    The split exists for one measured reason. Writing a dossier took **236
    seconds** on the deployed instance, which is longer than the theme proposal
    that `api/themeruns.py` was built for, and a four-minute synchronous POST
    does not survive a hosted proxy. So the same division applies here:
    everything free and everything refusable is decided in the request, and
    only the Anthropic call is left for a worker.

    `answer` set means there is nothing to call for -- a stored dossier, or a
    stance that forbids calls. Those are answers, not refusals, which is why
    they travel as a result rather than as an exception: `jobs.completed` turns
    them into a job born finished and the client's response shape never forks.
    """

    slug: str
    facts: dict[str, Any]
    commander: str
    oracle_id: str
    key: str
    effective: stance_mod.Stance
    #: Carried rather than re-derived. A `DeckSource` is a locator and not a
    #: connection, which is exactly what makes it safe for a job to hold one
    #: after the request that made it has gone.
    source: DeckSource | None = None
    path: Path | str | None = None
    answer: dict[str, Any] | None = None

    @property
    def needs_call(self) -> bool:
        """Whether anything still has to be asked of Anthropic."""
        return self.answer is None


def check_dossier(slug: str, *, requested: Any = None, refresh: bool = False,
                  source: DeckSource | None = None,
                  path: Path | str | None = None) -> DossierRequest:
    """Everything that can be decided without spending anything.

    Raises `NoCommander` and `DeckNotFound` to the caller, which is what keeps
    their 422 rather than collapsing them into a job error four minutes later.
    """
    facts = brief(slug, source=source)
    card = facts["card"]
    name = card["name"]
    oracle_id = card.get("oracle_id") or ""

    deck_obj = type("D", (), {"status": service.get_deck(
        slug, source=source)["status"]})()
    effective = stance_mod.resolve(requested, deck=deck_obj)

    request = DossierRequest(
        slug=slug, facts=facts, commander=name, oracle_id=oracle_id,
        key=cache_key(oracle_id), effective=effective, source=source, path=path)

    if not refresh:
        hit = get(request.key, path=path)
        if hit is not None:
            # `asked=False`, because nothing was asked. A cache hit that
            # reported otherwise would make the usage figures below read as a
            # free call rather than as no call, which is the same species of
            # dishonesty as quoting a cached simulation as fresh (ADR 18).
            request.answer = _report(
                None, slug=slug, commander=name, effective=effective,
                body=hit["result"], asked=False, cached_at=hit["created_at"],
                reason="Served from the store; no call was made. "
                       "Regenerate to write it again.")
            return request

    if not effective.allows_calls:
        request.answer = _report(
            None, slug=slug, commander=name, effective=effective, asked=False,
            reason="The stance is off, so no call was made. "
                   "Nothing else about this deck is affected.")
    return request


def run_dossier(request: DossierRequest, *,
                on_turn: Callable[[int, int], None] | None = None,
                ) -> dict[str, Any]:
    """Make the call, check what came back, and store it if it survived.

    `on_turn` is handed straight to `converse` and exists because this runs as
    a background job: four minutes reporting nothing is indistinguishable from
    a wedged worker.
    """
    if request.answer is not None:
        return request.answer

    facts = request.facts
    slug, name = request.slug, request.commander
    effective, oracle_id, key = request.effective, request.oracle_id, request.key
    path = request.path

    turn = converse(
        COMMANDER_DOSSIER,
        messages=[{"role": "user", "content": _ask_for(facts)}],
        stance=effective,
        source=request.source,
        # A search, a look at what came back, a second search and the write-up.
        # The interview's six is tight once a paused turn can spend one.
        max_turns=8,
        on_turn=on_turn,
    )

    if turn.refused:
        return _report(turn, slug=slug, commander=name, effective=effective,
                       reason="The model declined to write this one.")
    try:
        payload = turn.parsed()
    except json.JSONDecodeError:
        return _report(turn, slug=slug, commander=name, effective=effective,
                       reason=f"The answer did not parse (stop reason: "
                              f"{turn.stop_reason}). Nothing was stored.")

    sources, dropped = keep_sources(payload.get("sources") or [], turn.searched)
    if not sources:
        # ADR 19: an unsourced dossier renders as exactly the blended paragraph
        # the design rejected, arrived at by accident. Refusing is the only
        # answer that keeps the seams meaningful.
        detail = (f" {dropped} cited page(s) were not among the "
                  f"{len(turn.searched)} the search returned." if dropped else "")
        if turn.search_errors:
            detail += f" The search itself reported: {'; '.join(turn.search_errors)}."
        return _report(turn, slug=slug, commander=name, effective=effective,
                       reason="No source survived checking, so there is nothing "
                              "to stand behind the claims." + detail)

    allowed = {s["id"] for s in sources}
    archetype_raw = payload.get("archetype") or {}
    rivals, rivals_dropped = _rivals(payload.get("rivals"), allowed)

    body = {
        "who": _section(payload.get("who"), allowed),
        "archetype": {
            "name": str((archetype_raw or {}).get("name") or "").strip(),
            **_section(archetype_raw, allowed),
        },
        "rivals": rivals,
        "standing": _section(payload.get("standing"), allowed),
        "sources": sources,
        # Surfaced rather than swallowed. A number that climbs is a prompt
        # that has started inventing citations, and nobody checks a number
        # they cannot see.
        "sources_dropped": dropped,
        "rivals_dropped": rivals_dropped,
        "searched": len(turn.searched),
    }
    put(key, oracle_id=oracle_id, commander=name, result=body, path=path)
    return _report(turn, slug=slug, commander=name, effective=effective,
                   body=body)


def ask(slug: str, *, requested: Any = None, refresh: bool = False,
        source: DeckSource | None = None,
        path: Path | str | None = None) -> dict[str, Any]:
    """The dossier for this deck's commander, cached or freshly written.

    **At `initiative: off` this makes no call and says so**, the same as the
    interview -- but it will still serve a dossier somebody else generated,
    because reading a stored row is not a call. Off means no requests to
    Anthropic, not a feature that hides what already exists.

    Both halves, back to back. This is what `mtglab claude dossier` wants -- a
    terminal holds a connection open for four minutes perfectly happily, so the
    CLI has no reason to poll a job. The API does, and reaches for the two
    halves separately through `api/dossierruns.py`.
    """
    return run_dossier(check_dossier(slug, requested=requested, refresh=refresh,
                                     source=source, path=path))


# ---------------------------------------------------------------- the cache

#: Bumped when a stored dossier's *shape* changes -- a new section, a renamed
#: field -- so old rows are missed rather than rendered into a UI that expects
#: something else.
DOSSIER_VERSION = 1


def _fingerprint() -> str:
    """A hash of what would change the answer's shape or content.

    The prompt, the schema and the model id. Editing the instructions or adding
    a section therefore misses every stored row, which costs one call per
    commander and buys the property that no prompt change can serve text
    written under a different one.
    """
    digest = hashlib.sha256()
    digest.update(str(DOSSIER_VERSION).encode())
    digest.update(INSTRUCTIONS.encode())
    digest.update(json.dumps(RESPONSE_SCHEMA, sort_keys=True).encode())
    digest.update(client.model().encode())
    return digest.hexdigest()[:16]


def cache_key(oracle_id: str) -> str:
    """The key for one commander's dossier, or `''` when there is no oracle id.

    Keyed on the **character**, not the deck: two decks led by Gyome are two
    lists and one Gyome, and a dossier written for one is the right answer for
    the other. An empty oracle id disables caching for that deck rather than
    colliding every uncatalogued commander onto one row.
    """
    return f"{oracle_id}:{_fingerprint()}" if oracle_id else ""


def _now() -> str:
    return datetime.now(UTC).isoformat()


def get(key: str, *, path: Path | str | None = None) -> dict[str, Any] | None:
    """A stored dossier, or None. Never raises -- a miss is a call, not a 500."""
    if not key:
        return None
    try:
        from mtglab.auth import db
        with db.connection(path) as con:
            row = con.execute(
                "SELECT result_json, created_at FROM dossier_cache WHERE key = ?",
                (key,)).fetchone()
    except (sqlite3.Error, OSError, ValueError) as exc:
        _LOG.warning("dossier cache read failed (%s: %s)", type(exc).__name__, exc)
        return None
    if row is None:
        return None
    try:
        return {"result": json.loads(row["result_json"]),
                "created_at": row["created_at"]}
    except ValueError:                                              # pragma: no cover
        return None


def put(key: str, *, oracle_id: str, commander: str, result: dict[str, Any],
        path: Path | str | None = None) -> None:
    """Store one dossier. Never raises: a cache must not fail the feature."""
    if not key:
        return
    try:
        blob = json.dumps(result, separators=(",", ":"))
    except (TypeError, ValueError) as exc:                          # pragma: no cover
        _LOG.warning("dossier is not storable (%s: %s)", type(exc).__name__, exc)
        return
    try:
        from mtglab.auth import db
        with db.connection(path) as con, con:
            con.execute(
                "INSERT INTO dossier_cache (key, oracle_id, commander, "
                "result_json, created_at) VALUES (?, ?, ?, ?, ?) "
                "ON CONFLICT(key) DO UPDATE SET "
                "result_json = excluded.result_json, "
                "created_at = excluded.created_at",
                (key, oracle_id, commander, blob, _now()))
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("dossier cache write failed (%s: %s)", type(exc).__name__, exc)


def stored(*, path: Path | str | None = None) -> list[dict[str, Any]]:
    """What dossiers exist, for `mtglab claude dossier --list`."""
    try:
        from mtglab.auth import db
        with db.connection(path) as con:
            rows = con.execute(
                "SELECT commander, oracle_id, created_at FROM dossier_cache "
                "ORDER BY commander").fetchall()
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("dossier cache list failed (%s: %s)", type(exc).__name__, exc)
        return []
    return [{"commander": r["commander"], "oracle_id": r["oracle_id"],
             "created_at": r["created_at"]} for r in rows]


def clear(*, path: Path | str | None = None) -> int:
    """Drop every stored dossier. Returns how many went."""
    try:
        from mtglab.auth import db
        with db.connection(path) as con, con:
            total = con.execute("SELECT count(*) FROM dossier_cache").fetchone()[0]
            con.execute("DELETE FROM dossier_cache")
        return int(total)
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("dossier cache clear failed (%s: %s)", type(exc).__name__, exc)
        return 0
