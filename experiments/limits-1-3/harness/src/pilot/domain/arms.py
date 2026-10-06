"""Mutant arms: the code stored in `mutants/<id>.json` and the mutant source an arm feeds."""

from __future__ import annotations

from typing import Final

from pilot.domain.model_slate import MutantSource

# Criterion-targeted generation by the host model.
ARM_T: Final = "T"
# Planted controls.
ARM_P: Final = "P"
# Operator mutants of the source tree.
ARM_O: Final = "O"
# Criterion-targeted generation by the contrast model.
ARM_X: Final = "X"
# Criterion-targeted generation by the terra model (imported).
ARM_R: Final = "R"
# Criterion-targeted generation by the luna model.
ARM_L: Final = "L"

ARMS: Final = (ARM_T, ARM_P, ARM_O, ARM_X, ARM_R, ARM_L)
ARM_SOURCE: Final = {
    ARM_T: MutantSource.SONNET,
    ARM_X: MutantSource.OPUS,
    ARM_O: MutantSource.OPERATOR,
    ARM_R: MutantSource.TERRA,
    ARM_L: MutantSource.LUNA,
}
