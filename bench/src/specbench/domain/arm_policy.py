"""What each arm installs, requires, stops on and where its outcome comes from (ADR 0004 Arms, ADR 0007)."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.arms import Arm, EvaluationKind, Host, Module, host_for

MAX_ATTEMPTS: Final = 3
"""Blocks the stop hook issues before it lets go (Shallnot `MaxAttempts`)."""
MAX_HOST_WAITS: Final = 3
"""Blocks that only wait for the host to answer requests (Shallnot `MaxHostWaits`)."""
PLACEBO_MESSAGE: Final = "Re-read the spec and strengthen your tests"
"""What the placebo arm tells the coder wherever ALL blocks, in place of the product's findings."""

ALL_MODULES: Final = frozenset(Module)
_PROOF_MODULES: Final = (Module.ALIGNMENT, Module.DISCRIMINATING_POWER)
"""The modules whose proof gates under ALL; completeness only shows questions and never gates."""


class StopKind(Enum):
    NONE = "none"
    TESTS = "tests"
    GATE = "gate"


@dataclass(frozen=True)
class StopPolicy:
    """What the stop hook checks and how many times in a row it may send the coder back."""

    kind: StopKind
    max_attempts: int
    max_host_waits: int

    @property
    def max_consecutive_blocks(self) -> int:
        return self.max_attempts + self.max_host_waits


NO_STOP: Final = StopPolicy(StopKind.NONE, 0, 0)
TRUNK_STOP: Final = StopPolicy(StopKind.TESTS, MAX_ATTEMPTS, 0)
"""The trunk stops on the repository's tests and the coder's own tests only, so the product never shapes S_T."""
GATED_STOP: Final = StopPolicy(StopKind.GATE, MAX_ATTEMPTS, MAX_HOST_WAITS)


@dataclass(frozen=True)
class ArmPolicy:
    """The configuration of one arm.

    `modules` are installed in the workspace; `required` is the arm's `proofs.require` in Shallnot's configuration
    (Shallnot itself always checks traceability). `verdict_from` names the arm whose gate verdict this arm reads:
    CO accepts as TR because completeness never gates, and PL blocks wherever ALL blocks.
    """

    arm: Arm
    modules: frozenset[Module]
    required: tuple[Module, ...]
    stop: StopPolicy
    evaluation: EvaluationKind
    verdict_from: Arm
    message_override: str | None = None

    @property
    def host(self) -> Host:
        return host_for(self.arm)

    @property
    def branches(self) -> bool:
        return self.evaluation is EvaluationKind.BRANCH

    @property
    def has_shallnot(self) -> bool:
        return Module.SHALLNOT in self.modules

    @property
    def proofs_require(self) -> tuple[str, ...]:
        """The `proofs.require` list of the arm's `shallnot.yaml`, in the product's own names."""
        return tuple(module.value for module in self.required)


def _policy(
    arm: Arm,
    modules: frozenset[Module],
    required: tuple[Module, ...],
    stop: StopPolicy,
    evaluation: EvaluationKind,
    verdict_from: Arm | None = None,
    message_override: str | None = None,
) -> ArmPolicy:
    return ArmPolicy(arm, modules, required, stop, evaluation, verdict_from or arm, message_override)


def _with_shallnot(*modules: Module) -> frozenset[Module]:
    return frozenset({Module.SHALLNOT, *modules})


ARM_POLICIES: Final = {
    Arm.N: _policy(Arm.N, frozenset(), (), NO_STOP, EvaluationKind.TRUNK_SNAPSHOT),
    Arm.T: _policy(Arm.T, frozenset(), (), TRUNK_STOP, EvaluationKind.TRUNK_SNAPSHOT),
    Arm.TR: _policy(Arm.TR, _with_shallnot(), (), GATED_STOP, EvaluationKind.BRANCH),
    Arm.AL: _policy(Arm.AL, _with_shallnot(Module.ALIGNMENT), (Module.ALIGNMENT,), GATED_STOP, EvaluationKind.BRANCH),
    Arm.DP: _policy(
        Arm.DP,
        _with_shallnot(Module.DISCRIMINATING_POWER),
        (Module.DISCRIMINATING_POWER,),
        GATED_STOP,
        EvaluationKind.BRANCH,
    ),
    Arm.CO: _policy(
        Arm.CO,
        _with_shallnot(Module.COMPLETENESS),
        (),
        GATED_STOP,
        EvaluationKind.SELECTION_ONLY,
        verdict_from=Arm.TR,
    ),
    Arm.ALL: _policy(Arm.ALL, ALL_MODULES, _PROOF_MODULES, GATED_STOP, EvaluationKind.BRANCH),
    Arm.PL: _policy(
        Arm.PL,
        ALL_MODULES,
        _PROOF_MODULES,
        GATED_STOP,
        EvaluationKind.BRANCH,
        verdict_from=Arm.ALL,
        message_override=PLACEBO_MESSAGE,
    ),
    Arm.ALL_NATIVE: _policy(Arm.ALL_NATIVE, ALL_MODULES, _PROOF_MODULES, GATED_STOP, EvaluationKind.IN_SESSION),
    Arm.CHEAT: _policy(Arm.CHEAT, ALL_MODULES, _PROOF_MODULES, GATED_STOP, EvaluationKind.ADVERSARIAL),
}


def policy_for(arm: Arm) -> ArmPolicy:
    return ARM_POLICIES[arm]
