"""The whole construction of one task against a real tiny upstream and a fake `claude` that answers from recordings.

Real: git history, the offline probe, the blind inputs, the isolated call adapter, pacing and archiving, the
two-way verification with real pytest runs, sealing with age. Faked: GitHub, the environment builder and the model.
"""

from __future__ import annotations

import json
import sys
from datetime import datetime
from pathlib import Path

import pytest

from specbench.adapters.claude_cli import ClaudeCliModel
from specbench.adapters.construction_sources import MirrorConstructionSources
from specbench.adapters.construction_store import FileStatusStore
from specbench.adapters.file_corpus import FileCorpus
from specbench.adapters.git_history import GitHistory, GitMirrors
from specbench.adapters.git_trees import GitTreeFactory
from specbench.adapters.operator_engine import UnavailableOperatorEngine
from specbench.adapters.oracle_runners import PytestOracleRunner
from specbench.adapters.patch_builder import GitDiffPatchBuilder
from specbench.adapters.process import SubprocessPromptRunner, SubprocessRunner
from specbench.adapters.prompt_files import construction_schema, load_prompts
from specbench.adapters.run_store import FileCallStore, FileFailureLog, FileLedgerStore
from specbench.adapters.schemas import JsonSchemaChecker, SchemaName
from specbench.adapters.sealed_store import AgeSealStore, generate_identity
from specbench.application.construct import DropStage, TaskConstruction
from specbench.application.construct_run import ConstructionRun, Outcome, TaskSettlement, TaskStatus
from specbench.application.corpus import unseal_task
from specbench.application.h_execution import PerTaskHExecutor, PythonIsolation, TreeHCandidateExecutor
from specbench.application.paced_calls import CallGate, PacedConstructionModel
from specbench.application.reopen import reopen
from specbench.application.reverify import reverify
from specbench.application.stabilised_runs import StabilisedRuns
from specbench.application.verify_task import TaskVerifier
from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.construction import HAIKU, OPUS, SONNET
from specbench.domain.gaps import GAP_FILE_NAME
from specbench.domain.h_selection import H_SELECTION_PATH
from specbench.domain.manifest import RunLayout
from specbench.domain.oracle import oracle_from_bundle
from specbench.domain.pacing import PacingPolicy, RunPaused, RunWindow
from specbench.domain.sealing import CURRENT_BUNDLE_VERSION
from specbench.domain.selection import ProbedTask, Selection
from specbench.domain.task import Task
from support.fake_claude import install_fake_claude, recorded_baseline_calls, recorded_calls
from support.fakes import FixedClock
from support.tiny_upstream import FakeBuilder, candidate, denied_runner, make_upstream, probe_for

pytestmark = pytest.mark.integration

ISSUE_TEXT = "Stays under ten minutes should cost nothing at the barrier."
PULL_BODY = "Closes #3. Short stays are free."
TASK_ID = "ACME-PARKFEE-12"
GAP_SEED = 20261007

SPEC = {
    "title": "Parking fee",
    "overview": "The fee charged for one stay, from its length in minutes.",
    "interface": [{"import_path": "pkg.fee", "signature": "fee(minutes: int) -> int", "note": "the fee of a stay"}],
    "criteria": [
        {
            "number": 1,
            "statement": "WHEN a stay lasts less than ten minutes THE SYSTEM SHALL charge nothing",
            "examples": ["fee(minutes=7) => 0"],
            "status": "new",
            "dimension": "boundary",
            "implementingHunks": [{"path": "pkg/fee.py", "startLine": 2, "endLine": 3}],
        },
        {
            "number": 2,
            "statement": "WHEN a stay lasts zero minutes THE SYSTEM SHALL charge nothing",
            "examples": ["fee(minutes=0) => 0"],
            "status": "new",
            "dimension": "boundary",
            "implementingHunks": [{"path": "pkg/fee.py", "startLine": 2, "endLine": 2}],
        },
    ],
}
H_HEADER = "import pytest\nfrom hypothesis import given, settings, strategies as st\nfrom pkg.fee import fee\n\n\n"
H_SHORT_STAYS = H_HEADER + (
    "@settings(derandomize=True, max_examples=30, deadline=None)\n"
    "@given(st.integers(min_value=1, max_value=9))\n"
    "def test_c1_short_stays_are_free(minutes):\n    assert fee(minutes) == 0\n\n\n"
    '@pytest.mark.parametrize("minutes", [1, 9])\n'
    "def test_c1_the_first_and_last_free_minutes(minutes):\n    assert fee(minutes) == 0\n\n\n"
    "def test_c1_five_minutes_are_free():\n    assert fee(5) == 0\n\n\n"
    "def test_c1_ten_minutes_are_free():\n    assert fee(10) == 0\n"
)
H_ZERO = H_HEADER + (
    "@settings(derandomize=True, max_examples=5, deadline=None)\n"
    "@given(st.integers(min_value=0, max_value=0))\n"
    "def test_c2_no_stay_is_ever_charged(minutes):\n    assert fee(minutes) == 0\n\n\n"
    "def test_c2_a_zero_minute_stay_is_free():\n    assert fee(0) == 0\n\n\n"
    "def test_c2_a_zero_minute_stay_costs_no_more_than_one():\n    assert fee(0) <= fee(1)\n\n\n"
    "def test_c2_an_hour_still_costs_five():\n    assert fee(60) == 5\n"
)
H_FILES = [
    {"path": "tests/spec_h/test_spec_h_c1_short_stays.py", "content": H_SHORT_STAYS},
    {"path": "tests/spec_h/test_spec_h_c2_zero.py", "content": H_ZERO},
]
CLASSIFICATION = {
    "classifications": [
        {"test_id": "tests/test_fee.py::test_free", "implied": True, "criterion": 1, "reason": "stated by criterion 1"}
    ]
}


def edit(search: str, replace: str) -> dict[str, str]:
    return {"path": "pkg/fee.py", "search": search, "replace": replace}


VIOLATIONS = {
    "violations": [
        {"criterion": 1, "title": "skips three", "edits": [edit("minutes < 10", "minutes < 10 and minutes != 3")]},
        {"criterion": 1, "title": "short threshold", "edits": [edit("minutes < 10", "minutes < 1")]},
        {"criterion": 2, "title": "zero is charged", "edits": [edit("minutes < 10", "0 < minutes < 10")]},
        {"criterion": 2, "title": "zero is excluded", "edits": [edit("minutes < 10", "minutes < 10 and minutes != 0")]},
    ]
}
REFACTORS = {
    "refactors": [
        {
            "title": "conditional expression",
            "edits": [
                edit("    if minutes < 10:\n        return 0\n    return 5\n", "    return 0 if minutes < 10 else 5\n")
            ],
        },
        {
            "title": "named constant",
            "edits": [
                edit("def fee(minutes):", "FREE_MINUTES = 10\n\n\ndef fee(minutes):"),
                edit("minutes < 10", "minutes < FREE_MINUTES"),
            ],
        },
    ]
}
RESPONSES: list[dict[str, JsonValue]] = [
    {"payload": SPEC},
    {"payload": {"findings": []}},
    {"payload": {"predictions": [{"operation": "fee", "arguments": ["3"]}]}},
    {"payload": {"predictions": [{"operation": "fee", "arguments": ["5"]}]}},
    {"payload": {"files": H_FILES}},
    {"payload": CLASSIFICATION},
    {"payload": CLASSIFICATION},
    {"payload": VIOLATIONS},
    {"payload": REFACTORS},
]
BASELINES_MEASURED = 4
"""One per (model, effort) the run uses: Opus high, Haiku, Sonnet medium and Opus medium."""
EXPECTED_MODELS = [OPUS, HAIKU, SONNET, SONNET, OPUS, OPUS, HAIKU, OPUS, OPUS]


class FakeGitHub:
    def get(self, path: str, params: object = None) -> JsonValue:
        if path.endswith("/pulls/12"):
            return {"title": "feat: free first minutes", "body": PULL_BODY, "labels": []}
        return {"title": "Free short stays", "body": ISSUE_TEXT}


class World:
    """Everything one construction run needs, rebuilt on demand so a second run starts from the same files."""

    def __init__(self, root: Path) -> None:
        self.root = root
        self.upstream, merge = make_upstream(root)
        self.probe = probe_for(root, self.upstream, FakeBuilder())
        outcome = self.probe.probe(candidate(merge))
        assert isinstance(outcome, ProbedTask)
        self.task = outcome
        self.work = root / "work"
        self.corpus = FileCorpus(root / "corpus")
        self.identity, recipient = generate_identity()
        (root / "corpus").mkdir()
        (root / "corpus" / "recipients.txt").write_text(recipient + "\n")
        self.fake = install_fake_claude(root / "fake", RESPONSES)

    def run(self, night: datetime = datetime(2026, 10, 8, 23, 0)) -> ConstructionRun:
        layout = RunLayout(self.work / "construction")
        gate = CallGate(
            Harness.CLAUDE_CODE,
            PacingPolicy(RunWindow.parse("21:00-07:30"), 250),
            FileLedgerStore(layout),
            FixedClock(night),
        )
        model = PacedConstructionModel(
            ClaudeCliModel(SubprocessPromptRunner(lambda: 100), str(self.fake)),
            gate,
            FileCallStore(layout),
            FileFailureLog(layout),
        )
        plain = SubprocessRunner()
        mirrors = GitMirrors(self.root / "mirrors", plain)
        sources = MirrorConstructionSources(
            mirrors, lambda clone: GitHistory(clone, plain), FakeGitHub(), self.work, lambda _task, u, r: (u, r)
        )
        keys = {
            "spec": SchemaName.SPEC,
            "review": SchemaName.REVIEW,
            "h-tests": SchemaName.H_TESTS,
            "violations": SchemaName.VIOLATIONS,
            "refactors": SchemaName.REFACTORS,
            "assignment": SchemaName.ASSIGNMENT,
            "probe": SchemaName.PROBE,
        }
        construction = TaskConstruction(
            model,
            load_prompts(),
            {key: construction_schema(name) for key, name in keys.items()},
            JsonSchemaChecker(),
            GitDiffPatchBuilder(plain),
            GAP_SEED,
            self.h_executor(),
        )
        return ConstructionRun(sources, construction, self.settlement(), self.statuses(), gate.require_open)

    def h_executor(self) -> PerTaskHExecutor:
        """H candidates run as construction runs them: the task's git trees, real pytest, network denied."""
        mirrors = GitMirrors(self.root / "mirrors", SubprocessRunner())

        def executor_for(task: Task) -> TreeHCandidateExecutor:
            trees = GitTreeFactory(mirrors.path_of(task.repository))
            runs = StabilisedRuns(trees, PytestOracleRunner(Path(sys.executable), denied_runner()))
            return TreeHCandidateExecutor(runs, PythonIsolation())

        return PerTaskHExecutor(executor_for)

    def statuses(self) -> FileStatusStore:
        return FileStatusStore(self.work / "construction")

    def settlement(self) -> TaskSettlement:
        mirrors = GitMirrors(self.root / "mirrors", SubprocessRunner())

        def verifier_for(task: Task) -> TaskVerifier:
            return TaskVerifier(
                GitTreeFactory(mirrors.path_of(task.repository)),
                PytestOracleRunner(Path(sys.executable), denied_runner()),
                UnavailableOperatorEngine(),
            )

        return TaskSettlement(verifier_for, self.corpus, AgeSealStore(), self.statuses())


@pytest.fixture
def world(tmp_path: Path) -> World:
    return World(tmp_path)


def selection(task: ProbedTask) -> Selection:
    return Selection((task,), (), ())


def stdin_of(world: World, index: int) -> str:
    return recorded_calls(world.root / "fake")[index]["stdin"]


def test_one_task_is_constructed_verified_and_sealed(world: World) -> None:
    summary = world.run().run(selection(world.task), 1)
    assert summary.sealed == (TASK_ID,) and summary.dropped == (), summary
    assert _models(world) == EXPECTED_MODELS

    unsealed = unseal_task(world.corpus, AgeSealStore(), world.identity, TASK_ID)
    assert unsealed.task.is_constructed and [c.criterion_id for c in unsealed.task.criteria] == [
        f"{TASK_ID}-1",
        f"{TASK_ID}-2",
    ]
    files = unsealed.bundle.files
    assert b"ACME-PARKFEE-12-1~1: WHEN a stay lasts less than ten minutes" in files["SPEC.md"]
    oracle = oracle_from_bundle(unsealed.bundle)
    assert {t.part.value for t in oracle.tests} == {"U", "R", "H"}
    assert len(oracle.violations) == 4 and len(oracle.planted) == 2
    assert "oracle/U/tests/test_fee.py" in files and "oracle/H/tests/spec_h/test_spec_h_c1_short_stays.py" in files
    assert "oracle/O/tests/test_fee.py" not in files
    assignment = json.loads(files["construction/assignment.json"])
    assert assignment["u"] == {"tests/test_fee.py::test_free": 1} and assignment["o"] == []
    assert unsealed.bundle.schema_version == CURRENT_BUNDLE_VERSION
    assert json.loads(files[GAP_FILE_NAME]) == {"criteria": [f"{TASK_ID}-1", f"{TASK_ID}-2"], "seed": GAP_SEED}
    assert json.loads(files["construction/spec.json"])["criteria"][1]["implementingHunks"] == [
        {"endLine": 2, "path": "pkg/fee.py", "startLine": 2}
    ]


def test_h_is_selected_by_real_runs_at_merge_and_base_and_sealed_exactly_as_selected(world: World) -> None:
    world.run().run(selection(world.task), 1)
    files = unseal_task(world.corpus, AgeSealStore(), world.identity, TASK_ID).bundle.files
    record = json.loads(files[H_SELECTION_PATH])
    verdicts = {c["id"].split("::")[1]: c["verdict"] for c in record["candidates"]}
    assert verdicts["test_c1_ten_minutes_are_free"] == "rejected-merge"
    assert verdicts["test_c2_an_hour_still_costs_five"] == "rejected-base-pass"
    assert verdicts["test_c1_short_stays_are_free"] == verdicts["test_c2_no_stay_is_ever_charged"] == "kept"
    assert [c["exposed"] for c in record["criteria"]] == [True, False]
    assert record["divergences"] == ["tests/spec_h/test_spec_h_c1_short_stays.py::test_c1_ten_minutes_are_free"]
    manifest = json.loads(files["oracle/manifest.json"])
    assert {m["id"] for m in manifest if m["part"] == "H"} == {
        c["id"] for c in record["candidates"] if c["verdict"] == "kept"
    }
    sealed = files["oracle/H/tests/spec_h/test_spec_h_c1_short_stays.py"].decode()
    assert "test_c1_ten_minutes_are_free" not in sealed and "@settings(derandomize=True" in sealed
    assert files["construction/h-candidates/tests/spec_h/test_spec_h_c1_short_stays.py"].decode() == H_SHORT_STAYS


def test_a_reopened_task_is_built_again_from_the_archive_and_its_last_attempt_is_kept(world: World) -> None:
    world.run().run(selection(world.task), 1)
    world.statuses().write(TaskStatus(TASK_ID, Outcome.DROPPED, DropStage.VERIFICATION.value, "stale", None))
    aside = Path(reopen(TASK_ID, world.statuses()))
    assert (aside / "status.json").is_file() and (aside / "plain").is_dir()
    assert world.statuses().read(TASK_ID) is None
    again = world.run().run(selection(world.task), 1)
    assert again.sealed == (TASK_ID,)
    assert len(recorded_calls(world.root / "fake")) == len(RESPONSES)


def test_each_role_sees_only_what_the_design_gives_it(world: World) -> None:
    world.run().run(selection(world.task), 1)
    author, reviewer, _, _, writer, assigner, *_ = (stdin_of(world, i) for i in range(9))
    for text in (author, reviewer, writer):
        assert "test_free" not in text and "assert fee(5)" not in text and "test_needs_network" not in text
    assert ISSUE_TEXT in author and "if minutes < 10" in author and "pkg.fee: def fee(minutes)" in author
    assert "if minutes < 10" in reviewer and ISSUE_TEXT not in reviewer
    assert "if minutes < 10" not in writer and ISSUE_TEXT not in writer and "ACME-PARKFEE-12-1~1" in writer
    assert "assert fee(5) == 0" in assigner


def test_a_rerun_replays_every_call_from_the_archive(world: World) -> None:
    world.run().run(selection(world.task), 1)
    (world.work / "construction" / TASK_ID / "status.json").unlink()
    again = world.run().run(selection(world.task), 1)
    assert again.sealed == (TASK_ID,)
    assert len(recorded_calls(world.root / "fake")) == 9
    assert len(recorded_baseline_calls(world.root / "fake")) == BASELINES_MEASURED


def test_a_task_dropped_at_verification_is_settled_again_without_a_model_call(world: World) -> None:
    world.run().run(selection(world.task), 1)
    stale = "merge-passes-all: tests/spec_h/test_spec_h_c1_short_stays.py::test_c1_five_minutes_are_free (H) is missing"
    world.statuses().write(TaskStatus(TASK_ID, Outcome.DROPPED, DropStage.VERIFICATION.value, stale, None))
    calls = len(recorded_calls(world.root / "fake"))

    settled = reverify(TASK_ID, world.statuses(), world.settlement())

    assert settled.status.outcome is Outcome.SEALED and world.statuses().read(TASK_ID) == settled.status
    assert len(recorded_calls(world.root / "fake")) == calls
    assert unseal_task(world.corpus, AgeSealStore(), world.identity, TASK_ID).task == settled.task


def test_a_closed_window_pauses_before_any_call_and_leaves_no_status(world: World) -> None:
    with pytest.raises(RunPaused):
        world.run(night=datetime(2026, 10, 8, 12, 0)).run(selection(world.task), 1)
    assert recorded_calls(world.root / "fake") == []
    assert FileStatusStore(world.work / "construction").read(TASK_ID) is None


def test_a_spec_whose_example_spells_a_human_test_input_is_fixed_once_then_dropped(world: World) -> None:
    leaking = json.loads(json.dumps(SPEC))
    leaking["criteria"][0]["examples"] = ["fee(minutes=5) => 0"]
    install_fake_claude(
        world.root / "fake", [{"payload": leaking}, {"payload": {"findings": []}}, {"payload": leaking}]
    )
    summary = world.run().run(selection(world.task), 1)
    assert summary.sealed == ()
    ((task_id, reason),) = summary.dropped
    assert task_id == TASK_ID and reason.startswith("leak:") and "n:5" in reason
    assert _models(world) == [OPUS, HAIKU, OPUS]


def _models(world: World) -> list[str]:
    models: list[str] = []
    for call in recorded_calls(world.root / "fake"):
        argv = call["argv"]
        models.append(str(argv[argv.index("--model") + 1]))
    return models


def test_the_outcome_enum_names_sealed_and_dropped() -> None:
    assert {o.value for o in Outcome} == {"sealed", "dropped"}
