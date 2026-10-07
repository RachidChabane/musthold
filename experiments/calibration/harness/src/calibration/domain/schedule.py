"""Request ids and the order and passes in which they are sent (§4.5)."""

from __future__ import annotations

from collections.abc import Iterable
from dataclasses import dataclass
from enum import Enum
from typing import Final

from calibration.domain.canonical import sha256_hex, slugify
from calibration.domain.constants import (
    HASH_SEPARATOR,
    PASS2_HEX_DIGITS,
    PASS2_LABEL,
    PASS2_MODULUS,
    SEED_TEXT,
)

ALIGNMENT_PREFIX: Final = "A"
MUTANT_PREFIX: Final = "M"


class Pass(Enum):
    ONE = "pass1"
    TWO = "pass2"


def alignment_request_id(nodeid: str) -> str:
    return ALIGNMENT_PREFIX + HASH_SEPARATOR + slugify(nodeid)


def mutant_request_id(mutant_id: str) -> str:
    return MUTANT_PREFIX + HASH_SEPARATOR + mutant_id


def is_alignment_request(request_id: str) -> bool:
    return request_id.startswith(ALIGNMENT_PREFIX + HASH_SEPARATOR)


def request_slug(request_id: str) -> str:
    """The directory and file stem of a request: `A|tests_x.py__t` becomes `A_tests_x.py__t`."""
    return slugify(request_id)


def order_key(request_id: str) -> str:
    return sha256_hex(SEED_TEXT + HASH_SEPARATOR + request_id)


def pass1_order(request_ids: Iterable[str]) -> list[str]:
    """Ascending order of `sha256("20261007|" + request_id)`; the same for every model."""
    return sorted(set(request_ids), key=lambda request_id: (order_key(request_id), request_id))


def in_pass2(request_id: str) -> bool:
    """Every alignment request, and each mutant request whose pass-2 hash is divisible by 4."""
    if is_alignment_request(request_id):
        return True
    digest = sha256_hex(SEED_TEXT + HASH_SEPARATOR + PASS2_LABEL + HASH_SEPARATOR + request_id)
    return int(digest[:PASS2_HEX_DIGITS], 16) % PASS2_MODULUS == 0


@dataclass(frozen=True)
class Schedule:
    """The two passes of a run: pass 2 is the reverse of the pass-1 order restricted to its members."""

    pass1: tuple[str, ...]
    pass2: tuple[str, ...]

    @classmethod
    def of(cls, request_ids: Iterable[str]) -> Schedule:
        first = pass1_order(request_ids)
        second = tuple(request_id for request_id in reversed(first) if in_pass2(request_id))
        return cls(tuple(first), second)

    def ids_of(self, which: Pass) -> tuple[str, ...]:
        return self.pass1 if which is Pass.ONE else self.pass2
