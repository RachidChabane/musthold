"""`plan.json` of the decisional run: every model call, its id and its nonce, fixed before any call is made.

Sections: `arm_t` (host generation), `contrast_x` (contrast-model generation, imported entries
keep their original nonces), `terra_x` (imported only), `writers` (test writers S and O) and
`luna` (the exploratory stratum). No call id of a section equals an imported or retired id,
and no nonce appears twice.
"""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from typing import Any, Final

from pilot.domain.call_ids import GENERATE_CALL_ID, LUNA_GENERATE_CALL_ID, WRITE_TESTS_CALL_ID
from pilot.domain.import_manifest import TERRA_GENERATION_IDS
from pilot.domain.model_call import ModelPin
from pilot.domain.model_slate import (
    ARM_T_CALLS_PER_CRITERION,
    ARM_T_FIRST_CALL_INDEX,
    CONTRAST,
    CONTRAST_CALLS_PER_CRITERION,
    HOST,
    LUNA,
    PRIMARY_WRITERS,
    TERRA,
    WRITER_PIN,
    Writer,
)

SECTION_ARM_T: Final = "arm_t"
SECTION_CONTRAST: Final = "contrast_x"
SECTION_TERRA: Final = "terra_x"
SECTION_WRITERS: Final = "writers"
SECTION_LUNA: Final = "luna"
SECTIONS: Final = (SECTION_ARM_T, SECTION_CONTRAST, SECTION_TERRA, SECTION_WRITERS, SECTION_LUNA)
KIND_WRITE_TESTS: Final = "write_tests"
KIND_GENERATE: Final = "generate"
NonceFactory = Callable[[], str]


@dataclass(frozen=True)
class PlannedCall:
    call_id: str
    criterion: str
    model: str
    cli: str
    nonce: str
    imported: bool = False
    writer: str | None = None
    kind: str | None = None

    def to_mapping(self) -> dict[str, Any]:
        return {
            "call_id": self.call_id,
            "criterion": self.criterion,
            "model": self.model,
            "cli": self.cli,
            "nonce": self.nonce,
            "imported": self.imported,
            "writer": self.writer,
            "kind": self.kind,
        }


@dataclass(frozen=True)
class RunPlan:
    arm_t: tuple[PlannedCall, ...]
    contrast_x: tuple[PlannedCall, ...]
    terra_x: tuple[PlannedCall, ...]
    writers: tuple[PlannedCall, ...]
    luna: tuple[PlannedCall, ...]

    def to_mapping(self) -> dict[str, list[dict[str, Any]]]:
        return {
            section: [call.to_mapping() for call in getattr(self, section)] for section in SECTIONS
        }

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> RunPlan:
        return cls(**{section: tuple(PlannedCall(**entry) for entry in raw[section]) for section in SECTIONS})

    def all_calls(self) -> tuple[PlannedCall, ...]:
        return tuple(call for section in SECTIONS for call in getattr(self, section))


def build_plan(
    criteria: Sequence[str],
    imported_nonces: Mapping[str, str],
    nonce_factory: NonceFactory,
) -> RunPlan:
    """The plan of a run; `imported_nonces` maps each imported generation call id to its original nonce."""
    return RunPlan(
        arm_t=tuple(_arm_t(ref, index, nonce_factory) for ref in criteria for index in _arm_t_indices()),
        contrast_x=tuple(_contrast(ref, index, imported_nonces, nonce_factory) for ref in criteria for index in _contrast_indices()),
        terra_x=_terra(imported_nonces),
        writers=tuple(_writer_call(ref, writer, nonce_factory) for ref in criteria for writer in PRIMARY_WRITERS),
        luna=tuple(call for ref in criteria for call in _luna_calls(ref, nonce_factory)),
    )


def _arm_t_indices() -> range:
    return range(ARM_T_FIRST_CALL_INDEX, ARM_T_FIRST_CALL_INDEX + ARM_T_CALLS_PER_CRITERION)


def _contrast_indices() -> range:
    return range(1, CONTRAST_CALLS_PER_CRITERION + 1)


def _arm_t(ref: str, index: int, nonce_factory: NonceFactory) -> PlannedCall:
    return _generation(GENERATE_CALL_ID.format(ref=ref, index=index), ref, HOST, nonce_factory())


def _contrast(ref: str, index: int, imported: Mapping[str, str], nonce_factory: NonceFactory) -> PlannedCall:
    call_id = GENERATE_CALL_ID.format(ref=ref, index=index)
    if call_id in imported:
        return _generation(call_id, ref, CONTRAST, imported[call_id], imported=True)
    return _generation(call_id, ref, CONTRAST, nonce_factory())


def _terra(imported: Mapping[str, str]) -> tuple[PlannedCall, ...]:
    return tuple(
        _generation(call_id, _criterion_of(call_id), TERRA, imported[call_id], imported=True)
        for call_id in TERRA_GENERATION_IDS
        if call_id in imported
    )


def _criterion_of(call_id: str) -> str:
    return call_id.split("__")[1]


def _generation(call_id: str, ref: str, pin: ModelPin, nonce: str, imported: bool = False) -> PlannedCall:
    return PlannedCall(call_id, ref, pin.model, pin.cli.value, nonce, imported=imported, kind=KIND_GENERATE)


def _writer_call(ref: str, writer: Writer, nonce_factory: NonceFactory) -> PlannedCall:
    pin = WRITER_PIN[writer]
    call_id = WRITE_TESTS_CALL_ID.format(ref=ref, writer=writer.value)
    return PlannedCall(call_id, ref, pin.model, pin.cli.value, nonce_factory(), writer=writer.value, kind=KIND_WRITE_TESTS)


def _luna_calls(ref: str, nonce_factory: NonceFactory) -> tuple[PlannedCall, PlannedCall]:
    writer = Writer.LUNA.value
    tests = _writer_call(ref, Writer.LUNA, nonce_factory)
    generation = PlannedCall(
        LUNA_GENERATE_CALL_ID.format(ref=ref, writer=writer), ref, LUNA.model, LUNA.cli.value, nonce_factory(),
        writer=writer, kind=KIND_GENERATE,
    )
    return tests, generation
