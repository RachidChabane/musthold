"""The six local models of source (1), their servers and the order of their blocks."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Final

from calibration.domain.constants import KEV_HUB_OWNER


class Flavour(Enum):
    """The two server flavours of the `/v1/systemone` route."""

    OLLAMA = "ollama"
    KEV = "kev"


@dataclass(frozen=True)
class ModelSpec:
    """One model of §5.1: its id, the identifier sent in a request body and its server."""

    model_id: str
    description: str
    identifier: str
    flavour: Flavour
    repository: str | None = None

    @property
    def is_ollama(self) -> bool:
        return self.flavour is Flavour.OLLAMA

    @property
    def library(self) -> str:
        """The Ollama library name of the identifier (`nimble` of `nimble:9b`)."""
        return self.identifier.split(":", 1)[0]

    @property
    def tag(self) -> str:
        return self.identifier.split(":", 1)[1]


MODELS: Final = (
    ModelSpec("M1", "nimble 9B", "nimble:9b", Flavour.OLLAMA),
    ModelSpec("M2", "tev1 4B", "tev1:4b", Flavour.OLLAMA),
    ModelSpec("M3", "tev1 0.8B", "tev1:0.8b", Flavour.OLLAMA),
    ModelSpec("M4", "Kev 0.8B", "kev-0.8b", Flavour.KEV, f"{KEV_HUB_OWNER}/kev-0.8b"),
    ModelSpec("M5", "Kev 4B", "kev-4b", Flavour.KEV, f"{KEV_HUB_OWNER}/kev-4b"),
    ModelSpec("M6", "Kev 9B", "kev-9b", Flavour.KEV, f"{KEV_HUB_OWNER}/kev-9b"),
)
MODEL_BY_ID: Final = {model.model_id: model for model in MODELS}
BLOCK_ORDER: Final = ("M3", "M2", "M1", "M4", "M5", "M6")
MODEL_IDS: Final = tuple(model.model_id for model in MODELS)


class UnknownModel(KeyError):
    """A model id that is none of M1 to M6."""


def model_by_id(model_id: str) -> ModelSpec:
    try:
        return MODEL_BY_ID[model_id]
    except KeyError as error:
        raise UnknownModel(model_id) from error


def models_in_block_order() -> tuple[ModelSpec, ...]:
    return tuple(MODEL_BY_ID[model_id] for model_id in BLOCK_ORDER)


def identifiers() -> tuple[str, ...]:
    """The identifiers of §5.1, in model order: what a request body of an item carries."""
    return tuple(model.identifier for model in MODELS)
