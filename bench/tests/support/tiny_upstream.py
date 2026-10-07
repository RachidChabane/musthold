"""A tiny upstream repository whose merge adds a feature and its tests, with the probe pieces that read it."""

from __future__ import annotations

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
from specbench.application.ports import BuiltEnvironment, EnvRequest
from specbench.domain.candidates import Candidate
from specbench.domain.languages import Language
from support.upstream import commit_all, git

BASE_SOURCE = "def fee(minutes):\n    return 5\n"
MERGE_SOURCE = "def fee(minutes):\n    if minutes < 10:\n        return 0\n    return 5\n"
OLD_TEST = "from pkg.fee import fee\n\n\ndef test_old():\n    assert fee(60) == 5\n"
NEW_TESTS = (
    OLD_TEST
    + "\n\ndef test_free():\n    assert fee(5) == 0\n\n\ndef test_needs_network():\n    import socket\n    socket.create_connection(('1.1.1.1', 443), timeout=3)\n"
)
BASE_TESTS = "".join(f"def test_base_{n}():\n    assert True\n\n\n" for n in range(6))
REPOSITORY = "acme/parkfee"


def denied_runner() -> NetworkDeniedRunner:
    """A runner that denies the network, or a skipped test on a machine where the denial cannot be established."""
    try:
        runner = NetworkDeniedRunner(SubprocessRunner())
        runner.assert_denied()
    except NetworkDenialUnavailable as error:
        pytest.skip(str(error))
    return runner


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
    denied = denied_runner()
    return OfflineProbe(
        mirrors,
        lambda clone: GitHistory(clone, plain),
        builder or FakeBuilder(),
        lambda language, env_dir: PytestSuiteRunner(Path(sys.executable), denied),
        tmp_path / "work",
        lambda version: Path(sys.executable),
    )
