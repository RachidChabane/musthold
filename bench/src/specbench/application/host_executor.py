"""The single-shot host port: one product request answered by one isolated subagent.

It serves only the replays of closed questions (the judge replay and the local models). A gate is run through the
`GateHost` port of `specbench.application.ports`, one session per gate, and the trunk and branch engines never import
this module (see the import contracts in `pyproject.toml`).
"""

from __future__ import annotations

from typing import Protocol

from specbench.domain.sessions import HostRequest, HostResponse


class HostExecutor(Protocol):
    """Strategy: executes a product request in an isolated, blind subagent and returns its response."""

    def execute(self, request: HostRequest) -> HostResponse: ...
