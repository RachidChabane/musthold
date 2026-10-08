"""Values exchanged with an agent harness and with a host executor, and the raw record of one model call."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from pathlib import Path

from specbench.domain.arms import Arm, Harness, Host
from specbench.domain.canonical import JsonValue, canonical_json, sha256_hex

CALL_ID_PREFIX = "call-"
CALL_ID_DIGEST_LENGTH = 20
GATE_CALL_ID_PREFIX = "gate-"


class SessionKind(Enum):
    TRUNK = "trunk"
    BRANCH = "branch"
    GATE_HOST = "gate-host"


class RequestKind(Enum):
    MUTANTS = "mutants"
    VALIDITY = "validity"
    EQUIVALENCE = "equivalence"
    ALIGNMENT = "alignment"
    CLOSED_QUESTION = "closed-question"
    COMPLETENESS = "completeness"


@dataclass(frozen=True)
class SessionRequest:
    """One agent session: the frozen model, the prompt, and whether it resumes or forks an earlier one.

    `key` names the call within the run (task, arm, replicate, stop or attempt). Two sessions with the same
    prompt but different keys are different calls; the run directory resumes a call by its identity alone.
    """

    harness: Harness
    model: str
    kind: SessionKind
    key: str
    workdir: Path
    prompt: str
    resume_session_id: str | None = None
    fork: bool = False

    def identity(self) -> dict[str, JsonValue]:
        return {
            "fork": self.fork,
            "harness": self.harness.value,
            "key": self.key,
            "kind": self.kind.value,
            "model": self.model,
            "prompt": self.prompt,
            "resume": self.resume_session_id,
        }

    @property
    def call_id(self) -> str:
        return CALL_ID_PREFIX + sha256_hex(canonical_json(self.identity()).encode())[:CALL_ID_DIGEST_LENGTH]


@dataclass(frozen=True)
class SessionResult:
    session_id: str
    final_message: str
    exit_code: int
    events: tuple[str, ...]
    model_reported: str | None
    stderr: str = ""


@dataclass(frozen=True)
class HostRequest:
    """A request the product emits for its host to execute in an isolated, blind subagent."""

    request_id: str
    kind: RequestKind
    prompt: str
    response_schema: JsonValue


@dataclass(frozen=True)
class HostResponse:
    request_id: str
    host: Host
    payload: JsonValue


@dataclass(frozen=True)
class CallRecord:
    """Everything of one model call that the run directory keeps: argv, stdin, stdout, events and meta."""

    call_id: str
    argv: tuple[str, ...]
    stdin: str
    stdout: str
    events: tuple[str, ...]
    exit_code: int
    session_id: str
    model_reported: str | None


def live_gate_call_id(arm: Arm, record_dir: Path) -> str:
    """The identity of a live gate: its arm and its record directory, which is deterministic per task, arm and tree digest."""
    return GATE_CALL_ID_PREFIX + sha256_hex(f"{arm.value}\n{record_dir.as_posix()}".encode())[:CALL_ID_DIGEST_LENGTH]
