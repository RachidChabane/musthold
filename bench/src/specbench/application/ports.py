"""Ports of the benchmark: every dependency of a use case that touches the outside world."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from contextlib import AbstractContextManager
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Protocol

from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.languages import Language
from specbench.domain.oracle import OracleTest, Outcome
from specbench.domain.pacing import CallLedger
from specbench.domain.sealing import SealedBundle
from specbench.domain.sessions import CallRecord, HostRequest, HostResponse, SessionRequest, SessionResult


class GhClient(Protocol):
    """Read access to the GitHub API: `gh api` in production, a recording in dry runs."""

    def get(self, path: str, params: Mapping[str, str] | None = None) -> JsonValue: ...


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


class SealStore(Protocol):
    """Encrypts a bundle to the corpus recipients and decrypts it with an identity that lives outside the repository."""

    def seal(self, bundle: SealedBundle, recipients: Sequence[str]) -> bytes: ...

    def unseal(self, ciphertext: bytes, identity: str) -> SealedBundle: ...


@dataclass(frozen=True)
class EnvRequest:
    language: Language
    source_tree: Path
    destination: Path


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


class OracleRunner(Protocol):
    """Runs oracle tests in a tree and reports one outcome per requested test."""

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]: ...


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


class HostExecutor(Protocol):
    """Strategy: executes a product request in an isolated, blind subagent and returns its response."""

    def execute(self, request: HostRequest) -> HostResponse: ...


class CallStore(Protocol):
    """The raw records of model calls in the run directory; an existing record makes a call resumable."""

    def exists(self, call_id: str) -> bool: ...

    def write(self, record: CallRecord) -> None: ...

    def read(self, call_id: str) -> CallRecord: ...


class LedgerStore(Protocol):
    def load(self) -> CallLedger: ...

    def save(self, ledger: CallLedger) -> None: ...
