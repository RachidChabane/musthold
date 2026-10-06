"""The daily call budget: new model calls per UTC day and CLI, persisted in the run directory.

A resumed call (its raw record exists) costs nothing, so a rerun after a pause spends
only on calls that were never made.
"""

from __future__ import annotations

from collections.abc import Mapping
from datetime import date
from typing import Final

from pilot.domain.model_call import Cli
from pilot.domain.quota import QuotaExhausted

DEFAULT_CALL_BUDGET: Final = 250
CALL_BUDGET_FILENAME: Final = "call_budget.json"
CALL_BUDGET_PAUSE_TEXT: Final = "call budget reached"


class CallBudgetReached(QuotaExhausted):
    """The CLI has made its budget of new calls for the UTC day; a rerun on another day resumes."""


def calls_on(ledger: Mapping[str, Mapping[str, int]], day: date, cli: Cli) -> int:
    return ledger.get(day.isoformat(), {}).get(cli.value, 0)


def with_call(ledger: Mapping[str, Mapping[str, int]], day: date, cli: Cli) -> dict[str, dict[str, int]]:
    updated = {key: dict(counts) for key, counts in ledger.items()}
    counts = updated.setdefault(day.isoformat(), {})
    counts[cli.value] = counts.get(cli.value, 0) + 1
    return updated
