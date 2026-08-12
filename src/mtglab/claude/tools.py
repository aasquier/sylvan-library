"""Tool schemas over the read-only half of `api/service.py`.

This is how [rule 1](../../../CLAUDE.md) stops being a request and starts being
a mechanism. A prompt saying "do not rely on your memory of card text" is not
enforcement -- a model asked what Arahbo does with no corpus access answers
from its weights, and that is precisely the failure `CLAUDE.md` records as
having already happened twice. A model that must call `search_cards` to learn
what a card does gets the corpus row as the fact, because there is nothing else
on the table.

**The registry is the capability set.** `READ_ONLY` below is the complete list
of service functions any Claude surface can reach, and `run()` refuses
everything else by name. That is deliberate:
[ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
gives every mode a declaration of what it may write, and the cheapest way to
keep that honest across modes nobody has written yet is for the *package* to
have no write door at all. A mode can subset this list. It cannot extend it
without editing this file, which is where the boundary test is looking.

So `add_card`, `remove_card`, `set_card_field`, `set_deck_field`, `set_note`,
`swap_card` and `import_deck` are absent, and their absence is the feature.
`set_card_field(field="why")` is the specific call ADR 15 names as the thing no
code path may reach; it is not reachable from here, and
`tests/test_claude_boundary.py` fails if that ever changes.

Reading a `why` is a different act and stays allowed -- `get_deck` returns
every rationale, because interrogating a card's slot means seeing what was
claimed for it. Argue about it; never author it.

**Known gap, and it is a hole in rule 1.** ADR 15's table names `get_cards` --
exact-name lookup -- as a tool of every mode, and there is no such function in
`api/service.py` to wrap: `db.get_cards` is reached only through the private
`_corpus_for`. Exact lookup therefore goes through `search_cards`, and
`search_cards` filters to `legalities.commander = 'legal'`, which is right for
a deckbuilding search and wrong for a fact lookup.

**A banned card is invisible to this tool set.** Measured, not theorised: asked
which decks fail the gate and what the flagged cards do, a first turn got the
gate's answer correctly and then could not look up either Emrakul, the Aeons
Torn or Primeval Titan -- the two deliberate failures in
`atla-palani-dinos` and `goreclaw-stompy`. It said so and labelled the fallback
as unverified recall, which is ADR 14's third boundary working. It still
answered from recall, which is the first boundary not working, on precisely the
two cards this project most needs to discuss.

`service.cards_named()` -- exact names, through `db.get_cards`, no legality
filter -- closes it, and should land before any mode ships.
`tests/test_claude_tools.py` pins the gap so it stays visible.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from mtglab.api import service
from mtglab.decks.source import DeckSource


class ToolNotAllowed(Exception):
    """A tool was requested that this package does not expose.

    Raised rather than returned so it cannot be mistaken for a tool result the
    model should reason about. The caller decides whether to hand the model an
    `is_error` result or abandon the turn.
    """


class ToolArgumentsRejected(Exception):
    """Arguments that do not match the schema, checked before dispatch."""


@dataclass(frozen=True)
class Tool:
    """One tool: a schema for the model, and the function it actually calls."""

    name: str
    fn: Callable[..., Any]
    description: str
    properties: dict[str, Any] = field(default_factory=dict)
    required: tuple[str, ...] = ()
    #: Deck-facing service functions take a `DeckSource` so nothing here reads
    #: the filesystem directly -- the same rule the routes follow, and what
    #: lets a second deck tier arrive as one dependency swap.
    takes_source: bool = False

    def schema(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "description": self.description,
            "input_schema": {
                "type": "object",
                "properties": self.properties,
                "required": list(self.required),
                # Not enforced by the API without `strict`, which is why
                # `run()` checks it again. Stated anyway so the model is told
                # the shape rather than corrected after the fact.
                "additionalProperties": False,
            },
        }


_SLUG = {"type": "string",
         "description": "The deck's slug, as returned by list_decks."}


# Descriptions are prescriptive about *when* to call, not only what the tool
# does. Recent models reach for tools conservatively, and an under-described
# tool is the most common reason a model answers from recall instead -- which
# in this codebase is the exact failure rule 1 exists to prevent.
READ_ONLY: dict[str, Tool] = {
    t.name: t for t in (
        Tool(
            name="list_decks",
            fn=service.list_decks,
            takes_source=True,
            description=(
                "List every deck in the library with its commander, colour "
                "identity, bracket, status (built or theoretical), stage "
                "(draft or curated), card counts, and the gate's error and "
                "warning counts. Call this first when the user names a deck "
                "loosely ('the cat deck', 'my cEDH list') to find its slug, "
                "or when asked what decks exist."),
        ),
        Tool(
            name="get_deck",
            fn=service.get_deck,
            takes_source=True,
            required=("slug",),
            properties={"slug": _SLUG},
            description=(
                "The full contents of one deck: every card with its category, "
                "quantity, corpus-verified card text, and the rationale "
                "('why') the user wrote for its slot. Call this before "
                "discussing any specific deck -- it is the only way to know "
                "what is actually in it and what the user claimed for each "
                "card. You may argue against a rationale; you may never write "
                "one."),
        ),
        Tool(
            name="validate_deck",
            fn=service.validate_deck,
            takes_source=True,
            required=("slug",),
            properties={"slug": _SLUG},
            description=(
                "Run the gate: format legality, banned cards, colour "
                "identity, singleton, deck size, companion and partner rules, "
                "and missing rationales. Returns errors and warnings with the "
                "card each concerns. This is deterministic and reproducible -- "
                "call it instead of judging any of those things yourself, and "
                "when you report its output say that it came from the gate."),
        ),
        Tool(
            name="deck_stats",
            fn=service.stats_for,
            takes_source=True,
            required=("slug",),
            properties={"slug": _SLUG},
            description=(
                "Deterministic counts for one deck: mana curve, macro "
                "category totals (ramp, draw, removal, lands...) against the "
                "targets for its bracket, and colour distribution. Call this "
                "before any claim about a deck being short on ramp, heavy at "
                "three mana, or light on interaction -- the numbers exist and "
                "guessing at them is how a wrong claim gets made confidently."),
        ),
        Tool(
            name="suggest_replacements",
            fn=service.suggestions_for,
            takes_source=True,
            required=("slug",),
            properties={
                "slug": _SLUG,
                "limit": {"type": "integer",
                          "description": "Candidates per flagged card. Default 5."},
            },
            description=(
                "For cards the gate flagged as banned or outside the "
                "commander's colour identity, a scored shortlist of legal "
                "replacements with the reasons each scored well. Only covers "
                "errors a different card would fix -- a missing rationale is "
                "not one of them. Call this when the user asks what to run "
                "instead of a flagged card. The shortlist is a starting "
                "point for the argument, not a decision: which card goes in "
                "is the user's call, and the swap needs a rationale only they "
                "can write."),
        ),
        Tool(
            name="search_cards",
            fn=service.search_cards,
            required=("q",),
            properties={
                "q": {"type": "string",
                      "description": ("Matched against card name and rules "
                                      "text. Pass a card's exact full name to "
                                      "look that card up.")},
                "identity": {"type": "string",
                             "description": ("Colour identity filter as WUBRG "
                                             "letters, e.g. 'GW'. A subset "
                                             "filter: 'BG' returns everything "
                                             "legal in a Golgari deck, "
                                             "including colourless.")},
                "type_line": {"type": "string",
                              "description": "Substring of the type line, e.g. 'Instant'."},
                "cmc_max": {"type": "number",
                            "description": "Maximum mana value."},
                "price_max": {"type": "number",
                              "description": "Maximum price in USD, cheapest printing."},
                "sort": {"type": "string",
                         "enum": ["edhrec", "cmc", "name", "newest"],
                         "description": "Ordering. Default 'edhrec' (most played first)."},
                "limit": {"type": "integer",
                          "description": "Rows to return, capped at 200. Default 60."},
            },
            description=(
                "Search the Scryfall card corpus -- every Commander-legal "
                "card, with oracle text, mana cost, colour identity, type "
                "line, mana value, popularity rank and price. "
                "**Call this for every claim about what a card does or costs, "
                "including cards you are confident you remember.** Card text "
                "is checked, never recalled: the colour identity and the "
                "oracle text this returns are the facts, and your memory of "
                "them is not. Also the tool for finding cards -- it searches "
                "the whole history of the game, so deep cuts are reachable, "
                "not just staples."),
        ),
    )
}


def schemas(names: tuple[str, ...] | None = None) -> list[dict[str, Any]]:
    """Tool definitions for the `tools` parameter of a Messages request.

    `names` subsets the registry, which is how a mode declares a narrower tool
    set than the package offers. It can only ever narrow: an unknown name is a
    `ToolNotAllowed`, not a silently ignored entry.
    """
    chosen = READ_ONLY.keys() if names is None else names
    for name in chosen:
        if name not in READ_ONLY:
            raise ToolNotAllowed(f"no such tool: {name!r}")
    # Sorted so the rendered `tools` block is byte-identical between requests.
    # Tools render first in the prompt, so an unstable order would invalidate
    # the prompt cache on every turn -- for free, and invisibly.
    return [READ_ONLY[n].schema() for n in sorted(chosen)]


def run(name: str, arguments: dict[str, Any] | None = None, *,
        source: DeckSource | None = None) -> Any:
    """Execute one tool call from a model response.

    The allowlist check happens here, on the name the model actually sent,
    rather than being assumed from what was advertised in `tools`. A model can
    ask for anything; this function is what decides that asking for
    `set_card_field` gets a refusal instead of a write.
    """
    tool = READ_ONLY.get(name)
    if tool is None:
        raise ToolNotAllowed(
            f"{name!r} is not an available tool. This surface is read-only: "
            f"available tools are {', '.join(sorted(READ_ONLY))}.")

    args = dict(arguments or {})
    unknown = sorted(set(args) - set(tool.properties))
    if unknown:
        raise ToolArgumentsRejected(
            f"{name}: unexpected argument(s) {', '.join(unknown)}")
    missing = [r for r in tool.required if args.get(r) in (None, "")]
    if missing:
        raise ToolArgumentsRejected(
            f"{name}: missing required argument(s) {', '.join(missing)}")

    # Drop nulls rather than forwarding them: the service functions distinguish
    # "not filtering on price" (None) from a filter, and their own defaults say
    # so more clearly than an explicit None from a model would.
    call = {k: v for k, v in args.items() if v is not None}
    if tool.takes_source:
        call["source"] = source
    return tool.fn(**call)
