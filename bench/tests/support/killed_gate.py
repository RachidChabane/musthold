"""A gate-host session that streams events to its call directory and is then killed by SIGKILL, as a window close would.

Run as `python killed_gate.py RUN_DIR`; it never returns.
"""

from __future__ import annotations

import json
import os
import signal
import sys
from datetime import datetime
from pathlib import Path

from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.run_store import FileCallStore, FileEventStream, FileExecutionHistoryStore, FileLedgerStore
from specbench.application.paced_calls import CallGate
from specbench.application.paced_gate_host import PacedGateHost
from specbench.domain.arms import Arm, Harness
from specbench.domain.gate import GateMode, GateOutcome
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.pacing import DEFAULT_RUN_WINDOW, PacingPolicy, RunWindow
from specbench.domain.sessions import live_gate_call_id
from support.fakes import FixedClock

SUBAGENTS_STARTED = 3
ADMITTED_AT = datetime(2026, 10, 9, 7, 20)
ARM = Arm.ALL
CRITERIA = 4
BUDGET = 250


def subagent_event(number: int) -> str:
    block: dict[str, object] = {"type": "tool_use", "id": f"toolu_{number}", "name": "Agent", "input": {}}
    return json.dumps({"type": "assistant", "message": {"content": [block]}})


class StreamingThenKilled:
    def __init__(self, layout: RunLayout) -> None:
        self._layout = layout

    def run_gate(self, tree: Path, arm: Arm, record_dir: Path, mode: GateMode) -> GateOutcome:
        events = self._layout.call(live_gate_call_id(arm, record_dir)) / CALL_EVENTS
        events.parent.mkdir(parents=True, exist_ok=True)
        with events.open("w") as handle:
            for number in range(SUBAGENTS_STARTED):
                handle.write(subagent_event(number) + "\n")
                handle.flush()
            handle.write(subagent_event(SUBAGENTS_STARTED)[:25])
            handle.flush()
        os.kill(os.getpid(), signal.SIGKILL)
        raise AssertionError("unreachable")


def main() -> None:
    layout = RunLayout(Path(sys.argv[1]))
    gate = CallGate(
        Harness.CLAUDE_CODE,
        PacingPolicy(RunWindow.parse(DEFAULT_RUN_WINDOW), BUDGET),
        FileLedgerStore(layout),
        FixedClock(ADMITTED_AT),
    )
    host = PacedGateHost(
        StreamingThenKilled(layout),
        Harness.CLAUDE_CODE,
        CRITERIA,
        gate,
        FileCallStore(layout),
        FileEventStream(layout),
        ClaudeExecutionCounter(),
        FileExecutionHistoryStore(layout),
    )
    host.run_gate(Path("tree"), ARM, Path("rec"), GateMode.LIVE)


if __name__ == "__main__":
    main()
