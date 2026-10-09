from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.isolated_cli_host import GateHostMisuse, IsolatedCliHost
from specbench.adapters.isolated_host_config import IsolatedHostConfig
from specbench.adapters.isolated_host_factory import SigningMaterial, build_isolated_host
from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.application.ports import (
    ClaudeCredentialSource,
    EventSink,
    GateSessionEnd,
    GateSessionSpec,
    ProcessResult,
)
from specbench.domain.arms import Arm
from specbench.domain.canonical import JsonValue
from specbench.domain.gate import GateMode, GateStatus
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.sessions import live_gate_call_id
from support.credentials import FAKE_TOKEN, FailingCredentialSource, StaticCredentialSource
from support.pinned_claude import TEST_CLAUDE_VERSION, FixedClaude

BINARIES = Path("/tools/bin")


class SilentRunner:
    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        return ProcessResult(0, "{}", "")


class NoChecks:
    def problems(self, schema: JsonValue, document: JsonValue) -> list[str]:
        return []


class ScriptedSession:
    def __init__(self, lines: list[str]) -> None:
        self.lines = lines
        self.delivered: list[str] = []
        self.specs: list[GateSessionSpec] = []

    def run(self, spec: GateSessionSpec, on_event: EventSink) -> GateSessionEnd:
        self.specs.append(spec)
        for line in self.lines:
            self.delivered.append(line)
            if not on_event(line):
                return GateSessionEnd(143)
        return GateSessionEnd(0)


def init_line(plugins: list[str], servers: list[tuple[str, str]]) -> str:
    return json.dumps(
        {
            "type": "system",
            "subtype": "init",
            "session_id": "s-1",
            "claude_code_version": TEST_CLAUDE_VERSION,
            "plugins": [{"name": name} for name in plugins],
            "mcp_servers": [{"name": name, "status": status} for name, status in servers],
        }
    )


def agent_line(number: int) -> str:
    block = {"type": "tool_use", "id": f"toolu_{number}", "name": "Agent", "input": {"prompt": "x"}}
    return json.dumps({"type": "assistant", "message": {"content": [block]}, "parent_tool_use_id": None})


def make_tree(tmp_path: Path) -> Path:
    tree = tmp_path / "tree"
    (tree / "specs").mkdir(parents=True)
    (tree / "specs" / "spec.md").write_text("# spec\n")
    return tree


def make_host(
    tmp_path: Path,
    session: ScriptedSession,
    token: str | None = "token",
    credentials: ClaudeCredentialSource | None = None,
) -> IsolatedCliHost:
    config = IsolatedHostConfig(
        Path("/repo"), BINARIES, Path("/tools/plugins/spec-proof"), tmp_path / "rooms", FixedClaude()
    )
    return build_isolated_host(
        config,
        RunLayout(tmp_path / "run"),
        SilentRunner(),
        NoChecks(),
        session,
        SigningMaterial(Path("/signing/key.pem"), Path("/signing/trust-policy.yaml")),
        DiskTreeReader(),
        ClaudeExecutionCounter(),
        credentials or StaticCredentialSource(token),
        {"PATH": "/usr/bin"},
    )


ALIGNMENT_PLUGINS = ["spec-proof", "spec-proof-alignment"]
ALIGNMENT_SERVER = ("plugin:spec-proof-alignment:exec", "connected")


@pytest.mark.parametrize("arm", [Arm.N, Arm.T, Arm.ALL_NATIVE, Arm.CHEAT])
def test_an_arm_without_a_clean_room_gate_is_refused(tmp_path: Path, arm: Arm) -> None:
    host = make_host(tmp_path, ScriptedSession([]))
    with pytest.raises(GateHostMisuse, match="not gated by a clean-room"):
        host.run_gate(make_tree(tmp_path), arm, tmp_path / "rec", GateMode.LIVE)


def test_a_replay_without_recorded_answers_is_refused(tmp_path: Path) -> None:
    host = make_host(tmp_path, ScriptedSession([]))
    with pytest.raises(GateHostMisuse, match="recorded answers"):
        host.run_gate(make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.REPLAY)


def test_a_session_without_a_credential_never_starts(tmp_path: Path) -> None:
    session = ScriptedSession([init_line(ALIGNMENT_PLUGINS, [ALIGNMENT_SERVER])])
    outcome = make_host(tmp_path, session, token=None).run_gate(
        make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE
    )
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED
    assert (outcome.report, outcome.executions, session.specs) == (None, 0, [])


def test_a_missing_plugin_in_system_init_stops_the_session_as_misconfigured(tmp_path: Path) -> None:
    lines = [init_line(["spec-proof"], [ALIGNMENT_SERVER]), agent_line(1), agent_line(2)]
    session = ScriptedSession(lines)
    outcome = make_host(tmp_path, session).run_gate(make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED
    assert outcome.report is None
    assert outcome.session_id == "s-1"
    assert session.delivered == lines[:1]


def test_a_disconnected_executor_server_is_misconfigured_too(tmp_path: Path) -> None:
    session = ScriptedSession([init_line(ALIGNMENT_PLUGINS, [("plugin:spec-proof-alignment:exec", "failed")])])
    outcome = make_host(tmp_path, session).run_gate(make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED


def test_a_session_that_never_says_init_is_misconfigured(tmp_path: Path) -> None:
    session = ScriptedSession([agent_line(1)])
    outcome = make_host(tmp_path, session).run_gate(make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED
    assert outcome.executions == 2


def test_a_session_that_ends_without_a_result_is_paused_and_charged_for_what_it_streamed(tmp_path: Path) -> None:
    lines = [init_line(ALIGNMENT_PLUGINS, [ALIGNMENT_SERVER]), agent_line(1), agent_line(2)]
    outcome = make_host(tmp_path, ScriptedSession(lines)).run_gate(
        make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE
    )
    assert outcome.status is GateStatus.PAUSED
    assert outcome.executions == 3


def test_every_streamed_line_lands_in_the_events_file_of_the_call(tmp_path: Path) -> None:
    lines = [init_line(ALIGNMENT_PLUGINS, [ALIGNMENT_SERVER]), agent_line(1)]
    record = tmp_path / "rec"
    make_host(tmp_path, ScriptedSession(lines)).run_gate(make_tree(tmp_path), Arm.AL, record, GateMode.LIVE)
    events = RunLayout(tmp_path / "run").call(live_gate_call_id(Arm.AL, record)) / CALL_EVENTS
    assert events.read_text().splitlines() == lines


def test_a_second_attempt_appends_to_the_events_file_of_the_first(tmp_path: Path) -> None:
    lines = [init_line(ALIGNMENT_PLUGINS, [ALIGNMENT_SERVER]), agent_line(1)]
    record, tree = tmp_path / "rec", make_tree(tmp_path)
    for _ in range(2):
        make_host(tmp_path, ScriptedSession(lines)).run_gate(tree, Arm.AL, record, GateMode.LIVE)
    events = RunLayout(tmp_path / "run").call(live_gate_call_id(Arm.AL, record)) / CALL_EVENTS
    assert events.read_text().splitlines() == lines * 2


def test_the_session_runs_in_its_own_copy_of_the_tree_with_an_isolated_configuration(tmp_path: Path) -> None:
    session = ScriptedSession([init_line(["spec-proof"], [])])
    tree = make_tree(tmp_path)
    make_host(tmp_path, session).run_gate(tree, Arm.AL, tmp_path / "rec", GateMode.LIVE)
    [spec] = session.specs
    assert spec.cwd != tree
    assert (spec.cwd / "specs" / "spec.md").read_text() == "# spec\n"
    assert spec.env["CLAUDE_CODE_OAUTH_TOKEN"] == "token"
    assert spec.env["CLAUDE_CONFIG_DIR"].startswith(str(tmp_path / "rooms"))
    assert spec.env["XDG_STATE_HOME"].startswith(str(tmp_path / "rooms"))
    assert spec.env["PATH"].startswith(str(BINARIES))


def test_a_session_receives_the_token_of_its_source_in_its_environment_and_not_in_its_representation(
    tmp_path: Path,
) -> None:
    session = ScriptedSession([init_line(["spec-proof"], [])])
    make_host(tmp_path, session, token=FAKE_TOKEN).run_gate(
        make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE
    )
    [spec] = session.specs
    assert spec.env["CLAUDE_CODE_OAUTH_TOKEN"] == FAKE_TOKEN
    assert FAKE_TOKEN not in repr(spec)
    assert [path for path in tmp_path.rglob("*") if path.is_file() and FAKE_TOKEN.encode() in path.read_bytes()] == []


def test_a_source_that_cannot_tell_keeps_the_session_from_starting(tmp_path: Path) -> None:
    session = ScriptedSession([init_line(["spec-proof"], [])])
    host = make_host(tmp_path, session, credentials=FailingCredentialSource())
    outcome = host.run_gate(make_tree(tmp_path), Arm.AL, tmp_path / "rec", GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED and session.specs == []
