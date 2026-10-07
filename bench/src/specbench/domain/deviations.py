"""The deviation log of a run (`DEVIATIONS.md`): a closed set of kinds and one line format.

    - <kind> | arms: <scope> | <text>

`<scope>` is `none`, `all`, or arm names separated by commas. The writer and the
reader share this grammar; reading fails closed: any other non-blank line, apart
from the title, is a protocol deviation that touches every arm.
"""

from __future__ import annotations

import re
from collections.abc import Iterable
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.arms import Arm

DEVIATIONS_TITLE: Final = "# Deviations"
DEVIATIONS_FILE_NAME: Final = "DEVIATIONS.md"
ENTRY_PREFIX: Final = "- "
FIELD_SEPARATOR: Final = " | "
SCOPE_LABEL: Final = "arms: "
SCOPE_NONE: Final = "none"
SCOPE_ALL: Final = "all"
SCOPE_SEPARATOR: Final = ", "
_SCOPE_SPLIT = re.compile(r"\s*,\s*")
_ENTRY = re.compile(
    rf"^{re.escape(ENTRY_PREFIX)}(?P<kind>[a-z-]+){re.escape(FIELD_SEPARATOR)}"
    rf"{re.escape(SCOPE_LABEL)}(?P<scope>[^|]+?){re.escape(FIELD_SEPARATOR)}(?P<text>\S.*)$"
)
ARM_ORDER: Final = tuple(Arm)


class DeviationKind(Enum):
    PROVENANCE = "provenance"
    QUOTA_PAUSE = "quota-pause"
    QUOTA_BLOCKED = "quota-blocked"
    HARNESS_MISCONFIGURED = "harness-misconfigured"
    PROTOCOL = "protocol"


class ScopeRule(Enum):
    NONE_ONLY = "none only"
    SOME_ARM = "all or arm names"
    ANY = "none, all or arm names"


SCOPE_RULES: Final = {
    DeviationKind.PROVENANCE: ScopeRule.NONE_ONLY,
    DeviationKind.QUOTA_PAUSE: ScopeRule.NONE_ONLY,
    DeviationKind.QUOTA_BLOCKED: ScopeRule.SOME_ARM,
    DeviationKind.HARNESS_MISCONFIGURED: ScopeRule.SOME_ARM,
    DeviationKind.PROTOCOL: ScopeRule.ANY,
}


class InvalidDeviation(ValueError):
    pass


@dataclass(frozen=True)
class DeviationEntry:
    kind: DeviationKind
    arms: tuple[Arm, ...]
    text: str
    well_formed: bool = True

    @classmethod
    def create(cls, kind: DeviationKind, arms: Iterable[Arm], text: str) -> DeviationEntry:
        ordered = tuple(a for a in ARM_ORDER if a in set(arms))
        rule = SCOPE_RULES[kind]
        if rule is ScopeRule.NONE_ONLY and ordered:
            raise InvalidDeviation(f"a {kind.value} entry touches no arm; its scope is {SCOPE_NONE}")
        if rule is ScopeRule.SOME_ARM and not ordered:
            raise InvalidDeviation(f"a {kind.value} entry names {SCOPE_ALL} or the arms it affects")
        flat = " ".join(text.split())
        if not flat:
            raise InvalidDeviation("a deviation entry needs a text")
        return cls(kind, ordered, flat)

    @property
    def scope_text(self) -> str:
        if not self.arms:
            return SCOPE_NONE
        if self.arms == ARM_ORDER:
            return SCOPE_ALL
        return SCOPE_SEPARATOR.join(a.value for a in self.arms)

    def render(self) -> str:
        return f"{ENTRY_PREFIX}{self.kind.value}{FIELD_SEPARATOR}{SCOPE_LABEL}{self.scope_text}{FIELD_SEPARATOR}{self.text}"


def parse_scope(scope: str) -> tuple[Arm, ...]:
    value = scope.strip()
    if value == SCOPE_NONE:
        return ()
    if value == SCOPE_ALL:
        return ARM_ORDER
    names = [name for name in _SCOPE_SPLIT.split(value) if name]
    by_name = {a.value: a for a in ARM_ORDER}
    if not names or any(name not in by_name for name in names):
        raise InvalidDeviation(f"unknown arm scope {scope!r}")
    return tuple(by_name[name] for name in names)


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
            entries.append(DeviationEntry(DeviationKind.PROTOCOL, ARM_ORDER, line.strip(), well_formed=False))
    return entries


def render_deviations(entries: Iterable[DeviationEntry]) -> str:
    return "\n".join([DEVIATIONS_TITLE, "", *(e.render() for e in entries), ""])
