"""A reading of `deck.yaml`, for exactly the fields the motion sweep uses.

The app's deck model parsed the whole file -- the 99, the graveyard, the
labels, the notes -- because the app edited it. This toolbox only ever asks a
deck three questions: what is it called, who leads it, and which printing's
painting does its page show. So this is deliberately not the model: it is a
reading, three fields wide, with the model's own coercions reproduced for
those three fields so a file the app wrote answers here exactly as it
answered there. Everything else in the file passes through unread, which is
also the safety property -- a tool that cannot represent a deck cannot be
tempted to write one back.

The coercions, copied from `Deck.from_text` rather than re-derived:

- `slug` falls back to the directory name, because a hand-written file may
  not repeat it.
- `commander` accepts a bare string as a one-name list, the partner shape
  being a real list.
- `commander_art` is stringified and stripped, absent meaning "the pool's
  default printing".
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

#: The C loader when libyaml is compiled in, the pure-Python one otherwise.
#: `yaml.safe_load` always takes the Python path even with libyaml bound; the
#: app measured the difference at 36ms against 7ms per curated deck file.
#: Both loaders accept only the same safe subset, so nothing but speed changes.
SAFE_LOADER: type[yaml.SafeLoader] = getattr(yaml, "CSafeLoader", yaml.SafeLoader)


@dataclass(frozen=True)
class Deck:
    """One deck, as far as the motion sweep can see it."""

    slug: str
    commander: list[str]
    commander_art: str


def load(path: str | Path) -> Deck:
    """Read one `deck.yaml`. Raises what the filesystem or YAML raises --
    whether a missing deck is fatal is the caller's policy, and the CLI's
    `_load` turns it into the same sentence the app's CLI used."""
    path = Path(path)
    raw: Any = yaml.load(path.read_text(encoding="utf-8"), SAFE_LOADER) or {}

    commander = raw.get("commander") or []
    if isinstance(commander, str):
        commander = [commander]

    return Deck(
        slug=raw.get("slug") or path.parent.name or "",
        commander=list(commander),
        commander_art=str(raw.get("commander_art") or "").strip(),
    )
