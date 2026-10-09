"""Reading what the site is built from: a sealed run directory, or the synthetic fixtures of the repository.

Both read the same layout: `results.json`, `manifest.json`, `DEVIATIONS.md`, `trunks/` and `cases/`. A case's
evidence paths are relative to an evidence root: the run directory for a sealed run, the repository root for the
fixtures, whose cases point at the product's own fixture files. Every evidence file is read and digested here;
a path that leaves the evidence root is refused, symbolic links included.
"""

from __future__ import annotations

import hashlib
import json
from collections.abc import Mapping
from pathlib import Path
from typing import Final, cast

from specbench.adapters.claude_stream import parse_claude_stream
from specbench.adapters.codex_stream import parse_codex_exec
from specbench.adapters.run_store import RunStore
from specbench.adapters.schemas import SchemaName, schema_problems
from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.cases import CaseRecord, MutantPointer, case_from_record
from specbench.domain.jsonread import RecordError, mapping, sequence, text
from specbench.domain.manifest import RESULTS_FILE_NAME, RunLayout
from specbench.domain.records import TrunkRecord
from specbench.site.model import BuildMode, CaseMaterial, EvidenceFile, MutantMaterial, SiteInputs, Witness

ENCODING: Final = "utf-8"
CASE_SUFFIX: Final = "*.json"
FIXTURE_RUN: Final = Path("bench/tests/fixtures/site_run")
"""The synthetic run the fixture site is built from, relative to the repository root."""
FIXTURE_RESULTS: Final = Path("bench/tests/golden/results.json")
"""The analysis of the golden synthetic run, relative to the repository root."""


class EvidenceOutsideRoot(RecordError):
    """A case points at a file outside its evidence root."""


def _label(case: CaseRecord, path: str) -> str:
    named = {case.claim_events: "Agent session events", case.gate_report: "Gate report at the stop"}
    if case.mutant is not None:
        named |= {
            case.mutant.before: "Source at the stop",
            case.mutant.after: "Source with the mutant",
            case.mutant.answer: "Host answer that proposed the mutant",
        }
        if case.mutant.witness is not None:
            named[case.mutant.witness] = "Host answer with the witness"
    if case.repair is not None:
        named[case.repair.report] = "Gate report after the repair"
        if case.repair.diff is not None:
            named[case.repair.diff] = "Repair diff"
    named |= {item.path: item.label for item in case.evidence}
    return named.get(path, path)


class EvidenceReader:
    """Reads the files a case names, under one root, and remembers each one's digest."""

    def __init__(self, root: Path) -> None:
        self._root = root.resolve()

    def path(self, relative: str) -> Path:
        candidate = (self._root / relative).resolve()
        if not candidate.is_relative_to(self._root):
            raise EvidenceOutsideRoot(f"evidence {relative} lies outside {self._root}")
        return candidate

    def bytes(self, relative: str) -> bytes:
        return self.path(relative).read_bytes()

    def text(self, relative: str) -> str:
        return self.bytes(relative).decode(ENCODING)

    def json(self, relative: str) -> Mapping[str, object]:
        return mapping(json.loads(self.text(relative)), relative)

    def evidence(self, case: CaseRecord) -> tuple[EvidenceFile, ...]:
        files: list[EvidenceFile] = []
        for relative in case.paths:
            content = self.bytes(relative)
            files.append(
                EvidenceFile(_label(case, relative), relative, hashlib.sha256(content).hexdigest(), len(content))
            )
        return tuple(files)


def _claim(reader: EvidenceReader, case: CaseRecord) -> str | None:
    if case.claim_events is None:
        return None
    stream = reader.text(case.claim_events)
    if case.harness is Harness.CLAUDE_CODE:
        result = parse_claude_stream(stream).result
        return result.final_message if result is not None and result.final_message else None
    message = parse_codex_exec(stream).final_message
    return message or None


def _witness(reader: EvidenceReader, relative: str | None) -> Witness | None:
    if relative is None:
        return None
    body = mapping(reader.json(relative).get("witness"), f"{relative} witness")
    arguments = mapping(body.get("args"), f"{relative} witness args")
    expected = mapping(body.get("expected"), f"{relative} witness expected")
    return Witness(
        operation=text(body.get("operation"), f"{relative} witness operation"),
        arguments=tuple(
            (name, text(mapping(value, name).get("value"), f"{relative} argument {name}"))
            for name, value in arguments.items()
        ),
        expected_kind=text(expected.get("kind"), f"{relative} expected kind"),
        expected=text(expected.get("value"), f"{relative} expected value"),
    )


def _mutant(reader: EvidenceReader, pointer: MutantPointer | None) -> MutantMaterial | None:
    if pointer is None:
        return None
    mutants = sequence(reader.json(pointer.answer).get("mutants"), f"{pointer.answer} mutants")
    if pointer.index >= len(mutants):
        raise RecordError(f"{pointer.answer} holds no mutant {pointer.index}")
    proposed = mapping(mutants[pointer.index], f"{pointer.answer} mutant {pointer.index}")
    edited = {text(mapping(e, "edit").get("file"), "edit file") for e in sequence(proposed.get("edits"), "edits")}
    if pointer.file not in edited:
        raise RecordError(f"mutant {pointer.index} of {pointer.answer} does not edit {pointer.file}")
    return MutantMaterial(
        file=pointer.file,
        before=reader.text(pointer.before),
        after=reader.text(pointer.after),
        mechanism=text(proposed.get("mechanism"), f"{pointer.answer} mechanism"),
        witness=_witness(reader, pointer.witness),
        results=pointer.results,
    )


def read_case(
    record: JsonValue, reader: EvidenceReader, trunks: Mapping[tuple[str, Harness], TrunkRecord]
) -> CaseMaterial:
    problems = schema_problems(SchemaName.CASE, record)
    if problems:
        raise RecordError(f"case record: {'; '.join(problems)}")
    case = case_from_record(record)
    repair = case.repair
    return CaseMaterial(
        record=case,
        claim=_claim(reader, case),
        gate_report=reader.json(case.gate_report),
        mutant=_mutant(reader, case.mutant),
        repair_report=reader.json(repair.report) if repair is not None else None,
        repair_diff=reader.text(repair.diff) if repair is not None and repair.diff is not None else None,
        trunk=trunks.get((case.task_id, case.harness)),
        evidence=reader.evidence(case),
    )


def read_site_inputs(
    mode: BuildMode, run_dir: Path, results_path: Path, evidence_root: Path, source_commit: str, generator: str
) -> SiteInputs:
    layout = RunLayout(run_dir)
    store = RunStore(layout)
    trunks = {(t.task_id, t.harness): t for t in store.read_trunks()} if layout.trunks.is_dir() else {}
    reader = EvidenceReader(evidence_root)
    case_files = sorted(layout.cases.glob(CASE_SUFFIX)) if layout.cases.is_dir() else []
    cases = tuple(
        read_case(cast(JsonValue, json.loads(path.read_text(encoding=ENCODING))), reader, trunks) for path in case_files
    )
    return SiteInputs(
        mode=mode,
        results=json.loads(results_path.read_text(encoding=ENCODING)),
        manifest=store.read_manifest(),
        deviations=tuple(store.read_deviations()),
        cases=cases,
        source_commit=source_commit,
        generator=generator,
    )


def read_run_inputs(run_dir: Path, source_commit: str, generator: str) -> SiteInputs:
    """A sealed run: its own results, and evidence paths relative to the run directory."""
    return read_site_inputs(
        BuildMode.SEALED_RUN, run_dir, run_dir / RESULTS_FILE_NAME, run_dir, source_commit, generator
    )


def read_fixture_inputs(repository_root: Path, source_commit: str, generator: str) -> SiteInputs:
    """The synthetic fixtures: the golden synthetic analysis and the fixture run, badged as synthetic."""
    return read_site_inputs(
        BuildMode.SYNTHETIC,
        repository_root / FIXTURE_RUN,
        repository_root / FIXTURE_RESULTS,
        repository_root,
        source_commit,
        generator,
    )


def is_fixture_run(run_dir: Path, repository_root: Path) -> bool:
    """Whether a run directory is, or carries the outputs of, the synthetic fixtures: such a run is never a sealed run."""
    pairs = (
        (run_dir / "manifest.json", repository_root / FIXTURE_RUN / "manifest.json"),
        (run_dir / RESULTS_FILE_NAME, repository_root / FIXTURE_RESULTS),
    )
    return any(
        own.is_file() and fixture.is_file() and own.read_bytes() == fixture.read_bytes() for own, fixture in pairs
    )
