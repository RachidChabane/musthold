"""Operator mutants of the merge diff, from the product's engine used as a black box."""

from __future__ import annotations

from typing import Final

from specbench.application.ports import OperatorMutants

PRODUCT_BINARY: Final = "musthold-dp"
UNAVAILABLE_REASON: Final = (
    f"{PRODUCT_BINARY} exposes no command that emits operator mutants of a diff: its engine runs only inside "
    "`prove`; the survivor listing is not evaluated"
)


class UnavailableOperatorEngine:
    """`OperatorMutantEngine` for a product whose binary offers no mutant-generation verb: it records why."""

    @property
    def limitation(self) -> str | None:
        return UNAVAILABLE_REASON

    def mutants_of(self, base_commit: str, merge_commit: str) -> OperatorMutants | None:
        return None
