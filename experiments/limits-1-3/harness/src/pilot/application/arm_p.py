"""Arm P: planted controls read from `prereg/arm_p/items/` only (PREREG 6.2)."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Protocol

from pilot.application.mutant_records import NewMutant
from pilot.domain.edits import EditSet
from pilot.domain.ids import mutant_id

ARM_P = "P"


class ArmPItems(Protocol):
    def items(self) -> Mapping[str, EditSet]: ...


class ArmPIntegrityError(Exception):
    pass


def planted_mutants(source: ArmPItems) -> list[NewMutant]:
    """One mutant per item; an item whose file name is not its mutant id is rejected."""
    mutants = []
    for name, edit_set in sorted(source.items().items()):
        if mutant_id(edit_set) != name:
            raise ArmPIntegrityError(f"item {name} does not hash to its own mutant id")
        mutants.append(NewMutant(arm=ARM_P, edit_set=edit_set))
    return mutants
