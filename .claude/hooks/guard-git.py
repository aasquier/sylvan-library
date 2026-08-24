#!/usr/bin/env python3
"""Refuse the two git commands this repository has actually been burned by.

CLAUDE.md states both as prose, and prose is what this polish pass kept finding
to be insufficient: the `dev` extra "includes all of it" did not, and
`web/README.md`'s "no non-null assertions outside test files" went unenforced
for months while four drifted in. These two rules have a *measured* cost -- one
swept a test deck into a "docs only" pull request because `decks/` is the app's
live data directory, the other corrupted this repository's index -- so they get
a mechanism rather than a paragraph.

Reads a `PreToolUse` payload on stdin. Stays silent (allow) or prints a deny
decision. Deliberately narrow: it refuses four spellings and nothing else,
because a guard that fires on a command somebody legitimately needed is a guard
that gets switched off.

**Python rather than `jq`, and that was a finding rather than a preference.**
The first draft of this used `jq`, which is not installed on this machine at
all -- so it parsed nothing, denied nothing, and its own allow-cases "passed".
A guard whose failure mode is silent permission is worse than no guard, because
it reads as protection. Python 3 ships with macOS and every CI image here, and
`shlex` tokenises quotes properly, so `echo "git add -A"` is a mention rather
than an invocation.
"""

from __future__ import annotations

import json
import shlex
import sys

ADD_REASON = """Refused: this repository never stages with `git add -A`, `--all` or `.`.

`decks/` is the app's live data directory -- running the app writes real
deck.yaml files into the working tree -- so a blanket stage sweeps whatever the
app happened to create in with the change. It has already put a test deck into
a "docs only" pull request.

Stage explicit paths instead:

    git add go/internal/api/api.go go/internal/api/api_test.go"""

STASH_REASON = """Refused: `git stash` corrupts the index on this repository.

It has happened here, and the recovery cost real time. Commit the work in
progress instead -- a WIP commit is cheap, amendable, and rebases cleanly:

    git add <explicit paths> && git commit -m 'WIP: <what you were doing>'

`git stash list` and `git stash show` are read-only and are allowed."""

#: Tokens shlex hands back for shell operators, which end one command and start
#: the next. Without this, `git add src/x.py && git log` would be judged as a
#: single command whose arguments happen to include the word `git`.
SEPARATORS = {";", "&&", "||", "|", "&", "(", ")", "\n"}

#: `git -C <path> add -A` still stages everything, so the global options have to
#: be stepped over to find the subcommand. The two that take a value are the
#: only ones that would otherwise swallow it.
GLOBAL_OPTS_WITH_VALUE = {"-C", "-c", "--git-dir", "--work-tree", "--namespace"}

BLANKET_ADD_ARGS = {"-A", "--all", "."}
READ_ONLY_STASH = {"list", "show"}


def segments(command: str) -> list[list[str]]:
    """Tokenise a shell command line and split it on operators.

    Quote-aware, which is the point: a command that merely *mentions* one of
    these spellings inside a string is not running it.
    """
    lexer = shlex.shlex(command, posix=True, punctuation_chars=True)
    lexer.whitespace_split = True
    out: list[list[str]] = [[]]
    for token in lexer:
        if token in SEPARATORS or all(ch in ";&|()" for ch in token):
            out.append([])
        else:
            out.append(out.pop() + [token])
    return [segment for segment in out if segment]


def verdict(command: str) -> str | None:
    """The reason to refuse this command line, or `None` to allow it."""
    for segment in segments(command):
        if segment[0] != "git":
            continue
        rest = segment[1:]
        while rest and rest[0].startswith("-"):
            head = rest.pop(0)
            if head in GLOBAL_OPTS_WITH_VALUE and rest:
                rest.pop(0)
        if not rest:
            continue
        subcommand, args = rest[0], rest[1:]
        if subcommand == "add" and any(a in BLANKET_ADD_ARGS for a in args):
            return ADD_REASON
        if subcommand == "stash" and (not args or args[0] not in READ_ONLY_STASH):
            return STASH_REASON
    return None


def main() -> int:
    try:
        payload = json.load(sys.stdin)
        command = payload.get("tool_input", {}).get("command", "")
    except Exception as exc:  # noqa: BLE001 -- see below
        # Loud rather than silent. This guard cannot judge what it cannot read,
        # and allowing is the only safe answer for availability -- but an
        # unreadable payload must never look like a clean pass.
        print(f"guard-git: could not read the hook payload: {exc}", file=sys.stderr)
        return 0

    if not command:
        return 0
    try:
        reason = verdict(command)
    except ValueError as exc:
        # shlex raises on an unbalanced quote. Not a command we can judge.
        print(f"guard-git: could not parse the command: {exc}", file=sys.stderr)
        return 0

    if reason is not None:
        json.dump({
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": reason,
            },
        }, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
