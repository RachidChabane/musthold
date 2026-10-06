"""The analysis context: a run and its ground truth, with the derived views every module shares."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from functools import cached_property

from pilot.analysis.oracle import OracleClass, classify_oracle
from pilot.domain.analysis_constants import ARM_T
from pilot.domain.execution_validation import other_binding_veto
from pilot.domain.gate import CriterionEvidence, MutantResults
from pilot.domain.outcomes import KillResult
from pilot.domain.labels import TestLabel
from pilot.domain.matrix import PoolMutant
from pilot.domain.analysis_constants import ERROR_RESULT, KILLED_RESULT, MUTANT_UNDETERMINED, MUTANT_VALID_W
from pilot.domain.pipeline_constants import TAU
from pilot.domain.refs import criterion_ref
from pilot.domain.run_records import GroundTruth, MutantClassView, RunRecords


class IncompleteInputs(Exception):
    """The run or the ground truth lacks something the analysis needs."""


@dataclass(frozen=True)
class Context:
    run: RunRecords
    truth: GroundTruth

    @cached_property
    def labels(self) -> dict[str, TestLabel]:
        return {label.nodeid: label for label in self.truth.labels}

    @cached_property
    def nodeids(self) -> tuple[str, ...]:
        return tuple(sorted(self.labels))

    @cached_property
    def criteria(self) -> tuple[int, ...]:
        return tuple(sorted({label.cites for label in self.labels.values()}))

    @cached_property
    def declared(self) -> dict[str, int]:
        """D(t): the declared criterion of each test, from the traceability report."""
        return {nodeid: self.run.bindings[nodeid] for nodeid in self.nodeids if nodeid in self.run.bindings}

    @cached_property
    def killers(self) -> dict[str, frozenset[str]]:
        """Tests with a reproduced kill, per evaluated mutant; error, flaky and survived are not kills."""
        return {
            mutant_id: frozenset(n for n, r in results.items() if r == KILLED_RESULT)
            for mutant_id, results in self.run.kills.items()
        }

    @cached_property
    def errored(self) -> dict[str, frozenset[str]]:
        """Tests whose result is error, per evaluated mutant: neither a kill nor a survival."""
        return {
            mutant_id: frozenset(n for n, r in results.items() if r == ERROR_RESULT)
            for mutant_id, results in self.run.kills.items()
        }

    def vetoed(self, mutant_id: str, criterion: int) -> bool:
        """True when a test declared for another criterion has a reproduced kill or an error on the mutant."""
        results = {nodeid: KillResult(result) for nodeid, result in self.run.kills.get(mutant_id, {}).items()}
        return other_binding_veto(results, self.refs, criterion_ref(criterion))

    @cached_property
    def refs(self) -> dict[str, str]:
        """D(t) in the `PARK-n~1` text form, as the isolation predicate reads it."""
        return {nodeid: criterion_ref(number) for nodeid, number in self.declared.items()}

    def results_of(self, mutant_id: str) -> MutantResults:
        return MutantResults(self.killers.get(mutant_id, frozenset()), self.errored.get(mutant_id, frozenset()))

    @cached_property
    def oracle_class(self) -> dict[str, OracleClass]:
        return {
            mutant_id: classify_oracle(record.failed_criteria, self.run.differential.get(mutant_id))
            for mutant_id, record in self.truth.oracle.items()
        }

    def arm_t_evaluated(self) -> list[MutantClassView]:
        """Arm-T mutants that were classified (non-stillborn, status OK)."""
        return [c for c in self.run.classes.values() if c.arm == ARM_T and self.run.is_evaluated(c.mutant_id)]

    def oracle_valid_for(self, mutant_id: str, criterion: int | None) -> bool:
        found = self.oracle_class.get(mutant_id)
        return found is not None and found.valid_for(criterion)

    def gold_good(self, nodeid: str) -> bool:
        return self.labels[nodeid].test_class.is_gold_good

    def missing_oracle(self) -> list[str]:
        """Non-stillborn mutants of any arm without an oracle record."""
        return sorted(
            m for m, p in self.run.processing.items() if not p.stillborn and m not in self.truth.oracle
        )


def matrix_pools(ctx: Context) -> tuple[list[PoolMutant], list[frozenset[str]]]:
    """Primary-matrix VALID mutants and UNATTRIBUTED mutants of arm T (PREREG 8)."""
    valid: list[PoolMutant] = []
    unattributed: list[frozenset[str]] = []
    for view in sorted(ctx.arm_t_evaluated(), key=lambda v: v.mutant_id):
        killed = ctx.killers.get(view.mutant_id, frozenset())
        if view.primary_criterion is not None:
            valid.append(PoolMutant(view.primary_criterion, killed))
        elif view.unattributed:
            unattributed.append(killed)
    return valid, unattributed


class EvidenceTruth(Enum):
    """Which links of the gate's evidence are replaced by the oracle's class (PREREG 9.4)."""

    NONE = "none"
    VALIDITY = "validity"
    ISOLATION = "isolation"
    ALL = "all"

    @property
    def validity_by_oracle(self) -> bool:
        return self in (EvidenceTruth.VALIDITY, EvidenceTruth.ALL)

    @property
    def isolation_by_oracle(self) -> bool:
        return self in (EvidenceTruth.ISOLATION, EvidenceTruth.ALL)


def gate_evidence(ctx: Context, criterion: int, truth: EvidenceTruth = EvidenceTruth.NONE) -> CriterionEvidence:
    """Executed results the gate rules 3 to 5 read for one criterion: the mutants of its discriminating-power proof.

    VALID_x: undiscarded arm-T mutants generated for the criterion and VALID_x for it at or above tau,
    COLLATERAL ones included. VALID_w only: VALID_w for it, vetoed by execution. UNDETERMINED: undiscarded
    arm-T mutants generated for the criterion that are UNDETERMINED for it. A substituted link takes the
    oracle's class in place of its own decision.
    """
    valid_x: list[MutantResults] = []
    valid_w_only: list[MutantResults] = []
    undetermined: list[MutantResults] = []
    for view in ctx.arm_t_evaluated():
        if view.target != criterion or view.discarded:
            continue
        results = ctx.results_of(view.mutant_id)
        oracle_valid = ctx.oracle_valid_for(view.mutant_id, criterion)
        judged = view.verdict_for(criterion) == MUTANT_VALID_W and view.valid_confidence.get(criterion, 0.0) >= TAU
        valid_w = oracle_valid if truth.validity_by_oracle else judged
        if truth.isolation_by_oracle:
            valid_x_here = valid_w and oracle_valid
        elif truth.validity_by_oracle:
            valid_x_here = valid_w and not ctx.vetoed(view.mutant_id, criterion)
        else:
            valid_x_here = valid_w and criterion in view.valid_x
        if valid_x_here:
            valid_x.append(results)
        elif valid_w:
            valid_w_only.append(results)
        elif view.verdict_for(criterion) == MUTANT_UNDETERMINED and not truth.validity_by_oracle:
            undetermined.append(results)
    return CriterionEvidence(tuple(valid_x), tuple(valid_w_only), tuple(undetermined))


def oracle_valid_kill_sets(ctx: Context, criterion: int) -> list[frozenset[str]]:
    """Kill sets of every arm-T mutant that is oracle-valid for the criterion (PREREG 9.3)."""
    return [ctx.results_of(m).killed for m in oracle_valid_mutants(ctx, criterion)]


def oracle_valid_mutants(ctx: Context, criterion: int) -> list[str]:
    return [view.mutant_id for view in ctx.arm_t_evaluated() if ctx.oracle_valid_for(view.mutant_id, criterion)]
