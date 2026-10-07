"""Drives the product's alignment binary to its first `awaiting_host` wave, as `s1relay/internal/e2e/e2e_test.go` does."""

from __future__ import annotations

import os
import shutil
import tempfile
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from calibration.adapters.go_layout import PRODUCT_ALIGNMENT_BINARY, BuilderFailure
from calibration.domain.constants import ENCODING, EXIT_OK
from calibration.domain.ports import ProcessRunner

FIXTURE_PROJECT_DIR: Final = Path("s1relay") / "testdata" / "alignment" / "project"
PROJECT_COPY_NAME: Final = "project"
STATE_DIR_NAME: Final = "state"
CACHE_DIR_NAME: Final = "cache"
HOME_DIR_NAME: Final = "home"
SCRATCH_PREFIX: Final = "calibration-alignment-"
PATH_VARIABLE: Final = "PATH"
STATE_VARIABLE: Final = "XDG_STATE_HOME"
CACHE_VARIABLE: Final = "XDG_CACHE_HOME"
HOME_VARIABLE: Final = "HOME"
SHALLNOT_VARIABLE: Final = "SPEC_PROOF_SHALLNOT"
RECORDED_REPORT_VARIABLE: Final = "SHALLNOT_RECORDED_REPORT"
PROVE_ARGUMENTS: Final = ("prove", "--all")
EXPORT_ARGUMENTS: Final = ("systemone", "export")
EXIT_AWAITING_HOST: Final = 3
DRIVE_TIMEOUT_SECONDS: Final = 600.0


class BinaryAlignmentDriver:
    """`AlignmentDriver` over the compiled `spec-proof-alignment` found in `binaries_dir`."""

    def __init__(self, runner: ProcessRunner, binaries_dir: Path) -> None:
        self._runner = runner
        self._binary = binaries_dir / PRODUCT_ALIGNMENT_BINARY
        self._binaries_dir = binaries_dir

    def first_wave_export(self, export_dir: Path, shallnot_script: Path, report: Path) -> str:
        with tempfile.TemporaryDirectory(prefix=SCRATCH_PREFIX) as scratch:
            base = Path(scratch)
            project = base / PROJECT_COPY_NAME
            shutil.copytree(export_dir / FIXTURE_PROJECT_DIR, project)
            env = self._environment(base, shallnot_script, report)
            self._expect(PROVE_ARGUMENTS, EXIT_AWAITING_HOST, project, env)
            return self._expect(EXPORT_ARGUMENTS, EXIT_OK, project, env)

    def _environment(self, base: Path, shallnot_script: Path, report: Path) -> dict[str, str]:
        search_path = str(self._binaries_dir.absolute()) + os.pathsep + os.environ.get(PATH_VARIABLE, "")
        return {
            PATH_VARIABLE: search_path,
            STATE_VARIABLE: str(base / STATE_DIR_NAME),
            CACHE_VARIABLE: str(base / CACHE_DIR_NAME),
            HOME_VARIABLE: str(base / HOME_DIR_NAME),
            SHALLNOT_VARIABLE: str(shallnot_script.absolute()),
            RECORDED_REPORT_VARIABLE: str(report.absolute()),
        }

    def _expect(self, arguments: tuple[str, ...], exit_code: int, project: Path, env: Mapping[str, str]) -> str:
        command = [str(self._binary), *arguments]
        result = self._runner.run(command, cwd=project, env=env, timeout_seconds=DRIVE_TIMEOUT_SECONDS)
        if result.returncode != exit_code:
            reason = f"exit {result.returncode}, expected {exit_code}"
            raise BuilderFailure(command, reason, result.stderr.decode(ENCODING, "replace"))
        return result.stdout.decode(ENCODING)
