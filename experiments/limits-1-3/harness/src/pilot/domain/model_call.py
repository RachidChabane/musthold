"""Value objects of a model call (PREREG 4)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Any


class Cli(Enum):
    CLAUDE = "claude"
    CODEX = "codex"


class Effort(Enum):
    MEDIUM = "medium"
    HIGH = "high"


class ModelProvenance(Enum):
    REPORTED = "reported"
    REQUESTED_ONLY = "requested_only"


@dataclass(frozen=True)
class ModelPin:
    model: str
    cli: Cli

    @property
    def key(self) -> str:
        return f"{self.model}__{self.cli.value}"

    @property
    def provenance(self) -> ModelProvenance:
        if self.cli is Cli.CLAUDE:
            return ModelProvenance.REPORTED
        return ModelProvenance.REQUESTED_ONLY


@dataclass(frozen=True)
class ModelRequest:
    call_id: str
    pin: ModelPin
    effort: Effort
    item: str
    schema_text: str
    system_prompt: str


@dataclass(frozen=True)
class TokenUsage:
    input_tokens: int
    output_tokens: int


@dataclass(frozen=True)
class CanaryBaseline:
    input_tokens: int
    num_turns: int | None


@dataclass(frozen=True)
class ModelResult:
    payload: Any
    usage: TokenUsage
    cost_usd: float | None
    num_turns: int | None
    attempt: int
    provenance: ModelProvenance

    def to_mapping(self) -> dict[str, Any]:
        return {
            "status": "ok",
            "payload": self.payload,
            "input_tokens": self.usage.input_tokens,
            "output_tokens": self.usage.output_tokens,
            "total_cost_usd": self.cost_usd,
            "num_turns": self.num_turns,
            "attempt": self.attempt,
            "provenance": self.provenance.value,
        }

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> ModelResult:
        return cls(
            payload=raw["payload"],
            usage=TokenUsage(raw["input_tokens"], raw["output_tokens"]),
            cost_usd=raw["total_cost_usd"],
            num_turns=raw["num_turns"],
            attempt=raw["attempt"],
            provenance=ModelProvenance(raw["provenance"]),
        )


@dataclass(frozen=True)
class MissingResult:
    failures: tuple[str, ...]
    isolation_violations: tuple[str, ...]

    def to_mapping(self) -> dict[str, Any]:
        return {
            "status": "missing",
            "failures": list(self.failures),
            "isolation_violations": list(self.isolation_violations),
        }

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> MissingResult:
        return cls(tuple(raw["failures"]), tuple(raw["isolation_violations"]))


ModelOutcome = ModelResult | MissingResult


def outcome_from_mapping(raw: Mapping[str, Any]) -> ModelOutcome:
    if raw["status"] == "ok":
        return ModelResult.from_mapping(raw)
    return MissingResult.from_mapping(raw)
