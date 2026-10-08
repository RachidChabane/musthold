from __future__ import annotations

from datetime import date, datetime
from pathlib import Path

import pytest

from specbench.application.paced_calls import CallGate, ModelNotFrozen, PacedAgentHarness
from specbench.application.paced_host_executor import PacedHostExecutor
from specbench.domain.arms import FROZEN_MODEL, Arm, Harness, Host
from specbench.domain.call_ledger import EMPTY_LEDGER
from specbench.domain.deviations import (
    ARM_ORDER,
    DeviationEntry,
    DeviationKind,
    InvalidDeviation,
    parse_deviations,
    parse_entry,
    render_deviations,
)
from specbench.domain.pacing import (
    DEFAULT_RUN_WINDOW,
    EXIT_PAUSED,
    InvalidRunWindow,
    PacingPolicy,
    PauseReason,
    RunPaused,
    RunWindow,
    looks_like_quota_exhausted,
)
from specbench.domain.sessions import HostRequest, RequestKind, SessionKind, SessionRequest, SessionResult
from support.fakes import FakeAgentHarness, FakeHostExecutor, FixedClock, MemoryCalls, MemoryLedger

NIGHT = datetime(2026, 10, 8, 23, 30)
AFTER_MIDNIGHT = datetime(2026, 10, 9, 2, 0)
NOON = datetime(2026, 10, 9, 12, 0)


def window() -> RunWindow:
    parsed = RunWindow.parse(DEFAULT_RUN_WINDOW)
    assert parsed is not None
    return parsed


def test_the_default_window_spans_midnight() -> None:
    w = window()
    assert w.is_open(NIGHT) and w.is_open(AFTER_MIDNIGHT) and not w.is_open(NOON)
    assert w.is_open(datetime(2026, 10, 9, 21, 0)) and not w.is_open(datetime(2026, 10, 9, 7, 30))
    assert w.is_open(datetime(2026, 10, 9, 7, 29))


def test_a_night_is_named_by_the_date_its_window_opened() -> None:
    w = window()
    assert w.night_of(NIGHT) == date(2026, 10, 8)
    assert w.night_of(AFTER_MIDNIGHT) == date(2026, 10, 8)
    assert w.night_of(NOON) == date(2026, 10, 9)


@pytest.mark.parametrize("text", ["21:00", "25:00-07:00", "21:00-21:00", "x"])
def test_invalid_windows_are_refused(text: str) -> None:
    with pytest.raises(InvalidRunWindow):
        RunWindow.parse(text)


def test_always_disables_the_window() -> None:
    assert RunWindow.parse("always") is None
    assert PacingPolicy(None).night_of(NOON) == NOON.date()


def test_quota_messages_are_recognised() -> None:
    assert looks_like_quota_exhausted("", "You have hit your limit for today")
    assert looks_like_quota_exhausted("Error 429 too many requests")
    assert not looks_like_quota_exhausted("tests passed")


def test_policy_pauses_outside_the_window_and_over_budget_but_never_a_resumed_call() -> None:
    policy = PacingPolicy(window(), budget_per_night=2)
    with pytest.raises(RunPaused) as closed:
        policy.admit(NOON, Harness.CLAUDE_CODE, EMPTY_LEDGER, resumed=False)
    assert closed.value.reason is PauseReason.WINDOW_CLOSED
    ledger = EMPTY_LEDGER.with_call(date(2026, 10, 8), Harness.CLAUDE_CODE).with_call(
        date(2026, 10, 8), Harness.CLAUDE_CODE
    )
    with pytest.raises(RunPaused) as spent:
        policy.admit(AFTER_MIDNIGHT, Harness.CLAUDE_CODE, ledger, resumed=False)
    assert spent.value.reason is PauseReason.BUDGET_REACHED
    policy.admit(AFTER_MIDNIGHT, Harness.CODEX, ledger, resumed=False)
    policy.admit(NOON, Harness.CLAUDE_CODE, ledger, resumed=True)
    assert EXIT_PAUSED == 3


def request(
    harness: Harness = Harness.CLAUDE_CODE,
    prompt: str = "implement the spec",
    model: str | None = None,
    key: str = "T1/trunk/stop-1",
) -> SessionRequest:
    return SessionRequest(harness, model or FROZEN_MODEL[harness], SessionKind.TRUNK, key, Path("w"), prompt)


def paced(
    answer: object, now: datetime = NIGHT, budget: int = 2, harness: Harness = Harness.CLAUDE_CODE
) -> tuple[PacedAgentHarness, FakeAgentHarness, MemoryCalls, MemoryLedger]:
    inner = FakeAgentHarness(harness, answer)  # type: ignore[arg-type]
    calls, ledger = MemoryCalls(), MemoryLedger()
    gate = CallGate(harness, PacingPolicy(window(), budget), ledger, FixedClock(now))
    return PacedAgentHarness(inner, gate, calls), inner, calls, ledger


def ok(_: SessionRequest) -> SessionResult:
    return SessionResult("s-1", "done", 0, ('{"type":"result"}',), "claude-sonnet-5-5")


def test_a_new_call_is_archived_charged_and_a_repeat_is_free() -> None:
    harness, inner, calls, ledger = paced(ok)
    first = harness.run_session(request())
    again = harness.run_session(request())
    assert first.final_message == again.final_message == "done"
    assert inner.calls == 1
    assert ledger.ledger.calls_on(date(2026, 10, 8), Harness.CLAUDE_CODE) == 1
    assert len(calls.records) == 1


def test_identical_prompts_with_different_keys_are_different_calls() -> None:
    harness, inner, calls, ledger = paced(ok, budget=5)
    prompt = "run prove-all in the clean-room copy"
    harness.run_session(request(prompt=prompt, key="T1/ALL/stop-1"))
    harness.run_session(request(prompt=prompt, key="T2/ALL/stop-1"))
    harness.run_session(request(prompt=prompt, key="T1/ALL/stop-1"))
    assert inner.calls == 2 and len(calls.records) == 2
    assert request(prompt=prompt, key="a").call_id != request(prompt=prompt, key="b").call_id
    assert ledger.ledger.calls_on(date(2026, 10, 8), Harness.CLAUDE_CODE) == 2


def test_the_budget_pauses_new_calls_and_resumed_calls_still_pass() -> None:
    harness, inner, _, _ = paced(ok, budget=1)
    harness.run_session(request(prompt="one"))
    with pytest.raises(RunPaused) as paused:
        harness.run_session(request(prompt="two"))
    assert paused.value.reason is PauseReason.BUDGET_REACHED
    assert harness.run_session(request(prompt="one")).session_id == "s-1"
    assert inner.calls == 1


def test_a_closed_window_pauses_before_any_call() -> None:
    harness, inner, _, _ = paced(ok, now=NOON)
    with pytest.raises(RunPaused):
        harness.run_session(request())
    assert inner.calls == 0


def test_a_quota_failure_pauses_and_is_not_recorded_as_a_call() -> None:
    def exhausted(_: SessionRequest) -> SessionResult:
        return SessionResult("s-2", "", 1, (), None, stderr="You have hit your usage limit")

    harness, _, calls, ledger = paced(exhausted)
    with pytest.raises(RunPaused) as paused:
        harness.run_session(request())
    assert paused.value.reason is PauseReason.QUOTA_EXHAUSTED
    assert not calls.records and ledger.ledger.accounts == {}


def test_only_the_frozen_model_of_the_harness_may_run() -> None:
    harness, inner, _, _ = paced(ok)
    with pytest.raises(ModelNotFrozen):
        harness.run_session(request(model="claude-opus-5-5"))
    codex, _, _, _ = paced(ok, harness=Harness.CODEX)
    with pytest.raises(ModelNotFrozen):
        codex.run_session(request(Harness.CODEX, model="gpt-5.6-terra"))
    assert inner.calls == 0


def test_host_executor_is_paced_archived_and_resumable() -> None:
    payloads = {"req-1": {"verdict": "valid", "score": 0.5}}
    inner = FakeHostExecutor(Host.ISOLATED_CLI, payloads)  # type: ignore[arg-type]
    calls, ledger = MemoryCalls(), MemoryLedger()
    gate = CallGate(Harness.CLAUDE_CODE, PacingPolicy(window(), 5), ledger, FixedClock(NIGHT))
    executor = PacedHostExecutor(inner, Host.ISOLATED_CLI, gate, calls)
    host_request = HostRequest("req-1", RequestKind.VALIDITY, "judge this", {})
    first = executor.execute(host_request)
    second = executor.execute(host_request)
    assert first.payload == second.payload == {"verdict": "valid", "score": 0.5}
    assert inner.calls == 1 and ledger.ledger.calls_on(date(2026, 10, 8), Harness.CLAUDE_CODE) == 1


def test_deviation_entries_render_and_parse_through_one_grammar() -> None:
    entry = DeviationEntry.create(DeviationKind.QUOTA_BLOCKED, [Arm.PL, Arm.AL], "plus plan exhausted\nuntil Friday")
    assert entry.render() == "- quota-blocked | arms: AL, PL | plus plan exhausted until Friday"
    assert parse_entry(entry.render()) == entry
    everything = DeviationEntry.create(DeviationKind.PROTOCOL, ARM_ORDER, "stop hook did not fire")
    assert "arms: all" in everything.render()
    assert parse_deviations(render_deviations([entry, everything])) == [entry, everything]


def test_scope_rules_per_kind() -> None:
    with pytest.raises(InvalidDeviation):
        DeviationEntry.create(DeviationKind.QUOTA_PAUSE, [Arm.T], "pause")
    with pytest.raises(InvalidDeviation):
        DeviationEntry.create(DeviationKind.HARNESS_MISCONFIGURED, [], "no arm named")
    with pytest.raises(InvalidDeviation):
        DeviationEntry.create(DeviationKind.PROVENANCE, [], "  ")


def test_reading_fails_closed_on_a_malformed_line() -> None:
    entries = parse_deviations(
        "# Deviations\n\n- quota-pause | arms: none | paused\nfree text nobody parses\n- unknown | arms: none | x\n"
    )
    assert [e.well_formed for e in entries] == [True, False, False]
    assert all(e.kind is DeviationKind.PROTOCOL and e.arms == ARM_ORDER for e in entries[1:])
