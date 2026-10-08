"""Coder sessions behind the pacing decorator: quota pauses, executions charged from events, nothing archived twice."""

from __future__ import annotations

from datetime import date, datetime
from pathlib import Path

import pytest

from specbench.adapters.run_store import FileCallStore, FileLedgerStore
from specbench.application.paced_calls import CallGate, PacedAgentHarness
from specbench.application.ports import AgentHarness
from specbench.domain.arms import Harness
from specbench.domain.manifest import RunLayout
from specbench.domain.pacing import PacingPolicy, RunPaused
from specbench.domain.sessions import SessionOutcome, SessionRequest, SessionResult
from support.coder_rigs import KILL_AFTER_SECONDS, make_claude_rig, make_codex_rig
from support.fakes import FixedClock

NOON = datetime(2026, 10, 9, 12, 0)
TONIGHT = date(2026, 10, 9)
QUOTA_MESSAGE = "You have hit your usage limit; try again later"


class Paced:
    def __init__(self, inner: AgentHarness, layout: RunLayout, harness: Harness) -> None:
        self.ledgers = FileLedgerStore(layout)
        self.calls = FileCallStore(layout)
        gate = CallGate(harness, PacingPolicy(None, budget_per_night=100), self.ledgers, FixedClock(NOON))
        self.harness = PacedAgentHarness(inner, gate, self.calls)

    def run(self, request: SessionRequest) -> SessionResult:
        return self.harness.run_session(request)

    def charged(self, harness: Harness) -> int:
        return self.ledgers.load().calls_on(TONIGHT, harness)


def test_a_claude_result_that_reports_the_spent_quota_pauses_the_run_and_is_not_archived(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"is_error": True, "final": QUOTA_MESSAGE, "exit_after": 1}])
    paced = Paced(rig.harness, rig.layout, Harness.CLAUDE_CODE)
    request = rig.request("t1")

    with pytest.raises(RunPaused):
        paced.run(request)

    assert not paced.calls.exists(request.call_id)


def test_a_claude_cli_that_prints_the_quota_message_outside_the_stream_pauses_the_run(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"exit": 1, "stderr": QUOTA_MESSAGE}])
    paced = Paced(rig.harness, rig.layout, Harness.CLAUDE_CODE)

    with pytest.raises(RunPaused):
        paced.run(rig.request("t1"))


def test_a_codex_error_event_that_reports_the_spent_quota_pauses_the_run_and_is_not_archived(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"error": QUOTA_MESSAGE, "exit_after": 1}])
    paced = Paced(rig.harness, rig.layout, Harness.CODEX)
    request = rig.request("t1")

    with pytest.raises(RunPaused):
        paced.run(request)

    assert not paced.calls.exists(request.call_id)


def test_a_codex_exit_before_any_thread_with_the_quota_message_pauses_the_run(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"exit": 1, "stderr": QUOTA_MESSAGE}])
    paced = Paced(rig.harness, rig.layout, Harness.CODEX)

    with pytest.raises(RunPaused):
        paced.run(rig.request("t1"))


def test_a_completed_session_is_charged_the_executions_its_events_show(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"agent": "one"}, {"agent": "two"}]}])
    paced = Paced(rig.harness, rig.layout, Harness.CLAUDE_CODE)

    result = paced.run(rig.request("t1"))

    assert result.executions == 3 and paced.charged(Harness.CLAUDE_CODE) == 3


def test_an_interrupted_session_is_charged_what_it_spent_before_it_was_killed(tmp_path: Path) -> None:
    rig = make_claude_rig(
        tmp_path, [{"events": [{"agent": "one"}, {"hang": True}]}], timeout_seconds=KILL_AFTER_SECONDS
    )
    paced = Paced(rig.harness, rig.layout, Harness.CLAUDE_CODE)

    result = paced.run(rig.request("t1"))

    assert result.outcome is SessionOutcome.INTERRUPTED
    assert paced.charged(Harness.CLAUDE_CODE) == 2


def test_a_codex_session_with_subagents_is_charged_its_spawns(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [{"spawn": "decider"}, {"spawn": "decider"}]}])
    paced = Paced(rig.harness, rig.layout, Harness.CODEX)

    paced.run(rig.request("t1"))

    assert paced.charged(Harness.CODEX) == 3


def test_a_replayed_session_costs_nothing_and_keeps_its_executions(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"agent": "one"}]}])
    paced = Paced(rig.harness, rig.layout, Harness.CLAUDE_CODE)
    request = rig.request("t1")
    paced.run(request)

    replay = paced.run(request)

    assert replay.executions == 2 and paced.charged(Harness.CLAUDE_CODE) == 2
