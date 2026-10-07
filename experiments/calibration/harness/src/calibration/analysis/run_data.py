"""A run directory as `analyze` reads it (§15.1): the items, the schedule and the raw records, from the bytes on disk."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path

from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    CANARY_DIR,
    DEVIATIONS_FILE,
    ENCODING,
    EXCLUDED_FILE,
    ITEMS_DIR,
    MANIFEST_FILE,
    META_FILE,
    MODELS_LOCK_FILE,
    RAW_DIR,
    REQUEST_FILE,
    RESPONSE_FILE,
    SCHEDULE_FILE,
    SERVER_DIR,
    TIMING_FILE,
)
from calibration.domain.deviations import DeviationEntry, parse_deviations
from calibration.domain.items import Item
from calibration.domain.jsonread import json_object, sequence, text
from calibration.domain.lock import ModelsLock
from calibration.domain.models import MODEL_IDS
from calibration.domain.records import RawMeta
from calibration.domain.schedule import Pass, Schedule, request_slug


@dataclass(frozen=True)
class RawExchange:
    """The stored bytes of one request and its meta."""

    request: bytes
    response: bytes
    meta: RawMeta


@dataclass(frozen=True)
class RunData:
    run_dir: Path
    manifest: Mapping[str, JsonValue]
    lock: ModelsLock
    items: Mapping[str, Item]
    schedule: Schedule
    deviations: tuple[DeviationEntry, ...]
    timing: Mapping[str, JsonValue]
    excluded: Mapping[str, JsonValue]
    canaries: Mapping[str, Mapping[str, JsonValue]]
    cards: Mapping[str, tuple[Mapping[str, JsonValue], ...]]

    def exchange(self, model_id: str, which: Pass, request_id: str) -> RawExchange | None:
        directory = self.run_dir / RAW_DIR / model_id / which.value / request_slug(request_id)
        response = directory / RESPONSE_FILE
        if not response.is_file():
            return None
        return RawExchange(
            (directory / REQUEST_FILE).read_bytes(),
            response.read_bytes(),
            RawMeta.from_record(json.loads((directory / META_FILE).read_text(encoding=ENCODING))),
        )


def _read_json(path: Path) -> Mapping[str, JsonValue]:
    return json_object(json.loads(path.read_text(encoding=ENCODING)), path.name)


def read_run(run_dir: Path) -> RunData:
    """Reads the archive of a run: items, schedule, lock, deviations, timing, canaries and server cards."""
    items = {
        item.request_id: item
        for item in (
            Item.from_record(json.loads(path.read_text(encoding=ENCODING)))
            for path in sorted((run_dir / ITEMS_DIR).iterdir())
            if path.suffix == ".json"
        )
    }
    schedule_record = _read_json(run_dir / SCHEDULE_FILE)
    schedule = Schedule(
        tuple(text(i, "pass1 id") for i in sequence(schedule_record.get("pass1"), "pass1")),
        tuple(text(i, "pass2 id") for i in sequence(schedule_record.get("pass2"), "pass2")),
    )
    deviations_path = run_dir / DEVIATIONS_FILE
    deviations = parse_deviations(deviations_path.read_text(encoding=ENCODING)) if deviations_path.is_file() else []
    timing_path = run_dir / TIMING_FILE
    canaries = {
        model_id: _read_json(run_dir / CANARY_DIR / f"{model_id}.json")
        for model_id in MODEL_IDS
        if (run_dir / CANARY_DIR / f"{model_id}.json").is_file()
    }
    cards = {
        model_id: tuple(
            _read_json(path) for path in sorted((run_dir / SERVER_DIR / model_id).iterdir()) if path.suffix == ".json"
        )
        for model_id in MODEL_IDS
        if (run_dir / SERVER_DIR / model_id).is_dir()
    }
    return RunData(
        run_dir=run_dir,
        manifest=_read_json(run_dir / MANIFEST_FILE),
        lock=ModelsLock.from_record(json.loads((run_dir / MODELS_LOCK_FILE).read_text(encoding=ENCODING))),
        items=items,
        schedule=schedule,
        deviations=tuple(deviations),
        timing=_read_json(timing_path) if timing_path.is_file() else {},
        excluded=_read_json(run_dir / EXCLUDED_FILE),
        canaries=canaries,
        cards=cards,
    )
