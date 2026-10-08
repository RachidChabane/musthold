from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.adapters.night_state import (
    BUDGET_LOCK_FILE_NAME,
    BudgetLockTimeout,
    FileConstructionProgress,
    budget_lock,
)
from specbench.cli.commands import Console
from specbench.cli.main import run

pytestmark = pytest.mark.integration

NIGHT = "2026-10-08"
SELECTED = {"PY-1": "python", "PY-2": "python", "GO-1": "go"}


class Fixture:
    def __init__(self, root: Path) -> None:
        self.root = root
        self.pilot = root / "pilot.json"
        self.construct = root / "construct.json"
        self.sample = root / "sample.json"
        self.codex = root / "codex.json"
        self.selection = root / "selection"
        self.corpus = root / "corpus"
        self.state = root / "state"
        self._write_selection()

    def _write_selection(self) -> None:
        self.selection.mkdir()
        (self.selection / "selection.json").write_text(json.dumps({"tasks": [{"task_id": t} for t in SELECTED]}))
        for task_id, language in SELECTED.items():
            directory = self.selection / "tasks" / task_id
            directory.mkdir(parents=True)
            (directory / "task.json").write_text(json.dumps({"language": language, "task_id": task_id}))

    def seal(self, *task_ids: str) -> None:
        for task_id in task_ids:
            directory = self.corpus / SELECTED[task_id] / task_id
            directory.mkdir(parents=True)
            (directory / "task.json").write_text("{}")

    def ledger(self, path: Path, record: dict[str, dict[str, int]]) -> None:
        path.write_text(json.dumps(record))

    def budget(self, job: str, *extra: str) -> str:
        console = Console([])
        code = run(
            [
                "night-budget",
                job,
                "--night",
                NIGHT,
                "--pilot-ledger",
                str(self.pilot),
                "--construct-ledger",
                str(self.construct),
                "--sample-ledger",
                str(self.sample),
                "--codex-ledger",
                str(self.codex),
                "--selection",
                str(self.selection),
                "--corpus",
                str(self.corpus),
                "--state-dir",
                str(self.state),
                *extra,
            ],
            console,
        )
        assert code == 0
        return "\n".join(console.out)


@pytest.fixture
def fixture(tmp_path: Path) -> Fixture:
    return Fixture(tmp_path)


@pytest.mark.parametrize(("pilot_calls", "expected"), [(0, 120), (129, 120), (130, 119), (200, 49), (249, 0), (250, 0)])
def test_construction_share_follows_the_pilot_ledger_through_the_claude_alias(
    fixture: Fixture, pilot_calls: int, expected: int
) -> None:
    fixture.ledger(fixture.pilot, {NIGHT: {"claude": pilot_calls}, "2026-10-07": {"claude": 250}})
    assert fixture.budget("construct") == str(expected)


def test_without_a_pilot_ledger_construction_gets_its_whole_allowance(fixture: Fixture) -> None:
    assert fixture.budget("construct") == "120"


def test_nothing_is_reserved_for_construction_once_every_language_has_its_selected_tasks_sealed(
    fixture: Fixture,
) -> None:
    fixture.seal("PY-1", "PY-2")
    assert fixture.budget("construct") == "120"
    fixture.seal("GO-1")
    assert fixture.budget("construct") == "0"
    assert fixture.budget("sample") == str(250 - 1)


def test_a_reserve_task_sealed_in_place_of_a_dropped_one_completes_its_language(fixture: Fixture) -> None:
    fixture.seal("PY-1")
    reserve = fixture.corpus / "python" / "PY-RESERVE"
    reserve.mkdir(parents=True)
    (reserve / "task.json").write_text("{}")
    fixture.seal("GO-1")
    assert not FileConstructionProgress(fixture.selection, fixture.corpus).has_unsealed_task()


def test_the_sample_gets_what_the_pilot_the_canaries_and_construction_leave(fixture: Fixture) -> None:
    fixture.ledger(fixture.pilot, {NIGHT: {"claude": 40}})
    assert fixture.budget("sample", "--canary-reserve", "3") == str(250 - 40 - 3 - 120)


def test_construction_that_spent_more_than_its_share_keeps_the_sample_out_of_its_spend(fixture: Fixture) -> None:
    fixture.seal("PY-1", "PY-2", "GO-1")
    fixture.ledger(fixture.construct, {NIGHT: {"claude-code": 100}})
    assert fixture.budget("sample") == str(250 - 1 - 100)


def test_the_codex_ledger_and_cap_are_separate_from_claude(fixture: Fixture) -> None:
    fixture.ledger(fixture.pilot, {NIGHT: {"claude": 250, "codex": 25}})
    fixture.ledger(fixture.codex, {NIGHT: {"codex": 70}})
    assert fixture.budget("sample") == "0"
    assert fixture.budget("sample", "--harness", "codex", "--codex-cap", "100") == "75"
    assert json.loads(fixture.budget("sample", "--harness", "codex", "--explain"))["sample_spent"] == 70


def test_the_canary_reserve_is_recorded_once_and_used_afterwards(fixture: Fixture) -> None:
    fixture.budget("canaries", "--record-canary", "7")
    assert fixture.budget("canaries") == "7"
    assert fixture.budget("construct") == str(120)
    fixture.ledger(fixture.pilot, {NIGHT: {"claude": 200}})
    assert fixture.budget("construct") == str(250 - 200 - 7)
    assert fixture.budget("canaries", "--canary-reserve", "2") == "2"


def test_explain_prints_every_input_and_share_as_json(fixture: Fixture) -> None:
    fixture.ledger(fixture.pilot, {NIGHT: {"claude": 10}})
    report = json.loads(fixture.budget("pilot", "--explain"))
    assert report["pilot_spent"] == 10 and report["night"] == NIGHT and report["construct_pending"] is True
    assert report["shares"] == {"pilot": 250, "canaries": 1, "construct": 120, "sample": 250 - 10 - 1 - 120}


def test_a_missing_selection_is_a_reported_failure_not_a_traceback(fixture: Fixture) -> None:
    (fixture.selection / "selection.json").unlink()
    console = Console([])
    code = run(
        [
            "night-budget",
            "construct",
            "--pilot-ledger",
            str(fixture.pilot),
            "--construct-ledger",
            str(fixture.construct),
            "--sample-ledger",
            str(fixture.sample),
            "--codex-ledger",
            str(fixture.codex),
            "--selection",
            str(fixture.selection),
            "--corpus",
            str(fixture.corpus),
            "--state-dir",
            str(fixture.state),
        ],
        console,
    )
    assert code != 0 and console.out


def test_the_budget_lock_serialises_computations(tmp_path: Path) -> None:
    lock = tmp_path / BUDGET_LOCK_FILE_NAME
    with budget_lock(lock), pytest.raises(BudgetLockTimeout), budget_lock(lock, timeout_seconds=0.3):
        pass
    with budget_lock(lock):
        pass
