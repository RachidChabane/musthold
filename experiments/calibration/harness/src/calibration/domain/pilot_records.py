"""The run-02 records that `build-items` reads (§1): bindings, processing, differential and the mutant documents."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from calibration.domain.jsonread import (
    RecordError,
    as_list,
    as_mapping,
    boolean,
    mapping,
    optional_text,
    sequence,
    text,
)

ARM_T: Final = "T"
ARM_P: Final = "P"
MEASURED_ARMS: Final = (ARM_T, ARM_P)
STATUS_OK: Final = "OK"
DIFF_DIFFERS: Final = "DIFFERS"
DIFF_NO_DIFF_FOUND: Final = "NO_DIFF_FOUND"
DIFF_SKIPPED: Final = "SKIPPED"
CRITERION_PREFIX: Final = "PARK-"
CRITERION_SUFFIX: Final = "~1"


def criterion_ref(number: int) -> str:
    return f"{CRITERION_PREFIX}{number}{CRITERION_SUFFIX}"


def criterion_number(ref: str) -> int:
    """The `n` of `PARK-n~1`."""
    body = ref.removeprefix(CRITERION_PREFIX)
    number, _, _ = body.partition("~")
    return int(number)


@dataclass(frozen=True)
class ProcessingRow:
    mutant_id: str
    stillborn: bool
    status: str
    duplicate_of: str | None

    @property
    def evaluated(self) -> bool:
        """Non-stillborn with status OK: a mutant the pilot ran its tests and its judges on."""
        return not self.stillborn and self.status == STATUS_OK


@dataclass(frozen=True)
class MutantDocument:
    """The fields of `mutants/<mutant_id>.json` the item construction uses: the arm and the normalized edits."""

    mutant_id: str
    arm: str
    normalized_edits: tuple[Mapping[str, str], ...]


def parse_jsonl(content: str, where: str) -> list[Mapping[str, object]]:
    rows: list[Mapping[str, object]] = []
    for number, line in enumerate(content.splitlines(), start=1):
        if not line.strip():
            continue
        try:
            rows.append(mapping(json.loads(line), f"{where}:{number}"))
        except json.JSONDecodeError as error:
            raise RecordError(f"{where}:{number} is not JSON: {error}") from error
    return rows


def parse_bindings(content: str) -> dict[str, str]:
    """`{nodeid: ref}`; a list value keeps its first reference, as the pilot reads it."""
    bindings: dict[str, str] = {}
    for nodeid, value in mapping(json.loads(content), "bindings.json").items():
        items = as_list(value)
        reference = items[0] if items else value
        bindings[nodeid] = text(reference, f"bindings.json {nodeid}")
    return bindings


def parse_processing(content: str) -> list[ProcessingRow]:
    return [
        ProcessingRow(
            mutant_id=text(row.get("mutant_id"), "processing.mutant_id"),
            stillborn=boolean(row.get("stillborn"), "processing.stillborn"),
            status=text(row.get("status"), "processing.status"),
            duplicate_of=optional_text(row.get("duplicate_of"), "processing.duplicate_of"),
        )
        for row in parse_jsonl(content, "processing.jsonl")
    ]


def parse_differential(content: str) -> dict[str, Mapping[str, object]]:
    return {
        text(row.get("mutant_id"), "differential.mutant_id"): row for row in parse_jsonl(content, "differential.jsonl")
    }


def parse_mutant_document(content: str) -> MutantDocument:
    body = mapping(json.loads(content), "mutant document")
    edits: list[Mapping[str, str]] = []
    for index, raw in enumerate(sequence(body.get("normalized_edits"), "normalized_edits")):
        edit = as_mapping(raw)
        if edit is None:
            raise RecordError(f"normalized_edits[{index}] is not an object")
        edits.append(
            {
                "file": text(edit.get("file"), "edit.file"),
                "qualname": text(edit.get("qualname"), "edit.qualname"),
                "new_source": text(edit.get("new_source"), "edit.new_source"),
            }
        )
    return MutantDocument(
        mutant_id=text(body.get("mutant_id"), "mutant.mutant_id"),
        arm=text(body.get("arm"), "mutant.arm"),
        normalized_edits=tuple(edits),
    )
