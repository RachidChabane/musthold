"""Use case: deterministic, label-blind mutant processing F0 to F3 (PREREG 6.4)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path

from pilot.application.ports import (
    DifferentialTester,
    EditPreview,
    ImportSmoke,
    RunRequest,
    VariantRunner,
)
from pilot.domain.constants import (
    MAX_CHANGED_LINES,
    PYTEST_EXIT_INTERRUPTED,
    REFERENCE_RUN_COUNT,
)
from pilot.domain.edits import EditError, EditFailure
from pilot.domain.mutants import (
    Candidate,
    MutantRecord,
    MutantStatus,
    StillbornReason,
)
from pilot.domain.normalization import changed_line_count, normalize, normalized_digest
from pilot.domain.outcomes import RunResult, TestOutcome, classify_kill


class ReferenceUnstable(Exception):
    """The reference did not pass the full suite on every confirmation run."""


class MutantProcessor:
    def __init__(
        self,
        runner: VariantRunner,
        previewer: EditPreview,
        project_root: Path,
        import_smoke: ImportSmoke,
        differential: DifferentialTester,
    ) -> None:
        self._runner = runner
        self._previewer = previewer
        self._project_root = project_root
        self._import_smoke = import_smoke
        self._differential = differential

    def confirm_reference(self) -> list[str]:
        """F2 precondition: the reference passes the full suite on every run; returns node ids."""
        requests = {f"reference-{n}": RunRequest() for n in range(1, REFERENCE_RUN_COUNT + 1)}
        results = list(self._runner.run_many(requests).values())
        if not all(result.all_passed for result in results):
            raise ReferenceUnstable(f"reference did not pass {REFERENCE_RUN_COUNT}/{REFERENCE_RUN_COUNT}")
        return sorted(results[0].outcomes)

    def process(self, candidates: Sequence[Candidate]) -> list[MutantRecord]:
        nodeids = self.confirm_reference()
        records = self._static_stage(candidates)
        live = [r for r in records if r.status is MutantStatus.EVALUATED]
        self._import_stage(live)
        live = [r for r in live if r.status is MutantStatus.EVALUATED]
        results = self._runner.run_many({r.candidate.key: RunRequest(r.candidate.edit_set) for r in live})
        self._kill_stage(live, results, nodeids)
        for record in (r for r in live if r.status is MutantStatus.EVALUATED):
            record.differential = self._differential.compare(record.candidate.edit_set)
        return records

    def _static_stage(self, candidates: Sequence[Candidate]) -> list[MutantRecord]:
        records: list[MutantRecord] = []
        first_by_digest: dict[tuple[str, str], MutantRecord] = {}
        for candidate in candidates:
            record = self._static_record(candidate)
            if record.status is MutantStatus.EVALUATED:
                self._deduplicate(record, first_by_digest)
            records.append(record)
        return records

    def _static_record(self, candidate: Candidate) -> MutantRecord:
        try:
            previews = self._previewer.preview(self._project_root, candidate.edit_set)
        except EditError as error:
            return MutantRecord(candidate, MutantStatus.STILLBORN, reason=error.failure.value)
        changed = sum(changed_line_count(old, new) for old, new in previews.values())
        if changed > MAX_CHANGED_LINES:
            reason = EditFailure.TOO_MANY_CHANGED_LINES.value
            return MutantRecord(candidate, MutantStatus.STILLBORN, reason=reason)
        if all(normalize(old) == normalize(new) for old, new in previews.values()):
            return MutantRecord(candidate, MutantStatus.TRIVIALLY_EQUIVALENT)
        digest = normalized_digest({file: new for file, (_, new) in previews.items()})
        return MutantRecord(candidate, MutantStatus.EVALUATED, digest=digest)

    @staticmethod
    def _deduplicate(
        record: MutantRecord, first_by_digest: dict[tuple[str, str], MutantRecord]
    ) -> None:
        key = (record.candidate.arm, record.digest or "")
        first = first_by_digest.get(key)
        if first is None:
            first_by_digest[key] = record
            return
        record.status = MutantStatus.DUPLICATE
        record.duplicate_of = first.candidate.mutant_id
        first.merged_ids.append(record.candidate.mutant_id)

    def _import_stage(self, live: Sequence[MutantRecord]) -> None:
        for record in live:
            if not self._import_smoke.passes(record.candidate.edit_set):
                record.status = MutantStatus.STILLBORN
                record.reason = StillbornReason.IMPORT_SMOKE_FAILED.value

    def _kill_stage(
        self,
        live: Sequence[MutantRecord],
        results: Mapping[str, RunResult],
        nodeids: Sequence[str],
    ) -> None:
        pending = []
        for record in live:
            result = results[record.candidate.key]
            if result.returncode == PYTEST_EXIT_INTERRUPTED:
                record.status = MutantStatus.STILLBORN
                record.reason = StillbornReason.COLLECTION_ERROR.value
                continue
            pending.append((record, result))
        reruns = self._rerun_kills(pending)
        for record, result in pending:
            record.kill_vector = {
                nodeid: classify_kill(result.outcome_of(nodeid), reruns[record.candidate.key].get(nodeid))
                for nodeid in nodeids
            }

    def _rerun_kills(
        self, pending: Sequence[tuple[MutantRecord, RunResult]]
    ) -> dict[str, dict[str, TestOutcome]]:
        killed = {
            record.candidate.key: tuple(
                n for n, o in result.outcomes.items() if o is TestOutcome.FAILED
            )
            for record, result in pending
        }
        requests = {
            record.candidate.key: RunRequest(record.candidate.edit_set, killed[record.candidate.key])
            for record, _ in pending
            if killed[record.candidate.key]
        }
        reruns = self._runner.run_many(requests)
        return {
            key: dict(reruns[key].outcomes) if key in reruns else {} for key in killed
        }
