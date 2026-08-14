"""Constructing a Claude client, and saying clearly when we cannot.

Three things this module is careful about.

**The key is never bound to a name.** `config.py` loads `.env` into
`os.environ` and stops there; the Anthropic SDK reads `ANTHROPIC_API_KEY`
itself. A value we never hold is a value we cannot log, serialise into an error
response, or leak into a prompt. `available()` therefore asks whether a
credential is *present*, and never what it is.

**The SDK is optional.** `anthropic` rides with the `claude` extra and is
imported inside functions rather than at module scope, the same way
`python-dotenv` is in `config.py`. A base install has a gate, a solver and a
simulator that all work with no network and no account; ADR 15's "off is a real
position" is a deployment fact, not only a user setting.

**Sonnet 5's request surface is not the one recall suggests.** Writing it from
memory produces 400s, so the specifics are pinned here rather than rediscovered
at each call site:

* Adaptive thinking is **on by default**. Omitting `thinking` runs it; that is
  a change from its predecessor, where omitting meant no thinking. `max_tokens`
  caps thinking *and* answer together, so it needs headroom.
* `thinking: {"type": "enabled", "budget_tokens": N}` is **removed** -- a 400.
  Depth is `output_config={"effort": ...}` instead, defaulting to `high`.
* `temperature`, `top_p` and `top_k` are **rejected**. Steer with the prompt.
* Assistant-turn prefills are **rejected**. Shape output with
  `output_config={"format": ...}` when the time comes.
* Mid-conversation `role: "system"` messages are *not* supported on Sonnet 5,
  though they are on the Opus line -- worth knowing before a mode reaches for
  one to switch stance without invalidating a cached prefix.
"""

from __future__ import annotations

import os
from typing import Any

# Aaron's call, argued in ADR 14 and recorded in ROADMAP: start on Sonnet and
# find out whether it is enough, rather than paying for Opus on the assumption
# that it is not. Most of what the modes will do is conversation over tool
# results the pool already computed, which is not obviously Opus-shaped work.
#
# **Do not quietly raise this.** Moving up is a decision to make once there is
# evidence, and the override below exists so that evidence can be gathered --
# an A/B against a named model for one run -- not so a deployment can drift
# onto a costlier default nobody chose.
MODEL = "claude-sonnet-5"

_MODEL_ENV = "MTGLAB_CLAUDE_MODEL"

# Small on purpose. Every mode that follows will want its own ceiling; this is
# the floor for a health check and a first turn, and it is the number that
# needs revisiting first once adaptive thinking has real work to do.
DEFAULT_MAX_TOKENS = 4096


class ClaudeUnavailable(Exception):
    """No client can be built: the SDK is missing, or no credential is set.

    Distinct from an API error. This one is answerable locally -- install the
    extra, or set the key -- and the caller can say so instead of retrying.
    """


def model() -> str:
    """The model to call. `MODEL` unless deliberately overridden.

    The override is for measuring Sonnet against something else on the same
    prompt, which is the open question ADR 14 left. It is read at call time so
    a comparison run is one environment variable rather than an edit.
    """
    return os.environ.get(_MODEL_ENV) or MODEL


def sdk_installed() -> bool:
    try:
        import anthropic  # noqa: F401
        return True
    except ImportError:
        return False


def credential_present() -> bool:
    """Whether the SDK will find something to authenticate with.

    Deliberately only asks *whether*. Note `config.py` deletes a blank
    `ANTHROPIC_API_KEY` at import, because Anthropic's precedence treats an
    empty string as a credential that is present and then fails it as a 401 --
    so "set but empty" cannot reach this function and read as True.
    """
    return bool(os.environ.get("ANTHROPIC_API_KEY")
                or os.environ.get("ANTHROPIC_AUTH_TOKEN"))


def available() -> bool:
    """Can this process talk to Claude at all? Cheap; no network."""
    return sdk_installed() and credential_present()


def require() -> None:
    """`ClaudeUnavailable` with a fixable reason, or nothing at all.

    Split out of `connect` for the caller that needs to know *whether* a call
    is possible without making one -- `api/themeruns.py` refuses a proposal
    with no key from the request rather than four minutes into a job that was
    never going to work. Asking `connect` would have answered the same
    question and left an HTTP client nobody closes behind on every request.
    """
    if not sdk_installed():
        raise ClaudeUnavailable(
            "the anthropic SDK is not installed -- `pip install -e \".[claude]\"`")
    if not credential_present():
        raise ClaudeUnavailable(
            "no ANTHROPIC_API_KEY in the environment -- put one in .env "
            "(see .env.example), or `fly secrets set` it when deployed")


def connect() -> Any:
    """An `anthropic.Anthropic`, or `ClaudeUnavailable` with a fixable reason.

    Constructed with no arguments on purpose: that is the path where the SDK
    resolves the credential from the environment itself, which is the whole
    point of never holding the key here.
    """
    require()
    import anthropic
    return anthropic.Anthropic()


def explain(exc: Exception) -> str:
    """Turn an SDK exception into something worth reading a month later.

    The 401 case earns its own branch. The key this project runs on was created
    with a 30-day expiry and cannot be extended, so the first thing a rejected
    request means is probably "it expired", not "the integration broke" -- and
    the person reading the message will have forgotten the key had a lifetime.
    """
    if not sdk_installed():                                    # pragma: no cover
        return str(exc)
    import anthropic
    if isinstance(exc, anthropic.AuthenticationError):
        return ("the key was rejected (401). It may have expired -- API keys "
                "carry a fixed lifetime chosen when they are created, and an "
                "expired one cannot be reactivated. Check the key at "
                "platform.claude.com and create a new one if it has lapsed.")
    if isinstance(exc, anthropic.PermissionDeniedError):
        return ("the key was refused this request (403) -- most often a model "
                f"the workspace cannot reach. Asked for {model()!r}.")
    if isinstance(exc, anthropic.RateLimitError):
        return "rate limited (429). Retry after the delay in the response headers."
    if isinstance(exc, anthropic.NotFoundError):
        return f"no such model or endpoint (404). Asked for {model()!r}."
    if isinstance(exc, anthropic.APIConnectionError):
        return "could not reach api.anthropic.com -- check network access."
    if isinstance(exc, anthropic.APIStatusError):
        return f"API error {exc.status_code}: {exc.message}"
    return str(exc)                                            # pragma: no cover


def check(*, prompt: str = "Reply with exactly: pipe open") -> dict[str, Any]:
    """The smallest real call there is. Proves the key, the model and the wire.

    Costs a few dozen tokens, which is the point -- this is what gets run after
    a key rotation, or in six weeks when something 401s and the question is
    whether the integration broke or the key simply lapsed.

    Returns a report rather than printing one, so the CLI and a future health
    route render the same facts.
    """
    report: dict[str, Any] = {"model": model(), "ok": False}
    try:
        con = connect()
    except ClaudeUnavailable as exc:
        report["error"] = str(exc)
        return report

    try:
        resp = con.messages.create(
            model=model(),
            max_tokens=DEFAULT_MAX_TOKENS,
            # Effort, not a thinking budget -- `budget_tokens` is a 400 here.
            # `low` because a health check has nothing to reason about, and the
            # default is `high`.
            output_config={"effort": "low"},
            messages=[{"role": "user", "content": prompt}],
        )
    except Exception as exc:                                        # noqa: BLE001
        # Broad on purpose: every failure mode of this call is something to
        # report to a person, and `explain` is what decides how.
        report["error"] = explain(exc)
        return report

    report.update({
        "ok": resp.stop_reason != "refusal",
        "served_by": resp.model,
        "stop_reason": resp.stop_reason,
        "text": "".join(b.text for b in resp.content if b.type == "text").strip(),
        "input_tokens": resp.usage.input_tokens,
        "output_tokens": resp.usage.output_tokens,
    })
    return report
