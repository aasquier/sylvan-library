"""A JSON value reduced to its *shape*: the keys and the kinds, never the data.

The goldens under `golden/` pin shapes rather than bodies because a body
carries ids, timestamps and whatever the pool happened to hold, while the
thing a second implementation has to reproduce is the structure the frontend
reads: which keys, of which kind, how nested. A shape is itself JSON, so it
diffs in a review and a Go reader can compute the same one.

Kinds are `string`, `number`, `bool`, `null`, an object (a dict of key ->
shape) or an array (a one-element list holding the merged shape of every
element, or `[]` when empty). `int` and `float` both read as `number` on
purpose: JSON has one number and Go's encoder writes an integral float as
`1`, so an `int`/`float` split would report drift where the wire shows none.

Lists are heterogeneous in two known places -- FastAPI's 422 `detail` array,
whose entries do not all carry `ctx`, and any list of objects with a nullable
field -- so element shapes are **merged**: a key present in some elements
but not all is written with a trailing `?`, and a scalar that is sometimes
one kind and sometimes another becomes their sorted union, `null|string`.
Merging is deterministic, so the same body always yields the same shape.
"""

from __future__ import annotations

from typing import Any

Shape = Any  # a str, a dict[str, Shape], or a list[Shape] of length <= 1

#: A shape that matches anything; used for the keys a case declares volatile
#: (a job's `status` at submission, which may already be `done` in-process).
ANY = "*"


def shape(value: Any) -> Shape:
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "bool"
    if isinstance(value, (int, float)):
        return "number"
    if isinstance(value, str):
        return "string"
    if isinstance(value, list):
        merged: Shape | None = None
        for item in value:
            merged = shape(item) if merged is None else merge(merged, shape(item))
        return [] if merged is None else [merged]
    if isinstance(value, dict):
        return {str(k): shape(v) for k, v in sorted(value.items())}
    raise TypeError(f"not a JSON value: {type(value).__name__}")


def _kind(s: Shape) -> str:
    if isinstance(s, dict):
        return "object"
    if isinstance(s, list):
        return "array"
    return s


def merge(a: Shape, b: Shape) -> Shape:
    """The shape that describes anything either `a` or `b` describes."""
    if a == b:
        return a
    if isinstance(a, dict) and isinstance(b, dict):
        out: dict[str, Shape] = {}
        names = {k.rstrip("?") for k in a} | {k.rstrip("?") for k in b}
        for name in sorted(names):
            ka, kb = _lookup(a, name), _lookup(b, name)
            if ka is None or kb is None:
                present = a[ka] if ka is not None else b[kb]  # type: ignore[index]
                out[name + "?"] = present
                continue
            optional = ka.endswith("?") or kb.endswith("?")
            out[name + ("?" if optional else "")] = merge(a[ka], b[kb])
        return out
    if isinstance(a, list) and isinstance(b, list):
        if not a:
            return b
        if not b:
            return a
        return [merge(a[0], b[0])]
    # Different kinds: a sorted union of their names.
    kinds = set(_kind(a).split("|")) | set(_kind(b).split("|"))
    return "|".join(sorted(kinds))


def _lookup(table: dict[str, Shape], name: str) -> str | None:
    """The key under which `name` sits in a merged dict, `?`-suffixed or not."""
    if name in table:
        return name
    if name + "?" in table:
        return name + "?"
    return None


def mask(s: Shape, volatile: tuple[str, ...]) -> Shape:
    """`s` with the named keys replaced by `ANY`, wherever they appear.

    For the fields a case cannot pin: a job's progress at the moment of
    submission differs between an in-process client, which may see it
    finished, and a socket, which usually sees it queued; a job *list* holds
    whatever earlier cases left behind. The key must still be *present* --
    only its kind is left open -- and it is masked at every depth, so the
    same names serve a job and a list of jobs.
    """
    if not volatile:
        return s
    if isinstance(s, dict):
        return {k: (ANY if k.rstrip("?") in volatile else mask(v, volatile))
                for k, v in s.items()}
    if isinstance(s, list):
        return [mask(item, volatile) for item in s]
    return s
