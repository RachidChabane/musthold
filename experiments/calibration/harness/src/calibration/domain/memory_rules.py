"""The memory-fit rules of §6.4: a footprint F against the total physical memory T."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from calibration.domain.constants import (
    BYTES_PER_GIB,
    FOOTPRINT_OVERHEAD_GIB,
    MEMORY_DEFERRAL_LIMIT,
    MEMORY_RESERVE_FRACTION,
    MEMORY_STATIC_FRACTION,
    MEMORY_STOP_LIMIT,
)

PERCENT: int = 100


def footprint_bytes(weight_bytes: int) -> int:
    """F: the bytes of the weight files (the manifest size for Ollama) plus 1 GiB."""
    return weight_bytes + FOOTPRINT_OVERHEAD_GIB * BYTES_PER_GIB


def static_blocked(footprint: int, total_memory: int) -> bool:
    """A model with F > 0.5 T is BLOCKED (`memory-fit`) and is never downloaded or loaded."""
    return footprint > MEMORY_STATIC_FRACTION * total_memory


def required_free_bytes(footprint: int, total_memory: int) -> float:
    """F + 0.15 T: what the free memory must cover before a block loads its model."""
    return footprint + MEMORY_RESERVE_FRACTION * total_memory


def free_bytes(free_percent: int, total_memory: int) -> float:
    return free_percent / PERCENT * total_memory


def dynamic_fits(free_percent: int, total_memory: int, footprint: int) -> bool:
    return free_bytes(free_percent, total_memory) >= required_free_bytes(footprint, total_memory)


class FitDecision(Enum):
    LOAD = "load"
    DEFER = "defer"
    BLOCK_MEMORY_FIT = "block-memory-fit"


@dataclass(frozen=True)
class FitCheck:
    """One dynamic check, as `safety.jsonl` records it."""

    free_percent: int
    total_memory: int
    footprint: int
    required: float
    available: float
    deferrals: int
    decision: FitDecision


def dynamic_decision(free_percent: int, total_memory: int, footprint: int, prior_deferrals: int) -> FitCheck:
    """Load when the memory covers F + 0.15 T; otherwise defer, and block the model once five sessions deferred it."""
    required = required_free_bytes(footprint, total_memory)
    available = free_bytes(free_percent, total_memory)
    if available >= required:
        decision = FitDecision.LOAD
        deferrals = prior_deferrals
    else:
        deferrals = prior_deferrals + 1
        decision = FitDecision.BLOCK_MEMORY_FIT if deferrals >= MEMORY_DEFERRAL_LIMIT else FitDecision.DEFER
    return FitCheck(free_percent, total_memory, footprint, required, available, deferrals, decision)


def blocked_by_pressure(pressure_stops: int) -> bool:
    """A second memory-pressure stop of the same model makes it BLOCKED (`memory-pressure`)."""
    return pressure_stops >= MEMORY_STOP_LIMIT
