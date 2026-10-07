"""Candidates and probed tasks for tests that need a selection without running a probe."""

from __future__ import annotations

from datetime import date

from specbench.domain.candidates import Candidate
from specbench.domain.languages import Language
from specbench.domain.selection import ProbedTask
from specbench.domain.task import Toolchain


def candidate(repository: str, language: Language = Language.PYTHON, number: int = 1) -> Candidate:
    return Candidate(
        repository=repository,
        language=language,
        stars=500,
        spdx_id="MIT",
        pull_number=number,
        title="feat: add a thing",
        merged_on=date(2026, 8, 1),
        base_sha="a" * 40,
        merge_sha="b" * 40,
        non_test_files=("pkg/a.py",),
        non_test_lines=40,
        test_files=("tests/test_a.py",),
        licence_path="LICENSE",
        notice_path=None,
    )


def probed(c: Candidate) -> ProbedTask:
    return ProbedTask(c, "c" * 40, Toolchain("python", "3.13.14"), {"uv.lock": "d" * 64}, 2, 5, 9, "e" * 64)
