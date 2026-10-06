"""Entries of a run's `DEVIATIONS.md` (PREREG 4.4, 10 P6 and P7): a closed set of kinds, one line format.

Every entry is one line:

    - <kind> | endpoints: <scope> | <text>

`<scope>` is `none`, `all`, or decisional endpoint ids separated by commas. The
writer (pipeline and `log-deviation`) and the reader (analysis) share this module,
so an entry is rendered and parsed by the same grammar. Reading fails closed: any
other non-blank line, apart from the title, is a protocol deviation touching every
endpoint.
"""

from __future__ import annotations

import re
from collections.abc import Iterable
from dataclasses import dataclass
from enum import Enum
from typing import Final

from pilot.domain.endpoint_ids import ENDPOINT_IDS

DEVIATIONS_TITLE: Final = "# Deviations"
ENTRY_PREFIX: Final = "- "
FIELD_SEPARATOR: Final = " | "
SCOPE_LABEL: Final = "endpoints: "
SCOPE_NONE: Final = "none"
SCOPE_ALL: Final = "all"
SCOPE_SEPARATOR: Final = ", "
_SCOPE_SPLIT = re.compile(r"\s*,\s*")
_ENTRY = re.compile(
    rf"^{re.escape(ENTRY_PREFIX)}(?P<kind>[a-z-]+){re.escape(FIELD_SEPARATOR)}"
    rf"{re.escape(SCOPE_LABEL)}(?P<scope>[^|]+?){re.escape(FIELD_SEPARATOR)}(?P<text>\S.*)$"
)


class DeviationKind(Enum):
    """The closed set of entry kinds."""

    PROVENANCE = "provenance"
    QUOTA_PAUSE = "quota-pause"
    QUOTA_BLOCKED = "quota-blocked"
    PROTOCOL = "protocol"


class ScopeRule(Enum):
    NONE_ONLY = "none only"
    SOME_ENDPOINT = "all or endpoint ids"
    ANY = "none, all or endpoint ids"


OPERATOR_KINDS: Final = (DeviationKind.QUOTA_BLOCKED, DeviationKind.PROTOCOL)
"""Kinds an operator logs with `log-deviation`; the pipeline alone writes the others."""

SCOPE_RULES: Final = {
    DeviationKind.PROVENANCE: ScopeRule.NONE_ONLY,
    DeviationKind.QUOTA_PAUSE: ScopeRule.NONE_ONLY,
    DeviationKind.QUOTA_BLOCKED: ScopeRule.SOME_ENDPOINT,
    DeviationKind.PROTOCOL: ScopeRule.ANY,
}


class InvalidDeviation(ValueError):
    """An entry that the grammar of this module does not accept."""


@dataclass(frozen=True)
class DeviationEntry:
    kind: DeviationKind
    endpoints: tuple[str, ...]
    text: str
    well_formed: bool = True

    @classmethod
    def create(cls, kind: DeviationKind, endpoints: Iterable[str], text: str) -> DeviationEntry:
        """A validated entry: the scope obeys the kind's rule and the text is one non-empty line."""
        ordered = _ordered(endpoints)
        _check_scope(kind, ordered)
        flat = " ".join(text.split())
        if not flat:
            raise InvalidDeviation("a deviation entry needs a text")
        return cls(kind, ordered, flat)

    @classmethod
    def provenance(cls, text: str) -> DeviationEntry:
        return cls.create(DeviationKind.PROVENANCE, (), text)

    @classmethod
    def quota_pause(cls, text: str) -> DeviationEntry:
        return cls.create(DeviationKind.QUOTA_PAUSE, (), text)

    @classmethod
    def protocol_for_all(cls, text: str) -> DeviationEntry:
        return cls.create(DeviationKind.PROTOCOL, ENDPOINT_IDS, text)

    @property
    def scope_text(self) -> str:
        if not self.endpoints:
            return SCOPE_NONE
        if self.endpoints == ENDPOINT_IDS:
            return SCOPE_ALL
        return SCOPE_SEPARATOR.join(self.endpoints)

    def render(self) -> str:
        return f"{ENTRY_PREFIX}{self.kind.value}{FIELD_SEPARATOR}{SCOPE_LABEL}{self.scope_text}{FIELD_SEPARATOR}{self.text}"


def parse_scope(scope: str) -> tuple[str, ...]:
    """`none`, `all`, or comma-separated endpoint ids, returned in endpoint order."""
    value = scope.strip()
    if value == SCOPE_NONE:
        return ()
    if value == SCOPE_ALL:
        return ENDPOINT_IDS
    names = [name for name in _SCOPE_SPLIT.split(value) if name]
    unknown = [name for name in names if name not in ENDPOINT_IDS]
    if unknown or not names:
        raise InvalidDeviation(f"unknown endpoint scope {scope!r}")
    return _ordered(names)


def parse_entry(line: str) -> DeviationEntry:
    match = _ENTRY.match(line.rstrip())
    if match is None:
        raise InvalidDeviation(f"not a deviation entry: {line!r}")
    try:
        kind = DeviationKind(match.group("kind"))
    except ValueError as unknown:
        raise InvalidDeviation(f"unknown deviation kind {match.group('kind')!r}") from unknown
    return DeviationEntry.create(kind, parse_scope(match.group("scope")), match.group("text"))


def parse_deviations(text: str) -> list[DeviationEntry]:
    """Every entry of the file; a line that does not parse is a protocol deviation touching every endpoint."""
    entries: list[DeviationEntry] = []
    for line in text.splitlines():
        if not line.strip() or line.strip() == DEVIATIONS_TITLE:
            continue
        try:
            entries.append(parse_entry(line))
        except InvalidDeviation:
            entries.append(DeviationEntry(DeviationKind.PROTOCOL, ENDPOINT_IDS, line.strip(), well_formed=False))
    return entries


def _ordered(endpoints: Iterable[str]) -> tuple[str, ...]:
    wanted = set(endpoints)
    unknown = sorted(wanted - set(ENDPOINT_IDS))
    if unknown:
        raise InvalidDeviation(f"unknown endpoints {unknown}")
    return tuple(e for e in ENDPOINT_IDS if e in wanted)


def _check_scope(kind: DeviationKind, endpoints: tuple[str, ...]) -> None:
    rule = SCOPE_RULES[kind]
    if rule is ScopeRule.NONE_ONLY and endpoints:
        raise InvalidDeviation(f"a {kind.value} entry touches no endpoint; its scope is {SCOPE_NONE}")
    if rule is ScopeRule.SOME_ENDPOINT and not endpoints:
        raise InvalidDeviation(f"a {kind.value} entry names {SCOPE_ALL} or the endpoints it blocks")
