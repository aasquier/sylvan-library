"""The providers, and the licence gate. The gate blocks; there is no override.

Two providers answer the question the pipeline is not allowed to skip: *is
this file actually free?* Wikimedia Commons answers through `extmetadata` on
the imageinfo API, Openverse through its record API. A licence is confirmed
per file, at fetch time, through the API rather than read off a landing page —
the tarot deck's rule, kept because the famous trap (the 1971 recolouring of a
1909 deck) lives precisely in the gap between what a page implies and what
the record says.

`ALLOWED_LICENCES` is deliberately tiny. This project commits assets, and a
committed asset is redistribution, so only dedications that permit exactly
that with nothing owed qualify. CC-BY is a fine licence and it is still
refused here: an attribution obligation carried by a decoration is an
obligation the next maintainer inherits without knowing.

The `Transport` seam is `auth/mail.py`'s, for the same stated reason: **the
seam is what keeps the network out of the tests.** Everything above it —
request building, response parsing, the gate's verdict — is covered; the six
lines of urllib below it are the accepted residue.
"""

from __future__ import annotations

import datetime as _dt
import json
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Callable
from dataclasses import dataclass

from mtglab.animist.recipe import Source

COMMONS_API = "https://commons.wikimedia.org/w/api.php"
OPENVERSE_API = "https://api.openverse.org/v1/images"

# Scryfall asks for one in writing, Cloudflare blocks urllib's default outright
# (see the 403 story on `auth/mail.py`'s USER_AGENT), and Wikimedia's API
# etiquette asks that a tool identify itself. Assembled above the seam so a
# test can pin it — put it in the transport and the one header that decides
# whether requests work at all becomes the one thing tests cannot see.
USER_AGENT = "mtg-lab/0.1 (personal deckbuilding tool; asset provenance)"

TIMEOUT_SECONDS = 30

#: What may pass the gate, per provider vocabulary. One comment per entry
#: saying why it qualifies; anything not listed is refused, including
#: perfectly good licences with obligations attached.
ALLOWED_COMMONS = frozenset({
    # Commons' LicenseShortName for works with expired or no copyright.
    "Public domain",
    # The explicit dedication: all rights surrendered, worldwide.
    "CC0",
})
ALLOWED_OPENVERSE = frozenset({
    # Openverse's key for the CC0 dedication.
    "cc0",
    # The Public Domain Mark: identified as free of known copyright.
    "pdm",
})


class SourceError(RuntimeError):
    """The provider could not be reached or answered with the wrong shape."""


class LicenceRefused(RuntimeError):
    """The gate's verdict on a source that is not confirmably free.

    Carries what was found and what would have passed, so the refusal reads
    as a fact about the file rather than a fault in the tool.
    """

    def __init__(self, source_id: str, found: str, allowed: frozenset[str],
                 detail: str = "") -> None:
        self.source_id = source_id
        self.found = found
        self.allowed = allowed
        suffix = f" ({detail})" if detail else ""
        super().__init__(
            f"source `{source_id}`: licence {found!r} is not on the "
            f"allowlist {sorted(allowed)}{suffix} -- the gate blocks, and "
            "there is deliberately no flag past it")


@dataclass(frozen=True)
class LicenceConfirmation:
    """A gate pass: what the API said, and when and where it said it.

    `provenance.py` renders this as the "confirmed at fetch time" line, which
    makes that sentence machine-produced rather than remembered.
    """

    source_id: str
    licence: str
    checked_on: str     # ISO date
    api_url: str


@dataclass(frozen=True)
class RemoteFile:
    """One downloadable original: its upstream name, URL, and licence."""

    name: str
    url: str
    licence: str


@dataclass(frozen=True)
class Confirmed:
    """Everything the gate hands onward for one source.

    `skipped` is the filename-prefix guard's report — the files a category
    listed that the recipe's `require_filename_prefix` excluded. Reported,
    never silent: the tarot fetch skipping exactly two title images is part
    of that recipe's provenance story.
    """

    confirmation: LicenceConfirmation
    files: tuple[RemoteFile, ...]
    skipped: tuple[str, ...] = ()


# `(url, headers) -> (status, body)`. GET-only: nothing in this package ever
# writes to a provider.
Transport = Callable[[str, dict[str, str]], tuple[int, bytes]]


def _urllib_get(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(request, timeout=TIMEOUT_SECONDS) as response:
        return int(response.status), response.read()


def _get_json(url: str, transport: Transport, source_id: str) -> dict[str, object]:
    headers = {"User-Agent": USER_AGENT, "Accept": "application/json"}
    try:
        status, body = transport(url, headers)
    except urllib.error.HTTPError as exc:
        raise SourceError(f"source `{source_id}`: HTTP {exc.code} from "
                          f"{url}") from exc
    except OSError as exc:
        raise SourceError(f"source `{source_id}`: could not reach the "
                          f"provider: {exc}") from exc
    if status != 200:
        raise SourceError(f"source `{source_id}`: HTTP {status} from {url}")
    try:
        parsed = json.loads(body.decode("utf-8"))
    except (ValueError, UnicodeDecodeError) as exc:
        raise SourceError(f"source `{source_id}`: the response from {url} "
                          "was not JSON") from exc
    if not isinstance(parsed, dict):
        raise SourceError(f"source `{source_id}`: the response from {url} "
                          "was not a JSON object")
    return parsed


def _today() -> str:
    return _dt.date.today().isoformat()


def _confirm_openverse(source: Source, transport: Transport) -> Confirmed:
    url = f"{OPENVERSE_API}/{urllib.parse.quote(source.identifier)}/"
    record = _get_json(url, transport, source.id)
    licence = str(record.get("license", "") or "")
    if licence not in ALLOWED_OPENVERSE:
        raise LicenceRefused(source.id, licence, ALLOWED_OPENVERSE)
    file_url = str(record.get("url", "") or "")
    if not file_url:
        raise SourceError(f"source `{source.id}`: the Openverse record "
                          "carries no direct file URL")
    name = urllib.parse.unquote(file_url.rsplit("/", 1)[-1]) or source.identifier
    confirmation = LicenceConfirmation(source.id, licence, _today(), url)
    return Confirmed(confirmation, (RemoteFile(name, file_url, licence),))


def _commons_query(params: dict[str, str]) -> str:
    base = {"action": "query", "format": "json", "formatversion": "2",
            "prop": "imageinfo", "iiprop": "url|extmetadata"}
    return f"{COMMONS_API}?{urllib.parse.urlencode(base | params)}"


def _commons_pages(payload: dict[str, object], source_id: str,
                   ) -> list[dict[str, object]]:
    query = payload.get("query")
    if not isinstance(query, dict):
        raise SourceError(f"source `{source_id}`: the Commons response has "
                          "no `query` block")
    pages = query.get("pages")
    if not isinstance(pages, list):
        raise SourceError(f"source `{source_id}`: the Commons response has "
                          "no pages")
    return [p for p in pages if isinstance(p, dict)]


def _commons_file(page: dict[str, object], source_id: str,
                  ) -> tuple[str, str, str]:
    """One Commons page -> (file name, download URL, LicenseShortName)."""
    title = str(page.get("title", "") or "")
    name = title.removeprefix("File:")
    infos = page.get("imageinfo")
    if not isinstance(infos, list) or not infos or not isinstance(infos[0], dict):
        raise SourceError(f"source `{source_id}`: no imageinfo for "
                          f"{title or 'a page'} -- is it a file page?")
    info = infos[0]
    url = str(info.get("url", "") or "")
    meta = info.get("extmetadata")
    licence = ""
    if isinstance(meta, dict):
        short = meta.get("LicenseShortName")
        if isinstance(short, dict):
            licence = str(short.get("value", "") or "")
    return name, url, licence


def _confirm_wikimedia(source: Source, transport: Transport) -> Confirmed:
    title = source.title if source.title.startswith("File:") \
        else f"File:{source.title}"
    url = _commons_query({"titles": title})
    payload = _get_json(url, transport, source.id)
    pages = _commons_pages(payload, source.id)
    if not pages:
        raise SourceError(f"source `{source.id}`: Commons knows no {title!r}")
    name, file_url, licence = _commons_file(pages[0], source.id)
    if licence not in ALLOWED_COMMONS:
        raise LicenceRefused(source.id, licence, ALLOWED_COMMONS,
                             detail=f"File:{name}")
    confirmation = LicenceConfirmation(source.id, licence, _today(), url)
    return Confirmed(confirmation, (RemoteFile(name, file_url, licence),))


def _confirm_category(source: Source, transport: Transport) -> Confirmed:
    """Every file in a Commons category, licence-checked one by one.

    The prefix guard runs *before* the gate: a file the recipe excludes is
    skipped and reported, not licence-checked — the two title images in the
    tarot category are not free-use failures, they are simply not cards. A
    file that passes the prefix and fails the gate refuses the whole source,
    because a set with one unfree member is not a set this pipeline ships.
    """
    files: list[RemoteFile] = []
    skipped: list[str] = []
    continuation: dict[str, str] = {}
    first_url = ""
    while True:
        url = _commons_query({
            "generator": "categorymembers",
            "gcmtitle": f"Category:{source.category}",
            "gcmtype": "file", "gcmlimit": "500", **continuation})
        first_url = first_url or url
        payload = _get_json(url, transport, source.id)
        for page in _commons_pages(payload, source.id):
            name, file_url, licence = _commons_file(page, source.id)
            prefix = source.require_filename_prefix
            if prefix and not name.startswith(prefix):
                skipped.append(name)
                continue
            if licence not in ALLOWED_COMMONS:
                raise LicenceRefused(source.id, licence, ALLOWED_COMMONS,
                                     detail=f"File:{name}")
            files.append(RemoteFile(name, file_url, licence))
        more = payload.get("continue")
        if not isinstance(more, dict):
            break
        continuation = {str(k): str(v) for k, v in more.items()}
    if not files:
        raise SourceError(
            f"source `{source.id}`: category {source.category!r} yielded no "
            "files" + (f" with prefix {source.require_filename_prefix!r}"
                       if source.require_filename_prefix else ""))
    licences = {f.licence for f in files}
    confirmation = LicenceConfirmation(
        source.id, ", ".join(sorted(licences)), _today(), first_url)
    return Confirmed(confirmation, tuple(files), tuple(skipped))


def confirm(source: Source, *, transport: Transport | None = None) -> Confirmed:
    """Run the gate for one source. Passes return the files and the proof;
    anything else raises `LicenceRefused` or `SourceError` before a byte of
    image data moves."""
    get: Transport = transport if transport is not None else _urllib_get
    if source.provider == "openverse":
        return _confirm_openverse(source, get)
    if source.provider == "wikimedia":
        return _confirm_wikimedia(source, get)
    if source.provider == "wikimedia-category":
        return _confirm_category(source, get)
    raise SourceError(f"source `{source.id}`: unknown provider "
                      f"{source.provider!r}")
