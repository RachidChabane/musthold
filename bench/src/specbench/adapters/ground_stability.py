"""Stable human tests: U and R as the probe found them, minus every test that does not pass again at merge.

Verification demands that each oracle test gives the same outcome in three runs. The probe ran the suite once, so
before any model call is spent on a task the suite runs twice more here, and a test that fails in either run leaves
U or R. The result is cached beside the probe verdict, so a rerun repeats nothing.
"""

from __future__ import annotations

import json
from collections.abc import Callable
from pathlib import Path
from typing import Final

from specbench.adapters.offline_probe import PROBE_DIRECTORY
from specbench.application.ports import SuiteRunner, TreeFactory, TreeSpec
from specbench.domain.canonical import canonical_json
from specbench.domain.jsonread import mapping, sequence, text
from specbench.domain.oracle import Outcome
from specbench.domain.selection import ProbedTask

EXTRA_RUNS: Final = 2
STABLE_SUFFIX: Final = ".stable.json"
ENCODING: Final = "utf-8"

GroundIds = tuple[tuple[str, ...], tuple[str, ...]]


class StableGround:
    def __init__(
        self,
        work: Path,
        trees_for: Callable[[ProbedTask], TreeFactory],
        suite_for: Callable[[ProbedTask], SuiteRunner],
    ) -> None:
        self._work = work
        self._trees_for = trees_for
        self._suite_for = suite_for

    def stabilise(self, task: ProbedTask, u_ids: tuple[str, ...], r_ids: tuple[str, ...]) -> GroundIds:
        cache = self._work / PROBE_DIRECTORY / f"{task.task_id}{STABLE_SUFFIX}"
        if cache.is_file():
            record = mapping(json.loads(cache.read_text(encoding=ENCODING)), "stable ground")
            return (
                tuple(text(t, "u") for t in sequence(record.get("u"), "u")),
                tuple(text(t, "r") for t in sequence(record.get("r"), "r")),
            )
        suite = self._suite_for(task)
        passing = set(u_ids) | set(r_ids)
        with self._trees_for(task).materialize(TreeSpec(task.candidate.merge_sha)) as tree:
            for _ in range(EXTRA_RUNS):
                outcomes = suite.run_all(tree)
                passing &= {test for test, outcome in outcomes.items() if outcome is Outcome.PASSED}
        stable: GroundIds = (
            tuple(t for t in u_ids if t in passing),
            tuple(t for t in r_ids if t in passing),
        )
        cache.parent.mkdir(parents=True, exist_ok=True)
        cache.write_text(canonical_json({"r": list(stable[1]), "u": list(stable[0])}), encoding=ENCODING)
        return stable
