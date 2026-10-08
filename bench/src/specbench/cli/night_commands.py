"""The `night-budget` verb: the share of tonight's cap that a night job may spend, computed under `budget.lock`."""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import date
from pathlib import Path

from specbench.adapters.clock import SystemClock
from specbench.adapters.machine_slots import default_state_directory
from specbench.adapters.night_state import (
    BUDGET_LOCK_FILE_NAME,
    CANARY_RESERVE_FILE_NAME,
    FileCanaryReserve,
    FileConstructionProgress,
    FileNightLedgers,
    budget_lock,
)
from specbench.application.night_budget import NightBudget, NightSettings
from specbench.cli.commands import EXIT_OK, Console
from specbench.domain.arms import Harness
from specbench.domain.night_budget import NightJob
from specbench.domain.pacing import DEFAULT_RUN_WINDOW, RunWindow


@dataclass(frozen=True)
class NightBudgetRequest:
    """Everything one `night-budget` call needs; the shell front door fills in the repository's default paths."""

    job: NightJob
    harness: Harness
    night: date | None
    caps: dict[Harness, int]
    construct_allowance: int
    canary_reserve: int | None
    record_canary: int | None
    pilot_ledger: Path
    construct_ledger: Path
    sample_ledger: Path
    codex_ledger: Path
    selection: Path
    corpus: Path
    state_dir: Path | None
    explain: bool


def night_of_now() -> date:
    window = RunWindow.parse(DEFAULT_RUN_WINDOW)
    now = SystemClock().now()
    return now.date() if window is None else window.night_of(now)


def run_night_budget(request: NightBudgetRequest, console: Console) -> int:
    state = request.state_dir or default_state_directory()
    canaries = FileCanaryReserve(state / CANARY_RESERVE_FILE_NAME)
    ledgers = FileNightLedgers(
        {
            (NightJob.PILOT, Harness.CLAUDE_CODE): request.pilot_ledger,
            (NightJob.PILOT, Harness.CODEX): request.pilot_ledger,
            (NightJob.CONSTRUCT, Harness.CLAUDE_CODE): request.construct_ledger,
            (NightJob.SAMPLE, Harness.CLAUDE_CODE): request.sample_ledger,
            (NightJob.SAMPLE, Harness.CODEX): request.codex_ledger,
        }
    )
    explicit_reserve = {} if request.canary_reserve is None else {request.harness: request.canary_reserve}
    budget = NightBudget(
        ledgers,
        FileConstructionProgress(request.selection, request.corpus),
        canaries,
        NightSettings(request.caps, request.construct_allowance, explicit_reserve),
    )
    night = request.night or night_of_now()
    with budget_lock(state / BUDGET_LOCK_FILE_NAME):
        if request.record_canary is not None:
            canaries.record(request.harness, request.record_canary)
        if request.explain:
            console.say(json.dumps(budget.explain(request.harness, night), indent=2, sort_keys=True))
        else:
            console.say(str(budget.share(request.job, request.harness, night)))
    return EXIT_OK
