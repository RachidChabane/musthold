"""What a harness stream says an agent did, in one vocabulary for every harness."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum


class ActionKind(Enum):
    COMMAND = "command"
    FILE_ACCESS = "file-access"
    TOOL_CALL = "tool-call"


@dataclass(frozen=True)
class EventPointer:
    """Where an action sits in its stream: the zero-based line, and the harness's own id of the call if it has one."""

    line: int
    call_id: str | None = None


@dataclass(frozen=True)
class AgentAction:
    """One thing the agent asked its harness to do.

    `scope` is the id of the subagent call the action belongs to, or `None` for the main session. `command` is set
    for a shell command, `paths` for the files a file tool touches.
    """

    pointer: EventPointer
    kind: ActionKind
    tool: str
    command: str = ""
    paths: tuple[str, ...] = ()
    scope: str | None = None


class HarnessOutcome(Enum):
    OK = "ok"
    HARNESS_MISCONFIGURED = "harness_misconfigured"


@dataclass(frozen=True)
class HarnessCheck:
    """The verdict on a session's start: the plugins and servers the arm needs and the ones that are missing, and why the
    CLI that ran is not the pinned one."""

    outcome: HarnessOutcome
    missing: tuple[str, ...] = ()
    version_fault: str | None = None


@dataclass(frozen=True)
class TokenUsage:
    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_tokens: int = 0
    cache_creation_tokens: int = 0
    reasoning_tokens: int = 0

    def __add__(self, other: TokenUsage) -> TokenUsage:
        return TokenUsage(
            self.input_tokens + other.input_tokens,
            self.output_tokens + other.output_tokens,
            self.cache_read_tokens + other.cache_read_tokens,
            self.cache_creation_tokens + other.cache_creation_tokens,
            self.reasoning_tokens + other.reasoning_tokens,
        )


@dataclass(frozen=True)
class ModelUsage:
    """Tokens a model spent in one session, and what the harness says they cost (notional)."""

    model: str
    usage: TokenUsage
    cost_usd: float | None
