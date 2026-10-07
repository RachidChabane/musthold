"""The run-side verbs: argument parsing, the exit code of every block outcome and `log-deviation` end to end."""

from __future__ import annotations

import importlib
from argparse import ArgumentParser
from collections.abc import Callable, Sequence
from pathlib import Path
from typing import Final

import pytest

from calibration.application.block import BlockOutcome
from calibration.cli import run_verbs
from calibration.cli.layout import REPO_ROOT_ENV
from calibration.cli.run_verbs import GO_PROCS, OUTCOME_EXIT, register
from calibration.domain.constants import (
    DEFAULT_RUN_ID,
    DEVIATIONS_FILE,
    EXIT_FAILURE,
    EXIT_OK,
    EXIT_PAUSED,
    EXIT_PRECONDITION,
    EXPERIMENT_DIR,
    RUNS_DIR_NAME,
)
from calibration.domain.deviations import DEVIATIONS_TITLE, DeviationKind
from calibration.domain.models import MODEL_IDS

RUN_VERBS: Final = ("snapshot", "build-items", "pin", "canary", "run", "run-session", "log-deviation")
MODEL_VERBS: Final = ("canary", "run")
ANALYSIS_VERBS: Final = ("analyze", "derive", "report-lint")
ANALYSIS_VERBS_MODULE: Final = "calibration.cli.analysis_verbs"
ARGUMENT_ERROR: Final = 2
MODEL: Final = MODEL_IDS[0]
EXPECTED_EXIT: Final = {
    BlockOutcome.COMPLETE: EXIT_OK,
    BlockOutcome.BLOCKED: EXIT_OK,
    BlockOutcome.WINDOW_CLOSED: EXIT_PAUSED,
    BlockOutcome.HEAVY_WORK: EXIT_PAUSED,
    BlockOutcome.CANARY_FAILED: EXIT_FAILURE,
}


def run_side_parser() -> ArgumentParser:
    parser = ArgumentParser(prog="calibration")
    register(parser.add_subparsers(dest="verb", required=True))
    return parser


def dispatch(argv: Sequence[str]) -> int:
    """What `main` does, over the run-side verbs only."""
    arguments = run_side_parser().parse_args(argv)
    return arguments.handler(arguments)


def deviations_file(root: Path, run_id: str = DEFAULT_RUN_ID) -> Path:
    return root / EXPERIMENT_DIR / RUNS_DIR_NAME / run_id / DEVIATIONS_FILE


@pytest.fixture
def repository(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    monkeypatch.setenv(REPO_ROOT_ENV, str(tmp_path))
    return tmp_path


@pytest.fixture
def entry_point() -> Callable[[Sequence[str] | None], int]:
    """`calibration.cli.main.main`, whose parser also registers the analysis verbs."""
    pytest.importorskip(ANALYSIS_VERBS_MODULE)
    return importlib.import_module("calibration.cli.main").main


@pytest.mark.parametrize("verb", RUN_VERBS)
def test_every_run_verb_is_registered_with_a_handler(verb: str) -> None:
    arguments = run_side_parser().parse_args(
        [verb, *(["--model", MODEL] if verb in MODEL_VERBS else []), *deviation_arguments(verb)]
    )
    assert arguments.verb == verb
    assert callable(arguments.handler)


def deviation_arguments(verb: str) -> list[str]:
    if verb != "log-deviation":
        return []
    return ["--kind", DeviationKind.BLOCKED.value, "--scope", "all", "--text", "x"]


@pytest.mark.parametrize("verb", RUN_VERBS)
def test_the_run_id_defaults_to_the_first_run_and_can_be_chosen(verb: str) -> None:
    tail = [*(["--model", MODEL] if verb in MODEL_VERBS else []), *deviation_arguments(verb)]
    assert run_side_parser().parse_args([verb, *tail]).run_id == DEFAULT_RUN_ID
    assert run_side_parser().parse_args([verb, "--run-id", "run-07", *tail]).run_id == "run-07"


@pytest.mark.parametrize("verb", MODEL_VERBS)
def test_a_model_block_verb_requires_a_known_model(verb: str, capsys: pytest.CaptureFixture[str]) -> None:
    with pytest.raises(SystemExit) as missing:
        run_side_parser().parse_args([verb])
    with pytest.raises(SystemExit) as unknown:
        run_side_parser().parse_args([verb, "--model", "M9"])
    assert (missing.value.code, unknown.value.code) == (ARGUMENT_ERROR, ARGUMENT_ERROR)
    assert "--model" in capsys.readouterr().err


@pytest.mark.parametrize("model", MODEL_IDS)
def test_every_model_of_the_experiment_is_accepted(model: str) -> None:
    assert run_side_parser().parse_args(["run", "--model", model]).model == model


def test_build_items_takes_two_go_procs_and_refuses_a_dirty_harness_unless_told_otherwise() -> None:
    default = run_side_parser().parse_args(["build-items"])
    assert (default.go_procs, default.allow_dirty_harness) == (GO_PROCS, False)
    chosen = run_side_parser().parse_args(["build-items", "--go-procs", "4", "--allow-dirty-harness"])
    assert (chosen.go_procs, chosen.allow_dirty_harness) == (4, True)


def test_a_verb_that_is_not_registered_is_an_argument_error() -> None:
    with pytest.raises(SystemExit) as unknown:
        run_side_parser().parse_args(["analyze-everything"])
    assert unknown.value.code == ARGUMENT_ERROR


@pytest.mark.parametrize("missing", ["--kind", "--scope", "--text"])
def test_log_deviation_requires_kind_scope_and_text(missing: str) -> None:
    arguments = {"--kind": DeviationKind.BLOCKED.value, "--scope": "all", "--text": "x"}
    argv = [token for flag, value in arguments.items() if flag != missing for token in (flag, value)]
    with pytest.raises(SystemExit) as refused:
        run_side_parser().parse_args(["log-deviation", *argv])
    assert refused.value.code == ARGUMENT_ERROR


@pytest.mark.parametrize("kind", [DeviationKind.PROVENANCE, DeviationKind.PAUSE])
def test_an_operator_cannot_log_a_harness_kind(kind: DeviationKind) -> None:
    with pytest.raises(SystemExit) as refused:
        run_side_parser().parse_args(["log-deviation", "--kind", kind.value, "--scope", "none", "--text", "x"])
    assert refused.value.code == ARGUMENT_ERROR


def test_every_block_outcome_has_an_exit_code() -> None:
    assert set(OUTCOME_EXIT) == set(BlockOutcome)


@pytest.mark.parametrize("outcome", list(BlockOutcome))
def test_the_exit_code_of_a_block_outcome(outcome: BlockOutcome) -> None:
    assert OUTCOME_EXIT[outcome] == EXPECTED_EXIT[outcome]


def test_a_pause_is_distinguishable_from_success_and_from_failure() -> None:
    assert len({EXIT_OK, EXIT_PAUSED, EXIT_FAILURE, EXIT_PRECONDITION}) == 4


class FakeServices:
    def __init__(self, layout: object, run_id: str) -> None:
        self.run_id = run_id

    def block_deps(self) -> None:
        return None


def block_fake(outcome: BlockOutcome, calls: list[tuple[str, bool]]) -> type:
    class FakeBlock:
        def __init__(self, deps: object) -> None:
            pass

        def run(self, model: object, *, canary_only: bool = False) -> BlockOutcome:
            calls.append((getattr(model, "model_id"), canary_only))  # noqa: B009
            return outcome

    return FakeBlock


@pytest.mark.parametrize("outcome", list(BlockOutcome))
@pytest.mark.parametrize(("verb", "canary_only"), [("run", False), ("canary", True)])
def test_a_block_verb_exits_with_the_code_of_the_outcome_and_reports_it(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
    repository: Path,
    outcome: BlockOutcome,
    verb: str,
    canary_only: bool,
) -> None:
    calls: list[tuple[str, bool]] = []
    monkeypatch.setattr(run_verbs, "RunServices", FakeServices)
    monkeypatch.setattr(run_verbs, "ModelBlock", block_fake(outcome, calls))
    code = dispatch([verb, "--model", MODEL])
    assert code == EXPECTED_EXIT[outcome]
    assert calls == [(MODEL, canary_only)]
    assert f'"outcome": "{outcome.value}"' in capsys.readouterr().out


def test_log_deviation_appends_the_rendered_line_and_prints_it(
    repository: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    code = dispatch(["log-deviation", "--kind", "blocked", "--scope", "M3, K4", "--text", "  server   refused  "])
    line = "- blocked | scope: K4, M3 | server refused"
    assert code == EXIT_OK
    assert capsys.readouterr().out == line + "\n"
    assert deviations_file(repository).read_text() == f"{DEVIATIONS_TITLE}\n\n{line}\n"


def test_log_deviation_appends_to_the_log_of_the_chosen_run_without_rewriting_it(repository: Path) -> None:
    first = ["log-deviation", "--run-id", "run-05", "--kind", "protocol", "--scope", "all", "--text", "one"]
    assert dispatch(first) == EXIT_OK
    assert dispatch([*first[:-1], "two"]) == EXIT_OK
    assert deviations_file(repository, "run-05").read_text().splitlines()[2:] == [
        "- protocol | scope: all | one",
        "- protocol | scope: all | two",
    ]
    assert not deviations_file(repository).exists()


@pytest.mark.parametrize(
    ("scope", "text"),
    [("M9", "unknown scope"), ("", "empty scope"), ("all", "   ")],
)
def test_log_deviation_refuses_an_invalid_entry_with_the_precondition_code_and_writes_nothing(
    repository: Path, capsys: pytest.CaptureFixture[str], scope: str, text: str
) -> None:
    code = dispatch(["log-deviation", "--kind", "blocked", "--scope", scope, "--text", text])
    assert code == EXIT_PRECONDITION
    assert capsys.readouterr().out.startswith("calibration: ")
    assert not deviations_file(repository).exists()


def test_the_entry_point_runs_log_deviation(
    repository: Path,
    capsys: pytest.CaptureFixture[str],
    entry_point: Callable[[Sequence[str] | None], int],
) -> None:
    code = entry_point(["log-deviation", "--kind", "protocol", "--scope", "none", "--text", "noted"])
    assert code == EXIT_OK
    assert capsys.readouterr().out == "- protocol | scope: none | noted\n"


def test_the_entry_point_exits_with_an_argument_error_for_an_unknown_verb(
    entry_point: Callable[[Sequence[str] | None], int],
) -> None:
    with pytest.raises(SystemExit) as refused:
        entry_point(["no-such-verb"])
    assert refused.value.code == ARGUMENT_ERROR


@pytest.mark.parametrize("verb", ANALYSIS_VERBS)
def test_the_analysis_verbs_are_registered_beside_the_run_verbs(verb: str) -> None:
    pytest.importorskip(ANALYSIS_VERBS_MODULE)
    main_module = importlib.import_module("calibration.cli.main")
    parser = main_module.build_parser()
    choices = next(action.choices for action in parser._actions if action.dest == "verb")
    assert {*RUN_VERBS, verb} <= set(choices)
