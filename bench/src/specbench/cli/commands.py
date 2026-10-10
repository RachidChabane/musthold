"""The verbs of `specbench`: each wires ports to adapters and runs one use case. No rule lives here."""

from __future__ import annotations

import json
import subprocess
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path
from typing import Final, cast

from specbench.adapters.claude_cli import ClaudeCliModel
from specbench.adapters.clock import SystemClock
from specbench.adapters.construction_sources import MirrorConstructionSources
from specbench.adapters.construction_store import FileStatusStore
from specbench.adapters.env_builder import (
    PYTHON_ORACLE_TOOLING,
    EnvironmentBuildError,
    MisePythons,
    NativeEnvBuilder,
    go_offline_environment,
    python_executable,
)
from specbench.adapters.file_corpus import FileCorpus, read_plain_task
from specbench.adapters.gh_miner import (
    GhCliClient,
    GitHubSource,
    RecordingGhClient,
    ReplayGhClient,
    ThrottledGhClient,
    compact_archive,
)
from specbench.adapters.git_history import GitError, GitHistory, GitMirrors
from specbench.adapters.git_trees import GitTreeFactory, TreeBuildError
from specbench.adapters.ground_stability import StableGround
from specbench.adapters.mining_store import ATTRITION_FILE, CANDIDATES_FILE, MINING_FILE, write_mining
from specbench.adapters.network_sandbox import NetworkDeniedRunner
from specbench.adapters.offline_probe import OfflineProbe, environment_directory, suite_runner_for
from specbench.adapters.operator_engine import UnavailableOperatorEngine
from specbench.adapters.oracle_runners import GoTestOracleRunner, PytestOracleRunner
from specbench.adapters.patch_builder import GitDiffPatchBuilder
from specbench.adapters.pinned_claude_code import PinnedClaudeCode
from specbench.adapters.process import (
    MemoryFloorRunner,
    SubprocessPromptRunner,
    SubprocessRunner,
    system_free_percent,
)
from specbench.adapters.prompt_files import construction_schema, load_prompts
from specbench.adapters.run_journal import FileRunJournal, TranscriptRecorder
from specbench.adapters.run_store import FileCallStore, FileFailureLog, FileLedgerStore, RunStore
from specbench.adapters.schemas import JsonSchemaChecker, SchemaName
from specbench.adapters.sealed_store import (
    AgeSealStore,
    generate_identity,
    read_identity_file,
    refuse_inside_repository,
    write_identity_file,
)
from specbench.adapters.selection_store import read_attrition, read_candidates, read_selection, write_selection
from specbench.adapters.workspace_repo import DiskTreeReader, write_workspace_repo
from specbench.analysis.estimate import AnalysisConfig
from specbench.analysis.lint import lint_report
from specbench.analysis.report import render_report
from specbench.analysis.results import analyze
from specbench.application.build_workspace import build_agent_workspace
from specbench.application.construct import TaskConstruction
from specbench.application.construct_run import ConstructionRun, TaskSettlement
from specbench.application.construct_run import Outcome as TaskOutcome
from specbench.application.corpus import seal_task, unseal_task, verify_corpus
from specbench.application.mine import mine
from specbench.application.paced_calls import CallGate, PacedConstructionModel
from specbench.application.ports import EnvRequest, OracleRunner, ProcessRunner
from specbench.application.reverify import reverify
from specbench.application.select_tasks import select_feasible
from specbench.application.verify_task import TaskVerifier
from specbench.domain.arms import Harness
from specbench.domain.candidates import ranking_by_language
from specbench.domain.canonical import JsonValue, canonical_json, canonical_results_json
from specbench.domain.languages import Language
from specbench.domain.manifest import RunLayout
from specbench.domain.mining import MiningConfig
from specbench.domain.oracle import oracle_from_bundle
from specbench.domain.pacing import EXIT_PAUSED, PacingPolicy, RunWindow
from specbench.domain.sealing import SealedBundle
from specbench.domain.selection import Selection
from specbench.domain.task import TASK_FILE_NAME, Task

EXIT_OK: Final = 0
EXIT_FAILED: Final = 1
EXIT_UNSAFE: Final = 4
ENCODING: Final = "utf-8"
GITDIR_MARKER: Final = ".git"


@dataclass(frozen=True)
class Console:
    """Where a verb prints; tests capture it."""

    out: list[str]

    def say(self, line: str) -> None:
        self.out.append(line)
        print(line)


def find_repository_root(start: Path) -> Path:
    """The working tree an agent can reach: the nearest ancestor holding a git marker, else `start`."""
    for candidate in (start.resolve(), *start.resolve().parents):
        if (candidate / GITDIR_MARKER).exists():
            return candidate
    return start.resolve()


def _runner() -> ProcessRunner:
    return MemoryFloorRunner(SubprocessRunner(), system_free_percent)


def run_mine(
    out: Path, archive: Path, seed: int, freeze_date: date | None, per_language: int, dry_run: bool, console: Console
) -> int:
    config = MiningConfig(seed, freeze_date, tuple(Language), per_language)
    client = (
        ReplayGhClient(archive)
        if dry_run
        else RecordingGhClient(ThrottledGhClient(GhCliClient(SubprocessRunner())), archive)
    )
    result = mine(GitHubSource(client), config)
    write_mining(result, out)
    counts = result.manifest_record()["attrition"]
    console.say(
        f"mined {len(result.candidates)} candidates, sample of {len(result.sample.tasks)} tasks and "
        f"{len(result.sample.reserves)} reserves; attrition {json.dumps(counts)}"
    )
    return EXIT_OK


SUITE_TIMEOUT_SECONDS: Final = 900
MIRROR_TIMEOUT_SECONDS: Final = 3600


def _mining_seed(mining_dir: Path) -> int:
    """The seed recorded by `mine`, which also seeds the spec-gap draw of construction."""
    mining = cast(dict[str, JsonValue], json.loads((mining_dir / MINING_FILE).read_text(encoding=ENCODING)))
    return int(str(mining["seed"]))


def run_select(
    mining_dir: Path,
    mirrors_dir: Path,
    work_dir: Path,
    out: Path,
    tasks_per_language: int,
    reserves_per_language: int,
    console: Console,
) -> int:
    """Probe the mined candidates in seeded order and keep the first feasible tasks and reserves."""
    refuse_inside_repository(work_dir, find_repository_root(mining_dir), "work directory")
    seed = _mining_seed(mining_dir)
    candidates = read_candidates(mining_dir / CANDIDATES_FILE)
    denied = NetworkDeniedRunner(MemoryFloorRunner(SubprocessRunner(SUITE_TIMEOUT_SECONDS), system_free_percent))
    denied.assert_denied()
    plain = _runner()
    mirrors = GitMirrors(mirrors_dir, MemoryFloorRunner(SubprocessRunner(MIRROR_TIMEOUT_SECONDS), system_free_percent))
    probe = OfflineProbe(
        mirrors,
        lambda clone: GitHistory(clone, plain),
        NativeEnvBuilder(plain),
        lambda language, env_dir: suite_runner_for(language, env_dir, denied),
        work_dir,
        MisePythons(plain).interpreter,
    )
    selection = select_feasible(ranking_by_language(seed, candidates), probe, tasks_per_language, reserves_per_language)
    write_selection(
        out,
        seed,
        selection,
        read_attrition(mining_dir / ATTRITION_FILE),
        lambda task, name: GitHistory(mirrors.path_of(task.candidate.repository), plain).file_at(task.base_sha, name),
    )
    console.say(f"selected {len(selection.tasks)} tasks and {len(selection.reserves)} reserves into {out}")
    return EXIT_OK


VERIFICATION_ABORTS: Final = (subprocess.TimeoutExpired, TreeBuildError, EnvironmentBuildError, GitError)
CONSTRUCTION_DIRECTORY: Final = "construction"
GH_ARCHIVE_DIRECTORY: Final = "gh"
SCHEMA_KEYS: Final = {
    "spec": SchemaName.SPEC,
    "review": SchemaName.REVIEW,
    "h-tests": SchemaName.H_TESTS,
    "violations": SchemaName.VIOLATIONS,
    "refactors": SchemaName.REFACTORS,
    "assignment": SchemaName.ASSIGNMENT,
    "probe": SchemaName.PROBE,
}


def run_construct(
    selection_dir: Path,
    mining_dir: Path,
    corpus_dir: Path,
    mirrors_dir: Path,
    work_dir: Path,
    tasks_per_language: int,
    window_text: str,
    budget: int,
    console: Console,
) -> int:
    """Build, verify and seal the selected tasks; a pause or an unsafe machine ends the run with its own exit code.

    Every call runs the pinned Claude Code CLI; the run does not start without it."""
    root = find_repository_root(selection_dir)
    refuse_inside_repository(work_dir, root, "work directory")
    claude = PinnedClaudeCode(root).executable()
    layout = RunLayout(work_dir / CONSTRUCTION_DIRECTORY)
    policy = PacingPolicy(RunWindow.parse(window_text), budget)
    gate = CallGate(Harness.CLAUDE_CODE, policy, FileLedgerStore(layout), SystemClock())
    model = PacedConstructionModel(
        ClaudeCliModel(SubprocessPromptRunner(system_free_percent), claude),
        gate,
        FileCallStore(layout),
        FileFailureLog(layout),
    )
    plain = _runner()
    denied = NetworkDeniedRunner(MemoryFloorRunner(SubprocessRunner(SUITE_TIMEOUT_SECONDS), system_free_percent))
    denied.assert_denied()
    mirrors = GitMirrors(mirrors_dir, MemoryFloorRunner(SubprocessRunner(MIRROR_TIMEOUT_SECONDS), system_free_percent))
    gh = RecordingGhClient(ThrottledGhClient(GhCliClient(SubprocessRunner())), layout.root / GH_ARCHIVE_DIRECTORY)
    stable = StableGround(
        work_dir,
        lambda task: GitTreeFactory(mirrors.path_of(task.candidate.repository)),
        lambda task: suite_runner_for(task.candidate.language, environment_directory(work_dir, task.task_id), denied),
    )
    sources = MirrorConstructionSources(mirrors, lambda clone: GitHistory(clone, plain), gh, work_dir, stable.stabilise)
    construction = TaskConstruction(
        model,
        load_prompts(),
        {key: construction_schema(name) for key, name in SCHEMA_KEYS.items()},
        JsonSchemaChecker(),
        GitDiffPatchBuilder(plain),
        _mining_seed(mining_dir),
    )
    corpus = FileCorpus(corpus_dir)
    corpus.read_recipients()
    statuses = FileStatusStore(layout.root)
    settlement = _settlement(work_dir, mirrors, denied, corpus, statuses)
    run = ConstructionRun(sources, construction, settlement, statuses, gate.require_open)
    selection = read_selection(selection_dir, read_candidates(mining_dir / CANDIDATES_FILE), work_dir)
    summary = run.run(selection, tasks_per_language)
    _copy_notices(selection_dir, corpus, summary.sealed, selection)
    console.say(f"sealed {len(summary.sealed)}: {', '.join(summary.sealed)}")
    for task_id, reason in summary.dropped:
        console.say(f"dropped {task_id}: {reason}")
    for line in summary.unfinished:
        console.say(f"unfinished: {line}")
    return EXIT_OK if summary.complete else EXIT_FAILED


def _settlement(
    work_dir: Path, mirrors: GitMirrors, denied: ProcessRunner, corpus: FileCorpus, statuses: FileStatusStore
) -> TaskSettlement:
    """Verification on network-denied runs, journaled beside each task's report, then sealing or a drop."""

    def verifier_for(task: Task) -> TaskVerifier:
        transcripts = TranscriptRecorder(denied)
        return TaskVerifier(
            GitTreeFactory(mirrors.path_of(task.repository)),
            _oracle_runner(task.language, environment_directory(work_dir, task.task_id), None, transcripts),
            UnavailableOperatorEngine(),
            journal=FileRunJournal(statuses.verification_runs_path(task.task_id), transcripts),
        )

    return TaskSettlement(verifier_for, corpus, AgeSealStore(), statuses, VERIFICATION_ABORTS)


def _copy_notices(selection_dir: Path, corpus: FileCorpus, sealed: tuple[str, ...], selection: Selection) -> None:
    by_id = {t.task_id: t for t in (*selection.tasks, *selection.reserves)}
    for task_id in sealed:
        _copy_task_notices(selection_dir, corpus, task_id, by_id[task_id].candidate.language)


def _copy_task_notices(selection_dir: Path, corpus: FileCorpus, task_id: str, language: Language) -> None:
    """Keep a sealed task's upstream licence and notice beside its `task.json` and `sealed.age`."""
    for notice in sorted((selection_dir / "tasks" / task_id).iterdir()):
        if notice.name != TASK_FILE_NAME:
            corpus.write_notice(language, task_id, notice.name, notice.read_text(encoding=ENCODING))


def run_reverify(
    selection_dir: Path, corpus_dir: Path, mirrors_dir: Path, work_dir: Path, task_ids: list[str], console: Console
) -> int:
    """Verify again, without any model call, tasks construction dropped at verification, and settle each anew."""
    refuse_inside_repository(work_dir, find_repository_root(selection_dir), "work directory")
    denied = NetworkDeniedRunner(MemoryFloorRunner(SubprocessRunner(SUITE_TIMEOUT_SECONDS), system_free_percent))
    denied.assert_denied()
    mirrors = GitMirrors(mirrors_dir, MemoryFloorRunner(SubprocessRunner(MIRROR_TIMEOUT_SECONDS), system_free_percent))
    corpus = FileCorpus(corpus_dir)
    corpus.read_recipients()
    statuses = FileStatusStore(work_dir / CONSTRUCTION_DIRECTORY)
    settlement = _settlement(work_dir, mirrors, denied, corpus, statuses)
    all_sealed = True
    for task_id in task_ids:
        settled = reverify(task_id, statuses, settlement)
        status = settled.status
        if status.outcome is TaskOutcome.SEALED:
            _copy_task_notices(selection_dir, corpus, task_id, settled.task.language)
            console.say(f"sealed {task_id}: bundle {status.bundle_sha256}")
        else:
            all_sealed = False
            console.say(f"dropped {task_id}: {status.stage}: {status.reason}")
    return EXIT_OK if all_sealed else EXIT_FAILED


def run_compact(archive: Path, console: Console) -> int:
    console.say(f"compacted {compact_archive(archive)} archived responses in {archive}")
    return EXIT_OK


def run_keygen(identity_path: Path, recipients_file: Path, console: Console) -> int:
    identity, recipient = generate_identity()
    write_identity_file(identity_path, identity, find_repository_root(recipients_file.parent))
    recipients_file.parent.mkdir(parents=True, exist_ok=True)
    existing = recipients_file.read_text(encoding=ENCODING) if recipients_file.is_file() else ""
    recipients_file.write_text(existing + recipient + "\n", encoding=ENCODING)
    console.say(f"recipient {recipient}")
    return EXIT_OK


def run_seal(corpus_dir: Path, plain_dir: Path, console: Console) -> int:
    task_json, files = read_plain_task(plain_dir)
    entry = seal_task(FileCorpus(corpus_dir), AgeSealStore(), task_json, SealedBundle(files))
    console.say(f"sealed {entry.task_id}: bundle {entry.bundle_sha256}")
    return EXIT_OK


def run_unseal(corpus_dir: Path, identity_path: Path, task_id: str, out: Path, console: Console) -> int:
    root = find_repository_root(corpus_dir)
    refuse_inside_repository(out, root, "unseal destination")
    identity = read_identity_file(identity_path, root)
    unsealed = unseal_task(FileCorpus(corpus_dir), AgeSealStore(), identity, task_id)
    for relative, content in unsealed.bundle.files.items():
        target = out / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
    console.say(f"unsealed {task_id} into {out}")
    return EXIT_OK


def run_verify_corpus(corpus_dir: Path, identity_path: Path | None, console: Console) -> int:
    identity = None if identity_path is None else read_identity_file(identity_path, find_repository_root(corpus_dir))
    problems = verify_corpus(FileCorpus(corpus_dir), AgeSealStore(), identity)
    for problem in problems:
        console.say(problem)
    console.say("corpus verified" if not problems else f"{len(problems)} problems")
    return EXIT_OK if not problems else EXIT_FAILED


def run_build_env(language: Language, source: Path, destination: Path, console: Console) -> int:
    tooling = PYTHON_ORACLE_TOOLING if language is Language.PYTHON else ()
    built = NativeEnvBuilder(_runner()).build(EnvRequest(language, source, destination, tooling))
    console.say(f"built {language.value} environment {built.toolchain_version} at {destination}")
    return EXIT_OK


def _oracle_runner(
    language: Language, env_dir: Path, python: Path | None, runner: ProcessRunner | None = None
) -> OracleRunner:
    runner = runner or _runner()
    if language is Language.PYTHON:
        return PytestOracleRunner(python or python_executable(env_dir), runner)
    return GoTestOracleRunner(runner, go_offline_environment(env_dir))


def run_verify_task(
    corpus_dir: Path,
    identity_path: Path,
    task_id: str,
    clone: Path,
    env_dir: Path,
    python: Path | None,
    out: Path | None,
    console: Console,
) -> int:
    identity = read_identity_file(identity_path, find_repository_root(corpus_dir))
    unsealed = unseal_task(FileCorpus(corpus_dir), AgeSealStore(), identity, task_id)
    verifier = TaskVerifier(
        GitTreeFactory(clone), _oracle_runner(unsealed.task.language, env_dir, python), UnavailableOperatorEngine()
    )
    report = verifier.verify(unsealed.task, oracle_from_bundle(unsealed.bundle), unsealed.bundle)
    text = canonical_json(cast(JsonValue, report.to_record()))
    if out is not None:
        out.write_text(text, encoding=ENCODING)
    console.say(f"{task_id}: {'verified' if report.verified else 'dropped'}")
    return EXIT_OK if report.verified else EXIT_FAILED


def run_build_workspace(
    corpus_dir: Path, identity_path: Path, task_id: str, clone: Path, init_dir: Path | None, out: Path, console: Console
) -> int:
    root = find_repository_root(corpus_dir)
    refuse_inside_repository(out, root, "workspace destination")
    identity = read_identity_file(identity_path, root)
    unsealed = unseal_task(FileCorpus(corpus_dir), AgeSealStore(), identity, task_id)
    reader = DiskTreeReader()
    init_files = reader.read(init_dir) if init_dir is not None else {}
    workspace = build_agent_workspace(GitTreeFactory(clone), reader, unsealed.task, unsealed.bundle, init_files)
    commit = write_workspace_repo(workspace, out)
    console.say(f"workspace {task_id} at {out}: commit {commit}, {len(workspace.omitted)} base files omitted")
    return EXIT_OK


def run_analyze(run_dir: Path, resamples: int, permutations: int, console: Console) -> int:
    store = RunStore(RunLayout(run_dir))
    results = analyze(store.read_trunks(), AnalysisConfig(resamples=resamples), permutations)
    results_json = canonical_results_json(results)
    report = render_report(cast(dict[str, JsonValue], json.loads(results_json)))
    store.write_results(results_json, report)
    console.say(f"analysed {len(store.read_trunks())} trunks into {run_dir}")
    return EXIT_OK


def run_report_lint(report: Path, console: Console) -> int:
    findings = lint_report(report.read_text(encoding=ENCODING))
    for finding in findings:
        console.say(str(finding))
    return EXIT_OK if not findings else EXIT_FAILED


def run_window_status(window_text: str, now: datetime | None, console: Console) -> int:
    window = RunWindow.parse(window_text)
    moment = now or SystemClock().now()
    if window is None or window.is_open(moment):
        console.say("run window open")
        return EXIT_OK
    console.say("run window closed")
    return EXIT_PAUSED
