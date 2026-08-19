"""The catalogue of small wrongnesses, and where in a file each one fits.

Every operator here is a mistake a person actually makes -- an `<` that should
have been `<=`, an `and` that should have been `or`, a guard clause deleted
during a refactor because it looked redundant. The question the harness asks
is not "is this code correct" but **"would the suite notice if it stopped
being?"**, and a mutation nobody's test catches names the exact weak spot,
which is worth more than any coverage percentage.

Mutations are applied by **text surgery at the node's own column span**, never
by unparsing the tree. Unparsing is easier and wrong for this job: it rewrites
the whole file, so every line number moves, and a test that fails because a
traceback shifted would be recorded as having *caught* the mutation. A false
kill overstates the suite, which is the one direction this tool must never
err in.
"""

from __future__ import annotations

import ast
from collections.abc import Iterator
from dataclasses import dataclass

#: `<` for `<=` and so on -- an off-by-one at a boundary, the classic.
COMPARISONS: dict[type[ast.cmpop], str] = {
    ast.Lt: "<", ast.LtE: "<=", ast.Gt: ">", ast.GtE: ">=",
    ast.Eq: "==", ast.NotEq: "!=", ast.Is: "is", ast.IsNot: "is not",
    ast.In: "in", ast.NotIn: "not in",
}
_COMPARISON_SWAP = {
    "<": "<=", "<=": "<", ">": ">=", ">=": ">",
    "==": "!=", "!=": "==", "is": "is not", "is not": "is",
    "in": "not in", "not in": "in",
}
_ARITHMETIC_SWAP = {"+": "-", "-": "+", "*": "//", "/": "*"}
_BOOL_SWAP = {"and": "or", "or": "and"}


@dataclass(frozen=True)
class Mutation:
    """One edit: a span of one line, what is there, and what to put instead."""

    module: str
    #: Path relative to `src/`, so the harness can find the file in a shadow.
    relpath: str
    line: int                   #: 1-indexed, as a traceback would say it
    col: int
    end_col: int
    was: str
    now: str
    operator: str

    def describe(self) -> str:
        return (f"{self.relpath}:{self.line} [{self.operator}] "
                f"{self.was!r} -> {self.now!r}")

    def apply(self, source: str) -> str:
        """The file with this one edit made, and nothing else touched."""
        lines = source.splitlines(keepends=True)
        row = lines[self.line - 1]
        found = row[self.col:self.end_col]
        if found != self.was:
            raise ValueError(
                f"{self.relpath}:{self.line} moved: expected {self.was!r}, "
                f"found {found!r}")
        lines[self.line - 1] = row[:self.col] + self.now + row[self.end_col:]
        return "".join(lines)


def find(source: str, *, module: str, relpath: str) -> list[Mutation]:
    """Every mutation this catalogue can make to one file.

    Multi-line expressions are skipped rather than handled. A comparison whose
    operands sit on different lines needs a token scan across a continuation,
    and the harness has thousands of single-line sites to sample from -- the
    complexity would buy nothing but a wider net over the same fish.
    """
    tree = ast.parse(source)
    lines = source.splitlines()
    out: list[Mutation] = []
    for node in ast.walk(tree):
        out.extend(_from_node(node, lines, module=module, relpath=relpath))
    return sorted(out, key=lambda m: (m.line, m.col, m.operator))


def _from_node(node: ast.AST, lines: list[str], *, module: str,
               relpath: str) -> Iterator[Mutation]:
    def make(line: int, col: int, end: int, was: str, now: str,
             op: str) -> Mutation:
        return Mutation(module=module, relpath=relpath, line=line, col=col,
                        end_col=end, was=was, now=now, operator=op)

    if isinstance(node, ast.Compare) and len(node.ops) == 1:
        token = COMPARISONS.get(type(node.ops[0]))
        swap = _COMPARISON_SWAP.get(token or "")
        span = _token_between(lines, node.left, node.comparators[0], token)
        if token and swap and span:
            yield make(*span, token, swap, "comparison")

    elif isinstance(node, ast.BoolOp) and len(node.values) >= 2:
        token = "and" if isinstance(node.op, ast.And) else "or"
        span = _token_between(lines, node.values[0], node.values[1], token)
        if span:
            yield make(*span, token, _BOOL_SWAP[token], "boolean")

    elif isinstance(node, ast.BinOp):
        token = {ast.Add: "+", ast.Sub: "-", ast.Mult: "*",
                 ast.Div: "/"}.get(type(node.op))
        swap = _ARITHMETIC_SWAP.get(token or "")
        span = _token_between(lines, node.left, node.right, token)
        if token and swap and span:
            yield make(*span, token, swap, "arithmetic")

    elif isinstance(node, ast.Constant) and _single_line(node):
        if node.value is True or node.value is False:
            yield make(node.lineno, node.col_offset, node.end_col_offset or 0,
                       str(node.value), str(not node.value), "constant")
        elif isinstance(node.value, int) and not isinstance(node.value, bool):
            yield make(node.lineno, node.col_offset, node.end_col_offset or 0,
                       _literal(lines, node), str(node.value + 1), "boundary")

    elif isinstance(node, ast.If) and _guard_shaped(node) \
            and _single_line(node.test):
        # Deleting a guard is the refactor mistake, so the mutation is to make
        # the guard never fire rather than to remove the statement -- same
        # behaviour, and the file still parses at the same line numbers.
        yield make(node.test.lineno, node.test.col_offset,
                   node.test.end_col_offset or 0, _literal(lines, node.test),
                   "False", "guard")


def _guard_shaped(node: ast.If) -> bool:
    """An `if` whose whole body is one early exit -- a guard clause."""
    return (not node.orelse and len(node.body) == 1
            and isinstance(node.body[0],
                           ast.Return | ast.Raise | ast.Continue | ast.Break))


def _single_line(node: ast.AST) -> bool:
    return getattr(node, "lineno", 0) == getattr(node, "end_lineno", -1)


def _literal(lines: list[str], node: ast.AST) -> str:
    row = lines[node.lineno - 1]                                    # type: ignore[attr-defined]
    return row[node.col_offset:node.end_col_offset]                 # type: ignore[attr-defined]


def _token_between(lines: list[str], left: ast.AST, right: ast.AST,
                   token: str | None) -> tuple[int, int, int] | None:
    """Where the operator sits between two operands, on one line, or None.

    Scanned rather than assumed: `a  <  b` and `a<b` put the token in
    different columns, and the AST records the operands' spans but not the
    operator's. Only exact-token matches count, so the `<` inside `a < b<c`
    resolves to the right one because the search is bounded by the operands.
    """
    if token is None:
        return None
    lineno = getattr(left, "end_lineno", None)
    if lineno is None or lineno != getattr(right, "lineno", None):
        return None
    row = lines[lineno - 1]
    start, stop = left.end_col_offset or 0, right.col_offset            # type: ignore[attr-defined]
    gap = row[start:stop]
    # `not in` and `is not` contain a space, so a plain find is enough; and a
    # gap can hold a comment (`a  # why\n < b`) only across lines, excluded above.
    idx = gap.find(token)
    if idx < 0:
        return None
    col = start + idx
    return (lineno, col, col + len(token))
