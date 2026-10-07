"""The calibration half of the synthetic world: a run directory as the harness archives it (§15.1)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from pathlib import Path
from typing import Final

from calibration.domain.canonical import JsonValue, document_json, line_json, sha256_hex
from calibration.domain.constants import (
    CANARY_DIR,
    DEVIATIONS_FILE,
    ENCODING,
    EXCLUDED_FILE,
    ITEMS_DIR,
    KEV_COMMIT,
    MANIFEST_FILE,
    META_FILE,
    MODELS_LOCK_FILE,
    OLLAMA_VERSION,
    PILOT_INPUTS_FILE,
    RAW_DIR,
    REQUEST_FILE,
    RESPONSE_FILE,
    SAFETY_FILE,
    SCHEDULE_FILE,
    SERVER_DIR,
    TIMING_FILE,
)
from calibration.domain.deviations import DeviationEntry, DeviationKind, render_deviations
from calibration.domain.items import KindFamily
from calibration.domain.lock import MEMORY_FIT_REASON, PinStatus
from calibration.domain.models import MODELS, ModelSpec
from calibration.domain.records import RawMeta
from calibration.domain.schedule import Pass, Schedule, request_slug
from support.world_items import TEMPLATE_DIGEST, PlannedItem, planned_items
from support.world_model import ITEM_TESTS, MEASURED_MUTANTS, ORACLED_MUTANTS, TESTS
from support.world_wire import Fault, ModelBehaviour, StoredExchange, exchange_for, prefix_cache_hit

RUN_ID: Final = "run-01"
ANALYSIS_COMMIT: Final = "a" * 40
FREEZE_COMMIT: Final = "b" * 40
PREREG_COMMIT: Final = "c" * 40
HARNESS_COMMIT: Final = "d" * 40
TOTAL_MEMORY_BYTES: Final = 64 * 1024**3
FOOTPRINT_BYTES: Final = 8 * 1024**3
BLOCK_TIMESTAMP: Final = "2026-10-08T22:00:00"
BLOCK_NAME: Final = "2026-10-08T22-00-00"
EXCHANGE_FINISH: Final = "2026-10-08T22:00:01"
MASKER_REASON: Final = "masked_input_leaks"
SAFETY_EVENTS: Final = (
    {"event": "fit-check", "model": "M1", "decision": "run"},
    {"event": "fit-check", "model": "M5", "decision": "defer"},
    {"event": "pressure-stop", "model": "M4"},
    {"event": "heavy-work-check", "clear": False},
    {"event": "heavy-work-check", "clear": True},
)


def _default_behaviours() -> dict[str, ModelBehaviour]:
    alignment = sorted(p.item.request_id for p in planned_items() if p.item.family is KindFamily.ALIGNMENT)
    oracled = {mutant.mutant_id for mutant in ORACLED_MUTANTS}
    screens = sorted(
        p.item.request_id for p in planned_items() if p.item.family is KindFamily.SCREEN and p.item.unit in oracled
    )
    return {
        "M1": ModelBehaviour(
            0.85,
            0.88,
            0.85,
            0.6,
            flip_rate=0.03,
            faults={screens[1]: Fault.INVALID_TWICE, screens[4]: Fault.NO_RESPONSE},
        ),
        "M2": ModelBehaviour(0.75, 0.8, 0.8, 0.5, flip_rate=0.03, faults={screens[2]: Fault.CONFIDENCE_OUT_OF_RANGE}),
        "M3": ModelBehaviour(
            0.55,
            0.65,
            0.7,
            -0.6,
            flip_rate=0.05,
            faults={screens[0]: Fault.STATE_TOO_LARGE, screens[3]: Fault.STATE_TOO_LARGE},
        ),
        "M4": ModelBehaviour(
            0.65,
            0.7,
            0.6,
            0.3,
            flip_rate=0.05,
            faults={screens[5]: Fault.STATE_TOO_LARGE, alignment[1]: Fault.TRANSPORT_FAILURE},
        ),
        "M5": ModelBehaviour(
            0.8,
            0.85,
            0.75,
            0.5,
            flip_rate=0.03,
            faults={alignment[0]: Fault.STATE_TOO_LARGE, alignment[2]: Fault.INVALID_TWICE},
        ),
        "M6": ModelBehaviour(1.0, 0.95, 1.0, 0.0, jitter=0.0),
    }


BASE_DEVIATIONS: Final = (
    DeviationEntry.create(DeviationKind.PROVENANCE, ["none"], "the OS build changed between blocks"),
    DeviationEntry.create(DeviationKind.PAUSE, ["none"], "the run window closed during the block of M4"),
)


@dataclass(frozen=True)
class WorldSpec:
    """What varies between the worlds of the tests."""

    behaviours: Mapping[str, ModelBehaviour] = field(default_factory=_default_behaviours)
    blocked: frozenset[str] = frozenset()
    deviations: tuple[DeviationEntry, ...] = BASE_DEVIATIONS
    screen_blocked: bool = False


def _write(root: Path, relative: str, content: str | bytes) -> None:
    path = root / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(content, bytes):
        path.write_bytes(content)
    else:
        path.write_text(content, encoding=ENCODING)


def _lock_entry(model: ModelSpec, blocked: bool) -> dict[str, JsonValue]:
    entry: dict[str, JsonValue] = {
        "status": (PinStatus.BLOCKED if blocked else PinStatus.PINNED).value,
        "block_reason": MEMORY_FIT_REASON if blocked else None,
        "footprint_bytes": FOOTPRINT_BYTES,
        "model_digest": "" if blocked else sha256_hex(f"digest {model.model_id}"),
        "run_reference": None if model.is_ollama or blocked else f"{model.repository}@{'e' * 40}",
    }
    if not blocked:
        entry["identifier" if model.is_ollama else "repository"] = model.identifier
    return entry


def lock_record(blocked: frozenset[str]) -> dict[str, JsonValue]:
    """`models.lock.json` as `pin` writes it."""
    ollama = {m.model_id: _lock_entry(m, m.model_id in blocked) for m in MODELS if m.is_ollama}
    kev = {m.model_id: _lock_entry(m, m.model_id in blocked) for m in MODELS if not m.is_ollama}
    return {
        "total_memory_bytes": TOTAL_MEMORY_BYTES,
        "ollama": {"version": OLLAMA_VERSION, "models": ollama},
        "kev": {"commit": KEV_COMMIT, "uv_lock_sha256": sha256_hex("uv.lock"), "versions": {}, "models": kev},
    }


def manifest_record(spec: WorldSpec) -> dict[str, JsonValue]:
    return {
        "run_id": RUN_ID,
        "prereg_commit": PREREG_COMMIT,
        "harness_commit": HARNESS_COMMIT,
        "pilot": {"freeze_commit": FREEZE_COMMIT, "analysis_commit": ANALYSIS_COMMIT},
        "snapshot_digest": sha256_hex("snapshot"),
        "builders": {"source_digests": {"alignment": sha256_hex("alignment")}, "binary_digests": {}},
        "template_digests": {"alignment": TEMPLATE_DIGEST, "discpower": TEMPLATE_DIGEST},
        "source2_prompt_digests": {"alignment": sha256_hex("alignment.md"), "validity": sha256_hex("validity.md")},
        "counts": {
            "alignment_items": len(ITEM_TESTS),
            "screen_items": len(MEASURED_MUTANTS),
            "screen_blocked": spec.screen_blocked,
        },
        "product_export": {"digest": sha256_hex("export")},
    }


def excluded_record() -> dict[str, JsonValue]:
    rejected = next(test for test in TESTS if test.masked)
    return {
        "masker_rejections": [
            {
                "request_id": f"A|{rejected.nodeid}",
                "nodeid": rejected.nodeid,
                "label": rejected.label,
                "reason": MASKER_REASON,
                "detail": "the test name leaks the requirement",
            }
        ],
        "witness_unmapped": [],
    }


def _write_exchange(
    root: Path, model: ModelSpec, which: Pass, planned: PlannedItem, behaviour: ModelBehaviour, spec_pass: bool
) -> None:
    item = planned.item
    accuracy = behaviour.alignment_accuracy if item.family is KindFamily.ALIGNMENT else behaviour.screen_accuracy
    stored = exchange_for(
        model, behaviour, item.request_id, planned.plans, family_accuracy=accuracy, second_pass=spec_pass
    )
    if stored is None:
        return
    directory = f"{RAW_DIR}/{model.model_id}/{which.value}/{request_slug(item.request_id)}"
    _write(root, f"{directory}/{REQUEST_FILE}", item.bodies[model.identifier].encode(ENCODING))
    _write(root, f"{directory}/{RESPONSE_FILE}", stored.body)
    _write(root, f"{directory}/{META_FILE}", document_json(_meta(model, item.request_id, stored).to_record()))


def _meta(model: ModelSpec, request_id: str, stored: StoredExchange) -> RawMeta:
    return RawMeta(
        http_status=stored.status,
        attempt=stored.attempt,
        started=BLOCK_TIMESTAMP,
        finished=EXCHANGE_FINISH,
        latency_ms=1000.0,
        server_version=OLLAMA_VERSION if model.is_ollama else KEV_COMMIT,
        model_digest=sha256_hex(f"digest {model.model_id}"),
        prefix_cache_hit=prefix_cache_hit(model, request_id),
        transport_error=stored.transport_error,
    )


def _write_model(
    root: Path, model: ModelSpec, schedule: Schedule, by_id: Mapping[str, PlannedItem], spec: WorldSpec
) -> None:
    behaviour = spec.behaviours[model.model_id]
    for which in (Pass.ONE, Pass.TWO):
        for request_id in schedule.ids_of(which):
            _write_exchange(root, model, which, by_id[request_id], behaviour, which is Pass.TWO)
    _write(root, f"{CANARY_DIR}/{model.model_id}.json", document_json({"passed": True, "checks": []}))
    _write(
        root, f"{SERVER_DIR}/{model.model_id}/{BLOCK_NAME}.json", document_json({"server_version": model.identifier})
    )


def _blocked_entries(blocked: frozenset[str]) -> list[DeviationEntry]:
    return [
        DeviationEntry.create(DeviationKind.BLOCKED, [model_id], f"{MEMORY_FIT_REASON}: footprint {FOOTPRINT_BYTES}")
        for model_id in sorted(blocked)
    ]


def write_run(root: Path, spec: WorldSpec) -> None:
    """Writes the whole calibration run directory below `root`."""
    planned = planned_items()
    by_id = {p.item.request_id: p for p in planned}
    schedule = Schedule.of(by_id)
    for p in planned:
        _write(root, f"{ITEMS_DIR}/{request_slug(p.item.request_id)}.json", document_json(p.item.to_record()))
    _write(root, SCHEDULE_FILE, document_json({"pass1": list(schedule.pass1), "pass2": list(schedule.pass2)}))
    _write(root, MANIFEST_FILE, document_json(manifest_record(spec)))
    _write(root, MODELS_LOCK_FILE, document_json(lock_record(spec.blocked)))
    _write(root, EXCLUDED_FILE, document_json(excluded_record()))
    _write(
        root,
        PILOT_INPUTS_FILE,
        document_json({"files": {"bindings.json": sha256_hex("bindings")}, "seal_mismatches": []}),
    )
    _write(root, DEVIATIONS_FILE, render_deviations([*spec.deviations, *_blocked_entries(spec.blocked)]))
    _write(
        root, TIMING_FILE, document_json({"M3": {"started": BLOCK_TIMESTAMP, "ended": EXCHANGE_FINISH, "requests": 1}})
    )
    _write(root, SAFETY_FILE, "".join(line_json(event) for event in SAFETY_EVENTS))
    for model in MODELS:
        if model.model_id not in spec.blocked:
            _write_model(root, model, schedule, by_id, spec)
