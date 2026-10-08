from __future__ import annotations

from datetime import date, datetime
from pathlib import Path

import pytest

from specbench.application.paced_calls import CallGate, PacedAgentHarness
from specbench.domain.arms import FROZEN_MODEL, Arm, Harness
from specbench.domain.call_ledger import EMPTY_LEDGER, CallLedger, UnknownReservation
from specbench.domain.jsonread import RecordError
from specbench.domain.pacing import DEFAULT_RUN_WINDOW, PacingPolicy, PauseReason, RunPaused, RunWindow
from specbench.domain.reservation import (
    FULL_EVALUATION_EXECUTIONS,
    RE_EVALUATION_EXECUTIONS,
    ExecutionHistory,
    reservation_for,
    seed_executions,
)
from specbench.domain.sessions import SessionKind, SessionRequest, SessionResult, live_gate_call_id
from support.fakes import FakeAgentHarness, FixedClock, MemoryCalls, MemoryLedger

NIGHT = date(2026, 10, 8)
NEXT_NIGHT = date(2026, 10, 9)
CLAUDE = Harness.CLAUDE_CODE


def policy(budget: int) -> PacingPolicy:
    return PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), budget)


def gate_at(moment: datetime, budget: int = 250) -> tuple[CallGate, MemoryLedger, FixedClock]:
    ledger, clock = MemoryLedger(), FixedClock(moment)
    return CallGate(CLAUDE, policy(budget), ledger, clock), ledger, clock


def test_a_reservation_counts_against_the_budget_until_it_is_settled() -> None:
    ledger = EMPTY_LEDGER.with_call(NIGHT, CLAUDE).with_reservation(NIGHT, CLAUDE, "gate-a", 40)
    assert (ledger.calls_on(NIGHT, CLAUDE), ledger.reserved_on(NIGHT, CLAUDE), ledger.committed_on(NIGHT, CLAUDE)) == (
        1,
        40,
        41,
    )
    settled = ledger.settled(NIGHT, CLAUDE, "gate-a", 33)
    assert (settled.calls_on(NIGHT, CLAUDE), settled.reserved_on(NIGHT, CLAUDE)) == (34, 0)


def test_a_settlement_may_exceed_its_reservation_and_the_overshoot_is_recorded() -> None:
    ledger = EMPTY_LEDGER.with_reservation(NIGHT, CLAUDE, "gate-a", 10).settled(NIGHT, CLAUDE, "gate-a", 14)
    assert ledger.calls_on(NIGHT, CLAUDE) == 14


def test_settling_a_call_that_holds_no_reservation_is_refused() -> None:
    with pytest.raises(UnknownReservation):
        EMPTY_LEDGER.settled(NIGHT, CLAUDE, "gate-a", 1)
    with pytest.raises(RecordError):
        EMPTY_LEDGER.with_reservation(NIGHT, CLAUDE, "gate-a", 0)


def test_a_reserved_call_asking_again_on_its_night_keeps_the_one_reservation() -> None:
    once = EMPTY_LEDGER.with_reservation(NIGHT, CLAUDE, "gate-a", 40)
    assert once.with_reservation(NIGHT, CLAUDE, "gate-a", 55) == once


def test_the_pilots_claude_key_is_read_as_claude_code_and_the_old_shape_as_a_charge() -> None:
    ledger = CallLedger.from_record({"2026-10-07": {"claude": 250}, "2026-10-08": {"claude": 3, "claude-code": 4}})
    assert ledger.calls_on(date(2026, 10, 7), CLAUDE) == 250
    assert ledger.calls_on(NIGHT, CLAUDE) == 7
    assert ledger.calls_on(NIGHT, Harness.CODEX) == 0


def test_the_ledger_round_trips_through_its_record() -> None:
    ledger = EMPTY_LEDGER.with_call(NIGHT, CLAUDE).with_reservation(NEXT_NIGHT, Harness.CODEX, "gate-b", 9)
    assert CallLedger.from_record(ledger.to_record()) == ledger


@pytest.mark.parametrize("record", [{"yesterday": {"claude": 1}}, {"2026-10-08": {"gemini": 1}}])
def test_a_ledger_with_an_unknown_night_or_harness_is_refused(record: dict[str, dict[str, int]]) -> None:
    with pytest.raises(ValueError):
        CallLedger.from_record(record)


def test_pending_lists_unsettled_reservations_oldest_night_first() -> None:
    ledger = EMPTY_LEDGER.with_reservation(NEXT_NIGHT, CLAUDE, "gate-b", 5).with_reservation(NIGHT, CLAUDE, "gate-a", 5)
    assert [(a.night, a.call_id) for a in ledger.pending(CLAUDE)] == [(NIGHT, "gate-a"), (NEXT_NIGHT, "gate-b")]
    assert ledger.pending(Harness.CODEX) == ()


def test_a_gate_admitted_at_0720_and_finished_at_0745_is_charged_to_the_night_it_was_admitted() -> None:
    gate, ledger, clock = gate_at(datetime(2026, 10, 9, 7, 20))
    admission = gate.admit_reserved("gate-a", 20)
    clock.moment = datetime(2026, 10, 9, 7, 45)
    gate.settle(admission, 17)
    assert ledger.ledger.calls_on(NIGHT, CLAUDE) == 17
    assert ledger.ledger.calls_on(NEXT_NIGHT, CLAUDE) == 0


def test_a_single_call_finishing_after_the_window_closed_is_charged_to_its_admission_night() -> None:
    moment = datetime(2026, 10, 9, 7, 20)
    ledger, clock = MemoryLedger(), FixedClock(moment)

    def finish_late(_: SessionRequest) -> SessionResult:
        clock.moment = datetime(2026, 10, 9, 7, 45)
        return SessionResult("s-1", "done", 0, (), FROZEN_MODEL[CLAUDE])

    harness = PacedAgentHarness(
        FakeAgentHarness(CLAUDE, finish_late), CallGate(CLAUDE, policy(5), ledger, clock), MemoryCalls()
    )
    request = SessionRequest(CLAUDE, FROZEN_MODEL[CLAUDE], SessionKind.TRUNK, "T1/trunk", Path("w"), "go")
    harness.run_session(request)
    assert (ledger.ledger.calls_on(NIGHT, CLAUDE), ledger.ledger.calls_on(NEXT_NIGHT, CLAUDE)) == (1, 0)


def test_a_reservation_that_does_not_fit_in_the_remaining_share_pauses_the_run_with_exit_3() -> None:
    gate, ledger, _ = gate_at(datetime(2026, 10, 8, 22, 0), budget=100)
    ledger.save(EMPTY_LEDGER.with_call(NIGHT, CLAUDE).with_reservation(NIGHT, CLAUDE, "gate-a", 60))
    with pytest.raises(RunPaused) as paused:
        gate.admit_reserved("gate-b", 40)
    assert paused.value.reason is PauseReason.BUDGET_REACHED
    gate.admit_reserved("gate-c", 39)


def test_a_reserved_gate_pauses_a_single_call_that_would_pass_without_it() -> None:
    gate, ledger, _ = gate_at(datetime(2026, 10, 8, 22, 0), budget=50)
    gate.admit_reserved("gate-a", 50)
    with pytest.raises(RunPaused):
        gate.admit()
    assert ledger.ledger.committed_on(NIGHT, CLAUDE) == 50


def test_a_rerun_the_same_night_reuses_its_reservation_at_no_cost() -> None:
    gate, ledger, _ = gate_at(datetime(2026, 10, 8, 22, 0), budget=50)
    first = gate.admit_reserved("gate-a", 50)
    again = gate.admit_reserved("gate-a", 50)
    assert again == first and ledger.ledger.committed_on(NIGHT, CLAUDE) == 50


def test_a_closed_window_refuses_a_reservation() -> None:
    gate, _, _ = gate_at(datetime(2026, 10, 9, 12, 0))
    with pytest.raises(RunPaused) as paused:
        gate.admit_reserved("gate-a", 1)
    assert paused.value.reason is PauseReason.WINDOW_CLOSED


def test_the_reservation_is_one_plus_a_quarter_more_than_expected_plus_a_wave_of_four() -> None:
    assert reservation_for(0) == 5
    assert reservation_for(40) == 1 + 50 + 4
    assert reservation_for(78) == 1 + 98 + 4


def test_a_key_without_history_is_seeded_from_its_arm_and_scaled_by_its_criteria() -> None:
    assert seed_executions(Arm.ALL, 4) == FULL_EVALUATION_EXECUTIONS
    assert seed_executions(Arm.AL, 4) == RE_EVALUATION_EXECUTIONS
    assert seed_executions(Arm.ALL, 8) == 2 * FULL_EVALUATION_EXECUTIONS
    assert seed_executions(Arm.AL, 2) == RE_EVALUATION_EXECUTIONS // 2
    assert seed_executions(Arm.T, 4) == FULL_EVALUATION_EXECUTIONS
    with pytest.raises(RecordError):
        seed_executions(Arm.ALL, 0)


def test_the_expectation_is_the_running_median_per_harness_arm_and_criteria() -> None:
    history = ExecutionHistory()
    for spent in (50, 20, 30):
        history = history.with_observation(CLAUDE, Arm.ALL, 4, spent)
    assert history.expected(CLAUDE, Arm.ALL, 4) == 30
    assert history.expected(CLAUDE, Arm.ALL, 5) == seed_executions(Arm.ALL, 5)
    assert history.expected(Harness.CODEX, Arm.ALL, 4) == FULL_EVALUATION_EXECUTIONS
    assert history.reservation(CLAUDE, Arm.ALL, 4) == reservation_for(30)
    assert ExecutionHistory.from_json(history.to_json()) == history


def test_a_gate_is_identified_by_its_arm_and_record_directory() -> None:
    record = Path("runs/r/cleanroom/T1/ALL/abc/rec")
    assert live_gate_call_id(Arm.ALL, record) == live_gate_call_id(Arm.ALL, record)
    assert live_gate_call_id(Arm.ALL, record) != live_gate_call_id(Arm.AL, record)
    assert live_gate_call_id(Arm.ALL, record) != live_gate_call_id(Arm.ALL, Path("runs/r/cleanroom/T2/ALL/abc/rec"))
