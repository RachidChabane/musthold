"""What the night budget reads on disk: each job's ledger, the recorded canary reserve and the construction progress."""

from __future__ import annotations

import fcntl
import json
import time
from collections.abc import Generator, Mapping
from contextlib import contextmanager
from pathlib import Path
from typing import Final

from specbench.adapters.run_store import ENCODING, write_atomically
from specbench.adapters.selection_store import SELECTION_FILE, TASKS_DIRECTORY
from specbench.domain.arms import Harness
from specbench.domain.call_ledger import EMPTY_LEDGER, CallLedger
from specbench.domain.canonical import JsonValue, canonical_json
from specbench.domain.jsonread import integer, mapping, sequence, text
from specbench.domain.languages import Language
from specbench.domain.night_budget import NightJob
from specbench.domain.task import TASK_FILE_NAME

BUDGET_LOCK_FILE_NAME: Final = "budget.lock"
CANARY_RESERVE_FILE_NAME: Final = "canary_reserve.json"
LOCK_POLL_SECONDS: Final = 0.2
LOCK_TIMEOUT_SECONDS: Final = 60.0


class BudgetLockTimeout(RuntimeError):
    """Another process held the budget lock for longer than a computation takes."""


@contextmanager
def budget_lock(path: Path, timeout_seconds: float = LOCK_TIMEOUT_SECONDS) -> Generator[None]:
    """Serialise the computations and recordings of the night budget across processes with an exclusive `flock`."""
    path.parent.mkdir(parents=True, exist_ok=True)
    deadline = time.monotonic() + timeout_seconds
    with path.open("ab") as handle:
        while True:
            try:
                fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                if time.monotonic() >= deadline:
                    raise BudgetLockTimeout(f"{path} stayed locked for {timeout_seconds:g}s") from None
                time.sleep(LOCK_POLL_SECONDS)
        yield


class FileNightLedgers:
    """`NightLedgers`: one ledger file per (job, harness); a missing file or a missing entry is an empty ledger."""

    def __init__(self, paths: Mapping[tuple[NightJob, Harness], Path]) -> None:
        self._paths = paths

    def read(self, job: NightJob, harness: Harness) -> CallLedger:
        path = self._paths.get((job, harness))
        if path is None or not path.is_file():
            return EMPTY_LEDGER
        return CallLedger.from_record(mapping(json.loads(path.read_text(encoding=ENCODING)), str(path)))


class FileCanaryReserve:
    """`CanaryReserveStore`: `{"claude-code": 3}` in the state directory."""

    def __init__(self, path: Path) -> None:
        self._path = path

    def _all(self) -> dict[str, int]:
        if not self._path.is_file():
            return {}
        body = mapping(json.loads(self._path.read_text(encoding=ENCODING)), str(self._path))
        return {key: integer(value, f"canary reserve {key}") for key, value in body.items()}

    def measured(self, harness: Harness) -> int | None:
        return self._all().get(harness.value)

    def record(self, harness: Harness, executions: int) -> None:
        reserves: dict[str, JsonValue] = {**self._all(), harness.value: executions}
        write_atomically(self._path, canonical_json(reserves))


class FileConstructionProgress:
    """`ConstructionProgress`: the selection names the tasks per language, the corpus holds the sealed ones."""

    def __init__(self, selection_dir: Path, corpus_dir: Path) -> None:
        self._selection_dir = selection_dir
        self._corpus_dir = corpus_dir

    def has_unsealed_task(self) -> bool:
        selected = self._selected_per_language()
        return any(self._sealed_in(language) < count for language, count in selected.items())

    def _selected_per_language(self) -> dict[Language, int]:
        body = mapping(json.loads((self._selection_dir / SELECTION_FILE).read_text(encoding=ENCODING)), "selection")
        counts: dict[Language, int] = {}
        for record in sequence(body.get("tasks"), "selection tasks"):
            task_id = text(mapping(record, "selection task").get("task_id"), "task_id")
            task = json.loads(
                (self._selection_dir / TASKS_DIRECTORY / task_id / TASK_FILE_NAME).read_text(encoding=ENCODING)
            )
            language = Language(text(mapping(task, task_id).get("language"), "language"))
            counts[language] = counts.get(language, 0) + 1
        return counts

    def _sealed_in(self, language: Language) -> int:
        directory = self._corpus_dir / language.value
        if not directory.is_dir():
            return 0
        return sum(1 for task in directory.iterdir() if (task / TASK_FILE_NAME).is_file())
