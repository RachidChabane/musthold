"""The verbs of `specbench`: each wires ports to adapters and runs one use case. No rule lives here."""

from __future__ import annotations

import json
import sys
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path
from typing import Final, cast

from specbench.adapters.clock import SystemClock
from specbench.adapters.env_builder import NativeEnvBuilder, go_environment, python_executable
from specbench.adapters.file_corpus import FileCorpus, read_plain_task
from specbench.adapters.gh_miner import GhCliClient, GitHubSource, RecordingGhClient, ReplayGhClient
from specbench.adapters.git_trees import GitTreeFactory
from specbench.adapters.mining_store import write_mining
from specbench.adapters.operator_engine import UnavailableOperatorEngine
from specbench.adapters.oracle_runners import GoTestOracleRunner, PytestOracleRunner
from specbench.adapters.process import MemoryFloorRunner, SubprocessRunner, system_free_percent
from specbench.adapters.run_store import RunStore
from specbench.adapters.sealed_store import AgeSealStore, generate_identity, read_identity_file, write_identity_file
from specbench.adapters.workspace_repo import DiskTreeReader, write_workspace_repo
from specbench.analysis.estimate import AnalysisConfig
from specbench.analysis.lint import lint_report
from specbench.analysis.report import render_report
from specbench.analysis.results import analyze
from specbench.application.build_workspace import build_agent_workspace
from specbench.application.corpus import seal_task, unseal_task, verify_corpus
from specbench.application.mine import mine
from specbench.application.ports import EnvRequest, OracleRunner, ProcessRunner
from specbench.application.verify_task import TaskVerifier
from specbench.domain.canonical import JsonValue, canonical_json, canonical_results_json
from specbench.domain.languages import Language
from specbench.domain.manifest import RunLayout
from specbench.domain.mining import MiningConfig
from specbench.domain.oracle import oracle_from_bundle
from specbench.domain.pacing import EXIT_PAUSED, RunWindow
from specbench.domain.sealing import SealedBundle

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
    client = ReplayGhClient(archive) if dry_run else RecordingGhClient(GhCliClient(SubprocessRunner()), archive)
    result = mine(GitHubSource(client), config)
    write_mining(result, out)
    counts = result.manifest_record()["attrition"]
    console.say(
        f"mined {len(result.candidates)} candidates, sample of {len(result.sample)}; attrition {json.dumps(counts)}"
    )
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
    identity = read_identity_file(identity_path, find_repository_root(corpus_dir))
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
    built = NativeEnvBuilder(_runner()).build(EnvRequest(language, source, destination))
    console.say(f"built {language.value} environment {built.toolchain_version} at {destination}")
    return EXIT_OK


def _oracle_runner(language: Language, env_dir: Path | None) -> OracleRunner:
    runner = _runner()
    if language is Language.PYTHON:
        python = python_executable(env_dir) if env_dir is not None else Path(sys.executable)
        return PytestOracleRunner(python, runner)
    return GoTestOracleRunner(runner, go_environment(env_dir) if env_dir is not None else {})


def run_verify_task(
    corpus_dir: Path,
    identity_path: Path,
    task_id: str,
    clone: Path,
    env_dir: Path | None,
    out: Path | None,
    console: Console,
) -> int:
    identity = read_identity_file(identity_path, find_repository_root(corpus_dir))
    unsealed = unseal_task(FileCorpus(corpus_dir), AgeSealStore(), identity, task_id)
    verifier = TaskVerifier(
        GitTreeFactory(clone), _oracle_runner(unsealed.task.language, env_dir), UnavailableOperatorEngine()
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
    identity = read_identity_file(identity_path, find_repository_root(corpus_dir))
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
