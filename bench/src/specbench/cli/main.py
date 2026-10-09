"""Command line of `specbench`: argument parsing and error-to-exit-code mapping."""

from __future__ import annotations

import argparse
import os
import sys
from collections.abc import Callable, Sequence
from datetime import date, datetime
from pathlib import Path
from typing import Final

from specbench.adapters.env_builder import EnvironmentBuildError
from specbench.adapters.gh_miner import GhError
from specbench.adapters.git_history import GitError
from specbench.adapters.git_trees import TreeBuildError
from specbench.adapters.network_sandbox import NetworkDenialUnavailable
from specbench.adapters.patch_builder import PatchBuildError
from specbench.adapters.process import MachineUnsafe
from specbench.adapters.sealed_store import InsideRepositoryError
from specbench.adapters.site_output import NotASiteDirectory
from specbench.analysis.bootstrap import BOOTSTRAP_RESAMPLES
from specbench.analysis.selection_null import DEFAULT_PERMUTATIONS
from specbench.application.paced_calls import ModelNotFrozen
from specbench.application.ports import PinnedClaudeUnavailable
from specbench.cli import commands
from specbench.cli.commands import Console
from specbench.cli.night_commands import NightBudgetRequest, run_night_budget
from specbench.cli.site_command import SiteRequest, run_site
from specbench.domain.arms import Harness
from specbench.domain.canonical import CanonicalError
from specbench.domain.construction import ModelCallFailed
from specbench.domain.jsonread import RecordError
from specbench.domain.languages import Language
from specbench.domain.mining import RESERVE_TASKS_PER_LANGUAGE, SAMPLE_TASKS_PER_LANGUAGE
from specbench.domain.night_budget import DEFAULT_CONSTRUCT_ALLOWANCE, DEFAULT_NIGHTLY_CAP, NightJob
from specbench.domain.pacing import (
    ALWAYS_OPEN,
    DEFAULT_CALL_BUDGET,
    DEFAULT_RUN_WINDOW,
    EXIT_PAUSED,
    RUN_WINDOW_ENV,
    InvalidRunWindow,
    RunPaused,
)
from specbench.domain.sealing import SealingError
from specbench.domain.verification import VERIFICATION_RUNS
from specbench.domain.workspace import WorkspaceLeak

DEFAULT_SEED: Final = 20261007
DEFAULT_PER_LANGUAGE: Final = 4 * SAMPLE_TASKS_PER_LANGUAGE
USAGE_ERROR: Final = 2
EXPECTED_FAILURES: Final = (
    CanonicalError,
    EnvironmentBuildError,
    FileNotFoundError,
    GhError,
    GitError,
    ModelCallFailed,
    PatchBuildError,
    InsideRepositoryError,
    InvalidRunWindow,
    NotASiteDirectory,
    NetworkDenialUnavailable,
    ModelNotFrozen,
    PinnedClaudeUnavailable,
    RecordError,
    SealingError,
    TreeBuildError,
    WorkspaceLeak,
)
AddVerb = Callable[..., argparse.ArgumentParser]
Handler = Callable[[argparse.Namespace, Console], int]


def _path(value: str) -> Path:
    return Path(value)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="specbench", description="Deterministic core of the spec-proof benchmark.")
    verbs = parser.add_subparsers(dest="verb", required=True)
    _add_mining_verbs(verbs.add_parser)
    _add_corpus_verbs(verbs.add_parser)
    _add_task_verbs(verbs.add_parser)
    _add_run_verbs(verbs.add_parser)
    _add_night_verbs(verbs.add_parser)
    return parser


def _add_mining_verbs(add: AddVerb) -> None:

    mine = add("mine", help="mine candidate tasks through gh; --dry-run replays the archive")
    mine.add_argument("--out", type=_path, required=True)
    mine.add_argument("--archive", type=_path, required=True, help="directory holding every archived gh response")
    mine.add_argument("--seed", type=int, default=DEFAULT_SEED)
    mine.add_argument("--freeze-date", type=date.fromisoformat, default=None)
    mine.add_argument("--per-language", type=int, default=DEFAULT_PER_LANGUAGE)
    mine.add_argument("--dry-run", action="store_true", help="serve archived responses; no network")
    mine.add_argument("--compact", action="store_true", help="rewrite the archive in its projected form and stop")

    select = add("select", help="probe mined candidates offline in seeded order and keep the feasible tasks")
    select.add_argument("--mining", type=_path, required=True, help="the directory `mine` wrote")
    select.add_argument("--mirrors", type=_path, required=True, help="bare local clones of upstream repositories")
    select.add_argument("--work", type=_path, required=True, help="environments and verdicts, outside the repository")
    select.add_argument("--out", type=_path, required=True)
    select.add_argument("--tasks-per-language", type=int, default=SAMPLE_TASKS_PER_LANGUAGE)
    select.add_argument("--reserves-per-language", type=int, default=RESERVE_TASKS_PER_LANGUAGE)

    construct = add("construct", help="build, verify and seal the selected tasks, paced and resumable")
    construct.add_argument("--selection", type=_path, required=True, help="the directory `select` wrote")
    construct.add_argument("--mining", type=_path, required=True, help="the directory `mine` wrote")
    construct.add_argument("--corpus", type=_path, required=True)
    construct.add_argument("--mirrors", type=_path, required=True)
    construct.add_argument("--work", type=_path, required=True, help="outside the repository")
    construct.add_argument("--tasks-per-language", type=int, default=SAMPLE_TASKS_PER_LANGUAGE)
    construct.add_argument(
        "--night",
        action="store_true",
        help=f"only inside the run window ({RUN_WINDOW_ENV}, default {DEFAULT_RUN_WINDOW})",
    )
    construct.add_argument("--budget", type=int, default=DEFAULT_CALL_BUDGET, help="new model calls per night")

    keygen = add("keygen", help="create an age identity outside the repository and add its recipient")
    keygen.add_argument("--identity", type=_path, required=True)
    keygen.add_argument("--recipients", type=_path, required=True)


def _add_corpus_verbs(add: AddVerb) -> None:
    seal = add("seal", help="seal a plain task directory into the corpus and update the lock")
    seal.add_argument("--corpus", type=_path, required=True)
    seal.add_argument("--plain", type=_path, required=True)

    unseal = add("unseal", help="decrypt a task into a directory outside the repository")
    unseal.add_argument("--corpus", type=_path, required=True)
    unseal.add_argument("--identity", type=_path, required=True)
    unseal.add_argument("--task-id", required=True)
    unseal.add_argument("--out", type=_path, required=True)

    verify_corpus = add("verify-corpus", help="R0 corpus check against corpus.lock.json")
    verify_corpus.add_argument("--corpus", type=_path, required=True)
    verify_corpus.add_argument("--identity", type=_path, default=None)


def _add_task_verbs(add: AddVerb) -> None:
    build_env = add("build-env", help="build a native environment from a tree's lockfiles")
    build_env.add_argument("--language", choices=[language.value for language in Language], required=True)
    build_env.add_argument("--source", type=_path, required=True)
    build_env.add_argument("--dest", type=_path, required=True)

    verify_task = add("verify-task", help=f"two-way verification, {VERIFICATION_RUNS} runs per step, no model call")
    verify_task.add_argument("--corpus", type=_path, required=True)
    verify_task.add_argument("--identity", type=_path, required=True)
    verify_task.add_argument("--task-id", required=True)
    verify_task.add_argument(
        "--clone", type=_path, required=True, help="local clone holding the base and merge commits"
    )
    verify_task.add_argument("--env-dir", type=_path, required=True, help="the task environment built by build-env")
    verify_task.add_argument("--python", type=_path, default=None, help="interpreter override for Python tasks")
    verify_task.add_argument("--out", type=_path, default=None)

    workspace = add("build-workspace", help="a fresh single-commit workspace: base tree, SPEC.md, init output")
    workspace.add_argument("--corpus", type=_path, required=True)
    workspace.add_argument("--identity", type=_path, required=True)
    workspace.add_argument("--task-id", required=True)
    workspace.add_argument("--clone", type=_path, required=True)
    workspace.add_argument("--init-dir", type=_path, default=None, help="files the product's init wrote")
    workspace.add_argument("--out", type=_path, required=True)


def _add_run_verbs(add: AddVerb) -> None:
    analyze = add("analyze", help="results.json and REPORT.md from the trunk records of a run directory")
    analyze.add_argument("--run", type=_path, required=True)
    analyze.add_argument("--resamples", type=int, default=BOOTSTRAP_RESAMPLES)
    analyze.add_argument("--permutations", type=int, default=DEFAULT_PERMUTATIONS)

    site = add("site", help="build and check the results site of a sealed run, or of the synthetic fixtures")
    source = site.add_mutually_exclusive_group(required=True)
    source.add_argument("--run", type=_path, help="a sealed run directory")
    source.add_argument("--fixtures", action="store_true", help="the synthetic fixtures, badged on every page")
    site.add_argument("--root", type=_path, required=True, help="the repository root")
    site.add_argument("--out", type=_path, required=True)
    site.add_argument("--commit", required=True, help="the commit of the repository the site is built at")
    site.add_argument("--check", action="store_true", help="build and check without writing")

    lint = add("report-lint", help="lint a rendered report")
    lint.add_argument("--report", type=_path, required=True)

    window = add("window-status", help=f"exit 0 inside the run window, {EXIT_PAUSED} outside it")
    window.add_argument("--window", default=os.environ.get(RUN_WINDOW_ENV, DEFAULT_RUN_WINDOW))
    window.add_argument("--now", type=datetime.fromisoformat, default=None)


def _add_night_verbs(add: AddVerb) -> None:
    budget = add("night-budget", help="the executions a night job may spend tonight, computed under budget.lock")
    budget.add_argument("job", choices=[job.value for job in NightJob])
    budget.add_argument("--harness", choices=[harness.value for harness in Harness], default=Harness.CLAUDE_CODE.value)
    budget.add_argument("--night", type=date.fromisoformat, default=None, help="the night's date (default: now)")
    budget.add_argument("--claude-cap", type=int, default=DEFAULT_NIGHTLY_CAP)
    budget.add_argument("--codex-cap", type=int, default=DEFAULT_NIGHTLY_CAP)
    budget.add_argument("--construct-allowance", type=int, default=DEFAULT_CONSTRUCT_ALLOWANCE)
    budget.add_argument("--canary-reserve", type=int, default=None, help="executions the canaries use tonight")
    budget.add_argument("--record-canary", type=int, default=None, help="record the canaries' measured executions")
    budget.add_argument("--pilot-ledger", type=_path, required=True)
    budget.add_argument("--construct-ledger", type=_path, required=True)
    budget.add_argument("--sample-ledger", type=_path, required=True)
    budget.add_argument("--codex-ledger", type=_path, required=True)
    budget.add_argument("--selection", type=_path, required=True)
    budget.add_argument("--corpus", type=_path, required=True)
    budget.add_argument("--state-dir", type=_path, default=None)
    budget.add_argument("--explain", action="store_true", help="print every input and every share as JSON")


def _night_budget(a: argparse.Namespace, console: Console) -> int:
    request = NightBudgetRequest(
        NightJob(a.job),
        Harness(a.harness),
        a.night,
        {Harness.CLAUDE_CODE: a.claude_cap, Harness.CODEX: a.codex_cap},
        a.construct_allowance,
        a.canary_reserve,
        a.record_canary,
        a.pilot_ledger,
        a.construct_ledger,
        a.sample_ledger,
        a.codex_ledger,
        a.selection,
        a.corpus,
        a.state_dir,
        a.explain,
    )
    return run_night_budget(request, console)


def _handlers() -> dict[str, Handler]:
    return {
        "mine": lambda a, c: (
            commands.run_compact(a.archive, c)
            if a.compact
            else commands.run_mine(a.out, a.archive, a.seed, a.freeze_date, a.per_language, a.dry_run, c)
        ),
        "select": lambda a, c: commands.run_select(
            a.mining, a.mirrors, a.work, a.out, a.tasks_per_language, a.reserves_per_language, c
        ),
        "construct": lambda a, c: commands.run_construct(
            a.selection,
            a.mining,
            a.corpus,
            a.mirrors,
            a.work,
            a.tasks_per_language,
            os.environ.get(RUN_WINDOW_ENV, DEFAULT_RUN_WINDOW) if a.night else ALWAYS_OPEN,
            a.budget,
            c,
        ),
        "keygen": lambda a, c: commands.run_keygen(a.identity, a.recipients, c),
        "seal": lambda a, c: commands.run_seal(a.corpus, a.plain, c),
        "unseal": lambda a, c: commands.run_unseal(a.corpus, a.identity, a.task_id, a.out, c),
        "verify-corpus": lambda a, c: commands.run_verify_corpus(a.corpus, a.identity, c),
        "build-env": lambda a, c: commands.run_build_env(Language(a.language), a.source, a.dest, c),
        "verify-task": lambda a, c: commands.run_verify_task(
            a.corpus, a.identity, a.task_id, a.clone, a.env_dir, a.python, a.out, c
        ),
        "build-workspace": lambda a, c: commands.run_build_workspace(
            a.corpus, a.identity, a.task_id, a.clone, a.init_dir, a.out, c
        ),
        "analyze": lambda a, c: commands.run_analyze(a.run, a.resamples, a.permutations, c),
        "report-lint": lambda a, c: commands.run_report_lint(a.report, c),
        "site": lambda a, c: run_site(SiteRequest(a.run, a.root, a.out, a.commit, a.check), c),
        "window-status": lambda a, c: commands.run_window_status(a.window, a.now, c),
        "night-budget": _night_budget,
    }


def run(argv: Sequence[str], console: Console | None = None) -> int:
    """Run one verb and return its exit code; every expected failure is a message and a code, never a traceback."""
    console = console or Console([])
    try:
        arguments = build_parser().parse_args(argv)
        return _handlers()[arguments.verb](arguments, console)
    except SystemExit as exit_request:
        return int(exit_request.code or 0) if isinstance(exit_request.code, int) else USAGE_ERROR
    except RunPaused as paused:
        console.say(f"paused: {paused}")
        return EXIT_PAUSED
    except MachineUnsafe as unsafe:
        console.say(f"machine unsafe: {unsafe}")
        return commands.EXIT_UNSAFE
    except EXPECTED_FAILURES as error:
        console.say(f"error: {error}")
        return commands.EXIT_FAILED


def main() -> None:
    sys.exit(run(sys.argv[1:]))
