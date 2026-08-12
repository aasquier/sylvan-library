"""The rule that does not bend: no model response reaches a card's `why`.

[Rule 4](../CLAUDE.md) says every card carries a rationale the user wrote.
[ADR 8](../docs/adr/0008-the-gate-blocks.md) made that a gate rather than a
convention. [ADR 14](../docs/adr/0014-python-decides-claude-advises.md) kept it
intact against a language model -- Claude may argue about a `why`, never author
one -- and [ADR 15](../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
named the assertion in code that has to keep passing:

> no code path passes a model response into `set_card_field(field="why")`

This file is that assertion. It is written against the package's syntax tree
rather than against its behaviour on purpose, because the failure it guards
against is not a bug in code that exists -- it is the code somebody adds later.
"Tidy that rationale up" is one helpful-looking commit away at all times, and a
test that only exercises today's call paths would pass right through it.

So the check is structural: **whatever is under `src/mtglab/claude/` may not so
much as name a write function.** That fails on the commit that imports one,
before any prompt is written, before a mode exists to call it, and regardless
of how the call is guarded. A future ADR can decide to move this line. Nothing
else should be able to move it by accident.

No network, no key, no tokens. This must run everywhere, every time.
"""

import ast
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.api import service  # noqa: E402
from mtglab.claude import tools  # noqa: E402

CLAUDE_PKG = Path(__file__).resolve().parents[1] / "src" / "mtglab" / "claude"

#: Everything in `api/service.py` that changes a deck on disk. `set_card_field`
#: is the one ADR 15 names -- it is the only route to the `why` field -- but the
#: whole write surface is listed, because a mode that can call `swap_card` can
#: launder a rationale through its `why` argument just as effectively.
WRITE_SURFACE = frozenset({
    "add_card", "remove_card", "set_card_field", "set_deck_field",
    "set_note", "swap_card", "import_deck", "_commit",
})

#: The private write helpers and the editor module underneath them. Naming the
#: module matters as much as the functions: `edit.set_card_field` reaches the
#: same field one import lower down, and would not be caught by a check that
#: only knew about `service`.
FORBIDDEN_IMPORTS = frozenset({
    "mtglab.decks.edit",
})


def claude_sources() -> list[Path]:
    files = sorted(CLAUDE_PKG.rglob("*.py"))
    assert files, "the claude package has no modules -- has it moved?"
    return files


def names_used(tree: ast.AST) -> set[str]:
    """Every identifier the module mentions: attributes, imports, bare names.

    Deliberately blunt. A false positive here is a comment to reword; a false
    negative is the rule quietly not holding. Docstrings are excluded by
    construction (they are `ast.Constant`, not identifiers), which is what lets
    the modules in this package discuss `set_card_field` by name -- as they
    must, to explain why it is absent.
    """
    used: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Attribute):
            used.add(node.attr)
        elif isinstance(node, ast.Name):
            used.add(node.id)
        elif isinstance(node, ast.Import | ast.ImportFrom):
            used.update(a.name for a in node.names)
    return used


# ------------------------------------------------- the structural assertion

@pytest.mark.parametrize("path", claude_sources(), ids=lambda p: p.name)
def test_no_module_in_the_claude_package_names_a_write_function(path):
    """The assertion ADR 15 asked for, over source rather than behaviour."""
    used = names_used(ast.parse(path.read_text(encoding="utf-8")))
    reachable = used & WRITE_SURFACE
    assert not reachable, (
        f"{path.name} references {sorted(reachable)}. Nothing under "
        f"src/mtglab/claude/ may reach a deck write -- see ADR 15. If this is "
        f"deliberate, it needs a new ADR superseding it, not a change here.")


@pytest.mark.parametrize("path", claude_sources(), ids=lambda p: p.name)
def test_no_module_in_the_claude_package_imports_the_editor(path):
    used = names_used(ast.parse(path.read_text(encoding="utf-8")))
    assert not (used & FORBIDDEN_IMPORTS), (
        f"{path.name} imports the surgical editor. The write path is reachable "
        f"one import below `service`; this closes that door too.")


def test_the_write_surface_named_here_still_exists():
    """Guard against the guard rotting.

    If `set_card_field` is renamed, the tests above keep passing while checking
    for a function nobody can call any more -- a green suite that guards
    nothing. So assert the names are real, and let a rename fail loudly here
    where the fix is obvious.
    """
    missing = sorted(n for n in WRITE_SURFACE if not hasattr(service, n))
    assert not missing, (
        f"{missing} no longer exist in api/service.py. Update WRITE_SURFACE "
        f"to the current names -- do not delete the entries. (Promotion is not "
        f"listed separately: it is `set_deck_field(field='stage')`.)")


# ------------------------------------------------------- the runtime refusal

def test_the_registry_exposes_no_write_function():
    exposed = {t.fn.__name__ for t in tools.READ_ONLY.values()}
    assert not (exposed & WRITE_SURFACE)


def test_asking_for_set_card_field_by_name_is_refused():
    """The model can request any tool name it likes, including one never
    advertised. `run()` decides on the name it actually received."""
    with pytest.raises(tools.ToolNotAllowed):
        tools.run("set_card_field",
                  {"slug": "arahbo-cats", "name": "Sol Ring",
                   "field": "why", "value": "Fast mana, obviously."})


@pytest.mark.parametrize("name", sorted(WRITE_SURFACE))
def test_every_write_function_is_refused_by_name(name):
    with pytest.raises(tools.ToolNotAllowed):
        tools.run(name, {})


def test_a_mode_cannot_widen_the_tool_set_by_asking():
    """`schemas()` subsets the registry. ADR 15: a stance may widen what a mode
    does, never what it is allowed to do -- and neither may a mode."""
    with pytest.raises(tools.ToolNotAllowed):
        tools.schemas(("get_deck", "set_card_field"))


def test_no_tool_schema_accepts_a_why():
    """Belt and braces on the argument side.

    Even a read-only tool would be a laundering route if it took prose destined
    for a rationale. Nothing in the registry should have a `why` input.
    """
    for tool in tools.READ_ONLY.values():
        assert "why" not in tool.properties, f"{tool.name} takes a `why`"
