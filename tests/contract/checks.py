"""The assertions, as functions -- so they can be run, and shown to bite.

Every contract test is a thin wrapper over one of these, and
`tests/test_contract_harness.py` calls the same functions against a mutated
app to prove each one raises. Keeping the assertion apart from the pytest
plumbing is what makes that proof cheap: no subprocess, no re-collection,
just the function and a broken server.

Each check states the contract in its docstring in the words a Go
implementer needs, because that is who reads these next.
"""

from __future__ import annotations

import json
from typing import Any

from contract.shapes import ANY, Shape, mask, shape

#: The message the middleware answers with before routing, when a request has
#: no session. The frontend does not match on it, but `test_isolation.py` and
#: the deployed smoke test both do, and a second implementation that changed
#: the sentence would change what an operator greps for.
NO_SESSION = "authentication required"
#: Likewise for the admin prefix.
ADMIN_ONLY = "admin only"

#: The headers every response carries (`api/app.py:security_headers`), with
#: the one that depends on TLS left out -- `Strict-Transport-Security` rides
#: only when the cookie is `Secure`, and the harness runs without.
SECURITY_HEADERS = {
    "x-content-type-options": "nosniff",
    "x-frame-options": "DENY",
    "referrer-policy": "same-origin",
    "permissions-policy": "camera=(self), microphone=(), geolocation=()",
}


def check_refused_without_session(response: Any, path: str) -> None:
    """A protected route with no cookie answers **401**, before routing, with
    `{"detail": "authentication required"}`. A GET is enough: the refusal
    runs ahead of method matching, so a POST-only route answers 401 rather
    than 405 -- and 405 would itself confirm the path exists."""
    assert response.status_code == 401, (
        f"{path} answered {response.status_code} to a request with no "
        f"session; every protected route must answer 401")
    check_error_envelope(response)
    assert response.json()["detail"] == NO_SESSION, (
        f"{path} refused with {response.json()!r}; the middleware's own "
        f"sentence is {NO_SESSION!r}")


def check_public_not_refused(response: Any, path: str) -> None:
    """A public route is never refused by the middleware. The assertion is on
    the sentence rather than the status, because `/api/auth/login` answers
    401 for a bad password and that 401 is the endpoint working."""
    body = _json_or_none(response)
    detail = body.get("detail") if isinstance(body, dict) else None
    assert detail != NO_SESSION, (
        f"{path} is public but the middleware refused it")


def check_admin_refused(response: Any, path: str) -> None:
    """Under the admin prefix, a signed-in non-admin answers **403**, before
    routing, with `{"detail": "admin only"}`. 403 and not ADR 5's 404 is
    deliberate (ADR 17): an admin route's existence is published in this
    repository, so the 403 hides nothing and says something useful."""
    assert response.status_code == 403, (
        f"{path} answered {response.status_code} to a signed-in non-admin; "
        f"the prefix must answer 403")
    check_error_envelope(response)
    assert response.json()["detail"] == ADMIN_ONLY


def check_error_envelope(response: Any) -> None:
    """Every non-2xx answer is JSON with a `detail` key: a sentence for the
    user to read as written, or, for a request FastAPI itself could not
    parse, a list of `{type, loc, msg, ...}` objects. `web/src/lib/api.ts`
    reads exactly that and nothing else."""
    assert response.status_code >= 400, "not an error response"
    assert response.headers.get("content-type", "").startswith(
        "application/json"), (
        f"a {response.status_code} must be JSON, got "
        f"{response.headers.get('content-type')!r}")
    body = response.json()
    assert isinstance(body, dict) and "detail" in body, (
        f"a {response.status_code} must carry `detail`, got {body!r}")
    assert isinstance(body["detail"], (str, list)), (
        f"`detail` is a sentence or a validation list, got "
        f"{type(body['detail']).__name__}")


def check_not_found_not_forbidden(response: Any, what: str) -> None:
    """Somebody else's private thing is **404, never 403** (ADR 5, ADR 22).
    A 403 would confirm the thing exists, which is the fact being kept."""
    assert response.status_code == 404, (
        f"{what} answered {response.status_code}; another account's private "
        f"resource must be a 404 -- a 403 would confirm it exists (ADR 5)")
    check_error_envelope(response)


def check_security_headers(response: Any, where: str) -> None:
    """The four hardening headers ride on every response, the middleware's
    own refusals included."""
    for name, value in SECURITY_HEADERS.items():
        assert response.headers.get(name) == value, (
            f"{where}: {name!r} is {response.headers.get(name)!r}, "
            f"want {value!r}")


def check_rate_limited(response: Any) -> None:
    """Past the login budget the answer is **429** with a `Retry-After` in
    whole seconds, which the login form counts down."""
    assert response.status_code == 429, (
        f"expected 429 past the login budget, got {response.status_code}")
    check_error_envelope(response)
    retry = response.headers.get("retry-after")
    assert retry is not None, "a 429 must carry Retry-After"
    assert retry.isdigit() and int(retry) > 0, (
        f"Retry-After must be whole positive seconds, got {retry!r}")


def observed(response: Any, *, volatile: tuple[str, ...] = ()) -> dict[str, Any]:
    """What a golden case records about a response: status, the content type,
    a few headers worth pinning, and the body's shape."""
    kind = response.headers.get("content-type", "")
    record: dict[str, Any] = {"status": response.status_code,
                              "content_type": kind}
    for name in ("cache-control", "location"):
        if name in response.headers:
            record[name] = response.headers[name]
    if "retry-after" in response.headers:
        # Present, and whole seconds; the number is the window's remainder
        # at the instant of the request and pins nothing.
        record["retry-after"] = ANY
    if "set-cookie" in response.headers:
        record["set_cookie"] = _masked_cookie(response.headers["set-cookie"])
    if not response.content:
        # A HEAD, or a 204: there is no body to have a shape. Recorded as
        # such, so a body appearing later is drift too.
        record["bytes"] = "none"
    elif kind.startswith("application/json"):
        record["shape"] = mask(shape(response.json()), volatile)
    else:
        record["bytes"] = "some"
    return record


def check_golden(actual: dict[str, Any], recorded: dict[str, Any],
                 name: str) -> None:
    """A response matches its golden: same status, same content type, same
    pinned headers, and a body of exactly the recorded shape -- no key
    added, none removed, none of a different kind."""
    if actual == recorded:
        return
    lines = [(f"{name}: the wire shape drifted from golden/ "
              f"(record again with --update-golden if the change is meant):")]
    for key in sorted(set(actual) | set(recorded)):
        if actual.get(key) != recorded.get(key):
            lines.append(f"  {key}:")
            lines.append("    recorded: " + _dump(recorded.get(key)))
            lines.append("    actual:   " + _dump(actual.get(key)))
    raise AssertionError("\n".join(lines))


def _dump(value: Shape) -> str:
    return json.dumps(value, sort_keys=True)


def _masked_cookie(header: str) -> str:
    """`sid=<token>; ...` with the token masked: its attributes are the
    contract, its value never is."""
    first, _, rest = header.partition(";")
    name, _, _value = first.partition("=")
    attrs = []
    for attr in rest.split(";"):
        attr = attr.strip().lower()
        if not attr:
            continue
        # A deletion cookie carries `expires=<the moment of the request>`;
        # the attribute is the contract, the date is not.
        if attr.startswith("expires="):
            attr = f"expires={ANY}"
        attrs.append(attr)
    return f"{name}={ANY}; " + "; ".join(sorted(attrs))


def _json_or_none(response: Any) -> Any:
    if not response.headers.get("content-type", "").startswith(
            "application/json"):
        return None
    try:
        return response.json()
    except ValueError:
        return None
