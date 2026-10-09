"""`IsolatedCliHost`: the `GateHost` of every honest arm (ADR 0007, "Host execution"; ADR 0011, decisions 1, 10 and 12).

A gate runs in a deterministic clean room, `<rooms root>/<arm>/<tree digest>/`, with two areas:

- `session/` holds a copy of the tree in which the `LIVE` gate-host session runs `prove-all`, recording every consumed
  answer into the record directory. It survives a killed gate, so the next attempt resumes from the pending tickets.
  The CI round trips of the gate run in it too, answering the requests CI issued.
- `settle/` is rebuilt on every call, from the tree and the recorded answers alone (`CleanRoomSettlement`): the modules
  replay them with no execution, CI proves them on the archived host bundles, the statements are signed and Shallnot
  gates the arm's configuration under `trust: signed`. `LIVE` and `REPLAY` share this step, so the verdict depends on
  the recordings and the archived rounds and nothing else.

A gate whose CI proofs still wait for answers pauses with `ci_round_pending`; each further `LIVE` call settles again and,
when proofs still wait, runs one round trip and pauses once more, so that a pacing decorator can reserve each round on
its own and the call after the last round only settles. The reports are stored with the settle tree's path replaced by
a portable root.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field, replace
from pathlib import Path

from specbench.adapters.area_environment import AreaEnvironment
from specbench.adapters.area_preparer import AreaPreparer
from specbench.adapters.claude_credentials import token_or_none, without_oauth_token
from specbench.adapters.claude_gate_session import plugin_directories, session_argv, session_environment, session_prompt
from specbench.adapters.clean_room_settlement import CleanRoomSettlement, Settlement, SettleRequest
from specbench.adapters.gate_session_driver import GateSessionDriver, LiveRun
from specbench.adapters.isolated_host_config import IsolatedHostConfig
from specbench.adapters.session_state import SessionState, SessionStateFile
from specbench.adapters.ticket_usage import ticket_usages
from specbench.application.ports import (
    CiRoundTrip,
    ClaudeCredentialSource,
    ExecutionCounter,
    GateSessionSpec,
    TreeReader,
)
from specbench.domain.arm_policy import ArmPolicy, policy_for
from specbench.domain.arms import Arm, Host
from specbench.domain.ci_round import CiRoundRequest
from specbench.domain.ci_seed import ci_run_id
from specbench.domain.clean_room import CleanRoom, RoomArea
from specbench.domain.gate import GateMode, GateOutcome, GatePause, GateStatus, TicketUsage
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.product import UMBRELLA_PLUGIN_NAME, ProofModule, proof_modules_in_order
from specbench.domain.sealing import tree_digest
from specbench.domain.sessions import live_gate_call_id


class GateHostMisuse(ValueError):
    """The gate was asked of an arm or a mode this host does not serve."""


@dataclass(frozen=True)
class _Spent:
    """What this call spent on sessions: the executions, the session that spent them and the tickets it answered."""

    executions: int = 0
    session_id: str | None = None
    tickets: tuple[TicketUsage, ...] = field(default_factory=tuple[TicketUsage, ...])
    session_ran: bool = False


class IsolatedCliHost:
    """Strategy: a `claude -p` gate-host session in a clean room, or a replay of its recorded answers."""

    def __init__(
        self,
        config: IsolatedHostConfig,
        layout: RunLayout,
        driver: GateSessionDriver,
        preparer: AreaPreparer,
        settlement: CleanRoomSettlement,
        round_trip: CiRoundTrip,
        tree_reader: TreeReader,
        counter: ExecutionCounter,
        environment: AreaEnvironment,
        credentials: ClaudeCredentialSource,
        base_environment: Mapping[str, str],
    ) -> None:
        self._config = config
        self._layout = layout
        self._driver = driver
        self._preparer = preparer
        self._settlement = settlement
        self._round_trip = round_trip
        self._reader = tree_reader
        self._counter = counter
        self._environment = environment
        self._credentials = credentials
        self._base_environment = without_oauth_token(base_environment)

    def run_gate(self, tree: Path, arm: Arm, record_dir: Path, mode: GateMode) -> GateOutcome:
        policy = _served_policy(arm)
        modules = proof_modules_in_order(policy.modules)
        digest = tree_digest(self._reader.read(tree))
        room = CleanRoom.locate(self._config.rooms_root, arm, digest)
        run_id = ci_run_id(digest)
        spent = _Spent()
        if modules and mode is GateMode.LIVE:
            live = self._finish_session(tree, arm, policy, modules, room.session, record_dir, run_id)
            if isinstance(live, GateOutcome):
                return live
            spent = live
        elif modules and not record_dir.is_dir():
            raise GateHostMisuse(f"a replay needs the recorded answers in {record_dir}")
        request = SettleRequest(tree, arm, policy, modules, room.settle, record_dir, run_id)
        settlement = self._settlement.settle(request)
        if settlement.pending and mode is GateMode.LIVE and not spent.session_ran:
            request_round = CiRoundRequest(
                tree,
                arm,
                policy,
                modules,
                settlement.pending,
                room.session,
                record_dir,
                run_id,
                _required(settlement.ci_issued),
                settlement.ci_rounds + 1,
            )
            result = self._round_trip.run(request_round)
            spent = _Spent(result.executions, result.session_id, result.tickets)
            if result.status is not GateStatus.COMPLETE:
                return GateOutcome(None, result.status, result.session_id, result.executions, result.tickets)
            return _outcome(replace(settlement, ci_rounds=request_round.number), spent)
        return _outcome(settlement, spent)

    def _finish_session(
        self,
        tree: Path,
        arm: Arm,
        policy: ArmPolicy,
        modules: Sequence[ProofModule],
        area: RoomArea,
        record_dir: Path,
        run_id: str,
    ) -> _Spent | GateOutcome:
        """Runs the gate-host session unless an earlier call finished it; a session that did not finish is the outcome."""
        state_file = SessionStateFile(area)
        if state_file.read().finished:
            return _Spent()
        live = self._run_session(tree, arm, policy, modules, area, record_dir, run_id)
        executions = self._counter.count(live.lines)
        if live.status is not GateStatus.COMPLETE:
            return GateOutcome(None, live.status, live.session_id, executions, ())
        state_file.write(SessionState(live.session_id, finished=True))
        return _Spent(executions, live.session_id, ticket_usages(live.lines, area), session_ran=True)

    def _run_session(
        self,
        tree: Path,
        arm: Arm,
        policy: ArmPolicy,
        modules: Sequence[ProofModule],
        area: RoomArea,
        record_dir: Path,
        run_id: str,
    ) -> LiveRun:
        token = token_or_none(self._credentials)
        if not token:
            return LiveRun(GateStatus.HARNESS_MISCONFIGURED, None, ())
        self._preparer.prepare(tree, area, policy, modules, run_id)
        record_dir.mkdir(parents=True, exist_ok=True)
        area.config_directory.mkdir(parents=True, exist_ok=True)
        spec = GateSessionSpec(
            session_argv(
                session_prompt(record_dir),
                plugin_directories(self._config.repository, self._config.umbrella_plugin, modules),
                self._config.claude.executable(),
            ),
            session_environment(self._base_environment, self._environment.of(area), area.config_directory, token),
            area.tree,
        )
        events = self._layout.call(live_gate_call_id(arm, record_dir)) / CALL_EVENTS
        plugins = {UMBRELLA_PLUGIN_NAME, *[module.plugin for module in modules]}
        return self._driver.run(
            spec, events, plugins, {module.server for module in modules}, self._config.claude.version
        )


def _outcome(settlement: Settlement, spent: _Spent) -> GateOutcome:
    if settlement.complete:
        return GateOutcome(
            settlement.report,
            GateStatus.COMPLETE,
            spent.session_id,
            spent.executions,
            spent.tickets,
            settlement.local_report,
            settlement.ci_rounds,
        )
    pause = GatePause.CI_ROUND_PENDING if settlement.pending else None
    return GateOutcome(
        None,
        GateStatus.PAUSED,
        spent.session_id,
        spent.executions,
        spent.tickets,
        ci_rounds=settlement.ci_rounds,
        pause=pause,
    )


def _required(path: Path | None) -> Path:
    if path is None:
        raise GateHostMisuse("a settlement with pending modules names where CI left their requests")
    return path


def _served_policy(arm: Arm) -> ArmPolicy:
    policy = policy_for(arm)
    if policy.host is not Host.ISOLATED_CLI or not policy.has_shallnot:
        raise GateHostMisuse(f"{arm.value} is not gated by a clean-room gate-host session")
    return policy
