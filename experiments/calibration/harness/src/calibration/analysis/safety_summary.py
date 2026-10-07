"""The machine-safety summary (§15.2 item 12): deferrals, pauses and blocks per model."""

from __future__ import annotations

import json
from collections import Counter
from collections.abc import Sequence
from pathlib import Path
from typing import Final

from calibration.domain.canonical import JsonValue
from calibration.domain.constants import ENCODING, SAFETY_FILE
from calibration.domain.deviations import DeviationEntry, DeviationKind
from calibration.domain.jsonread import json_object
from calibration.domain.models import MODEL_IDS

EVENT_FIT: Final = "fit-check"
EVENT_PRESSURE_STOP: Final = "pressure-stop"
EVENT_HEAVY: Final = "heavy-work-check"
DECISION_DEFER: Final = "defer"


def safety_summary(run_dir: Path, deviations: Sequence[DeviationEntry]) -> dict[str, JsonValue]:
    """Per model: how often the dynamic fit rule deferred the block, how often the guard stopped it for memory
    pressure and whether it is blocked; and the pauses of the whole log."""
    deferrals: Counter[str] = Counter()
    stops: Counter[str] = Counter()
    heavy_waits = 0
    path = run_dir / SAFETY_FILE
    if path.is_file():
        for line in path.read_text(encoding=ENCODING).splitlines():
            if not line.strip():
                continue
            event = json_object(json.loads(line), SAFETY_FILE)
            model = event.get("model")
            if event.get("event") == EVENT_FIT and event.get("decision") == DECISION_DEFER and isinstance(model, str):
                deferrals[model] += 1
            elif event.get("event") == EVENT_PRESSURE_STOP and isinstance(model, str):
                stops[model] += 1
            elif event.get("event") == EVENT_HEAVY and event.get("clear") is False:
                heavy_waits += 1
    blocked = {
        identifier
        for entry in deviations
        if entry.kind is DeviationKind.BLOCKED
        for identifier in (MODEL_IDS if entry.covers_all else entry.scope)
        if identifier in MODEL_IDS
    }
    return {
        "per_model": {
            model: {"deferrals": deferrals[model], "pressure_stops": stops[model], "blocked": model in blocked}
            for model in MODEL_IDS
        },
        "pauses": sum(1 for entry in deviations if entry.kind is DeviationKind.PAUSE),
        "heavy_work_waits": heavy_waits,
    }
