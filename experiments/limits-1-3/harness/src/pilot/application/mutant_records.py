"""Mutant files, processing, kill and differential records (RUN_RECORDS)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from pilot.application.model_caller import Log
from pilot.application.ports import RunStore
from pilot.domain.edits import EditSet
from pilot.domain.ids import mutant_id, normalized_edit_set
from pilot.domain.mutants import Candidate, MutantRecord, MutantStatus
from pilot.domain.pipeline_constants import (
    DIFFERENTIAL_FILENAME,
    KILLS_FILENAME,
    MUTANTS_DIRNAME,
    PROCESSING_FILENAME,
)

STATUS_OK = "OK"
MUTANT_FILE_TEMPLATE = MUTANTS_DIRNAME + "/{mutant_id}.json"


@dataclass(frozen=True)
class NewMutant:
    arm: str
    edit_set: EditSet
    target: str | None = None
    slot: str | None = None
    generator: Mapping[str, Any] | None = None
    mechanism_summary: str | None = None

    @property
    def mutant_id(self) -> str:
        return mutant_id(self.edit_set)


def mutant_path(identifier: str) -> str:
    return MUTANT_FILE_TEMPLATE.format(mutant_id=identifier)


def write_mutants(store: RunStore, mutants: Sequence[NewMutant], log: Log) -> list[Candidate]:
    """Writes `mutants/<id>.json`; the first writer of an id wins, later same-id mutants are skipped."""
    candidates: list[Candidate] = []
    skipped = 0
    for mutant in mutants:
        path = mutant_path(mutant.mutant_id)
        if store.exists(path) and store.read_json(path)["arm"] != mutant.arm:
            skipped += 1
            continue
        if not store.exists(path):
            store.write_json(path, _mutant_document(mutant))
        candidates.append(Candidate(mutant.arm, mutant.mutant_id, mutant.edit_set))
    if skipped:
        log(f"[mutants] {skipped} mutant(s) share the id of a mutant of another arm and are not repeated")
    return _unique(candidates)


def _unique(candidates: Sequence[Candidate]) -> list[Candidate]:
    seen: set[str] = set()
    unique = []
    for candidate in candidates:
        if candidate.mutant_id not in seen:
            seen.add(candidate.mutant_id)
            unique.append(candidate)
    return unique


def _mutant_document(mutant: NewMutant) -> dict[str, Any]:
    return {
        "mutant_id": mutant.mutant_id,
        "arm": mutant.arm,
        "target": mutant.target,
        "slot": mutant.slot,
        "generator": dict(mutant.generator) if mutant.generator is not None else None,
        "edits": mutant.edit_set.to_mapping()["edits"],
        "normalized_edits": normalized_edit_set(mutant.edit_set)["edits"],
        "mechanism_summary": mutant.mechanism_summary,
        "merged_from": [],
    }


def load_mutant(store: RunStore, identifier: str) -> dict[str, Any]:
    return store.read_json(mutant_path(identifier))


def load_edit_set(store: RunStore, identifier: str) -> EditSet:
    return EditSet.from_mapping({"edits": load_mutant(store, identifier)["edits"]})


def record_processing(store: RunStore, records: Sequence[MutantRecord]) -> None:
    """Merges the records of one processing run into the three jsonl files, replacing same-id rows."""
    _merge(store, PROCESSING_FILENAME, [_processing_row(r) for r in records])
    _merge(store, KILLS_FILENAME, [_kill_row(r) for r in records if _is_evaluated(r)])
    _merge(store, DIFFERENTIAL_FILENAME, [_differential_row(r) for r in records if _is_evaluated(r) and r.differential])
    for record in records:
        if record.merged_ids:
            _record_merges(store, record)


def _is_evaluated(record: MutantRecord) -> bool:
    return record.status is MutantStatus.EVALUATED


def _merge(store: RunStore, filename: str, rows: Sequence[Mapping[str, Any]]) -> None:
    existing = store.read_jsonl(filename) if store.exists(filename) else []
    replaced = {row["mutant_id"] for row in rows}
    kept = [row for row in existing if row["mutant_id"] not in replaced]
    store.write_jsonl(filename, [*kept, *rows], ("mutant_id",))


def _processing_row(record: MutantRecord) -> dict[str, Any]:
    stillborn = record.status is MutantStatus.STILLBORN
    return {
        "mutant_id": record.candidate.mutant_id,
        "stillborn": stillborn,
        "stillborn_reason": record.reason if stillborn else None,
        "status": _status_name(record.status),
        "duplicate_of": record.duplicate_of,
        "changed_lines": record.changed_lines,
    }


def _status_name(status: MutantStatus) -> str:
    if status in (MutantStatus.TRIVIALLY_EQUIVALENT, MutantStatus.DUPLICATE):
        return status.name
    return STATUS_OK


def _kill_row(record: MutantRecord) -> dict[str, Any]:
    return {
        "mutant_id": record.candidate.mutant_id,
        "results": {nodeid: result.value for nodeid, result in sorted(record.kill_vector.items())},
    }


def _differential_row(record: MutantRecord) -> dict[str, Any]:
    result = record.differential
    assert result is not None
    return {
        "mutant_id": record.candidate.mutant_id,
        "status": result.kind.value,
        "function": result.function,
        "args": result.args,
        "original_out": result.original_out,
        "mutant_out": result.mutant_out,
        "reason": result.reason,
    }


def _record_merges(store: RunStore, record: MutantRecord) -> None:
    path = mutant_path(record.candidate.mutant_id)
    document = store.read_json(path)
    document["merged_from"] = sorted(set(document["merged_from"]) | set(record.merged_ids))
    store.write_json(path, document)


def evaluated_mutant_ids(store: RunStore, arms: Sequence[str]) -> list[str]:
    """Ids of non-stillborn, status-OK mutants of the given arms, in id order."""
    rows = store.read_jsonl(PROCESSING_FILENAME) if store.exists(PROCESSING_FILENAME) else []
    return sorted(
        row["mutant_id"]
        for row in rows
        if not row["stillborn"]
        and row["status"] == STATUS_OK
        and load_mutant(store, row["mutant_id"])["arm"] in arms
    )
