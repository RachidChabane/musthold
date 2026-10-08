from __future__ import annotations

from pathlib import Path

import pytest

from specbench.application.ports import GateHost
from specbench.domain.agent_events import TokenUsage
from specbench.domain.arms import Arm, Module
from specbench.domain.gate import GateMode, GateOutcome, GateStatus, TicketUsage
from specbench.domain.jsonread import RecordError
from specbench.domain.sessions import RequestKind
from support.fakes import FakeGateHost

TICKET = TicketUsage("req-1", RequestKind.VALIDITY, Module.DISCRIMINATING_POWER, TokenUsage(input_tokens=10), None)


def complete(executions: int) -> GateOutcome:
    return GateOutcome({"verdict": "pass"}, GateStatus.COMPLETE, "session-1", executions, (TICKET,))


def test_a_complete_gate_carries_its_report_and_a_paused_one_does_not() -> None:
    assert complete(78).report == {"verdict": "pass"}
    assert GateOutcome(None, GateStatus.PAUSED, "session-1", 12, ()).report is None
    assert GateOutcome(None, GateStatus.HARNESS_MISCONFIGURED, None, 1, ()).session_id is None


@pytest.mark.parametrize(
    ("report", "status"),
    [(None, GateStatus.COMPLETE), ({"verdict": "pass"}, GateStatus.PAUSED), ({}, GateStatus.HARNESS_MISCONFIGURED)],
)
def test_a_report_exists_exactly_when_the_gate_is_complete(report: dict[str, str] | None, status: GateStatus) -> None:
    with pytest.raises(RecordError):
        GateOutcome(report, status, None, 0, ())


def test_a_gate_cannot_spend_a_negative_number_of_executions() -> None:
    with pytest.raises(RecordError):
        complete(-1)


def test_the_fake_host_satisfies_the_port_and_records_what_it_was_asked() -> None:
    fake = FakeGateHost(lambda _tree, _arm, _record, mode: complete(0 if mode is GateMode.REPLAY else 40))
    host: GateHost = fake
    tree, record = Path("clean-room"), Path("clean-room-rec")
    assert host.run_gate(tree, Arm.ALL, record, GateMode.LIVE).executions == 40
    assert host.run_gate(tree, Arm.ALL, record, GateMode.REPLAY).executions == 0
    assert [mode for *_, mode in fake.received] == [GateMode.LIVE, GateMode.REPLAY]
