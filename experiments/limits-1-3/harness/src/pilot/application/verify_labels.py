"""Use case `verify-labels`: execute the reference, violations, planted items and R (PREREG 3.2)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.application.ports import RunRequest, VariantRunner
from pilot.domain.constants import REFERENCE_VARIANT_ID
from pilot.domain.edits import EditSet
from pilot.domain.label_matrix import LabelMatrix
from pilot.domain.label_rules import ExpectedCounts, RuleFailure, evaluate
from pilot.domain.labels import TestLabel
from pilot.domain.outcome_matrix import VariantOutcomes


@dataclass(frozen=True)
class LabelVerification:
    matrix: LabelMatrix
    failures: list[RuleFailure]
    runs: Mapping[str, VariantOutcomes]

    @property
    def passed(self) -> bool:
        return not self.failures


def run_variant_outcomes(
    runner: VariantRunner, variants: Mapping[str, EditSet]
) -> dict[str, VariantOutcomes]:
    requests = {REFERENCE_VARIANT_ID: RunRequest(), **{k: RunRequest(v) for k, v in variants.items()}}
    results = runner.run_many(requests)
    return {name: VariantOutcomes(result.outcomes) for name, result in results.items()}


def verify_labels(
    runner: VariantRunner,
    labels: Sequence[TestLabel],
    variants: Mapping[str, EditSet],
    expected: ExpectedCounts,
) -> LabelVerification:
    runs = run_variant_outcomes(runner, variants)
    matrix = LabelMatrix.from_runs([label.nodeid for label in labels], runs)
    return LabelVerification(matrix, evaluate(labels, runs, expected), runs)
