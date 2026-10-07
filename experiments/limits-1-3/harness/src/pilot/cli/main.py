"""Argparse front door: `python -m pilot.cli <verb>`."""

from __future__ import annotations

import argparse
import os
from collections.abc import Callable, Sequence
from pathlib import Path

from pilot.cli import analysis_verbs, commands, pipeline_commands
from pilot.cli.layout import default_root
from pilot.domain.call_budget import DEFAULT_CALL_BUDGET
from pilot.domain.call_type import CallType
from pilot.domain.run_window import DEFAULT_RUN_WINDOW, RUN_WINDOW_ENV
from pilot.domain.deviations import OPERATOR_KINDS
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


def _add_window(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--run-window", default=os.environ.get(RUN_WINDOW_ENV, DEFAULT_RUN_WINDOW),
        help="daily local-time window HH:MM-HH:MM (may cross midnight) for model calls, or 'always'",
    )


def _add_run(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--run-id", required=True)
    _add_counts(parser, "tests", "criteria")
    _add_window(parser)
    parser.add_argument(
        "--call-budget", type=int, default=DEFAULT_CALL_BUDGET,
        help="new model calls per UTC day and CLI before the verb stops cleanly (exit 3)",
    )


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

    sub = verb("isolation-table", commands.isolation_table_command, "C_D8 and the isolation of the hand-written violations")
    _add_out_dir(sub)
    _add_counts(sub, "tests", "criteria")

    sub = verb("canary", pipeline_commands.canary_command, "one synthetic call per (model, call type)")
    sub.add_argument("--run-id", required=True)
    sub.add_argument("--models", nargs="+", default=None, help="restrict to these model names")
    sub.add_argument("--call-types", nargs="+", default=None, choices=[c.value for c in CallType])
    _add_window(sub)

    verb("assemble-arm-p", commands.assemble_arm_p_command, "arm-P items and labels from the ground truth")

    verb("leak-audit", pipeline_commands.leak_audit_command, "PREREG 3.3 cross-family leak audit")

    sub = verb("run-window", pipeline_commands.run_window_command, "exit 0 inside the run window, 3 outside")
    _add_window(sub)

    sub = verb("import-run-01", pipeline_commands.import_run_01_command, "carry the sealed run-01's completed calls into a run, by digest")
    sub.add_argument("--run-id", required=True)
    sub.add_argument("--plan-only", action="store_true", help="print the digests of the files an import would carry; write nothing")

    sub = verb("relabel", pipeline_commands.relabel_command, "PREREG 3.4 blind relabels by two model raters")
    _add_run(sub)

    sub = verb("run-core", pipeline_commands.run_core_command, "PREREG 14 step 3")
    _add_run(sub)

    sub = verb("run-blindspot", pipeline_commands.run_blindspot_command, "contrast mutants, test writers, operator mutants and writer kill vectors")
    _add_run(sub)

    sub = verb("run-secondary", pipeline_commands.run_secondary_command, "J-H NAME-SHOWN, J-X and the mutant audit")
    _add_run(sub)

    sub = verb("run-luna", pipeline_commands.run_luna_command, "the exploratory luna stratum, inside its window")
    _add_run(sub)

    sub = verb("log-deviation", pipeline_commands.log_deviation_command, "append one entry to DEVIATIONS.md")
    sub.add_argument("--run-id", required=True)
    sub.add_argument("--kind", required=True, choices=[kind.value for kind in OPERATOR_KINDS])
    sub.add_argument("--endpoints", required=True, help="none, all, or endpoint ids such as D3,D4")
    sub.add_argument("--text", required=True)

    analysis_verbs.register(verb)
    return parser, handlers


def main(argv: Sequence[str] | None = None) -> int:
    parser, handlers = build_parser()
    args = parser.parse_args(argv)
    return handlers[args.verb](args)
