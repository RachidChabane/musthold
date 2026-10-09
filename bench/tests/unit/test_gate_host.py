from __future__ import annotations

from pathlib import Path

import pytest

from specbench.application.ports import GateHost
from specbench.domain.agent_events import TokenUsage
from specbench.domain.arms import Arm, Module
from specbench.domain.gate import GateMode, GateOutcome, GatePause, GateStatus, TicketUsage
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


def test_a_paused_gate_can_wait_for_a_ci_round_trip_and_a_complete_one_cannot() -> None:
    waiting = GateOutcome(None, GateStatus.PAUSED, "session-1", 12, (), ci_rounds=1, pause=GatePause.CI_ROUND_PENDING)
    assert waiting.pause is GatePause.CI_ROUND_PENDING
    with pytest.raises(RecordError):
        GateOutcome({"verdict": "pass"}, GateStatus.COMPLETE, None, 0, (), pause=GatePause.CI_ROUND_PENDING)


def test_the_local_readout_belongs_to_a_complete_gate_only() -> None:
    both = GateOutcome({"verdict": "pass"}, GateStatus.COMPLETE, None, 0, (), local_report={"verdict": "fail"})
    assert both.local_report == {"verdict": "fail"}
    with pytest.raises(RecordError):
        GateOutcome(None, GateStatus.PAUSED, None, 0, (), local_report={"verdict": "fail"})


def test_a_gate_cannot_rest_on_a_negative_number_of_ci_rounds() -> None:
    with pytest.raises(RecordError):
        GateOutcome(None, GateStatus.PAUSED, None, 0, (), ci_rounds=-1)


def test_the_new_fields_survive_a_record_round_trip_and_an_older_record_reads_without_them() -> None:
    outcome = GateOutcome(
        {"verdict": "pass"},
        GateStatus.COMPLETE,
        "session-1",
        40,
        (TICKET,),
        local_report={"verdict": "fail"},
        ci_rounds=3,
    )
    assert GateOutcome.from_record(outcome.to_record()) == outcome
    older = {
        key: value for key, value in outcome.to_record().items() if key not in {"ci_rounds", "local_report", "pause"}
    }
    reread = GateOutcome.from_record(older)
    assert (reread.local_report, reread.ci_rounds, reread.pause) == (None, 0, None)
    waiting = GateOutcome(None, GateStatus.PAUSED, None, 3, (), ci_rounds=2, pause=GatePause.CI_ROUND_PENDING)
    assert GateOutcome.from_record(waiting.to_record()) == waiting
