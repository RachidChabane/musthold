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
from specbench.application.construct import TaskConstruction
from specbench.application.construct_run import ConstructionRun, Outcome
from specbench.application.corpus import unseal_task
from specbench.application.paced_calls import CallGate, PacedConstructionModel
from specbench.application.verify_task import TaskVerifier
from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.construction import HAIKU, OPUS, SONNET
from specbench.domain.manifest import RunLayout
from specbench.domain.oracle import oracle_from_bundle
from specbench.domain.pacing import PacingPolicy, RunPaused, RunWindow
from specbench.domain.selection import ProbedTask, Selection
from support.fake_claude import install_fake_claude, recorded_calls
from support.fakes import FixedClock
from support.tiny_upstream import FakeBuilder, candidate, denied_runner, make_upstream, probe_for

pytestmark = pytest.mark.integration

ISSUE_TEXT = "Stays under ten minutes should cost nothing at the barrier."
PULL_BODY = "Closes #3. Short stays are free."
TASK_ID = "ACME-PARKFEE-12"

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
        },
        {
            "number": 2,
            "statement": "WHEN a stay lasts ten minutes or more THE SYSTEM SHALL charge 5",
            "examples": ["fee(minutes=60) => 5"],
            "status": "unchanged",
        },
    ],
}
H_TESTS = (
    "from hypothesis import given, settings, strategies as st\n"
    "from pkg.fee import fee\n\n\n"
    "@settings(derandomize=True, max_examples=30, deadline=None)\n"
    "@given(st.integers(min_value=0, max_value=9))\n"
    "def test_c1_short_stays_are_free(minutes):\n    assert fee(minutes) == 0\n\n\n"
    "@settings(derandomize=True, max_examples=30, deadline=None)\n"
    "@given(st.integers(min_value=10, max_value=1000))\n"
    "def test_c2_longer_stays_cost_five(minutes):\n    assert fee(minutes) == 5\n"
)
CLASSIFICATION = {
    "classifications": [
        {"test_id": "tests/test_fee.py::test_free", "implied": True, "criterion": 1, "reason": "stated by criterion 1"}
    ]
}


def edit(search: str, replace: str) -> dict[str, str]:
    return {"path": "pkg/fee.py", "search": search, "replace": replace}


VIOLATIONS = {
    "violations": [
        {"criterion": 1, "title": "charges one", "edits": [edit("return 0", "return 1")]},
        {"criterion": 1, "title": "short threshold", "edits": [edit("minutes < 10", "minutes < 6")]},
        {"criterion": 2, "title": "charges six", "edits": [edit("return 5", "return 6")]},
        {"criterion": 2, "title": "long threshold", "edits": [edit("minutes < 10", "minutes < 11")]},
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
    {"payload": {"files": [{"path": "tests/spec_h/test_spec_h_fee.py", "content": H_TESTS}]}},
    {"payload": CLASSIFICATION},
    {"payload": CLASSIFICATION},
    {"payload": VIOLATIONS},
    {"payload": REFACTORS},
]
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
        denied = denied_runner()
        mirrors = GitMirrors(self.root / "mirrors", plain)
        sources = MirrorConstructionSources(mirrors, lambda clone: GitHistory(clone, plain), FakeGitHub(), self.work)
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
        )

        def verifier_for(task: ProbedTask) -> TaskVerifier:
            return TaskVerifier(
                GitTreeFactory(mirrors.path_of(task.candidate.repository)),
                PytestOracleRunner(Path(sys.executable), denied),
                UnavailableOperatorEngine(),
            )

        return ConstructionRun(
            sources,
            construction,
            verifier_for,
            self.corpus,
            AgeSealStore(),
            FileStatusStore(layout.root),
            gate.require_open,
        )


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
    assert "oracle/U/tests/test_fee.py" in files and "oracle/H/tests/spec_h/test_spec_h_fee.py" in files
    assert "oracle/O/tests/test_fee.py" not in files
    assignment = json.loads(files["construction/assignment.json"])
    assert assignment["u"] == {"tests/test_fee.py::test_free": 1} and assignment["o"] == []


def test_each_role_sees_only_what_the_design_gives_it(world: World) -> None:
    world.run().run(selection(world.task), 1)
    author, reviewer, _, _, writer, assigner, *_ = (stdin_of(world, i) for i in range(9))
    for text in (author, reviewer, writer):
        assert "test_free" not in text and "assert fee(5)" not in text and "test_needs_network" not in text
    assert ISSUE_TEXT in author and "if minutes < 10" in author and "pkg.fee: def fee(minutes)" in author
    assert "if minutes < 10" not in reviewer and ISSUE_TEXT not in reviewer
    assert "if minutes < 10" not in writer and ISSUE_TEXT not in writer and "ACME-PARKFEE-12-1~1" in writer
    assert "assert fee(5) == 0" in assigner


def test_a_rerun_replays_every_call_from_the_archive(world: World) -> None:
    world.run().run(selection(world.task), 1)
    (world.work / "construction" / TASK_ID / "status.json").unlink()
    again = world.run().run(selection(world.task), 1)
    assert again.sealed == (TASK_ID,)
    assert len(recorded_calls(world.root / "fake")) == 9


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
