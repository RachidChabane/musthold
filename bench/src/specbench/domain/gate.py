"""What one gate-host session returns: the Shallnot report, how the session ended, and the executions it spent."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from specbench.domain.agent_events import TokenUsage
from specbench.domain.arms import Module
from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import RecordError
from specbench.domain.sessions import RequestKind


class GateMode(Enum):
    """`LIVE` runs the gate-host session; `REPLAY` re-reads recorded responses and makes no model call."""

    LIVE = "live"
    REPLAY = "replay"


class GateStatus(Enum):
    COMPLETE = "complete"
    PAUSED = "paused"
    HARNESS_MISCONFIGURED = "harness_misconfigured"


@dataclass(frozen=True)
class TicketUsage:
    """What one ticket of the gate cost: joined to the session's Agent calls through the rewritten prompt `ticket <t>`."""

    request_id: str
    kind: RequestKind
    module: Module
    tokens: TokenUsage
    resolved_model: str | None


@dataclass(frozen=True)
class GateOutcome:
    """The result of `GateHost.run_gate`.

    `report` is the parsed Shallnot report and is present exactly when the gate is complete. `executions` counts the
    model executions the session spent, the session itself included; a replay spends none.
    """

    report: JsonValue | None
    status: GateStatus
    session_id: str | None
    executions: int
    tickets: tuple[TicketUsage, ...]

    def __post_init__(self) -> None:
        if self.executions < 0:
            raise RecordError("a gate cannot spend a negative number of executions")
        if (self.report is not None) != (self.status is GateStatus.COMPLETE):
            raise RecordError("a gate has a report exactly when it is complete")
