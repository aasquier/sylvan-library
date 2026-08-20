"""Mutation testing: does the suite actually hold what it claims to hold?

Coverage says a line ran. This asks the harder question -- **if that line were
wrong, would anything go red?** -- by making it wrong on purpose in a scratch
copy of the package and running the tests that ought to defend it.

`operators.py` is the catalogue of wrongnesses; `harness.py` draws a seeded
sample, applies them one at a time, and counts the survivors. (`harness` and
not `run`, because `mutate.run` is the function this package exports, and a
submodule of the same name shadows it — which it did, twice, before the
rename.) A survivor is a finding
naming an exact weak spot, which is worth more than a percentage. It is also
sometimes an *equivalent* mutant that no test could ever kill; telling those
apart is reading work the tool does not pretend to do for you.
"""

from mtglab.mutate.harness import (
    TARGETS,
    Report,
    Result,
    catalogue,
    missing_tests,
    run,
    select,
    shadow,
)
from mtglab.mutate.operators import Mutation, find

__all__ = ["TARGETS", "Mutation", "Report", "Result", "catalogue", "find",
           "missing_tests", "run", "select", "shadow"]
