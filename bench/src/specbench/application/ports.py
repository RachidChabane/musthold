"""Ports of the benchmark: every dependency of a use case that touches the outside world."""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from contextlib import AbstractContextManager
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path
from typing import Protocol

from specbench.domain.arms import Arm, Harness
from specbench.domain.call_ledger import CallLedger
from specbench.domain.candidates import Candidate
from specbench.domain.canonical import JsonValue
from specbench.domain.construction import ModelRequest, ModelResponse
from specbench.domain.gate import GateMode, GateOutcome
from specbench.domain.languages import Language
from specbench.domain.manifest import Provenance
from specbench.domain.mining import BaseSnapshot, PullRequest, PullSummary, Rejection, Repository
from specbench.domain.night_budget import NightJob
from specbench.domain.oracle import OracleTest, Outcome
from specbench.domain.reservation import ExecutionHistory
from specbench.domain.sealing import CorpusLock, SealedBundle
from specbench.domain.selection import ProbedTask
from specbench.domain.sessions import CallRecord, SessionRequest, SessionResult


class GhClient(Protocol):
    """Read access to the GitHub API: `gh api` in production, a recording in dry runs."""

    def get(self, path: str, params: Mapping[str, str] | None = None) -> JsonValue: ...


class MiningSource(Protocol):
    """The facts of the corpus filters, read from GitHub in production and from a recording in dry runs."""

    def repositories(self, language: Language) -> Sequence[Repository]: ...

    def merged_pull_summaries(self, repository: Repository, since: date) -> Sequence[PullSummary]: ...

    def pull_request(self, repository: Repository, number: int) -> PullRequest | None: ...

    def base_snapshot(self, repository: Repository, base_sha: str) -> BaseSnapshot: ...


class Clock(Protocol):
    def now(self) -> datetime: ...


@dataclass(frozen=True)
class ProcessResult:
    returncode: int
    stdout: str
    stderr: str


class ProcessRunner(Protocol):
    def run(
        self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None
    ) -> ProcessResult: ...


class PromptRunner(Protocol):
    """Runs a command that reads its prompt from standard input."""

    def run_with_input(
        self, argv: Sequence[str], stdin: str, cwd: Path, env: Mapping[str, str] | None = None
    ) -> ProcessResult: ...


class SealStore(Protocol):
    """Encrypts a bundle to the corpus recipients and decrypts it with an identity that lives outside the repository."""

    def seal(self, bundle: SealedBundle, recipients: Sequence[str]) -> bytes: ...

    def unseal(self, ciphertext: bytes, identity: str) -> SealedBundle: ...


class CorpusRepository(Protocol):
    """The corpus on disk: `<language>/<task-id>/{task.json,sealed.age}`, `corpus.lock.json` and the recipients."""

    def task_ids(self) -> list[str]: ...

    def read_task_json(self, task_id: str) -> str: ...

    def read_sealed(self, task_id: str) -> bytes: ...

    def write_task(self, language: Language, task_id: str, task_json: str, ciphertext: bytes) -> None: ...

    def read_lock(self) -> CorpusLock: ...

    def write_lock(self, lock: CorpusLock) -> None: ...

    def read_recipients(self) -> list[str]: ...


@dataclass(frozen=True)
class EnvRequest:
    """A tree to build an environment for; `tooling` names Python packages the oracle needs beyond the lockfile,
    `python` the interpreter of a Python environment."""

    language: Language
    source_tree: Path
    destination: Path
    tooling: tuple[str, ...] = ()
    python: Path | None = None


@dataclass(frozen=True)
class BuiltEnvironment:
    language: Language
    destination: Path
    lockfile_digests: Mapping[str, str]
    toolchain_version: str


class EnvBuilder(Protocol):
    """Builds a native per-task environment from the lockfiles of a source tree."""

    def build(self, request: EnvRequest) -> BuiltEnvironment: ...


@dataclass(frozen=True)
class TreeSpec:
    """A tree to materialize: a commit, patches applied in order, and files overlaid on top."""

    commit: str
    patches: tuple[bytes, ...] = ()
    overlay: Mapping[str, bytes] | None = None


class TreeFactory(Protocol):
    def materialize(self, spec: TreeSpec) -> AbstractContextManager[Path]: ...


class TreeReader(Protocol):
    """Reads a materialized tree as files by relative path."""

    def read(self, root: Path) -> Mapping[str, bytes]: ...


class OracleRunner(Protocol):
    """Runs oracle tests in a tree and reports one outcome per requested test."""

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]: ...


class SuiteRunner(Protocol):
    """Runs every test of a tree and reports one outcome per test that ran."""

    def run_all(self, tree: Path) -> Mapping[str, Outcome]: ...


@dataclass(frozen=True)
class OperatorMutants:
    mutants: Mapping[str, TreeSpec]


class OperatorMutantEngine(Protocol):
    """Operator mutants of the touched lines of a merge diff, from the product's engine used as a black box."""

    @property
    def limitation(self) -> str | None: ...

    def mutants_of(self, base_commit: str, merge_commit: str) -> OperatorMutants | None: ...


class AgentHarness(Protocol):
    """Strategy: one coding-agent CLI at its frozen model."""

    @property
    def harness(self) -> Harness: ...

    def run_session(self, request: SessionRequest) -> SessionResult: ...


class GateHost(Protocol):
    """Strategy: one gate-host session that runs `prove-all` on a tree in a clean room and returns the Shallnot report.

    `record_dir` receives (`LIVE`) or supplies (`REPLAY`) the recorded responses, so a gate that was killed resumes from
    the tickets still pending.
    """

    def run_gate(self, tree: Path, arm: Arm, record_dir: Path, mode: GateMode) -> GateOutcome: ...


@dataclass(frozen=True)
class GateSessionSpec:
    """One gate-host session to start: the command, its whole environment, and where it runs."""

    argv: tuple[str, ...]
    env: Mapping[str, str]
    cwd: Path


@dataclass(frozen=True)
class GateSessionEnd:
    exit_code: int
    stderr: str = ""


EventSink = Callable[[str], bool]
"""Receives each event line of a running session as it is streamed; returns `False` to have the session stopped."""


class GateSessionRunner(Protocol):
    """Starts a gate-host session and streams its event lines to the sink until it ends or the sink stops it."""

    def run(self, spec: GateSessionSpec, on_event: EventSink) -> GateSessionEnd: ...


class FeasibilityProbe(Protocol):
    """Establishes that a candidate builds, runs its tests offline and has a fail-to-pass set, or says why not."""

    def probe(self, candidate: Candidate) -> ProbedTask | Rejection: ...


class ConstructionModel(Protocol):
    """One isolated single-shot model call whose role fixes the model."""

    def call(self, request: ModelRequest) -> ModelResponse: ...


class IsolatedModelRunner(Protocol):
    """The live runs behind a `ConstructionModel`: a call is held to a token bound built on the baseline of its model
    and effort, and the baseline is itself one trivial call that must stay under a ceiling."""

    def call(self, request: ModelRequest, baseline_input_tokens: int) -> ModelResponse: ...

    def measure_baseline(self, request: ModelRequest) -> ModelResponse: ...


class FailureLog(Protocol):
    """Where a call that produced no usable result is noted, so a rerun and an audit can see it."""

    def append(self, call_id: str, key: str, message: str) -> None: ...


class SchemaChecker(Protocol):
    """Validates a model's structured output against the JSON schema the call carried."""

    def problems(self, schema: JsonValue, document: JsonValue) -> list[str]: ...


class PatchBuilder(Protocol):
    """Turns the before and after text of files into a unified diff that `git apply` accepts."""

    def build(self, before: Mapping[str, str], after: Mapping[str, str]) -> bytes: ...


class CallStore(Protocol):
    """The raw records of model calls in the run directory; an existing record makes a call resumable."""

    def exists(self, call_id: str) -> bool: ...

    def write(self, record: CallRecord) -> None: ...

    def read(self, call_id: str) -> CallRecord: ...


class LedgerStore(Protocol):
    def load(self) -> CallLedger: ...

    def save(self, ledger: CallLedger) -> None: ...


class ExecutionHistoryStore(Protocol):
    """The executions that completed gates spent, which size the reservation of the next one."""

    def load(self) -> ExecutionHistory: ...

    def save(self, history: ExecutionHistory) -> None: ...


class EventStream(Protocol):
    """The events a session streamed to its call directory, which survive a session that was killed."""

    def lines(self, call_id: str) -> tuple[str, ...]: ...


class ExecutionCounter(Protocol):
    """Counts the model executions a harness's stream shows: the session plus every subagent it started."""

    def count(self, events: Sequence[str]) -> int: ...


class NightLedgers(Protocol):
    """The ledger of each job that spends a harness's nightly cap; a harness with no ledger of its own reads as empty."""

    def read(self, job: NightJob, harness: Harness) -> CallLedger: ...


class ConstructionProgress(Protocol):
    def has_unsealed_task(self) -> bool:
        """Whether a language has fewer sealed tasks than the selection chose for it."""
        ...


class CanaryReserveStore(Protocol):
    """The executions the canaries measured per harness, recorded after a canary night."""

    def measured(self, harness: Harness) -> int | None: ...

    def record(self, harness: Harness, executions: int) -> None: ...


class FileTreeFactory(Protocol):
    """Materializes a tree from its files alone, with no history behind it."""

    def materialize_files(self, files: Mapping[str, bytes]) -> AbstractContextManager[Path]: ...


class ProvenanceSource(Protocol):
    """Reads what a run is about to execute: the product artifacts, the Shallnot pin and the coding-agent CLIs."""

    def read(self) -> Provenance: ...
