"""Confidence sources (§3.1): the ids of the experiment and the `confidenceSource` strings the product keys on."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Final

SYSTEM_ONE_KIND: Final = "system-one"
HOST_VOTES_KIND: Final = "host-votes"
SOURCE_SEPARATOR: Final = ":"
ENDPOINT_SEPARATOR: Final = "@"


class SourceId(Enum):
    """The sources of §3.1; S1 stands for the six models, each of them a cell of its own."""

    S1 = "S1"
    JH5 = "JH5"
    JH3 = "JH3"
    VH2 = "VH2"


HOST_VOTE_COUNT: Final = {SourceId.JH5: 5, SourceId.JH3: 3, SourceId.VH2: 2}


def system_one_source(model: str, endpoint_digest: str) -> str:
    """`system-one:<model>@<endpointDigest>`, the text `format.SystemOneEndpointSource` builds."""
    return f"{SYSTEM_ONE_KIND}{SOURCE_SEPARATOR}{model}{ENDPOINT_SEPARATOR}{endpoint_digest}"


def host_votes_source(votes: int) -> str:
    return f"{HOST_VOTES_KIND}({votes})"


def host_source_of(source: SourceId) -> str:
    return host_votes_source(HOST_VOTE_COUNT[source])


@dataclass(frozen=True)
class ParsedSystemOneSource:
    model: str
    endpoint_digest: str


def parse_system_one_source(text: str) -> ParsedSystemOneSource | None:
    """The model and the endpoint digest of a `system-one:<model>@<digest>` text, None for any other text."""
    prefix = SYSTEM_ONE_KIND + SOURCE_SEPARATOR
    if not text.startswith(prefix):
        return None
    origin = text[len(prefix) :]
    cut = origin.rfind(ENDPOINT_SEPARATOR)
    if cut <= 0 or cut == len(origin) - len(ENDPOINT_SEPARATOR):
        return None
    return ParsedSystemOneSource(origin[:cut], origin[cut + 1 :])
