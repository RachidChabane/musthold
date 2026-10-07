"""A machine of 24 GiB with the six models of §5.1 as the registry and the Hub describe them, ready for `pin`."""

from __future__ import annotations

import json

from calibration.application.deviation_log import DeviationLog
from calibration.application.pin import PinUseCase
from calibration.application.pin_ports import (
    HeadMeta,
    HubFile,
    HubRepository,
    KevEnvironmentInfo,
    OllamaManifest,
    OllamaModelRecord,
)
from calibration.domain.canonical import sha256_hex
from calibration.domain.constants import BYTES_PER_GIB, KEV_COMMIT, MODELS_LOCK_FILE, OLLAMA_VERSION
from calibration.domain.lock import ModelsLock
from calibration.domain.models import MODEL_BY_ID
from support.build_fakes import (
    FakeKevEnvironment,
    FakeMemoryProbe,
    FakeModelHub,
    FakeOllamaProvisioner,
    FakeOllamaRegistry,
    InMemoryRunStore,
)

GIB = BYTES_PER_GIB
TOTAL_MEMORY = 24 * GIB
MIB = 1024**2
OLLAMA_MANIFEST_BYTES = {"M1": 6 * GIB, "M2": 3 * GIB, "M3": 1 * GIB}
KEV_BASE_REPOSITORY = {"M4": "Qwen/Qwen3.5-0.8B", "M5": "Qwen/Qwen3.5-4B", "M6": "Qwen/Qwen3.5-9B"}
KEV_BASE_WEIGHT_BYTES = {"M4": 2 * GIB, "M5": 8 * GIB, "M6": 18 * GIB}
KEV_BASE_SHARDS = 2
ADAPTER_BYTES = 200 * MIB
HEAD_BYTES = 4 * MIB
BASE_REVISION_NAME = "v1.0"
ADAPTER_WEIGHT_FILE = "adapter_model.safetensors"
HEAD_FILE = "head.pt"
NON_WEIGHT_FILE = "README.md"
KEV_VERSIONS = {"mlx": "0.29.1", "mlx-lm": "0.28.0", "torch": "2.8.0"}
OLLAMA_DETAILS = {"family": "nimble", "parameter_size": "9B", "quantization_level": "Q4_K_M"}
OLLAMA_PARAMETERS = {"num_ctx": 4096, "temperature": 1.0}


def ollama_digest(model_id: str) -> str:
    return f"sha256:{sha256_hex(MODEL_BY_ID[model_id].identifier)}"


def adapter_revision(model_id: str) -> str:
    return sha256_hex(f"adapter-{model_id}")[:40]


def base_revision_sha(model_id: str) -> str:
    return sha256_hex(f"base-{model_id}")[:40]


def kev_repository(model_id: str) -> str:
    repository = MODEL_BY_ID[model_id].repository
    assert repository is not None
    return repository


def _ollama_manifests() -> dict[str, OllamaManifest]:
    manifests: dict[str, OllamaManifest] = {}
    for model_id, size in OLLAMA_MANIFEST_BYTES.items():
        model = MODEL_BY_ID[model_id]
        manifests[model.identifier] = OllamaManifest(model.library, model.tag, ollama_digest(model_id), size)
    return manifests


def _ollama_records() -> dict[str, OllamaModelRecord]:
    return {
        MODEL_BY_ID[model_id].identifier: OllamaModelRecord(
            MODEL_BY_ID[model_id].identifier, ollama_digest(model_id), OLLAMA_DETAILS, OLLAMA_PARAMETERS
        )
        for model_id in OLLAMA_MANIFEST_BYTES
    }


def _base_files(model_id: str) -> tuple[HubFile, ...]:
    shard = KEV_BASE_WEIGHT_BYTES[model_id] // KEV_BASE_SHARDS
    shards = tuple(HubFile(f"model-{index}.safetensors", shard) for index in range(KEV_BASE_SHARDS))
    return (*shards, HubFile("config.json", 1))


def _hub_repositories() -> dict[tuple[str, str | None], HubRepository]:
    repositories: dict[tuple[str, str | None], HubRepository] = {}
    for model_id, base in KEV_BASE_REPOSITORY.items():
        adapter = kev_repository(model_id)
        repositories[(adapter, None)] = HubRepository(
            adapter,
            adapter_revision(model_id),
            (HubFile(ADAPTER_WEIGHT_FILE, ADAPTER_BYTES), HubFile(HEAD_FILE, HEAD_BYTES), HubFile(NON_WEIGHT_FILE, 1)),
        )
        repositories[(base, BASE_REVISION_NAME)] = HubRepository(
            base, base_revision_sha(model_id), _base_files(model_id)
        )
    return repositories


def _heads() -> dict[str, HeadMeta]:
    return {
        kev_repository(model_id): HeadMeta(base, BASE_REVISION_NAME) for model_id, base in KEV_BASE_REPOSITORY.items()
    }


class PinWorld:
    """Fakes of every port of `pin` over one shared event log; tests change what the fakes serve before `run`."""

    def __init__(self, total_memory: int = TOTAL_MEMORY) -> None:
        self.events: list[str] = []
        self.store = InMemoryRunStore()
        self.memory = FakeMemoryProbe(total_memory)
        self.registry = FakeOllamaRegistry(_ollama_manifests(), self.events)
        self.hub = FakeModelHub(_hub_repositories(), self.events)
        self.kev = FakeKevEnvironment(
            KevEnvironmentInfo(KEV_COMMIT, sha256_hex("uv.lock"), KEV_VERSIONS), _heads(), self.events
        )
        self.provisioner = FakeOllamaProvisioner(OLLAMA_VERSION, _ollama_records(), self.events)

    def use_case(self) -> PinUseCase:
        return PinUseCase(
            self.store, DeviationLog(self.store), self.memory, self.registry, self.hub, self.kev, self.provisioner
        )

    def lock_text(self) -> str:
        return self.store.read_text(MODELS_LOCK_FILE)

    def lock(self) -> ModelsLock:
        return ModelsLock.from_record(json.loads(self.lock_text()))
