"""Harnesses, their frozen models, arms and the host each arm uses (ADR 0007)."""

from __future__ import annotations

from enum import Enum
from typing import Final


class Harness(Enum):
    CLAUDE_CODE = "claude-code"
    CODEX = "codex"


FROZEN_MODEL: Final = {Harness.CLAUDE_CODE: "claude-sonnet-5-5", Harness.CODEX: "gpt-6-luna"}
"""One frozen model per harness, identical in every arm of that harness."""

JUDGE_REPLAY_MODEL: Final = {Harness.CLAUDE_CODE: "claude-opus-5-5", Harness.CODEX: "claude-sonnet-5-5"}


class Arm(Enum):
    N = "N"
    T = "T"
    TR = "TR"
    AL = "AL"
    DP = "DP"
    CO = "CO"
    ALL = "ALL"
    PL = "PL"
    ALL_NATIVE = "ALL-native"
    CHEAT = "cheat"


class Host(Enum):
    """Who executes the product's requests: a separate gate-host session, or the coder's own session."""

    ISOLATED_CLI = "IsolatedCliHost"
    PLUGIN = "PluginHost"


S_T_ARMS: Final = (Arm.N, Arm.T, Arm.TR, Arm.AL, Arm.DP, Arm.CO, Arm.ALL, Arm.PL)
"""Arms evaluated on the same S_T snapshot of the trunk."""

PLUGIN_HOST_ARMS: Final = frozenset({Arm.ALL_NATIVE, Arm.CHEAT})
ACCEPTANCE_ARMS: Final = (Arm.T, Arm.TR, Arm.AL, Arm.DP, Arm.CO, Arm.ALL, Arm.PL, Arm.ALL_NATIVE)
"""Arms whose acceptance and defect rates enter X; N has no S_T and the cheat arm reports success rates instead."""


class Module(Enum):
    SHALLNOT = "shallnot"
    ALIGNMENT = "alignment"
    DISCRIMINATING_POWER = "discriminating-power"
    COMPLETENESS = "completeness"


class EvaluationKind(Enum):
    """How an arm's outcome is obtained: from the trunk's own snapshots, a forked branch, a session of its own, or a gapped spec."""

    TRUNK_SNAPSHOT = "trunk-snapshot"
    BRANCH = "branch"
    SELECTION_ONLY = "selection-only"
    IN_SESSION = "in-session"
    ADVERSARIAL = "adversarial"
    SPEC_GAP = "spec-gap"


def host_for(arm: Arm) -> Host:
    """Honest arms are gated by a gate-host session; only the in-session deployment and the adversary use the coder's."""
    return Host.PLUGIN if arm in PLUGIN_HOST_ARMS else Host.ISOLATED_CLI
