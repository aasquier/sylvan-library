"""Argon2id, at the OWASP minimum profile, and nothing else.

The parameters are not a default to tune later — they are the decision in
ADR 5, and the reason is memory rather than security. Argon2 is deliberately
memory-hard, so the widely-copied `m=64MB` setting means **each concurrent
login allocates 64 MB**. On the 1 GB instance `docs/HOSTING.md` §4 sizes, a
handful of simultaneous logins alongside DuckDB and a running simulation is how
the machine gets OOM-killed. `m=19MiB, t=2, p=1` is OWASP's stated minimum, is
still far beyond brute-force economics for a private site with a dozen
accounts, and cannot exhaust the box.

The other thing here is `verify_dummy()`, which exists to be *wasted*. If an
unknown username returns before a hash is computed and a known one returns
after, the response time answers "does this account exist" for anybody with a
stopwatch — and answering that is the same leak ADR 16 forbids the reset
endpoint from giving away through the front door.
"""

from __future__ import annotations

from contextlib import suppress
from functools import cache

from argon2 import PasswordHasher
from argon2.exceptions import (
    InvalidHashError,
    VerificationError,
    VerifyMismatchError,
)

from mtglab import caches

# OWASP minimum profile. `memory_cost` is in KiB: 19456 KiB == 19 MiB.
MEMORY_COST_KIB = 19_456
TIME_COST = 2
PARALLELISM = 1

# A password field long enough to be a denial of service is a password field
# with no upper bound: Argon2 hashes its input at whatever length it arrives,
# and 19 MiB of memory per attempt is already the expensive part. 1024 bytes is
# far past any real passphrase and far short of anything worth worrying about.
MAX_PASSWORD_BYTES = 1024

# The floor is a floor, not advice. Long is what matters; complexity rules push
# people toward `Passw0rd!` and this project has no interest in that argument.
MIN_PASSWORD_LENGTH = 12


class WeakPassword(ValueError):
    """Raised rather than storing a password the caller should have refused."""


@cache
def _hasher() -> PasswordHasher:
    return PasswordHasher(time_cost=TIME_COST, memory_cost=MEMORY_COST_KIB,
                          parallelism=PARALLELISM)


@cache
def _dummy_hash() -> str:
    """A real hash of a value nobody knows, computed once and reused.

    Once, not per call: computing it fresh each time would cost two hashes on
    an unknown user against one on a known user, which is the same timing
    signal pointing the other way.
    """
    return _hasher().hash("mtglab-dummy-password-for-timing-parity")


def check_strength(password: str) -> None:
    """Raise `WeakPassword` if this is not allowed to be stored."""
    if len(password.encode("utf-8")) > MAX_PASSWORD_BYTES:
        raise WeakPassword(
            f"password is longer than {MAX_PASSWORD_BYTES} bytes")
    if len(password) < MIN_PASSWORD_LENGTH:
        raise WeakPassword(
            f"password must be at least {MIN_PASSWORD_LENGTH} characters "
            "-- length beats punctuation, so a short phrase is a fine answer")


def hash_password(password: str) -> str:
    """Hash a password for storage. Refuses one that should not be stored."""
    check_strength(password)
    return _hasher().hash(password)


def verify(stored_hash: str | None, password: str) -> bool:
    """Is this the password behind that hash?

    `None` -- an account that has been created but has no password yet -- costs
    a dummy verification and returns False, so an invited-but-unclaimed account
    is not identifiable by how fast it is refused.

    Every argon2 failure mode collapses to `False`: a mismatch, a corrupt hash,
    a hash produced by some other library. There is no caller that wants to
    tell those apart at the point of a login, and one that returned a different
    answer for "malformed stored hash" would be reporting on the database to
    whoever typed the password.
    """
    if stored_hash is None:
        verify_dummy(password)
        return False
    try:
        return _hasher().verify(stored_hash, password)
    except (VerifyMismatchError, VerificationError, InvalidHashError):
        return False


def verify_dummy(password: str) -> None:
    """Burn a verification against a hash of nothing. For unknown accounts.

    The result is thrown away -- the work is the whole point. It will not
    match, because nobody knows the string it was made from.
    """
    with suppress(VerifyMismatchError, VerificationError, InvalidHashError):
        _hasher().verify(_dummy_hash(), password)


def needs_rehash(stored_hash: str) -> bool:
    """Was this hash made with weaker parameters than the ones above?

    Checked at login, when the plaintext is in hand and re-hashing is free --
    the only moment an old hash can be upgraded without asking anybody to do
    anything.
    """
    try:
        return _hasher().check_needs_rehash(stored_hash)
    except InvalidHashError:
        return False


#: Both of these are `functools.cache`, which counts itself -- so the register
#: only has to be told they exist. `_dummy_hash`'s docstring makes a claim
#: worth checking rather than trusting ("computed once and reused"): a hit rate
#: below ~100% across a run of failed logins would mean the timing defence is
#: paying for two hashes where it budgeted for one.
caches.register_lru("auth.hasher", _hasher,
                    note="the Argon2 hasher, built once per process")
caches.register_lru("auth.dummy-hash", _dummy_hash,
                    note="the throwaway hash an unknown account is verified "
                         "against, so login costs the same either way")
