"""Fake implementations of the application ports, scripted by the test."""

from __future__ import annotations

from collections.abc import Callable, Generator, Mapping, Sequence
from contextlib import contextmanager
from datetime import datetime
from pathlib import Path

from specbench.application.ports import OperatorMutants, TreeSpec
from specbench.domain.arms import Harness, Host
from specbench.domain.canonical import JsonValue
from specbench.domain.oracle import OracleTest, Outcome
from specbench.domain.pacing import CallLedger
from specbench.domain.sessions import CallRecord, HostRequest, HostResponse, SessionRequest, SessionResult


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
