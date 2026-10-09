"""`IsolatedCliHost`: the `GateHost` of every honest arm (ADR 0007, "Host execution"; ADR 0011, decisions 1 and 10).

A gate runs in a deterministic clean room, `<rooms root>/<arm>/<tree digest>/`, with two areas:

- `session/` holds a copy of the tree in which the `LIVE` gate-host session runs `prove-all`, recording every consumed
  answer into the record directory. It survives a killed gate, so the next attempt resumes from the pending tickets.
- `settle/` is rebuilt on every call, from the tree and the recorded answers alone: the modules replay them with no
  execution, the proofs are published, and Shallnot gates the arm's configuration. `LIVE` and `REPLAY` share this step,
  so the report depends on the recordings and nothing else.

The report is stored with the settle tree's path replaced by a portable root.
"""

from __future__ import annotations

import json
import shutil
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final, cast

from specbench.adapters.claude_credentials import without_oauth_token
from specbench.adapters.claude_gate_session import plugin_directories, session_argv, session_environment, session_prompt
from specbench.adapters.claude_stream import check_claude_harness, parse_claude_stream
from specbench.adapters.execution_counter import intact_stream
from specbench.adapters.gate_session_stream import ticket_launches
from specbench.adapters.host_store import HostStoreRequests
from specbench.adapters.product_cli import EXIT_AWAITING_HOST, ProductCli, ProveRequest
from specbench.adapters.proof_publication import ProofPublication, PublicationContext
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.application.ports import (
    ClaudeCredentialSource,
    CredentialSourceFailed,
    ExecutionCounter,
    GateSessionRunner,
    GateSessionSpec,
    TreeReader,
)
from specbench.domain.agent_events import HarnessOutcome
from specbench.domain.arm_policy import ArmPolicy, policy_for
from specbench.domain.arms import Arm, Host
from specbench.domain.canonical import JsonValue
from specbench.domain.clean_room import CleanRoom, RoomArea, portable
from specbench.domain.gate import GateMode, GateOutcome, GateStatus, TicketUsage
from specbench.domain.host_requests import request_id_of, request_kind_of
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.product import UMBRELLA_PLUGIN_NAME, ProofModule, proof_modules_in_order
from specbench.domain.sealing import tree_digest
from specbench.domain.sessions import live_gate_call_id
from specbench.domain.shallnot_config import arm_shallnot_config

SHALLNOT_BINARY: Final = "shallnot"
SHALLNOT_CONFIG: Final = "shallnot.yaml"
SHALLNOT_ENV: Final = "SPEC_PROOF_SHALLNOT"
STATE_ENV: Final = "XDG_STATE_HOME"
CACHE_ENV: Final = "XDG_CACHE_HOME"
PATH_ENV: Final = "PATH"
REPORT_FILE: Final = "report.json"
RUN_ID_DIGEST_LENGTH: Final = 16
CI_RUN_PREFIX: Final = "bench"
EVENT_TYPE: Final = "type"
EVENT_SUBTYPE: Final = "subtype"
SYSTEM_EVENT: Final = "system"
INIT_SUBTYPE: Final = "init"


class GateHostMisuse(ValueError):
    """The gate was asked of an arm or a mode this host does not serve."""


@dataclass(frozen=True)
class IsolatedHostConfig:
    """What the host needs from the machine: the product build, the plugin trees, the rooms and the credential."""

    repository: Path
    binaries: Path
    umbrella_plugin: Path
    rooms_root: Path
    tool_path: tuple[Path, ...] = ()
    claude_executable: str = "claude"

    @property
    def shallnot(self) -> Path:
        return self.binaries / SHALLNOT_BINARY


@dataclass(frozen=True)
class _LiveRun:
    """How a `LIVE` session ended: its status (`COMPLETE` when the settle step may follow), id and streamed lines."""

    status: GateStatus
    session_id: str | None
    lines: tuple[str, ...]


class _EventRecorder:
    """Appends every streamed line to the call's events file and stops the session whose `system/init` lacks a plugin or
    an executor server."""

    def __init__(self, events_file: Path, plugins: set[str], servers: set[str]) -> None:
        events_file.parent.mkdir(parents=True, exist_ok=True)
        self._handle = events_file.open("a", encoding="utf-8")
        self._plugins = plugins
        self._servers = servers
        self.lines: list[str] = []
        self.init_seen = False
        self.misconfigured = False

    def accept(self, line: str) -> bool:
        self._handle.write(line + "\n")
        self._handle.flush()
        self.lines.append(line)
        if self.init_seen or not _is_init(line):
            return True
        self.init_seen = True
        check = check_claude_harness(parse_claude_stream(line).init, self._plugins, self._servers)
        self.misconfigured = check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED
        return not self.misconfigured

    def close(self) -> None:
        self._handle.close()


def _is_init(line: str) -> bool:
    try:
        body: object = json.loads(line)
    except json.JSONDecodeError:
        return False
    if not isinstance(body, Mapping):
        return False
    fields = cast("Mapping[str, object]", body)
    return fields.get(EVENT_TYPE) == SYSTEM_EVENT and fields.get(EVENT_SUBTYPE) == INIT_SUBTYPE


class IsolatedCliHost:
    """Strategy: a `claude -p` gate-host session in a clean room, or a replay of its recorded answers."""

    def __init__(
        self,
        config: IsolatedHostConfig,
        layout: RunLayout,
        cli: ProductCli,
        shallnot: ShallnotCli,
        sessions: GateSessionRunner,
        publication: ProofPublication,
        tree_reader: TreeReader,
        counter: ExecutionCounter,
        credentials: ClaudeCredentialSource,
        base_environment: Mapping[str, str],
    ) -> None:
        self._config = config
        self._layout = layout
        self._cli = cli
        self._shallnot = shallnot
        self._sessions = sessions
        self._publication = publication
        self._reader = tree_reader
        self._counter = counter
        self._credentials = credentials
        self._base_environment = without_oauth_token(base_environment)

    def run_gate(self, tree: Path, arm: Arm, record_dir: Path, mode: GateMode) -> GateOutcome:
        policy = _served_policy(arm)
        modules = proof_modules_in_order(policy.modules)
        digest = tree_digest(self._reader.read(tree))
        room = CleanRoom.locate(self._config.rooms_root, arm, digest)
        live: _LiveRun | None = None
        if modules and mode is GateMode.LIVE:
            live = self._run_session(tree, arm, policy, modules, room.session, record_dir)
            if live.status is not GateStatus.COMPLETE:
                return GateOutcome(None, live.status, live.session_id, self._counter.count(live.lines), ())
        elif modules and not record_dir.is_dir():
            raise GateHostMisuse(f"a replay needs the recorded answers in {record_dir}")
        run_id = f"{CI_RUN_PREFIX}-{arm.value.lower()}-{digest[:RUN_ID_DIGEST_LENGTH]}"
        report = self._settle(tree, arm, policy, modules, room.settle, record_dir, run_id)
        if report is None:
            executions = self._counter.count(live.lines) if live else 0
            return GateOutcome(None, GateStatus.PAUSED, live.session_id if live else None, executions, ())
        if live is None:
            return GateOutcome(report, GateStatus.COMPLETE, None, 0, ())
        tickets = self._tickets(live.lines, room.session)
        return GateOutcome(report, GateStatus.COMPLETE, live.session_id, self._counter.count(live.lines), tickets)

    def _run_session(
        self,
        tree: Path,
        arm: Arm,
        policy: ArmPolicy,
        modules: Sequence[ProofModule],
        area: RoomArea,
        record_dir: Path,
    ) -> _LiveRun:
        token = self._token()
        if not token:
            return _LiveRun(GateStatus.HARNESS_MISCONFIGURED, None, ())
        self._prepare(tree, area, policy, modules)
        record_dir.mkdir(parents=True, exist_ok=True)
        area.config_directory.mkdir(parents=True, exist_ok=True)
        plugins = {UMBRELLA_PLUGIN_NAME, *[module.plugin for module in modules]}
        recorder = _EventRecorder(
            self._layout.call(live_gate_call_id(arm, record_dir)) / CALL_EVENTS, plugins, {m.server for m in modules}
        )
        spec = GateSessionSpec(
            session_argv(
                session_prompt(record_dir),
                plugin_directories(self._config.repository, self._config.umbrella_plugin, modules),
                self._config.claude_executable,
            ),
            session_environment(self._base_environment, self._area_environment(area), area.config_directory, token),
            area.tree,
        )
        try:
            end = self._sessions.run(spec, recorder.accept)
        finally:
            recorder.close()
        stream = parse_claude_stream(intact_stream(recorder.lines))
        session_id = stream.init.session_id if stream.init else None
        if recorder.misconfigured or not recorder.init_seen:
            return _LiveRun(GateStatus.HARNESS_MISCONFIGURED, session_id, tuple(recorder.lines))
        finished = end.exit_code == 0 and stream.result is not None and not stream.result.is_error
        status = GateStatus.COMPLETE if finished else GateStatus.PAUSED
        return _LiveRun(status, session_id, tuple(recorder.lines))

    def _token(self) -> str | None:
        """The token of the session, or `None` when no source holds one or a source cannot tell."""
        try:
            return self._credentials.token()
        except CredentialSourceFailed:
            return None

    def _settle(
        self,
        tree: Path,
        arm: Arm,
        policy: ArmPolicy,
        modules: Sequence[ProofModule],
        area: RoomArea,
        record_dir: Path,
        run_id: str,
    ) -> JsonValue | None:
        """The portable Shallnot report of the recorded answers, or `None` when they leave a ticket unanswered."""
        shutil.rmtree(area.root, ignore_errors=True)
        self._prepare(tree, area, policy, modules)
        env = self._area_environment(area)
        for module in modules:
            if self._cli.prove(module, area.tree, env, ProveRequest(replay=record_dir)) == EXIT_AWAITING_HOST:
                return None
        if modules:
            self._publication.publish(PublicationContext(area, env, modules, run_id))
        config = area.tree / f"shallnot.{arm.value.lower()}.yaml"
        config.write_text(_arm_config(area.tree / SHALLNOT_CONFIG, policy, self._publication), encoding="utf-8")
        gate = self._shallnot.gate(area.tree, config, area.root / REPORT_FILE, env)
        return portable(gate.report, {str(area.tree), str(area.tree.resolve())})

    def _prepare(self, tree: Path, area: RoomArea, policy: ArmPolicy, modules: Sequence[ProofModule]) -> None:
        """Copies the tree and installs Shallnot and each module in it; a prepared area is left as it is."""
        if area.is_ready:
            return
        shutil.rmtree(area.root, ignore_errors=True)
        shutil.copytree(tree, area.tree, symlinks=True)
        area.state.mkdir(parents=True)
        area.cache.mkdir(parents=True)
        env = self._area_environment(area)
        if policy.has_shallnot:
            self._shallnot.init(area.tree, env)
        for module in modules:
            self._cli.init(module, area.tree, env)
            self._cli.enable(module, area.tree, env)
        area.ready_marker.write_text("ready\n", encoding="utf-8")

    def _area_environment(self, area: RoomArea) -> dict[str, str]:
        search_path = [str(self._config.binaries), *map(str, self._config.tool_path)]
        inherited = self._base_environment.get(PATH_ENV)
        return {
            PATH_ENV: ":".join([*search_path, *([inherited] if inherited else [])]),
            STATE_ENV: str(area.state),
            CACHE_ENV: str(area.cache),
            SHALLNOT_ENV: str(self._config.shallnot),
        }

    def _tickets(self, lines: Sequence[str], area: RoomArea) -> tuple[TicketUsage, ...]:
        requests = HostStoreRequests(area.state)
        usages: list[TicketUsage] = []
        for launch in ticket_launches(intact_stream(lines)):
            request_id = request_id_of(launch.ticket)
            stored = requests.find(request_id)
            if stored is None:
                continue
            kind = request_kind_of(stored.module.module, stored.kind)
            usages.append(TicketUsage(request_id, kind, stored.module.module, launch.tokens, launch.resolved_model))
        return tuple(usages)


def _served_policy(arm: Arm) -> ArmPolicy:
    policy = policy_for(arm)
    if policy.host is not Host.ISOLATED_CLI or not policy.has_shallnot:
        raise GateHostMisuse(f"{arm.value} is not gated by a clean-room gate-host session")
    return policy


def _arm_config(base: Path, policy: ArmPolicy, publication: ProofPublication) -> str:
    return arm_shallnot_config(base.read_text(encoding="utf-8"), policy.proofs_require, publication.trust)
