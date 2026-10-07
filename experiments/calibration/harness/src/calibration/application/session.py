"""One nightly session (§6.5): the next unfinished model blocks in the order M3, M2, M1, M4, M5, M6, each launched under
the memory guard, with the memory rules of §6.4 and the cleanup that a guard stop leaves to its parent."""

from __future__ import annotations

import json
from contextlib import AbstractContextManager
from dataclasses import dataclass
from typing import Final, Protocol

from calibration.application.block_support import ItemCatalog, RawRecorder
from calibration.application.deviation_log import DeviationLog
from calibration.application.ports import MemoryProbe, RunStore, ServerFactory
from calibration.application.safety_log import (
    EVENT_FIT,
    EVENT_GUARD,
    EVENT_PRESSURE_STOP,
    EVENT_SHUTDOWN,
    EVENT_WINDOW,
    SafetyLog,
)
from calibration.domain.constants import (
    EXIT_FAILURE,
    EXIT_GUARD_INTERRUPTED,
    EXIT_GUARD_REFUSED,
    EXIT_OK,
    EXIT_PAUSED,
    MODELS_LOCK_FILE,
)
from calibration.domain.deviations import DeviationKind, standing_of
from calibration.domain.lock import MEMORY_FIT_REASON, MEMORY_PRESSURE_REASON, LockedModel, ModelsLock, PinStatus
from calibration.domain.memory_rules import FitDecision, blocked_by_pressure, dynamic_decision
from calibration.domain.models import ModelSpec, models_in_block_order
from calibration.domain.ports import Clock
from calibration.domain.schedule import Pass
from calibration.domain.window import WINDOW_CLOSED_TEXT, RunWindow, is_window_open

NO_SCOPE: Final = ["none"]
DEFERRAL_DECISION: Final = FitDecision.DEFER.value


class BlockLauncher(Protocol):
    def launch(self, model_id: str) -> int:
        """Runs `run --model <id>` as one guarded job and returns its exit code."""
        ...


@dataclass(frozen=True)
class SessionDeps:
    store: RunStore
    clock: Clock
    memory: MemoryProbe
    launcher: BlockLauncher
    servers: ServerFactory
    lock: AbstractContextManager[None]
    window: RunWindow | None


class RunSession:
    def __init__(self, deps: SessionDeps) -> None:
        self._deps = deps
        self._store = deps.store
        self._log = DeviationLog(deps.store)
        self._safety = SafetyLog(deps.store)

    def run(self) -> int:
        with self._deps.lock:
            return self._run_blocks()

    def _run_blocks(self) -> int:
        lock = ModelsLock.from_record(json.loads(self._store.read_text(MODELS_LOCK_FILE)))
        schedule = ItemCatalog(self._store).schedule()
        failed = False
        for model in models_in_block_order():
            locked = lock.entry(model.model_id)
            if self._skip(model.model_id, locked, schedule.pass1, schedule.pass2):
                continue
            if not is_window_open(self._deps.window, self._deps.clock.now()):
                return self._window_closed()
            if not self._fits(model.model_id, locked, lock.total_memory_bytes):
                continue
            code = self._deps.launcher.launch(model.model_id)
            self._cleanup(model)
            if code == EXIT_OK:
                continue
            if code == EXIT_PAUSED:
                return EXIT_PAUSED
            if code == EXIT_GUARD_REFUSED:
                self._guard_refusal(model.model_id, code)
                return EXIT_PAUSED
            if code == EXIT_GUARD_INTERRUPTED:
                self._pressure_stop(model.model_id)
                return EXIT_PAUSED
            failed = True
        return EXIT_FAILURE if failed else EXIT_OK

    def _skip(self, model_id: str, locked: LockedModel, pass1: tuple[str, ...], pass2: tuple[str, ...]) -> bool:
        if locked.status is PinStatus.BLOCKED or standing_of(self._log.entries(), [model_id]).blocked:
            return True
        recorder = RawRecorder(self._store, model_id, "", locked.model_digest)
        return recorder.complete(Pass.ONE, pass1) and recorder.complete(Pass.TWO, pass2)

    def _window_closed(self) -> int:
        self._safety.append(EVENT_WINDOW, None, self._deps.clock.now(), {"open": False})
        self._log.record(DeviationKind.PAUSE, NO_SCOPE, WINDOW_CLOSED_TEXT)
        return EXIT_PAUSED

    def _fits(self, model_id: str, locked: LockedModel, total_memory: int) -> bool:
        """The dynamic rule of §6.4: load only when the free memory covers F + 0.15 T."""
        reading = self._deps.memory.reading()
        prior = sum(
            1
            for event in self._safety.events()
            if event.get("event") == EVENT_FIT
            and event.get("model") == model_id
            and event.get("decision") == DEFERRAL_DECISION
        )
        check = dynamic_decision(reading.free_percent, total_memory, locked.footprint_bytes, prior)
        self._safety.append(
            EVENT_FIT,
            model_id,
            self._deps.clock.now(),
            {
                "free_percent": check.free_percent,
                "footprint_bytes": check.footprint,
                "required_bytes": check.required,
                "available_bytes": check.available,
                "deferrals": check.deferrals,
                "decision": check.decision.value,
            },
        )
        if check.decision is FitDecision.LOAD:
            return True
        if check.decision is FitDecision.BLOCK_MEMORY_FIT:
            self._log.record(
                DeviationKind.BLOCKED, [model_id], f"{MEMORY_FIT_REASON}: deferred in {check.deferrals} sessions"
            )
        else:
            self._log.record(DeviationKind.PAUSE, NO_SCOPE, f"{model_id} deferred: free memory is below F + 0.15 T")
        return False

    def _guard_refusal(self, model_id: str, code: int) -> None:
        reading = self._deps.memory.reading()
        self._safety.append(
            EVENT_GUARD,
            model_id,
            self._deps.clock.now(),
            {"exit_code": code, "free_percent": reading.free_percent, "pressure_normal": reading.pressure_normal},
        )
        self._log.record(DeviationKind.PAUSE, NO_SCOPE, f"the block of {model_id} stopped with exit code {code}")

    def _pressure_stop(self, model_id: str) -> None:
        stops = 1 + sum(
            1
            for event in self._safety.events()
            if event.get("event") == EVENT_PRESSURE_STOP and event.get("model") == model_id
        )
        self._safety.append(EVENT_PRESSURE_STOP, model_id, self._deps.clock.now(), {"stops": stops})
        self._log.record(DeviationKind.PAUSE, NO_SCOPE, f"memory pressure stopped the block of {model_id}")
        if blocked_by_pressure(stops):
            self._log.record(DeviationKind.BLOCKED, [model_id], f"{MEMORY_PRESSURE_REASON}: {stops} stops")

    def _cleanup(self, model: ModelSpec) -> None:
        """After a block, and after any guard stop: unload, check the server list is empty, stop, check the port."""
        report = self._deps.servers.server_for(model).stop()
        self._safety.append(
            EVENT_SHUTDOWN,
            model.model_id,
            self._deps.clock.now(),
            {
                "by": "session",
                "unloaded": report.unloaded,
                "models_after_unload": list(report.models_after_unload),
                "server_stopped": report.server_stopped,
                "port_free": report.port_free,
            },
        )
        if not report.clean:
            self._log.record(
                DeviationKind.PROTOCOL, [model.model_id], "the cleanup after the block found the server not stopped"
            )
