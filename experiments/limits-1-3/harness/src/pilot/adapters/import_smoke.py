"""F0 import smoke test: the edited package must import in an isolated interpreter."""

from __future__ import annotations

import sys

from pilot.adapters.process import ProcessRunner
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.domain.constants import IMPORT_SMOKE_TIMEOUT_SECONDS, PACKAGE_NAME, SOURCE_DIRNAME
from pilot.domain.edits import EditSet

ISOLATED_FLAG = "-I"
SMOKE_SCRIPT = f"import sys; sys.path.insert(0, {SOURCE_DIRNAME!r}); import {PACKAGE_NAME}"


class ProjectImportSmoke:
    """Isolated mode ignores PYTHONPATH and the working directory, so `src` is added by the script."""

    def __init__(self, workspace: ProjectWorkspace, process: ProcessRunner) -> None:
        self._workspace = workspace
        self._process = process

    def passes(self, edit_set: EditSet) -> bool:
        with self._workspace.materialized(edit_set) as copy:
            result = self._process.run(
                [sys.executable, ISOLATED_FLAG, "-c", SMOKE_SCRIPT],
                copy,
                deadline_seconds=IMPORT_SMOKE_TIMEOUT_SECONDS,
            )
        return result.returncode == 0
