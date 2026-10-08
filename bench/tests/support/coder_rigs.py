"""Test rigs for the coder-session adapters: a run directory, a workspace, a fake owner home and a fake CLI."""

from __future__ import annotations

import os
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path

from specbench.adapters.claude_session import OAUTH_TOKEN_VARIABLE, ClaudeSessionHarness, ClaudeSessionSetup
from specbench.adapters.codex_session import CodexSessionHarness, CodexSessionSetup
from specbench.adapters.streaming_process import StreamingRunner
from specbench.domain.arms import FROZEN_MODEL, Arm, Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import SessionKind, SessionRequest
from support.fake_claude import install_fake_claude_sessions
from support.fake_codex import install_fake_codex

PLENTY_OF_MEMORY = 100
KILL_AFTER_SECONDS = 6
TEST_TOKEN = "test-oauth-token"
TRUNK_ID = "task-a"
DEFAULT_PROMPT = "implement the change"
OWNER_CONFIG = 'model = "gpt-6-luna"\n\n[projects."/home/owner/work"]\ntrust_level = "trusted"\n'


def free_runner(timeout_seconds: int = 60) -> StreamingRunner:
    return StreamingRunner(lambda: PLENTY_OF_MEMORY, timeout_seconds, minimum_percent=0)


def request_for(
    harness: Harness,
    workspace: Path,
    key: str,
    kind: SessionKind,
    arm: Arm | None,
    resume: str | None,
    fork: bool,
    prompt: str,
    trunk_id: str = TRUNK_ID,
) -> SessionRequest:
    return SessionRequest(
        harness, FROZEN_MODEL[harness], kind, key, workspace, prompt, resume, fork, trunk_id=trunk_id, arm=arm
    )


def base_environment(owner_home: Path) -> dict[str, str]:
    return {"PATH": os.environ["PATH"], "HOME": str(owner_home)}


@dataclass(frozen=True)
class ClaudeRig:
    root: Path
    layout: RunLayout
    workspace: Path
    owner_home: Path
    fake_dir: Path
    harness: ClaudeSessionHarness

    def request(
        self,
        key: str,
        *,
        kind: SessionKind = SessionKind.TRUNK,
        arm: Arm | None = Arm.T,
        resume: str | None = None,
        fork: bool = False,
        prompt: str = DEFAULT_PROMPT,
    ) -> SessionRequest:
        return request_for(Harness.CLAUDE_CODE, self.workspace, key, kind, arm, resume, fork, prompt)


def make_claude_rig(
    root: Path,
    responses: Sequence[Mapping[str, JsonValue]],
    *,
    environ: Mapping[str, str] | None = None,
    plugin_dirs: tuple[Path, ...] = (),
    required_plugins: tuple[str, ...] = (),
    toolchain_roots: tuple[Path, ...] = (),
    timeout_seconds: int = 60,
    executable_name: str | None = None,
) -> ClaudeRig:
    layout = RunLayout(root / "run")
    workspace = root / "ws"
    workspace.mkdir()
    owner_home = root / "owner"
    owner_home.mkdir()
    fake_dir = root / "fake"
    fake_dir.mkdir()
    executable = install_fake_claude_sessions(fake_dir, responses)
    default = {**base_environment(owner_home), OAUTH_TOKEN_VARIABLE: TEST_TOKEN}
    setup = ClaudeSessionSetup(layout, plugin_dirs, required_plugins, toolchain_roots=toolchain_roots)
    harness = ClaudeSessionHarness(
        setup,
        free_runner(timeout_seconds),
        default if environ is None else environ,
        executable_name or str(executable),
    )
    return ClaudeRig(root, layout, workspace.resolve(), owner_home, fake_dir, harness)


@dataclass(frozen=True)
class CodexRig:
    root: Path
    layout: RunLayout
    workspace: Path
    codex_home: Path
    fake_dir: Path
    harness: CodexSessionHarness

    def request(
        self,
        key: str,
        *,
        kind: SessionKind = SessionKind.TRUNK,
        arm: Arm | None = Arm.T,
        resume: str | None = None,
        fork: bool = False,
        prompt: str = DEFAULT_PROMPT,
    ) -> SessionRequest:
        return request_for(Harness.CODEX, self.workspace, key, kind, arm, resume, fork, prompt)

    @property
    def config(self) -> Path:
        return self.codex_home / "config.toml"


def make_codex_rig(
    root: Path,
    responses: Sequence[Mapping[str, JsonValue]],
    *,
    owner_config: str = OWNER_CONFIG,
    timeout_seconds: int = 60,
) -> CodexRig:
    layout = RunLayout(root / "run")
    workspace = root / "ws"
    workspace.mkdir()
    codex_home = root / "owner-codex"
    codex_home.mkdir()
    (codex_home / "config.toml").write_bytes(owner_config.encode())
    (codex_home / "auth.json").write_text('{"token": "owner-login"}')
    fake_dir = root / "fake"
    fake_dir.mkdir()
    executable = install_fake_codex(fake_dir, responses)
    environ = {**base_environment(root / "owner"), "CODEX_HOME": str(codex_home)}
    harness = CodexSessionHarness(
        CodexSessionSetup(layout, codex_home), free_runner(timeout_seconds), environ, str(executable)
    )
    return CodexRig(root, layout, workspace.resolve(), codex_home, fake_dir, harness)
