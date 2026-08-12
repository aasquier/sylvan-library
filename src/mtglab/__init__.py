"""mtg-lab: local Magic: the Gathering deckbuilding, simulation and shopping toolkit."""
from importlib.metadata import PackageNotFoundError, version

try:
    __version__ = version("mtg-lab")
except PackageNotFoundError:  # running from a checkout without an install
    __version__ = "0.0.0.dev0"
