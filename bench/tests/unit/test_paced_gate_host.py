from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path

import pytest

from specbench.application.paced_calls import CallGate
from specbench.application.paced_gate_host import PacedGateHost
from specbench.domain.agent_events import TokenUsage
from specbench.domain.arms import Arm, Harness, Module
from specbench.domain.gate import GateMode, GateOutcome, GateStatus, TicketUsage
from specbench.domain.pacing import DEFAULT_RUN_WINDOW, PacingPolicy, PauseReason, RunPaused, RunWindow
from specbench.domain.reservation import reservation_for, seed_executions
from specbench.domain.sessions import RequestKind, live_gate_call_id
from support.fakes import FakeGateHost, FixedClock, LineCounter, MemoryCalls, MemoryEvents, MemoryHistory, MemoryLedger

NIGHT = date(2026, 10, 8)
NEXT_NIGHT = date(2026, 10, 9)
CLAUDE = Harness.CLAUDE_CODE
TREE = Path("cleanroom/T1/ALL/abc")
RECORD = Path("cleanroom/T1/ALL/abc/rec")
CRITERIA = 4
TICKET = TicketUsage("req-1", RequestKind.ALIGNMENT, Module.ALIGNMENT, TokenUsage(input_tokens=7), "claude-sonnet-5-5")


def complete(executions: int) -> GateOutcome:
    return GateOutcome({"verdict": "pass"}, GateStatus.COMPLETE, "session-1", executions, (TICKET,))


@dataclass
class Rig:
    host: PacedGateHost
    inner: FakeGateHost
    calls: MemoryCalls
    ledger: MemoryLedger
    events: MemoryEvents
    history: MemoryHistory
    clock: FixedClock

    def run(self, mode: GateMode = GateMode.LIVE, arm: Arm = Arm.ALL) -> GateOutcome:
        return self.host.run_gate(TREE, arm, RECORD, mode)


def rig(answer: Callable[[], GateOutcome], budget: int = 250, moment: datetime = datetime(2026, 10, 8, 22, 0)) -> Rig:
    ledger, calls, events, history, clock = (
        MemoryLedger(),
        MemoryCalls(),
        MemoryEvents(),
        MemoryHistory(),
        FixedClock(moment),
    )
    gate = CallGate(CLAUDE, PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), budget), ledger, clock)
    inner = FakeGateHost.returning(answer)
    host = PacedGateHost(inner, CLAUDE, CRITERIA, gate, calls, events, LineCounter(), history)
    return Rig(host, inner, calls, ledger, events, history, clock)


def stream(rig_: Rig, executions: int, arm: Arm = Arm.ALL) -> None:
    rig_.events.streamed[live_gate_call_id(arm, RECORD)] = tuple(f'{{"n": {n}}}' for n in range(executions))


def test_a_live_gate_reserves_runs_and_charges_what_the_session_spent_even_beyond_its_reservation() -> None:
    spent = reservation_for(seed_executions(Arm.ALL, CRITERIA)) + 3
    r = rig(lambda: complete(spent))
    outcome = r.run()
    assert outcome.status is GateStatus.COMPLETE
    assert r.ledger.ledger.calls_on(NIGHT, CLAUDE) == spent
    assert r.ledger.ledger.reserved_on(NIGHT, CLAUDE) == 0


def test_a_resumed_gate_costs_nothing_and_returns_the_recorded_outcome() -> None:
    r = rig(lambda: complete(30))
    first = r.run()
    charged = r.ledger.ledger
    again = r.run()
    assert again == first
    assert r.ledger.ledger == charged
    assert len(r.inner.received) == 1


def test_a_replay_passes_through_without_touching_the_ledger() -> None:
    r = rig(lambda: complete(0))
    assert r.run(GateMode.REPLAY).executions == 0
    assert r.ledger.ledger.accounts == {}
    assert r.calls.records == {}


def test_a_paused_gate_is_charged_for_what_it_spent_and_run_again_next_time() -> None:
    paused = GateOutcome(None, GateStatus.PAUSED, "session-1", 12, ())
    r = rig(lambda: paused)
    r.run()
    assert r.ledger.ledger.calls_on(NIGHT, CLAUDE) == 12
    assert r.calls.records == {}
    r.run()
    assert len(r.inner.received) == 2


def test_the_reservation_is_sized_from_the_seed_then_from_the_median_of_completed_gates() -> None:
    sizes: list[int] = []

    def answer() -> GateOutcome:
        sizes.append(r.ledger.ledger.reserved_on(NIGHT, CLAUDE))
        return complete(10)

    r = rig(answer)
    r.run()
    r.host.run_gate(Path("cleanroom/T1/ALL/def"), Arm.ALL, Path("cleanroom/T1/ALL/def/rec"), GateMode.LIVE)
    assert sizes == [reservation_for(seed_executions(Arm.ALL, CRITERIA)), reservation_for(10)]


def test_a_gate_whose_reservation_exceeds_the_remaining_share_pauses_before_any_call() -> None:
    r = rig(lambda: complete(1), budget=reservation_for(seed_executions(Arm.ALL, CRITERIA)) - 1)
    with pytest.raises(RunPaused) as paused:
        r.run()
    assert paused.value.reason is PauseReason.BUDGET_REACHED
    assert r.inner.received == []


def test_a_killed_gate_is_settled_from_its_events_on_its_admission_night_and_the_next_night_reserves_afresh() -> None:
    class Killed(BaseException):
        pass

    def killed() -> GateOutcome:
        raise Killed

    r = rig(killed, moment=datetime(2026, 10, 9, 7, 20))
    stream(r, 21)
    with pytest.raises(Killed):
        r.run()
    assert r.ledger.ledger.reserved_on(NIGHT, CLAUDE) > 0

    r.clock.moment = datetime(2026, 10, 9, 21, 30)
    rerun = PacedGateHost(
        FakeGateHost.returning(lambda: complete(5)),
        CLAUDE,
        CRITERIA,
        CallGate(CLAUDE, PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), 250), r.ledger, r.clock),
        r.calls,
        r.events,
        LineCounter(),
        r.history,
    )
    rerun.run_gate(TREE, Arm.ALL, RECORD, GateMode.LIVE)
    assert r.ledger.ledger.calls_on(NIGHT, CLAUDE) == 21
    assert r.ledger.ledger.calls_on(NEXT_NIGHT, CLAUDE) == 5
    assert r.ledger.ledger.reserved_on(NIGHT, CLAUDE) == 0
    assert r.ledger.ledger.pending(CLAUDE) == ()


def test_a_same_night_rerun_after_a_kill_keeps_its_reservation_and_charges_every_event_of_both_attempts() -> None:
    attempts: list[int] = []

    def answer() -> GateOutcome:
        attempts.append(r.ledger.ledger.reserved_on(NIGHT, CLAUDE))
        if len(attempts) == 1:
            raise KeyboardInterrupt
        return complete(8)

    r = rig(answer)
    stream(r, 9)
    with pytest.raises(KeyboardInterrupt):
        r.run()
    stream(r, 14)
    r.run()
    assert attempts[0] == attempts[1]
    assert r.ledger.ledger.calls_on(NIGHT, CLAUDE) == 14
    assert r.ledger.ledger.pending(CLAUDE) == ()


def test_a_gate_that_starts_while_another_is_still_running_leaves_the_running_one_reserved() -> None:
    other_record = Path("cleanroom/T1/AL/def/rec")
    started: list[int] = []

    def answer() -> GateOutcome:
        started.append(len(started))
        if len(started) == 1:
            r.host.run_gate(Path("cleanroom/T1/AL/def"), Arm.AL, other_record, GateMode.LIVE)
            assert len(r.ledger.ledger.pending(CLAUDE)) == 1
            return complete(6)
        return complete(4)

    r = rig(answer)
    first = r.run()
    assert first.executions == 6
    assert r.ledger.ledger.calls_on(NIGHT, CLAUDE) == 10
    assert r.ledger.ledger.pending(CLAUDE) == ()
    assert r.run() == first
    assert len(r.inner.received) == 2
