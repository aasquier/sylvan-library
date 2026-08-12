"""mtg-lab: local Magic: the Gathering deckbuilding, simulation and shopping toolkit."""


def __getattr__(name: str) -> str:
    # PEP 562, and it is about startup cost rather than style: importing
    # importlib.metadata eagerly here drags email, inspect and zipfile into
    # every `import mtglab`, measured at ~55ms -- roughly doubling CLI
    # startup -- for a value whose only reader is `create_app`. Deferring it
    # means the one consumer pays ~3ms after FastAPI has imported those
    # modules anyway, and everything else pays nothing.
    if name == "__version__":
        from importlib.metadata import PackageNotFoundError, version
        try:
            resolved = version("mtg-lab")
        except PackageNotFoundError:  # running from a checkout, no install
            resolved = "0.0.0.dev0"
        globals()["__version__"] = resolved
        return resolved
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
