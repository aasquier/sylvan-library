"""The Claude surface: opinions and research, never facts and never a `why`.

[ADR 14](../../../docs/adr/0014-python-decides-claude-advises.md) drew the line
-- deterministic Python owns everything with a right answer, Claude owns the
questions a card pool query cannot settle.
[ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
says what a surface *is*: a mode (prompt, tools, capabilities) plus a stance.

The pipe is a client, a model, and tool schemas over the read-only half of
`api/service.py`. On top of it sit the stance (the user's dial) and the modes,
of which there is currently one: the **rationale interview**, which asks about
a card's slot so the user can write its `why`. Every mode reaches the pool
through `tools.READ_ONLY` and nothing else -- deliberately the only door, so
the boundary that matters is one allowlist rather than a rule repeated in every
mode that gets added later.

The boundary, restated because it is the whole reason this package is shaped
this way: **no code path here may pass a model response into a card's `why`.**
Reading a rationale is fine and necessary -- arguing against a card's slot
means seeing what was claimed for it. Writing one is not, at any stance, ever.
`tests/test_claude_boundary.py` enforces that over the package's syntax tree
rather than over this paragraph.
"""

from mtglab.claude.client import MODEL, ClaudeUnavailable, available, check, connect
from mtglab.claude.interview import RATIONALE_INTERVIEW, CardNotInDeck, ask, brief
from mtglab.claude.modes import Mode, ModeExhausted, converse
from mtglab.claude.stance import (
    OFF,
    PRESETS,
    Stance,
    ceiling,
    default_for,
    describe,
    resolve,
)
from mtglab.claude.tools import READ_ONLY, run, schemas

# `connect()` rather than `client()`, matching `db.connect()` -- and because a
# module named `client` and a function named `client` re-exported beside it
# shadow each other, which is a confusing five minutes for whoever hits it.
__all__ = [
    "MODEL", "ClaudeUnavailable", "available", "check", "connect",
    "READ_ONLY", "run", "schemas",
    # The first mode, and the generic parts a second one will reuse. `ask` is
    # the whole public surface of the interview: it resolves the stance, makes
    # no call at `off`, and returns questions -- never a rationale.
    "RATIONALE_INTERVIEW", "CardNotInDeck", "Mode", "ModeExhausted",
    "ask", "brief", "converse",
    # The stance is stdlib-only on purpose: the dial has to be readable, and
    # settable to `off`, on an install with neither the SDK nor a key.
    "OFF", "PRESETS", "Stance", "ceiling", "default_for", "describe", "resolve",
]
