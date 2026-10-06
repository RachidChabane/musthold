"""Pre-registration tool: arm-P items under neutral ids, with their labels kept out of the pipeline (PREREG 6.2)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from pilot.domain.edits import EditSet
from pilot.domain.ids import mutant_id
from pilot.domain.variants import VariantId, VariantKind

REALISTIC_SOURCE_SUFFIXES = ("a", "b")
KIND_VIOLATION = "r"
KIND_PLANTED = "n1"
TARGET_TEMPLATE = "PARK-{number}~1"


@dataclass(frozen=True)
class ArmPItem:
    mutant_id: str
    source: str
    declared_target: str
    kind: str
    edit_set: EditSet


class DuplicateArmPItem(Exception):
    pass


def assemble_items(violations: Mapping[str, EditSet], planted: Mapping[str, EditSet]) -> list[ArmPItem]:
    """The `a` and `b` violations (kind r) and every n1 item, keyed by the hash of their edit set."""
    items = [
        *(_item(name, edit_set, KIND_VIOLATION) for name, edit_set in violations.items() if _is_selected(name)),
        *(_item(name, edit_set, KIND_PLANTED) for name, edit_set in planted.items()),
    ]
    ids = [item.mutant_id for item in items]
    if len(set(ids)) != len(ids):
        raise DuplicateArmPItem("two arm-P items share one edit set")
    return sorted(items, key=lambda item: item.mutant_id)


def _is_selected(name: str) -> bool:
    variant = VariantId.parse(name)
    return variant.kind is VariantKind.VIOLATION and name.rsplit("_", 1)[1] in REALISTIC_SOURCE_SUFFIXES


def _item(name: str, edit_set: EditSet, kind: str) -> ArmPItem:
    variant = VariantId.parse(name)
    return ArmPItem(
        mutant_id=mutant_id(edit_set),
        source=name,
        declared_target=TARGET_TEMPLATE.format(number=variant.criterion),
        kind=kind,
        edit_set=edit_set,
    )
