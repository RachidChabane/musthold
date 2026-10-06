"""Batched model calls: bounded concurrency, per-(model, call type) baselines, clean stop on quota."""

from __future__ import annotations

import threading
from collections.abc import Callable, Mapping, Sequence
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass

from pilot.application.ports import ModelPort
from pilot.domain.call_type import CALL_EFFORT, CallType
from pilot.domain.model_call import (
    CanaryBaseline,
    Effort,
    ModelOutcome,
    ModelPin,
    ModelRequest,
    PayloadCheck,
)
from pilot.domain.pipeline_constants import PROGRESS_INTERVAL
from pilot.domain.quota import QuotaExhausted

Log = Callable[[str], None]
BaselineBook = Mapping[tuple[str, CallType], CanaryBaseline]


class MissingCanary(Exception):
    """A (model, call type) pair has no canary baseline; run `canary` first."""


@dataclass(frozen=True)
class CallSpec:
    call_id: str
    pin: ModelPin
    call_type: CallType
    item: str
    schema_text: str
    validate: PayloadCheck | None = None
    effort: Effort | None = None


class ModelCaller:
    """Runs call specs through a `ModelPort` with at most `max_workers` in flight."""

    def __init__(
        self,
        port: ModelPort,
        baselines: BaselineBook,
        system_prompt: str,
        log: Log,
        max_workers: int,
        require_baseline: bool = True,
    ) -> None:
        self._port = port
        self._baselines = baselines
        self._system_prompt = system_prompt
        self._log = log
        self._max_workers = max_workers
        self._require_baseline = require_baseline

    def call_all(self, stage: str, specs: Sequence[CallSpec]) -> dict[str, ModelOutcome]:
        self._check_baselines(specs)
        outcomes: dict[str, ModelOutcome] = {}
        stop = threading.Event()
        lock = threading.Lock()

        def work(spec: CallSpec) -> None:
            if stop.is_set():
                return
            try:
                outcome = self._port.call(self._request(spec), self._baselines.get((spec.pin.key, spec.call_type)))
            except QuotaExhausted:
                stop.set()
                return
            with lock:
                outcomes[spec.call_id] = outcome
                self._progress(stage, len(outcomes), len(specs))

        with ThreadPoolExecutor(max_workers=self._max_workers) as pool:
            list(pool.map(work, specs))
        if stop.is_set():
            self._log(f"[{stage}] quota or rate limit reached after {len(outcomes)}/{len(specs)} calls; rerun to resume")
            raise QuotaExhausted(stage)
        return outcomes

    def _check_baselines(self, specs: Sequence[CallSpec]) -> None:
        if not self._require_baseline:
            return
        missing = sorted({f"{s.pin.key}/{s.call_type.value}" for s in specs if (s.pin.key, s.call_type) not in self._baselines})
        if missing:
            raise MissingCanary(f"no canary baseline for {missing}; run canary first")

    def _request(self, spec: CallSpec) -> ModelRequest:
        return ModelRequest(
            call_id=spec.call_id,
            pin=spec.pin,
            effort=spec.effort or CALL_EFFORT[spec.call_type],
            item=spec.item,
            schema_text=spec.schema_text,
            system_prompt=self._system_prompt,
            validate=spec.validate,
        )

    def _progress(self, stage: str, done: int, total: int) -> None:
        if done == total or done % PROGRESS_INTERVAL == 0:
            self._log(f"[{stage}] {done}/{total} calls done")
