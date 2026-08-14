# 24. No Python autoformatter, and the lint rules hold the line instead

**Status:** Accepted · **Decided:** 2026-08-14 · **Recorded:** 2026-08-14

## Context

`ruff format` has been available for the whole life of this project and has
never been run. The 2026-08-14 quality pass widened `ruff check`'s rule set by
six groups and deliberately left the formatter out, recording the measurement
and deferring the decision rather than making it — which is the state this ADR
closes.

The question is not really "should the code be formatted." It is **what a
formatter is for**, and whether this repository has that problem. A formatter
buys three things:

1. It ends style arguments between contributors.
2. It removes style from code review.
3. It enforces a line-length and wrapping discipline that nothing else does.

The first two are worth a great deal on a team. This repository has one
contributor and no style arguments in its history. The third is the one that
had to be measured rather than assumed, because `[tool.ruff.lint]` selects
`E4`, `E7` and `E9` and **not `E501`** — nothing in the pipeline checks line
length at all.

## Options considered

### Adopt `ruff format` across the tree

Measured 2026-08-14 against 39,823 lines of Python in `src/` and `tests/`:

| | |
| --- | --- |
| Files reformatted | **101 of 111** |
| Lines changed | ~15,100 (9,816 added, 5,291 removed) |
| Net growth | **+4,525 lines, or +11%** |

Two of those changes are not style. They are the formatter overruling
decisions this repository made on purpose and wrote down:

- **It breaks the argparse table.** `pyproject.toml` carries a
  `per-file-ignores` entry giving `src/mtglab/cli.py` an `E702` exemption, with
  a comment explaining it: the subcommand wiring is a dense table, one command
  per line, `;` joining each parser to its first argument, and *"splitting it
  doubles the length of `main()` without making the command tree any clearer."*
  **The formatter does not read lint ignores**, so it splits every one of those
  lines — about twenty of them. The exemption stays in the file, now
  suppressing a rule that nothing can trigger.
- **It dedents the CLI's command table.** `cli.py`'s module docstring opens with
  the aligned list of every `mtglab` command and what it does. Docstring
  reindentation strips the leading whitespace off all twenty lines. That
  alignment is content — it is the reference table a reader of `--help` sees —
  and a formatter cannot tell it from accidental indentation.

### Adopt it and grow the exception list

`ruff format` honours `# fmt: off` / `# fmt: skip`, so both casualties above
could be fenced off. This was the closest call, and it loses on what it turns
the decision into: the value of a formatter is that it is unarguable, and a
formatter with hand-placed fences around the two most deliberate pieces of
formatting in the codebase is a formatter that has already conceded the
argument. It also leaves a reader unable to tell whether a given piece of
layout is intentional or merely un-fenced.

### Adopt `E501` instead, and keep formatting by hand

The narrow version of the third benefit: enforce a line length, let everything
else stay as written. Measured, and this is what settled it — **117 lines of
39,823 exceed 88 characters (0.3%)**, and of the 61 lines over 100 characters,
**60 are in `tests/tiny_pool.py`**, which holds real card oracle text as string
literals.

A formatter cannot split a long string, so those 60 lines survive `ruff format`
unchanged. The discipline a line-length rule would impose is already present,
and the one file that violates it is the one file no tool can fix.

### Keep formatting by hand (chosen)

## Decision

**No Python autoformatter.** `ruff check`'s seventeen rule groups hold the
line, and layout stays a thing a person chooses.

This is a decision about *this* repository and it turns on facts that could
change. The reasoning, so it can be re-run rather than re-argued:

- **The problem a formatter solves is not present.** One contributor, no style
  disputes, and 0.3% of lines over 88 characters without any rule requiring it.
- **The cost is not the diff, it is what the diff hides.** A 15,100-line
  mechanical change is unreviewable by construction, and this project has
  already written down what that costs: ENGINEERING §4 makes the same argument
  about burying fifty type-checker judgments inside a thousand-occurrence
  rename. A reviewer who cannot read a diff rubber-stamps it.
- **Two pieces of deliberate formatting would be destroyed**, one of which the
  configuration explicitly protects.
- **Ruff's linter already covers the part that catches bugs.** `I` sorts
  imports, `UP`, `B`, `SIM`, `C4`, `RET`, `PIE` and `RUF` are semantic. None of
  those needs a formatter; they are what `ruff check` already fails on.

## Consequences

- **Layout is reviewable, which means it is also a thing to get wrong.** There
  is no tool that will fix a badly wrapped call, and no check that will fail
  because of one.
- **This will be proposed again**, by a reviewer or a future contributor, and
  correctly — "why is there no formatter?" is the right question to ask of a
  Python repository in 2026. This file is the answer, and the numbers in it are
  reproducible with `ruff format --check src tests`.
- **The trigger for revisiting it is a second regular contributor**, not a line
  count. At that point benefits 1 and 2 become real and the argparse table is a
  cheap price. ADR 3 defers the compiled backend behind a written trigger for
  the same reason: a decision with a stated condition for reversal is worth
  more than one defended indefinitely.
- **`cli.py`'s `E702` exemption is now load-bearing in a second way.** It marks
  a style the linter permits *and* that no formatter is going to come along and
  normalise. The comment in `pyproject.toml` explains the first; this ADR is
  the second.
- **`tests/tiny_pool.py` stays over 100 columns** and that is the correct
  outcome. Its long lines are card text, which is data.
