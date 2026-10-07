from __future__ import annotations

import json
import socket
import sys
from pathlib import Path

import pytest

from specbench.adapters.network_sandbox import NetworkDenialUnavailable, NetworkDeniedRunner
from specbench.adapters.process import SubprocessRunner
from specbench.application.ports import ProcessResult
from specbench.domain.mining import Rejection, Stage
from specbench.domain.selection import ProbedTask
from support.tiny_upstream import (
    BASE_SOURCE,
    OLD_TEST,
    FakeBuilder,
    candidate,
    denied_runner,
    make_upstream,
    probe_for,
)
from support.upstream import git

pytestmark = pytest.mark.integration


def test_the_sandbox_refuses_outbound_connections_but_allows_loopback() -> None:
    runner = denied_runner()
    server = socket.socket()
    server.bind(("127.0.0.1", 0))
    server.listen(1)
    code = f"import socket; socket.create_connection(('127.0.0.1', {server.getsockname()[1]}), timeout=3)"
    assert runner.run([sys.executable, "-c", code]).returncode == 0
    server.close()


def test_the_denial_check_fails_when_a_request_gets_through() -> None:
    class Open:
        def run(self, argv: object, cwd: object = None, env: object = None) -> object:
            return ProcessResult(0, "", "")

    runner = NetworkDeniedRunner(SubprocessRunner())
    runner._inner = Open()  # type: ignore[assignment]
    with pytest.raises(NetworkDenialUnavailable):
        runner.assert_denied()


def test_a_candidate_with_a_fail_to_pass_test_is_probed_offline(tmp_path: Path) -> None:
    upstream, merge = make_upstream(tmp_path)
    outcome = probe_for(tmp_path, upstream).probe(candidate(merge))
    assert isinstance(outcome, ProbedTask), outcome
    task = outcome.to_task()
    assert task.base_sha == git(upstream, "rev-parse", f"{merge}^")
    assert outcome.u_count == 1
    assert outcome.merge_total == 9
    assert outcome.r_count == 7
    ground = json.loads((tmp_path / "work" / "probe" / f"{outcome.task_id}.tests.json").read_text())
    assert ground["u"] == ["tests/test_fee.py::test_free"]
    assert "tests/test_fee.py::test_old" in ground["r"]


def test_a_test_that_needs_the_network_cannot_enter_the_oracle(tmp_path: Path) -> None:
    upstream, merge = make_upstream(tmp_path)
    outcome = probe_for(tmp_path, upstream).probe(candidate(merge))
    assert isinstance(outcome, ProbedTask)
    ground = json.loads((tmp_path / "work" / "probe" / f"{outcome.task_id}.tests.json").read_text())
    assert "tests/test_fee.py::test_needs_network" not in ground["u"] + ground["r"]


def test_the_verdict_is_cached_so_a_rerun_does_not_rebuild(tmp_path: Path) -> None:
    upstream, merge = make_upstream(tmp_path)
    first = probe_for(tmp_path, upstream, FakeBuilder(fail=True)).probe(candidate(merge))
    assert isinstance(first, Rejection) and first.stage is Stage.ENVIRONMENT_BUILD
    again = probe_for(tmp_path, upstream, FakeBuilder()).probe(candidate(merge))
    assert again == first


def test_a_pull_request_whose_files_are_not_in_the_first_parent_diff_is_rejected(tmp_path: Path) -> None:
    upstream, merge = make_upstream(tmp_path)
    outcome = probe_for(tmp_path, upstream).probe(candidate(merge, non_test_files=("pkg/other.py",)))
    assert isinstance(outcome, Rejection) and outcome.stage is Stage.MERGE_DIFF


def test_a_pull_request_with_no_touched_failing_test_is_rejected(tmp_path: Path) -> None:
    upstream, merge = make_upstream(
        tmp_path, BASE_SOURCE + "# note\n", OLD_TEST + "\n\ndef test_again():\n    assert True\n"
    )
    outcome = probe_for(tmp_path, upstream).probe(candidate(merge))
    assert isinstance(outcome, Rejection) and outcome.stage is Stage.NO_FAIL_TO_PASS
