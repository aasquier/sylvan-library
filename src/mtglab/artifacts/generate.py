"""Generate the five deliverables from deck.yaml.

Required for every new deck and every refactor:
  1. primer-quick.md      -- one page, get someone playing
  2. primer-advanced.md   -- lines, sequencing, matchups, failure modes
  3. decklist-annotated.md-- the 99 with a reason for every card
  4. moxfield.txt         -- bulk import
  5. swaps.md             -- only when the deck changed (out/in + shopping)

Generation is deterministic: same deck.yaml, same output. That is the point.
The prose that only a human or a conversation can supply -- strategy, lines,
matchup notes -- lives in `notes:` in the deck file, so it survives regeneration
instead of being retyped each time.
"""

from __future__ import annotations

from datetime import date
from pathlib import Path
from typing import Any

from mtglab.decks.model import CATEGORIES, Deck

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

def annotated_decklist(deck: Deck, cards: dict | None = None) -> str:
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

def swap_list(deck: Deck, previous: Deck, cards: dict | None = None,
              prices: dict[str, float] | None = None) -> str:
    """Diff two versions of a deck into an out/in list plus a shopping list.

    `previous` is normally the deck as of the last git commit, which is why the
    deck file being in git matters -- the swap document is a computed diff, not
    a hand-kept changelog.
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

def write_all(deck: Deck, outdir: str | Path, *, cards: dict | None = None,
              previous: Deck | None = None, prices: dict[str, float] | None = None,
              stats: dict[str, Any] | None = None) -> list[Path]:
    out = Path(outdir)
    out.mkdir(parents=True, exist_ok=True)
    written: list[Path] = []

    files = {
        "primer-quick.md": quick_primer(deck, stats),
        "primer-advanced.md": advanced_primer(deck, stats),
        "decklist-annotated.md": annotated_decklist(deck, cards),
        "moxfield.txt": moxfield_txt(deck),
    }
    if previous is not None:
        files["swaps.md"] = swap_list(deck, previous, cards, prices)

    for name, content in files.items():
        path = out / name
        path.write_text(content, encoding="utf-8")
        written.append(path)
    return written
