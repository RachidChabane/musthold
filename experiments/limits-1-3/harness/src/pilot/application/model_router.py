"""Routing model port: one `ModelPort` per CLI behind a single entry point."""

from __future__ import annotations

import threading
from collections.abc import Mapping

from pilot.application.ports import ModelPort
from pilot.domain.constants import MAX_CONCURRENT_MODEL_CALLS
from pilot.domain.model_call import CanaryBaseline, Cli, ModelOutcome, ModelRequest


class RoutingModelPort:
    """Routes by CLI and bounds the calls in flight across every CLI with one semaphore."""

    def __init__(
        self, ports: Mapping[Cli, ModelPort], max_in_flight: int = MAX_CONCURRENT_MODEL_CALLS
    ) -> None:
        self._ports = ports
        self._in_flight = threading.BoundedSemaphore(max_in_flight)

    def call(self, request: ModelRequest, baseline: CanaryBaseline | None) -> ModelOutcome:
        with self._in_flight:
            return self._ports[request.pin.cli].call(request, baseline)
