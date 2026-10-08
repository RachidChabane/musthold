"""One computation of the nightly budget, for every job that spends a harness's cap."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from datetime import date

from specbench.application.ports import CanaryReserveStore, ConstructionProgress, NightLedgers
from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.night_budget import (
    DEFAULT_CLAUDE_CANARY_RESERVE,
    DEFAULT_CODEX_CANARY_RESERVE,
    NightInputs,
    NightJob,
    share_of,
)

DEFAULT_CANARY_RESERVE = {
    Harness.CLAUDE_CODE: DEFAULT_CLAUDE_CANARY_RESERVE,
    Harness.CODEX: DEFAULT_CODEX_CANARY_RESERVE,
}


@dataclass(frozen=True)
class NightSettings:
    """The nightly cap per harness (the Codex cap is its own), construction's ceiling, and an explicit canary reserve."""

    caps: Mapping[Harness, int]
    construct_allowance: int
    canary_reserve: Mapping[Harness, int]


class NightBudget:
    """Hands out the cap of a night in priority order: pilot, canary reserve, construction, then the sample.

    The Codex cap is separate: nothing on one harness is ever subtracted from the other's cap.
    """

    def __init__(
        self,
        ledgers: NightLedgers,
        progress: ConstructionProgress,
        canaries: CanaryReserveStore,
        settings: NightSettings,
    ) -> None:
        self._ledgers = ledgers
        self._progress = progress
        self._canaries = canaries
        self._settings = settings

    def share(self, job: NightJob, harness: Harness, night: date) -> int:
        return share_of(job, self._inputs(harness, night))

    def explain(self, harness: Harness, night: date) -> dict[str, JsonValue]:
        inputs = self._inputs(harness, night)
        return {
            "canary_reserve": inputs.canary_reserve,
            "cap": inputs.cap,
            "construct_allowance": inputs.construct_allowance,
            "construct_pending": inputs.construct_pending,
            "construct_spent": inputs.construct_spent,
            "harness": harness.value,
            "night": night.isoformat(),
            "pilot_spent": inputs.pilot_spent,
            "sample_spent": self._ledgers.read(NightJob.SAMPLE, harness).calls_on(night, harness),
            "shares": {job.value: share_of(job, inputs) for job in NightJob},
        }

    def _inputs(self, harness: Harness, night: date) -> NightInputs:
        has_construction = harness is Harness.CLAUDE_CODE
        return NightInputs(
            cap=self._settings.caps[harness],
            pilot_spent=self._ledgers.read(NightJob.PILOT, harness).calls_on(night, harness),
            canary_reserve=self._canary_reserve(harness),
            construct_allowance=self._settings.construct_allowance if has_construction else 0,
            construct_spent=self._ledgers.read(NightJob.CONSTRUCT, harness).calls_on(night, harness),
            construct_pending=has_construction and self._progress.has_unsealed_task(),
        )

    def _canary_reserve(self, harness: Harness) -> int:
        explicit = self._settings.canary_reserve.get(harness)
        if explicit is not None:
            return explicit
        measured = self._canaries.measured(harness)
        return DEFAULT_CANARY_RESERVE[harness] if measured is None else measured
