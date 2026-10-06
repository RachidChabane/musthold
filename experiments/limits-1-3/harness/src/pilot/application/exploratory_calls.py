"""Calls of an exploratory stream: made inside the window, with a per-call status that never reaches the deviations."""

from __future__ import annotations

from collections.abc import Callable, Sequence
from datetime import UTC, datetime

from pilot.application.model_caller import CallRunner, CallSpec
from pilot.application.ports import RunStore
from pilot.domain.exploratory import CallStatus, Window
from pilot.domain.model_call import MissingResult, ModelOutcome
from pilot.domain.pipeline_constants import EXPLORATORY_STATUS_FILENAME
from pilot.domain.quota import QuotaExhausted

Clock = Callable[[], datetime]
BLOCKED_FAILURE = "exploratory window closed before the call was made"


def utc_now() -> datetime:
    return datetime.now(UTC)


class ExploratoryLedger:
    """`exploratory_status.json`: stream -> call id -> complete | blocked | pending."""

    def __init__(self, store: RunStore) -> None:
        self._store = store

    def statuses(self) -> dict[str, dict[str, str]]:
        return dict(self._store.read_json(EXPLORATORY_STATUS_FILENAME)) if self._store.exists(EXPLORATORY_STATUS_FILENAME) else {}

    def mark(self, stream: str, status_by_call: dict[str, CallStatus]) -> None:
        statuses = self.statuses()
        merged = {**statuses.get(stream, {}), **{call_id: status.value for call_id, status in status_by_call.items()}}
        statuses[stream] = dict(sorted(merged.items()))
        self._store.write_json(EXPLORATORY_STATUS_FILENAME, dict(sorted(statuses.items())))


class WindowNotOpen(Exception):
    """The stream's window has not opened yet; nothing was called or recorded."""


class WindowedCaller:
    """A `CallRunner` for one exploratory stream.

    Inside the window every call is made and recorded as complete; a quota stop leaves the others
    pending. Once the window has closed no call is made: recorded calls are served, the others are
    blocked and answered as missing, so the stages that consume them report them as missing.
    """

    def __init__(self, inner: CallRunner, stream: str, ledger: ExploratoryLedger, window: Window, clock: Clock = utc_now) -> None:
        self._inner = inner
        self._stream = stream
        self._ledger = ledger
        self._window = window
        self._clock = clock

    def is_recorded(self, call_id: str) -> bool:
        return self._inner.is_recorded(call_id)

    def call_all(self, stage: str, specs: Sequence[CallSpec]) -> dict[str, ModelOutcome]:
        now = self._clock()
        if self._window.not_yet_open(now):
            raise WindowNotOpen(f"{self._stream} opens at {self._window.opens}")
        recorded = [spec for spec in specs if self._inner.is_recorded(spec.call_id)]
        rest = [spec for spec in specs if spec not in recorded]
        if self._window.is_closed(now):
            self._mark(recorded, rest, CallStatus.BLOCKED)
            outcomes = self._inner.call_all(stage, recorded)
            return {**outcomes, **{spec.call_id: MissingResult((BLOCKED_FAILURE,), ()) for spec in rest}}
        self._mark(recorded, rest, CallStatus.PENDING)
        try:
            outcomes = self._inner.call_all(stage, specs)
        except QuotaExhausted:
            done = [spec for spec in specs if self._inner.is_recorded(spec.call_id)]
            self._mark(done, [spec for spec in specs if spec not in done], CallStatus.PENDING)
            raise
        self._mark(list(specs), [], CallStatus.PENDING)
        return outcomes

    def _mark(self, done: Sequence[CallSpec], rest: Sequence[CallSpec], rest_status: CallStatus) -> None:
        statuses = {spec.call_id: CallStatus.COMPLETE for spec in done}
        statuses.update({spec.call_id: rest_status for spec in rest})
        self._ledger.mark(self._stream, statuses)
