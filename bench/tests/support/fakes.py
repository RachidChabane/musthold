"""Fake implementations of the application ports, scripted by the test."""

from __future__ import annotations

from collections.abc import Callable, Generator, Mapping, Sequence
from contextlib import contextmanager
from datetime import datetime
from enum import Enum
from pathlib import Path

from specbench.application.ports import HExecutionRequest, OperatorMutants, TreeSpec
from specbench.domain.arms import Arm, Harness, Host
from specbench.domain.call_ledger import CallLedger
from specbench.domain.canonical import JsonValue
from specbench.domain.gate import GateMode, GateOutcome
from specbench.domain.h_selection import CandidateRuns, HExecution
from specbench.domain.h_tests import HTest
from specbench.domain.oracle import OracleTest, Outcome
from specbench.domain.reservation import ExecutionHistory
from specbench.domain.sessions import CallRecord, HostRequest, HostResponse, SessionRequest, SessionResult
from specbench.domain.verification import VERIFICATION_RUNS


class RecordingTrees:
    """A `TreeFactory` that hands out empty directories and remembers which spec each one stands for."""

    def __init__(self, root: Path) -> None:
        self._root = root
        self.specs: dict[Path, TreeSpec] = {}

    @contextmanager
    def materialize(self, spec: TreeSpec) -> Generator[Path]:
        directory = self._root / f"tree-{len(self.specs)}"
        directory.mkdir()
        self.specs[directory] = spec
        yield directory


Behaviour = Callable[[TreeSpec, OracleTest, int], Outcome]


class ScriptedRunner:
    """An `OracleRunner` whose outcome for (tree spec, test, run index) is computed by the test."""

    def __init__(self, trees: RecordingTrees, behaviour: Behaviour) -> None:
        self._trees = trees
        self._behaviour = behaviour
        self._calls: dict[Path, int] = {}

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]:
        index = self._calls.get(tree, 0)
        self._calls[tree] = index + 1
        spec = self._trees.specs[tree]
        return {t.test_id: self._behaviour(spec, t, index) for t in tests}


class ListJournal:
    """A `RunJournal` that keeps every recorded run in order."""

    def __init__(self) -> None:
        self.runs: list[tuple[str, int, dict[str, Outcome]]] = []

    def record(self, step: str, run: int, outcomes: Mapping[str, Outcome]) -> None:
        self.runs.append((step, run, dict(outcomes)))


class FixedOperatorEngine:
    def __init__(self, mutants: OperatorMutants | None, limitation: str | None = None) -> None:
        self._mutants = mutants
        self._limitation = limitation

    @property
    def limitation(self) -> str | None:
        return self._limitation

    def mutants_of(self, base_commit: str, merge_commit: str) -> OperatorMutants | None:
        return self._mutants


class FixedClock:
    def __init__(self, moment: datetime) -> None:
        self.moment = moment

    def now(self) -> datetime:
        return self.moment


class MemoryCalls:
    def __init__(self) -> None:
        self.records: dict[str, CallRecord] = {}

    def exists(self, call_id: str) -> bool:
        return call_id in self.records

    def write(self, record: CallRecord) -> None:
        self.records[record.call_id] = record

    def read(self, call_id: str) -> CallRecord:
        return self.records[call_id]


class MemoryLedger:
    def __init__(self) -> None:
        self.ledger = CallLedger({})

    def load(self) -> CallLedger:
        return self.ledger

    def save(self, ledger: CallLedger) -> None:
        self.ledger = ledger


class FakeAgentHarness:
    """A scripted coding agent: answers each prompt from a callback and counts its calls."""

    def __init__(self, harness: Harness, answer: Callable[[SessionRequest], SessionResult]) -> None:
        self._harness = harness
        self._answer = answer
        self.calls = 0

    @property
    def harness(self) -> Harness:
        return self._harness

    def run_session(self, request: SessionRequest) -> SessionResult:
        self.calls += 1
        return self._answer(request)


class FakeHostExecutor:
    def __init__(self, host: Host, payloads: Mapping[str, JsonValue]) -> None:
        self._host = host
        self._payloads = payloads
        self.calls = 0

    def execute(self, request: HostRequest) -> HostResponse:
        self.calls += 1
        return HostResponse(request.request_id, self._host, self._payloads[request.request_id])


class FakeGateHost:
    """A scripted `GateHost`: answers each call from a callback and remembers every call it received."""

    def __init__(self, answer: Callable[[Path, Arm, Path, GateMode], GateOutcome]) -> None:
        self._answer = answer
        self.received: list[tuple[Path, Arm, Path, GateMode]] = []

    @classmethod
    def returning(cls, outcome: Callable[[], GateOutcome]) -> FakeGateHost:
        """A host that answers every call from `outcome`, whatever the tree, arm, record directory and mode."""
        return cls(lambda _tree, _arm, _record, _mode: outcome())

    def run_gate(self, tree: Path, arm: Arm, record_dir: Path, mode: GateMode) -> GateOutcome:
        self.received.append((tree, arm, record_dir, mode))
        return self._answer(tree, arm, record_dir, mode)


class MemoryEvents:
    """An `EventStream` the test fills: the events a session streamed to its call directory."""

    def __init__(self) -> None:
        self.streamed: dict[str, tuple[str, ...]] = {}

    def lines(self, call_id: str) -> tuple[str, ...]:
        return self.streamed.get(call_id, ())


class LineCounter:
    """An `ExecutionCounter` that reads one execution per event line."""

    def count(self, events: Sequence[str]) -> int:
        return len(events)


class MemoryHistory:
    def __init__(self) -> None:
        self.history = ExecutionHistory()

    def load(self) -> ExecutionHistory:
        return self.history

    def save(self, history: ExecutionHistory) -> None:
        self.history = history


class Tree(Enum):
    MERGE = "merge"
    BASE = "base"


HBehaviour = Callable[[HTest, Tree, int], Outcome]


def discriminating(test: HTest, tree: Tree, run: int) -> Outcome:
    """A candidate that passes at merge and fails at base, in every run."""
    return Outcome.PASSED if tree is Tree.MERGE else Outcome.FAILED


class ScriptedHExecutor:
    """An `HCandidateExecutor` whose outcome for (candidate, tree, run index) is computed by the test."""

    def __init__(self, behaviour: HBehaviour = discriminating, unbuildable: frozenset[str] = frozenset()) -> None:
        self._behaviour = behaviour
        self._unbuildable = unbuildable
        self.requests: list[HExecutionRequest] = []

    def execute(self, request: HExecutionRequest) -> HExecution:
        self.requests.append(request)
        runs = {
            t.test_id: CandidateRuns(
                tuple(self._behaviour(t, Tree.MERGE, run) for run in range(VERIFICATION_RUNS)),
                ()
                if t.path in self._unbuildable
                else tuple(self._behaviour(t, Tree.BASE, run) for run in range(VERIFICATION_RUNS)),
            )
            for t in request.tests
        }
        return HExecution(runs, frozenset(f.path for f in request.candidates) & self._unbuildable)
