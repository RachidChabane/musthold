"""`IsolatedCiRoundTrip`: the CI round trip of an honest arm (ADR 0011, decision 12).

CI left requests that no pushed answer covers. The bench stages them as the artifact a CI job would upload, pulls them
into the session area's host store through a `gh` shim that no session can reach, lets a gate-host session answer them
with the shipped fulfil skills (resuming the session of the stop when it has one), pushes the answers and archives the
pushed ref under `<record dir>/ci/<k>/<module>/`. The archive is what a replay reads, so a replay makes no model call.
"""

from __future__ import annotations

import shutil
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.adapters.area_environment import PATH_ENV, PATH_SEPARATOR, AreaEnvironment
from specbench.adapters.area_preparer import AreaPreparer
from specbench.adapters.ci_round_archive import CiRoundArchive
from specbench.adapters.claude_credentials import token_or_none, without_oauth_token
from specbench.adapters.claude_gate_session import (
    plugin_directories,
    round_prompt,
    session_argv,
    session_environment,
)
from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.gate_session_driver import GateSessionDriver, LiveRun
from specbench.adapters.isolated_host_config import IsolatedHostConfig
from specbench.adapters.product_cli import ProductCli
from specbench.adapters.session_state import SessionState, SessionStateFile
from specbench.adapters.ticket_usage import ticket_usages
from specbench.application.ports import ClaudeCredentialSource, ExecutionCounter, GateSessionSpec
from specbench.domain.ci_round import CiRoundRequest, CiRoundResult
from specbench.domain.clean_room import RoomArea
from specbench.domain.gate import GateStatus
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.product import UMBRELLA_PLUGIN_NAME
from specbench.domain.sessions import ci_round_call_id

ARTIFACTS_DIRECTORY: Final = "ci-artifacts"
EXPORTS_DIRECTORY: Final = "ci-exports"
ARTIFACTS_ENV: Final = "SPECBENCH_CI_ARTIFACTS"
ARTIFACT_PREFIX: Final = "spec-proof-host-"


class IsolatedCiRoundTrip:
    def __init__(
        self,
        config: IsolatedHostConfig,
        layout: RunLayout,
        driver: GateSessionDriver,
        preparer: AreaPreparer,
        environment: AreaEnvironment,
        cli: ProductCli,
        repository: CleanRoomRepository,
        counter: ExecutionCounter,
        credentials: ClaudeCredentialSource,
        base_environment: Mapping[str, str],
    ) -> None:
        self._config = config
        self._layout = layout
        self._driver = driver
        self._preparer = preparer
        self._environment = environment
        self._cli = cli
        self._repository = repository
        self._counter = counter
        self._credentials = credentials
        self._base_environment = without_oauth_token(base_environment)

    def run(self, request: CiRoundRequest) -> CiRoundResult:
        area = request.session_area
        token = token_or_none(self._credentials)
        if not token:
            return CiRoundResult(GateStatus.HARNESS_MISCONFIGURED, None, 0, ())
        self._preparer.prepare(request.tree, area, request.policy, request.modules, request.run_id)
        if self._repository.has_tracked_changes(area.tree):
            return CiRoundResult(GateStatus.HARNESS_MISCONFIGURED, None, 0, ())
        env = self._environment.of(area)
        self._pull(request, area, env)
        live = self._run_session(request, area, env, token)
        executions = self._counter.count(live.lines)
        tickets = ticket_usages(live.lines, area)
        if live.status is not GateStatus.COMPLETE:
            return CiRoundResult(live.status, live.session_id, executions, tickets)
        exports = self._push(request, area, env)
        if exports is None:
            return CiRoundResult(GateStatus.PAUSED, live.session_id, executions, tickets)
        CiRoundArchive(request.record_dir).archive(request.number, exports)
        return CiRoundResult(GateStatus.COMPLETE, live.session_id, executions, tickets)

    def _pull(self, request: CiRoundRequest, area: RoomArea, env: Mapping[str, str]) -> None:
        """Stages each pending module's requests as its CI artifact and pulls them with the bench's own `gh`."""
        artifacts = area.root / ARTIFACTS_DIRECTORY
        shutil.rmtree(artifacts, ignore_errors=True)
        for module in request.pending:
            staged = artifacts / request.run_id / f"{ARTIFACT_PREFIX}{module.host_ref_name}"
            shutil.copytree(request.ci_issued / module.host_ref_name, staged)
        pull_env = {
            **env,
            ARTIFACTS_ENV: str(artifacts),
            PATH_ENV: PATH_SEPARATOR.join([str(self._config.gh_shim), env[PATH_ENV]]),
        }
        for module in request.pending:
            self._cli.host_pull(module, area.tree, pull_env, request.run_id)

    def _run_session(self, request: CiRoundRequest, area: RoomArea, env: Mapping[str, str], token: str) -> LiveRun:
        state_file = SessionStateFile(area)
        state = state_file.read()
        area.config_directory.mkdir(parents=True, exist_ok=True)
        spec = GateSessionSpec(
            session_argv(
                round_prompt(request.pending),
                plugin_directories(self._config.repository, self._config.umbrella_plugin, request.modules),
                self._config.claude.executable(),
                resume=state.session_id,
            ),
            session_environment(self._base_environment, env, area.config_directory, token),
            area.tree,
        )
        events = self._layout.call(ci_round_call_id(request.arm, request.record_dir, request.number)) / CALL_EVENTS
        plugins = {UMBRELLA_PLUGIN_NAME, *[module.plugin for module in request.modules]}
        live = self._driver.run(
            spec, events, plugins, {module.server for module in request.modules}, self._config.claude.version
        )
        if live.session_id:
            state_file.write(SessionState(live.session_id, state.finished))
        return live

    def _push(self, request: CiRoundRequest, area: RoomArea, env: Mapping[str, str]) -> dict[str, Path] | None:
        """Pushes the answers of each pending module and exports its ref; `None` when a module published nothing."""
        exports: dict[str, Path] = {}
        root = area.root / EXPORTS_DIRECTORY / str(request.number)
        shutil.rmtree(root, ignore_errors=True)
        for module in request.pending:
            if not self._cli.host_push(module, area.tree, env):
                return None
            export = root / module.host_ref_name
            if not self._repository.export_host_ref(area.tree, module.host_ref_name, export):
                return None
            exports[module.host_ref_name] = export
        return exports
