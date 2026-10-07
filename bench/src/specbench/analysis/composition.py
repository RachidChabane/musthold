"""Composition check and absent-module probes, read from recorded Shallnot verdicts; no gate rule is computed here.

At every S_T, ALL's per-criterion verdict must equal Shallnot's combination of the AL and DP proofs, which the
run records as a second Shallnot report. Mismatches are counted per module. The absent-module probe asserts per
arm that an absent module never counted as passed.
"""

from __future__ import annotations

from collections import Counter
from collections.abc import Sequence
from dataclasses import dataclass

from specbench.domain.arms import Arm, Harness, Module
from specbench.domain.records import TrunkRecord


@dataclass(frozen=True)
class CompositionSummary:
    module: Module
    criteria: int
    mismatches: int


@dataclass(frozen=True)
class AbsentModuleSummary:
    arm: Arm
    module: Module
    probes: int
    counted_as_passed: int


def composition_summary(trunks: Sequence[TrunkRecord], harness: Harness) -> list[CompositionSummary]:
    rows: Counter[Module] = Counter()
    wrong: Counter[Module] = Counter()
    for trunk in trunks:
        if trunk.harness is not harness:
            continue
        for row in trunk.composition:
            rows[row.module] += 1
            wrong[row.module] += not row.matches
    return [CompositionSummary(module, rows[module], wrong[module]) for module in sorted(rows, key=lambda m: m.value)]


def absent_module_summary(trunks: Sequence[TrunkRecord], harness: Harness) -> list[AbsentModuleSummary]:
    probes: Counter[tuple[Arm, Module]] = Counter()
    passed: Counter[tuple[Arm, Module]] = Counter()
    for trunk in trunks:
        if trunk.harness is not harness:
            continue
        for probe in trunk.absent_probes:
            probes[(probe.arm, probe.module)] += 1
            passed[(probe.arm, probe.module)] += probe.counted_as_passed
    return [
        AbsentModuleSummary(arm, module, probes[(arm, module)], passed[(arm, module)])
        for arm, module in sorted(probes, key=lambda key: (key[0].value, key[1].value))
    ]
