"""`ShallnotProvider`: the pinned Shallnot built by the exported `script/shallnot-build`, or a recorded executable."""

from __future__ import annotations

from pathlib import Path
from typing import Final

from calibration.domain.constants import ENCODING
from calibration.domain.ports import ProcessRunner

PIN_FILE: Final = "script/shallnot.pin"
BUILD_SCRIPT: Final = "script/shallnot-build"
BUILT_BINARY: Final = ".tools/bin/shallnot"
SOURCE_ENV: Final = "SHALLNOT_SRC"
MEM_GUARD_RUN: Final = ("run", "--")
BUILD_TIMEOUT_SECONDS: Final = 1800
STDERR_PREVIEW_CHARS: Final = 600
GIT: Final = "git"
COMMIT_EXISTS_ARGUMENTS: Final = ("cat-file", "-e")
PEEL_TO_COMMIT: Final = "^{commit}"


class ShallnotUnavailable(Exception):
    """The pinned Shallnot cannot be built: its commit is missing from the source, or the build fails."""


class PinnedShallnotProvider:
    """Implements `ShallnotProvider`: builds at the commit of the export's `script/shallnot.pin`, under the guard."""

    def __init__(self, runner: ProcessRunner, mem_guard: Path, shallnot_source: Path) -> None:
        self._runner = runner
        self._mem_guard = mem_guard
        self._source = shallnot_source.resolve()

    def provide(self, export_dir: Path) -> Path:
        pin = (export_dir / PIN_FILE).read_text(encoding=ENCODING).strip()
        self._require_commit(pin)
        self._build(export_dir)
        binary = export_dir / BUILT_BINARY
        if not binary.is_file():
            raise ShallnotUnavailable(f"{BUILD_SCRIPT} produced no {BUILT_BINARY} in {export_dir}")
        return binary

    def _require_commit(self, pin: str) -> None:
        argv = [GIT, "-C", str(self._source), *COMMIT_EXISTS_ARGUMENTS, f"{pin}{PEEL_TO_COMMIT}"]
        if self._runner.run(argv).returncode != 0:
            raise ShallnotUnavailable(f"commit {pin} is missing from the Shallnot source {self._source}")

    def _build(self, export_dir: Path) -> None:
        argv = [str(self._mem_guard), *MEM_GUARD_RUN, str(export_dir / BUILD_SCRIPT)]
        result = self._runner.run(
            argv, cwd=export_dir, env={SOURCE_ENV: str(self._source)}, timeout_seconds=BUILD_TIMEOUT_SECONDS
        )
        if result.returncode != 0:
            detail = result.stderr.decode(ENCODING, errors="replace")[:STDERR_PREVIEW_CHARS].strip()
            raise ShallnotUnavailable(f"{BUILD_SCRIPT} exited {result.returncode}: {detail}")


class RecordedShallnotProvider:
    """Implements `ShallnotProvider` with a fixed executable, for tests and CI."""

    def __init__(self, script: Path) -> None:
        self._script = script

    def provide(self, export_dir: Path) -> Path:
        return self._script
