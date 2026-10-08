"""What one gate-host session returns: the Shallnot report, how the session ended, and the executions it spent."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum

from specbench.domain.agent_events import TokenUsage
from specbench.domain.arms import Module
from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import RecordError, enum_of, integer, mapping, optional_text, sequence, text
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

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "kind": self.kind.value,
            "module": self.module.value,
            "request_id": self.request_id,
            "resolved_model": self.resolved_model,
            "tokens": {
                "cache_creation": self.tokens.cache_creation_tokens,
                "cache_read": self.tokens.cache_read_tokens,
                "input": self.tokens.input_tokens,
                "output": self.tokens.output_tokens,
                "reasoning": self.tokens.reasoning_tokens,
            },
        }

    @classmethod
    def from_record(cls, body: Mapping[str, object]) -> TicketUsage:
        tokens = mapping(body.get("tokens"), "ticket tokens")
        return cls(
            text(body.get("request_id"), "ticket request_id"),
            enum_of(RequestKind, body.get("kind"), "ticket kind"),
            enum_of(Module, body.get("module"), "ticket module"),
            TokenUsage(
                integer(tokens.get("input"), "tokens input"),
                integer(tokens.get("output"), "tokens output"),
                integer(tokens.get("cache_read"), "tokens cache_read"),
                integer(tokens.get("cache_creation"), "tokens cache_creation"),
                integer(tokens.get("reasoning"), "tokens reasoning"),
            ),
            optional_text(body.get("resolved_model"), "ticket resolved_model"),
        )


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

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "executions": self.executions,
            "report": self.report,
            "session_id": self.session_id,
            "status": self.status.value,
            "tickets": [ticket.to_record() for ticket in self.tickets],
        }

    @classmethod
    def from_record(cls, body: Mapping[str, JsonValue]) -> GateOutcome:
        return cls(
            body.get("report"),
            enum_of(GateStatus, body.get("status"), "gate status"),
            optional_text(body.get("session_id"), "gate session_id"),
            integer(body.get("executions"), "gate executions"),
            tuple(TicketUsage.from_record(mapping(t, "gate ticket")) for t in sequence(body.get("tickets"), "tickets")),
        )
