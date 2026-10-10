"""Oracle scoring of an arm's outcome (ADR 0004, Defect definition): the oracle tree and the flaky rule, as pure rules.

The oracle tree is the agent's final non-test source, the upstream test files at their merge-commit paths
(overwriting the agent's copies) and the sealed U, H and O files at their bundle-relative paths, so that the node
ids of the oracle manifest address the same files in every tree. A sealed path never overwrites another file of the
agent's tree.
"""

from __future__ import annotations

from collections import Counter
from collections.abc import Iterable, Mapping, Sequence
from typing import Final

from specbench.domain.languages import Language, is_test_path
from specbench.domain.oracle import Oracle, OracleTest, Outcome, Part
from specbench.domain.records import Truth

SCORING_RUNS: Final = 3
FAILURE_QUORUM: Final = 2
"""A test is settled as failing when it fails in at least this many of the runs; fewer failures are flaky and clean."""

PRIMARY_PARTS: Final = (Part.U, Part.R)
FULL_PARTS: Final = (Part.U, Part.H, Part.R)
OUT_OF_SPEC_PARTS: Final = (Part.O,)
FAILURE_ORDER: Final = (Outcome.FAILED, Outcome.ERROR, Outcome.MISSING)


class OracleTreeCollision(ValueError):
    """A sealed H or O file whose path is already a file of the agent's tree."""


def compose_oracle_tree(
    language: Language,
    agent_files: Mapping[str, bytes],
    upstream_tests: Mapping[str, bytes],
    sealed_overlay: Mapping[str, bytes],
) -> dict[str, bytes]:
    """The files the oracle runs on; the agent's own test files never reach it.

    The sealed files sit at their bundle-relative paths, so a sealed path that is also a path of a non-test file the
    agent leaves in the tree is an error, never an overwrite. Test files of the agent are dropped before the overlay,
    so a sealed file at the path of one of them overwrites nothing; an upstream test file is replaced by design.
    """
    tree = {path: content for path, content in agent_files.items() if not is_test_path(language, path)}
    collisions = sorted((tree.keys() & sealed_overlay.keys()) - upstream_tests.keys())
    if collisions:
        raise OracleTreeCollision(f"sealed files would overwrite files of the agent's tree: {', '.join(collisions)}")
    tree.update(upstream_tests)
    tree.update(sealed_overlay)
    return tree


def settled_outcome(runs: Sequence[Outcome]) -> Outcome:
    """One outcome from the runs of one test: failing at the quorum, FLAKY below it, PASSED when no run failed.

    The kind of a settled failure is the most frequent one, the earlier of `FAILURE_ORDER` on a tie, so the
    result does not depend on the order of the runs.
    """
    failures = Counter(outcome for outcome in runs if outcome.is_failure)
    if sum(failures.values()) >= FAILURE_QUORUM:
        return max(FAILURE_ORDER, key=lambda kind: (failures[kind], -FAILURE_ORDER.index(kind)))
    return Outcome.FLAKY if failures else Outcome.PASSED


def settle(runs: Sequence[Mapping[str, Outcome]], expected: Sequence[str]) -> dict[str, Outcome]:
    """The settled outcome of every expected test; a test a run does not report is MISSING in that run."""
    return {test_id: settled_outcome([run.get(test_id, Outcome.MISSING) for run in runs]) for test_id in expected}


def truth_of(oracle: Oracle, settled: Mapping[str, Outcome], exposed: frozenset[str] = frozenset()) -> Truth:
    """Primary truth (U or R fails), full truth (U, H or R fails) and O failures; a FLAKY test counts clean.

    The full truth is also given without the H tests of the `exposed` criteria, those whose H the merge shaped.
    """

    def failed(tests: Iterable[OracleTest]) -> bool:
        return any(settled[t.test_id].is_failure for t in tests)

    unexposed = (t for t in oracle.tests_of(*FULL_PARTS) if t.part is not Part.H or t.criterion not in exposed)
    return Truth(
        failed(oracle.tests_of(*PRIMARY_PARTS)),
        failed(oracle.tests_of(*FULL_PARTS)),
        failed(oracle.tests_of(*OUT_OF_SPEC_PARTS)),
        failed(unexposed),
    )
