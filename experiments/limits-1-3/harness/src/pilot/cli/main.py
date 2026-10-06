"""Argparse front door: `python -m pilot.cli <verb>`."""

from __future__ import annotations

import argparse
from collections.abc import Callable, Sequence
from pathlib import Path

from pilot.cli import commands
from pilot.cli.layout import default_root
from pilot.domain.constants import (
    EXPECTED_CRITERION_COUNT,
    EXPECTED_PLANTED_COUNT,
    EXPECTED_TEST_COUNT,
    EXPECTED_TEST_FILES,
    EXPECTED_VIOLATION_COUNT,
)

Handler = Callable[[argparse.Namespace], int]


def _add_root(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--root", type=Path, default=default_root(), help="experiment root")


def _add_out_dir(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--out-dir", type=Path, default=None, help="where artifacts are written")


def _add_counts(parser: argparse.ArgumentParser, *names: str) -> None:
    defaults = {
        "tests": EXPECTED_TEST_COUNT,
        "violations": EXPECTED_VIOLATION_COUNT,
        "planted": EXPECTED_PLANTED_COUNT,
        "criteria": EXPECTED_CRITERION_COUNT,
    }
    for name in names:
        parser.add_argument(f"--expected-{name}", type=int, default=defaults[name])


def build_parser() -> tuple[argparse.ArgumentParser, dict[str, Handler]]:
    parser = argparse.ArgumentParser(prog="pilot.cli")
    verbs = parser.add_subparsers(dest="verb", required=True)
    handlers: dict[str, Handler] = {}

    def verb(name: str, handler: Handler, help_text: str) -> argparse.ArgumentParser:
        sub = verbs.add_parser(name, help=help_text)
        _add_root(sub)
        handlers[name] = handler
        return sub

    sub = verb("verify-labels", commands.verify_labels_command, "PREREG 3.2 items 1-9")
    _add_out_dir(sub)
    _add_counts(sub, "tests", "violations", "planted", "criteria")

    sub = verb("runner-crosscheck", commands.runner_crosscheck_command, "matrix runner vs label_matrix.csv")
    _add_out_dir(sub)

    sub = verb("leak-check", commands.leak_check_command, "PREREG 2.5-2.6 static rules")
    _add_counts(sub, "tests")
    sub.add_argument("--test-files", nargs="+", default=list(EXPECTED_TEST_FILES))

    sub = verb("traceability-check", commands.traceability_check_command, "null gate G0 precondition")
    _add_out_dir(sub)
    _add_counts(sub, "tests", "criteria")

    sub = verb("canary", commands.canary_command, "one synthetic call per (model, CLI) pair")
    sub.add_argument("--run-id", required=True)
    return parser, handlers


def main(argv: Sequence[str] | None = None) -> int:
    parser, handlers = build_parser()
    args = parser.parse_args(argv)
    return handlers[args.verb](args)
