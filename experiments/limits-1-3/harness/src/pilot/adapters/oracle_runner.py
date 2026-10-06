"""Runs the per-criterion oracle suites on mutant copies (PREREG 6.6) and caches the results.

One plain pytest process per mutant runs every oracle file. A test that times out counts
as failed (pytest-timeout reports it as a failure), as does a collection error of the file.
A file with no reported outcome at all is an infrastructure failure: the mutant is left
unclassified and reported, never cached.
"""

from __future__ import annotations

import hashlib
import json
import re
import threading
from collections.abc import Mapping, Sequence
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.plain_pytest import PlainPytestRunner
from pilot.adapters.process import local_process_runner
from pilot.adapters.project_copy import ProjectWorkspace, temporary_work_root
from pilot.domain.analysis_constants import (
    ORACLE_CLASSES_FILENAME,
    ORACLE_PARALLELISM,
    ORACLE_TESTS_DIRNAME,
    PROCESSING_STATUSES,
    STATUS_DUPLICATE,
    STATUS_OK,
    STATUS_TRIVIALLY_EQUIVALENT,
)
from pilot.domain.constants import SOURCE_DIRNAME
from pilot.domain.ground_truth_paths import ORACLE_FILE_PATTERN
from pilot.domain.edits import EditSet
from pilot.domain.outcomes import TestOutcome
from pilot.domain.pipeline_constants import MUTANTS_DIRNAME, PROCESSING_FILENAME

ENCODING = "utf-8"
SOURCE_EXECUTED = "executed"
SOURCE_TRIVIAL = "trivially-equivalent"
SOURCE_INHERITED = "inherited:"
_ORACLE_FILE = re.compile(ORACLE_FILE_PATTERN)
_PATH_PART = re.compile(r"^([^:]+)")


class OracleRunError(Exception):
    """A mutant's oracle run produced no usable outcome for some oracle file."""


@dataclass(frozen=True)
class OracleTask:
    mutant_id: str
    arm: str
    status: str
    duplicate_of: str | None
    edit_set: EditSet


def oracle_digest(oracle_files: Sequence[Path], source_root: Path) -> str:
    """Digest of the oracle suites and the reference source they run against."""
    digest = hashlib.sha256()
    for path in sorted(oracle_files):
        digest.update(path.name.encode())
        digest.update(path.read_bytes())
    for path in sorted(source_root.rglob("*.py")):
        digest.update(str(path.relative_to(source_root)).encode())
        digest.update(path.read_bytes())
    return digest.hexdigest()


def read_oracle_tasks(run_dir: Path) -> list[OracleTask]:
    tasks: list[OracleTask] = []
    for line in (run_dir / PROCESSING_FILENAME).read_text(encoding=ENCODING).splitlines():
        if not line.strip():
            continue
        record = json.loads(line)
        if record["status"] not in PROCESSING_STATUSES:
            raise OracleRunError(f"processing.jsonl {record['mutant_id']}: unknown status {record['status']!r}")
        if record["stillborn"]:
            continue
        raw = json.loads((run_dir / MUTANTS_DIRNAME / f"{record['mutant_id']}.json").read_text(encoding=ENCODING))
        tasks.append(
            OracleTask(
                record["mutant_id"], raw["arm"], record["status"], record.get("duplicate_of"),
                EditSet.from_mapping({"edits": raw["edits"]}),
            )
        )
    return sorted(tasks, key=lambda t: t.mutant_id)


def failed_criteria(outcomes: Mapping[str, TestOutcome], expected: Sequence[int]) -> dict[int, bool]:
    """Per oracle criterion: True when any of its tests did not pass; raises when a file reported nothing."""
    seen: dict[int, list[TestOutcome]] = {c: [] for c in expected}
    for nodeid, outcome in outcomes.items():
        file_part = _PATH_PART.match(nodeid)
        match = _ORACLE_FILE.search(file_part.group(1)) if file_part else None
        if match and int(match.group(1)) in seen:
            seen[int(match.group(1))].append(outcome)
    silent = sorted(c for c, found in seen.items() if not found)
    if silent:
        raise OracleRunError(f"no outcome for oracle criteria {silent}")
    return {c: any(not o.is_pass for o in found) for c, found in seen.items()}


def oracle_criteria(oracle_files: Sequence[Path]) -> list[int]:
    found = [_ORACLE_FILE.search(p.name) for p in oracle_files]
    return sorted(int(m.group(1)) for m in found if m)


class OracleClassifier:
    """Runs and caches oracle results for the mutants of a run."""

    def __init__(self, project: Path, oracle_files: Sequence[Path], run_dir: Path) -> None:
        self._project = project
        self._oracle_files = list(oracle_files)
        self._run_dir = run_dir
        self._cache_path = run_dir / ORACLE_CLASSES_FILENAME
        self._lock = threading.Lock()

    def classify(self, workers: int = ORACLE_PARALLELISM) -> tuple[int, int, list[str]]:
        """Returns (executed, reused, unresolved mutant ids)."""
        digest = oracle_digest(self._oracle_files, self._project / SOURCE_DIRNAME)
        cache = self._load(digest)
        tasks = read_oracle_tasks(self._run_dir)
        pending = [t for t in tasks if t.status == STATUS_OK and t.mutant_id not in cache["mutants"]]
        reused = sum(1 for t in tasks if t.mutant_id in cache["mutants"])
        for task in (t for t in tasks if t.status == STATUS_TRIVIALLY_EQUIVALENT):
            cache["mutants"].setdefault(task.mutant_id, self._entry(task, [], SOURCE_TRIVIAL))
        unresolved: list[str] = []
        with temporary_work_root() as work_root:
            workspace = ProjectWorkspace(self._project, work_root, AstEditApplier())
            runner = PlainPytestRunner(local_process_runner(), self._oracle_files)
            with ThreadPoolExecutor(max_workers=min(workers, ORACLE_PARALLELISM)) as pool:
                futures = {t.mutant_id: pool.submit(self._run_one, workspace, runner, t) for t in pending}
                for task in pending:
                    try:
                        cache["mutants"][task.mutant_id] = futures[task.mutant_id].result()
                    except OracleRunError:
                        unresolved.append(task.mutant_id)
                    self._save(cache)
        for task in (t for t in tasks if t.status == STATUS_DUPLICATE and t.duplicate_of):
            original = cache["mutants"].get(task.duplicate_of or "")
            if original is not None:
                source = f"{SOURCE_INHERITED}{task.duplicate_of}"
                cache["mutants"][task.mutant_id] = self._entry(task, original["failed_criteria"], source)
        self._save(cache)
        return len(pending) - len(unresolved), reused, sorted(unresolved)

    def _run_one(self, workspace: ProjectWorkspace, runner: PlainPytestRunner, task: OracleTask) -> dict[str, object]:
        nodeids = [f"{ORACLE_TESTS_DIRNAME}/{p.name}" for p in self._oracle_files]
        with workspace.materialized(task.edit_set) as copy:
            result = runner(copy, nodeids)
        if result.deadline_exceeded:
            raise OracleRunError(f"{task.mutant_id}: oracle run exceeded its deadline")
        verdicts = failed_criteria(result.outcomes, oracle_criteria(self._oracle_files))
        failed = sorted(c for c, bad in verdicts.items() if bad)
        return self._entry(task, failed, SOURCE_EXECUTED, {str(c): bad for c, bad in sorted(verdicts.items())})

    @staticmethod
    def _entry(
        task: OracleTask, failed: Sequence[int], source: str, outcomes: Mapping[str, bool] | None = None
    ) -> dict[str, object]:
        return {"arm": task.arm, "failed_criteria": list(failed), "source": source, "criterion_failed": dict(outcomes or {})}

    def _load(self, digest: str) -> dict[str, Any]:
        if self._cache_path.is_file():
            cache = json.loads(self._cache_path.read_text(encoding=ENCODING))
            if cache.get("oracle_digest") == digest:
                return cache
        return {"oracle_digest": digest, "mutants": {}}

    def _save(self, cache: dict[str, Any]) -> None:
        with self._lock:
            self._cache_path.write_text(json.dumps(cache, indent=2, sort_keys=True) + "\n", encoding=ENCODING)
