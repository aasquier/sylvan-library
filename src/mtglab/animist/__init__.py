"""The animist: the asset pipeline that wakes still images into scenery.

Named for Nissa, the Animist, who wakes the land itself. What this package
wakes is the site's decoration: free-use photographs and paintings, fetched
from sources that publish their licences, turned into the committed WebP
assets the app serves.

[ADR 29](../../../docs/adr/0029-an-asset-is-committed-only-with-a-recipe.md)
is the boundary this package exists to hold, in three sentences. **An asset is
committed only with a recipe and provenance.** **The licence gate blocks and
has no override** — a source whose licence the gate cannot confirm as free is
refused before a single byte of it is transformed. **Wizards' art is animated
at runtime only, never baked into a committed file** — the Wheel of Fortune
spins as CSS over a hotlink, and nothing here may be pointed at a card image.

The shape mirrors `mtglab.claude`: Pillow rides in its own extra, the modules
that touch pixels import PIL inside functions, and `available()` answers
whether it is installed — so recipe validation and the licence gate work on a
base install, and the deployed container never notices this package exists.
"""

from mtglab.animist.recipe import (
    KNOWN_OPS,
    PROVIDERS,
    Recipe,
    RecipeError,
    load_recipe,
)
from mtglab.animist.sources import LicenceConfirmation, LicenceRefused

__all__ = [
    "KNOWN_OPS", "PROVIDERS", "Recipe", "RecipeError", "load_recipe",
    "LicenceConfirmation", "LicenceRefused",
    "available",
]


def available() -> bool:
    """Whether Pillow is installed — the `animist` extra's whole question.

    Mirrors `claude.available()`: the answer an entry point checks before
    reaching for the modules that touch pixels, so a base install gets "the
    animist extra is not installed" instead of an ImportError traceback.
    """
    try:
        import PIL  # noqa: F401
    except ImportError:
        return False
    return True
