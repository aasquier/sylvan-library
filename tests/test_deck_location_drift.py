"""No source comment may teach that a deck lives in git.

ADR 30 moved decks out of git and made `decks/` the app's own data directory.
Its entry in `docs/adr/README.md` says it "supersedes half a sentence of 1,
and it is the half everything since had leaned on" -- which was exactly right
and exactly the problem: the sentence was superseded and the leaning was not
swept. Fourteen comments across eleven files went on describing the old world
for two days, in four flavours:

- `deck.yaml` "is tracked in git";
- "deck history is git history", with `git log -p decks/<slug>/deck.yaml`
  offered as the swap record -- a command that now prints nothing;
- `swaps.md` diffing against the last commit, when it diffs against
  `artifacts/deck.last-built.yaml`, a snapshot the build keeps precisely
  because there is no revision;
- `git checkout` named as a deck's undo, when the undo is the graveyard
  (ADR 27) and a deleted deck's only copy is the directory `delete` moves
  aside.

The last two are not stale prose, they are wrong instructions: a session that
believes them looks for a diff that cannot exist and offers a recovery that
cannot work. One of the fourteen was printed to the terminal by
`mtglab decks log`.

This is the third time this class has been caught -- "deck history is git
history" shipped in two components for two ADRs' worth of time, found in
2026-08-16's polish run by driving the live surface -- so this run leaves a
tripwire rather than a fourth correction. A rule enforced by nothing drifts;
that is the whole thesis, and this file is it.

**Scope is code, not the record.** `src/mtglab` and `web/src` only. The ADRs
are immutable and ADR 1 is *titled* with the superseded claim, which is how a
decision log is supposed to work: the old reasoning stays and a new file
overrules it. Prose that narrates the change ("decks are no longer in git")
is fine anywhere; what is banned is a comment that still asserts the old
world as current fact.
"""

from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TREES = (ROOT / "src" / "mtglab", ROOT / "web" / "src")
SUFFIXES = {".py", ".ts", ".tsx"}

#: Each pattern is one superseded claim, written as it was actually found.
#: Deliberately narrow: this is a tripwire for the sentence that drifted, not
#: a ban on the word "git", which the bundle, the pool and the asset pipeline
#: all discuss correctly.
#:
#: Matched against the file **flattened to one line**, which is not a detail.
#: The first draft of this file searched line by line, and every offence it
#: was written for happened to fit on one -- so it passed, and a mutation test
#: that re-broke `model.py`'s docstring did not fire, because the restored
#: claim wrapped after "directory and". A guard aimed at prose has to read the
#: way prose is written: this codebase wraps its comments at 79 columns, so
#: any sentence long enough to be worth banning is a sentence that spans two
#: lines. The tripwire nearly shipped as decoration.
#: How far apart "deck" and the claim may sit and still be the same thought.
#: Generous, because these sentences are long and the alternative failed: the
#: first draft used 40 characters *and* forbade a full stop in between, which
#: `deck.yaml` supplies in its own name -- so the pattern could not even reach
#: across the phrase it was named for.
NEAR = 160


def about_a_deck(claim: str) -> re.Pattern[str]:
    """`claim` said of a deck, in either order."""
    return re.compile(
        rf"(?:deck.{{0,{NEAR}}}?{claim}|{claim}.{{0,{NEAR}}}?deck)", re.I)


FORBIDDEN = (
    # Affirmative forms only. A bare `in git` cannot tell the claim from its
    # denial, and the sentences that *correct* this drift -- "decks are not in
    # git (ADR 30)" -- are the ones most likely to sit next to the word deck.
    # Banning the topic would have failed every fix in the commit that added
    # this file. So: ban the verbs that only ever assert.
    (about_a_deck(r"(?:is |are |being |stay |stays )?"
                  r"(?:tracked|held|file-backed|committed|diffed) in git\b"),
     "a deck is not kept in git (ADR 30)"),
    (about_a_deck(r"\blives? in git\b"),
     "a deck does not live in git (ADR 30)"),
    (about_a_deck(r"in git,? which is the source of truth"),
     "`deck.yaml` is the source of truth on disk, not in git (ADR 30)"),
    (about_a_deck(r"last git commit"),
     "a deck has no commits to be as of (ADR 30)"),
    # Not "deck history is git history": the sentence that actually shipped
    # said *its* history, the antecedent a line above. Pronouns are why this
    # one drops the deck word entirely -- nothing else here has a history that
    # is git history.
    (re.compile(r"history is git history", re.I),
     "deck history is the activity log (ADR 28)"),
    (re.compile(r"deck'?s? git history", re.I),
     "deck history is the activity log (ADR 28)"),
    (re.compile(r"travels? with the deck through git", re.I),
     "a deck travels with nothing through git (ADR 30)"),
    (about_a_deck(r"git (?:checkout|log|diff)\b"),
     "no git command reads or restores a deck (ADR 27, ADR 30)"),
    (re.compile(r"file-backed in git", re.I),
     "the curated decks are file-backed on disk, not in git (ADR 30)"),
    # Added 2026-08-19, after the sweep above missed a live one. The History
    # tab's empty state read "Edits made before this log existed are in git,
    # not here" -- **rendered**, not a comment, and pinned by a Vitest
    # assertion on its own wrong words. It slipped through because the
    # affirmative list bans verbs (`tracked`, `held`, `committed`) and this
    # sentence used none of them: it put the *edits* in git rather than the
    # deck, which is the one thing ADR 28 says lives somewhere else entirely.
    #
    # Subject-anchored rather than deck-anchored, and that is what makes a
    # bare copula safe here. `about_a_deck` cannot be used -- "no deck is in
    # git" and "none of them is in git" are the file's own corrections, and a
    # `deck ... is in git` pattern flags both. Requiring an edit-word first
    # excludes them: neither sentence has one.
    (re.compile(r"\b(?:edits?|changes?|swaps?|history|revisions?|records?)"
                r"\b[^.]{0,80}?\b(?:is|are|was|were) in git\b", re.I),
     "what was done to a deck is the activity log, never git (ADR 28, ADR 30)"),
)


def sources() -> list[Path]:
    return sorted(p for tree in TREES for p in tree.rglob("*")
                  if p.suffix in SUFFIXES and ".test." not in p.name)


def test_there_are_sources_to_sweep() -> None:
    """A tripwire over an empty set passes forever and measures nothing."""
    assert len(sources()) > 100


def flatten(text: str) -> tuple[str, list[int]]:
    """The file as one line, and where each kept character came from.

    Runs of whitespace collapse to a single space so a wrapped sentence reads
    as a sentence. The offset list is what turns a match back into the line
    number a person needs to go and fix.
    """
    out: list[str] = []
    offsets: list[int] = []
    space = False
    for index, char in enumerate(text):
        if char.isspace():
            space = True
            continue
        if space and out:
            out.append(" ")
            offsets.append(index)
        space = False
        out.append(char)
        offsets.append(index)
    return "".join(out), offsets


def test_no_source_says_a_deck_lives_in_git() -> None:
    """One test over every file rather than one per file.

    The sweep found fourteen sites in eleven files at once; a parametrised
    version would have reported eleven failures for one mistake, and the
    person fixing it wants the list.
    """
    offences = []
    for path in sources():
        text = path.read_text()
        flat, offsets = flatten(text)
        for pattern, why in FORBIDDEN:
            for match in pattern.finditer(flat):
                line = text.count("\n", 0, offsets[match.start()]) + 1
                offences.append(f"{path.relative_to(ROOT)}:{line}: {why}\n"
                                f"    ...{match.group(0)}...")
    assert not offences, "\n" + "\n".join(offences)
