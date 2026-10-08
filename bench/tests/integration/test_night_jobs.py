"""`script/night-jobs` in dry-run mode, on fixture ledgers and with every state directory under `tmp_path`."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path

import pytest

from specbench.adapters.machine_slots import MachineSlots

pytestmark = [
    pytest.mark.integration,
    pytest.mark.skipif(sys.platform != "darwin", reason="the night jobs run on macOS: they use BSD date and launchd"),
]

REPOSITORY = Path(__file__).resolve().parents[3]
NIGHT_JOBS = REPOSITORY / "script" / "night-jobs"
EXIT_PAUSED = 3
MACOS_BASH = "/bin/bash"
NIGHT_SHIFT = timedelta(hours=12)
BUDGET_LINE = re.compile(r"construct: budget (\d+)")
LAUNCH_BUDGET = re.compile(r"script/construct --night --budget (\d+)")


@dataclass
class Result:
    code: int
    log: str

    def construct_budget(self) -> int | None:
        launched = LAUNCH_BUDGET.search(self.log)
        return int(launched.group(1)) if launched else None


class Night:
    """The sandbox one run of the night jobs sees; nothing in it is the owner's real state."""

    def __init__(self, root: Path) -> None:
        self.root = root
        self.slots = root / "slots"
        self.pilot_ledger = root / "pilot-call-budget.json"
        self.night = (datetime.now() - NIGHT_SHIFT).date().isoformat()

    def pilot_spent(self, calls: int) -> None:
        self.pilot_ledger.write_text(json.dumps({self.night: {"claude": calls}}))

    def run(self, job: str, shell: str | None = None, **extra: str) -> Result:
        environment = {
            **os.environ,
            "NIGHT_JOBS_DRY_RUN": "1",
            "NIGHT_JOBS_LOG_DIR": str(self.root / "logs"),
            "NIGHT_BUDGET_PILOT_LEDGER": str(self.pilot_ledger),
            "SPECBENCH_WORK": str(self.root / "work"),
            "SPEC_PROOF_STATE_DIR": str(self.root / "state"),
            "SPEC_PROOF_SLOTS_DIR": str(self.slots),
            **extra,
        }
        done = subprocess.run(
            [*([shell] if shell else []), str(NIGHT_JOBS), job],
            env=environment,
            check=False,
            capture_output=True,
            text=True,
            cwd=REPOSITORY,
        )
        return Result(done.returncode, done.stdout + done.stderr)


@pytest.fixture
def night(tmp_path: Path) -> Night:
    return Night(tmp_path)


@pytest.mark.parametrize(("pilot_calls", "expected"), [(0, 120), (129, 120), (200, 49), (248, 1)])
def test_construction_is_launched_with_what_the_pilot_and_the_canary_reserve_leave(
    night: Night, pilot_calls: int, expected: int
) -> None:
    night.pilot_spent(pilot_calls)
    result = night.run("construct")
    assert result.code == 0
    assert result.construct_budget() == expected
    assert BUDGET_LINE.search(result.log)


def test_construction_does_not_start_once_the_pilot_has_spent_the_cap(night: Night) -> None:
    night.pilot_spent(250)
    result = night.run("construct")
    assert result.code == 0 and result.construct_budget() is None
    assert "spent" in result.log


@pytest.mark.parametrize("pilot_calls", [0, 200, 250])
def test_the_shared_computation_agrees_with_the_standalone_fallback_arithmetic(night: Night, pilot_calls: int) -> None:
    night.pilot_spent(pilot_calls)
    delegated = night.run("construct")
    standalone = night.run("construct", NIGHT_JOBS_BUDGET_DELEGATE="0")
    assert delegated.construct_budget() == standalone.construct_budget()


def test_a_failing_computation_falls_back_to_the_arithmetic_and_the_night_goes_on(night: Night) -> None:
    night.pilot_spent(100)
    result = night.run("construct", SPECBENCH_SEED="0")
    assert result.code == 0
    assert "night-budget failed" in result.log
    assert result.construct_budget() == 120


def test_the_pilot_and_the_canaries_are_only_printed_not_run(night: Night) -> None:
    pilot = night.run("pilot")
    assert pilot.code == 0 and any(word in pilot.log for word in ("dry-run:", "already running", "is finished"))
    canaries = night.run("canaries")
    assert canaries.code == 0 and "script/canary-claude" in canaries.log and "script/canary-build" in canaries.log


def test_a_job_runs_inside_its_slots_when_slots_are_on(night: Night) -> None:
    result = night.run("canaries", NIGHT_JOBS_SLOTS="1")
    assert result.code == 0 and "script/canary-claude" in result.log
    assert MachineSlots(night.slots).free() == 2


def test_a_job_pauses_with_exit_3_when_the_machine_has_no_free_slot(night: Night) -> None:
    held = MachineSlots(night.slots).try_acquire(2)
    assert held is not None
    paused = night.run("construct", NIGHT_JOBS_SLOTS="1")
    assert paused.code == EXIT_PAUSED
    assert "slot" in paused.log and "script/construct" not in paused.log
    held.release()
    assert night.run("construct", NIGHT_JOBS_SLOTS="1").code == 0


def test_slots_are_off_unless_asked_for(night: Night) -> None:
    held = MachineSlots(night.slots).try_acquire(2)
    assert held is not None
    assert night.run("canaries").code == 0
    held.release()


def test_an_unknown_job_is_a_usage_error(night: Night) -> None:
    assert night.run("nonsense").code == 2


@pytest.mark.skipif(not Path(MACOS_BASH).exists(), reason="the system bash 3.2 is part of macOS")
def test_the_jobs_and_the_budget_script_run_under_the_bash_that_macos_ships(night: Night) -> None:
    night.pilot_spent(100)
    constructed = night.run("construct", shell=MACOS_BASH, NIGHT_JOBS_SLOTS="1")
    assert (
        constructed.code == 0 and constructed.construct_budget() == 120 and "night-budget failed" not in constructed.log
    )
    assert night.run("canaries", shell=MACOS_BASH, NIGHT_JOBS_SLOTS="1").code == 0
