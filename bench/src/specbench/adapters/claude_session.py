"""`AgentHarness` over `claude -p`: one coder session per call, isolated from the owner's Claude Code.

Isolation (ADR 0011 decision 9):

- every trunk runs with its own `CLAUDE_CONFIG_DIR` under the run, authenticated by `CLAUDE_CODE_OAUTH_TOKEN`; a
  missing token ends the session as `harness_misconfigured` before anything starts, never as a fallback to the
  owner's home;
- the bench's settings file lives in the run, outside the workspace, and is the only settings file besides the home's:
  honest arms read the `user` source alone, so project settings that an agent writes never load on resume; arms whose
  coder is the host read `user,project`, because the product's own files stay where `init` puts them;
- network is denied through the sandbox's empty allowlist, and `WebFetch` and `WebSearch` through permissions.

Events stream to `calls/<call-id>/events.jsonl` as they arrive. The stream is then parsed with `claude_stream` and
scanned for network use, URLs and paths outside the workspace.
"""

from __future__ import annotations

import json
from collections.abc import Collection, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.claude_cli import NESTING_ENV_VARS
from specbench.adapters.claude_homes import ClaudeConfigHomes
from specbench.adapters.claude_stream import ClaudeStream, check_claude_harness, parse_claude_stream
from specbench.adapters.execution_counter import ClaudeExecutionCounter, intact_stream
from specbench.adapters.run_store import write_atomically
from specbench.adapters.session_environment import bench_home, neutral_environment
from specbench.adapters.session_events import SessionEventLog
from specbench.adapters.streaming_process import StreamedProcess, StreamingRunner
from specbench.domain.agent_events import HarnessOutcome
from specbench.domain.arm_policy import ArmPolicy, policy_for
from specbench.domain.arms import Arm, Harness
from specbench.domain.contamination import DEFAULT_ALLOWED_PREFIXES, Finding, ScanPolicy, scan_actions
from specbench.domain.jsonread import RecordError
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import SessionKind, SessionOutcome, SessionRequest, SessionResult

CLAUDE_EXECUTABLE: Final = "claude"
OAUTH_TOKEN_VARIABLE: Final = "CLAUDE_CODE_OAUTH_TOKEN"
CONFIG_DIR_VARIABLE: Final = "CLAUDE_CONFIG_DIR"
INHERITED_PREFIXES_DROPPED: Final = ("ANTHROPIC_", "CLAUDE_CODE_", "CLAUDE_CONFIG_DIR")
"""Variables that would reroute the model, the credential or the home of a session are never inherited."""
NOT_STARTED_EXIT_CODE: Final = 78
"""`EX_CONFIG`: the session never started because the harness is not configured."""
MISSING_TOKEN_MESSAGE: Final = (
    f"{OAUTH_TOKEN_VARIABLE} is not set: create the long-lived credential with `claude setup-token` and export it; "
    "a coder session never falls back to the credentials of ~/.claude"
)
STDERR_EXCERPT_LENGTH: Final = 300
DIAGNOSTICS_LENGTH: Final = 4000
DENIED_TOOLS: Final = ("WebFetch", "WebSearch")
PERMISSION_MODE: Final = "acceptEdits"


@dataclass(frozen=True)
class ClaudeSessionSetup:
    """What every Claude coder session of a run shares.

    `plugin_dirs` and `required_plugins` apply to the arms whose coder is the host. `toolchain_roots` are the places
    outside a workspace where a coder legitimately runs: the task environments and the toolchains; they are writable
    in the sandbox and are not out-of-workspace paths for the scan.
    """

    layout: RunLayout
    plugin_dirs: tuple[Path, ...] = ()
    required_plugins: tuple[str, ...] = ()
    required_servers: tuple[str, ...] = ()
    toolchain_roots: tuple[Path, ...] = ()


@dataclass(frozen=True)
class ClaudeAssessment:
    """What a session's own events say about how it went."""

    outcome: SessionOutcome
    problems: tuple[str, ...]
    session_id: str
    final_message: str
    model_reported: str | None
    executions: int


def claude_settings(toolchain_roots: Sequence[Path]) -> dict[str, object]:
    """The bench's settings of a coder session: network denied, web tools denied, edits accepted."""
    return {
        "permissions": {"defaultMode": PERMISSION_MODE, "deny": list(DENIED_TOOLS)},
        "sandbox": {
            "enabled": True,
            "autoAllowBashIfSandboxed": True,
            "allowUnsandboxedCommands": False,
            "network": {"allowedDomains": []},
            "filesystem": {"allowWrite": [str(root) for root in toolchain_roots]},
        },
    }


def claude_argv(
    request: SessionRequest, policy: ArmPolicy, settings_file: Path, plugin_dirs: Sequence[Path]
) -> list[str]:
    """The command line of one session. The prompt is not on it: it goes through standard input."""
    argv = [
        CLAUDE_EXECUTABLE,
        "-p",
        "--model",
        request.model,
        "--output-format",
        "stream-json",
        "--verbose",
        "--include-hook-events",
        "--permission-mode",
        PERMISSION_MODE,
        "--setting-sources",
        ",".join(policy.setting_sources),
        "--settings",
        str(settings_file),
    ]
    if policy.loads_product_plugins:
        for plugin_dir in plugin_dirs:
            argv += ["--plugin-dir", str(plugin_dir)]
    if request.resume_session_id is not None:
        argv += ["--resume", request.resume_session_id]
        if request.fork:
            argv.append("--fork-session")
    return argv


def claude_environment(environ: Mapping[str, str], config_dir: Path, token: str) -> dict[str, str]:
    """The environment of a session: the inherited one without the variables that reroute Claude, plus the home and
    the credential."""
    kept = {
        name: value
        for name, value in environ.items()
        if name not in NESTING_ENV_VARS and not name.startswith(INHERITED_PREFIXES_DROPPED)
    }
    return {**kept, CONFIG_DIR_VARIABLE: str(config_dir), OAUTH_TOKEN_VARIABLE: token}


def scan_policy_for(workspace: Path, unresolved: Path, toolchain_roots: Sequence[Path]) -> ScanPolicy:
    prefixes = (*DEFAULT_ALLOWED_PREFIXES, *(str(root) for root in toolchain_roots), str(unresolved))
    return ScanPolicy(str(workspace), prefixes)


def describe_finding(finding: Finding) -> str:
    return f"{finding.kind.value} at event {finding.pointer.line}: {finding.evidence}"


def assess_claude_session(
    events: Sequence[str],
    policy: ArmPolicy,
    setup: ClaudeSessionSetup,
    scan: ScanPolicy,
    interrupted: bool,
) -> ClaudeAssessment:
    """Judge a session from its events alone, so that a recorded session is judged the same way as a live one.

    Precedence: a misconfigured harness, then an interrupted session, then contamination.
    """
    try:
        stream = _parsed(events)
    except RecordError as error:
        return ClaudeAssessment(SessionOutcome.HARNESS_MISCONFIGURED, (str(error),), "", "", None, 0)
    problems = _configuration_problems(stream, policy, setup)
    findings = tuple(describe_finding(finding) for finding in scan_actions(stream.actions, scan))
    started = stream.init is not None
    if problems and (started or not interrupted):
        outcome = SessionOutcome.HARNESS_MISCONFIGURED
    elif interrupted:
        outcome = SessionOutcome.INTERRUPTED
    elif findings:
        outcome = SessionOutcome.CONTAMINATED
    else:
        outcome = SessionOutcome.COMPLETED
    return ClaudeAssessment(
        outcome,
        (*problems, *findings),
        _session_id(stream),
        stream.result.final_message if stream.result is not None else "",
        _model_reported(stream),
        ClaudeExecutionCounter().count(events),
    )


def _parsed(events: Sequence[str]) -> ClaudeStream:
    text = intact_stream(events)
    return parse_claude_stream(text) if text else ClaudeStream(None, (), (), {}, None)


def _configuration_problems(stream: ClaudeStream, policy: ArmPolicy, setup: ClaudeSessionSetup) -> tuple[str, ...]:
    required_plugins: Collection[str] = setup.required_plugins if policy.loads_product_plugins else ()
    required_servers: Collection[str] = setup.required_servers if policy.loads_product_plugins else ()
    check = check_claude_harness(stream.init, required_plugins, required_servers)
    problems = (
        [f"missing {name}" for name in check.missing] if check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED else []
    )
    if stream.init is not None and not policy.loads_product_plugins:
        problems += [f"unexpected plugin {name}" for name in stream.init.plugins]
    return tuple(problems)


def _session_id(stream: ClaudeStream) -> str:
    if stream.result is not None:
        return stream.result.session_id
    return stream.init.session_id if stream.init is not None else ""


def _model_reported(stream: ClaudeStream) -> str | None:
    if stream.result is not None and stream.result.model_usage:
        return ",".join(usage.model for usage in stream.result.model_usage)
    return stream.init.model if stream.init is not None else None


class ClaudeSessionHarness:
    """Strategy: Claude Code at its frozen model. Wrap it in `PacedAgentHarness` to pace, archive and resume it."""

    def __init__(
        self,
        setup: ClaudeSessionSetup,
        runner: StreamingRunner,
        environ: Mapping[str, str],
        executable: str = CLAUDE_EXECUTABLE,
    ) -> None:
        self._setup = setup
        self._runner = runner
        self._environ = environ
        self._executable = executable
        self._homes = ClaudeConfigHomes(setup.layout)

    @property
    def harness(self) -> Harness:
        return Harness.CLAUDE_CODE

    def run_session(self, request: SessionRequest) -> SessionResult:
        arm = _arm_of(request)
        token = self._environ.get(OAUTH_TOKEN_VARIABLE, "")
        if not token:
            return _not_started(MISSING_TOKEN_MESSAGE)
        policy = policy_for(arm)
        workspace = request.workdir.resolve()
        home = self._homes.home_for(request, workspace)
        argv = claude_argv(request, policy, self._write_settings(arm), self._setup.plugin_dirs)
        argv[0] = self._executable
        session_home = bench_home(self._setup.layout, request.trunk_id)
        session_home.mkdir(parents=True, exist_ok=True)
        environment = claude_environment(
            neutral_environment(self._environ, session_home, request.environment), home, token
        )
        call_directory = self._setup.layout.call(request.call_id)
        with SessionEventLog(call_directory) as log:
            try:
                streamed = self._runner.run(argv, request.prompt, workspace, environment, log.append)
            except FileNotFoundError:
                return _not_started(f"the {self._executable} executable was not found")
        scan = scan_policy_for(workspace, request.workdir, self._setup.toolchain_roots)
        assessment = assess_claude_session(log.lines, policy, self._setup, scan, _interrupted(streamed))
        return SessionResult(
            assessment.session_id,
            assessment.final_message,
            streamed.returncode,
            log.lines,
            assessment.model_reported,
            _diagnostics(streamed, log.lines, assessment),
            assessment.outcome,
            assessment.executions,
            _with_stderr(assessment, streamed),
        )

    def _write_settings(self, arm: Arm) -> Path:
        path = self._setup.layout.settings_file(arm)
        content = json.dumps(claude_settings(self._setup.toolchain_roots), indent=2, sort_keys=True) + "\n"
        if not path.is_file() or path.read_text(encoding="utf-8") != content:
            write_atomically(path, content)
        return path


def _arm_of(request: SessionRequest) -> Arm:
    if request.harness is not Harness.CLAUDE_CODE:
        raise ValueError(f"{request.harness.value} is not a Claude Code session")
    if request.kind is SessionKind.GATE_HOST:
        raise ValueError("a gate-host session is not a coder session")
    if request.arm is None or not request.trunk_id:
        raise ValueError("a coder session names its trunk and its arm")
    if request.fork and request.resume_session_id is None:
        raise ValueError("a fork names the session it forks")
    return request.arm


def _interrupted(streamed: StreamedProcess) -> bool:
    return streamed.timed_out or streamed.returncode < 0


def _diagnostics(streamed: StreamedProcess, events: Sequence[str], assessment: ClaudeAssessment) -> str:
    """What the pacing layer reads to tell a spent quota from a failure: standard error and, when the stream could not
    be read as events, what the CLI printed on standard output instead."""
    printed = events if assessment.outcome is SessionOutcome.HARNESS_MISCONFIGURED else ()
    text = "\n".join(part for part in (streamed.stderr.strip(), *printed) if part)
    return text[:DIAGNOSTICS_LENGTH]


def _with_stderr(assessment: ClaudeAssessment, streamed: StreamedProcess) -> tuple[str, ...]:
    if assessment.outcome is SessionOutcome.COMPLETED or not streamed.stderr.strip():
        return assessment.problems
    return (*assessment.problems, f"stderr: {streamed.stderr.strip()[:STDERR_EXCERPT_LENGTH]}")


def _not_started(message: str) -> SessionResult:
    return SessionResult(
        "",
        "",
        NOT_STARTED_EXIT_CODE,
        (),
        None,
        message,
        SessionOutcome.HARNESS_MISCONFIGURED,
        0,
        (message,),
    )
