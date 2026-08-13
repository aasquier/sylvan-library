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
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

DOCKERFILE = ROOT / "Dockerfile"

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


def test_every_required_extra_is_declared_in_pyproject():
    """A Dockerfile naming an extra that does not exist fails the build, which
    is late. `pip` treats an unknown extra as a warning in some versions, which
    is worse."""
    declared = (ROOT / "pyproject.toml").read_text(encoding="utf-8")
    block = declared.split("[project.optional-dependencies]", 1)
    assert len(block) == 2, "no optional-dependencies table in pyproject.toml"
    names = set(re.findall(r"^(\w+) = \[", block[1], flags=re.MULTILINE))
    missing = set(REQUIRED_EXTRAS) - names
    assert not missing, f"the Dockerfile would install undeclared extras: {missing}"
