"""What `analyze` checks before computing anything: blindness, qualification and the bindings (PREREG 2.2, 2.3)."""

from __future__ import annotations

import json
from dataclasses import dataclass, replace
from datetime import UTC, datetime, timedelta
from typing import Final

import pytest

from calibration.analysis.preconditions import (
    binding_problems,
    blindness_problems,
    check_preconditions,
    earliest_started,
    qualification,
)
from calibration.analysis.run_data import RunData, read_run
from calibration.analysis.truth import PilotTruth, read_pilot_truth
from calibration.domain.canonical import JsonValue
from calibration.domain.freeze import PreconditionFailed
from calibration.domain.ports import Timestamp
from support.fake_ports import COMMIT_TIME, STAMP_IMPRINT, STAMP_TIME, FakeTimestampReader, FakeVersionControl
from support.synthetic_world import World, build_world
from support.world_model import MEASURED_MUTANTS, ORACLED_MUTANTS, TESTS

PILOT_START: Final = datetime(2026, 9, 12, 19, 3, 11, tzinfo=UTC)
TIMING: Final = {
    "core": {"started": "2026-09-12T21:03:11+02:00", "finished": "2026-09-12T23:40:02+02:00"},
    "secondary": {"started": "2026-09-13T21:01:00+02:00"},
}
ONE_SECOND: Final = timedelta(seconds=1)


@dataclass(frozen=True)
class Loaded:
    world: World
    run: RunData
    truth: PilotTruth


@pytest.fixture(scope="module")
def loaded(tmp_path_factory: pytest.TempPathFactory) -> Loaded:
    world = build_world(tmp_path_factory.mktemp("preconditions") / "world")
    return Loaded(world, read_run(world.run_dir), read_pilot_truth(world.prereg_dir, world.pilot_run_dir))


def _stamp(moment: datetime) -> Timestamp:
    return Timestamp(STAMP_IMPRINT, moment)


def _check(
    loaded: Loaded,
    *,
    truth: PilotTruth | None = None,
    stamp: datetime | None = STAMP_TIME,
    commit_time: datetime = COMMIT_TIME,
    inputs: dict[str, JsonValue] | None = None,
) -> dict[str, JsonValue]:
    return check_preconditions(
        loaded.run,
        truth if truth is not None else loaded.truth,
        FakeVersionControl(commit_time),
        FakeTimestampReader(stamp),
        loaded.world.freeze_receipt,
        loaded.world.pilot_run_dir,
        inputs if inputs is not None else {"seal_mismatches": []},
    )


class TestQualification:
    def test_every_measured_mutant_of_arms_t_and_p_is_counted(self, loaded: Loaded) -> None:
        record = qualification(loaded.truth)
        assert record.evaluated == len(MEASURED_MUTANTS)

    def test_the_mutants_without_an_oracle_record_are_listed(self, loaded: Loaded) -> None:
        record = qualification(loaded.truth)
        missing = sorted(m.mutant_id for m in MEASURED_MUTANTS if m.failed is None)
        assert record.missing == tuple(missing)
        assert record.with_oracle == len(ORACLED_MUTANTS)

    def test_the_record_serialises_its_three_counts(self, loaded: Loaded) -> None:
        record = qualification(loaded.truth).to_record()
        assert set(record) == {"evaluated", "with_oracle", "missing"}

    def test_a_truth_without_any_oracle_has_no_qualified_mutant(self, loaded: Loaded) -> None:
        record = qualification(replace(loaded.truth, oracle={}))
        assert (record.with_oracle, len(record.missing)) == (0, len(MEASURED_MUTANTS))


class TestBindingProblems:
    def test_the_world_binds_every_test_to_its_cited_criterion(self, loaded: Loaded) -> None:
        assert binding_problems(loaded.truth) == []

    def test_a_rebound_test_is_named_with_both_criteria(self, loaded: Loaded) -> None:
        test = TESTS[0]
        rebound = replace(loaded.truth, bindings={**loaded.truth.bindings, test.nodeid: "PARK-4~1"})
        (problem,) = binding_problems(rebound)
        assert test.nodeid in problem
        assert "PARK-4~1" in problem
        assert test.cites_ref in problem

    def test_an_unbound_test_is_a_problem(self, loaded: Loaded) -> None:
        test = TESTS[1]
        bindings = {k: v for k, v in loaded.truth.bindings.items() if k != test.nodeid}
        assert len(binding_problems(replace(loaded.truth, bindings=bindings))) == 1


class TestEarliestStarted:
    def test_the_earliest_aware_start_is_returned_in_utc(self) -> None:
        assert earliest_started(TIMING) == PILOT_START

    def test_a_naive_start_is_read_in_local_time(self) -> None:
        moment = earliest_started({"core": {"started": "2026-09-12T10:00:00"}})
        assert moment == datetime(2026, 9, 12, 10, 0, 0).astimezone(UTC)

    def test_stages_without_a_start_are_ignored(self) -> None:
        assert (
            earliest_started({"a": {"seconds": 1}, "b": "x", "c": {"started": "2026-09-12T19:03:11+00:00"}})
            == PILOT_START
        )

    def test_no_stage_gives_none(self) -> None:
        assert earliest_started({}) is None


class TestBlindness:
    def test_a_stamp_before_the_commit_and_the_first_stage_has_no_problem(self) -> None:
        assert blindness_problems(_stamp(PILOT_START - ONE_SECOND), COMMIT_TIME, TIMING) == []

    def test_a_stamp_equal_to_the_commit_time_fails(self) -> None:
        pilot_late = {"core": {"started": "2026-10-01T00:00:00+00:00"}}
        problems = blindness_problems(_stamp(COMMIT_TIME), COMMIT_TIME, pilot_late)
        assert len(problems) == 1
        assert "analysis commit" in problems[0]

    def test_a_stamp_equal_to_the_first_stage_start_fails(self) -> None:
        problems = blindness_problems(_stamp(PILOT_START), COMMIT_TIME, TIMING)
        assert len(problems) == 1
        assert "first pilot stage" in problems[0]

    def test_a_stamp_after_both_gives_both_problems(self) -> None:
        assert len(blindness_problems(_stamp(COMMIT_TIME + ONE_SECOND), COMMIT_TIME, TIMING)) == 2

    def test_an_unreadable_stamp_is_a_problem(self) -> None:
        assert blindness_problems(None, COMMIT_TIME, TIMING) == ["the timestamp of the pre-registration cannot be read"]

    def test_a_pilot_without_stage_starts_is_a_problem(self) -> None:
        assert blindness_problems(_stamp(STAMP_TIME), COMMIT_TIME, {}) == ["the pilot run records no stage start"]


class TestCheckPreconditions:
    def test_a_blind_run_with_matching_bindings_is_checked(self, loaded: Loaded) -> None:
        record = _check(loaded)
        assert record["blindness"] == "checked"
        assert record["qualification"] == qualification(loaded.truth).to_record()

    def test_a_stamp_after_the_pilot_analysis_commit_fails_blindness(self, loaded: Loaded) -> None:
        with pytest.raises(PreconditionFailed) as failure:
            _check(loaded, stamp=COMMIT_TIME + ONE_SECOND)
        assert failure.value.code == "blindness"

    def test_a_stamp_after_the_first_pilot_stage_fails_blindness(self, loaded: Loaded) -> None:
        with pytest.raises(PreconditionFailed) as failure:
            _check(loaded, stamp=PILOT_START + ONE_SECOND)
        assert failure.value.code == "blindness"

    def test_a_missing_stamp_fails_blindness(self, loaded: Loaded) -> None:
        with pytest.raises(PreconditionFailed) as failure:
            _check(loaded, stamp=None)
        assert failure.value.code == "blindness"

    def test_seal_mismatches_fail_the_seal(self, loaded: Loaded) -> None:
        with pytest.raises(PreconditionFailed) as failure:
            _check(loaded, inputs={"seal_mismatches": ["bindings.json"]})
        assert failure.value.code == "seal"
        assert "bindings.json" in failure.value.message

    def test_a_rebound_test_fails_the_bindings(self, loaded: Loaded) -> None:
        rebound = replace(loaded.truth, bindings={**loaded.truth.bindings, TESTS[0].nodeid: "PARK-4~1"})
        with pytest.raises(PreconditionFailed) as failure:
            _check(loaded, truth=rebound)
        assert failure.value.code == "bindings"

    def test_blindness_is_judged_before_the_seal_and_the_bindings(self, loaded: Loaded) -> None:
        rebound = replace(loaded.truth, bindings={})
        with pytest.raises(PreconditionFailed) as failure:
            _check(loaded, truth=rebound, stamp=None, inputs={"seal_mismatches": ["x"]})
        assert failure.value.code == "blindness"

    def test_the_pilot_timing_file_is_what_the_world_wrote(self, loaded: Loaded) -> None:
        timing = json.loads((loaded.world.pilot_run_dir / "timing.json").read_text(encoding="utf-8"))
        assert earliest_started(timing) == PILOT_START
