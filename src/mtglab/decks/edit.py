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
rewrites only those. ADR 12 has the full argument and the five rules every
operation here obeys.

**How an edit proves itself.** Each operation computes the document it *ought*
to produce by mutating the parsed deck -- an ordinary dict, no text involved --
and then refuses to return its text unless that text parses to exactly that
document. The naive parse-mutate-dump is used as the oracle it is good at being,
while the text surgery does the writing it is good at doing. That is the same
move as ADR 10, and it means an operation cannot quietly damage a neighbouring
card, drop a note, or reorder the 99: any of those show up as a document
mismatch and the edit is refused with nothing written.

The failure this module must not have is silently corrupting the one file the
whole project is built on, so it is checked rather than argued.
"""

from __future__ import annotations

import copy
import re
from dataclasses import dataclass
from typing import Any

import yaml

# `  - name: Primeval Titan` -- the first line of a card entry. The name may be
# quoted, and a card name can contain almost anything, so the value is taken
# whole and compared after parsing rather than matched precisely here.
_ENTRY_START = re.compile(r"^(?P<indent>\s*)-\s+name:\s*(?P<value>.*?)\s*$")
_KEY = re.compile(r"^(?P<indent>\s*)(?P<key>[A-Za-z_][\w-]*):(?P<rest>.*)$")

# The two lists a card can live in. `swap_board` is the bubble: cards kept just
# outside the 99 with the reason they did not make it.
CARD_LISTS = ("cards", "swap_board")

# What `set_card_field` will write. Deliberately short. `name` belongs to
# `replace_card`, which also drops the overrides identifying the outgoing card;
# `scryfall_id` and `mana_cost` are overrides for cards the corpus does not yet
# know, and hand-editing them through this path would mask a stale corpus.
SETTABLE_FIELDS = ("category", "qty", "why")

# Sentinels for `_rewrite_entry`: a key absent from the change set is copied
# through untouched, and one mapped to `_DROP` is deleted.
_KEEP = object()
_DROP = object()


class EditFailed(Exception):
    """The edit could not be made safely, so nothing was changed."""


@dataclass(frozen=True)
class _Entry:
    start: int          # index of the `- name:` line
    end: int            # index one past the entry's last line
    dash_indent: int    # columns before the `-`
    key_indent: int     # columns before `category:`, `why:` and friends


# ------------------------------------------------------------------- reading

def _open(text: str) -> tuple[dict[str, Any], list[str]]:
    """Parse the deck and split its lines, the way every operation starts."""
    try:
        doc = yaml.safe_load(text) or {}
    except yaml.YAMLError as exc:
        raise EditFailed(f"deck.yaml does not parse: {exc}") from exc
    if not isinstance(doc, dict):
        raise EditFailed("deck.yaml is not a mapping")
    return doc, text.split("\n")


def _unquote(value: str) -> str:
    """Parse a YAML scalar the way the loader will, so quoting never matters."""
    try:
        parsed = yaml.safe_load(value)
    except yaml.YAMLError:
        return value.strip()
    return parsed if isinstance(parsed, str) else value.strip()


def _requires_rationale(doc: dict[str, Any]) -> bool:
    """Whether this deck's cards must each carry a `why` (rule 4, ADR 13).

    A curated deck must; a draft is honestly incomplete and owes them. Absent
    means curated, matching `Deck.from_text`, so an edit to one of the six
    existing decks is never quietly held to the looser standard.
    """
    return str(doc.get("stage") or "curated").strip().lower() != "draft"


def _block_header(lines: list[str], key: str) -> int:
    """The index of a top-level key's own line.

    Matches an emptied list -- `swap_board: []` -- as well as a block, because
    a list this module emptied has to be one it can fill again.
    """
    header = re.compile(rf"^{re.escape(key)}:\s*(\[\s*\])?\s*(#.*)?$")
    for i, line in enumerate(lines):
        if header.match(line):
            return i
    raise EditFailed(f"no `{key}:` block in this deck file")


def _block_span(lines: list[str], key: str) -> tuple[int, int]:
    """The line range of a top-level block's body, as `[start, end)`.

    Ends at the next line in column zero, so the following key -- or a comment
    introducing it -- stays outside the block and out of reach of any edit.
    """
    i = _block_header(lines, key)
    for j in range(i + 1, len(lines)):
        if lines[j].strip() and not lines[j][:1].isspace():
            return i + 1, j
    return i + 1, len(lines)


def _entry_spans(lines: list[str], span: tuple[int, int]) -> list[_Entry]:
    """Every item in a sequence's line range, in document order.

    An entry runs to the start of the next one, so anything between two cards --
    Goreclaw's `# ---- RAMP 14` banners, blank lines -- lands in the earlier
    entry's span and is trimmed back off by `_split_tail` before anything is
    written. Nothing between two cards is ever treated as part of either.
    """
    start, stop = span
    dash_indent: int | None = None
    starts: list[int] = []
    for i in range(start, min(stop, len(lines))):
        match = _ENTRY_START.match(lines[i])
        if not match:
            continue
        indent = len(match["indent"])
        if dash_indent is None:
            dash_indent = indent
        if indent == dash_indent:
            starts.append(i)

    if dash_indent is None:
        return []

    out: list[_Entry] = []
    for n, i in enumerate(starts):
        end = starts[n + 1] if n + 1 < len(starts) else stop
        # Read the key indent off the entry rather than assuming two spaces.
        key_indent = dash_indent + 2
        for follower in lines[i + 1:end]:
            if follower.strip():
                key_indent = len(follower) - len(follower.lstrip())
                break
        out.append(_Entry(start=i, end=end, dash_indent=dash_indent,
                          key_indent=key_indent))
    return out


def _split_tail(body: list[str], key_indent: int) -> tuple[list[str], list[str]]:
    """Split an entry's own lines from the gap that follows it.

    The gap is blank lines and comments sitting at or left of the key indent --
    a section banner introducing the *next* group of cards, and the blank line
    before it. They are inside the entry's line span only as an artefact of
    where the next entry starts, and no edit may touch them.
    """
    cut = len(body)
    while cut > 0:
        line = body[cut - 1]
        stripped = line.strip()
        if not stripped:
            cut -= 1
            continue
        indent = len(line) - len(line.lstrip())
        if stripped.startswith("#") and indent <= key_indent:
            cut -= 1
            continue
        break
    return body[:cut], body[cut:]


def _locate_card(doc: dict[str, Any], lines: list[str], name: str,
                 ) -> tuple[str, int, _Entry]:
    """Find a card, agreeing on both its parsed position and its lines.

    Returning the two from a single lookup is what makes verification mean
    anything. If the document index and the text span could ever disagree -- a
    name appearing in both `cards` and `swap_board` is enough to do it -- an
    edit could check one entry and rewrite a different one, which is the exact
    failure the verification exists to catch.
    """
    wanted = name.strip().lower()
    for key in CARD_LISTS:
        items = doc.get(key) or []
        if not isinstance(items, list):
            continue
        try:
            span = _block_span(lines, key)
        except EditFailed:
            continue
        spans = _entry_spans(lines, span)
        if len(spans) != len(items):
            # The parse and the text disagree about how many cards there are,
            # so no span can be trusted. ADR 12 anticipated this: the file uses
            # YAML this module does not handle -- an anchor, a flow mapping, a
            # bare string entry -- and the answer is to refuse, not to guess.
            raise EditFailed(
                f"`{key}` parses to {len(items)} entries but {len(spans)} were "
                "found in the text; this file uses YAML the editor cannot edit "
                "safely")
        for i, item in enumerate(items):
            if isinstance(item, dict) and \
                    str(item.get("name", "")).strip().lower() == wanted:
                return key, i, spans[i]
    raise EditFailed(f"no card entry named {name!r}")


# ------------------------------------------------------------------- writing

# Where a folded note wraps. pyyaml treats `width` as the column it breaks
# *after*, so the emitted lines run several characters wider than the number
# given. The hand-written notes in the deck files top out at 79 columns; 72
# lands under that, and matters because a note is prose sitting next to other
# prose that nothing is allowed to reflow.
_PROSE_WIDTH = 72


class _Folded(str):
    """A string to render as a YAML folded block, the way the decks are written."""


class _FoldedDumper(yaml.SafeDumper):
    pass


_FoldedDumper.add_representer(
    _Folded,
    lambda dumper, data: dumper.represent_scalar(
        "tag:yaml.org,2002:str", str(data), style=">"),
)


def _render(key: str, value: Any, indent: int, *, width: int = 96,
            fold: bool = False) -> list[str]:
    """Render one `key: value` pair as YAML lines at the given indent.

    Delegates the scalar rules to pyyaml rather than hand-rolling them: a `why`
    containing a colon, a leading `>` or a trailing space each need different
    quoting, and getting one wrong writes a file that no longer parses.

    `fold` asks for the block style the deck files use for prose. It is a
    request, not an instruction: folding collapses single newlines and adjusts
    the trailing one, so for some strings it is not value-preserving. When the
    folded form would not read back as what was passed in, this falls back to
    the dumper's own choice, which always does.
    """
    payload = {key: _Folded(value) if fold else value}
    text = yaml.dump(payload, Dumper=_FoldedDumper, default_flow_style=False,
                     allow_unicode=True, sort_keys=False,
                     width=max(20, width - indent))
    if fold and yaml.safe_load(text) != {key: value}:
        return _render(key, value, indent, width=width, fold=False)
    pad = " " * indent
    return [pad + line if line else line for line in text.rstrip("\n").split("\n")]


def _card_lines(entry: _Entry, *, name: str, category: str, why: str,
                qty: int) -> list[str]:
    """A whole card entry, in the key order the deck files are written in."""
    rendered = _render("name", name, entry.key_indent)
    out = [" " * entry.dash_indent + "- " + rendered[0].lstrip()]
    out.extend(rendered[1:])
    out.extend(_render("category", category, entry.key_indent))
    if qty != 1:
        out.extend(_render("qty", qty, entry.key_indent))
    # Always written, even blank. In a draft the empty `why:` is the to-do list
    # recorded in the file itself, which is how `decks import` writes one and
    # where ADR 13 wants the outstanding work to be visible.
    out.extend(_render("why", why, entry.key_indent))
    return out


def _rewrite_entry(lines: list[str], entry: _Entry,
                   changes: dict[str, Any]) -> list[str]:
    """Rewrite one entry, applying `changes` key by key.

    A key mapped to `_DROP` is deleted; a key in `changes` that the entry does
    not have yet is appended. Everything else is copied verbatim, including the
    continuation lines of folded scalars that were not touched.
    """
    body, tail = _split_tail(lines[entry.start:entry.end], entry.key_indent)
    rebuilt: list[str] = []
    seen: set[str] = set()
    # True while walking the continuation lines of a key that was rewritten or
    # dropped -- a folded `why: >` owns every line indented beneath it, and
    # keeping those would strand the old rationale under the new one.
    dropping = False

    for offset, line in enumerate(body):
        if offset == 0:
            seen.add("name")
            value = changes.get("name", _KEEP)
            if value is _KEEP:
                rebuilt.append(line)
            else:
                rendered = _render("name", value, entry.key_indent)
                # Re-attach the dash, which `_render` knows nothing about.
                rebuilt.append(" " * entry.dash_indent + "- " + rendered[0].lstrip())
                rebuilt.extend(rendered[1:])
                dropping = True
            continue

        match = _KEY.match(line)
        if match and len(match["indent"]) == entry.key_indent:
            key = match["key"]
            seen.add(key)
            dropping = False
            value = changes.get(key, _KEEP)
            if value is _KEEP:
                rebuilt.append(line)
            elif value is _DROP:
                dropping = True
            else:
                rebuilt.extend(_render(key, value, entry.key_indent))
                dropping = True
            continue

        # Only lines indented past the key can belong to the value being
        # replaced. A comment at or left of the key indent is the file's own,
        # so it survives even in the middle of a rewritten entry.
        indent = len(line) - len(line.lstrip())
        if dropping and (not line.strip() or indent > entry.key_indent):
            continue
        dropping = False
        rebuilt.append(line)

    for key, value in changes.items():
        if key not in seen and value is not _DROP:
            rebuilt.extend(_render(key, value, entry.key_indent))

    return rebuilt + tail


# -------------------------------------------------------------- verification

def _first_difference(expected: Any, actual: Any, path: str = "") -> str:
    """Where two documents first disagree, as a readable path.

    Only ever called on documents already known to differ, so it always finds
    something to say.
    """
    if isinstance(expected, dict) and isinstance(actual, dict):
        for key in dict.fromkeys([*expected, *actual]):
            if key not in expected:
                return f"{path}.{key} appeared"
            if key not in actual:
                return f"{path}.{key} disappeared"
            if expected[key] != actual[key]:
                return _first_difference(expected[key], actual[key], f"{path}.{key}")
    elif isinstance(expected, list) and isinstance(actual, list):
        if len(expected) != len(actual):
            return (f"{path or 'the list'} has {len(actual)} entries, "
                    f"expected {len(expected)}")
        for i, (want, got) in enumerate(zip(expected, actual, strict=True)):
            if want != got:
                return _first_difference(want, got, f"{path}[{i}]")
    return f"{path or 'the document'} is {actual!r}, expected {expected!r}"


def _verified(updated: str, expected: dict[str, Any]) -> str:
    """Hand back the edited text only if it means exactly what it should.

    `expected` is the deck as an ordinary dict, mutated the obvious way. Any
    difference at all -- a neighbouring card damaged, a note lost, the 99
    reordered, a folded scalar's tail stranded -- fails here, and a refused
    edit has changed nothing because these functions return text rather than
    writing it.
    """
    try:
        after = yaml.safe_load(updated)
    except yaml.YAMLError as exc:
        raise EditFailed(
            f"the edit produced YAML that no longer parses: {exc}") from exc
    if after != expected:
        raise EditFailed("the edit changed more than it was asked to: "
                         + _first_difference(expected, after))
    return updated


# -------------------------------------------------------------- operations

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
        # that rule exists to prevent. Required even in a draft: a draft owes
        # rationales it has not written, which is not the same as a card whose
        # slot was actively reconsidered and still cannot be argued for.
        raise EditFailed("a replacement needs a `why`; refusing to invent one")

    doc, lines = _open(text)
    list_key, position, entry = _locate_card(doc, lines, old_name)

    changes: dict[str, Any] = {"name": new_name, "why": why}
    if category is not None:
        changes["category"] = category
    # `mana_cost` and `scryfall_id` describe the card that is leaving. Carrying
    # them over would attach one card's identity to another.
    changes["mana_cost"] = _DROP
    changes["scryfall_id"] = _DROP

    rebuilt = _rewrite_entry(lines, entry, changes)
    updated = "\n".join(lines[:entry.start] + rebuilt + lines[entry.end:])

    expected = copy.deepcopy(doc)
    item = dict(expected[list_key][position])
    item["name"] = new_name
    item["why"] = why
    if category is not None:
        item["category"] = category
    item.pop("mana_cost", None)
    item.pop("scryfall_id", None)
    expected[list_key][position] = item
    return _verified(updated, expected)


def add_card(text: str, *, name: str, category: str, why: str = "",
             qty: int = 1, list_key: str = "cards") -> str:
    """Add a card to the 99 or to the swap board.

    Inserted next to the cards it belongs with -- after the last entry already
    in its category -- rather than at the end of the list. The deck files are
    grouped by category under section banners, and appending a land to the
    bottom of the file would file it under whatever the last banner happened to
    say. Falls back to the end of the list when the category is new.

    The rationale is required unless the deck is a draft, where a blank `why`
    is the counted work the deck still owes (ADR 13). It is never generated:
    an empty `why` on a curated deck is refused, not filled in.
    """
    if list_key not in CARD_LISTS:
        raise EditFailed(f"cards live in {' or '.join(CARD_LISTS)}, not {list_key!r}")
    name = name.strip()
    category = category.strip()
    if not name:
        raise EditFailed("a card needs a name")
    if not category:
        raise EditFailed("a card needs a category")
    if qty < 1:
        raise EditFailed("quantity must be at least 1")

    doc, lines = _open(text)
    if _requires_rationale(doc) and not why.strip():
        # Rule 4, at the point where a card enters the deck. See ADR 12 rule 3.
        raise EditFailed(
            "a card in a curated deck needs a `why`; refusing to invent one")

    for key in CARD_LISTS:
        for item in doc.get(key) or []:
            if isinstance(item, dict) and \
                    str(item.get("name", "")).strip().lower() == name.lower():
                where = "the deck" if key == "cards" else "the swap board"
                raise EditFailed(
                    f"{name!r} is already in {where}; change its quantity or "
                    "rationale instead of adding a second entry")
    for commander in doc.get("commander") or []:
        if str(commander).strip().lower() == name.lower():
            raise EditFailed(f"{name!r} is the commander, which sits outside the 99")

    span = _block_span(lines, list_key)
    spans = _entry_spans(lines, span)
    items = list(doc.get(list_key) or [])
    if len(spans) != len(items):
        raise EditFailed(
            f"`{list_key}` parses to {len(items)} entries but {len(spans)} were "
            "found in the text; this file uses YAML the editor cannot edit safely")

    if not spans:
        # An empty list is written `swap_board: []`, which cannot carry block
        # items beneath it. Reopen the block before filling it.
        header = _block_header(lines, list_key)
        lines = [*lines[:header], f"{list_key}:", *lines[header + 1:]]
        position, at = 0, header + 1
        shape = _Entry(start=at, end=at, dash_indent=2, key_indent=4)
    else:
        anchor = len(spans) - 1
        for i, item in enumerate(items):
            if isinstance(item, dict) and \
                    str(item.get("category", "")).strip().lower() == category.lower():
                anchor = i
        shape = spans[anchor]
        content, _tail = _split_tail(lines[shape.start:shape.end], shape.key_indent)
        position, at = anchor + 1, shape.start + len(content)

    rendered = _card_lines(shape, name=name, category=category,
                           why=why.strip(), qty=qty)
    updated = "\n".join(lines[:at] + rendered + lines[at:])

    added: dict[str, Any] = {"name": name, "category": category}
    if qty != 1:
        added["qty"] = qty
    added["why"] = why.strip()
    expected = copy.deepcopy(doc)
    expected[list_key] = list(expected.get(list_key) or [])
    expected[list_key].insert(position, added)
    return _verified(updated, expected)


def remove_card(text: str, *, name: str) -> str:
    """Take a card out of the 99 or the swap board, and nothing else out.

    Section banners and the blank lines around them belong to the cards below
    them, not to the card being removed, so they stay. The blank line *after*
    an entry goes with it, which is what keeps the spacing even.
    """
    doc, lines = _open(text)
    list_key, position, entry = _locate_card(doc, lines, name)

    content, tail = _split_tail(lines[entry.start:entry.end], entry.key_indent)
    cut = entry.start + len(content)
    leads_to_a_banner = any(line.strip().startswith("#") for line in tail)
    if position < len(doc[list_key]) - 1 and not leads_to_a_banner:
        # An entry owns the blank line after it, so taking the card takes the
        # gap and the spacing stays even. Unless the gap leads to a section
        # banner, which owns the blank above it: removing the last land must
        # not weld `# ---- RAMP 14` onto the land before it.
        while cut < entry.end and not lines[cut].strip():
            cut += 1

    expected = copy.deepcopy(doc)
    del expected[list_key][position]

    if not expected[list_key]:
        # A block key with nothing under it parses to None, not to an empty
        # list, and `Deck.from_text` would iterate it. Say `[]` explicitly.
        header = _block_header(lines, list_key)
        lines = [*lines[:header], f"{list_key}: []", *lines[header + 1:]]

    updated = "\n".join(lines[:entry.start] + lines[cut:])
    return _verified(updated, expected)


def set_card_field(text: str, *, name: str, field: str, value: Any) -> str:
    """Change one field of one card: its category, its quantity, or its `why`.

    This is the write path behind the rationale editor, and the one place a
    `why` can be filled in without replacing the card. The value comes from the
    caller and is written as given -- nothing here composes, templates, tidies
    or infers one (ADR 12 rule 3).
    """
    if field not in SETTABLE_FIELDS:
        raise EditFailed(
            f"{field!r} is not settable; choose one of {', '.join(SETTABLE_FIELDS)}")

    doc, lines = _open(text)
    list_key, position, entry = _locate_card(doc, lines, name)

    if field == "qty":
        try:
            value = int(value)
        except (TypeError, ValueError) as exc:
            raise EditFailed(f"quantity must be a whole number, not {value!r}") from exc
        if value < 1:
            raise EditFailed("quantity must be at least 1; remove the card instead")
    else:
        value = str(value)
        if field == "category" and not value.strip():
            raise EditFailed("a card needs a category")
        if field == "why":
            value = value.strip()
            if not value and _requires_rationale(doc):
                raise EditFailed(
                    "a card in a curated deck needs a `why`; refusing to blank it")

    rebuilt = _rewrite_entry(lines, entry, {field: value})
    updated = "\n".join(lines[:entry.start] + rebuilt + lines[entry.end:])

    expected = copy.deepcopy(doc)
    item = dict(expected[list_key][position])
    item[field] = value
    expected[list_key][position] = item
    return _verified(updated, expected)


def set_note(text: str, *, key: str, value: str) -> str:
    """Set one deck-level note, the prose the advanced primer reads directly.

    Notes are the deck's thinking -- the mulligan rule, the pitfalls, the lines
    -- and they survive regeneration because they live in the source of truth
    rather than in an artifact. Creates the `notes:` block if the deck has none,
    placing it where `Deck.dump` would: after the strategy, before the cards.
    """
    key = key.strip()
    if not key:
        raise EditFailed("a note needs a key")
    if not _KEY.match(f"{key}:"):
        raise EditFailed(
            f"{key!r} is not a usable note key; use letters, digits and underscores")
    if not value.strip():
        # Symmetrical with a card's `why`: an empty note is not a note, and
        # writing one would put a blank heading in the advanced primer.
        raise EditFailed("a note needs text")
    value = value.strip()

    doc, lines = _open(text)
    notes = doc.get("notes")
    if notes is not None and not isinstance(notes, dict):
        raise EditFailed("`notes:` is not a mapping in this deck file")

    # Whether the block exists is a question about the text, not the parse: a
    # `notes:` header with nothing under it parses to None, and answering from
    # the parse would write the deck a second `notes:` block.
    try:
        span: tuple[int, int] | None = _block_span(lines, "notes")
    except EditFailed:
        span = None

    if span is None:
        # `cards:` is the anchor because every deck has one, and it is what
        # notes sit above in the order `Deck.dump` writes.
        try:
            anchor = _block_span(lines, "cards")[0] - 1
        except EditFailed:
            anchor = len(lines)
        while anchor > 0 and not lines[anchor - 1].strip():
            anchor -= 1
        rendered = ["", "notes:",
                    *_render(key, value, 2, width=_PROSE_WIDTH, fold=True)]
        updated = "\n".join(lines[:anchor] + rendered + lines[anchor:])
    else:
        indent = next((len(lines[i]) - len(lines[i].lstrip())
                       for i in range(*span) if lines[i].strip()), 2)
        start = end = None
        for i in range(*span):
            match = _KEY.match(lines[i])
            if match and len(match["indent"]) == indent and match["key"] == key:
                start = i
                end = span[1]
                for j in range(i + 1, span[1]):
                    if lines[j].strip() and \
                            len(lines[j]) - len(lines[j].lstrip()) <= indent:
                        end = j
                        break
                break
        rendered = _render(key, value, indent, width=_PROSE_WIDTH, fold=True)
        if start is None:
            body, _tail = _split_tail(lines[span[0]:span[1]], indent)
            at = span[0] + len(body)
            updated = "\n".join(lines[:at] + rendered + lines[at:])
        else:
            _content, tail = _split_tail(lines[start:end], indent)
            updated = "\n".join(lines[:start] + rendered + tail + lines[end:])

    expected = copy.deepcopy(doc)
    expected["notes"] = {**(notes or {}), key: value}
    return _verified(updated, expected)
