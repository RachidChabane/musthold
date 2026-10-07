"""Patch bytes from the before and after text of files, produced by `git diff --no-index` so `git apply` accepts them."""

from __future__ import annotations

import tempfile
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessRunner

BEFORE_DIRECTORY: Final = "a"
AFTER_DIRECTORY: Final = "b"
DIFF_ARGUMENTS: Final = ("git", "diff", "--no-index", "--no-color", "--no-prefix", "-U3")
DIFFERENCES_EXIT_CODE: Final = 1
ENCODING: Final = "utf-8"


class PatchBuildError(RuntimeError):
    """`git diff` could not compare the files."""


class GitDiffPatchBuilder:
    def __init__(self, runner: ProcessRunner) -> None:
        self._runner = runner

    def build(self, before: Mapping[str, str], after: Mapping[str, str]) -> bytes:
        with tempfile.TemporaryDirectory(prefix="specbench-patch-") as scratch:
            root = Path(scratch)
            for directory, files in ((BEFORE_DIRECTORY, before), (AFTER_DIRECTORY, after)):
                for path, text in files.items():
                    target = root / directory / path
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_text(text, encoding=ENCODING)
            result = self._runner.run([*DIFF_ARGUMENTS, BEFORE_DIRECTORY, AFTER_DIRECTORY], cwd=root)
        if result.returncode not in (0, DIFFERENCES_EXIT_CODE):
            raise PatchBuildError(result.stderr.strip())
        return result.stdout.encode(ENCODING)
