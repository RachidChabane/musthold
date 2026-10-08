"""The product binaries and plugins the benchmark drives, per proof module (ADR 0007, "Host execution")."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final

from specbench.domain.arms import Module

VERIFY_BINARY: Final = "spec-proof-verify"
LEDGER_BINARY: Final = "spec-proof-ledger"
UMBRELLA_PLUGIN_NAME: Final = "spec-proof"
EXECUTOR_SERVER_SUFFIX: Final = "exec"
CLAUDE_PLUGIN_SUBDIRECTORY: Final = "plugin/claude"


@dataclass(frozen=True)
class ProofModule:
    """One proof module: its binary, its Claude plugin, the executor server that plugin starts and its host-ref name."""

    module: Module
    binary: str
    plugin: str
    source_directory: str
    host_ref_name: str

    @property
    def server(self) -> str:
        return f"plugin:{self.plugin}:{EXECUTOR_SERVER_SUFFIX}"

    @property
    def plugin_directory(self) -> PurePosixPath:
        """The plugin tree relative to the repository root."""
        return PurePosixPath(self.source_directory) / CLAUDE_PLUGIN_SUBDIRECTORY


PROOF_MODULES: Final = (
    ProofModule(Module.DISCRIMINATING_POWER, "spec-proof-dp", "spec-proof-dp", "discpower", "discpower"),
    ProofModule(Module.ALIGNMENT, "spec-proof-alignment", "spec-proof-alignment", "alignment", "alignment"),
    ProofModule(
        Module.COMPLETENESS, "spec-proof-completeness", "spec-proof-completeness", "completeness", "completeness"
    ),
)
"""In the order the modules generate and are proved in."""


def proof_modules_in_order(modules: frozenset[Module]) -> tuple[ProofModule, ...]:
    """The proof modules among `modules`, in generate order; Shallnot is not a proof module."""
    return tuple(candidate for candidate in PROOF_MODULES if candidate.module in modules)
