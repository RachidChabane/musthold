"""The construction directory on disk, outside the repository: per-task status, plain task files, verification
reports and the journal of their runs."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Final

from specbench.adapters.file_corpus import read_plain_task
from specbench.application.construct import Constructed
from specbench.application.construct_run import Outcome, TaskStatus
from specbench.domain.canonical import canonical_json, stable_json
from specbench.domain.jsonread import enum_of, mapping, optional_text
from specbench.domain.sealing import SealedBundle
from specbench.domain.task import TASK_FILE_NAME, parse_task
from specbench.domain.verification import VerificationReport

STATUS_FILE: Final = "status.json"
PLAIN_DIRECTORY: Final = "plain"
VERIFICATION_FILE: Final = "verification.json"
VERIFICATION_RUNS_FILE: Final = "verification-runs.json"
H_SELECTION_RUNS_FILE: Final = "h-selection-runs.json"
SUPERSEDED_DIRECTORY: Final = "superseded"
ATTEMPT_ENTRIES: Final = (
    STATUS_FILE,
    PLAIN_DIRECTORY,
    VERIFICATION_FILE,
    VERIFICATION_RUNS_FILE,
    H_SELECTION_RUNS_FILE,
)
"""What one construction attempt leaves in a task's directory; reopening the task sets all of it aside."""
ENCODING: Final = "utf-8"


class FileStatusStore:
    """`StatusStore` under `<root>/<task id>/`; the root is checked to lie outside the repository by its creator."""

    def __init__(self, root: Path) -> None:
        self._root = root

    def _directory(self, task_id: str) -> Path:
        return self._root / task_id

    def read(self, task_id: str) -> TaskStatus | None:
        path = self._directory(task_id) / STATUS_FILE
        if not path.is_file():
            return None
        record = mapping(json.loads(path.read_text(encoding=ENCODING)), "status")
        return TaskStatus(
            task_id,
            enum_of(Outcome, record.get("outcome"), "outcome"),
            optional_text(record.get("stage"), "stage"),
            optional_text(record.get("reason"), "reason"),
            optional_text(record.get("bundle_sha256"), "bundle_sha256"),
        )

    def write(self, status: TaskStatus) -> None:
        directory = self._directory(status.task_id)
        directory.mkdir(parents=True, exist_ok=True)
        record = {
            "bundle_sha256": status.bundle_sha256,
            "outcome": status.outcome.value,
            "reason": status.reason,
            "stage": status.stage,
        }
        (directory / STATUS_FILE).write_text(canonical_json(record), encoding=ENCODING)

    def write_plain(self, constructed: Constructed) -> None:
        plain = self._directory(constructed.task.task_id) / PLAIN_DIRECTORY
        plain.mkdir(parents=True, exist_ok=True)
        (plain / TASK_FILE_NAME).write_text(constructed.task.to_canonical_json(), encoding=ENCODING)
        for path, content in constructed.bundle.files.items():
            target = plain / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(content)

    def read_plain(self, task_id: str) -> Constructed | None:
        plain = self._directory(task_id) / PLAIN_DIRECTORY
        if not (plain / TASK_FILE_NAME).is_file():
            return None
        task_json, files = read_plain_task(plain)
        return Constructed(parse_task(task_json), SealedBundle(files))

    def write_verification(self, task_id: str, report: VerificationReport) -> None:
        directory = self._directory(task_id)
        directory.mkdir(parents=True, exist_ok=True)
        (directory / VERIFICATION_FILE).write_text(stable_json(report.to_record()), encoding=ENCODING)

    def verification_runs_path(self, task_id: str) -> Path:
        """Where the journal of the task's verification runs is kept, beside its verification report."""
        return self._directory(task_id) / VERIFICATION_RUNS_FILE

    def h_selection_runs_path(self, task_id: str) -> Path:
        """Where the journal of the runs that selected the task's H tests is kept."""
        return self._directory(task_id) / H_SELECTION_RUNS_FILE

    def set_aside(self, task_id: str) -> str:
        """Moves the last attempt's status, plain files, reports and journals to `superseded/<n>/`, n counting from 1."""
        directory = self._directory(task_id)
        superseded = directory / SUPERSEDED_DIRECTORY
        earlier = [int(p.name) for p in superseded.iterdir() if p.name.isdigit()] if superseded.is_dir() else []
        target = superseded / str(max(earlier, default=0) + 1)
        target.mkdir(parents=True)
        for name in ATTEMPT_ENTRIES:
            if (directory / name).exists():
                (directory / name).rename(target / name)
        return str(target)
