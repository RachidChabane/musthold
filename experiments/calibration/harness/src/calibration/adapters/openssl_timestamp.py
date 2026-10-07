"""`TimestampReader` over `openssl ts -reply -in <file> -text`: the message imprint and the genTime, UTC."""

from __future__ import annotations

import re
from datetime import UTC, datetime
from pathlib import Path
from typing import Final

from calibration.domain.constants import ENCODING
from calibration.domain.ports import ProcessRunner, Timestamp

OPENSSL: Final = "openssl"
MESSAGE_DATA_HEADER: Final = "Message data:"
IMPRINT_LINE: Final = re.compile(r"^\s*[0-9a-f]{4} - (?P<bytes>.{47})")
HEX_BYTE: Final = re.compile(r"\b[0-9a-f]{2}\b")
TIME_LINE: Final = re.compile(
    r"^Time stamp:\s+(?P<month>[A-Za-z]{3})\s+(?P<day>\d+)\s+(?P<time>\d{1,2}:\d{2}:\d{2})\s+(?P<year>\d{4})\s+GMT\s*$"
)
MONTHS: Final = ("Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec")
TIME_SEPARATOR: Final = ":"


def parse_listing(listing: str) -> Timestamp | None:
    """The imprint and genTime of an `openssl ts -reply -text` listing, None when either is missing."""
    imprint: list[str] = []
    in_data = False
    generated: datetime | None = None
    for line in listing.splitlines():
        if line.strip() == MESSAGE_DATA_HEADER:
            in_data = True
            continue
        bytes_match = IMPRINT_LINE.match(line) if in_data else None
        if bytes_match:
            imprint += HEX_BYTE.findall(bytes_match.group("bytes"))
            continue
        in_data = False
        generated = _generation_time(line) or generated
    if not imprint or generated is None:
        return None
    return Timestamp("".join(imprint), generated)


def _generation_time(line: str) -> datetime | None:
    match = TIME_LINE.match(line)
    if match is None or match.group("month") not in MONTHS:
        return None
    hours, minutes, seconds = (int(part) for part in match.group("time").split(TIME_SEPARATOR))
    try:
        return datetime(
            int(match.group("year")),
            MONTHS.index(match.group("month")) + 1,
            int(match.group("day")),
            hours,
            minutes,
            seconds,
            tzinfo=UTC,
        )
    except ValueError:
        return None


class OpensslTimestampReader:
    """Implements `TimestampReader`."""

    def __init__(self, runner: ProcessRunner) -> None:
        self._runner = runner

    def read(self, reply: Path) -> Timestamp | None:
        if not reply.is_file():
            return None
        result = self._runner.run([OPENSSL, "ts", "-reply", "-in", str(reply), "-text"])
        if result.returncode != 0:
            return None
        return parse_listing(result.stdout.decode(ENCODING, errors="replace"))
