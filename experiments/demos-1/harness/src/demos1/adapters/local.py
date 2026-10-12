"""The host's clock and file system, and the deviations file."""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import stat
import time
from collections.abc import Callable, Sequence
from datetime import UTC, datetime
from pathlib import Path, PurePosixPath
from typing import Final, cast

from demos1.domain.container import Measure
from demos1.domain.deviations import FILE_TITLE, Deviation
from demos1.domain.jsonvalue import JsonValue, encode

MEASURE_SUFFIX: Final = ".measure.json"
READ_CHUNK: Final = 1 << 20
BLOCK_BYTES: Final = 512
"""`st_blocks` counts 512-byte blocks."""


class SystemClock:
    def now(self) -> datetime:
        return datetime.now(UTC)

    def monotonic(self) -> float:
        return time.monotonic()

    def sleep(self, seconds: float) -> None:
        if seconds > 0:
            time.sleep(seconds)


class ReplayClock(SystemClock):
    """The clock of a replay from saved responses: nothing is waited for."""

    def sleep(self, seconds: float) -> None:
        return None


class LocalFiles:
    def make_fresh(self, directories: Sequence[PurePosixPath]) -> None:
        for directory in directories:
            path = Path(directory)
            path.mkdir(parents=True, exist_ok=True)

    def ensure_directory(self, directory: PurePosixPath) -> None:
        Path(directory).mkdir(parents=True, exist_ok=True)

    def exists(self, path: PurePosixPath) -> bool:
        return Path(path).exists()

    def is_directory(self, path: PurePosixPath) -> bool:
        return Path(path).is_dir()

    def read_text(self, path: PurePosixPath) -> str | None:
        try:
            return Path(path).read_text(encoding="utf-8", errors="replace")
        except (FileNotFoundError, IsADirectoryError, NotADirectoryError):
            return None

    def write_text(self, path: PurePosixPath, content: str) -> None:
        target = Path(path)
        target.parent.mkdir(parents=True, exist_ok=True)
        partial = target.with_name(target.name + ".partial")
        partial.write_text(content, encoding="utf-8")
        os.replace(partial, target)

    def append_text(self, path: PurePosixPath, content: str) -> None:
        target = Path(path)
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open("a", encoding="utf-8") as handle:
            handle.write(content)

    def list_names(self, directory: PurePosixPath) -> list[str]:
        try:
            return sorted(entry.name for entry in Path(directory).iterdir())
        except (FileNotFoundError, NotADirectoryError):
            return []

    def move(self, source: PurePosixPath, target: PurePosixPath) -> None:
        destination = Path(target)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(source), str(destination))

    def sha256(self, path: PurePosixPath) -> str | None:
        digest = hashlib.sha256()
        try:
            with Path(path).open("rb") as handle:
                while chunk := handle.read(READ_CHUNK):
                    digest.update(chunk)
        except (FileNotFoundError, IsADirectoryError):
            return None
        return digest.hexdigest()

    def read_json(self, path: PurePosixPath) -> JsonValue | None:
        content = self.read_text(path)
        if content is None:
            return None
        try:
            return cast(JsonValue, json.loads(content))
        except json.JSONDecodeError:
            return None

    def write_json(self, path: PurePosixPath, document: JsonValue) -> None:
        self.write_text(path, encode(document))

    def tree_size(self, path: PurePosixPath) -> int:
        """Allocated bytes of every file under the path, as `du` counts them, each inode once."""
        seen: set[tuple[int, int]] = set()
        total = 0
        for directory, _, names in os.walk(path):
            for name in names:
                try:
                    status = os.lstat(os.path.join(directory, name))
                except FileNotFoundError:
                    continue
                key = (status.st_dev, status.st_ino)
                if key in seen:
                    continue
                seen.add(key)
                total += status.st_blocks * BLOCK_BYTES
        return total

    def free_bytes(self, path: PurePosixPath) -> int:
        return shutil.disk_usage(path).free

    def remove_tree(self, path: PurePosixPath) -> None:
        target = Path(path)
        if target.is_symlink() or target.is_file():
            target.unlink()
            return
        if not target.exists():
            return

        def writable_then_retry(function: Callable[[str], object], failed: str, _: BaseException) -> None:
            for entry in (Path(failed).parent, Path(failed)):
                if entry.exists() and not entry.is_symlink():
                    entry.chmod(entry.stat().st_mode | stat.S_IWUSR | stat.S_IXUSR | stat.S_IRUSR)
            function(failed)

        shutil.rmtree(target, onexc=writable_then_retry)

    def read_measure(self, prefix: PurePosixPath) -> Measure | None:
        document = self.read_json(PurePosixPath(str(prefix) + MEASURE_SUFFIX))
        if not isinstance(document, dict):
            return None
        fields = cast(dict[str, object], document)
        wall, peak, status = fields.get("wall_seconds"), fields.get("memory_peak_bytes"), fields.get("exit_status")
        limit = fields.get("limit_seconds")
        if not isinstance(wall, int | float) or not isinstance(status, int):
            return None
        return Measure(
            wall_seconds=float(wall),
            memory_peak_bytes=peak if isinstance(peak, int) else None,
            exit_status=status,
            timed_out=fields.get("timed_out") is True,
            limit_seconds=limit if isinstance(limit, int) else None,
        )


class DeviationFile:
    """$DEMOS1_WORK/DEVIATIONS.md, appended to, never rewritten."""

    def __init__(self, files: LocalFiles, path: PurePosixPath) -> None:
        self._files = files
        self._path = path

    def append(self, deviation: Deviation) -> None:
        if not self._files.exists(self._path):
            self._files.append_text(self._path, FILE_TITLE + "\n")
        self._files.append_text(self._path, deviation.render())
