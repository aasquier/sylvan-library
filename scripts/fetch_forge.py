"""Fetch the Forge distribution for the worker image, pinned by digest.

Used by `Dockerfile.forge` and nothing else. A separate script rather than a
`RUN` one-liner because this downloads and unpacks **executable code** —
`ocr.py` set the house rule for that: every such file is pinned by SHA-256,
and a pin needs enough code around it to be readable. The digest here is the
one GitHub publishes for the release asset, checked before a single byte is
unpacked.

    python scripts/fetch_forge.py <url> <sha256> <dest-dir>

Stdlib only, because the fetch stage of the image has nothing else installed.
"""

from __future__ import annotations

import hashlib
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path


def main(url: str, digest: str, dest: str) -> None:
    print(f"fetching {url}", flush=True)
    with tempfile.NamedTemporaryFile(suffix=".tar.bz2") as scratch:
        with urllib.request.urlopen(url, timeout=120) as response:
            hasher = hashlib.sha256()
            while chunk := response.read(1 << 20):
                hasher.update(chunk)
                scratch.write(chunk)
        scratch.flush()
        found = hasher.hexdigest()
        if found != digest:
            raise SystemExit(
                f"digest mismatch for {url}:\n  expected {digest}\n  "
                f"found    {found}\nRefusing to unpack it.")
        print(f"sha256 verified: {found}", flush=True)

        target = Path(dest)
        target.mkdir(parents=True, exist_ok=True)
        with tarfile.open(scratch.name, "r:bz2") as archive:
            # `data` refuses absolute paths, traversal and device nodes --
            # the reason to extract with 3.12's filter rather than bare.
            archive.extractall(target, filter="data")
        print(f"unpacked into {target}", flush=True)


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit(__doc__)
    main(sys.argv[1], sys.argv[2], sys.argv[3])
