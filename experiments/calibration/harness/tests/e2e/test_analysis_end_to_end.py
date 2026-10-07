"""`analyze` end to end over synthetic worlds: the recorded bytes in, `decisions.json` and the result files out.

No model, no network and no server: the replies are synthetic, the product reader is a pure-Python fake, the pilot's
matrix is the working tree's own and version control and the timestamp are fakes.
"""

from __future__ import annotations

import json
import os
from collections.abc import Mapping
from dataclasses import replace
from datetime import UTC, datetime
from itertools import product
from pathlib import Path
from typing import Final, cast

import pytest

from calibration.adapters.pilot_matrix import SnapshotPilotMatrix
from calibration.analysis.bands import Band
from calibration.analysis.model import Kind
from calibration.analysis.pipeline import AnalysisOutput
from calibration.domain.canonical import JsonValue, result_json
from calibration.domain.deviations import DeviationEntry, DeviationKind
from calibration.domain.freeze import PreconditionFailed
from calibration.domain.models import MODEL_IDS
from calibration.domain.sources import SourceId
from support.synthetic_world import World, WorldSpec, build_world
from support.world_analysis import analyze_world, snapshot_matrix
from support.world_model import ORACLED_MUTANTS, UNORACLED_SOURCE, short_hash
from support.world_run import BASE_DEVIATIONS

GOLDEN_FILE: Final = Path(__file__).resolve().parents[1] / "golden" / "decisions.json"
UPDATE_GOLDEN_VARIABLE: Final = "CALIBRATION_UPDATE_GOLDEN"
BLOCKED_MODEL: Final = "M5"
PERFECT_MODEL: Final = "M6"
USELESS_MODEL: Final = "M3"
HOST: Final = "host"
SOURCE_KINDS: Final = {SourceId.JH5: tuple(Kind)[:3], SourceId.JH3: tuple(Kind)[:3], SourceId.VH2: (Kind.K4,)}
CELL_COUNT: Final = 31
LATE_STAMP: Final = datetime(2026, 9, 25, tzinfo=UTC)
FAULT_TEXT: Final = "the screen answers of the block were read with a different harness build"


def expected_cell_keys() -> set[str]:
    keys = {f"{kind.value}|{SourceId.S1.value}|{model}" for kind, model in product(Kind, MODEL_IDS)}
    for source, kinds in SOURCE_KINDS.items():
        keys.update(f"{kind.value}|{source.value}|{HOST}" for kind in kinds)
    return keys


def _cell(output: AnalysisOutput, kind: Kind, source: SourceId, model: str) -> Mapping[str, JsonValue]:
    return cast(Mapping[str, JsonValue], output.decisions["cells"])[f"{kind.value}|{source.value}|{model}"]  # type: ignore[index]


def _s1(output: AnalysisOutput, kind: Kind, model: str) -> dict[str, object]:
    return cast(dict[str, object], _cell(output, kind, SourceId.S1, model))


def _counts(cell: Mapping[str, object]) -> dict[str, object]:
    return cast(dict[str, object], cell["counts"])


def _fall_through(output: AnalysisOutput, kind: Kind, model: str) -> dict[str, int]:
    return cast(dict[str, int], _counts(_s1(output, kind, model))["fall_through"])


@pytest.fixture(scope="module")
def matrix(tmp_path_factory: pytest.TempPathFactory) -> SnapshotPilotMatrix:
    return snapshot_matrix(tmp_path_factory.mktemp("snapshot"))


@pytest.fixture(scope="module")
def base_world(tmp_path_factory: pytest.TempPathFactory) -> World:
    return build_world(tmp_path_factory.mktemp("base"))


@pytest.fixture(scope="module")
def base_output(base_world: World, matrix: SnapshotPilotMatrix) -> AnalysisOutput:
    return analyze_world(base_world, matrix)


@pytest.fixture(scope="module")
def repeated_output(tmp_path_factory: pytest.TempPathFactory, matrix: SnapshotPilotMatrix) -> AnalysisOutput:
    return analyze_world(build_world(tmp_path_factory.mktemp("repeat")), matrix)


@pytest.fixture(scope="module")
def blocked_output(tmp_path_factory: pytest.TempPathFactory, matrix: SnapshotPilotMatrix) -> AnalysisOutput:
    spec = WorldSpec(blocked=frozenset({BLOCKED_MODEL}))
    return analyze_world(build_world(tmp_path_factory.mktemp("blocked"), spec), matrix)


@pytest.fixture(scope="module")
def protocol_output(tmp_path_factory: pytest.TempPathFactory, matrix: SnapshotPilotMatrix) -> AnalysisOutput:
    protocol = DeviationEntry.create(DeviationKind.PROTOCOL, [PERFECT_MODEL], FAULT_TEXT)
    spec = WorldSpec(deviations=(*BASE_DEVIATIONS, protocol))
    return analyze_world(build_world(tmp_path_factory.mktemp("protocol"), spec), matrix)


class TestDeterminism:
    def test_two_runs_give_byte_identical_decisions(
        self, base_output: AnalysisOutput, repeated_output: AnalysisOutput
    ) -> None:
        assert result_json(base_output.decisions) == result_json(repeated_output.decisions)

    def test_two_runs_give_identical_result_files(
        self, base_output: AnalysisOutput, repeated_output: AnalysisOutput
    ) -> None:
        assert base_output.files == repeated_output.files

    def test_rerunning_over_the_same_world_changes_nothing(
        self, base_world: World, base_output: AnalysisOutput, matrix: SnapshotPilotMatrix
    ) -> None:
        again = analyze_world(base_world, matrix)
        assert result_json(again.decisions) == result_json(base_output.decisions)


class TestCells:
    def test_every_cell_of_the_band_table_appears(self, base_output: AnalysisOutput) -> None:
        cells = cast(Mapping[str, object], base_output.decisions["cells"])
        assert set(cells) == expected_cell_keys()
        assert len(cells) == CELL_COUNT

    def test_every_cell_carries_its_conclusion_and_band(self, base_output: AnalysisOutput) -> None:
        for outcome in base_output.cell_outcomes.values():
            assert outcome.record["conclusion"]
            assert outcome.record["band"] == outcome.verdict.band.value

    def test_the_run_has_no_new_deviation(self, base_output: AnalysisOutput) -> None:
        assert base_output.new_deviations == ()

    def test_the_server_formula_and_regression_checks_find_nothing(self, base_output: AnalysisOutput) -> None:
        per_model = cast(
            dict[str, dict[str, object]], cast(dict[str, object], base_output.decisions["checks"])["per_model"]
        )
        for model_id in MODEL_IDS:
            assert per_model[model_id]["formula_findings"] == []
            assert cast(dict[str, object], per_model[model_id]["screen_regression"])["failed"] is False

    def test_each_fault_of_the_world_is_a_typed_fall_through(self, base_output: AnalysisOutput) -> None:
        assert _fall_through(base_output, Kind.K4, "M1")["MISSING"] == 2 * 4
        assert _fall_through(base_output, Kind.K4, "M2")["RELAY_REFUSED"] == 4
        assert _fall_through(base_output, Kind.K4, "M3")["STATE_TOO_LARGE"] == 2 * 4
        assert _fall_through(base_output, Kind.K4, "M4")["STATE_TOO_LARGE"] == 4
        assert _fall_through(base_output, Kind.K1, "M4")["MISSING"] == 1
        assert _fall_through(base_output, Kind.K1, "M5") == {"MISSING": 1, "STATE_TOO_LARGE": 1, "RELAY_REFUSED": 0}

    def test_the_mutant_without_an_oracle_is_excluded_and_counted(self, base_output: AnalysisOutput) -> None:
        excluded = cast(dict[str, object], base_output.decisions["excluded"])
        assert excluded["without_oracle"] == [short_hash("P", UNORACLED_SOURCE)]
        assert _counts(_s1(base_output, Kind.K4, PERFECT_MODEL))["units"] == 4 * len(ORACLED_MUTANTS)

    def test_a_cell_with_too_many_fall_throughs_is_inconclusive_for_data(self, base_output: AnalysisOutput) -> None:
        assert _s1(base_output, Kind.K1, BLOCKED_MODEL)["band"] == Band.INCONCLUSIVE_DATA.value

    def test_the_perfect_model_reaches_threshold_ready_on_k3(self, base_output: AnalysisOutput) -> None:
        assert _s1(base_output, Kind.K3, PERFECT_MODEL)["band"] == Band.THRESHOLD_READY.value

    def test_the_anti_calibrated_model_is_useless_on_k4(self, base_output: AnalysisOutput) -> None:
        assert _s1(base_output, Kind.K4, USELESS_MODEL)["band"] == Band.USELESS.value

    def test_every_admission_test_of_the_small_world_is_unreachable(self, base_output: AnalysisOutput) -> None:
        outcomes = {str(outcome.record["admission_outcome"]) for outcome in base_output.cell_outcomes.values()}
        assert outcomes == {"UNREACHABLE"}


class TestBlockedModel:
    def test_the_blocked_models_cells_are_blocked(self, blocked_output: AnalysisOutput) -> None:
        for kind in Kind:
            cell = _s1(blocked_output, kind, BLOCKED_MODEL)
            assert cell["band"] == Band.BLOCKED.value
            assert cell["metrics"] is None

    def test_the_other_cells_are_unaffected(self, base_output: AnalysisOutput, blocked_output: AnalysisOutput) -> None:
        base_cells = cast(dict[str, object], base_output.decisions["cells"])
        blocked_cells = cast(dict[str, object], blocked_output.decisions["cells"])
        untouched = {key for key in base_cells if f"|{BLOCKED_MODEL}" not in key}
        assert len(untouched) == CELL_COUNT - len(Kind)
        for key in untouched:
            assert blocked_cells[key] == base_cells[key], key


class TestProtocolDeviation:
    def test_a_threshold_ready_cell_in_scope_becomes_inconclusive(
        self, base_output: AnalysisOutput, protocol_output: AnalysisOutput
    ) -> None:
        assert _s1(base_output, Kind.K3, PERFECT_MODEL)["band"] == Band.THRESHOLD_READY.value
        capped = _s1(protocol_output, Kind.K3, PERFECT_MODEL)
        assert capped["band"] == Band.INCONCLUSIVE.value
        assert capped["capped_by"] == ["protocol"]

    def test_cells_out_of_scope_keep_their_band(
        self, base_output: AnalysisOutput, protocol_output: AnalysisOutput
    ) -> None:
        for model in MODEL_IDS:
            if model != PERFECT_MODEL:
                assert _s1(protocol_output, Kind.K3, model) == _s1(base_output, Kind.K3, model)


class TestBlindness:
    def test_a_stamp_after_the_pilot_analysis_commit_stops_the_analysis(
        self, base_world: World, matrix: SnapshotPilotMatrix
    ) -> None:
        with pytest.raises(PreconditionFailed) as stopped:
            analyze_world(base_world, matrix, generated_at=LATE_STAMP)
        assert stopped.value.code == "blindness"


class TestGoldenFile:
    def test_decisions_json_equals_the_golden_file(self, base_output: AnalysisOutput) -> None:
        text = result_json(base_output.decisions)
        if os.environ.get(UPDATE_GOLDEN_VARIABLE):
            GOLDEN_FILE.parent.mkdir(parents=True, exist_ok=True)
            GOLDEN_FILE.write_text(text, encoding="utf-8")
        assert GOLDEN_FILE.is_file(), f"no golden file: run once with {UPDATE_GOLDEN_VARIABLE}=1"
        assert text == GOLDEN_FILE.read_text(encoding="utf-8")

    def test_the_golden_file_is_valid_json_with_every_cell(self) -> None:
        document = json.loads(GOLDEN_FILE.read_text(encoding="utf-8"))
        assert set(document["cells"]) == expected_cell_keys()


def test_the_world_spec_is_a_frozen_value() -> None:
    spec = WorldSpec()
    assert replace(spec, screen_blocked=True).screen_blocked is True
