"""One model block (§6.1 to §6.5): its server, its canary, pass 1 and pass 2, inside the nightly window."""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from typing import Final

from calibration.application.block_support import BlockTiming, ItemCatalog, MachineGate, RawRecorder
from calibration.application.build_ports import BuiltTools
from calibration.application.canary import CanaryRunner
from calibration.application.deviation_log import DeviationLog
from calibration.application.exchange import Exchange
from calibration.application.layout import canary_path, server_card_path
from calibration.application.manifest import RunManifest
from calibration.application.ports import (
    HarnessIdentity,
    HeavyWorkProbe,
    ModelServer,
    ProvenanceProbe,
    RunStore,
    ServerCard,
    ServerFactory,
)
from calibration.application.safety_log import EVENT_BLOCK, EVENT_SHUTDOWN, EVENT_WINDOW, SafetyLog
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.constants import ENCODING, HEAVY_CHECK_EVERY, MODELS_LOCK_FILE, SERVER_DIR
from calibration.domain.deviations import DeviationKind, standing_of
from calibration.domain.jsonread import json_object
from calibration.domain.lock import LockedModel, ModelsLock, PinStatus
from calibration.domain.models import ModelSpec
from calibration.domain.ports import Clock, HttpTransport, Sleeper
from calibration.domain.provenance_rules import harness_changes, lock_changes, provenance_changes
from calibration.domain.schedule import Pass
from calibration.domain.server_wire import question_specs
from calibration.domain.window import WINDOW_CLOSED_TEXT, RunWindow, is_window_open

NO_SCOPE: Final = ["none"]
HEAVY_PAUSE_TEXT: Final = "another heavy local job is running"


class BlockOutcome(Enum):
    COMPLETE = "complete"
    BLOCKED = "blocked"
    WINDOW_CLOSED = "window-closed"
    HEAVY_WORK = "heavy-work"
    CANARY_FAILED = "canary-failed"


@dataclass(frozen=True)
class BlockDeps:
    store: RunStore
    clock: Clock
    sleeper: Sleeper
    transport: HttpTransport
    servers: ServerFactory
    tools: BuiltTools
    heavy: HeavyWorkProbe
    provenance: ProvenanceProbe
    harness: HarnessIdentity
    window: RunWindow | None
    canary_closed_text: str


class ModelBlock:
    """Runs the next unfinished work of one model; a request whose response file exists is not resent."""

    def __init__(self, deps: BlockDeps) -> None:
        self._deps = deps
        self._store = deps.store
        self._log = DeviationLog(deps.store)
        self._safety = SafetyLog(deps.store)
        self._gate = MachineGate(deps.heavy, self._safety, deps.clock, deps.sleeper, deps.window)

    def run(self, model: ModelSpec, *, canary_only: bool = False) -> BlockOutcome:
        lock = ModelsLock.from_record(json.loads(self._store.read_text(MODELS_LOCK_FILE)))
        locked = lock.entry(model.model_id)
        if self._is_blocked(model, locked):
            return BlockOutcome.BLOCKED
        catalog = ItemCatalog(self._store)
        schedule = catalog.schedule()
        if not canary_only and self._finished(model, locked, schedule.pass1, schedule.pass2):
            return BlockOutcome.COMPLETE
        if not self._window_open():
            return self._closed()
        if not self._gate.wait_until_clear(model.model_id):
            return self._closed()
        return self._serve(model, lock, locked, catalog, canary_only)

    def _is_blocked(self, model: ModelSpec, locked: LockedModel) -> bool:
        return locked.status is PinStatus.BLOCKED or standing_of(self._log.entries(), [model.model_id]).blocked

    def _finished(self, model: ModelSpec, locked: LockedModel, pass1: tuple[str, ...], pass2: tuple[str, ...]) -> bool:
        recorder = RawRecorder(self._store, model.model_id, "", locked.model_digest)
        return recorder.complete(Pass.ONE, pass1) and recorder.complete(Pass.TWO, pass2)

    def _window_open(self) -> bool:
        return is_window_open(self._deps.window, self._deps.clock.now())

    def _closed(self) -> BlockOutcome:
        self._safety.append(EVENT_WINDOW, None, self._deps.clock.now(), {"open": False})
        self._log.record(DeviationKind.PAUSE, NO_SCOPE, WINDOW_CLOSED_TEXT)
        return BlockOutcome.WINDOW_CLOSED

    def _serve(
        self, model: ModelSpec, lock: ModelsLock, locked: LockedModel, catalog: ItemCatalog, canary_only: bool
    ) -> BlockOutcome:
        started = self._deps.clock.now()
        timing = BlockTiming(started, [])
        server = self._deps.servers.server_for(model)
        self._safety.append(EVENT_BLOCK, model.model_id, started, {"phase": "start"})
        try:
            server.start(model, locked.run_reference)
            card = server.card()
            self._record_card(model, started, card, lock, locked)
            outcome = self._measure(model, server, card, catalog, timing, canary_only)
        finally:
            self._shutdown(model, server)
            timing.record(self._store, model.model_id, self._deps.clock.now())
        return outcome

    def _record_card(
        self, model: ModelSpec, started: datetime, card: ServerCard, lock: ModelsLock, locked: LockedModel
    ) -> None:
        provenance = dict(self._deps.provenance.reading().values)
        identity = dict(self._deps.harness.identity())
        for change in lock_changes(model, lock, locked, card.server_version, card.model_digest):
            self._log.record(DeviationKind.PROTOCOL, [model.model_id], change)
        for change in provenance_changes(self._previous_provenance(), provenance):
            self._log.record(DeviationKind.PROVENANCE, NO_SCOPE, change)
        recorded = self._recorded_harness()
        for change in harness_changes(recorded, identity):
            self._log.record(DeviationKind.PROTOCOL, ["all"], change)
        document: dict[str, JsonValue] = {
            "block_start": started.isoformat(),
            "model": model.model_id,
            "card": card.to_record(),
            "provenance": provenance,
            "harness": identity,
        }
        self._store.write_text(server_card_path(model.model_id, started.isoformat()), document_json(document))

    def _recorded_harness(self) -> dict[str, str]:
        manifest = RunManifest(self._store).read()
        return {
            key: value
            for key, value in manifest.items()
            if key in self._deps.harness.identity() and isinstance(value, str)
        }

    def _previous_provenance(self) -> dict[str, str]:
        """The provenance of the most recent block of any model, by block start."""
        cards: list[str] = []
        for model_dir in self._store.list_dir(SERVER_DIR):
            cards.extend(
                f"{SERVER_DIR}/{model_dir}/{name}" for name in self._store.list_dir(f"{SERVER_DIR}/{model_dir}")
            )
        if not cards:
            return {}
        latest = max(cards, key=lambda path: path.rsplit("/", 1)[-1])
        provenance = json_object(json.loads(self._store.read_text(latest)), latest).get("provenance")
        return {key: str(value) for key, value in json_object(provenance, latest).items()}

    def _measure(
        self,
        model: ModelSpec,
        server: ModelServer,
        card: ServerCard,
        catalog: ItemCatalog,
        timing: BlockTiming,
        canary_only: bool,
    ) -> BlockOutcome:
        exchange = Exchange(self._deps.transport, self._deps.clock, self._deps.sleeper, model.flavour)
        if not self._canary_passes(model, server, exchange):
            return BlockOutcome.CANARY_FAILED
        if canary_only:
            return BlockOutcome.COMPLETE
        recorder = RawRecorder(self._store, model.model_id, card.server_version, card.model_digest)
        schedule = catalog.schedule()
        sent = _Sent(0, server.prefix_cache_hits())
        for which in (Pass.ONE, Pass.TWO):
            ids = schedule.ids_of(which)
            outcome = self._send_pass(model, server, exchange, catalog, recorder, which, ids, timing, sent)
            if outcome is not None:
                return outcome
        return BlockOutcome.COMPLETE

    def _canary_passes(self, model: ModelSpec, server: ModelServer, exchange: Exchange) -> bool:
        path = canary_path(model.model_id)
        if self._store.exists(path):
            recorded = json_object(json.loads(self._store.read_text(path)), path)
            if recorded.get("passed") is True:
                return True
            if not standing_of(self._log.entries(), [model.model_id]).protocol:
                return False
        runner = CanaryRunner(
            self._deps.tools.builders, self._deps.tools.reader, exchange, server, self._deps.canary_closed_text
        )
        result = runner.run(model)
        self._store.write_text(path, document_json(result.to_record()))
        return result.passed

    def _send_pass(
        self,
        model: ModelSpec,
        server: ModelServer,
        exchange: Exchange,
        catalog: ItemCatalog,
        recorder: RawRecorder,
        which: Pass,
        request_ids: tuple[str, ...],
        timing: BlockTiming,
        sent: _Sent,
    ) -> BlockOutcome | None:
        for request_id in request_ids:
            if recorder.has_response(which, request_id):
                continue
            if not self._window_open():
                return self._closed()
            if sent.count > 0 and sent.count % HEAVY_CHECK_EVERY == 0 and not self._gate.clear(model.model_id):
                self._log.record(DeviationKind.PAUSE, NO_SCOPE, HEAVY_PAUSE_TEXT)
                return BlockOutcome.HEAVY_WORK
            item = catalog.item(request_id)
            body = item.bodies[model.identifier].encode(ENCODING)
            result = exchange.send(server.endpoint_url, body, question_specs(item.closed))
            hits = server.prefix_cache_hits()
            hit = None if hits is None or sent.hits is None else hits > sent.hits
            sent.hits = hits
            recorder.record(which, request_id, body, result, hit)
            timing.latencies_ms.append(result.final.latency_ms)
            sent.count += 1
        return None

    def _shutdown(self, model: ModelSpec, server: ModelServer) -> None:
        report = server.stop()
        self._safety.append(
            EVENT_SHUTDOWN,
            model.model_id,
            self._deps.clock.now(),
            {
                "unloaded": report.unloaded,
                "models_after_unload": list(report.models_after_unload),
                "server_stopped": report.server_stopped,
                "port_free": report.port_free,
            },
        )
        if not report.clean:
            self._log.record(DeviationKind.PROTOCOL, [model.model_id], "the end-of-block shutdown check failed")


@dataclass
class _Sent:
    """Requests sent in this block and the last Kev prefix-cache hit counter."""

    count: int
    hits: int | None
