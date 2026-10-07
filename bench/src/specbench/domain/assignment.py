"""U to O assignment (ADR 0004): which human assertions the spec and its Interface imply.

Each upstream test is classified independently by two models. It stays in U only when both say the spec plus the
Interface implies it and both name the same criterion; anything else moves it to O. The rule is deterministic and
every classification is sealed with the task.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import boolean, mapping, optional_text, sequence, text


@dataclass(frozen=True)
class Classification:
    test_id: str
    implied: bool
    criterion: int | None
    reason: str


def classifications_from_record(record: object) -> dict[str, Classification]:
    body = mapping(record, "assignment")
    found: dict[str, Classification] = {}
    for raw in sequence(body.get("classifications"), "classifications"):
        entry = mapping(raw, "classification")
        criterion = entry.get("criterion")
        found[text(entry.get("test_id"), "test_id")] = Classification(
            text(entry.get("test_id"), "test_id"),
            boolean(entry.get("implied"), "implied"),
            None if criterion is None else int(str(criterion)),
            optional_text(entry.get("reason"), "reason") or "",
        )
    return found


@dataclass(frozen=True)
class Assignment:
    """U with the criterion each test checks, O, and the tests the two models disagreed on."""

    u: Mapping[str, int]
    o: tuple[str, ...]
    disagreements: tuple[str, ...]

    def to_record(
        self, first: Mapping[str, Classification], second: Mapping[str, Classification]
    ) -> dict[str, JsonValue]:
        def view(source: Mapping[str, Classification]) -> dict[str, JsonValue]:
            return {
                t: {"criterion": c.criterion, "implied": c.implied, "reason": c.reason}
                for t, c in sorted(source.items())
            }

        return {
            "disagreements": list(self.disagreements),
            "o": list(self.o),
            "opus": view(first),
            "haiku": view(second),
            "u": dict(sorted(self.u.items())),
        }


def assign(
    test_ids: Sequence[str],
    first: Mapping[str, Classification],
    second: Mapping[str, Classification],
    criteria_count: int,
) -> Assignment:
    """U needs both classifiers to say implied, name the same existing criterion; a missing answer counts as not implied."""
    u: dict[str, int] = {}
    o: list[str] = []
    disagreements: list[str] = []
    for test_id in test_ids:
        a, b = first.get(test_id), second.get(test_id)
        agree = (
            a is not None
            and b is not None
            and a.implied
            and b.implied
            and a.criterion is not None
            and a.criterion == b.criterion
            and 1 <= a.criterion <= criteria_count
        )
        if agree and a is not None and a.criterion is not None:
            u[test_id] = a.criterion
        else:
            o.append(test_id)
            if a is None or b is None or a.implied != b.implied or a.criterion != b.criterion:
                disagreements.append(test_id)
    return Assignment(u, tuple(o), tuple(disagreements))
