"""`safety.jsonl` (§6): every guard reading, heavy-work check, fit computation and window decision."""

from __future__ import annotations

import json
from collections.abc import Mapping
from datetime import datetime
from typing import Final

from calibration.application.ports import RunStore
from calibration.domain.canonical import JsonValue, line_json
from calibration.domain.constants import SAFETY_FILE
from calibration.domain.jsonread import json_object

EVENT_GUARD: Final = "guard-reading"
EVENT_HEAVY_WORK: Final = "heavy-work-check"
EVENT_FIT: Final = "fit-check"
EVENT_WINDOW: Final = "window"
EVENT_BLOCK: Final = "block"
EVENT_SHUTDOWN: Final = "shutdown"
EVENT_PRESSURE_STOP: Final = "pressure-stop"


class SafetyLog:
    """Append-only; each line names its event, the model it concerns and the local time."""

    def __init__(self, store: RunStore) -> None:
        self._store = store

    def append(self, event: str, model_id: str | None, now: datetime, details: Mapping[str, JsonValue]) -> None:
        record: dict[str, JsonValue] = {"event": event, "model": model_id, "time": now.isoformat(), **details}
        self._store.append_text(SAFETY_FILE, line_json(record))

    def events(self) -> list[Mapping[str, JsonValue]]:
        if not self._store.exists(SAFETY_FILE):
            return []
        return [
            json_object(json.loads(line), SAFETY_FILE)
            for line in self._store.read_text(SAFETY_FILE).splitlines()
            if line.strip()
        ]
