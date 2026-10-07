"""`models.lock.json` (§5.3): what `pin` records and what a block reads before it starts a server."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import RecordError, enum_of, integer, mapping, optional_text, text

MEMORY_FIT_REASON: Final = "memory-fit"
MEMORY_PRESSURE_REASON: Final = "memory-pressure"


class PinStatus(Enum):
    PINNED = "pinned"
    BLOCKED = "blocked"


@dataclass(frozen=True)
class LockedModel:
    """One model of the lock.

    `model_digest` is the Ollama manifest digest or the Kev adapter revision. `run_reference` is the Kev
    `--run` argument, `<repository>@<revision>`. `footprint_bytes` is F of §6.4.
    """

    model_id: str
    status: PinStatus
    block_reason: str | None
    footprint_bytes: int
    model_digest: str
    run_reference: str | None = None

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "status": self.status.value,
            "block_reason": self.block_reason,
            "footprint_bytes": self.footprint_bytes,
            "model_digest": self.model_digest,
            "run_reference": self.run_reference,
        }

    @classmethod
    def from_record(cls, model_id: str, raw: object) -> LockedModel:
        body = mapping(raw, f"lock.{model_id}")
        return cls(
            model_id=model_id,
            status=enum_of(PinStatus, body.get("status"), f"lock.{model_id}.status"),
            block_reason=optional_text(body.get("block_reason"), f"lock.{model_id}.block_reason"),
            footprint_bytes=integer(body.get("footprint_bytes"), f"lock.{model_id}.footprint_bytes"),
            model_digest=text(body.get("model_digest"), f"lock.{model_id}.model_digest"),
            run_reference=optional_text(body.get("run_reference"), f"lock.{model_id}.run_reference"),
        )


@dataclass(frozen=True)
class ModelsLock:
    """The typed view of the lock; the full record, with every size and detail, stays in `record`."""

    total_memory_bytes: int
    ollama_version: str
    kev_commit: str
    models: Mapping[str, LockedModel]
    record: Mapping[str, object]

    @classmethod
    def from_record(cls, raw: object) -> ModelsLock:
        body = mapping(raw, "models.lock")
        entries: dict[str, LockedModel] = {}
        for section in ("ollama", "kev"):
            models = mapping(mapping(body.get(section), f"lock.{section}").get("models"), f"lock.{section}.models")
            for model_id, entry in models.items():
                entries[model_id] = LockedModel.from_record(model_id, entry)
        return cls(
            total_memory_bytes=integer(body.get("total_memory_bytes"), "lock.total_memory_bytes"),
            ollama_version=text(mapping(body.get("ollama"), "lock.ollama").get("version"), "lock.ollama.version"),
            kev_commit=text(mapping(body.get("kev"), "lock.kev").get("commit"), "lock.kev.commit"),
            models=entries,
            record=body,
        )

    def entry(self, model_id: str) -> LockedModel:
        try:
            return self.models[model_id]
        except KeyError as error:
            raise RecordError(f"the lock holds no entry for {model_id}") from error
