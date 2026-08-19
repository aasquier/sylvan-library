"""What the deployed image installs, pinned against what the app imports.

There is no type system spanning a `pyproject.toml` extra, a `Dockerfile` and
an optional import three packages away, so this is the seam -- the same shape
of check as `SIMULATOR_KEYS` in `tests/test_glossary.py`, which exists because
TypeScript cannot look up a Python table.

The failure it guards against already happened once. The image was built with
`.[api]` alone, on the stated grounds that "ADR 15's modes are not built, so an
unused SDK is dependency surface with no caller." That was true the day it was
written. By the time the instance went up, four modes were built, the deploy
had `ANTHROPIC_API_KEY` set as a secret, and `mtglab claude check` on the live
machine answered `unavailable` -- the dossier and both theme-interview modes
were 503s behind buttons the UI happily rendered.

Nothing in the test suite noticed, and nothing could have: every Claude test
stubs the SDK, so they all pass whether or not it is installed. Only the
running container knows, and the only thing that reads the container's
dependency list offline is this.

No network, no Docker. `image` in CI is the only place the file is ever built
(this Mac cannot run a container at all), so a check that needs a build is a
check that gives feedback too late to be worth having.
"""

import re
import sys
import tomllib
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

DOCKERFILE = ROOT / "Dockerfile"
FLY_TOML = ROOT / "fly.toml"
CI_YML = ROOT / ".github" / "workflows" / "ci.yml"
CLAUDE_MD = ROOT / "CLAUDE.md"

# Extras the runtime image must install, and the surface each one is there for.
# An entry is a promise the UI already makes: a control the app renders whose
# handler imports something from that extra.
REQUIRED_EXTRAS = {
    "api": "the web app itself -- fastapi, uvicorn, argon2-cffi",
    "claude": "the dossier and both theme-interview modes (ADR 19, ADR 20)",
}


def install_extras() -> set[str]:
    """The extras named in the image's `pip install`."""
    text = DOCKERFILE.read_text(encoding="utf-8")
    # `RUN pip install --no-cache-dir ".[api,claude]"` -- the bracketed list is
    # the whole of what this file needs to know about the build.
    found = re.search(r"pip install[^\n]*\"\.\[([^\]]+)\]\"", text)
    assert found, "no `pip install \".[...]\"` in the Dockerfile"
    return {part.strip() for part in found.group(1).split(",")}


@pytest.mark.parametrize("extra", sorted(REQUIRED_EXTRAS))
def test_the_image_installs_the_extras_its_surfaces_need(extra):
    assert extra in install_extras(), (
        f"the deployed image would ship without `{extra}` -- "
        f"{REQUIRED_EXTRAS[extra]}. A surface the app renders would answer 503 "
        "on the instance while every test here passed."
    )


def test_the_image_does_not_ship_the_dev_extra():
    """`dev` pulls pytest, ruff and mypy, and includes the other two, so it
    would satisfy the check above while tripling the layer."""
    assert "dev" not in install_extras(), \
        "the runtime image should not carry the test toolchain"


def declared_extras() -> set[str]:
    """Every name in pyproject's `[project.optional-dependencies]` table.

    Parsed rather than matched. The regex form of this read every `name = [`
    from the table header to the end of the file, so it also collected
    `markers`, `select` and eleven other tool settings -- harmless while the
    only question asked was whether two known names were present, and wrong
    the moment anything asked what the whole set is.
    """
    parsed = tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    extras = parsed["project"].get("optional-dependencies")
    assert extras, "no optional-dependencies table in pyproject.toml"
    return set(extras)


def test_every_required_extra_is_declared_in_pyproject():
    """A Dockerfile naming an extra that does not exist fails the build, which
    is late. `pip` treats an unknown extra as a warning in some versions, which
    is worse."""
    missing = set(REQUIRED_EXTRAS) - declared_extras()
    assert not missing, f"the Dockerfile would install undeclared extras: {missing}"


# ------------------------------------- what a documented checkout can test

# The modules CI asserts are importable before it runs the suite, and the
# distribution that provides each. Explicit, so a module added to that guard
# without an entry here fails this test rather than passing through it.
GUARD_DISTRIBUTIONS = {
    "fastapi": "fastapi",
    "httpx": "httpx",
    "hypothesis": "hypothesis",
    "argon2": "argon2-cffi",
}


def ci_import_guard() -> set[str]:
    """The modules ci.yml's "Test dependencies must be present" step imports."""
    found = re.search(r"python -c \"import ([^\"]+)\"",
                      CI_YML.read_text(encoding="utf-8"))
    assert found, "no `python -c \"import ...\"` dependency guard in ci.yml"
    return {name.strip() for name in found.group(1).split(",")}


def extra_requirements(extra: str) -> set[str]:
    """The distribution names in one pyproject extra, lowercased.

    Bracketed extras are stripped -- `uvicorn[standard]>=0.29` is a requirement
    on `uvicorn`, and which extras of it are wanted is not this file's business.
    """
    block = re.search(rf"^{extra} = \[(.*?)\]", (ROOT / "pyproject.toml")
                      .read_text(encoding="utf-8"),
                      flags=re.MULTILINE | re.DOTALL)
    assert block, f"no `{extra}` extra in pyproject.toml"
    return {name.lower()
            for name in re.findall(r"\"([A-Za-z0-9_.-]+)", block.group(1))}


def dev_requirements() -> set[str]:
    return extra_requirements("dev")


@pytest.mark.parametrize("module", sorted(GUARD_DISTRIBUTIONS))
def test_the_dev_extra_alone_is_a_complete_test_environment(module):
    """`.[dev]` is what CLAUDE.md's Setup section tells a checkout to install,
    so it has to run the whole suite on its own.

    It did not. Six modules open with `pytest.importorskip("fastapi")` and
    fastapi lived in the `api` extra only, so the documented install ran 1444
    tests where CI ran 1918 -- the HTTP layer, the 403/404 matrix and
    `test_isolation.py`, the route-classification sweep ADR 5 calls the
    highest-value test in the auth story. It was invisible from CI, which
    installs `.[dev,api]` and asserts these very imports, and invisible from
    the pinned skip count, which is only counted there. Commandment 11 asks for
    the full suite locally before a PR; this is what makes that sentence true.

    Asserted against ci.yml's own guard rather than a list repeated here, so
    the two cannot drift: whatever CI decides it needs, `dev` must provide.
    """
    assert module in ci_import_guard(), (
        f"`{module}` is in GUARD_DISTRIBUTIONS but ci.yml no longer asserts it "
        "is importable. Drop it here too, or put the guard back."
    )
    distribution = GUARD_DISTRIBUTIONS[module]
    assert distribution.lower() in dev_requirements(), (
        f"ci.yml requires `{module}` before it will run the suite, but the "
        f"`dev` extra does not install `{distribution}`. A "
        f"`pip install -e \".[dev]\"` -- what CLAUDE.md documents -- would skip "
        "every test that needs it, and CI cannot notice: it installs "
        "`.[dev,api]`."
    )


# `depth` is absent from this list DELIBERATELY -- see the test after it.
@pytest.mark.parametrize("extra", ["api", "claude", "animist"])
def test_dev_includes_every_other_extra(extra):
    """CLAUDE.md's Setup section says `dev` "includes all of it plus the test
    tooling". That is a contract a checkout relies on, and it was false.

    Everything from `claude` and `animist` was vendored into `dev` deliberately,
    each with its own comment saying which boundary test would otherwise not
    run. From `api`, python-dotenv and argon2-cffi made it across and fastapi
    and uvicorn did not -- so `.[dev]` could neither run the HTTP tests nor
    satisfy `mypy`, which is strict over `cli.py`'s uvicorn import. Two of
    Commandment 11's four gates, unreproducible from the documented install.

    Checked as a subset rather than by listing names, so a package added to any
    extra later inherits the rule instead of quietly escaping it.
    """
    missing = extra_requirements(extra) - dev_requirements()
    assert not missing, (
        f"the `{extra}` extra declares {sorted(missing)}, which `dev` does not. "
        "CLAUDE.md tells a checkout to install `.[dev]` and calls it complete, "
        "so either add these to `dev` or stop claiming it includes everything."
    )


def test_the_depth_extra_is_deliberately_not_in_dev():
    """The one argued exception to the vendoring rule above (ADR 32).

    `depth` is ~800MB of torch wheels backing a function no test may import:
    the suite drives the whole pipeline through the `DepthModel` Protocol
    and fakes, and the real model runs only on the maintainer's machine.
    Pinned in *both* directions so the decision cannot drift either way
    silently -- if torch ever lands in `dev`, somebody should be re-reading
    ADR 32 first, and if `depth` stops declaring torch, the extra has
    stopped meaning anything.
    """
    depth = extra_requirements("depth")
    assert "torch" in depth, "the depth extra no longer declares torch"
    overlap = depth & dev_requirements()
    assert not overlap, (
        f"`dev` now installs {sorted(overlap)} from the `depth` extra -- "
        "that is 800MB per environment for a function no test imports; "
        "re-read ADR 32 before doing this deliberately."
    )


def setup_section() -> str:
    """CLAUDE.md's `## Setup` section, which is the documented bootstrap."""
    text = CLAUDE_MD.read_text(encoding="utf-8")
    start = text.index("\n## Setup\n")
    end = text.index("\n## ", start + 1)
    return text[start:end]


def test_the_setup_section_names_every_extra():
    """The last seam in this file, and the one that had nothing holding it.

    Everything above pins `pyproject.toml` against the `Dockerfile` and against
    `ci.yml`. Nothing pinned it against CLAUDE.md -- which is the document a
    fresh session actually reads, and which is therefore the one place a wrong
    sentence gets believed rather than checked.

    It had drifted, in the same paragraph twice. The 2026-08-16 run found
    "`dev` (which includes all of it)" false; the 2026-08-19 run found the
    *list* false -- four extras named where five were declared, and the missing
    one was `depth`, the single deliberate exception to the rule the sentence
    was stating. An enumeration nobody counts stops being an enumeration.

    Deliberately one-directional: an extra must be named, but the section may
    name other things freely. Prose is not a table, and a check that forbade
    the word `dev` appearing twice would be a check nobody could satisfy.
    """
    section = setup_section()
    unmentioned = {extra for extra in declared_extras()
                   if f"`{extra}`" not in section}
    assert not unmentioned, (
        f"pyproject declares {sorted(unmentioned)} and CLAUDE.md's Setup "
        "section never names it. That section is what a fresh session is "
        "handed; an extra it omits is an extra nobody installs."
    )


def test_every_module_in_cis_guard_is_accounted_for():
    """The other direction. A module added to ci.yml's guard with no entry in
    `GUARD_DISTRIBUTIONS` would simply not be checked above -- the parametrize
    reads this table, not the workflow."""
    unmapped = ci_import_guard() - set(GUARD_DISTRIBUTIONS)
    assert not unmapped, (
        f"ci.yml asserts these imports and this file does not know which "
        f"distribution ships them: {sorted(unmapped)}. Add them to "
        "GUARD_DISTRIBUTIONS so `dev` is checked for them."
    )


# --------------------------------------- what makes the deployment private

# Settings the deployed instance is only safe with, and what each one stops.
# All three default to *off* or to a placeholder in code, which is right for a
# laptop and is exactly why they have to be asserted here: the safe value is
# the one nobody has to type, so a line going missing is silent.
REQUIRED_ENV = {
    "MTGLAB_REQUIRE_AUTH":
        "the middleware that refuses every path outside PUBLIC_PATHS. "
        "`config.require_auth()` is a flag that defaults to False, so an "
        "instance missing this line serves the whole app to the internet",
    "MTGLAB_CLIENT_IP_HEADER":
        "login rate limiting by real client address. Without it "
        "`auth.client_address` sees Fly's proxy, so every attempt from "
        "everybody shares one bucket and one mistyped password 429s the "
        "instance",
}


def fly_env() -> dict[str, str]:
    """`fly.toml`'s `[env]` table, as a plain dict.

    Parsed with a regex rather than a TOML library: `tomllib` would do it, but
    this reads one flat table of scalars and the point is to be readable by
    somebody checking whether the check is right.
    """
    text = FLY_TOML.read_text(encoding="utf-8")
    block = text.split("\n[env]", 1)
    assert len(block) == 2, "no [env] table in fly.toml"
    # Stop at the next table header; `[[mounts]]` is the one that follows.
    body = re.split(r"\n\[", block[1], maxsplit=1)[0]
    return {m[1]: m[2] for m in
            re.finditer(r'^\s*(\w+)\s*=\s*"([^"]*)"\s*$', body,
                        flags=re.MULTILINE)}


@pytest.mark.parametrize("name", sorted(REQUIRED_ENV))
def test_the_deployment_sets_what_makes_it_private(name):
    """`fly.toml` is the only thing standing between the app and the public.

    There is no type system spanning a TOML table and a `getenv` default, which
    is this module's whole reason for existing -- and this is the seam where
    that gap is worst, because **every one of these defaults to the open
    setting.** `config.require_auth()` returns False when unset, so an instance
    deployed without `MTGLAB_REQUIRE_AUTH` comes up serving every route to
    anybody, with a passing health check and nothing in the log to say so. It
    is the same failure shape `fly.toml` already warns about for the
    `you@example.com` placeholder: the app looks entirely well and is silently
    wrong.

    Off-by-default is the right default -- CLAUDE.md is explicit that a login
    in front of one person on a laptop is a regression -- so the fix is not to
    invert it. The fix is to assert the deployment file, which is what this
    does, at pull-request time rather than after a deploy.

    Worth having because that file is edited for unrelated reasons: the
    machine-awake block sits a hundred lines below `[env]` in the same file,
    and a stray line-drop while editing it would not otherwise fail anything.
    """
    env = fly_env()
    assert name in env, (
        f"fly.toml's [env] no longer sets {name} -- {REQUIRED_ENV[name]}")
    assert env[name].strip(), f"{name} is set to an empty value in fly.toml"


# ------------------------------------------- what the deploy job may deploy

def deploy_condition() -> str:
    """The `if:` expression on `ci.yml`'s `deploy` job, whitespace collapsed.

    Read as text for the same reason `fly_env` is: the point is a check a
    person can audit against the file, and a YAML parser would add a
    dependency to assert one string.
    """
    text = CI_YML.read_text(encoding="utf-8")
    job = text.split("\n  deploy:\n", 1)
    assert len(job) == 2, "no `deploy` job in ci.yml"
    # `if: >-` folds the following indented block into one expression; it ends
    # at the next key at the same indentation.
    found = re.search(r"^    if: >-\n((?:      .*\n)+)", job[1], re.MULTILINE)
    assert found, "the deploy job has no `if:` condition at all"
    return " ".join(found.group(1).split())


def deploys(event: str, ref: str) -> bool:
    """Would the deploy job run, for this event and ref?

    Evaluates the real condition rather than matching text against it. That is
    not over-engineering, it is the whole point: the first version of this test
    asserted `"github.ref == 'refs/heads/main'" in condition`, which **passes
    against the bug it was written to catch** -- the broken condition contains
    that substring too, nested inside the `push` arm where it does not apply to
    a dispatch. A truth table can tell those apart and a substring cannot.

    The grammar is tiny and fully known: `==`, `&&`, `||`, parentheses, single-
    quoted literals, and the two context fields. Translating it to Python and
    evaluating with no builtins and no names beyond those two is a closer
    reading of GitHub's semantics than a hand-rolled parser would be.
    """
    expr = deploy_condition()
    python = expr.replace("&&", "and").replace("||", "or")
    scope = {"__builtins__": {}}
    context = {"event_name": event, "ref": ref}
    # `github.event_name` is an attribute path, so hand it an object.
    python = python.replace("github.", "gh.")
    scope["gh"] = type("Ctx", (), context)()
    return bool(eval(python, scope))


# What may reach the instance, and what may not. The third row is the bug.
DEPLOY_CASES = [
    ("push", "refs/heads/main", True,
     "the ordinary case -- a merge to main is a deploy (ADR 23)"),
    ("workflow_dispatch", "refs/heads/main", True,
     "ADR 23's manual button, pointed at main"),
    ("workflow_dispatch", "refs/heads/some-feature", False,
     "a dispatch from a branch must NOT deploy that branch"),
    ("push", "refs/heads/some-feature", False,
     "a push to a branch is not a release"),
    ("pull_request", "refs/pull/17/merge", False,
     "a fork's PR must never reach the instance"),
]


@pytest.mark.parametrize(("event", "ref", "expected", "why"), DEPLOY_CASES)
def test_the_deploy_job_deploys_main_and_nothing_else(event, ref, expected, why):
    """A green feature branch is still the wrong thing to put on the instance.

    This was wrong from the moment ADR 23 landed until later the same day. The
    condition read `workflow_dispatch || (push && ref == main)`, so the ref
    check applied to pushes only and a manual dispatch deployed **whatever
    branch it was launched from** -- appearing in the Actions list as an
    ordinary deploy, because it is the same workflow with the same job names.

    `needs` is no help: it proves the four checks passed, not that the ref is
    the one anybody agreed to ship. And the mistake does not undo itself.
    `auth/db.py`'s ladder is forward-only, so a branch carrying a schema change
    migrates the volume on boot and deploying `main` afterwards leaves the new
    schema in place under the old code.

    Asserting the job's condition rather than the workflow's trigger is the
    point. Dispatching the suite on a branch stays available; only the deploy
    job refuses.
    """
    assert deploys(event, ref) is expected, (
        f"ci.yml's deploy job would {'not ' if expected else ''}run for "
        f"{event} on {ref}, and it should{'' if expected else ' not'}: {why}. "
        f"Condition is: {deploy_condition()}")


def test_no_deck_is_tracked_by_git():
    """ADR 30: decks are live app data, not repository content.

    `.gitignore` covers the whole `decks/` tree, but an ignore rule does not
    evict a file that is already tracked, and a `git add -f` would sail past
    it silently. This is the check that the removal stays removed. Runs
    against `git ls-files` rather than the ignore file because the ignore
    file is the intent and the index is the fact -- the config-tests lesson,
    applied to the repository itself.

    When the suite runs outside a git checkout (an unpacked sdist), there is
    no index to consult and nothing to enforce; CI always runs from a
    checkout, and CI is where this gate matters.
    """
    import subprocess

    proc = subprocess.run(["git", "-C", str(ROOT), "ls-files", "--", "decks/"],
                          capture_output=True, text=True)
    if proc.returncode != 0:
        return  # not a checkout; CI is the enforcement point
    tracked = [line for line in proc.stdout.splitlines() if line.strip()]
    assert tracked == [], (
        "decks are live app data (ADR 30) and must not be tracked: "
        f"{tracked[:5]}")
