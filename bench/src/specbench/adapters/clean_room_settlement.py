"""The settle step of a clean-room gate: the verdicts of the recorded answers, the signed one and its local readout.

It is a pure function of the tree and the record directory, so `LIVE` and `REPLAY` share it and a replay reproduces the
live report byte for byte at zero executions:

1. The area is rebuilt from the tree: the agent's tree committed alone, the arm's setup on top, the CI seed of the tree
   restored (`domain.ci_seed`).
2. Each module replays the recorded answers once. The statements this produces are kept aside for the local readout.
3. The answers are pushed as round 0 of the host bundles. CI then proves each module on the bundles, which are the
   archived round trips of the gate when it has any. A module that exits 3 waits for CI-issued requests to be answered.
4. A module still waiting with a round archived beyond the bundle moves on to that round; one waiting with none returns
   the settlement for the host to run a round trip. After `CI_ROUND_TRIPS` rounds what still waits stays pending and
   Shallnot reports it as `awaiting_host_judgment`, an escalation.
5. The statements are signed, verified offline and gated under `trust: signed`; the kept local statements are gated
   under `trust: local` for the secondary readout.
"""

from __future__ import annotations

import shutil
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.area_environment import AreaEnvironment
from specbench.adapters.area_preparer import AreaPreparer
from specbench.adapters.ci_round_archive import CiRoundArchive, read_manifest
from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.host_bundle_merge import merge_bundles
from specbench.adapters.product_cli import (
    EXIT_AWAITING_HOST,
    LOCAL_CACHE_DIRECTORY,
    VERIFIED_DIRECTORY,
    ProductCli,
    ProveRequest,
)
from specbench.adapters.proof_publication import (
    CI_ISSUED_DIRECTORY,
    LocalPublication,
    PublicationContext,
    SignedPublication,
)
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.domain.arm_policy import ArmPolicy
from specbench.domain.arms import Arm
from specbench.domain.canonical import JsonValue
from specbench.domain.ci_seed import CI_ROUND_TRIPS, CiIdentity
from specbench.domain.clean_room import RoomArea, portable
from specbench.domain.product import ProofModule
from specbench.domain.shallnot_config import ProofTrust, arm_shallnot_config

SHALLNOT_CONFIG: Final = "shallnot.yaml"
SIGNED_REPORT_FILE: Final = "report.json"
LOCAL_REPORT_FILE: Final = "report.local.json"
LOCAL_PROOFS_DIRECTORY: Final = "local-proofs"
ROUND_ZERO_DIRECTORY: Final = "host-round-0"
MERGED_DIRECTORY: Final = "host-merged"


@dataclass(frozen=True)
class SettleRequest:
    tree: Path
    arm: Arm
    policy: ArmPolicy
    modules: tuple[ProofModule, ...]
    area: RoomArea
    record_dir: Path
    run_id: str


@dataclass(frozen=True)
class Settlement:
    """What settling found: the verdicts, or what is missing for them.

    A complete settlement carries the signed `report` and, for an arm with proof modules, the `local_report`.
    `unanswered` means the recordings leave a ticket of the gate without an answer. `pending` names the modules whose
    CI-issued requests need another round trip, whose requests lie below `ci_issued`.
    """

    report: JsonValue | None = None
    local_report: JsonValue | None = None
    ci_rounds: int = 0
    unanswered: bool = False
    pending: tuple[ProofModule, ...] = ()
    ci_issued: Path | None = None

    @property
    def complete(self) -> bool:
        return self.report is not None


class CleanRoomSettlement:
    def __init__(
        self,
        cli: ProductCli,
        shallnot: ShallnotCli,
        preparer: AreaPreparer,
        repository: CleanRoomRepository,
        environment: AreaEnvironment,
        local: LocalPublication,
        signed: SignedPublication,
    ) -> None:
        self._cli = cli
        self._shallnot = shallnot
        self._preparer = preparer
        self._repository = repository
        self._environment = environment
        self._local = local
        self._signed = signed

    def settle(self, request: SettleRequest) -> Settlement:
        area = request.area
        shutil.rmtree(area.root, ignore_errors=True)
        identity = self._preparer.prepare(request.tree, area, request.policy, request.modules, request.run_id)
        env = self._environment.of(area)
        for module in request.modules:
            replay = ProveRequest(replay=request.record_dir)
            if self._cli.prove(module, area.tree, env, replay) == EXIT_AWAITING_HOST:
                return Settlement(unanswered=True)
        if not request.modules:
            return Settlement(report=self._gate(request, env, ProofTrust.SIGNED))
        context = PublicationContext(area, env, request.modules, identity)
        round_zero = self._push_round_zero(context)
        self._keep_local_proofs(context)
        rounds, pending = self._prove_in_ci(request, context, round_zero)
        if pending:
            return Settlement(ci_rounds=rounds, pending=pending, ci_issued=area.tree / CI_ISSUED_DIRECTORY)
        self._signed.sign_and_verify(context)
        report = self._gate(request, env, ProofTrust.SIGNED)
        self._restore_local_proofs(context)
        return Settlement(report, self._gate(request, env, ProofTrust.LOCAL), rounds)

    def _push_round_zero(self, context: PublicationContext) -> dict[str, Path]:
        """Each module's recorded answers as the directory a CI job would build from the ref the developer pushed."""
        tree = context.area.tree
        bundles: dict[str, Path] = {}
        for module in context.modules:
            bundle = context.area.root / ROUND_ZERO_DIRECTORY / module.host_ref_name
            bundle.mkdir(parents=True, exist_ok=True)
            bundles[module.host_ref_name] = bundle
            if self._cli.host_push(module, tree, context.env):
                self._repository.export_host_ref(tree, module.host_ref_name, bundle)
        return bundles

    def _prove_in_ci(
        self, request: SettleRequest, context: PublicationContext, round_zero: Mapping[str, Path]
    ) -> tuple[int, tuple[ProofModule, ...]]:
        """The rounds the verdict rests on and the modules that wait for another round trip, if any."""
        archive = CiRoundArchive(request.record_dir)
        archived = min(archive.rounds(), CI_ROUND_TRIPS)
        rounds = 0
        while True:
            bundles = {
                module.host_ref_name: self._bundle(
                    archive, module, rounds, round_zero, context.identity, context.area.root
                )
                for module in context.modules
            }
            waiting = self._signed.prove_in_ci(context, bundles)
            if not waiting or rounds >= CI_ROUND_TRIPS:
                return rounds, ()
            if rounds >= archived:
                return rounds, waiting
            rounds += 1

    def _bundle(
        self,
        archive: CiRoundArchive,
        module: ProofModule,
        rounds: int,
        round_zero: Mapping[str, Path],
        identity: CiIdentity,
        area_root: Path,
    ) -> Path:
        """The bundle CI imports for a module: the answers of round 0 and those of the latest archived round, which holds
        every answer its session gave in the rounds before it."""
        recorded = round_zero[module.host_ref_name]
        latest = archive.latest_bundle(module.host_ref_name, rounds)
        if latest is None:
            return recorded
        read_manifest(latest).require_head(identity.head)
        return merge_bundles(recorded, latest, area_root / MERGED_DIRECTORY / module.host_ref_name)

    def _keep_local_proofs(self, context: PublicationContext) -> None:
        """Publishes the replayed statements as local ones and keeps them for the readout CI's proofs would overwrite."""
        self._local.publish(context)
        kept = context.area.root / LOCAL_PROOFS_DIRECTORY
        for directory in (LOCAL_CACHE_DIRECTORY, VERIFIED_DIRECTORY):
            source = context.area.tree / directory
            shutil.copytree(source, kept / Path(directory).name)

    def _restore_local_proofs(self, context: PublicationContext) -> None:
        tree = context.area.tree
        kept = context.area.root / LOCAL_PROOFS_DIRECTORY
        for directory in (LOCAL_CACHE_DIRECTORY, VERIFIED_DIRECTORY):
            shutil.rmtree(tree / directory, ignore_errors=True)
            shutil.copytree(kept / Path(directory).name, tree / directory)

    def _gate(self, request: SettleRequest, env: Mapping[str, str], trust: ProofTrust) -> JsonValue:
        """The portable report of Shallnot gating the arm's configuration under `trust`."""
        area = request.area
        suffix = "" if trust is ProofTrust.SIGNED else f".{trust.value}"
        config = area.tree / f"shallnot.{request.arm.value.lower()}{suffix}.yaml"
        base = (area.tree / SHALLNOT_CONFIG).read_text(encoding="utf-8")
        config.write_text(arm_shallnot_config(base, request.policy.proofs_require, trust), encoding="utf-8")
        report_file = SIGNED_REPORT_FILE if trust is ProofTrust.SIGNED else LOCAL_REPORT_FILE
        gate = self._shallnot.gate(area.tree, config, area.root / report_file, env)
        return portable(gate.report, {str(area.tree), str(area.tree.resolve())})
