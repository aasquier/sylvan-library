"""The card-coverage pre-flight: does Forge implement every card in this deck?

**This is the non-negotiable piece.** Forge implements ~99.8% of cards legal in
Commander, which is high enough to be dangerous: a deck can lose a card and
still produce a winner, a win rate and a turn count that all look fine. Every
number after a silent drop is wrong, and nothing downstream can tell. So the
question "what did Forge actually put in the deck" gets answered *before* a
game runs, from data rather than from a log line, and `run.py` will not start a
simulation without it.

Ground truth is `res/cardsfolder/cardsfolder.zip` in the distribution: one
small text file per implemented card, each carrying one or more `Name:` lines.
That is the same data the engine loads at startup, so agreeing with it is
agreeing with Forge itself. 33,587 files yield 34,532 names in about two
seconds, cached per (path, mtime) thereafter.

Two things the index taught us, both of which the exporter depends on:

* Card names in the index are always **face names**. A modal DFC contributes
  two entries ("Barkchannel Pathway" and "Tidechannel Pathway"), never the
  combined "Barkchannel Pathway // Tidechannel Pathway" that Scryfall reports.
  Split cards are the same -- "Alive" and "Well", not "Alive // Well".
* So a Scryfall-shaped `A // B` name has to be resolved to a face before it can
  be looked up, and `resolve` is the one place that happens. The exporter
  writes exactly what the pre-flight verified, which is the property that makes
  a clean report mean something.
"""

from __future__ import annotations

import zipfile
from dataclasses import dataclass, field
from pathlib import Path

from mtglab import config
from mtglab.decks.model import Deck

CARDSFOLDER = Path("res") / "cardsfolder" / "cardsfolder.zip"


class ForgeNotInstalled(RuntimeError):
    """Raised when the Forge distribution is missing or incomplete.

    A plain exception, for the same reason `CorpusRequired` is one: this is
    reachable from an API worker thread as well as from the CLI, and
    `sys.exit` in a thread is a confusing way to fail.
    """


def cardsfolder_path(forge_home: Path | None = None) -> Path:
    home = Path(forge_home) if forge_home is not None else config.forge_home()
    path = home / CARDSFOLDER
    if not path.exists():
        raise ForgeNotInstalled(
            f"no Forge card data at {path} -- set MTGLAB_FORGE_HOME to an "
            f"unpacked Forge distribution")
    return path


_INDEX_CACHE: dict[tuple[str, int, int], frozenset[str]] = {}


def implemented_names(forge_home: Path | None = None) -> frozenset[str]:
    """Every card name Forge implements, read from its own card scripts.

    Cached on (path, mtime, size) rather than on the path alone, so upgrading
    Forge in place invalidates it instead of serving a stale answer -- the
    failure mode that would matter most, since a Forge upgrade is exactly when
    coverage changes.
    """
    path = cardsfolder_path(forge_home)
    stat = path.stat()
    key = (str(path), int(stat.st_mtime), stat.st_size)
    cached = _INDEX_CACHE.get(key)
    if cached is not None:
        return cached

    names: set[str] = set()
    with zipfile.ZipFile(path) as archive:
        for info in archive.infolist():
            if info.is_dir() or not info.filename.endswith(".txt"):
                continue
            # `replace` rather than strict: a decoding error in one card script
            # should cost that card, not the whole pre-flight.
            text = archive.read(info).decode("utf-8", "replace")
            for line in text.splitlines():
                if line.startswith("Name:"):
                    names.add(line[5:].strip())
    index = frozenset(names)
    _INDEX_CACHE[key] = index
    return index


def resolve(name: str, index: frozenset[str]) -> str | None:
    """The name Forge knows this card by, or None if it implements no face.

    Scryfall's combined `A // B` name never appears in Forge's index, so the
    faces are tried in order. Front first: a modal DFC or a transforming
    permanent is the front face as far as a decklist is concerned, and for a
    split card either half names the same physical card.

    Returning the *resolved* name rather than a bool is what lets the exporter
    and this check agree by construction.
    """
    if name in index:
        return name
    if " // " in name:
        for face in name.split(" // "):
            face = face.strip()
            if face in index:
                return face
    return None


@dataclass
class CoverageReport:
    """What Forge would and would not put in the deck.

    `missing` is the answer to the question this module exists for. It being
    empty is the only condition under which a simulation result means anything.
    """

    slug: str
    checked: int = 0
    # deck.yaml name -> the name written into the .dck. Equal for all but
    # double-faced and split cards.
    resolved: dict[str, str] = field(default_factory=dict)
    missing: list[str] = field(default_factory=list)

    @property
    def ok(self) -> bool:
        return not self.missing

    @property
    def renamed(self) -> list[tuple[str, str]]:
        """Cards whose `.dck` line differs from the deck.yaml name."""
        return [(k, v) for k, v in sorted(self.resolved.items()) if k != v]

    def summary(self) -> str:
        if self.ok:
            line = f"{self.slug}: all {self.checked} cards implemented by Forge"
            if self.renamed:
                line += f" ({len(self.renamed)} resolved to a face name)"
            return line
        return (f"{self.slug}: Forge does not implement "
                f"{len(self.missing)} of {self.checked} cards:\n  " +
                "\n  ".join(self.missing))


def check(deck: Deck, index: frozenset[str] | None = None,
          forge_home: Path | None = None) -> CoverageReport:
    """Pre-flight one deck. Commander and companion are checked too.

    Distinct names, not the 99 by quantity: thirty-six Forests missing would be
    one problem, not thirty-six, and a report should read like the fix.
    """
    if index is None:
        index = implemented_names(forge_home)

    wanted = list(deck.commander)
    if deck.companion:
        wanted.append(deck.companion)
    wanted.extend(c.name for c in deck.cards)

    report = CoverageReport(slug=deck.slug)
    seen: set[str] = set()
    for name in wanted:
        if name in seen:
            continue
        seen.add(name)
        report.checked += 1
        forge_name = resolve(name, index)
        if forge_name is None:
            report.missing.append(name)
        else:
            report.resolved[name] = forge_name
    return report
