"""The oracle parts of ADR 0004 (U, R, H, O), the violations and the planted refactors, as read from a sealed bundle."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final, cast

from specbench.domain.sealing import SealedBundle, SealingError

SPEC_PATH: Final = "SPEC.md"
ORACLE_MANIFEST_PATH: Final = "oracle/manifest.json"
VIOLATIONS_INDEX_PATH: Final = "violations/index.json"
PLANTED_INDEX_PATH: Final = "planted/index.json"
ORACLE_PREFIX: Final = "oracle/"
MIN_VIOLATIONS_PER_CRITERION: Final = 2
MIN_PLANTED: Final = 2


class Part(Enum):
    U = "U"
    R = "R"
    H = "H"
    O = "O"  # noqa: E741 - the part's name in the ADR


OVERLAID_PARTS: Final = (Part.U, Part.H, Part.O)
"""Parts whose files are shipped in the bundle; R is made of tests that already exist in the repository."""


class Outcome(Enum):
    PASSED = "passed"
    FAILED = "failed"
    ERROR = "error"
    MISSING = "missing"
    FLAKY = "flaky"

    @property
    def is_failure(self) -> bool:
        return self in (Outcome.FAILED, Outcome.ERROR, Outcome.MISSING)


@dataclass(frozen=True)
class OracleTest:
    test_id: str
    part: Part
    criterion: str | None


@dataclass(frozen=True)
class Violation:
    violation_id: str
    criterion: str
    patch_path: str


@dataclass(frozen=True)
class PlantedRefactor:
    planted_id: str
    patch_path: str


@dataclass(frozen=True)
class Oracle:
    tests: tuple[OracleTest, ...]
    violations: tuple[Violation, ...]
    planted: tuple[PlantedRefactor, ...]

    def tests_of(self, *parts: Part) -> tuple[OracleTest, ...]:
        return tuple(t for t in self.tests if t.part in parts)

    def criterion_tests(self, criterion: str) -> tuple[OracleTest, ...]:
        return tuple(t for t in self.tests_of(Part.U, Part.H) if t.criterion == criterion)

    def overlay_paths(self, bundle: SealedBundle, *parts: Part) -> dict[str, str]:
        """Destination path in the tree -> bundle path, for the files of the given parts."""
        mapping: dict[str, str] = {}
        for part in parts:
            prefix = f"{ORACLE_PREFIX}{part.value}/"
            for path in bundle.files:
                if path.startswith(prefix):
                    mapping[path.removeprefix(prefix)] = path
        return mapping


def _load(bundle: SealedBundle, path: str) -> object:
    if path not in bundle.files:
        raise SealingError(f"bundle lacks {path}")
    return json.loads(bundle.files[path])


def _entries(value: object, where: str) -> list[dict[str, object]]:
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in cast(list[object], value)):
        raise SealingError(f"{where} must be a list of objects")
    return cast(list[dict[str, object]], value)


def _test(entry: Mapping[str, object]) -> OracleTest:
    part = Part(entry["part"])
    criterion = entry.get("criterion")
    if part in (Part.U, Part.H) and not isinstance(criterion, str):
        raise SealingError(f"{part.value} test {entry.get('id')} names no criterion")
    return OracleTest(str(entry["id"]), part, criterion if isinstance(criterion, str) else None)


def oracle_from_bundle(bundle: SealedBundle) -> Oracle:
    tests = tuple(_test(e) for e in _entries(_load(bundle, ORACLE_MANIFEST_PATH), ORACLE_MANIFEST_PATH))
    violations = tuple(
        Violation(str(e["id"]), str(e["criterion"]), str(e["patch"]))
        for e in _entries(_load(bundle, VIOLATIONS_INDEX_PATH), VIOLATIONS_INDEX_PATH)
    )
    planted = tuple(
        PlantedRefactor(str(e["id"]), str(e["patch"]))
        for e in _entries(_load(bundle, PLANTED_INDEX_PATH), PLANTED_INDEX_PATH)
    )
    ids = [t.test_id for t in tests]
    if len(set(ids)) != len(ids):
        raise SealingError("oracle test ids repeat")
    for item in (*violations, *planted):
        patch = item.patch_path
        if patch not in bundle.files:
            raise SealingError(f"patch {patch} is not in the bundle")
    return Oracle(tests, violations, planted)
