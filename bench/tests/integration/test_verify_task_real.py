from __future__ import annotations

import difflib
import os
import shutil
import subprocess
import sys
from datetime import date
from pathlib import Path

import pytest

from specbench.adapters.git_trees import GitTreeFactory, TreeBuildError
from specbench.adapters.operator_engine import UnavailableOperatorEngine
from specbench.adapters.oracle_runners import GoTestOracleRunner, PytestOracleRunner
from specbench.adapters.process import MemoryFloorRunner, SubprocessRunner, system_free_percent
from specbench.application.ports import TreeSpec
from specbench.application.verify_task import TaskVerifier
from specbench.domain.canonical import canonical_json
from specbench.domain.languages import Language
from specbench.domain.oracle import OracleTest, Outcome, Part, oracle_from_bundle
from specbench.domain.sealing import SealedBundle
from specbench.domain.task import Criterion, CriterionStatus, Task, Toolchain
from specbench.domain.verification import CheckName, CheckStatus

pytestmark = pytest.mark.integration

TASK = "ACME-PARKFEE-12"
BASE_FEE = "def fee(minutes):\n    return 5\n"
MERGE_FEE = "def fee(minutes):\n    if minutes < 10:\n        return 0\n    return 5\n"
GIT_ENV = {
    "GIT_AUTHOR_NAME": "t",
    "GIT_AUTHOR_EMAIL": "t@example.com",
    "GIT_COMMITTER_NAME": "t",
    "GIT_COMMITTER_EMAIL": "t@example.com",
    "GIT_CONFIG_GLOBAL": os.devnull,
    "GIT_CONFIG_SYSTEM": os.devnull,
}


def git(repo: Path, *arguments: str) -> str:
    done = subprocess.run(
        ["git", "-C", str(repo), *arguments], capture_output=True, text=True, check=True, env={**os.environ, **GIT_ENV}
    )
    return done.stdout.strip()


def patch(path: str, old: str, new: str) -> bytes:
    return "".join(difflib.unified_diff(old.splitlines(True), new.splitlines(True), f"a/{path}", f"b/{path}")).encode()


def commit_all(repo: Path, message: str) -> str:
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", message)
    return git(repo, "rev-parse", "HEAD")


@pytest.fixture
def upstream(tmp_path: Path) -> tuple[Path, str, str]:
    repo = tmp_path / "upstream"
    (repo / "pkg").mkdir(parents=True)
    (repo / "tests").mkdir()
    (repo / "pkg" / "__init__.py").write_text("")
    (repo / "pkg" / "fee.py").write_text(BASE_FEE)
    (repo / "tests" / "test_r.py").write_text("from pkg.fee import fee\n\n\ndef test_r1():\n    assert fee(60) == 5\n")
    git(repo, "init", "-q", "-b", "main")
    base = commit_all(repo, "base")
    (repo / "pkg" / "fee.py").write_text(MERGE_FEE)
    merge = commit_all(repo, "merge")
    return repo, base, merge


def bundle(violation_one: str) -> SealedBundle:
    tests = [
        {"id": "tests/test_u.py::test_u1", "part": "U", "criterion": f"{TASK}-1"},
        {"id": "tests/test_r.py::test_r1", "part": "R", "criterion": None},
        {"id": "tests/test_h.py::test_h1", "part": "H", "criterion": f"{TASK}-2"},
        {"id": "tests/test_o.py::test_o1", "part": "O", "criterion": None},
    ]
    variants = {
        "V1": violation_one,
        "V2": "    if minutes < 10:\n        return 2 if minutes == 5 else 0\n    return 5\n",
        "V3": "    if minutes < 9:\n        return 0\n    return 5\n",
        "V4": "    if minutes < 11:\n        return 0\n    return 5\n",
    }
    criterion_of = {"V1": 1, "V2": 1, "V3": 2, "V4": 2}
    files: dict[str, bytes] = {
        "SPEC.md": b"# Spec\n",
        "oracle/manifest.json": canonical_json(tests).encode(),  # type: ignore[arg-type]
        "oracle/U/tests/test_u.py": b"from pkg.fee import fee\n\n\ndef test_u1():\n    assert fee(5) == 0\n",
        "oracle/H/tests/test_h.py": b"from pkg.fee import fee\n\n\ndef test_h1():\n    assert fee(9) == 0 and fee(10) == 5\n",
        "oracle/O/tests/test_o.py": b"from pkg.fee import fee\n\n\ndef test_o1():\n    assert isinstance(fee(1), int)\n",
    }
    index: list[dict[str, str]] = []
    for name, body in variants.items():
        files[f"violations/{name}.patch"] = patch("pkg/fee.py", MERGE_FEE, "def fee(minutes):\n" + body)
        index.append({"id": name, "criterion": f"{TASK}-{criterion_of[name]}", "patch": f"violations/{name}.patch"})
    files["violations/index.json"] = canonical_json(index).encode()  # type: ignore[arg-type]
    planted = {
        "P1": "def fee(minutes):\n    return 0 if minutes < 10 else 5\n",
        "P2": "FREE_MINUTES = 10\n\n\ndef fee(minutes):\n    if minutes < FREE_MINUTES:\n        return 0\n    return 5\n",
    }
    for name, content in planted.items():
        files[f"planted/{name}.patch"] = patch("pkg/fee.py", MERGE_FEE, content)
    files["planted/index.json"] = canonical_json([{"id": n, "patch": f"planted/{n}.patch"} for n in planted]).encode()  # type: ignore[arg-type]
    return SealedBundle(files)


def task(base: str, merge: str) -> Task:
    return Task(
        task_id=TASK,
        language=Language.PYTHON,
        repository="acme/parkfee",
        pull_number=12,
        merged_on=date(2026, 8, 3),
        spdx_id="MIT",
        licence_path="LICENSE",
        notice_path=None,
        base_sha=base,
        merge_sha=merge,
        lockfile_digests={},
        toolchain=Toolchain("python", "3.13"),
        criteria=(Criterion(f"{TASK}-1", CriterionStatus.NEW), Criterion(f"{TASK}-2", CriterionStatus.NEW)),
    )


def verifier(repo: Path) -> TaskVerifier:
    runner = MemoryFloorRunner(SubprocessRunner(), system_free_percent)
    return TaskVerifier(
        GitTreeFactory(repo), PytestOracleRunner(Path(sys.executable), runner), UnavailableOperatorEngine()
    )


GOOD_V1 = "    if minutes < 10:\n        return 1 if minutes == 5 else 0\n    return 5\n"
STRAY_V1 = "    if minutes < 10:\n        return 1\n    return 5\n"


def test_a_real_task_is_verified_end_to_end(upstream: tuple[Path, str, str]) -> None:
    repo, base, merge = upstream
    sealed = bundle(GOOD_V1)
    report = verifier(repo).verify(task(base, merge), oracle_from_bundle(sealed), sealed)
    assert report.verified, report.to_record()
    assert report.runs == 3
    survivors = next(c for c in report.checks if c.name is CheckName.OPERATOR_SURVIVORS)
    assert survivors.status is CheckStatus.NOT_EVALUATED
    assert report.limitation is not None and "spec-proof-dp" in report.limitation


def test_a_violation_that_also_breaks_another_criterion_drops_the_task(upstream: tuple[Path, str, str]) -> None:
    repo, base, merge = upstream
    sealed = bundle(STRAY_V1)
    report = verifier(repo).verify(task(base, merge), oracle_from_bundle(sealed), sealed)
    assert not report.verified
    diagonal = next(c for c in report.checks if c.name is CheckName.VIOLATIONS_DIAGONAL)
    assert any("V1 also fails tests/test_h.py::test_h1" in problem for problem in diagonal.problems)


def test_a_missing_commit_or_a_patch_that_does_not_apply_is_an_error(upstream: tuple[Path, str, str]) -> None:
    repo, _, merge = upstream
    factory = GitTreeFactory(repo)
    with pytest.raises(TreeBuildError), factory.materialize(TreeSpec("f" * 40)):
        pass
    with (
        pytest.raises(TreeBuildError),
        factory.materialize(TreeSpec(merge, (patch("pkg/fee.py", "nothing\n", "else\n"),))),
    ):
        pass


def test_pytest_runner_reports_failures_errors_and_collection_errors(tmp_path: Path) -> None:
    (tmp_path / "tests").mkdir()
    (tmp_path / "tests" / "test_x.py").write_text(
        "import pytest\n\n\nclass TestK:\n    def test_inside(self):\n        assert True\n\n\n"
        "def test_ok():\n    assert True\n\n\ndef test_bad():\n    assert False\n\n\n"
        "@pytest.mark.parametrize('n', [1, 2])\ndef test_param(n):\n    assert n == 1\n"
    )
    (tmp_path / "tests" / "test_broken.py").write_text(
        "import module_that_does_not_exist\n\n\ndef test_never():\n    pass\n"
    )
    ids = [
        "tests/test_x.py::test_ok",
        "tests/test_x.py::test_bad",
        "tests/test_x.py::TestK::test_inside",
        "tests/test_x.py::test_param[1]",
        "tests/test_x.py::test_param[2]",
        "tests/test_broken.py::test_never",
        "tests/test_x.py::test_absent",
    ]
    tests = [OracleTest(i, Part.R, None) for i in ids]
    outcomes = PytestOracleRunner(Path(sys.executable), SubprocessRunner()).run(tmp_path, tests)
    assert outcomes == {
        ids[0]: Outcome.PASSED,
        ids[1]: Outcome.FAILED,
        ids[2]: Outcome.PASSED,
        ids[3]: Outcome.PASSED,
        ids[4]: Outcome.FAILED,
        ids[5]: Outcome.MISSING,
        ids[6]: Outcome.MISSING,
    }


@pytest.mark.skipif(shutil.which("go") is None, reason="go is not installed")
def test_go_runner_reports_passes_failures_and_build_errors(tmp_path: Path) -> None:
    (tmp_path / "go.mod").write_text("module example.com/tiny\n\ngo 1.21\n")
    (tmp_path / "a.go").write_text("package tiny\n\nfunc One() int { return 1 }\n")
    (tmp_path / "a_test.go").write_text(
        'package tiny\n\nimport "testing"\n\nfunc TestOk(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal("no")\n\t}\n}\n\n'
        'func TestBad(t *testing.T) {\n\tt.Run("sub", func(t *testing.T) { t.Fatal("always") })\n}\n'
    )
    (tmp_path / "sub").mkdir()
    (tmp_path / "sub" / "b.go").write_text("package sub\n\nfunc Broken() int { return undefined }\n")
    (tmp_path / "sub" / "b_test.go").write_text('package sub\n\nimport "testing"\n\nfunc TestNever(t *testing.T) {}\n')
    tests = [OracleTest(i, Part.R, None) for i in (".::TestOk", ".::TestBad", "sub::TestNever", ".::TestAbsent")]
    environment = {
        "GOFLAGS": "-mod=mod",
        "GOPROXY": "off",
        "GOTOOLCHAIN": "local",
        "GOMODCACHE": str(tmp_path / "modcache"),
    }
    outcomes = GoTestOracleRunner(MemoryFloorRunner(SubprocessRunner(), system_free_percent), environment).run(
        tmp_path, tests
    )
    assert outcomes == {
        ".::TestOk": Outcome.PASSED,
        ".::TestBad": Outcome.FAILED,
        "sub::TestNever": Outcome.MISSING,
        ".::TestAbsent": Outcome.MISSING,
    }
