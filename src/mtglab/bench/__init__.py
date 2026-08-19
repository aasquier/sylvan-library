"""The measuring shelf: what the app costs, and where the cost actually is.

Three tools, and the reason each exists is a bug that got past a checklist.

`targets.py` declares what is worth timing -- endpoints and the library calls
under them -- so a run measures the same things this run and next quarter's,
and a number in `docs/polish/LEDGER.md` means something a year later.

`run.py` samples them and, for anything slow, splits the time in two. **Wall
clock minus Python-level time is the native budget**, and that split is the
whole point: the 80ms card search showed *0.4ms of Python* because the time
was inside DuckDB's C code, while a 200ms deck shelf was 162ms of failed
`import pandas` inside the import machinery. One number cannot tell those
apart, and a checklist line saying "record the response time" found neither.

`profile.py` is the follow-through: a cProfile table for one target, sorted so
the answer is the top line. A load probe finds *which* endpoint; only a
profile finds *why*, and guessing the why in between is how the import storm
was misattributed to YAML for three days.
"""

from mtglab.bench.profile import Profile, profile_target
from mtglab.bench.run import Sample, run_suite
from mtglab.bench.targets import Target, suite

__all__ = ["Profile", "Sample", "Target", "profile_target", "run_suite",
           "suite"]
