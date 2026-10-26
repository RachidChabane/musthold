from __future__ import annotations

import shutil
import sys
from pathlib import Path

import pytest

from specbench.adapters.git_trees import GitTreeFactory, TreeBuildError
from specbench.adapters.operator_engine import UnavailableOperatorEngine
from specbench.adapters.oracle_runners import GoTestOracleRunner, PytestOracleRunner
from specbench.adapters.process import MemoryFloorRunner, SubprocessRunner, system_free_percent
from specbench.application.ports import TreeSpec
from specbench.application.verify_task import TaskVerifier
from specbench.domain.oracle import OracleTest, Outcome, Part, oracle_from_bundle
from specbench.domain.verification import CheckName, CheckStatus
from support.upstream import GOOD_V1, PARAMETRIZED_H, STRAY_V1, bundle, make_upstream, patch, task

pytestmark = pytest.mark.integration


@pytest.fixture
def upstream(tmp_path: Path) -> tuple[Path, str, str]:
    return make_upstream(tmp_path)


def verifier(repo: Path) -> TaskVerifier:
    runner = MemoryFloorRunner(SubprocessRunner(), system_free_percent)
    return TaskVerifier(
        GitTreeFactory(repo), PytestOracleRunner(Path(sys.executable), runner), UnavailableOperatorEngine()
    )


def test_a_real_task_is_verified_end_to_end(upstream: tuple[Path, str, str]) -> None:
    repo, base, merge = upstream
    sealed = bundle(GOOD_V1)
    report = verifier(repo).verify(task(base, merge), oracle_from_bundle(sealed), sealed)
    assert report.verified, report.to_record()
    assert report.runs == 3
    survivors = next(c for c in report.checks if c.name is CheckName.OPERATOR_SURVIVORS)
    assert survivors.status is CheckStatus.NOT_EVALUATED
    assert report.limitation is not None and "musthold-dp" in report.limitation


def test_a_parametrized_h_test_named_without_parameters_is_verified_end_to_end(
    upstream: tuple[Path, str, str],
) -> None:
    repo, base, merge = upstream
    sealed = bundle(GOOD_V1, PARAMETRIZED_H)
    report = verifier(repo).verify(task(base, merge), oracle_from_bundle(sealed), sealed)
    assert report.verified, report.to_record()


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
        "@pytest.mark.parametrize('n', [1, 2])\ndef test_param(n):\n    assert n == 1\n\n\n"
        "@pytest.mark.parametrize('n', [1, 2])\ndef test_all_cases_pass(n):\n    assert n > 0\n"
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
        "tests/test_x.py::test_param",
        "tests/test_x.py::test_all_cases_pass",
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
        ids[7]: Outcome.FAILED,
        ids[8]: Outcome.PASSED,
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


@pytest.mark.skipif(shutil.which("go") is None, reason="go is not installed")
def test_go_runner_reports_each_test_after_one_that_panics_on_its_own(tmp_path: Path) -> None:
    (tmp_path / "go.mod").write_text("module example.com/blocklist\n\ngo 1.21\n")
    (tmp_path / "db.go").write_text(
        "package blocklist\n\ntype DB struct{ index map[string]bool }\n\n"
        "func New(names ...string) *DB {\n\tif len(names) == 0 {\n\t\treturn nil\n\t}\n"
        "\tdb := &DB{index: map[string]bool{}}\n\tfor _, n := range names {\n\t\tdb.index[n] = true\n\t}\n\treturn db\n}\n\n"
        "func (d *DB) Match(name string) bool { return d.index[name] }\n"
    )
    (tmp_path / "db_test.go").write_text(
        'package blocklist\n\nimport "testing"\n\n'
        'func TestH6_EmptyListMatchesNothing(t *testing.T) {\n\tif New().Match("example.com") {\n\t\tt.Fatal("matched")\n\t}\n}\n\n'
        'func TestH7_ListedNameMatches(t *testing.T) {\n\tif !New("example.com").Match("example.com") {\n\t\tt.Fatal("no match")\n\t}\n}\n\n'
        'func TestH8_OtherNameDoesNotMatch(t *testing.T) {\n\tif New("example.com").Match("example.org") {\n\t\tt.Fatal("matched")\n\t}\n}\n'
    )
    ids = (".::TestH6_EmptyListMatchesNothing", ".::TestH7_ListedNameMatches", ".::TestH8_OtherNameDoesNotMatch")
    environment = {"GOFLAGS": "-mod=mod", "GOPROXY": "off", "GOTOOLCHAIN": "local", "GOMODCACHE": str(tmp_path / "mc")}
    outcomes = GoTestOracleRunner(MemoryFloorRunner(SubprocessRunner(), system_free_percent), environment).run(
        tmp_path, [OracleTest(i, Part.H, None) for i in ids]
    )
    assert outcomes == {ids[0]: Outcome.FAILED, ids[1]: Outcome.PASSED, ids[2]: Outcome.PASSED}
