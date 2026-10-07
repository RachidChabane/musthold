"""The deviation log of a run (§14.2): one line grammar, a closed set of kinds and scopes, and their effects.

    - <kind> | scope: <scope> | <text>

`<scope>` is `none`, `all`, or identifiers separated by commas. Reading fails closed: any other non-blank line,
apart from the title, is a `protocol` entry with scope `all`.
"""

from __future__ import annotations

import re
from collections.abc import Iterable
from dataclasses import dataclass
from enum import Enum
from typing import Final

from calibration.domain.models import MODEL_IDS

DEVIATIONS_TITLE: Final = "# Deviations"
ENTRY_PREFIX: Final = "- "
FIELD_SEPARATOR: Final = " | "
SCOPE_LABEL: Final = "scope: "
SCOPE_NONE: Final = "none"
SCOPE_ALL: Final = "all"
SCOPE_SEPARATOR: Final = ", "
KIND_IDENTIFIERS: Final = ("K1", "K2", "K3", "K4")
SOURCE_IDENTIFIERS: Final = ("JH5", "JH3", "VH2")
RULE_IDENTIFIERS: Final = ("CS1", "CS2", "CS3")
SCOPE_IDENTIFIERS: Final = (*KIND_IDENTIFIERS, *MODEL_IDS, *SOURCE_IDENTIFIERS, *RULE_IDENTIFIERS)
_SCOPE_SPLIT: Final = re.compile(r"\s*,\s*")
_ENTRY: Final = re.compile(
    rf"^{re.escape(ENTRY_PREFIX)}(?P<kind>[a-z-]+){re.escape(FIELD_SEPARATOR)}"
    rf"{re.escape(SCOPE_LABEL)}(?P<scope>[^|]+?){re.escape(FIELD_SEPARATOR)}(?P<text>\S.*)$"
)


class DeviationKind(Enum):
    PROVENANCE = "provenance"
    PAUSE = "pause"
    BLOCKED = "blocked"
    PROTOCOL = "protocol"


OPERATOR_KINDS: Final = (DeviationKind.BLOCKED, DeviationKind.PROTOCOL)


class InvalidDeviation(ValueError):
    pass


@dataclass(frozen=True)
class DeviationEntry:
    kind: DeviationKind
    scope: tuple[str, ...]
    text: str
    well_formed: bool = True

    @property
    def covers_all(self) -> bool:
        return self.scope == (SCOPE_ALL,)

    def touches(self, identifiers: Iterable[str]) -> bool:
        """Whether the entry is in scope of an output or a rule named by these identifiers."""
        return self.covers_all or any(identifier in self.scope for identifier in identifiers)

    @classmethod
    def create(cls, kind: DeviationKind, scope: Iterable[str], text: str) -> DeviationEntry:
        names = _canonical_scope(scope)
        if kind in (DeviationKind.PROVENANCE, DeviationKind.PAUSE) and names not in ((SCOPE_NONE,),):
            raise InvalidDeviation(f"a {kind.value} entry touches no output; its scope is {SCOPE_NONE}")
        flat = " ".join(text.split())
        if not flat:
            raise InvalidDeviation("a deviation entry needs a text")
        return cls(kind, names, flat)

    def render(self) -> str:
        scope = SCOPE_SEPARATOR.join(self.scope)
        return f"{ENTRY_PREFIX}{self.kind.value}{FIELD_SEPARATOR}{SCOPE_LABEL}{scope}{FIELD_SEPARATOR}{self.text}"


def _canonical_scope(scope: Iterable[str]) -> tuple[str, ...]:
    names = tuple(dict.fromkeys(name.strip() for name in scope if name.strip()))
    if names in ((SCOPE_NONE,), (SCOPE_ALL,)):
        return names
    unknown = [name for name in names if name not in SCOPE_IDENTIFIERS]
    if not names or unknown:
        raise InvalidDeviation(f"scope {names!r} is none, all or identifiers of {SCOPE_IDENTIFIERS}")
    return tuple(name for name in SCOPE_IDENTIFIERS if name in names)


def parse_scope(text: str) -> tuple[str, ...]:
    return _canonical_scope(_SCOPE_SPLIT.split(text.strip()))


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
    entries: list[DeviationEntry] = []
    for line in text.splitlines():
        if not line.strip() or line.strip() == DEVIATIONS_TITLE:
            continue
        try:
            entries.append(parse_entry(line))
        except InvalidDeviation:
            entries.append(DeviationEntry(DeviationKind.PROTOCOL, (SCOPE_ALL,), line.strip(), well_formed=False))
    return entries


def render_deviations(entries: Iterable[DeviationEntry]) -> str:
    return "\n".join([DEVIATIONS_TITLE, "", *(entry.render() for entry in entries), ""])


def counts_by_kind(entries: Iterable[DeviationEntry]) -> dict[str, int]:
    counts = {kind.value: 0 for kind in DeviationKind}
    for entry in entries:
        counts[entry.kind.value] += 1
    return counts


@dataclass(frozen=True)
class Standing:
    """What the deviation log does to one output or rule (§14.2 effects)."""

    blocked: bool
    protocol: bool


def standing_of(entries: Iterable[DeviationEntry], identifiers: Iterable[str]) -> Standing:
    names = tuple(identifiers)
    logged = tuple(entries)
    return Standing(
        blocked=any(e.kind is DeviationKind.BLOCKED and e.touches(names) for e in logged),
        protocol=any(e.kind is DeviationKind.PROTOCOL and e.touches(names) for e in logged),
    )
