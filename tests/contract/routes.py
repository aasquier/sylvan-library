"""`routes.json`, loaded once, validated, and handed out typed.

The file is the one table; this module is the Python reader of it. A Go
reader does the same job from the same bytes, which is the whole point of the
file being JSON rather than the three dicts it replaced in
`tests/test_isolation.py`. Validation happens at load so a malformed entry
fails every test that imports this rather than silently shrinking a sweep --
the failure mode the sweep exists to prevent.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROUTES_FILE = HERE / "routes.json"

#: The four buckets a route can be filed under, in the order the sweeps
#: report them. `public` is a list in the file (it mirrors a frozenset in
#: code); the other three are path -> reason.
BUCKETS = ("public", "shared", "user_scoped", "admin")


@dataclass(frozen=True)
class Classification:
    """Every `/api` route, filed, plus what a sweep fills placeholders with."""

    admin_prefix: str
    public: frozenset[str]
    shared: dict[str, str]
    user_scoped: dict[str, str]
    admin: dict[str, str]
    placeholders: dict[str, str]

    @property
    def protected(self) -> set[str]:
        """Every route that must answer 401 without a session."""
        return set(self.shared) | set(self.user_scoped) | set(self.admin)

    @property
    def every(self) -> set[str]:
        return self.protected | set(self.public)

    def concrete(self, path: str) -> str:
        """A templated route path with something plausible in every segment.

        The values do not have to resolve to anything. Every sweep that uses
        this asserts on a refusal that happens *before* the handler runs, so a
        real slug and a nonsense one produce the same answer -- and one that
        produced a different answer would be the bug.
        """
        for placeholder, value in self.placeholders.items():
            path = path.replace(placeholder, value)
        return path


def load(path: Path = ROUTES_FILE) -> Classification:
    """Read and validate the file. Raises `ValueError` naming what is wrong."""
    data = json.loads(path.read_text(encoding="utf-8"))
    missing = [key for key in (*BUCKETS, "admin_prefix", "placeholders")
               if key not in data]
    if missing:
        raise ValueError(f"{path.name}: missing {missing}")

    public = data["public"]
    if not isinstance(public, list) or len(set(public)) != len(public):
        raise ValueError(f"{path.name}: `public` must be a list of distinct paths")
    buckets = {"public": set(public)}
    for name in BUCKETS[1:]:
        table = data[name]
        if not isinstance(table, dict):
            raise ValueError(f"{path.name}: `{name}` must map path -> reason")
        for route, reason in table.items():
            if not isinstance(reason, str) or len(reason) <= 15:
                raise ValueError(
                    f"{path.name}: {route} is filed as {name} without a reason")
        buckets[name] = set(table)

    seen: dict[str, str] = {}
    for name, paths in buckets.items():
        for route in paths:
            if not route.startswith("/api"):
                raise ValueError(f"{path.name}: {route} is not an /api route")
            if route in seen:
                raise ValueError(
                    f"{path.name}: {route} is filed as both {seen[route]} "
                    f"and {name}")
            seen[route] = name

    prefix = data["admin_prefix"]
    misfiled = sorted(r for r in buckets["admin"]
                      if not (r == prefix or r.startswith(prefix + "/")))
    if misfiled:
        raise ValueError(
            f"{path.name}: filed as admin but not under {prefix}: {misfiled}")
    strayed = sorted(r for name in ("public", "shared", "user_scoped")
                     for r in buckets[name]
                     if r == prefix or r.startswith(prefix + "/"))
    if strayed:
        raise ValueError(
            f"{path.name}: under {prefix} but not filed as admin: {strayed}")

    return Classification(
        admin_prefix=prefix,
        public=frozenset(public),
        shared=dict(data["shared"]),
        user_scoped=dict(data["user_scoped"]),
        admin=dict(data["admin"]),
        placeholders=dict(data["placeholders"]),
    )


ROUTES = load()
