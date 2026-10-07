"""Ports shared by the run side and by `analyze`: the clock, processes, HTTP, version control and the product readers."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Protocol


class Clock(Protocol):
    def now(self) -> datetime:
        """The current local time, without a zone."""
        ...

    def monotonic(self) -> float:
        """Seconds from an arbitrary origin, for latencies."""
        ...


class Sleeper(Protocol):
    def sleep(self, seconds: float) -> None: ...


@dataclass(frozen=True)
class ProcessResult:
    returncode: int
    stdout: bytes
    stderr: bytes


class ProcessTimeout(Exception):
    """A command that outlived its timeout; the process and its children were killed."""


class ProcessRunner(Protocol):
    def run(
        self,
        argv: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        unset: Sequence[str] = (),
        input_bytes: bytes | None = None,
        timeout_seconds: float | None = None,
    ) -> ProcessResult:
        """Runs one command to its end. `env` is laid over the inherited environment and `unset` is removed from it."""
        ...


@dataclass(frozen=True)
class HttpReply:
    status: int
    body: bytes


class TransportFailure(Exception):
    """No HTTP reply came: a refused connection, a reset, a malformed reply or a timeout."""

    def __init__(self, reason: str, *, timed_out: bool = False) -> None:
        super().__init__(reason)
        self.reason = reason
        self.timed_out = timed_out


class HttpTransport(Protocol):
    """HTTP through the standard library; the bytes of the body are what travels, unchanged."""

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply: ...

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply: ...


@dataclass(frozen=True)
class Timestamp:
    """A parsed RFC 3161 reply: the message imprint (hex) and the time the authority stamped, UTC."""

    imprint: str
    generated_at: datetime


class TimestampReader(Protocol):
    def read(self, reply: Path) -> Timestamp | None:
        """The imprint and the genTime of the reply (not its certificate chain); None when it cannot be read."""
        ...


class VersionControl(Protocol):
    """Read access to the repository of the experiment, always through plain `git` and never the working tree."""

    def peel_tag(self, tag: str) -> str | None:
        """The commit a tag peels to, None when the tag does not exist."""
        ...

    def head_commit(self) -> str: ...

    def is_clean(self, paths: Sequence[str]) -> bool:
        """Whether no tracked or untracked file under these paths differs from HEAD."""
        ...

    def archive(
        self, revision: str, pathspecs: Sequence[str], destination: Path, *, exclude: Sequence[str] = ()
    ) -> None:
        """Unpacks `git archive <revision> -- <pathspecs> ':(exclude)<x>'...` into `destination`, which is created."""
        ...

    def is_ancestor(self, ancestor: str, descendant: str) -> bool: ...

    def commit_time(self, commit: str) -> datetime:
        """The committer date of the commit, in UTC."""
        ...

    def commits_touching(self, revision: str, path: str) -> list[str]:
        """The commits reachable from `revision` that changed `path`, oldest first."""
        ...

    def blob(self, revision: str, path: str) -> bytes | None:
        """The content of `path` at `revision`, None when it does not exist there."""
        ...


@dataclass(frozen=True)
class DecodeRequest:
    """One recorded reply to read the way the relay and the module would: `closed` and `reply` are verbatim texts."""

    key: str
    closed: str
    reply: str


@dataclass(frozen=True)
class DecodedAnswer:
    question_id: str
    choice: str
    confidence: float | None
    probabilities: Mapping[str, float]


@dataclass(frozen=True)
class DecodeOutcome:
    """`accepted` is False for a reply the relay refuses (RELAY_REFUSED); `stage` and `error` say where and why."""

    key: str
    accepted: bool
    stage: str | None
    error: str | None
    model: str | None
    answers: tuple[DecodedAnswer, ...]


@dataclass(frozen=True)
class ScreenReadRequest:
    key: str
    document: str
    question: str


@dataclass(frozen=True)
class ScreenReading:
    """`power.ViolationProbability` of a relayed answer, and what the answer document states."""

    key: str
    error: str | None
    choice: str | None
    has_probability: bool
    violates_returned: float
    violation_probability: float


@dataclass(frozen=True)
class EndpointDigest:
    address: str
    digest: str
    source: str


@dataclass(frozen=True)
class ThresholdCheck:
    """A threshold entry applied to a default and resolved per source by `format.Threshold.For`."""

    default: float
    by_source: Mapping[str, float]
    resolved: Mapping[str, float]


@dataclass(frozen=True)
class ProductDefaults:
    """The default thresholds and template digests of the frozen modules, read from their own code."""

    tau_screen: float
    tau_route: float
    screen_template_digest: str
    alignment_template_digest: str


class ProductReader(Protocol):
    """The product's own reading of a recorded reply, a threshold entry and an endpoint address (§4.6, §12)."""

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]: ...

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]: ...

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest: ...

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck: ...

    def defaults(self) -> ProductDefaults: ...


@dataclass(frozen=True)
class MatrixSignature:
    """The pilot's matrix signature of one test and the binary verdict it implies."""

    nodeid: str
    declared: int
    signature: str
    verdict: str
    none_share: float


class PilotMatrix(Protocol):
    """`pilot.domain.matrix` of the snapshot, behind a typed port."""

    def classify(
        self,
        nodeids: Sequence[str],
        declared: Mapping[str, int],
        valid_pool: Sequence[tuple[int, frozenset[str]]],
        unattributed: Sequence[frozenset[str]],
    ) -> dict[str, MatrixSignature]: ...
