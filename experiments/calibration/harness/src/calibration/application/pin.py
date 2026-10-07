"""`pin` (§5.3, §6.4): every size is read from registry or Hub metadata, the static memory rule runs before any
download, and only the models it admits are downloaded. The lock is written once per server family and an existing
entry is never rewritten."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from calibration.application.deviation_log import DeviationLog
from calibration.application.pin_ports import (
    HubRepository,
    KevEnvironment,
    ModelHub,
    OllamaProvisioner,
    OllamaRegistry,
)
from calibration.application.ports import MemoryProbe, RunStore
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.constants import KEV_COMMIT, MODELS_LOCK_FILE, OLLAMA_VERSION
from calibration.domain.deviations import DeviationKind
from calibration.domain.jsonread import json_object
from calibration.domain.lock import MEMORY_FIT_REASON, PinStatus
from calibration.domain.memory_rules import footprint_bytes, static_blocked
from calibration.domain.models import MODELS, ModelSpec

DIGEST_PREFIX: Final = "sha256:"


class PinError(Exception):
    """A pin that cannot be recorded: the registry and the server disagree, or a version is not the pinned one."""


@dataclass(frozen=True)
class PinReport:
    pinned: tuple[str, ...]
    blocked: tuple[str, ...]
    already_pinned: tuple[str, ...]


def _bare_digest(digest: str) -> str:
    return digest.removeprefix(DIGEST_PREFIX).lower()


class PinUseCase:
    def __init__(
        self,
        store: RunStore,
        log: DeviationLog,
        memory: MemoryProbe,
        registry: OllamaRegistry,
        hub: ModelHub,
        kev: KevEnvironment,
        provisioner: OllamaProvisioner,
    ) -> None:
        self._store = store
        self._log = log
        self._memory = memory
        self._registry = registry
        self._hub = hub
        self._kev = kev
        self._provisioner = provisioner

    def run(self) -> PinReport:
        lock = self._read_lock()
        lock["total_memory_bytes"] = self._memory.total_memory_bytes()
        done = self._pinned_ids(lock)
        pinned: list[str] = []
        blocked: list[str] = []
        for family, models, pin_family in (
            ("ollama", [m for m in MODELS if m.is_ollama and m.model_id not in done], self._pin_ollama),
            ("kev", [m for m in MODELS if not m.is_ollama and m.model_id not in done], self._pin_kev),
        ):
            if not models:
                continue
            section = json_object(lock.get(family, {}), family)
            updated, family_blocked = pin_family(models, dict(section), self._total(lock))
            lock[family] = updated
            blocked.extend(family_blocked)
            pinned.extend(m.model_id for m in models if m.model_id not in family_blocked)
            self._store.write_text(MODELS_LOCK_FILE, document_json(lock))
        return PinReport(tuple(pinned), tuple(blocked), tuple(sorted(done)))

    def _read_lock(self) -> dict[str, JsonValue]:
        if not self._store.exists(MODELS_LOCK_FILE):
            return {}
        return dict(json_object(json.loads(self._store.read_text(MODELS_LOCK_FILE)), MODELS_LOCK_FILE))

    @staticmethod
    def _total(lock: Mapping[str, JsonValue]) -> int:
        total = lock["total_memory_bytes"]
        if isinstance(total, bool) or not isinstance(total, int):
            raise PinError("the total memory is not an integer")
        return total

    @staticmethod
    def _pinned_ids(lock: Mapping[str, JsonValue]) -> set[str]:
        found: set[str] = set()
        for family in ("ollama", "kev"):
            section = lock.get(family)
            if isinstance(section, Mapping):
                models = section.get("models")
                if isinstance(models, Mapping):
                    found.update(models)
        return found

    def _block(self, model: ModelSpec, footprint: int, total: int, reason: str) -> dict[str, JsonValue]:
        self._log.record(DeviationKind.BLOCKED, [model.model_id], f"{reason}: footprint {footprint} bytes of {total}")
        return {
            "status": PinStatus.BLOCKED.value,
            "block_reason": reason,
            "footprint_bytes": footprint,
            "model_digest": "",
            "run_reference": None,
        }

    def _pin_ollama(
        self, models: list[ModelSpec], section: dict[str, JsonValue], total: int
    ) -> tuple[dict[str, JsonValue], list[str]]:
        entries: dict[str, JsonValue] = dict(json_object(section.get("models", {}), "ollama.models"))
        manifests = {m.model_id: self._registry.manifest(m.library, m.tag) for m in models}
        footprints = {model_id: footprint_bytes(manifest.size_bytes) for model_id, manifest in manifests.items()}
        admitted = [m for m in models if not static_blocked(footprints[m.model_id], total)]
        blocked = [m.model_id for m in models if m not in admitted]
        for model in models:
            if model.model_id in blocked:
                entries[model.model_id] = self._block(model, footprints[model.model_id], total, MEMORY_FIT_REASON)
        provision = self._provisioner.provision([m.identifier for m in admitted])
        if provision.version != OLLAMA_VERSION:
            raise PinError(f"the Ollama server is {provision.version}, the pin is {OLLAMA_VERSION}")
        for model in admitted:
            manifest = manifests[model.model_id]
            record = provision.models[model.identifier]
            if _bare_digest(record.digest) != _bare_digest(manifest.digest):
                raise PinError(f"{model.identifier}: the server holds {record.digest}, the registry {manifest.digest}")
            entries[model.model_id] = {
                "status": PinStatus.PINNED.value,
                "block_reason": None,
                "footprint_bytes": footprints[model.model_id],
                "model_digest": manifest.digest,
                "run_reference": None,
                "identifier": model.identifier,
                "manifest_size_bytes": manifest.size_bytes,
                "details": dict(record.details),
                "parameters": dict(record.parameters),
            }
        return {"version": provision.version, "models": entries}, blocked

    def _pin_kev(
        self, models: list[ModelSpec], section: dict[str, JsonValue], total: int
    ) -> tuple[dict[str, JsonValue], list[str]]:
        info = self._kev.setup()
        if info.commit != KEV_COMMIT:
            raise PinError(f"the Kev checkout is {info.commit}, the pin is {KEV_COMMIT}")
        entries: dict[str, JsonValue] = dict(json_object(section.get("models", {}), "kev.models"))
        blocked: list[str] = []
        for model in models:
            entry, is_blocked = self._pin_kev_model(model, total)
            entries[model.model_id] = entry
            if is_blocked:
                blocked.append(model.model_id)
        return {
            "commit": info.commit,
            "uv_lock_sha256": info.uv_lock_sha256,
            "versions": dict(info.versions),
            "models": entries,
        }, blocked

    def _pin_kev_model(self, model: ModelSpec, total: int) -> tuple[dict[str, JsonValue], bool]:
        repository = self._repository_of(model)
        adapter = self._hub.repository(repository)
        head = self._kev.head_meta(repository, adapter.revision)
        base = self._hub.repository(head.base, head.base_revision)
        weights = [*adapter.weight_files, *base.weight_files]
        footprint = footprint_bytes(sum(f.size_bytes for f in weights))
        if static_blocked(footprint, total):
            return self._block(model, footprint, total, MEMORY_FIT_REASON), True
        self._kev.download(repository, adapter.revision)
        self._kev.download(head.base, head.base_revision)
        return {
            "status": PinStatus.PINNED.value,
            "block_reason": None,
            "footprint_bytes": footprint,
            "model_digest": adapter.revision,
            "run_reference": f"{repository}@{adapter.revision}",
            "repository": repository,
            "revision": adapter.revision,
            "base": head.base,
            "base_revision": base.revision,
            "base_revision_named": head.base_revision,
            "files": _file_records(adapter, base),
        }, False

    @staticmethod
    def _repository_of(model: ModelSpec) -> str:
        if model.repository is None:
            raise PinError(f"{model.model_id} names no Hub repository")
        return model.repository


def _file_records(adapter: HubRepository, base: HubRepository) -> list[JsonValue]:
    return [
        {"repository": repo.repository, "revision": repo.revision, "path": file.path, "size_bytes": file.size_bytes}
        for repo in (adapter, base)
        for file in repo.weight_files
    ]
