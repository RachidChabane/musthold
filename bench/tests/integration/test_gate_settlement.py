from __future__ import annotations

import os
import signal
import subprocess
import sys
from datetime import date, datetime
from pathlib import Path

import pytest

from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.run_store import FileCallStore, FileEventStream, FileExecutionHistoryStore, FileLedgerStore
from specbench.application.paced_calls import CallGate
from specbench.application.paced_gate_host import PacedGateHost
from specbench.domain.agent_events import TokenUsage
from specbench.domain.arms import Arm, Harness, Module
from specbench.domain.gate import GateMode, GateOutcome, GateStatus, TicketUsage
from specbench.domain.manifest import RunLayout
from specbench.domain.pacing import DEFAULT_RUN_WINDOW, PacingPolicy, RunWindow
from specbench.domain.reservation import reservation_for, seed_executions
from specbench.domain.sessions import RequestKind
from support.fakes import FakeGateHost, FixedClock
from support.killed_gate import ARM, CRITERIA, SUBAGENTS_STARTED

pytestmark = pytest.mark.integration

ADMISSION_NIGHT = date(2026, 10, 8)
RESUME_NIGHT = date(2026, 10, 9)
CLAUDE = Harness.CLAUDE_CODE
TICKET = TicketUsage(
    "req-1", RequestKind.CLOSED_QUESTION, Module.ALIGNMENT, TokenUsage(input_tokens=5, output_tokens=2), None
)


def host_over(layout: RunLayout, moment: datetime, inner: FakeGateHost) -> PacedGateHost:
    gate = CallGate(
        CLAUDE, PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), 250), FileLedgerStore(layout), FixedClock(moment)
    )
    return PacedGateHost(
        inner,
        CLAUDE,
        CRITERIA,
        gate,
        FileCallStore(layout),
        FileEventStream(layout),
        ClaudeExecutionCounter(),
        FileExecutionHistoryStore(layout),
    )


def kill_a_gate(run_dir: Path) -> int:
    src = Path(__file__).parents[2] / "src"
    tests = Path(__file__).parents[1]
    env = {**os.environ, "PYTHONPATH": os.pathsep.join([str(src), str(tests)])}
    script = tests / "support" / "killed_gate.py"
    return subprocess.run([sys.executable, str(script), str(run_dir)], env=env, check=False, timeout=60).returncode


def test_a_session_killed_at_the_window_close_settles_from_its_events_file_on_its_admission_night(
    tmp_path: Path,
) -> None:
    layout = RunLayout(tmp_path / "run")
    assert kill_a_gate(layout.root) == -signal.SIGKILL
    ledger = FileLedgerStore(layout)
    assert ledger.load().reserved_on(ADMISSION_NIGHT, CLAUDE) == reservation_for(seed_executions(ARM, CRITERIA))
    assert ledger.load().calls_on(ADMISSION_NIGHT, CLAUDE) == 0

    resumed = FakeGateHost.returning(
        lambda: GateOutcome({"verdict": "pass"}, GateStatus.COMPLETE, "session-2", 6, (TICKET,))
    )
    outcome = host_over(layout, datetime(2026, 10, 9, 21, 30), resumed).run_gate(
        Path("tree"), Arm.ALL, Path("rec"), GateMode.LIVE
    )

    settled_from_events = 1 + SUBAGENTS_STARTED
    assert ledger.load().calls_on(ADMISSION_NIGHT, CLAUDE) == settled_from_events
    assert ledger.load().calls_on(RESUME_NIGHT, CLAUDE) == outcome.executions
    assert ledger.load().pending(CLAUDE) == ()
    assert FileExecutionHistoryStore(layout).load().expected(CLAUDE, ARM, CRITERIA) == outcome.executions


def test_a_completed_gate_is_archived_with_its_tickets_and_resumes_for_free(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path / "run")
    complete = GateOutcome({"verdict": "pass"}, GateStatus.COMPLETE, "session-1", 9, (TICKET,))
    inner = FakeGateHost.returning(lambda: complete)
    first = host_over(layout, datetime(2026, 10, 8, 22, 0), inner).run_gate(
        Path("t"), Arm.ALL, Path("r"), GateMode.LIVE
    )
    ledger_after = FileLedgerStore(layout).load()

    again = host_over(layout, datetime(2026, 10, 9, 22, 0), inner).run_gate(
        Path("t"), Arm.ALL, Path("r"), GateMode.LIVE
    )

    assert again == first == complete
    assert FileLedgerStore(layout).load() == ledger_after
    assert len(inner.received) == 1


def test_the_ledger_file_is_replaced_whole_so_a_kill_never_leaves_half_a_ledger(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path / "run")
    store = FileLedgerStore(layout)
    store.save(store.load().with_call(ADMISSION_NIGHT, CLAUDE))
    store.save(store.load().with_call(ADMISSION_NIGHT, CLAUDE))
    assert store.load().calls_on(ADMISSION_NIGHT, CLAUDE) == 2
    assert [path.name for path in layout.root.iterdir()] == [layout.call_ledger.name]


def test_a_ledger_written_by_the_pilot_is_read_through_the_claude_alias(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path / "run")
    layout.root.mkdir()
    layout.call_ledger.write_text('{"2026-10-07":{"claude":250},"2026-10-08":{"claude":90}}')
    ledger = FileLedgerStore(layout).load()
    assert (ledger.calls_on(date(2026, 10, 7), CLAUDE), ledger.calls_on(ADMISSION_NIGHT, CLAUDE)) == (250, 90)
