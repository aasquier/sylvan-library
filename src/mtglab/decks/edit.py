"""Surgical edits to a deck.yaml, preserving every byte it does not change.

`deck.yaml` is the source of truth and its history is git history (ADR 1), so
`swaps.md` is literally a diff. That makes the *size* of an edit part of its
correctness: a one-card swap has to be a one-card diff, or the swap record it
produces is unreadable.

Load-and-dump cannot do that, and this was measured rather than assumed:

    pyyaml `Deck.dump()`   829 changed lines on Goreclaw, and all 8 comments gone
    ruamel round-trip      6-132 changed lines depending on the file

ruamel keeps comments, which pyyaml cannot, but both reflow folded scalars to
their own preferred width and the decks were hand-wrapped at several. So this
module edits the *text*: it finds the lines belonging to one card entry and
rewrites only those.

The safety net is that every edit verifies itself. `replace_card` parses the
file before and after and refuses to return a result in which anything other
than the targeted entry has changed. A silent corruption of the source of truth
is the one failure this module must not have, so it is checked rather than
argued.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

import yaml

# `  - name: Primeval Titan` -- the first line of a card entry. The name may be
# quoted, and a card name can contain almost anything, so the value is taken
# whole and compared after parsing rather than matched precisely here.
_ENTRY_START = re.compile(r"^(?P<indent>\s*)-\s+name:\s*(?P<value>.*?)\s*$")
_KEY = re.compile(r"^(?P<indent>\s*)(?P<key>[A-Za-z_][\w-]*):(?P<rest>.*)$")


class EditFailed(Exception):
    """The edit could not be made safely, so nothing was changed."""


@dataclass(frozen=True)
class _Entry:
    start: int          # index of the `- name:` line
    end: int            # index one past the entry's last line
    dash_indent: int    # columns before the `-`
    key_indent: int     # columns before `category:`, `why:` and friends


def _unquote(value: str) -> str:
    """Parse a YAML scalar the way the loader will, so quoting never matters."""
    try:
        parsed = yaml.safe_load(value)
    except yaml.YAMLError:
        return value.strip()
    return parsed if isinstance(parsed, str) else value.strip()


def _find_entry(lines: list[str], name: str) -> _Entry:
    wanted = name.strip().lower()
    for i, line in enumerate(lines):
        match = _ENTRY_START.match(line)
        if not match or _unquote(match["value"]).lower() != wanted:
            continue

        dash_indent = len(match["indent"])
        # Keys of this entry are indented past the dash. Read the indent from
        # the next key line rather than assuming two spaces.
        key_indent = dash_indent + 2
        for follower in lines[i + 1:]:
            if follower.strip():
                key_indent = len(follower) - len(follower.lstrip())
                break

        end = len(lines)
        for j in range(i + 1, len(lines)):
            following = lines[j]
            if not following.strip():
                continue
            indent = len(following) - len(following.lstrip())
            # The next sibling entry, or anything dedented out of this list.
            if indent <= dash_indent:
                end = j
                break
        return _Entry(start=i, end=end, dash_indent=dash_indent,
                      key_indent=key_indent)

    raise EditFailed(f"no card entry named {name!r}")


def _render(key: str, value: str, indent: int, *, width: int = 96) -> list[str]:
    """Render one `key: value` pair as YAML lines at the given indent.

    Delegates the scalar rules to pyyaml rather than hand-rolling them: a `why`
    containing a colon, a leading `>` or a trailing space each need different
    quoting, and getting one wrong writes a file that no longer parses.
    """
    text = yaml.safe_dump({key: value}, default_flow_style=False,
                          allow_unicode=True, sort_keys=False,
                          width=max(20, width - indent))
    pad = " " * indent
    return [pad + line if line else line for line in text.rstrip("\n").split("\n")]


def _entry_index(doc: object, name: str) -> tuple[str, int] | None:
    """Where a card sits in the parsed document, as (list name, position)."""
    if not isinstance(doc, dict):
        return None
    for key in ("cards", "swap_board"):
        for i, entry in enumerate(doc.get(key) or []):
            if isinstance(entry, dict) and \
                    str(entry.get("name", "")).lower() == name.strip().lower():
                return key, i
    return None


def replace_card(text: str, *, old_name: str, new_name: str, why: str,
                 category: str | None = None) -> str:
    """Replace one card entry's name and rationale, in place.

    `category` defaults to whatever the outgoing card was filed under, since a
    replacement usually fills the same role -- but the caller can move it.

    Raises `EditFailed` rather than returning a damaged file. Every other card,
    every note, every comment and every blank line survives untouched.
    """
    if not why.strip():
        # Rule 4. A card that cannot justify its slot is a card to cut, and a
        # rationale invented by a machine is exactly the empty justification
        # that rule exists to prevent.
        raise EditFailed("a replacement needs a `why`; refusing to invent one")

    before = yaml.safe_load(text) or {}
    located = _entry_index(before, old_name)
    if located is None:
        raise EditFailed(f"no card entry named {old_name!r}")
    list_key, position = located

    lines = text.split("\n")
    entry = _find_entry(lines, old_name)

    original = before[list_key][position]
    rebuilt: list[str] = []
    seen_why = False
    # True while walking the continuation lines of a key that was rewritten or
    # dropped -- a folded `why: >` owns every indented line beneath it, and
    # keeping those would leave the old rationale stranded under the new one.
    dropping = False

    for offset, line in enumerate(lines[entry.start:entry.end]):
        if offset == 0:
            rendered = _render("name", new_name, entry.key_indent)
            # Re-attach the dash, which `_render` knows nothing about.
            rebuilt.append(" " * entry.dash_indent + "- " + rendered[0].lstrip())
            rebuilt.extend(rendered[1:])
            continue

        key = _KEY.match(line)
        if key and len(key["indent"]) == entry.key_indent:
            name = key["key"]
            dropping = False
            if name == "why":
                seen_why = True
                rebuilt.extend(_render("why", why, entry.key_indent))
                dropping = True
                continue
            if name == "category" and category is not None:
                rebuilt.extend(_render("category", category, entry.key_indent))
                dropping = True
                continue
            # `mana_cost` and `scryfall_id` describe the card that is leaving.
            # Carrying them over would attach one card's identity to another.
            if name in ("mana_cost", "scryfall_id"):
                dropping = True
                continue
            rebuilt.append(line)
            continue

        if not dropping:
            rebuilt.append(line)

    if not seen_why:
        rebuilt.extend(_render("why", why, entry.key_indent))

    updated = "\n".join(lines[:entry.start] + rebuilt + lines[entry.end:])
    _verify(before, updated, list_key, position, new_name, why,
            category or original.get("category"))
    return updated


def _verify(before: object, updated: str, list_key: str, position: int,
            new_name: str, why: str, category: str | None) -> None:
    """Refuse to hand back a file that changed more than it was asked to."""
    try:
        after = yaml.safe_load(updated)
    except yaml.YAMLError as exc:
        raise EditFailed(f"the edit produced YAML that no longer parses: {exc}") from exc

    if not isinstance(after, dict) or not isinstance(before, dict):
        raise EditFailed("deck.yaml is not a mapping")

    entry = after.get(list_key, [])[position]
    if entry.get("name") != new_name:
        raise EditFailed("the replacement did not land on the expected entry")
    if entry.get("why") != why:
        raise EditFailed("the rationale did not survive the edit")
    if category is not None and entry.get("category") != category:
        raise EditFailed("the category changed unexpectedly")

    # Everything else must be identical. Compare the whole document with the
    # one entry masked out on both sides.
    left = yaml.safe_load(yaml.safe_dump(before))
    right = yaml.safe_load(yaml.safe_dump(after))
    left[list_key][position] = None
    right[list_key][position] = None
    if left != right:
        raise EditFailed(
            "the edit changed something other than the card it targeted")
