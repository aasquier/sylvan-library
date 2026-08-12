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

**Two card tools, and the split is load-bearing.** `get_cards` looks names up;
`search_cards` finds candidates. They are not interchangeable, because
`search_cards` filters to `legalities.commander = 'legal'` -- correct when the
question is "what could I play", wrong when it is "what is this card", since it
makes **a banned card invisible**.

That was a real hole in rule 1, found by running a turn rather than by
reasoning about one: asked what the two cards failing the gate do, a first turn
could not look up Emrakul, the Aeons Torn or Primeval Titan, said the corpus
had nothing, and answered from labelled recall. ADR 14's third boundary
working; its first boundary not. `get_cards` closes it -- no filters at all,
`legal_commander` reported per card, so a banned card returns its real oracle
text *and* its ban status.

So: a claim about a **named** card goes through `get_cards`, always.
`search_cards` is for the question "what else is out there", and its
description says so.
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
            name="get_cards",
            fn=service.cards_named,
            required=("names",),
            properties={
                "names": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": ("Exact card names, up to 100. Case does "
                                    "not matter, and either face of a "
                                    "double-faced card resolves the whole "
                                    "card."),
                },
            },
            description=(
                "Look up named cards in the corpus and return what they "
                "actually do: oracle text, mana cost, mana value, type line, "
                "colour identity, keywords, power and toughness, loyalty, "
                "Commander legality, Game Changer status, Reserved List "
                "status and popularity rank. "
                "**Every claim you make about a specific card must come from "
                "this tool, including cards you are certain you remember.** "
                "Your recollection of a card is not evidence; this is. Colour "
                "identity in particular is the corpus's own field and accounts "
                "for back faces, so never infer it from a mana cost. "
                "Power and toughness are strings, because '*' and '1+*' are "
                "real printed values, and are the front face's on a "
                "double-faced card. A null power means the card is not a "
                "creature on its front face -- it does not mean the stats are "
                "unknown. "
                "Unlike search_cards this filters on nothing, so it is also "
                "the only way to read a banned card -- it comes back with its "
                "text and with legal_commander false. Names that do not "
                "resolve are listed in not_found: if a card you asked about is "
                "there, you do not know what it does, and saying so is the "
                "correct answer."),
        ),
        Tool(
            name="search_cards",
            fn=service.search_cards,
            required=("q",),
            properties={
                "q": {"type": "string",
                      "description": ("Matched against card name and rules "
                                      "text, e.g. 'destroy target creature' "
                                      "or 'Cat'.")},
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
                "Find candidate cards you cannot already name, filtered by "
                "rules text, colour identity, type, mana value and price. "
                "This is the discovery tool: use it for 'what else could go "
                "here', 'what green cards do X', 'cheaper alternatives to "
                "this'. It searches the whole history of the game, so deep "
                "cuts are reachable, not only staples. "
                "**It returns only Commander-legal cards, so it is not a "
                "lookup tool** -- a banned card will simply be missing, and "
                "absence here is not evidence a card does not exist. When you "
                "already know a card's name and want to know what it does, "
                "call get_cards instead."),
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
