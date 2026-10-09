"""Pacing of the gate host: a live gate reserves its executions before it starts and settles them when it ends.

A gate is one session that spends dozens of executions in a burst, so the single-call admission of `CallGate` cannot
bound it. Here a live gate reserves `r` executions on tonight's share (and pauses with exit 3 when `r` does not fit),
the adapter streams the session's events to the call directory, and the charge is the executions those events show,
on the night of admission. A gate that is killed leaves its reservation behind; the next gate settles it from the
events file before reserving for itself.

A signed gate may need CI round trips (ADR 0011, decision 12). The inner host then returns a paused outcome with the
reason `ci_round_pending`, and this decorator reserves one round on its own, under the identity of that round, and calls
the inner host again. The entries of a gate (its session, then each round, then the settle of the last round) are charged each as they end and recorded
as steps, so a gate killed between rounds resumes from the next round with the earlier spend still counted.
"""

from __future__ import annotations

from dataclasses import dataclass, field, replace
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
from specbench.domain.ci_seed import CI_ROUND_TRIPS
from specbench.domain.gate import GateMode, GateOutcome, GatePause, GateStatus, TicketUsage
from specbench.domain.jsonread import RecordError
from specbench.domain.reservation import ExecutionHistory
from specbench.domain.sessions import CallRecord, ci_round_call_id, live_gate_call_id

SESSION_ENTRY = 0
"""The entry of a gate that runs its session and settles it; the entries after it are its CI round trips."""


@dataclass(frozen=True)
class _Carried:
    """What the finished entries of a gate spent: their count, executions and tickets."""

    entries: int = 0
    executions: int = 0
    tickets: tuple[TicketUsage, ...] = field(default_factory=tuple[TicketUsage, ...])

    def after(self, outcome: GateOutcome, spent: int) -> _Carried:
        return _Carried(self.entries + 1, self.executions + spent, (*self.tickets, *outcome.tickets))


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
        carried = self._carried(arm, record_dir)
        while True:
            outcome, spent = self._run_entry(tree, arm, record_dir, carried.entries)
            if outcome.pause is not GatePause.CI_ROUND_PENDING:
                return self._finished(arm, record_dir, outcome, carried, spent)
            carried = carried.after(outcome, spent)
            self._record_step(arm, record_dir, carried.entries - 1, outcome, spent)
            if carried.entries > CI_ROUND_TRIPS + 1:
                return replace(outcome, executions=carried.executions, tickets=carried.tickets)

    def _finished(self, arm: Arm, record_dir: Path, outcome: GateOutcome, carried: _Carried, spent: int) -> GateOutcome:
        """The outcome of the whole gate: the last entry's, with everything the earlier entries spent added to it."""
        merged = replace(outcome, executions=carried.executions + spent, tickets=(*carried.tickets, *outcome.tickets))
        if outcome.status is GateStatus.COMPLETE:
            call_id = live_gate_call_id(arm, record_dir)
            self._calls.write(_record_of(call_id, arm, merged, merged.executions, self._events.lines(call_id)))
        return merged

    def _run_entry(self, tree: Path, arm: Arm, record_dir: Path, entry: int) -> tuple[GateOutcome, int]:
        """Reserves the entry, runs the inner host once, and settles what the entry spent; returns the outcome and the spend."""
        entry_id = self._entry_id(arm, record_dir, entry)
        resumed_reservation = self._gate.holds_reservation(entry_id)
        history = self._history.load()
        size = (
            history.reservation(self._harness, arm, self._criteria)
            if entry == SESSION_ENTRY
            else history.round_reservation(self._harness, arm, self._criteria)
        )
        admission = self._gate.admit_reserved(entry_id, size)
        outcome = self._inner.run_gate(tree, arm, record_dir, GateMode.LIVE)
        spent = self._spent(entry_id, outcome, resumed_reservation)
        self._gate.settle(admission, spent)
        if outcome.status is GateStatus.COMPLETE or outcome.pause is GatePause.CI_ROUND_PENDING:
            self._history.save(self._observed(entry, arm, spent))
        return outcome, spent

    def _observed(self, entry: int, arm: Arm, spent: int) -> ExecutionHistory:
        history = self._history.load()
        if entry == SESSION_ENTRY:
            return history.with_observation(self._harness, arm, self._criteria, spent)
        return history.with_round_observation(self._harness, arm, self._criteria, spent)

    def _entry_id(self, arm: Arm, record_dir: Path, entry: int) -> str:
        """The identity of an entry: the gate's own for its session, the round's for a CI round trip."""
        if entry == SESSION_ENTRY:
            return live_gate_call_id(arm, record_dir)
        return ci_round_call_id(arm, record_dir, entry)

    def _carried(self, arm: Arm, record_dir: Path) -> _Carried:
        carried = _Carried()
        while self._calls.exists(ci_round_call_id(arm, record_dir, carried.entries)):
            step = _outcome_of(self._calls.read(ci_round_call_id(arm, record_dir, carried.entries)))
            carried = carried.after(step, step.executions)
        return carried

    def _record_step(self, arm: Arm, record_dir: Path, entry: int, outcome: GateOutcome, spent: int) -> None:
        """Keeps a finished entry of a gate that goes on, so that a restart counts what it spent."""
        step_id = ci_round_call_id(arm, record_dir, entry)
        events = self._events.lines(self._entry_id(arm, record_dir, entry))
        self._calls.write(_record_of(step_id, arm, outcome, spent, events))

    def _spent(self, entry_id: str, outcome: GateOutcome, resumed_reservation: bool) -> int:
        """A fresh reservation is charged what the session reports. A reservation kept from a killed attempt of the same
        night is charged the events of both attempts, which share one events file."""
        if not resumed_reservation:
            return outcome.executions
        return max(outcome.executions, self._counter.count(self._events.lines(entry_id)))

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


def _record_of(call_id: str, arm: Arm, outcome: GateOutcome, spent: int, events: tuple[str, ...]) -> CallRecord:
    recorded = replace(outcome, executions=spent)
    session_id = outcome.session_id or call_id
    return CallRecord(
        call_id, (arm.value, GateMode.LIVE.value), "", stable_json(recorded.to_record()), events, 0, session_id, None
    )


def _outcome_of(record: CallRecord) -> GateOutcome:
    body = parse_stable_json(record.stdout)
    if not isinstance(body, dict):
        raise RecordError(f"the record of {record.call_id} is not a gate outcome")
    return GateOutcome.from_record(body)
