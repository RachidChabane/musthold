from __future__ import annotations

from datetime import date, datetime
from pathlib import Path

from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.application.paced_calls import CallGate
from specbench.application.paced_gate_host import PacedGateHost
from specbench.domain.arms import Arm, Harness
from specbench.domain.call_ledger import EMPTY_LEDGER, Admission, CallLedger
from specbench.domain.gate import GateMode, GateOutcome, GateStatus
from specbench.domain.pacing import DEFAULT_RUN_WINDOW, PacingPolicy, RunPaused, RunWindow
from specbench.domain.reservation import ExecutionHistory, reservation_for
from specbench.domain.sessions import live_gate_call_id
from support.fakes import FakeGateHost, FixedClock, LineCounter, MemoryCalls, MemoryEvents, MemoryHistory, MemoryLedger

NIGHT = date(2026, 10, 8)
CLAUDE = Harness.CLAUDE_CODE
EVENING = datetime(2026, 10, 8, 22, 0)

sizes = st.integers(min_value=1, max_value=120)
budgets = st.integers(min_value=1, max_value=400)
call_ids = st.text(alphabet="abcdef", min_size=1, max_size=3)


@settings(max_examples=200)
@given(budget=budgets, steps=st.lists(st.tuples(call_ids, sizes, st.floats(min_value=0, max_value=1)), max_size=25))
def test_charged_plus_reserved_never_exceeds_the_budget_when_every_call_settles_within_its_reservation(
    budget: int, steps: list[tuple[str, int, float]]
) -> None:
    ledger = MemoryLedger()
    gate = CallGate(CLAUDE, PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), budget), ledger, FixedClock(EVENING))
    in_flight: list[tuple[Admission, int, float]] = []
    for call_id, size, share in steps:
        try:
            admission = gate.admit_reserved(call_id, size)
        except RunPaused:
            pass
        else:
            if not any(a.call_id == call_id for a, _, _ in in_flight):
                in_flight.append((admission, size, share))
        assert ledger.ledger.committed_on(NIGHT, CLAUDE) <= budget
        if in_flight and share > 0.5:
            settled, reserved, fraction = in_flight.pop(0)
            gate.settle(settled, int(reserved * fraction))
            assert ledger.ledger.committed_on(NIGHT, CLAUDE) <= budget
    for settled, reserved, fraction in in_flight:
        gate.settle(settled, int(reserved * fraction))
    assert ledger.ledger.reserved_on(NIGHT, CLAUDE) == 0
    assert ledger.ledger.calls_on(NIGHT, CLAUDE) <= budget


@given(
    charged=st.integers(min_value=0, max_value=50),
    held=st.lists(sizes, max_size=5),
    asked=sizes,
    budget=budgets,
)
def test_a_reservation_is_admitted_exactly_when_it_fits_what_is_left(
    charged: int, held: list[int], asked: int, budget: int
) -> None:
    ledger: CallLedger = EMPTY_LEDGER
    for _ in range(charged):
        ledger = ledger.with_call(NIGHT, CLAUDE)
    for index, size in enumerate(held):
        ledger = ledger.with_reservation(NIGHT, CLAUDE, f"held-{index}", size)
    policy = PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), budget)
    fits = asked <= budget - ledger.committed_on(NIGHT, CLAUDE)
    try:
        policy.admit_reserved(EVENING, CLAUDE, ledger, "new", asked)
    except RunPaused:
        assert not fits
    else:
        assert fits


@given(executions=st.integers(min_value=1, max_value=200), reruns=st.integers(min_value=1, max_value=4))
def test_a_resumed_gate_costs_zero_executions_however_often_it_is_asked_again(executions: int, reruns: int) -> None:
    ledger, calls, events = MemoryLedger(), MemoryCalls(), MemoryEvents()
    record, tree = Path("rec"), Path("tree")
    events.streamed[live_gate_call_id(Arm.ALL, record)] = ("{}",) * executions
    outcome = GateOutcome({"ok": True}, GateStatus.COMPLETE, "s", executions, ())
    inner = FakeGateHost.returning(lambda: outcome)
    gate = CallGate(CLAUDE, PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), 1000), ledger, FixedClock(EVENING))
    host = PacedGateHost(inner, CLAUDE, 4, gate, calls, events, LineCounter(), MemoryHistory())
    host.run_gate(tree, Arm.ALL, record, GateMode.LIVE)
    charged = ledger.ledger
    for _ in range(reruns):
        assert host.run_gate(tree, Arm.ALL, record, GateMode.LIVE).executions == executions
    assert ledger.ledger == charged
    assert len(inner.received) == 1


@given(st.lists(st.integers(min_value=1, max_value=300), min_size=1, max_size=15))
def test_the_reservation_always_covers_the_median_it_is_sized_from(spent: list[int]) -> None:
    history = ExecutionHistory()
    for executions in spent:
        history = history.with_observation(CLAUDE, Arm.ALL, 4, executions)
    expected = history.expected(CLAUDE, Arm.ALL, 4)
    assert min(spent) <= expected <= max(spent)
    assert history.reservation(CLAUDE, Arm.ALL, 4) == reservation_for(expected) > expected


@given(st.dictionaries(call_ids, st.tuples(sizes, st.integers(min_value=0, max_value=300)), max_size=6))
def test_the_ledger_record_round_trips_and_a_settlement_moves_executions_from_reserved_to_charged(
    gates: dict[str, tuple[int, int]],
) -> None:
    ledger = EMPTY_LEDGER
    for call_id, (size, _) in gates.items():
        ledger = ledger.with_reservation(NIGHT, CLAUDE, call_id, size)
    assert CallLedger.from_record(ledger.to_record()) == ledger
    for call_id, (_, actual) in gates.items():
        ledger = ledger.settled(NIGHT, CLAUDE, call_id, actual)
    assert ledger.calls_on(NIGHT, CLAUDE) == sum(actual for _, actual in gates.values())
    assert ledger.reserved_on(NIGHT, CLAUDE) == 0
