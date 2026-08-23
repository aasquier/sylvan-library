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
FORGE_DOCKERFILE = ROOT / "Dockerfile.forge"
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


# The documents that walk somebody through installing this, and the section in
# each that does the walking. CLAUDE.md is what a fresh session reads; README
# is what a stranger reads first; CONTRIBUTING is where README sends them.
SETUP_SECTIONS = {
    "CLAUDE.md": (CLAUDE_MD, "\n## Setup\n"),
    "README.md": (ROOT / "README.md", "\n## Running it\n"),
    "CONTRIBUTING.md": (ROOT / "CONTRIBUTING.md", "\n## Getting set up\n"),
}


def setup_section(doc: str) -> str:
    """The named document's setup section, which is its documented bootstrap."""
    path, heading = SETUP_SECTIONS[doc]
    text = path.read_text(encoding="utf-8")
    start = text.index(heading)
    end = text.index("\n## ", start + 1)
    return text[start:end]


@pytest.mark.parametrize("doc", sorted(SETUP_SECTIONS))
def test_the_setup_section_names_every_extra(doc):
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

    Widened 2026-08-21 over README.md and CONTRIBUTING.md, which the relic
    sweep found carrying the same drift CLAUDE.md was cured of -- three extras
    named where five were declared, in the two files this test did not read.
    The fourth instance of the completeness-claim failure, and the reason the
    fix is a wider test rather than three corrected sentences.

    Deliberately one-directional: an extra must be named, but the section may
    name other things freely. Prose is not a table, and a check that forbade
    the word `dev` appearing twice would be a check nobody could satisfy.
    """
    section = setup_section(doc)
    unmentioned = {extra for extra in declared_extras()
                   if f"`{extra}`" not in section}
    assert not unmentioned, (
        f"pyproject declares {sorted(unmentioned)} and {doc}'s setup "
        "section never names it. That section is somebody's documented "
        "bootstrap; an extra it omits is an extra nobody installs."
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


# ------------------------------------------ what the deploy job waits for

def ci_jobs() -> list[str]:
    """Every job id in `ci.yml`, in file order.

    Text again, and the indentation is the whole grammar: a job id is the only
    thing in this file that is a bare key two spaces deep *inside the `jobs:`
    block*. Everything a job declares sits at four, and the two-space keys
    above `jobs:` belong to `on:`, `permissions:` and `concurrency:`, which is
    why the split comes first rather than scanning the file whole.
    """
    text = CI_YML.read_text(encoding="utf-8")
    block = text.split("\njobs:\n", 1)
    assert len(block) == 2, "no `jobs:` block in ci.yml"
    return re.findall(r"^  ([a-z][a-z0-9_-]*):\s*$", block[1], re.MULTILINE)


def deploy_needs() -> set[str]:
    """The `needs:` list on `ci.yml`'s deploy job."""
    text = CI_YML.read_text(encoding="utf-8")
    job = text.split("\n  deploy:\n", 1)
    assert len(job) == 2, "no `deploy` job in ci.yml"
    found = re.search(r"^    needs: \[([^\]]*)\]\s*$", job[1], re.MULTILINE)
    assert found, "the deploy job has no `needs:` list at all"
    return {part.strip() for part in found.group(1).split(",") if part.strip()}


def test_the_deploy_job_waits_for_every_other_job_in_the_file():
    """A green merge deploys itself (ADR 23), so `needs` is what "green" means.

    The deploy job's own comment calls this "the whole safety argument": it
    cannot start until the checks above it pass, so it is *structurally*
    unable to ship something that failed one. Branch protection is not a
    second copy of that -- protection governs merging, and ADR 23's manual
    button is a `workflow_dispatch` on main that no protection rule touches.
    For that path `needs` is the only guard there is.

    Nothing checked it. Adding a job to this file is already known to be two
    steps -- the `image` job's own comment says so about the required-checks
    setting -- and it is really three, because the third is this list. Miss it
    and the new job runs, goes red, and the instance is replaced anyway; the
    Actions page shows one red job beside a successful deploy, which reads
    like a flaky check rather than an unguarded release.

    Expected is **derived from the file** rather than restated here, which is
    the point and is the lesson `deploys()` above was written for: a list
    spelled out in this module would be a second copy to keep in step, and a
    substring check would pass against the bug. Adding a job to `ci.yml`
    fails this until somebody decides, in the same change, whether a deploy
    should wait for it.
    """
    jobs = ci_jobs()
    assert "deploy" in jobs, "no `deploy` job in ci.yml"
    expected = {job for job in jobs if job != "deploy"}
    needs = deploy_needs()
    assert needs == expected, (
        f"ci.yml's deploy job waits for {sorted(needs)}, but the file "
        f"declares {sorted(expected)} ahead of it. "
        f"Unwatched: {sorted(expected - needs)}; "
        f"named but absent: {sorted(needs - expected)}. "
        "A green main deploys itself, so every check has to be in `needs`.")


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


# ------------------------------------------ what the type checker still checks

def test_no_first_party_module_is_exempt_from_strict_mypy():
    """The exception list emptied on 2026-08-19 and has to stay empty.

    `[tool.mypy] strict = true` covers the package; an entry under
    `[[tool.mypy.overrides]]` takes a named module back out of it. Three of
    those are legitimate and all three name somebody else's package -- a
    dependency that ships no stubs. A first-party one is different in kind:
    it exempts code this repo writes, and the module that sat there absorbed
    every new untyped function without a word, going 79 -> 109 -> 126 while
    the comment beside it said the list was shrinking.

    Derived rather than restated. What counts as first-party is read off
    `src/`, so a module added under a new package name is caught the same way
    `mtglab.cli` would be -- a hard-coded list of forbidden names would only
    ever catch the one that already happened.
    """
    parsed = tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    ours = {p.name for p in (ROOT / "src").iterdir() if p.is_dir()}
    exempt = {module
              for override in parsed["tool"]["mypy"].get("overrides", [])
              for module in override["module"]
              if module.split(".")[0] in ours}
    assert not exempt, (
        f"{sorted(exempt)} is exempt from strict mypy. The list was emptied "
        "deliberately; putting our own code back on it is a decision to argue "
        "in the comment above the override, not one to take because a new "
        "function was quicker to leave unannotated."
    )


# ------------------------------------------------------ what stands at the door

GO_MOD = ROOT / "go" / "go.mod"
GO_PLAN = ROOT / "docs" / "go-migration" / "PLAN.md"


def dockerfile_cmd() -> list[str]:
    """The runtime stage's CMD, as the JSON array it is written as.

    Read as text, like `fly_env` and `deploy_condition`: the point is a check
    somebody can audit against the file without a Dockerfile parser.
    """
    import json
    text = DOCKERFILE.read_text(encoding="utf-8")
    found = re.search(r"^CMD (\[.*?\])\s*$", text, re.MULTILINE | re.DOTALL)
    assert found, "no CMD in the Dockerfile"
    # A JSON array may be continued across lines with backslashes.
    return json.loads(found.group(1).replace("\\\n", ""))


def test_the_container_runs_the_front_door_with_the_library_behind_it():
    """ADR 38, decision 2: the Go binary takes the port and the Python server
    runs behind it on loopback, for the whole of the migration.

    Three things about the CMD, each of which a one-token edit could undo
    while the image still built and still answered `/api/health`: it is the
    door that is PID 1 (the entrypoint `exec`s into argv[0]); it proxies to a
    loopback upstream and supervises a command after `--`; and the supervised
    command is the Python `mtglab ui` bound to that same loopback port --
    a mismatch here is a door answering 502 to every API request with a
    healthy-looking container behind it.
    """
    cmd = dockerfile_cmd()
    assert cmd[0] == "/opt/door/mtglab" and cmd[1] == "ui", cmd
    assert "--" in cmd, "the door supervises nothing; the Python server would never start"
    door, child = cmd[: cmd.index("--")], cmd[cmd.index("--") + 1:]
    assert "--host" in door and door[door.index("--host") + 1] == "0.0.0.0"
    assert "--port" in door and door[door.index("--port") + 1] == "8080", \
        "fly.toml's internal_port is 8080"
    upstream = door[door.index("--upstream") + 1]
    assert child[:2] == ["mtglab", "ui"], child
    assert "--no-open" in child
    assert child[child.index("--host") + 1] == "127.0.0.1", \
        "the Python server must bind loopback only; the door is the public face"
    child_port = child[child.index("--port") + 1]
    assert upstream == f"http://127.0.0.1:{child_port}", (
        f"the door proxies to {upstream} but the child listens on {child_port}")


def test_the_forge_worker_image_runs_the_go_shim_and_carries_no_python():
    """Phase 7: the worker's door is `mtglab forge-shim`, not a Python module.

    Three claims, each of which a one-line edit could undo while the image
    still built. **The CMD is the Go subcommand** -- the shim is a second hat
    on the binary the app image already serves, so the two images ship one
    artefact and a version skew between them is impossible rather than merely
    unlikely. **The runtime stage is not a Python image**, because the whole
    point of the flip was to stop shipping an interpreter, a venv, DuckDB and
    numpy to run a three-endpoint HTTP server. And **the binary is the one
    the app image builds**, from the same module with the same Go, so a
    deploy that updates the app and the worker minutes apart cannot put two
    different codebases on the two ends of `wire.go`.

    The `fetch` stage may still be a Python image -- it runs
    `scripts/fetch_forge.py` at build time and ships nothing.
    """
    text = FORGE_DOCKERFILE.read_text(encoding="utf-8")

    import json
    found = re.search(r"^CMD (\[.*?\])\s*$", text, re.MULTILINE | re.DOTALL)
    assert found, "no CMD in Dockerfile.forge"
    cmd = json.loads(found.group(1).replace("\\\n", ""))
    assert cmd == ["mtglab", "forge-shim"], (
        f"the worker runs {cmd}; Phase 7 made it the Go binary's subcommand")

    runtime = re.search(r"^FROM (\S+) AS runtime$", text, re.MULTILINE)
    assert runtime, "Dockerfile.forge has no runtime stage"
    assert "python" not in runtime.group(1), (
        f"the worker's runtime stage is {runtime.group(1)}; it needs a JRE "
        "and a static binary, not an interpreter")

    builder = re.search(r"^FROM golang:(\d+\.\d+)\S* AS builder$", text, re.MULTILINE)
    assert builder, "Dockerfile.forge has no `golang:<minor>` builder stage"
    assert builder.group(1) == "1.26", (
        f"the worker builds with golang:{builder.group(1)} while go.mod pins 1.26")
    assert re.search(r"CGO_ENABLED=1 go build .*\./cmd/mtglab", text), (
        "the worker must build the same `./cmd/mtglab` the app image builds, "
        "with CGO on -- the shim shares that binary with the front door, "
        "which links DuckDB")
    assert "libstdc++6" in text, (
        "a CGO build links libstdc++; the runtime stage must name it rather "
        "than inherit it from the JRE by luck")

    # And nothing installs the Python package into the shipped stage.
    assert "pip install" not in text.split("AS runtime", 1)[1], (
        "the worker's runtime stage installs Python packages")


def test_the_go_module_pins_the_last_go_that_runs_on_this_mac():
    """ADR 38, decision 5: `go 1.26`, because Go 1.27 requires macOS 13 and
    the maintainer's machine is macOS 12 -- a runway, named so nobody is
    surprised, and a pin a `go mod tidy` on a newer toolchain could quietly
    bump. The Dockerfile's door stage is held to the same minor, so the
    image never builds with a Go the module does not claim to support."""
    text = GO_MOD.read_text(encoding="utf-8")
    found = re.search(r"^go (\S+)$", text, re.MULTILINE)
    assert found, "go.mod has no `go` directive"
    assert found.group(1) == "1.26", (
        f"go.mod says go {found.group(1)}; 1.26 is the last release for this "
        "Mac (ADR 38). Re-read the decision before moving it.")
    assert not re.search(r"^toolchain ", text, re.MULTILINE), (
        "go.mod grew a `toolchain` line, which would make the build reach for "
        "a Go newer than the one this Mac can run")
    docker = DOCKERFILE.read_text(encoding="utf-8")
    found = re.search(r"^FROM golang:(\d+\.\d+)\S* AS door$", docker, re.MULTILINE)
    assert found, "the Dockerfile has no `golang:<minor>` door stage"
    assert found.group(1) == "1.26", (
        f"the door stage builds with golang:{found.group(1)} while go.mod "
        "pins 1.26")


def test_claude_md_names_every_fingerprinted_go_package():
    """ADR 18's fingerprint is a list, and CLAUDE.md describes it in prose.

    `sim/cache.Fingerprint` hashes one package's embedded source per entry in
    `engineSources`, and the go/ section names them. That prose said **four**
    packages while the code held five, from the moment it was written: it was
    drafted while `pyfloat` still sat inside `internal/sim`, #249 moved it to
    a package of its own, the code followed and the sentence did not.

    That makes it the fourth completeness claim in a file which already
    carries three warnings that such a claim is "a claim to re-check against
    the code, not a fact to inherit" -- so this is the same cure the `dev`
    extra got: the sentence is now checked rather than remembered.

    It matters more here than the count suggests. `engineSources` is the
    **only** guard against a file walking out of the key, because each
    package's embed is held complete against its own directory, and a file
    that leaves the package satisfies both sides of that check -- gone from
    the directory and gone from the list -- while quietly ceasing to be
    fingerprinted. Prose a person actually reads is the second guard, and a
    second guard that has rotted is not one.
    """
    source = (ROOT / "go" / "internal" / "sim" / "cache" / "cache.go").read_text(
        encoding="utf-8")
    block = re.search(r"var engineSources = \[\]engineSource\{(.*?)\n\}",
                      source, re.DOTALL)
    assert block, "cache.go has no `engineSources` list to read"
    packages = re.findall(r'\{"internal/(\S+?)",', block.group(1))
    assert packages, "the `engineSources` list parsed to nothing"

    want = {pkg.rsplit("/", 1)[-1] for pkg in packages}
    spelled = {3: "three", 4: "four", 5: "five", 6: "six", 7: "seven"}

    # **Both** documents, because the sentence rotted in both at once and
    # curing one would have left the other saying four. They are phrased the
    # same way on purpose, so one reading serves for both.
    for label, path in (("CLAUDE.md", CLAUDE_MD),
                        ("docs/go-migration/PLAN.md", GO_PLAN)):
        text = path.read_text(encoding="utf-8")
        start = text.index("fingerprint is a hash of **embedded Go source**")
        passage = text[start:start + 1200]

        # Read the ENUMERATION, not the paragraph. The prose names a package
        # by its last path segment (`internal/sim/tier1` is "tier1"), and the
        # surrounding sentences discuss those names too -- so a window-wide
        # substring search passes even when the list itself has dropped one,
        # which is exactly the mutation that proved this needed writing twice.
        listed = re.search(r"`source\.go`:(.+?)\.\s", passage, re.DOTALL)
        assert listed, f"{label} no longer enumerates the fingerprinted packages"
        named = set(re.findall(r"\w+", listed.group(1).replace("\n", " "))) - {"and"}
        assert named == want, (
            f"`engineSources` fingerprints {sorted(want)} and {label}'s list "
            f"names {sorted(named)}. Adding a package under the engine is a "
            "decision to take deliberately -- say so there as well as in the "
            "code.")

        # And the count it spells out, which is the half a reader trusts
        # without counting the names.
        written = re.search(r"\b(three|four|five|six|seven) packages\b", passage)
        assert written, f"{label} no longer spells out how many packages there are"
        assert written.group(1) == spelled[len(packages)], (
            f"{label} says {written.group(1)} packages; `engineSources` holds "
            f"{len(packages)}")


def test_the_route_table_is_read_by_both_doors():
    """`tests/contract/routes.json` is the one classification (PLAN section
    8); the Go module's reader must point at that file and nowhere else, or
    the Go middleware's sweep silently runs against a copy."""
    reader = (ROOT / "go" / "internal" / "routes" / "routes.go").read_text(encoding="utf-8")
    assert 'const File = "tests/contract/routes.json"' in reader

