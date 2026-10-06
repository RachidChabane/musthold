"""Decorator over a `ModelPort`: stops a verb cleanly once the CLI has made its daily budget of new calls."""

from __future__ import annotations

import threading
from collections.abc import Callable
from datetime import date

from pilot.application.ports import ModelPort, RunStore
from pilot.domain.call_budget import (
    CALL_BUDGET_FILENAME,
    CallBudgetReached,
    calls_on,
    with_call,
)
from pilot.domain.model_call import CanaryBaseline, ModelOutcome, ModelRequest

Today = Callable[[], date]
Recorded = Callable[[str], bool]


class BudgetedModelPort:
    """Counts a call when it starts; a call whose result is already recorded is never counted."""

    def __init__(self, inner: ModelPort, store: RunStore, budget: int, today: Today, recorded: Recorded) -> None:
        self._inner = inner
        self._store = store
        self._budget = budget
        self._today = today
        self._recorded = recorded
        self._lock = threading.Lock()

    def call(self, request: ModelRequest, baseline: CanaryBaseline | None) -> ModelOutcome:
        if not self._recorded(request.call_id):
            self._reserve(request)
        return self._inner.call(request, baseline)

    def _reserve(self, request: ModelRequest) -> None:
        with self._lock:
            ledger = self._store.read_json(CALL_BUDGET_FILENAME) if self._store.exists(CALL_BUDGET_FILENAME) else {}
            day = self._today()
            if calls_on(ledger, day, request.pin.cli) >= self._budget:
                raise CallBudgetReached(f"{request.pin.cli.value} {day.isoformat()}")
            self._store.write_json(CALL_BUDGET_FILENAME, with_call(ledger, day, request.pin.cli))
