"""One CI round trip of a clean-room gate: the requests CI issued, and what answering them cost (ADR 0011, decision 12)."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from specbench.domain.arm_policy import ArmPolicy
from specbench.domain.arms import Arm
from specbench.domain.clean_room import RoomArea
from specbench.domain.gate import GateStatus, TicketUsage
from specbench.domain.product import ProofModule


@dataclass(frozen=True)
class CiRoundRequest:
    """What a host needs to answer round `number`: the pending modules, where CI left their requests and the room.

    `ci_issued` is the `.musthold/ci-issued` directory of the settle step that found the modules pending; each
    pending module's requests are the directory below it that carries its host-ref name.
    """

    tree: Path
    arm: Arm
    policy: ArmPolicy
    modules: tuple[ProofModule, ...]
    pending: tuple[ProofModule, ...]
    session_area: RoomArea
    record_dir: Path
    run_id: str
    ci_issued: Path
    number: int


@dataclass(frozen=True)
class CiRoundResult:
    """How a round ended and what it spent. Only a `COMPLETE` round has archived its answers."""

    status: GateStatus
    session_id: str | None
    executions: int
    tickets: tuple[TicketUsage, ...]
