"""The corpus directory: `<root>/<language>/<task-id>/{task.json,sealed.age}`, `corpus.lock.json`, `recipients.txt`."""

from __future__ import annotations

from pathlib import Path
from typing import Final

from specbench.domain.languages import Language
from specbench.domain.sealing import EMPTY_LOCK, LOCK_FILE_NAME, CorpusLock, SealingError, parse_lock
from specbench.domain.task import SEALED_FILE_NAME, TASK_FILE_NAME

RECIPIENTS_FILE_NAME: Final = "recipients.txt"
ENCODING: Final = "utf-8"
COMMENT_PREFIX: Final = "#"


class FileCorpus:
    def __init__(self, root: Path) -> None:
        self._root = root

    def _task_directory(self, task_id: str) -> Path:
        for language in Language:
            candidate = self._root / language.value / task_id
            if candidate.is_dir():
                return candidate
        raise SealingError(f"task {task_id} is not in the corpus")

    def task_ids(self) -> list[str]:
        return sorted(
            directory.name
            for language in Language
            if (self._root / language.value).is_dir()
            for directory in (self._root / language.value).iterdir()
            if (directory / TASK_FILE_NAME).is_file()
        )

    def read_task_json(self, task_id: str) -> str:
        return (self._task_directory(task_id) / TASK_FILE_NAME).read_text(encoding=ENCODING)

    def read_sealed(self, task_id: str) -> bytes:
        return (self._task_directory(task_id) / SEALED_FILE_NAME).read_bytes()

    def write_task(self, language: Language, task_id: str, task_json: str, ciphertext: bytes) -> None:
        directory = self._root / language.value / task_id
        directory.mkdir(parents=True, exist_ok=True)
        (directory / TASK_FILE_NAME).write_text(task_json, encoding=ENCODING)
        (directory / SEALED_FILE_NAME).write_bytes(ciphertext)

    def write_notice(self, language: Language, task_id: str, name: str, content: str) -> None:
        """An upstream licence or notice file, kept beside the task's `task.json` and `sealed.age`."""
        directory = self._root / language.value / task_id
        directory.mkdir(parents=True, exist_ok=True)
        (directory / name).write_text(content, encoding=ENCODING)

    def read_lock(self) -> CorpusLock:
        path = self._root / LOCK_FILE_NAME
        return parse_lock(path.read_text(encoding=ENCODING)) if path.is_file() else EMPTY_LOCK

    def write_lock(self, lock: CorpusLock) -> None:
        self._root.mkdir(parents=True, exist_ok=True)
        (self._root / LOCK_FILE_NAME).write_text(lock.to_canonical_json(), encoding=ENCODING)

    def read_recipients(self) -> list[str]:
        path = self._root / RECIPIENTS_FILE_NAME
        if not path.is_file():
            raise SealingError(f"{path} lists no recipient")
        lines = [line.strip() for line in path.read_text(encoding=ENCODING).splitlines()]
        return [line for line in lines if line and not line.startswith(COMMENT_PREFIX)]


def read_plain_task(directory: Path) -> tuple[str, dict[str, bytes]]:
    """A task laid out as plain files outside the repository: `task.json` plus every other file as the bundle."""
    task_json = (directory / TASK_FILE_NAME).read_text(encoding=ENCODING)
    files = {
        path.relative_to(directory).as_posix(): path.read_bytes()
        for path in sorted(directory.rglob("*"))
        if path.is_file() and path.relative_to(directory).as_posix() != TASK_FILE_NAME
    }
    return task_json, files
