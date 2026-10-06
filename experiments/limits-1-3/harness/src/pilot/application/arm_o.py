"""Arm O: operator mutants of the source tree, no model call (PREREG 6.3)."""

from __future__ import annotations

from collections.abc import Mapping

from pilot.application.mutant_records import NewMutant
from pilot.domain.arms import ARM_O
from pilot.domain.operator_mutants import operator_mutants

PYTHON_SUFFIX = ".py"


def operator_candidates(source_files: Mapping[str, str]) -> list[NewMutant]:
    python_files = {path: text for path, text in source_files.items() if path.endswith(PYTHON_SUFFIX)}
    return [NewMutant(arm=ARM_O, edit_set=mutant.edit_set) for mutant in operator_mutants(python_files)]
