from __future__ import annotations

import json
import socket
import subprocess
import sys
from datetime import date
from pathlib import Path

import pytest

from specbench.adapters.env_builder import EnvironmentBuildError
from specbench.adapters.git_history import GitHistory, GitMirrors
from specbench.adapters.network_sandbox import NetworkDenialUnavailable, NetworkDeniedRunner
from specbench.adapters.offline_probe import OfflineProbe
from specbench.adapters.oracle_runners import PytestSuiteRunner
from specbench.adapters.process import SubprocessRunner
from specbench.application.ports import BuiltEnvironment, EnvRequest, ProcessResult
from specbench.domain.candidates import Candidate
from specbench.domain.languages import Language
from specbench.domain.mining import Rejection, Stage
from specbench.domain.selection import ProbedTask
from support.upstream import commit_all, git

pytestmark = pytest.mark.integration

BASE_SOURCE = "def fee(minutes):\n    return 5\n"
MERGE_SOURCE = "def fee(minutes):\n    if minutes < 10:\n        return 0\n    return 5\n"
OLD_TEST = "from pkg.fee import fee\n\n\ndef test_old():\n    assert fee(60) == 5\n"
NEW_TESTS = (
    OLD_TEST
    + "\n\ndef test_free():\n    assert fee(5) == 0\n\n\ndef test_needs_network():\n    import socket\n    socket.create_connection(('1.1.1.1', 443), timeout=3)\n"
)
BASE_TESTS = "".join(f"def test_base_{n}():\n    assert True\n\n\n" for n in range(6))
REPOSITORY = "acme/parkfee"


class FakeBuilder:
    def __init__(self, fail: bool = False) -> None:
        self._fail = fail

    def build(self, request: EnvRequest) -> BuiltEnvironment:
        if self._fail:
            raise EnvironmentBuildError("uv sync failed: no matching python")
        request.destination.mkdir(parents=True, exist_ok=True)
        return BuiltEnvironment(request.language, request.destination, {"requirements.txt": "f" * 64}, "3.13.14")


def make_upstream(root: Path, merge_source: str = MERGE_SOURCE, merge_tests: str = NEW_TESTS) -> tuple[Path, str]:
    repo = root / "upstream"
    (repo / "pkg").mkdir(parents=True)
    (repo / "tests").mkdir()
    (repo / "pkg" / "__init__.py").write_text("")
    (repo / "pkg" / "fee.py").write_text(BASE_SOURCE)
    (repo / "requirements.txt").write_text("")
    (repo / "tests" / "test_base.py").write_text(BASE_TESTS)
    git(repo, "init", "-q", "-b", "main")
    commit_all(repo, "base")
    (repo / "pkg" / "fee.py").write_text(merge_source)
    (repo / "tests" / "test_fee.py").write_text(merge_tests)
    merge = commit_all(repo, "merge")
    return repo, merge


def candidate(merge: str, **changes: object) -> Candidate:
    fields: dict[str, object] = {
        "repository": REPOSITORY,
        "language": Language.PYTHON,
        "stars": 500,
        "spdx_id": "MIT",
        "pull_number": 12,
        "title": "feat: free first minutes",
        "merged_on": date(2026, 8, 3),
        "base_sha": "a" * 40,
        "merge_sha": merge,
        "non_test_files": ("pkg/fee.py",),
        "non_test_lines": 40,
        "test_files": ("tests/test_fee.py",),
        "licence_path": "LICENSE",
        "notice_path": None,
    }
    return Candidate(**{**fields, **changes})  # type: ignore[arg-type]


def probe_for(tmp_path: Path, upstream: Path, builder: FakeBuilder | None = None) -> OfflineProbe:
    mirrors = GitMirrors(tmp_path / "mirrors", SubprocessRunner())
    if not mirrors.path_of(REPOSITORY).exists():
        subprocess.run(["git", "clone", "--bare", "-q", str(upstream), str(mirrors.path_of(REPOSITORY))], check=True)
    plain = SubprocessRunner()
    denied = NetworkDeniedRunner(SubprocessRunner())
    return OfflineProbe(
        mirrors,
        lambda clone: GitHistory(clone, plain),
        builder or FakeBuilder(),
        lambda language, env_dir: PytestSuiteRunner(Path(sys.executable), denied),
        tmp_path / "work",
    )


def test_the_sandbox_refuses_outbound_connections_but_allows_loopback() -> None:
    runner = NetworkDeniedRunner(SubprocessRunner())
    runner.assert_denied()
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
