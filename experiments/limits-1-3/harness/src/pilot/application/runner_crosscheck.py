"""Use case `runner-crosscheck`: the harness matrix runner must reproduce the label matrix."""

from __future__ import annotations

from collections.abc import Mapping

from pilot.application.ports import VariantRunner
from pilot.application.verify_labels import run_variant_outcomes
from pilot.domain.constants import REFERENCE_VARIANT_ID
from pilot.domain.edits import EditSet
from pilot.domain.label_matrix import LabelMatrix


def runner_crosscheck(
    runner: VariantRunner,
    committed: LabelMatrix,
    variants: Mapping[str, EditSet],
) -> list[str]:
    """Differences between the committed matrix and the runner's; empty means an exact match."""
    runs = run_variant_outcomes(runner, variants)
    measured = LabelMatrix.from_runs(runs[REFERENCE_VARIANT_ID].project_nodeids(), runs)
    return committed.differences(measured)
