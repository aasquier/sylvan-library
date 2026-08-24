"""The toolbox suite's one fixture concern: import the tree being tested.

The app's conftest was a wall of guards -- app.db detectors, ledger stubs,
Hypothesis profiles. None of that crosses, because none of its hazards
exists here: the toolbox has no database of its own, no model ledger, and no
Hypothesis tests. Every test already points the config at a pytest scratch
directory with `config.use_paths`, which is the whole isolation story a
picture pipeline needs.

What does cross is the worktree lesson. This repository runs sessions in git
worktrees that borrow the main checkout's venv, so a bare import of
`animist` could resolve somewhere other than the tree under test if these
packages were ever installed into that venv. Anchoring the tools root at the
head of `sys.path` -- before pytest collects a single test -- makes the
suite test the code beside it, always.
"""

import sys
from pathlib import Path

TOOLS = Path(__file__).resolve().parents[1]
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))
