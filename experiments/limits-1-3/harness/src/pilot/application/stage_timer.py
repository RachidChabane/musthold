"""Wall-clock record of each stage in `timing.json` (RUN_RECORDS)."""

from __future__ import annotations

import time
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from datetime import UTC, datetime

from pilot.application.ports import RunStore
from pilot.domain.pipeline_constants import TIMING_FILENAME

Clock = Callable[[], datetime]


def _utc_now() -> datetime:
    return datetime.now(UTC)


class StageTimer:
    """Seconds accumulate over resumed runs of a stage; `started` keeps the first start."""

    def __init__(self, store: RunStore, clock: Clock = _utc_now) -> None:
        self._store = store
        self._clock = clock

    @contextmanager
    def stage(self, name: str) -> Iterator[None]:
        started = self._clock()
        began = time.monotonic()
        try:
            yield
        finally:
            self._record(name, started, self._clock(), time.monotonic() - began)

    def _record(self, name: str, started: datetime, finished: datetime, seconds: float) -> None:
        timing = self._store.read_json(TIMING_FILENAME) if self._store.exists(TIMING_FILENAME) else {}
        previous = timing.get(name, {})
        timing[name] = {
            "started": previous.get("started", started.isoformat()),
            "finished": finished.isoformat(),
            "seconds": previous.get("seconds", 0.0) + seconds,
        }
        self._store.write_json(TIMING_FILENAME, timing)
