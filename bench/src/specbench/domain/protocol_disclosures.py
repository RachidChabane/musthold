"""The protocol deviation entries of the clean-room CI simulation, logged at the start of a run (ADR 0011, decision 12)."""

from __future__ import annotations

from typing import Final

from specbench.domain.arms import Arm
from specbench.domain.deviations import DeviationEntry, DeviationKind

CI_SIMULATION: Final = DeviationEntry.create(
    DeviationKind.PROTOCOL,
    (Arm.TR, Arm.AL, Arm.DP, Arm.CO, Arm.ALL, Arm.PL, Arm.ALL_NATIVE, Arm.CHEAT),
    "The clean-room gate's CI run is simulated: SPEC_PROOF_CI_RUN_ID is bench-<tree digest prefix>, and the seed "
    "record of the head is restored with the seed of the agent tree's own commit, so every arm gets the same CI seed "
    "on the same tree; CI-issued artifacts reach host pull through a gh shim no session can reach.",
)

ALL_NATIVE_REGATE: Final = DeviationEntry.create(
    DeviationKind.PROTOCOL,
    (Arm.ALL_NATIVE,),
    "The clean-room re-gate of ALL-native returns its CI-issued requests to ALL-native's own session, as the cheat "
    "arm does (ADR 0007, Host execution), instead of a reserved isolated-fallback live gate; only misses of its "
    "harvested local responses use isolated-fallback; the extra executions are reported in E5.",
)

RUN_START_DISCLOSURES: Final = (CI_SIMULATION, ALL_NATIVE_REGATE)
