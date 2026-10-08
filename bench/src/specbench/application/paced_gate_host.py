"""Pacing of the gate host: a live gate reserves its executions before it starts and settles them when it ends.

A gate is one session that spends dozens of executions in a burst, so the single-call admission of `CallGate` cannot
bound it. Here a live gate reserves `r` executions on tonight's share (and pauses with exit 3 when `r` does not fit),
the adapter streams the session's events to the call directory, and the charge is the executions those events show,
on the night of admission. A gate that is killed leaves its reservation behind; the next gate settles it from the
events file before reserving for itself.
"""

from __future__ import annotations

from pathlib import Path

from specbench.application.paced_calls import CallGate
from specbench.application.ports import (
    CallStore,
    EventStream,
    ExecutionCounter,
    ExecutionHistoryStore,
    GateHost,
)
from specbench.domain.arms import Arm, Harness
from specbench.domain.call_ledger import Admission
from specbench.domain.canonical import parse_stable_json, stable_json
from specbench.domain.gate import GateMode, GateOutcome, GateStatus
from specbench.domain.jsonread import RecordError
from specbench.domain.sessions import CallRecord, live_gate_call_id


class PacedGateHost:
    """Decorator over a `GateHost` for one task: a recorded gate is replayed for free, a live one is reserved and settled.

    `criteria` is the number of criteria of the task's spec, which sizes the reservation together with the harness and
    the arm. A `REPLAY` makes no model call and passes through untouched. Each ledger has one writing process at a
    time; the load-and-save of the ledger is not locked across processes. Only a complete gate is archived; a paused or
    misconfigured one is charged for what it spent and run again on the next attempt.
    """

    def __init__(
        self,
        inner: GateHost,
        harness: Harness,
        criteria: int,
        gate: CallGate,
        calls: CallStore,
        events: EventStream,
        counter: ExecutionCounter,
        history: ExecutionHistoryStore,
    ) -> None:
        self._inner = inner
        self._harness = harness
        self._criteria = criteria
        self._gate = gate
        self._calls = calls
        self._events = events
        self._counter = counter
        self._history = history

    def run_gate(self, tree: Path, arm: Arm, record_dir: Path, mode: GateMode) -> GateOutcome:
        if mode is GateMode.REPLAY:
            return self._inner.run_gate(tree, arm, record_dir, mode)
        call_id = live_gate_call_id(arm, record_dir)
        if self._calls.exists(call_id):
            return _outcome_of(self._calls.read(call_id))
        self._settle_orphans()
        resumed_reservation = self._gate.holds_reservation(call_id)
        admission = self._gate.admit_reserved(
            call_id, self._history.load().reservation(self._harness, arm, self._criteria)
        )
        outcome = self._inner.run_gate(tree, arm, record_dir, mode)
        spent = self._spent(call_id, outcome, resumed_reservation)
        self._gate.settle(admission, spent)
        if outcome.status is GateStatus.COMPLETE:
            self._calls.write(_record_of(call_id, arm, outcome, spent))
            self._history.save(self._history.load().with_observation(self._harness, arm, self._criteria, spent))
        return outcome

    def _spent(self, call_id: str, outcome: GateOutcome, resumed_reservation: bool) -> int:
        """A fresh reservation is charged what the session reports. A reservation kept from a killed attempt of the same
        night is charged the events of both attempts, which share one events file."""
        if not resumed_reservation:
            return outcome.executions
        return max(outcome.executions, self._counter.count(self._events.lines(call_id)))

    def _settle_orphans(self) -> None:
        """Settle the gates of earlier nights that were admitted and never finished, from the events they streamed.

        A reservation of tonight is left alone: its gate may still be running (a branch's gate runs while the next
        branch's coder session uses the workspace), and the next night settles it on the night it was admitted.
        """
        tonight = self._gate.tonight()
        for orphan in self._gate.pending():
            if orphan.night < tonight:
                self._gate.settle(orphan, self._spent_by(orphan))

    def _spent_by(self, orphan: Admission) -> int:
        assert orphan.call_id is not None
        return self._counter.count(self._events.lines(orphan.call_id))


def _record_of(call_id: str, arm: Arm, outcome: GateOutcome, spent: int) -> CallRecord:
    recorded = GateOutcome(outcome.report, outcome.status, outcome.session_id, spent, outcome.tickets)
    session_id = outcome.session_id or call_id
    return CallRecord(
        call_id, (arm.value, GateMode.LIVE.value), "", stable_json(recorded.to_record()), (), 0, session_id, None
    )


def _outcome_of(record: CallRecord) -> GateOutcome:
    body = parse_stable_json(record.stdout)
    if not isinstance(body, dict):
        raise RecordError(f"the record of {record.call_id} is not a gate outcome")
    return GateOutcome.from_record(body)
