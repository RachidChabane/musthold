from __future__ import annotations

from collections.abc import Mapping
from pathlib import Path

from specbench.adapters.ground_stability import EXTRA_RUNS, StableGround
from specbench.domain.oracle import Outcome
from support.fakes import RecordingTrees
from support.selections import candidate, probed


class Flaky:
    """A suite whose `b` fails in its second run and whose `u2` fails in its first."""

    def __init__(self) -> None:
        self.runs = 0

    def run_all(self, tree: Path) -> Mapping[str, Outcome]:
        self.runs += 1
        failing = {"u2"} if self.runs == 1 else {"b"}
        return {t: Outcome.FAILED if t in failing else Outcome.PASSED for t in ("u1", "u2", "a", "b")}


def test_a_test_that_fails_again_leaves_u_and_r_and_the_result_is_cached(tmp_path: Path) -> None:
    suite = Flaky()
    task = probed(candidate("acme/py0"))
    stable = StableGround(tmp_path / "work", lambda _: RecordingTrees(tmp_path), lambda _: suite)
    assert stable.stabilise(task, ("u1", "u2"), ("a", "b")) == (("u1",), ("a",))
    assert suite.runs == EXTRA_RUNS
    assert stable.stabilise(task, ("u1", "u2"), ("a", "b")) == (("u1",), ("a",))
    assert suite.runs == EXTRA_RUNS
