"""Typed view of a run directory (harness/RUN_RECORDS.md) and of the ground truth, as `analysis` reads them."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from typing import Any

from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.analysis_constants import MUTANT_VALID_W, STATUS_OK
from pilot.domain.labels import TestLabel


@dataclass(frozen=True)
class MutantMeta:
    mutant_id: str
    arm: str
    target: int | None
    slot: str | None
    generator_model: str | None
    generator_cli: str | None
    generator_call_id: str | None
    hosts: tuple[str, ...] = ()
    """The definitions the mutant edits, as `file::qualname`."""


@dataclass(frozen=True)
class ProcessingRecord:
    mutant_id: str
    stillborn: bool
    status: str
    duplicate_of: str | None


@dataclass(frozen=True)
class MutantClassView:
    mutant_id: str
    arm: str
    target: int | None
    per_criterion: Mapping[int, str]
    valid_x: frozenset[int]
    valid_confidence: Mapping[int, float]
    equivalent: bool
    unattributed: bool
    off_target: bool
    collateral: bool
    primary_criterion: int | None
    discarded: bool
    survived_all: bool

    @property
    def primary_included(self) -> bool:
        return self.primary_criterion is not None

    def verdict_for(self, criterion: int) -> str | None:
        return self.per_criterion.get(criterion)

    @property
    def valid_w_criteria(self) -> tuple[int, ...]:
        return tuple(sorted(c for c, v in self.per_criterion.items() if v == MUTANT_VALID_W))

    @property
    def primary_valid_x(self) -> bool:
        """Primary-matrix mutant of its target that execution validation keeps (VALID_x for the target)."""
        return self.primary_criterion is not None and self.primary_criterion in self.valid_x

    @property
    def target_valid(self) -> bool:
        return self.target is not None and self.verdict_for(self.target) == MUTANT_VALID_W


@dataclass(frozen=True)
class AlignmentView:
    modal_ref: str | None
    modal_share: float
    modal_coverage: str | None
    mean_confidence: float | None
    verdict: AlignmentVerdict
    verdict_s1: AlignmentVerdict
    missing_reps: int


@dataclass(frozen=True)
class AlignmentCall:
    judge: str
    arm: str
    nodeid: str
    rep: int
    call_id: str
    status: str
    verifies_ref: str | None = None


@dataclass(frozen=True)
class ProbeRow:
    judge: str
    nodeid: str
    rep: int
    status: str
    guess: str | None
    confidence: float | None


@dataclass(frozen=True)
class RelabelRow:
    rater: str
    nodeid: str
    label: str


@dataclass(frozen=True)
class ValidityRow:
    mutant_id: str
    judge: str
    call_id: str
    status: str
    payload: Mapping[str, Any] | None


@dataclass(frozen=True)
class WitnessRow:
    mutant_id: str
    judge: str
    criterion: int
    confirmed: bool


@dataclass(frozen=True)
class CallRecord:
    """Top-level `raw/<call_id>/result.json` of one model call."""

    call_id: str
    status: str
    cost_usd: float | None
    input_tokens: int
    output_tokens: int
    isolation_violations: int


@dataclass(frozen=True)
class PlantedItem:
    mutant_id: str
    source: str
    declared_target: int
    kind: str


@dataclass(frozen=True)
class SuiteView:
    """One writer's suite for one criterion after the reference filter: the node ids of the tests kept."""

    writer: str
    criterion: int
    call_id: str
    status: str
    kept: tuple[str, ...]
    rejected_static: int
    dropped_reference: int


@dataclass(frozen=True)
class RunRecords:
    run_id: str
    manifest: Mapping[str, Any]
    deviations: str
    traceability_report: Mapping[str, Any]
    bindings: Mapping[str, int]
    reference_runs: Sequence[Mapping[str, str]]
    mutants: Mapping[str, MutantMeta]
    processing: Mapping[str, ProcessingRecord]
    kills: Mapping[str, Mapping[str, str]]
    differential: Mapping[str, str]
    validity: Sequence[ValidityRow]
    witnesses: Sequence[WitnessRow]
    classes: Mapping[str, MutantClassView]
    audit_sample: Mapping[str, Any] | None
    alignment_calls: Sequence[AlignmentCall]
    alignment: Mapping[tuple[str, str, str], AlignmentView]
    probes: Sequence[ProbeRow]
    relabels: Sequence[RelabelRow]
    calls: Mapping[str, CallRecord]
    timing: Mapping[str, Mapping[str, Any]]
    canary_files: tuple[str, ...]
    sources: Mapping[str, tuple[str, ...]] = field(default_factory=dict)
    suites: Mapping[str, Mapping[int, SuiteView]] = field(default_factory=dict)
    writer_kills: Mapping[tuple[str, str], Mapping[str, str]] = field(default_factory=dict)
    exploratory_status: Mapping[str, Mapping[str, str]] = field(default_factory=dict)
    imports: Mapping[str, Any] | None = None
    plan: Mapping[str, Any] | None = None

    @property
    def reference_passes(self) -> dict[str, bool]:
        """Per test: passed in every reference run."""
        nodeids = {n for run in self.reference_runs for n in run}
        return {n: all(run.get(n) == "passed" for run in self.reference_runs) for n in nodeids}

    def is_evaluated(self, mutant_id: str) -> bool:
        record = self.processing.get(mutant_id)
        return record is not None and not record.stillborn and record.status == STATUS_OK


@dataclass(frozen=True)
class OracleRecord:
    """Oracle result of one mutant: the criteria whose oracle suite failed or errored."""

    mutant_id: str
    arm: str
    failed_criteria: tuple[int, ...]
    source: str = "executed"


@dataclass(frozen=True)
class GroundTruth:
    """Everything only the analysis reads: labels, planted items, oracle results, label matrix."""

    labels: Sequence[TestLabel]
    planted: Mapping[str, PlantedItem]
    oracle: Mapping[str, OracleRecord]
    label_matrix_rows: Mapping[str, Mapping[str, str]]
    verify_log: str
    crosscheck_differences: Sequence[str] | None
    d8_criteria: Sequence[int]
    extra: Mapping[str, Any] = field(default_factory=dict)
