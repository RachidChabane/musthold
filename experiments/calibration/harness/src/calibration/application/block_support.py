"""Collaborators of a model block: the items, the raw records, the machine gate and the block timing."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass
from datetime import datetime
from typing import Final

from calibration.application.exchange import ExchangeResult
from calibration.application.layout import (
    item_path,
    raw_dir,
    raw_meta_path,
    raw_request_path,
    raw_response_path,
)
from calibration.application.ports import HeavyWorkProbe, RunStore
from calibration.application.safety_log import EVENT_HEAVY_WORK, SafetyLog
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.constants import (
    HEAVY_WAIT_SECONDS,
    SCHEDULE_FILE,
    TIMING_FILE,
)
from calibration.domain.items import Item
from calibration.domain.jsonread import json_object, sequence, text
from calibration.domain.ports import Clock, Sleeper
from calibration.domain.records import RawMeta
from calibration.domain.schedule import Pass, Schedule
from calibration.domain.window import RunWindow, is_window_open

EARLIER_ATTEMPT_FILE: Final = "response.attempt1.bin"
P95_FRACTION: Final = 0.95
MEDIAN_FRACTION: Final = 0.5


class ItemCatalog:
    """The items and the schedule of a run, read from its directory."""

    def __init__(self, store: RunStore) -> None:
        self._store = store
        self._cache: dict[str, Item] = {}

    def schedule(self) -> Schedule:
        body = json_object(json.loads(self._store.read_text(SCHEDULE_FILE)), SCHEDULE_FILE)
        return Schedule(
            tuple(text(i, "pass1 id") for i in sequence(body.get("pass1"), "pass1")),
            tuple(text(i, "pass2 id") for i in sequence(body.get("pass2"), "pass2")),
        )

    def item(self, request_id: str) -> Item:
        if request_id not in self._cache:
            self._cache[request_id] = Item.from_record(json.loads(self._store.read_text(item_path(request_id))))
        return self._cache[request_id]


class RawRecorder:
    """Writes the raw record of one request; `response.bin` comes last, so a request whose response file exists is
    complete and is not resent."""

    def __init__(self, store: RunStore, model_id: str, server_version: str, model_digest: str) -> None:
        self._store = store
        self._model_id = model_id
        self._server_version = server_version
        self._model_digest = model_digest

    def has_response(self, which: Pass, request_id: str) -> bool:
        return self._store.exists(raw_response_path(self._model_id, which, request_id))

    def complete(self, which: Pass, request_ids: tuple[str, ...]) -> bool:
        return all(self.has_response(which, request_id) for request_id in request_ids)

    def record(
        self, which: Pass, request_id: str, body: bytes, result: ExchangeResult, prefix_cache_hit: bool | None
    ) -> RawMeta:
        final = result.final
        meta = RawMeta(
            http_status=final.status,
            attempt=result.number,
            started=final.started,
            finished=final.finished,
            latency_ms=final.latency_ms,
            server_version=self._server_version,
            model_digest=self._model_digest,
            prefix_cache_hit=prefix_cache_hit,
            transport_error=final.transport_error,
        )
        self._store.write_bytes(raw_request_path(self._model_id, which, request_id), body)
        if result.earlier is not None:
            earlier_path = f"{raw_dir(self._model_id, which, request_id)}/{EARLIER_ATTEMPT_FILE}"
            self._store.write_bytes(earlier_path, result.earlier.body)
        self._store.write_text(raw_meta_path(self._model_id, which, request_id), document_json(meta.to_record()))
        self._store.write_bytes(raw_response_path(self._model_id, which, request_id), final.body)
        return meta


class MachineGate:
    """`heavy-work-check` of §6.1: no other heavy local work, before the block and again every 25 requests."""

    def __init__(
        self, probe: HeavyWorkProbe, safety: SafetyLog, clock: Clock, sleeper: Sleeper, window: RunWindow | None
    ) -> None:
        self._probe = probe
        self._safety = safety
        self._clock = clock
        self._sleeper = sleeper
        self._window = window

    def clear(self, model_id: str) -> bool:
        report = self._probe.check()
        details: dict[str, JsonValue] = {"clear": report.clear, "detail": report.describe()}
        self._safety.append(EVENT_HEAVY_WORK, model_id, self._clock.now(), details)
        return report.clear

    def wait_until_clear(self, model_id: str) -> bool:
        """Waits in 5-minute steps within the window; False when the window closes first."""
        while not self.clear(model_id):
            self._sleeper.sleep(HEAVY_WAIT_SECONDS)
            if not is_window_open(self._window, self._clock.now()):
                return False
        return True


def percentile(sorted_values: list[float], fraction: float) -> float:
    """Nearest rank of a non-empty ascending list."""
    rank = max(1, math.ceil(fraction * len(sorted_values)))
    return sorted_values[rank - 1]


@dataclass
class BlockTiming:
    """The requests of one block: its start, its end and the median and p95 latency."""

    started: datetime
    latencies_ms: list[float]

    def record(self, store: RunStore, model_id: str, finished: datetime) -> None:
        document: dict[str, JsonValue] = {}
        if store.exists(TIMING_FILE):
            document = dict(json_object(json.loads(store.read_text(TIMING_FILE)), TIMING_FILE))
        ordered = sorted(self.latencies_ms)
        entry: dict[str, JsonValue] = {
            "started": self.started.isoformat(),
            "finished": finished.isoformat(),
            "requests": len(ordered),
            "median_latency_ms": percentile(ordered, MEDIAN_FRACTION) if ordered else None,
            "p95_latency_ms": percentile(ordered, P95_FRACTION) if ordered else None,
        }
        document[f"{model_id}/{self.started.isoformat()}"] = entry
        store.write_text(TIMING_FILE, document_json(document))
