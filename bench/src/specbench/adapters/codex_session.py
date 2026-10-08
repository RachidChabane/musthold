"""`AgentHarness` over `codex exec`: one coder session per call, on the owner's `CODEX_HOME` and never a copy of it.

- A fresh session is `codex exec ... -s workspace-write -C <workspace>`. `codex exec resume` and `codex exec fork` take
  neither `-s` nor `-C`, so every session also carries the sandbox, the network and the web search as `-c` flags and
  runs with the workspace as its working directory.
- The project's trust comes from `-c projects."<workspace>".trust_level="trusted"`. Codex persists a table for it in
  the owner's `config.toml`; it is removed after every session, even one that failed, and the file is read again.
- `--ignore-user-config` is never passed: it disables every hook.
- The prompt goes through standard input (`-`), so a message that starts with a dash is never read as an option.

Events stream to `calls/<call-id>/events.jsonl`. The rollouts of the session's thread and of the threads it spawned
give the `spawn_agent` count and the actions of the subagents.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.codex_rollouts import CodexRollouts
from specbench.adapters.codex_stream import CodexExec, RolloutFile, parse_codex_exec
from specbench.adapters.codex_trust import CodexTrustTable
from specbench.adapters.execution_counter import SESSION_EXECUTIONS, intact_stream
from specbench.adapters.session_environment import bench_home, neutral_environment
from specbench.adapters.session_events import SessionEventLog
from specbench.adapters.streaming_process import StreamedProcess, StreamingRunner
from specbench.domain.agent_events import AgentAction
from specbench.domain.arm_policy import ArmPolicy, policy_for
from specbench.domain.arms import Arm, Harness
from specbench.domain.codex_config import trust_override
from specbench.domain.contamination import DEFAULT_ALLOWED_PREFIXES, ScanPolicy, scan_actions
from specbench.domain.jsonread import RecordError
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import SessionKind, SessionOutcome, SessionRequest, SessionResult

CODEX_EXECUTABLE: Final = "codex"
CODEX_HOME_VARIABLE: Final = "CODEX_HOME"
DEFAULT_HOME_DIRECTORY: Final = ".codex"
SANDBOX_MODE: Final = "workspace-write"
STDIN_PROMPT: Final = "-"
NOT_STARTED_EXIT_CODE: Final = 78
STDERR_EXCERPT_LENGTH: Final = 300
CONFIG_WEB_SEARCH: Final = "web_search=disabled"
CONFIG_NETWORK: Final = "sandbox_workspace_write.network_access=false"
CONFIG_SANDBOX_MODE: Final = f'sandbox_mode="{SANDBOX_MODE}"'
FEATURES: Final = ("hooks", "multi_agent")
DISABLED_FEATURES: Final = ("apps",)
CONFIG_NO_APPS_INSTRUCTIONS: Final = "include_apps_instructions=false"


@dataclass(frozen=True)
class CodexSessionSetup:
    """What every Codex coder session of a run shares: the run, the owner's home and the places a coder may run in."""

    layout: RunLayout
    codex_home: Path
    toolchain_roots: tuple[Path, ...] = ()


@dataclass(frozen=True)
class CodexBaseline:
    """The spawns the thread a session continues already holds."""

    own_spawns: int = 0


@dataclass(frozen=True)
class CodexRolloutFacts:
    """What the rollouts add to the exec stream: the main thread's record and the records of the threads it spawned."""

    main: RolloutFile | None
    children: tuple[RolloutFile, ...]
    own_spawns: int

    def child_actions(self) -> tuple[AgentAction, ...]:
        return tuple(action for child in self.children for action in child.actions)

    def child_spawns(self) -> int:
        return sum(len(child.spawns) for child in self.children)


NO_ROLLOUTS: Final = CodexRolloutFacts(None, (), 0)


@dataclass(frozen=True)
class CodexAssessment:
    outcome: SessionOutcome
    problems: tuple[str, ...]
    session_id: str
    final_message: str
    model_reported: str | None
    executions: int
    errors: tuple[str, ...] = ()


def codex_home_of(environ: Mapping[str, str]) -> Path:
    """Where Codex keeps its state: `CODEX_HOME`, else `~/.codex`."""
    configured = environ.get(CODEX_HOME_VARIABLE)
    if configured:
        return Path(configured)
    return Path(environ.get("HOME", "~")).expanduser() / DEFAULT_HOME_DIRECTORY


def codex_argv(request: SessionRequest, policy: ArmPolicy, workspace: Path) -> list[str]:
    """The command line of one session; the prompt comes through standard input."""
    argv = [CODEX_EXECUTABLE, "exec"]
    session = request.resume_session_id
    if session is not None:
        argv.append("fork" if request.fork else "resume")
    argv += ["-m", request.model, "--json"]
    for feature in FEATURES:
        argv += ["--enable", feature]
    for feature in DISABLED_FEATURES:
        argv += ["--disable", feature]
    if session is None:
        argv += ["-s", SANDBOX_MODE, "-C", str(workspace)]
    if policy.loads_product_plugins:
        argv.append("--dangerously-bypass-hook-trust")
    overrides = [CONFIG_WEB_SEARCH, CONFIG_NETWORK, CONFIG_NO_APPS_INSTRUCTIONS, trust_override(str(workspace))]
    if session is not None:
        overrides.insert(0, CONFIG_SANDBOX_MODE)
    for override in overrides:
        argv += ["-c", override]
    if session is not None:
        argv.append(session)
    argv.append(STDIN_PROMPT)
    return argv


def assess_codex_session(
    events: Sequence[str],
    rollouts: CodexRolloutFacts,
    baseline: CodexBaseline,
    scan: ScanPolicy,
    interrupted: bool,
) -> CodexAssessment:
    """Judge a session from its stream and its rollouts. Precedence: a misconfigured harness, an interrupted
    session, then contamination."""
    try:
        text = intact_stream(events)
        stream = parse_codex_exec(text) if text else None
    except RecordError as error:
        return CodexAssessment(SessionOutcome.HARNESS_MISCONFIGURED, (str(error),), "", "", None, 0)
    thread_id = stream.thread_id if stream is not None else None
    problems = _configuration_problems(stream, rollouts)
    findings = _findings(stream, rollouts, scan)
    if thread_id is None and not interrupted:
        outcome = SessionOutcome.HARNESS_MISCONFIGURED
        problems = ("the stream has no thread.started event", *problems)
    elif problems and thread_id is not None:
        outcome = SessionOutcome.HARNESS_MISCONFIGURED
    elif interrupted:
        outcome = SessionOutcome.INTERRUPTED
    elif findings:
        outcome = SessionOutcome.CONTAMINATED
    else:
        outcome = SessionOutcome.COMPLETED
    return CodexAssessment(
        outcome,
        (*problems, *findings),
        thread_id or "",
        stream.final_message if stream is not None else "",
        _model_reported(rollouts),
        _executions(stream, rollouts, baseline),
        stream.errors if stream is not None else (),
    )


def _configuration_problems(stream: CodexExec | None, rollouts: CodexRolloutFacts) -> tuple[str, ...]:
    if stream is None or rollouts.main is None or not rollouts.main.turns:
        return ()
    sandbox = rollouts.main.turns[-1].sandbox
    if sandbox is None:
        return ()
    problems: list[str] = []
    if sandbox.kind != SANDBOX_MODE:
        problems.append(f"the sandbox is {sandbox.kind}, not {SANDBOX_MODE}")
    if sandbox.network_access is not False:
        problems.append("the sandbox does not deny the network")
    return tuple(problems)


def _findings(stream: CodexExec | None, rollouts: CodexRolloutFacts, scan: ScanPolicy) -> tuple[str, ...]:
    actions = (*(stream.actions if stream is not None else ()), *rollouts.child_actions())
    return tuple(
        f"{finding.kind.value} at event {finding.pointer.line}: {finding.evidence}"
        for finding in scan_actions(actions, scan)
    )


def _model_reported(rollouts: CodexRolloutFacts) -> str | None:
    if rollouts.main is None or not rollouts.main.turns:
        return None
    return rollouts.main.turns[-1].model


def _executions(stream: CodexExec | None, rollouts: CodexRolloutFacts, baseline: CodexBaseline) -> int:
    """The session itself, the `spawn_agent` calls it made, and the calls its subagents made. The rollout is the
    authority; the stream's own spawn items are a floor for a rollout that could not be read."""
    if stream is None or stream.thread_id is None:
        return 0
    gained = max(rollouts.own_spawns - baseline.own_spawns, 0)
    return SESSION_EXECUTIONS + max(gained, len(stream.spawns)) + rollouts.child_spawns()


class CodexSessionHarness:
    """Strategy: Codex at its frozen model. Wrap it in `PacedAgentHarness` to pace, archive and resume it."""

    def __init__(
        self,
        setup: CodexSessionSetup,
        runner: StreamingRunner,
        environ: Mapping[str, str],
        executable: str = CODEX_EXECUTABLE,
    ) -> None:
        self._setup = setup
        self._runner = runner
        self._environ = environ
        self._executable = executable
        self._codex_home = setup.codex_home.resolve()
        self._rollouts = CodexRollouts(self._codex_home)
        self._trust = CodexTrustTable(self._codex_home)

    @property
    def harness(self) -> Harness:
        return Harness.CODEX

    def run_session(self, request: SessionRequest) -> SessionResult:
        policy = policy_for(_arm_of(request))
        workspace = request.workdir.resolve()
        argv = codex_argv(request, policy, workspace)
        argv[0] = self._executable
        baseline = CodexBaseline(
            self._rollouts.own_spawns(request.resume_session_id) if request.resume_session_id is not None else 0
        )
        with SessionEventLog(self._setup.layout.call(request.call_id)) as log:
            try:
                streamed = self._run(argv, request, workspace, log)
            except FileNotFoundError:
                return _not_started(f"the {self._executable} executable was not found")
        facts = self._rollout_facts(log.lines)
        scan = ScanPolicy(str(workspace), self._allowed_prefixes(request.workdir))
        assessment = assess_codex_session(log.lines, facts, baseline, scan, _interrupted(streamed))
        return SessionResult(
            assessment.session_id,
            assessment.final_message,
            streamed.returncode,
            log.lines,
            assessment.model_reported,
            _diagnostics(streamed, assessment.errors),
            assessment.outcome,
            assessment.executions,
            _with_stderr(assessment, streamed),
        )

    def _run(
        self, argv: Sequence[str], request: SessionRequest, workspace: Path, log: SessionEventLog
    ) -> StreamedProcess:
        session_home = bench_home(self._setup.layout, request.trunk_id)
        session_home.mkdir(parents=True, exist_ok=True)
        environment = {
            **neutral_environment(self._environ, session_home, request.environment),
            CODEX_HOME_VARIABLE: str(self._codex_home),
        }
        try:
            return self._runner.run(argv, request.prompt, workspace, environment, log.append)
        finally:
            self._trust.restore(workspace)

    def _rollout_facts(self, events: Sequence[str]) -> CodexRolloutFacts:
        text = intact_stream(events)
        try:
            stream = parse_codex_exec(text) if text else None
        except RecordError:
            return NO_ROLLOUTS
        if stream is None or stream.thread_id is None:
            return NO_ROLLOUTS
        main = self._rollouts.read(stream.thread_id)
        children = tuple(
            rollout
            for spawn in stream.spawns
            for thread in spawn.receiver_thread_ids
            if (rollout := self._rollouts.read(thread)) is not None
        )
        return CodexRolloutFacts(main, children, len(main.spawns) if main is not None else 0)

    def _allowed_prefixes(self, unresolved: Path) -> tuple[str, ...]:
        return (*DEFAULT_ALLOWED_PREFIXES, *(str(root) for root in self._setup.toolchain_roots), str(unresolved))


def _arm_of(request: SessionRequest) -> Arm:
    if request.harness is not Harness.CODEX:
        raise ValueError(f"{request.harness.value} is not a Codex session")
    if request.kind is SessionKind.GATE_HOST:
        raise ValueError("a gate-host session is not a coder session")
    if request.arm is None or not request.trunk_id:
        raise ValueError("a coder session names its trunk and its arm")
    if request.fork and request.resume_session_id is None:
        raise ValueError("a fork names the session it forks")
    return request.arm


def _interrupted(streamed: StreamedProcess) -> bool:
    return streamed.timed_out or streamed.returncode < 0


def _diagnostics(streamed: StreamedProcess, errors: Sequence[str]) -> str:
    """What the pacing layer reads to tell a spent quota from a failure: standard error and the stream's own errors."""
    return "\n".join(part for part in (streamed.stderr.strip(), *errors) if part)


def _with_stderr(assessment: CodexAssessment, streamed: StreamedProcess) -> tuple[str, ...]:
    if assessment.outcome is SessionOutcome.COMPLETED or not streamed.stderr.strip():
        return assessment.problems
    return (*assessment.problems, f"stderr: {streamed.stderr.strip()[:STDERR_EXCERPT_LENGTH]}")


def _not_started(message: str) -> SessionResult:
    return SessionResult(
        "", "", NOT_STARTED_EXIT_CODE, (), None, message, SessionOutcome.HARNESS_MISCONFIGURED, 0, (message,)
    )
