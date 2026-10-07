"""The catalogue of the synthetic world: its criteria, tests and mutants, derived deterministically."""

from __future__ import annotations

import hashlib
from dataclasses import dataclass
from typing import Final

from calibration.domain.pilot_records import criterion_ref

CRITERIA: Final = (1, 2, 3, 4)
CRITERION_REFS: Final = tuple(criterion_ref(number) for number in CRITERIA)
NONE_OPTION: Final = "NONE"
COVERAGE_LEVELS: Final = ("NONE", "WEAK", "PARTIAL", "FULL")
ADEQUATE_LEVELS: Final = ("PARTIAL", "FULL")
CHOICE_VIOLATES: Final = "VIOLATES"
CHOICE_CONSISTENT: Final = "CONSISTENT"
SCREEN_OPTIONS: Final = (CHOICE_VIOLATES, CHOICE_CONSISTENT)
ARM_T: Final = "T"
ARM_P: Final = "P"
ARM_OTHER: Final = "O"
HASH_PREFIX_LENGTH: Final = 16
TOKEN_HEX_DIGITS: Final = 4
UNIT_DIGITS: Final = 12
UNIT_SCALE: Final = float(16**UNIT_DIGITS)
REPLY_STATUS_OK: Final = "OK"
KILLED: Final = "killed"
SURVIVED: Final = "survived"
FLAKY: Final = "flaky"
ERROR: Final = "error"
DIFFERS: Final = "DIFFERS"
NO_DIFF_FOUND: Final = "NO_DIFF_FOUND"
SKIPPED: Final = "SKIPPED"
TARGET_KILL_SHARE: Final = 0.7
STRAY_KILL_SHARE: Final = 0.04
FLAKY_SHARE: Final = 0.05
ERROR_SHARE: Final = 0.04
COLLATERAL_SHARE: Final = 0.25
DUD_PERIOD: Final = 7
DIFFERS_SHARE: Final = 0.6
NO_DIFF_SHARE: Final = 0.85


def unit_float(*parts: object) -> float:
    """A deterministic number in [0, 1) derived from the parts."""
    digest = hashlib.sha256("|".join(str(part) for part in parts).encode("utf-8")).hexdigest()
    return int(digest[:UNIT_DIGITS], 16) / UNIT_SCALE


def short_hash(*parts: object) -> str:
    return hashlib.sha256("|".join(str(part) for part in parts).encode("utf-8")).hexdigest()[:HASH_PREFIX_LENGTH]


@dataclass(frozen=True)
class TestSpec:
    __test__ = False

    nodeid: str
    cites: int
    label: str
    checks: tuple[int, ...] | str
    alignment_truth: str
    masked: bool = False

    @property
    def cites_ref(self) -> str:
        return criterion_ref(self.cites)

    @property
    def check_refs(self) -> list[str] | str:
        if isinstance(self.checks, str):
            return self.checks
        return [criterion_ref(number) for number in self.checks]


def _test(number: int, name: str, label: str, checks: tuple[int, ...] | str, *, masked: bool = False) -> TestSpec:
    aligned = label in ("GOOD", "GOOD_BROAD")
    return TestSpec(
        f"tests/test_park{number}.py::test_{name}",
        number,
        label,
        checks,
        "ALIGNED" if aligned else "NOT_ALIGNED",
        masked,
    )


TESTS: Final = (
    _test(1, "exit_before_entry_is_rejected", "GOOD", (1,)),
    _test(1, "exit_before_entry_with_lost_ticket", "WEAK", (1,)),
    _test(1, "exit_equals_entry", "VACUOUS", "NONE"),
    _test(2, "grace_period_is_fifteen_minutes", "GOOD_BROAD", (2, 1)),
    _test(2, "grace_period_neighbour", "MIS_OTHER", (3,)),
    _test(2, "grace_period_is_positive", "WEAK", (2,)),
    _test(3, "partial_hour_is_a_full_hour", "GOOD", (3,)),
    _test(3, "charge_is_quoted_in_cents", "OFFSPEC", "UNCOVERED"),
    _test(3, "hour_charge_is_non_negative", "WEAK", (3,)),
    _test(4, "cap_is_eighteen_euros", "GOOD", (4,)),
    _test(4, "cap_neighbour", "MIS_OTHER", (1,)),
    _test(4, "cap_mock", "VACUOUS", "NONE"),
    _test(4, "cap_with_leaking_name", "GOOD", (4,), masked=True),
)
ITEM_TESTS: Final = tuple(spec for spec in TESTS if not spec.masked)


@dataclass(frozen=True)
class MutantSpec:
    mutant_id: str
    arm: str
    target: int | None
    call_id: str | None
    source: str | None
    failed: tuple[int, ...] | None
    stillborn: bool = False
    status: str = REPLY_STATUS_OK
    duplicate_of: str | None = None

    @property
    def evaluated(self) -> bool:
        return not self.stillborn and self.status == REPLY_STATUS_OK

    @property
    def measured(self) -> bool:
        return self.evaluated and self.arm in (ARM_T, ARM_P)

    @property
    def cluster_number(self) -> int:
        """The criterion the mutant was written against or planted from."""
        if self.arm == ARM_P and self.source is not None:
            return int(self.source.removeprefix("PARK-").split("_")[0])
        return self.target or CRITERIA[0]


def _failed(mutant_id: str, anchor: int, index: int) -> tuple[int, ...]:
    if index % DUD_PERIOD == DUD_PERIOD - 1:
        return ()
    failed = {anchor}
    if unit_float(mutant_id, "collateral") < COLLATERAL_SHARE:
        failed.add(CRITERIA[int(unit_float(mutant_id, "other") * len(CRITERIA))])
    return tuple(sorted(failed))


ARM_T_COUNT: Final = 15
ARM_T_PER_CALL: Final = 3
ARM_P_SOURCES: Final = ("PARK-1_a", "PARK-2_a", "PARK-3_a", "PARK-4_a", "PARK-1_b", "PARK-2_b", "PARK-3_b", "PARK-4_b")
UNORACLED_SOURCE: Final = "PARK-1_c"


def _arm_t() -> list[MutantSpec]:
    mutants: list[MutantSpec] = []
    for index in range(ARM_T_COUNT):
        call = index // ARM_T_PER_CALL
        target = CRITERIA[call % len(CRITERIA)]
        mutant_id = short_hash("T", index)
        mutants.append(MutantSpec(mutant_id, ARM_T, target, f"gen-{call + 1}", None, _failed(mutant_id, target, index)))
    return mutants


def _arm_p() -> list[MutantSpec]:
    mutants: list[MutantSpec] = []
    for index, source in enumerate(ARM_P_SOURCES):
        number = int(source.removeprefix("PARK-").split("_")[0])
        mutant_id = short_hash("P", source)
        mutants.append(MutantSpec(mutant_id, ARM_P, number, None, source, _failed(mutant_id, number, index)))
    unoracled = short_hash("P", UNORACLED_SOURCE)
    mutants.append(MutantSpec(unoracled, ARM_P, 1, None, UNORACLED_SOURCE, None))
    return mutants


def _others(measured: list[MutantSpec]) -> list[MutantSpec]:
    first, second = measured[0], measured[1]
    return [
        MutantSpec(short_hash("O", 0), ARM_OTHER, 1, "gen-op", None, (1,)),
        MutantSpec(short_hash("S", 0), ARM_T, 1, "gen-1", None, None, stillborn=True),
        MutantSpec(short_hash("S", 1), ARM_T, 2, "gen-2", None, None, stillborn=True),
        MutantSpec(short_hash("D", 0), ARM_T, 1, "gen-1", None, None, status="DUPLICATE", duplicate_of=first.mutant_id),
        MutantSpec(
            short_hash("D", 1), ARM_T, 2, "gen-2", None, None, status="DUPLICATE", duplicate_of=second.mutant_id
        ),
        MutantSpec(short_hash("E", 0), ARM_T, 3, "gen-3", None, None, status="TRIVIALLY_EQUIVALENT"),
    ]


def build_mutants() -> tuple[MutantSpec, ...]:
    measured = [*_arm_t(), *_arm_p()]
    return tuple(sorted([*measured, *_others(measured)], key=lambda mutant: mutant.mutant_id))


MUTANTS: Final = build_mutants()
MEASURED_MUTANTS: Final = tuple(mutant for mutant in MUTANTS if mutant.measured)
ORACLED_MUTANTS: Final = tuple(mutant for mutant in MEASURED_MUTANTS if mutant.failed is not None)
