"""Reads a run directory (harness/RUN_RECORDS.md) into `RunRecords`.

Aggregate files whose container shape RUN_RECORDS leaves open are read in both the
list-of-records and the keyed-mapping form.
"""

from __future__ import annotations

import json
from collections.abc import Iterable, Mapping
from pathlib import Path
from typing import Any

from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.analysis_constants import (
    CALL_STATUSES,
    CLASS_VERDICTS,
    DIFF_STATUSES,
    KILL_RESULTS,
    PROCESSING_STATUSES,
    TEST_OUTCOMES,
)
from pilot.domain.pipeline_constants import (
    ALIGNMENT_AGGREGATE_FILENAME,
    ALIGNMENT_FILENAME,
    AUDIT_SAMPLE_FILENAME,
    BINDINGS_FILENAME,
    DEVIATIONS_FILENAME,
    DIFFERENTIAL_FILENAME,
    KILLS_FILENAME,
    MUTANT_CLASSES_FILENAME,
    MUTANTS_DIRNAME,
    PROBE_FILENAME,
    PROCESSING_FILENAME,
    REFERENCE_FILENAME,
    RELABEL_FILENAME,
    RUN_MANIFEST_FILENAME,
    TIMING_FILENAME,
    VALIDITY_FILENAME,
    WITNESS_FILENAME,
)
from pilot.domain.constants import CANARY_DIRNAME, RAW_DIRNAME, TRACEABILITY_FILENAME
from pilot.domain.refs import parse_criterion, parse_criterion_or_none
from pilot.domain.run_records import (
    AlignmentCall,
    AlignmentView,
    CallRecord,
    MutantClassView,
    MutantMeta,
    ProbeRow,
    ProcessingRecord,
    RelabelRow,
    RunRecords,
    ValidityRow,
    WitnessRow,
)

RESULT_FILENAME = "result.json"
JSON_SUFFIX = ".json"
ENCODING = "utf-8"


class RunRecordsError(Exception):
    """A required run file is missing or malformed."""


def read_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding=ENCODING))
    except FileNotFoundError as error:
        raise RunRecordsError(f"missing run file: {path}") from error
    except json.JSONDecodeError as error:
        raise RunRecordsError(f"malformed JSON in {path}: {error}") from error


def read_json_optional(path: Path, default: Any) -> Any:
    return read_json(path) if path.is_file() else default


def read_jsonl(path: Path, required: bool = False) -> list[dict[str, Any]]:
    if not path.is_file():
        if required:
            raise RunRecordsError(f"missing run file: {path}")
        return []
    return [json.loads(line) for line in path.read_text(encoding=ENCODING).splitlines() if line.strip()]


def _checked(value: Any, allowed: tuple[str, ...], where: str) -> str:
    if value not in allowed:
        raise RunRecordsError(f"{where}: unknown value {value!r}, expected one of {allowed}")
    return str(value)


def read_run(run_dir: Path) -> RunRecords:
    """Reads and validates a run directory; any malformed record is a `RunRecordsError`."""
    if not run_dir.is_dir():
        raise RunRecordsError(f"no run directory: {run_dir}")
    try:
        return _read_run(run_dir)
    except (KeyError, ValueError, TypeError, AttributeError) as error:
        raise RunRecordsError(f"malformed run record in {run_dir}: {error!r}") from error


def _read_run(run_dir: Path) -> RunRecords:
    audit_path = run_dir / AUDIT_SAMPLE_FILENAME
    return RunRecords(
        run_id=run_dir.name,
        manifest=read_json_optional(run_dir / RUN_MANIFEST_FILENAME, {}),
        deviations=_read_text(run_dir / DEVIATIONS_FILENAME),
        traceability_report=read_json(run_dir / TRACEABILITY_FILENAME),
        bindings={n: _binding(v) for n, v in read_json(run_dir / BINDINGS_FILENAME).items()},
        reference_runs=_reference_runs(read_json(run_dir / REFERENCE_FILENAME)),
        mutants=_read_mutants(run_dir / MUTANTS_DIRNAME),
        processing=_read_processing(run_dir / PROCESSING_FILENAME),
        kills={r["mutant_id"]: _kill_results(r) for r in read_jsonl(run_dir / KILLS_FILENAME, True)},
        differential={
            r["mutant_id"]: _checked(r["status"], DIFF_STATUSES, "differential.jsonl")
            for r in read_jsonl(run_dir / DIFFERENTIAL_FILENAME)
        },
        validity=[_validity_row(r) for r in read_jsonl(run_dir / VALIDITY_FILENAME)],
        witnesses=[_witness_row(r) for r in read_jsonl(run_dir / WITNESS_FILENAME)],
        classes=_read_classes(run_dir / MUTANT_CLASSES_FILENAME),
        audit_sample=read_json(audit_path) if audit_path.is_file() else None,
        alignment_calls=[_alignment_call(r) for r in read_jsonl(run_dir / ALIGNMENT_FILENAME)],
        alignment=_read_alignment(run_dir / ALIGNMENT_AGGREGATE_FILENAME),
        probes=[_probe_row(r) for r in read_jsonl(run_dir / PROBE_FILENAME)],
        relabels=[RelabelRow(r["rater"], r["nodeid"], r["label"]) for r in read_jsonl(run_dir / RELABEL_FILENAME)],
        calls=_read_calls(run_dir / RAW_DIRNAME),
        timing=read_json_optional(run_dir / TIMING_FILENAME, {}),
        canary_files=_canary_files(run_dir / CANARY_DIRNAME),
    )


def _read_text(path: Path) -> str:
    return path.read_text(encoding=ENCODING) if path.is_file() else ""


def _reference_runs(document: Mapping[str, Any]) -> list[Mapping[str, str]]:
    for run in document["runs"]:
        for nodeid, outcome in run.items():
            _checked(outcome, TEST_OUTCOMES, f"reference.json {nodeid}")
    return list(document["runs"])


def _kill_results(row: Mapping[str, Any]) -> Mapping[str, str]:
    for nodeid, result in row["results"].items():
        _checked(result, KILL_RESULTS, f"kills.jsonl {row['mutant_id']} {nodeid}")
    return row["results"]


def _binding(value: Any) -> int:
    ref = value[0] if isinstance(value, list) else value
    return parse_criterion(str(ref))


def _records(document: Any, key_name: str) -> list[dict[str, Any]]:
    """A list of records, or a mapping whose keys are the records' `key_name`."""
    if isinstance(document, list):
        return list(document)
    return [{key_name: key, **value} for key, value in document.items()]


def _read_mutants(directory: Path) -> dict[str, MutantMeta]:
    found: dict[str, MutantMeta] = {}
    for path in sorted(directory.glob(f"*{JSON_SUFFIX}")):
        raw = read_json(path)
        generator = raw.get("generator") or {}
        found[raw["mutant_id"]] = MutantMeta(
            mutant_id=raw["mutant_id"],
            arm=raw["arm"],
            target=parse_criterion_or_none(raw.get("target")),
            slot=raw.get("slot"),
            generator_model=generator.get("model"),
            generator_cli=generator.get("cli"),
            generator_call_id=generator.get("call_id"),
        )
    return found


def _read_processing(path: Path) -> dict[str, ProcessingRecord]:
    return {
        r["mutant_id"]: ProcessingRecord(
            r["mutant_id"],
            bool(r["stillborn"]),
            _checked(r["status"], PROCESSING_STATUSES, f"processing.jsonl {r['mutant_id']}"),
            r.get("duplicate_of"),
        )
        for r in read_jsonl(path, True)
    }


def _read_classes(path: Path) -> dict[str, MutantClassView]:
    views = (_class_view(r) for r in _records(read_json(path), "mutant_id"))
    return {view.mutant_id: view for view in views}


def _class_view(raw: Mapping[str, Any]) -> MutantClassView:
    primary = raw.get("primary") or {}
    return MutantClassView(
        mutant_id=raw["mutant_id"],
        arm=raw["arm"],
        target=parse_criterion_or_none(raw.get("target")),
        per_criterion={
            parse_criterion(ref): _checked(v, CLASS_VERDICTS, f"mutant_classes.json {raw['mutant_id']}")
            for ref, v in raw["per_criterion"].items()
        },
        valid_confidence={parse_criterion(ref): float(v) for ref, v in raw.get("valid_confidence", {}).items()},
        equivalent=bool(raw.get("equivalent")),
        unattributed=bool(raw.get("unattributed")),
        off_target=bool(raw.get("off_target")),
        collateral=bool(raw.get("collateral")),
        primary_criterion=parse_criterion_or_none(primary.get("criterion")) if primary.get("included") else None,
        discarded=bool(raw.get("discarded")),
        survived_all=bool(raw.get("survived_all")),
    )


def _read_alignment(path: Path) -> dict[tuple[str, str, str], AlignmentView]:
    if not path.is_file():
        return {}
    document = read_json(path)
    views: dict[tuple[str, str, str], AlignmentView] = {}
    for judge, arm, nodeid, raw in _flatten_alignment(document):
        views[(judge, arm, nodeid)] = AlignmentView(
            modal_ref=raw.get("modal_ref"),
            modal_share=float(raw.get("modal_share", 0.0)),
            modal_coverage=raw.get("modal_coverage"),
            mean_confidence=raw.get("mean_confidence"),
            verdict=AlignmentVerdict(raw["verdict"]),
            verdict_s1=AlignmentVerdict(raw.get("verdict_s1", raw["verdict"])),
            missing_reps=int(raw.get("missing_reps", 0)),
        )
    return views


def _flatten_alignment(document: Any) -> Iterable[tuple[str, str, str, Mapping[str, Any]]]:
    if isinstance(document, list):
        for raw in document:
            yield raw["judge"], raw["arm"], raw["nodeid"], raw
        return
    for judge, arms in document.items():
        for arm, tests in arms.items():
            for nodeid, raw in tests.items():
                yield judge, arm, nodeid, raw


def _validity_row(raw: Mapping[str, Any]) -> ValidityRow:
    status = _checked(raw["status"], CALL_STATUSES, "validity.jsonl")
    return ValidityRow(raw["mutant_id"], raw["judge"], raw.get("call_id", ""), status, raw.get("payload"))


def _witness_row(raw: Mapping[str, Any]) -> WitnessRow:
    return WitnessRow(raw["mutant_id"], raw["judge"], parse_criterion(raw["criterion"]), bool(raw["confirmed"]))


def _alignment_call(raw: Mapping[str, Any]) -> AlignmentCall:
    status = _checked(raw["status"], CALL_STATUSES, "alignment.jsonl")
    return AlignmentCall(
        raw["judge"], raw["arm"], raw["nodeid"], int(raw["rep"]), raw.get("call_id", ""), status, raw.get("verifies_ref")
    )


def _probe_row(raw: Mapping[str, Any]) -> ProbeRow:
    status = _checked(raw["status"], CALL_STATUSES, "probe.jsonl")
    return ProbeRow(raw["judge"], raw["nodeid"], int(raw["rep"]), status, raw.get("guess"), raw.get("confidence"))


def _read_calls(raw_dir: Path) -> dict[str, CallRecord]:
    """Top-level `result.json` of each call directory only; attempt subdirectories would double-count."""
    calls: dict[str, CallRecord] = {}
    for path in sorted(raw_dir.glob(f"*/{RESULT_FILENAME}")):
        raw = read_json(path)
        calls[path.parent.name] = CallRecord(
            call_id=path.parent.name,
            status=raw.get("status", ""),
            cost_usd=raw.get("total_cost_usd"),
            input_tokens=int(raw.get("input_tokens", 0)),
            output_tokens=int(raw.get("output_tokens", 0)),
            isolation_violations=len(raw.get("isolation_violations", [])),
        )
    return calls


def _canary_files(directory: Path) -> tuple[str, ...]:
    return tuple(sorted(p.name for p in directory.glob(f"*{JSON_SUFFIX}"))) if directory.is_dir() else ()
