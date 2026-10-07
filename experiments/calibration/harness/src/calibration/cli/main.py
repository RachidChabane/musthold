"""Entry point: `python -m calibration.cli <verb>`."""

from __future__ import annotations

import sys
from argparse import ArgumentParser, Namespace
from collections.abc import Callable, Sequence

from calibration.cli import analysis_verbs, run_verbs
from calibration.domain.constants import EXIT_FAILURE

PROGRAM = "calibration"
Handler = Callable[[Namespace], int]


def build_parser() -> ArgumentParser:
    parser = ArgumentParser(prog=PROGRAM, description="Harness of the System One confidence calibration experiment")
    subparsers = parser.add_subparsers(dest="verb", required=True)
    run_verbs.register(subparsers)
    analysis_verbs.register(subparsers)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    arguments = build_parser().parse_args(argv)
    handler: Handler | None = getattr(arguments, "handler", None)
    if handler is None:
        return EXIT_FAILURE
    return handler(arguments)


if __name__ == "__main__":
    sys.exit(main())
