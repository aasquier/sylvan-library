"""Card-art motion, derived at runtime and never committed (ADR 32).

The animist's recipes, provenance, licence gate and committed outputs stay
closed to Wizards' art — that is ADR 29 rule 3 and it stands. What this
package does is use the animist's *pure functions* as a library: it fetches
a card's art crop at build time on the dev machine, derives motion (a depth
map and a parallax loop), and caches the result under `data/cache/`, which
is gitignored locally and the volume when deployed. Git never holds a byte
of it; the app serves it as part of the same free fan content that already
hotlinks the still, with the artist credited beside it.
"""

from __future__ import annotations


def depth_available() -> bool:
    """Whether the `depth` extra is installed — torch and the model loader.

    The same shape as `animist.available()`: entry points ask this before
    touching the heavy modules, so a base install gets a sentence instead
    of a traceback.
    """
    try:
        import torch  # noqa: F401
        import transformers  # noqa: F401
    except ImportError:
        return False
    return True
