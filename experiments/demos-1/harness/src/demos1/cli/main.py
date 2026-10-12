"""The composition root: one verb per subcommand, each wiring the adapters to its use case."""

from __future__ import annotations

import argparse
import os
import signal
import sys
from collections.abc import Callable, Sequence
from pathlib import Path, PurePosixPath
from typing import Final

from demos1.adapters.builds import ImageBuild, ProductBuild
from demos1.adapters.engine import ColimaMachine, DockerEngine
from demos1.adapters.github import GhCliApi, RecordingApi, ReplayApi
from demos1.adapters.host import GitRepository, LocalHostFacts
from demos1.adapters.local import DeviationFile, LocalFiles, ReplayClock, SystemClock
from demos1.application.attempt_run import AttemptContext, AttemptVerb
from demos1.application.attempts import UndefinedStage
from demos1.application.command_runner import CommandRunner, RefusalBudget, WaitBoundReached
from demos1.application.environment import EnvironmentProbe, ProbeUnavailable
from demos1.application.machine_session import machine_running
from demos1.application.ports import OperatorStopped
from demos1.application.reporting import SummarizeVerb, SummaryPaths, report_lint
from demos1.application.search import SearchFailed
from demos1.application.select_run import (
    Refused,
    SelectContext,
    SelectVerb,
    candidate_pool,
    run_selection,
    write_selection,
)
from demos1.domain.environment import PRODUCT_TAG
from demos1.domain.layout import WorkLayout
from demos1.domain.machine import docker_socket

WORK_ENV: Final = "DEMOS1_WORK"
PRODUCT_ENV: Final = "DEMOS1_PRODUCT"
REPO_ENV: Final = "DEMOS1_REPO"
DEFAULT_WORK: Final = ".spec-proof-demos-1"
DEFAULT_PRODUCT: Final = ".spec-proof-demos-1-product"
REPO_DEPTH: Final = 6
"""harness/src/demos1/cli/main.py lies six directories below the repository root."""
IMAGE_CONTEXT: Final = "experiments/demos-1/image"
EXIT_OK: Final = 0
EXIT_FAILED: Final = 1
EXIT_REFUSED: Final = 3
EXIT_WAIT_BOUND: Final = 4
EXIT_UNDEFINED: Final = 5
EXIT_STOPPED: Final = 130


STOP_EXITS: Final[tuple[tuple[tuple[type[BaseException], ...], int, str], ...]] = (
    ((Refused, ProbeUnavailable), EXIT_REFUSED, "refused"),
    ((SearchFailed,), EXIT_FAILED, "stopped"),
    ((WaitBoundReached,), EXIT_WAIT_BOUND, "stopped"),
    ((UndefinedStage,), EXIT_UNDEFINED, "stopped on an undefined stage"),
    ((OperatorStopped, KeyboardInterrupt), EXIT_STOPPED, "interrupted"),
)
"""How each way a verb stops maps to its exit status."""


def repo_root() -> Path:
    override = os.environ.get(REPO_ENV)
    return Path(override) if override else Path(__file__).resolve().parents[REPO_DEPTH]


def work_layout() -> WorkLayout:
    home = Path.home()
    work = Path(os.environ.get(WORK_ENV) or home / DEFAULT_WORK).resolve()
    product = Path(os.environ.get(PRODUCT_ENV) or home / DEFAULT_PRODUCT).resolve()
    return WorkLayout(PurePosixPath(work), PurePosixPath(product))


class Wiring:
    """The adapters every verb shares."""

    def __init__(self) -> None:
        self.root = repo_root()
        self.work = work_layout()
        self.clock = SystemClock()
        self.files = LocalFiles()
        self.socket = docker_socket(PurePosixPath(Path.home()))
        self.engine = DockerEngine(self.socket, PurePosixPath(self.root / "script"), self.clock.now)
        self.machine = ColimaMachine()
        self.deviations = DeviationFile(self.files, self.work.deviations)
        self.refusals = RefusalBudget()

    def runner_of(self, persist: Callable[[], None]) -> CommandRunner:
        return CommandRunner(self.engine, self.files, self.clock, self.refusals, persist)

    def probe(self) -> EnvironmentProbe:
        return EnvironmentProbe(
            self.files,
            LocalHostFacts(self.engine),
            self.machine,
            self.runner_of(lambda: None),
            os.getuid(),
            os.getgid(),
        )


def _select(arguments: argparse.Namespace) -> int:
    wiring = Wiring()
    if arguments.replay is not None:
        out = WorkLayout(PurePosixPath(Path(arguments.out).resolve()), wiring.work.product_root)
        pool = candidate_pool(wiring.files, PurePosixPath(wiring.root))
        result = run_selection(ReplayApi.from_directory(PurePosixPath(arguments.replay)), ReplayClock(), pool)
        write_selection(wiring.files, out, result)
        print(f"select: lists recomputed from {arguments.replay} into {out.lists}")
        return EXIT_OK
    verb = SelectVerb(
        wiring.files,
        GitRepository(PurePosixPath(wiring.root)),
        wiring.probe(),
        lambda directory: RecordingApi(GhCliApi(wiring.clock.now), directory),
        wiring.clock,
        wiring.deviations,
        wiring.machine,
    )
    verb.run(SelectContext(PurePosixPath(wiring.root), wiring.work))
    print(f"select: lists written to {wiring.work.lists}")
    return EXIT_OK


def _attempt(_: argparse.Namespace) -> int:
    wiring = Wiring()
    verb = AttemptVerb(
        wiring.files,
        wiring.engine,
        wiring.clock,
        wiring.deviations,
        wiring.probe(),
        wiring.runner_of,
        wiring.machine,
        wiring.refusals,
    )
    records = verb.run(AttemptContext(wiring.work, os.getuid(), os.getgid()))
    for record in records:
        print(f"attempt: {record.attempt} {record.outcome}")
    return EXIT_OK


def _summarize(_: argparse.Namespace) -> int:
    wiring = Wiring()
    paths = SummaryPaths(PurePosixPath(wiring.root))
    SummarizeVerb(wiring.files, GitRepository(PurePosixPath(wiring.root))).run(wiring.work, paths)
    print(f"summarize: wrote {paths.document} and {paths.markdown}")
    return EXIT_OK


def _report_lint(_: argparse.Namespace) -> int:
    wiring = Wiring()
    violations = report_lint(wiring.files, SummaryPaths(PurePosixPath(wiring.root)))
    for violation in violations:
        print(violation.render())
    return EXIT_FAILED if violations else EXIT_OK


def _build_product(arguments: argparse.Namespace) -> int:
    wiring = Wiring()
    document = ProductBuild(wiring.root, Path(wiring.work.product_root)).build(arguments.ref)
    print(f"build-product: {document.get('commit')} into {wiring.work.product_bin}")
    return EXIT_OK


def _build_image(_: argparse.Namespace) -> int:
    wiring = Wiring()
    with machine_running(wiring.machine, wiring.work.mounts()):
        document = ImageBuild(wiring.root / IMAGE_CONTEXT, wiring.work.image_build, wiring.socket).build()
    print(f"build-image: {document.get('image_id')}")
    return EXIT_OK


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(prog="demos1")
    verbs = root.add_subparsers(dest="verb", required=True)
    select = verbs.add_parser("select", help="PREREG 5-7: the four lists, once, within one UTC date")
    select.add_argument("--replay", help="recompute the lists from a directory of saved responses, offline")
    select.add_argument("--out", help="with --replay: the directory that receives frame.jsonl and lists.json")
    select.set_defaults(run=_select)
    verbs.add_parser("attempt", help="PREREG 8-10: the attempts, pytest first, resumable").set_defaults(run=_attempt)
    verbs.add_parser("summarize", help="PREREG 12.2: notes/demos-1/summary.json and SUMMARY.md").set_defaults(
        run=_summarize
    )
    verbs.add_parser("report-lint", help="PREREG 12.3: lint SUMMARY.md").set_defaults(run=_report_lint)
    product = verbs.add_parser("build-product", help="PREREG 2: the linux binaries of the product tag")
    product.add_argument("--ref", default=PRODUCT_TAG, help=f"the commit to build (default {PRODUCT_TAG})")
    product.set_defaults(run=_build_product)
    verbs.add_parser("build-image", help="PREREG 2: the image, from both source images by digest").set_defaults(
        run=_build_image
    )
    return root


def main(argv: Sequence[str] | None = None) -> int:
    signal.signal(signal.SIGTERM, signal.default_int_handler)
    arguments = parser().parse_args(argv)
    if arguments.verb == "select" and (arguments.replay is None) != (arguments.out is None):
        print("select: --replay and --out go together", file=sys.stderr)
        return EXIT_REFUSED
    try:
        return int(arguments.run(arguments))
    except BaseException as stopped:
        for kinds, exit_code, words in STOP_EXITS:
            if isinstance(stopped, kinds):
                print(f"{arguments.verb}: {words}: {stopped}", file=sys.stderr)
                return exit_code
        raise
