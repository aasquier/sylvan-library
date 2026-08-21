"""Generate the five deliverables from deck.yaml.

Required for every new deck and every refactor:
  1. primer-quick.md      -- one page, get someone playing
  2. primer-advanced.md   -- lines, sequencing, matchups, failure modes
  3. decklist-annotated.md-- the 99 with a reason for every card
  4. moxfield.txt         -- bulk import
  5. swaps.md             -- only when the deck changed (out/in + shopping)

Only for a `curated` deck. A draft has cards whose slot nobody has argued for
yet, and a primer that quietly omits the argument reads exactly like one that
had it -- so `write_all` refuses and names the cards still owed (ADR 13).

Generation is deterministic: same deck.yaml, same output. That is the point.
The prose that only a human or a conversation can supply -- strategy, lines,
matchup notes -- lives in `notes:` in the deck file, so it survives regeneration
instead of being retyped each time.
"""

from __future__ import annotations

from collections.abc import Mapping
from datetime import date
from pathlib import Path
from typing import TYPE_CHECKING, Any

from mtglab.decks.model import CATEGORIES, Deck

if TYPE_CHECKING:
    from mtglab.cards.db import CardRecord

CATEGORY_TITLES = {
    "land": "Lands",
    "ramp": "Ramp & Mana Acceleration",
    "card-advantage": "Card Advantage",
    "tutor": "Tutors",
    "interaction": "Interaction & Removal",
    "protection": "Protection",
    "threat": "Threats",
    "engine": "Engines",
    "sac-outlet": "Sacrifice Outlets",
    "payoff": "Payoffs",
    "recursion": "Recursion",
    "win-con": "Win Conditions",
    "utility": "Utility",
}


def _ordered_categories(deck: Deck) -> list[str]:
    present = deck.by_category
    ordered = [c for c in CATEGORIES if c in present]
    ordered += sorted(c for c in present if c not in CATEGORIES)
    # Lands last in the annotated list -- readers want the spells first.
    if "land" in ordered:
        ordered.remove("land")
        ordered.append("land")
    return ordered


def _note(deck: Deck, key: str, default: str = "") -> str:
    return (deck.notes.get(key) or default).strip()


def _header(deck: Deck) -> str:
    cmd = " + ".join(deck.commander) or "(no commander set)"
    bits = [f"**Commander:** {cmd}"]
    if deck.companion:
        bits.append(f"**Companion:** {deck.companion} *(outside the 100)*")
    if deck.bracket:
        bits.append(f"**Bracket:** {deck.bracket}")
    bits.append(f"**Size:** {deck.total_cards} + commander")
    return "  \n".join(bits)


# ------------------------------------------------------------------ 1. quick

def quick_primer(deck: Deck, stats: dict[str, Any] | None = None) -> str:
    counts = deck.category_counts
    lines = [
        f"# {deck.name} — Quick Start",
        "",
        _header(deck),
        "",
        "## What this deck does",
        "",
        deck.strategy or "_(set `strategy:` in deck.yaml)_",
        "",
        "## The 30-second version",
        "",
        _note(deck, "gameplan", "_(set `notes.gameplan` in deck.yaml)_"),
        "",
        "## Mulligan rule",
        "",
        _note(deck, "mulligan", "_(set `notes.mulligan` — derive it from a Tier 1 sweep)_"),
        "",
        "## Turn-by-turn shape",
        "",
        _note(deck, "curve_plan", "_(set `notes.curve_plan`)_"),
        "",
        "## Three things that will kill you",
        "",
        _note(deck, "pitfalls", "_(set `notes.pitfalls`)_"),
        "",
        "## Deck at a glance",
        "",
        "| Category | Count |",
        "| --- | ---: |",
    ]
    for cat in _ordered_categories(deck):
        lines.append(f"| {CATEGORY_TITLES.get(cat, cat)} | {counts[cat]} |")

    if stats:
        lines += ["", "## Simulated consistency", ""]
        for k, v in stats.items():
            lines.append(f"- **{k}:** {v}")

    lines += ["", "---", (f"_Generated {date.today().isoformat()} from `deck.yaml`. "
                          "Edit the deck file, not this document._")]
    return "\n".join(lines)


# --------------------------------------------------------------- 2. advanced

def advanced_primer(deck: Deck, stats: dict[str, Any] | None = None) -> str:
    sections = [
        ("Core engine", "engine_detail"),
        ("Lines and sequencing", "lines"),
        ("Win conditions", "wincons"),
        ("Mana base notes", "manabase"),
        ("Matchups", "matchups"),
        ("Politics and table talk", "politics"),
        ("Failure modes and how to recover", "failure_modes"),
        ("Sideboard / swap philosophy", "swap_philosophy"),
        ("Rules corners worth knowing", "rules_corners"),
    ]
    lines = [f"# {deck.name} — Advanced Primer", "", _header(deck), ""]
    for title, key in sections:
        lines += [f"## {title}", "", _note(deck, key, f"_(set `notes.{key}` in deck.yaml)_"), ""]

    if stats:
        lines += ["## Simulation results", ""]
        for k, v in stats.items():
            lines.append(f"- **{k}:** {v}")
        lines.append("")

    lines += ["---", f"_Generated {date.today().isoformat()} from `deck.yaml`._"]
    return "\n".join(lines)


# -------------------------------------------------------------- 3. annotated

def annotated_decklist(deck: Deck, cards: dict[str, CardRecord] | None = None) -> str:
    lines = [f"# {deck.name} — Annotated Decklist", "", _header(deck), ""]
    if deck.strategy:
        lines += [deck.strategy, ""]

    for cmd in deck.commander:
        lines += ["## Command Zone", "", f"**{cmd}** — "
                  + _note(deck, "commander_why", "_(set `notes.commander_why`)_"), ""]
        break

    for cat in _ordered_categories(deck):
        entries = sorted(deck.by_category[cat], key=lambda c: c.name)
        title = CATEGORY_TITLES.get(cat, cat.title())
        lines += [f"## {title} ({sum(e.qty for e in entries)})", ""]
        for entry in entries:
            rec = (cards or {}).get(entry.name)
            cost = f" `{rec.mana_cost}`" if rec and rec.mana_cost else ""
            qty = f"{entry.qty}x " if entry.qty > 1 else ""
            why = entry.why or "_**no rationale recorded — this card should not ship**_"
            lines.append(f"- **{qty}{entry.name}**{cost} — {why}")
        lines.append("")

    if deck.swap_board:
        lines += ["## Swap Board (outside the 99)", ""]
        for entry in sorted(deck.swap_board, key=lambda c: c.name):
            lines.append(f"- **{entry.name}** — {entry.why or '_(no note)_'}")
        lines.append("")

    lines += ["---", f"_Generated {date.today().isoformat()} from `deck.yaml`._"]
    return "\n".join(lines)


# --------------------------------------------------------------- 4. moxfield

def moxfield_txt(deck: Deck) -> str:
    """Moxfield bulk-import format.

    Moxfield has no public API, so plain text import is the supported path.
    The `SIDEBOARD:` marker carries the commander -- Moxfield reads a deck's
    commander from there for Commander-format decks.
    """
    lines: list[str] = []
    for entry in sorted(deck.cards, key=lambda c: c.name):
        lines.append(f"{entry.qty} {entry.name}")
    if deck.commander or deck.companion:
        lines.append("")
        lines.append("SIDEBOARD:")
        for cmd in deck.commander:
            lines.append(f"1 {cmd}")
        if deck.companion:
            lines.append(f"1 {deck.companion}")
    return "\n".join(lines) + "\n"


# ------------------------------------------------------------------ 5. swaps

def swap_list(deck: Deck, previous: Deck, cards: dict[str, CardRecord] | None = None,
              prices: dict[str, float] | None = None) -> str:
    """Diff two versions of a deck into an out/in list plus a shopping list.

    `previous` is normally the deck as of its last build, stashed at
    `artifacts/deck.last-built.yaml` -- decks are not in git (ADR 30), so the
    baseline is one the build keeps for itself. Either way the swap document is
    a computed diff, not a hand-kept changelog.
    """
    old = {c.name for c in previous.cards}
    new = {c.name for c in deck.cards}
    cut, add = sorted(old - new), sorted(new - old)

    lines = [f"# {deck.name} — Swap List", "",
             f"**{len(cut)} out / {len(add)} in**", ""]

    lines += ["## Out", ""]
    for name in cut:
        entry = next((c for c in previous.cards if c.name == name), None)
        lines.append(f"- **{name}** — {entry.why if entry and entry.why else '_(cut)_'}")
    lines += ["", "## In", ""]
    for name in add:
        entry = next((c for c in deck.cards if c.name == name), None)
        lines.append(f"- **{name}** — {entry.why if entry and entry.why else '_(added)_'}")

    if prices:
        need = [(n, prices.get(n)) for n in add]
        known = [(n, p) for n, p in need if p is not None]
        unknown = [n for n, p in need if p is None]
        total = sum(p for _, p in known)
        lines += ["", "## Shopping list", "",
                  "| Card | Cheapest non-foil (USD) |", "| --- | ---: |"]
        for name, price in sorted(known, key=lambda x: -x[1]):
            lines.append(f"| {name} | {price:.2f} |")
        for name in unknown:
            lines.append(f"| {name} | _no price data_ |")
        lines.append(f"| **Total ({len(known)} priced)** | **{total:.2f}** |")

        lines += ["", "### TCGplayer Mass Entry", "",
                  "Paste into <https://www.tcgplayer.com/massentry>:", "", "```"]
        lines += [f"1 {n}" for n in add]
        lines += ["```"]

    lines += ["", "---", f"_Generated {date.today().isoformat()} by diffing deck.yaml._"]
    return "\n".join(lines)


# ------------------------------------------------------------------- driver

class DraftDeck(Exception):
    """Artifacts were requested for a deck nobody has finished reasoning about."""


#: The build's own baseline for the next `swaps.md` (ADR 30). Decks are live
#: app data rather than repository content, so "diff against the previous git
#: revision" stopped being a thing a build could ask for; instead every build
#: stashes the deck as it was built, and the next bare build diffs against it.
#: A normalised dump rather than the hand-written file on purpose: `swap_list`
#: compares decks, not text, and a snapshot that round-trips through
#: `Deck.load` is all the baseline it needs.
SNAPSHOT = "deck.last-built.yaml"

#: The deliverables themselves, in the order the module docstring numbers them.
#: `SNAPSHOT` is deliberately *not* in here and the separation now carries
#: weight: since the API serves artifacts by name, this tuple is the set a
#: reader may ask for, so "is this a deliverable" and "may this be served" are
#: one question with one answer. The snapshot is the build's own bookkeeping --
#: the baseline the next `swaps.md` diffs against -- and nobody asked for a
#: copy of the deck they already have.
#:
#: It doubles as the path-traversal guard on `DeckSource.read_artifact`: a
#: name that is not in this tuple is refused before anything touches a
#: filesystem, so there is no `..` to sanitise.
DELIVERABLES = ("primer-quick.md", "primer-advanced.md",
                "decklist-annotated.md", "moxfield.txt", "swaps.md")


def render_all(deck: Deck, *, cards: dict[str, CardRecord] | None = None,
               previous: Deck | None = None, prices: dict[str, float] | None = None,
               stats: dict[str, Any] | None = None) -> dict[str, str]:
    """The deliverables as text, written nowhere. Raises `DraftDeck`.

    Split out of `write_all` when the API learned to build (the gap that
    function's own docstring predicted). The API may not write to a filesystem
    -- deck-facing endpoints go through a `DeckSource`, which is a locator and
    may not be a disk at all -- so generation had to stop being the same act as
    storage. Everything above this line is a pure function of the deck; below
    it is somebody's storage.

    `swaps.md` is present only when `previous` is, which is why the return is a
    dict rather than a fixed tuple: a first build has nothing to diff.

    The snapshot is placed last, and `write_all` relies on dict order to write
    it last. That ordering used to be the whole guard -- a refusal partway
    through left the old baseline in place -- and it is now belt and braces,
    because rendering raises before a single byte is stored.
    """
    if deck.stage == "draft":
        _refuse_draft(deck)

    files = {
        "primer-quick.md": quick_primer(deck, stats),
        "primer-advanced.md": advanced_primer(deck, stats),
        "decklist-annotated.md": annotated_decklist(deck, cards),
        "moxfield.txt": moxfield_txt(deck),
    }
    if previous is not None:
        files["swaps.md"] = swap_list(deck, previous, cards, prices)
    files[SNAPSHOT] = deck.dump()
    return files


def _refuse_draft(deck: Deck) -> None:
    """Name the cards still owed a rationale, then refuse."""
    pending = deck.unjustified
    shown = ", ".join(c.name for c in pending[:8])
    more = f", and {len(pending) - 8} more" if len(pending) > 8 else ""
    detail = (f"{len(pending)} card(s) still need a `why`: {shown}{more}"
              if pending else
              "every card is justified -- set `stage: curated` to promote it")
    raise DraftDeck(
        f"{deck.slug} is a draft, and the artifacts are the shareable "
        f"surface. {detail}")


def write_all(deck: Deck, outdir: str | Path, *, cards: dict[str, CardRecord] | None = None,
              previous: Deck | None = None, prices: dict[str, float] | None = None,
              stats: dict[str, Any] | None = None) -> list[Path]:
    """Write the five deliverables to a directory. Raises `DraftDeck` for a draft.

    The filesystem half, and since the API learned to build, the CLI's half
    alone. The refusal now lives in `render_all` so that both callers still
    inherit it -- that was always the reason it was not in the CLI ("the API
    will generate artifacts eventually, and a rule that only the terminal
    enforces is a rule with a hole in it"), and the prediction came true on
    2026-08-21. It was right about the hole and wrong about where the rule
    would have to sit: the API stores through a `DeckSource`, not a path, so
    the shared rule had to move up into the part that renders rather than the
    part that writes.

    This is deliberately *not* something `--force` can override, and the
    distinction is worth being precise about. `--force` overrides the gate's
    errors, which are things the deck got wrong. A draft is not wrong; it is
    unfinished, and it says so in the file. The way out is to write the missing
    rationales and promote it, which is an honest edit to the source of truth
    rather than a flag. Same argument as ADR 8: a primer for a deck nobody has
    reasoned about is worse than no primer, because it looks exactly like one
    for a deck somebody did.
    """
    # Rendered first, entirely, before anything is written: a `DraftDeck` or a
    # failure inside any one deliverable now leaves the directory exactly as it
    # was, including the old baseline the next `swaps.md` diffs against.
    files = render_all(deck, cards=cards, previous=previous, prices=prices,
                       stats=stats)
    return store(files, outdir)


def store(files: Mapping[str, str], outdir: str | Path) -> list[Path]:
    """Write a rendered build into a directory, and prune what it did not make.

    The pruning is the part worth explaining. A build with no baseline writes
    no `swaps.md`, so a rebuild used to leave the *previous* build's swap list
    sitting in the directory, describing a diff that no longer exists -- an
    artifact that is not merely stale but wrong, and indistinguishable from a
    current one. Found on 2026-08-21 by the parity test across the two deck
    sources: the memory tier replaced the set and the file tier merged into
    it, and only one of them could be right.

    Only `DELIVERABLES` are pruned. Anything else a person has put in that
    directory is theirs and is left alone -- the snapshot included, which is
    written by this same call and must survive a build that produces no
    `swaps.md`.

    Shared by `write_all` and `decks.source.FileDeckSource.write_artifacts` so
    the CLI and the API cannot disagree about what a rebuild leaves behind.
    """
    out = Path(outdir)
    out.mkdir(parents=True, exist_ok=True)
    written: list[Path] = []
    # `render_all` places the snapshot last and this loop preserves that order.
    for name, content in files.items():
        path = out / name
        path.write_text(content, encoding="utf-8")
        written.append(path)
    for name in DELIVERABLES:
        if name not in files:
            (out / name).unlink(missing_ok=True)
    return written
