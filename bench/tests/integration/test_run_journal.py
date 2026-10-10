from __future__ import annotations

import json
import sys
import tempfile
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.oracle_runners import PytestOracleRunner
from specbench.adapters.process import SubprocessRunner
from specbench.adapters.run_journal import (
    LISTED_OUTCOMES,
    SCRATCH_PLACEHOLDER,
    TREE_PLACEHOLDER,
    FileRunJournal,
    TranscriptRecorder,
    run_record,
)
from specbench.application.ports import ProcessResult
from specbench.domain.oracle import OracleTest, Outcome, Part

pytestmark = pytest.mark.integration

H_FILE = "tests/spec_h/test_spec_h_fee.py"


def write_tree(root: Path, rate: int) -> Path:
    tree = Path(tempfile.mkdtemp(prefix="specbench-tree-", dir=root))
    (tree / "tests" / "spec_h").mkdir(parents=True)
    (tree / "fee.py").write_text(f"def fee(minutes):\n    return 0 if minutes < 10 else {rate}\n")
    (tree / H_FILE).write_text(
        "import pytest\n\nfrom fee import fee\n\n\n"
        "def test_c1_free_below_ten_minutes():\n    assert fee(9) == 0\n\n\n"
        "@pytest.mark.parametrize('minutes', [10, 60])\ndef test_c2_flat_rate_from_ten_minutes(minutes):\n"
        "    assert fee(minutes) == 5\n"
    )
    return tree


def test_each_run_keeps_its_failing_tests_and_the_runner_output_without_scratch_paths(tmp_path: Path) -> None:
    transcripts = TranscriptRecorder(SubprocessRunner())
    runner = PytestOracleRunner(Path(sys.executable), transcripts)
    tests = [
        OracleTest(f"{H_FILE}::test_c1_free_below_ten_minutes", Part.H, "ACME-PARKFEE-12-1"),
        OracleTest(f"{H_FILE}::test_c2_flat_rate_from_ten_minutes", Part.H, "ACME-PARKFEE-12-2"),
    ]
    path = tmp_path / "ACME-PARKFEE-12" / "verification-runs.json"
    journal = FileRunJournal(path, transcripts)
    good, broken = write_tree(tmp_path, 5), write_tree(tmp_path, 6)
    journal.record("merge", 1, runner.run(good, tests))
    journal.record("violation V2-1", 1, runner.run(broken, tests))

    text = path.read_text(encoding="utf-8")
    merge, violation = json.loads(text)["runs"]
    assert (merge["step"], merge["not_passed"], merge["tests"]) == ("merge", {}, 2)
    assert "stdout_tail" not in merge["invocations"][0]
    assert violation["not_passed"] == {tests[1].test_id: Outcome.FAILED.value}
    assert violation["not_passed_total"] == 1
    (invocation,) = violation["invocations"]
    assert invocation["returncode"] != 0
    assert any("assert 6 == 5" in line for line in invocation["stdout_tail"])
    assert any(SCRATCH_PLACEHOLDER in argument for argument in invocation["argv"])
    assert str(good) not in text and str(broken) not in text and tempfile.gettempdir() not in text


class PanickingGoTest:
    """A `ProcessRunner` standing for `go test` whose test binary panicked inside the tree."""

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        trace = f"panic: runtime error: invalid memory address\n\t{cwd}/queuev2/reader.go:157 +0x4c\nFAIL\n"
        return ProcessResult(1, trace, "")


def test_the_materialized_tree_is_named_by_a_placeholder(tmp_path: Path) -> None:
    tree = Path(tempfile.mkdtemp(prefix="specbench-tree-", dir=tmp_path))
    transcripts = TranscriptRecorder(PanickingGoTest())
    transcripts.run(["go", "test", "-json", "./queuev2"], cwd=tree)
    path = tmp_path / "verification-runs.json"
    FileRunJournal(path, transcripts).record("merge", 1, {"queuev2::TestReaderStartStop": Outcome.MISSING})
    (run,) = json.loads(path.read_text(encoding="utf-8"))["runs"]
    assert f"\t{TREE_PLACEHOLDER}/queuev2/reader.go:157 +0x4c" in run["invocations"][0]["stdout_tail"]


def test_a_run_lists_a_bounded_number_of_tests() -> None:
    outcomes = {f"tests/test_rates.py::test_rate[{n}]": Outcome.FAILED for n in range(LISTED_OUTCOMES + 7)}
    record = run_record("base", 3, outcomes, [])
    assert isinstance(record, dict)
    listed = record["not_passed"]
    assert isinstance(listed, dict) and len(listed) == LISTED_OUTCOMES
    assert record["not_passed_total"] == LISTED_OUTCOMES + 7
