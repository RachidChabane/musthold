from __future__ import annotations

import math
from typing import Any

import pytest

from specbench.analysis import blind_spot, calibration, composition, judge, outcomes
from specbench.analysis.bootstrap import ClusterTable, point_estimate, wilson
from specbench.analysis.estimate import AnalysisConfig, estimate
from specbench.analysis.outcomes import Mode
from specbench.analysis.results import analyze, build_table
from specbench.analysis.selection_null import selection_null
from specbench.domain.arms import FROZEN_MODEL, Arm, Harness, Module
from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import RecordError
from specbench.domain.records import ConfidenceSource, MutantSource, TrunkRecord, Verdict, Writer, trunk_from_record

CLAUDE = Harness.CLAUDE_CODE
H = CLAUDE.value
CONFIG = AnalysisConfig(resamples=500, seed=1)


def tree(value: object) -> Any:
    return value


def truth(defective: bool, full: bool | None = None, o_failed: bool = False) -> dict[str, JsonValue]:
    return {"defective": defective, "defective_full": defective if full is None else full, "o_failed": o_failed}


def arm(
    verdict: str,
    selection_pass: bool | None,
    final: dict[str, JsonValue] | None,
    escalated: bool = False,
    calls: int = 10,
    seconds: int = 100,
) -> dict[str, JsonValue]:
    return {
        "escalated_during": escalated,
        "final": final,
        "model": FROZEN_MODEL[CLAUDE],
        "model_calls": calls,
        "seconds": seconds,
        "selection_pass": selection_pass,
        "verdict": verdict,
    }


def trunk(
    index: int, repository: str, s_t_defective: bool, arms: dict[str, JsonValue], **extra: JsonValue
) -> TrunkRecord:
    record: dict[str, JsonValue] = {
        "arms": arms,
        "harness": H,
        "language": "python",
        "model": FROZEN_MODEL[CLAUDE],
        "reached_s_t": True,
        "repository": repository,
        "s_n": truth(s_t_defective),
        "s_t": truth(s_t_defective),
        "schema": "specbench.trunk/1",
        "task_id": f"T{index}",
    }
    return trunk_from_record({**record, **extra})


def four_trunks() -> list[TrunkRecord]:
    """Defective at S_T: T1, T2. ALL at S_T passes T2, T3, T4; in the loop T1 escalates with a defect, T2 is repaired."""
    t_arm = {"T": arm("pass", True, truth(True))}
    clean_t = {"T": arm("pass", True, truth(False))}
    return [
        trunk(
            1, "r1", True, {**t_arm, "ALL": arm("escalate", False, truth(True), escalated=True, calls=30, seconds=300)}
        ),
        trunk(2, "r1", True, {**t_arm, "ALL": arm("pass", True, truth(False), calls=20, seconds=200)}),
        trunk(3, "r2", False, {**clean_t, "ALL": arm("pass", True, truth(False))}),
        trunk(4, "r2", False, {**clean_t, "ALL": arm("pass", True, truth(False))}),
    ]


def statistic_value(trunks: list[TrunkRecord], statistic: outcomes.Statistic) -> float:
    return point_estimate(build_table(trunks), statistic)


def test_x_in_the_loop_standard_lenient_and_strict() -> None:
    trunks = four_trunks()
    assert statistic_value(trunks, outcomes.x_loop(H, Arm.ALL, Mode.STANDARD)) == pytest.approx(1.0)
    assert statistic_value(trunks, outcomes.x_loop(H, Arm.ALL, Mode.LENIENT)) == pytest.approx(0.5)
    assert statistic_value(trunks, outcomes.x_loop(H, Arm.ALL, Mode.STRICT)) == pytest.approx(1.0)
    assert statistic_value(trunks, outcomes.x_loop(H, Arm.T, Mode.STANDARD)) == pytest.approx(0.0)


def test_selection_estimand_sensitivity_and_specificity() -> None:
    trunks = four_trunks()
    assert statistic_value(trunks, outcomes.x_selection(H, Arm.ALL)) == pytest.approx(1 - (1 / 3) / 0.5)
    assert statistic_value(trunks, outcomes.sensitivity(H, Arm.ALL)) == pytest.approx(0.5)
    assert statistic_value(trunks, outcomes.specificity(H, Arm.ALL)) == pytest.approx(1.0)


def test_yield_false_rejection_escaped_defects_and_o_rate() -> None:
    trunks = four_trunks()
    assert statistic_value(trunks, outcomes.yield_rate(H, Arm.ALL, Mode.STANDARD)) == pytest.approx(0.75)
    assert statistic_value(trunks, outcomes.false_rejection(H, Arm.ALL, Mode.STANDARD)) == pytest.approx(0.0)
    assert statistic_value(trunks, outcomes.escaped_defects(H, Arm.ALL, Mode.LENIENT)) == pytest.approx(0.25)
    assert statistic_value(trunks, outcomes.accepted_o_rate(H, Arm.ALL, Mode.STANDARD)) == pytest.approx(0.0)


def test_cost_per_accepted_change_includes_repair_work() -> None:
    trunks = four_trunks()
    calls = statistic_value(trunks, outcomes.cost_per_accepted(H, Arm.ALL, Mode.STANDARD, "calls"))
    assert calls == pytest.approx((30 + 20 + 10 + 10) / 3)


def test_model_truth_inflation_is_the_full_truth_minus_the_primary() -> None:
    clean_t = {"T": arm("pass", True, truth(False))}
    trunks = [
        trunk(1, "r1", True, {"T": arm("pass", True, truth(True)), "ALL": arm("pass", True, truth(False, full=True))}),
        trunk(2, "r2", False, {**clean_t, "ALL": arm("pass", True, truth(False))}),
    ]
    primary = outcomes.x_loop(H, Arm.ALL, Mode.STANDARD)
    full = outcomes.x_loop(H, Arm.ALL, Mode.STANDARD, full_truth=True)
    assert statistic_value(trunks, primary) == pytest.approx(1.0)
    assert statistic_value(trunks, full) == pytest.approx(1 - 0.5 / 0.5)


def test_an_undefined_baseline_gives_no_estimate_instead_of_an_error() -> None:
    clean = [
        trunk(i, f"r{i}", False, {"T": arm("pass", True, truth(False)), "ALL": arm("pass", True, truth(False))})
        for i in (1, 2)
    ]
    value = statistic_value(clean, outcomes.x_loop(H, Arm.ALL, Mode.STANDARD))
    assert math.isnan(value)
    results = analyze(clean, CONFIG, permutations=50)
    assert tree(results)["harnesses"][H]["arms"]["ALL"]["x_loop"]["std"]["estimate"] is None


def test_accepting_without_a_final_truth_is_refused() -> None:
    broken = trunk(1, "r1", True, {"T": arm("pass", True, None)})
    with pytest.raises(ValueError, match="final truth"):
        build_table([broken])


def test_trunks_that_never_reached_s_t_count_only_for_n() -> None:
    unreached = trunk_from_record(
        {
            "arms": {},
            "harness": H,
            "language": "python",
            "model": FROZEN_MODEL[CLAUDE],
            "reached_s_t": False,
            "repository": "r9",
            "s_n": truth(True),
            "s_t": None,
            "schema": "specbench.trunk/1",
            "task_id": "T9",
        }
    )
    results = analyze([*four_trunks(), unreached], CONFIG, permutations=50)
    trunks = tree(results)["harnesses"][H]["trunks"]
    assert trunks["total"] == 5 and trunks["reached_s_t"] == 4 and trunks["unreached"] == 1
    assert trunks["s_n_defective"]["successes"] == 3


def test_an_arm_result_keeps_the_local_readout_apart_from_the_verdict_acceptance_reads() -> None:
    record = trunk(1, "r1", True, {"AL": arm("escalate", True, truth(True)) | {"local_verdict": "pass"}})
    result = record.arms[Arm.AL]
    assert result.verdict is Verdict.ESCALATE
    assert result.local_verdict is Verdict.PASS
    assert result.accepted() is False
    assert result.accepted(lenient=True) is True
    assert trunk(1, "r1", True, {"AL": arm("pass", True, truth(True))}).arms[Arm.AL].local_verdict is None


def test_a_local_verdict_outside_the_verdicts_is_refused() -> None:
    with pytest.raises(RecordError):
        trunk(1, "r1", True, {"AL": arm("escalate", True, truth(True)) | {"local_verdict": "maybe"}})


def test_trunk_records_refuse_a_model_that_is_not_frozen() -> None:
    with pytest.raises(RecordError):
        trunk_from_record({**_raw(), "model": "claude-opus-5-5"})
    with pytest.raises(RecordError):
        trunk_from_record({**_raw(), "arms": {"T": arm("pass", True, truth(False)) | {"model": "claude-haiku-4-5"}}})
    with pytest.raises(RecordError):
        trunk_from_record({**_raw(), "reached_s_t": False})


def test_each_arm_reports_its_cost_per_trunk_and_the_denominators_of_its_rates() -> None:
    trunks = [
        trunk(
            1,
            "r1",
            True,
            {
                "T": arm("pass", True, truth(True), calls=0, seconds=60),
                "ALL": arm("blocked", False, truth(False), calls=30, seconds=900),
            },
        ),
        trunk(
            2,
            "r2",
            False,
            {
                "T": arm("pass", True, truth(False), calls=0, seconds=120),
                "ALL": arm("pass", True, truth(False), calls=10, seconds=300),
            },
        ),
    ]
    results = tree(analyze(trunks, CONFIG, permutations=50))
    section = results["harnesses"][H]
    all_arm = section["arms"]["ALL"]
    assert all_arm["calls_per_trunk"]["estimate"] == pytest.approx(20.0)
    assert all_arm["seconds_per_trunk"]["estimate"] == pytest.approx(600.0)
    assert (all_arm["clean_trunks"], all_arm["defective_trunks"]) == (1, 1)
    assert section["repositories"] == 2
    assert results["method"]["permutations"] == 50
    assert results["method"]["level"] == pytest.approx(0.95)


def _raw() -> dict[str, JsonValue]:
    return {
        "arms": {},
        "harness": H,
        "language": "go",
        "model": FROZEN_MODEL[CLAUDE],
        "reached_s_t": True,
        "repository": "r1",
        "s_n": truth(False),
        "s_t": truth(False),
        "schema": "specbench.trunk/1",
        "task_id": "T1",
    }


def cell(criterion: str, writer: str, source: str, escaped: int, evaluable: int) -> dict[str, JsonValue]:
    return {"criterion": criterion, "writer": writer, "source": source, "escaped": escaped, "evaluable": evaluable}


def blind_spot_trunk(index: int, repository: str, rates: dict[tuple[str, str], tuple[int, int]]) -> TrunkRecord:
    cells: list[JsonValue] = [cell(f"T{index}-1", w, s, e, n) for (w, s), (e, n) in rates.items()]
    return trunk(index, repository, False, {}, blind_spot=cells)


def test_sb_coder_is_the_contrast_of_contrasts_with_task_averaged_rates() -> None:
    rates = {
        ("coder", "generator"): (3, 4),
        ("coder", "operator"): (1, 4),
        ("upstream", "generator"): (1, 4),
        ("upstream", "operator"): (1, 4),
    }
    first = blind_spot_trunk(1, "r1", rates)
    second = blind_spot_trunk(2, "r2", dict.fromkeys(rates, (0, 4)))
    trunks = [first, second]
    # task 1: [(.75 - .25) - (.25 - .25)] = .5; task 2: 0; mean .25
    assert statistic_value(trunks, blind_spot.sb_coder(H, MutantSource.GENERATOR)) == pytest.approx(0.25)
    assert statistic_value(
        trunks, blind_spot.cell_rate(H, MutantSource.GENERATOR, Writer.CODER, MutantSource.GENERATOR)
    ) == pytest.approx(0.375)


def test_sb_coder_uses_only_tasks_where_all_four_cells_are_evaluable() -> None:
    full = {
        ("coder", "generator"): (2, 4),
        ("coder", "operator"): (1, 4),
        ("upstream", "generator"): (0, 4),
        ("upstream", "operator"): (0, 4),
    }
    partial = {**full, ("upstream", "operator"): (0, 0)}
    trunks = [blind_spot_trunk(1, "r1", full), blind_spot_trunk(2, "r2", partial)]
    table = build_table(trunks)
    assert estimate(table, blind_spot.sb_coder(H, MutantSource.GENERATOR), CONFIG).value == pytest.approx(0.25)
    assert math.isnan(point_estimate(table, blind_spot.sb_coder(H, MutantSource.OPUS)))


def test_judge_deltas_are_paired_by_snapshot() -> None:
    def snapshot(index: int, defective: bool, same: bool, replay: bool) -> TrunkRecord:
        return trunk(
            index,
            f"r{index}",
            defective,
            {},
            judge_replay={"replay_model": "claude-opus-5-5", "same_pass": same, "replay_pass": replay},
        )

    trunks = [
        snapshot(1, True, True, False),
        snapshot(2, True, False, False),
        snapshot(3, False, True, True),
        snapshot(4, False, False, True),
    ]
    same, replay = judge.Judge.SAME_MODEL, judge.Judge.REPLAY
    assert statistic_value(trunks, judge.sensitivity(H, same)) == pytest.approx(0.5)
    assert statistic_value(trunks, judge.sensitivity(H, replay)) == pytest.approx(1.0)
    assert statistic_value(trunks, judge.specificity(H, same)) == pytest.approx(0.5)
    assert statistic_value(trunks, judge.false_rejection(H, replay)) == pytest.approx(0.0)
    delta = outcomes.difference(judge.x_selection(H, replay), judge.x_selection(H, same))
    # same passes {T1, T3}: defect .5 -> X 0; replay passes {T3, T4}: defect 0 -> X 1
    assert statistic_value(trunks, delta) == pytest.approx(1.0)


def decision(source: str, permille: int | None, truth_value: bool | None) -> dict[str, JsonValue]:
    return {"source": source, "confidence_permille": permille, "truth": truth_value}


def test_calibration_metrics_per_source() -> None:
    decisions: list[JsonValue] = [
        decision("endpoint", 900, True),
        decision("endpoint", 900, True),
        decision("endpoint", 100, False),
        decision("endpoint", 600, False),
        decision("host-votes", 1000, True),
        decision("undetermined", None, None),
    ]
    trunks = [trunk(1, "r1", False, {}, decisions=decisions)]
    source = ConfidenceSource.ENDPOINT
    assert statistic_value(trunks, calibration.brier(H, source)) == pytest.approx((0.01 + 0.01 + 0.01 + 0.36) / 4)
    # bin 9: p .9,.9 truth 1,1 -> .2; bin 1: p .1 truth 0 -> .1; bin 6: p .6 truth 0 -> .6 ; total .9 / 4
    assert statistic_value(trunks, calibration.ece(H, source)) == pytest.approx(0.9 / 4)
    # confidence .9, .9, .9 (for p=.1) right; .6 wrong: the top bins hold 3 right answers: coverage 3/4
    assert statistic_value(trunks, calibration.coverage_at_error(H, source)) == pytest.approx(0.75)
    assert statistic_value(trunks, calibration.undetermined_rate(H)) == pytest.approx(1 / 6)
    assert statistic_value(trunks, calibration.coverage_at_error(H, ConfidenceSource.HOST_VOTES)) == pytest.approx(1.0)


def test_composition_mismatches_and_absent_module_probes_are_counted_from_recorded_verdicts() -> None:
    rows: list[JsonValue] = [
        {"criterion": "T1-1", "module": "alignment", "all_verdict": "pass", "composed_verdict": "pass"},
        {"criterion": "T1-2", "module": "alignment", "all_verdict": "pass", "composed_verdict": "escalate"},
        {"criterion": "T1-1", "module": "discriminating-power", "all_verdict": "block", "composed_verdict": "block"},
    ]
    probes: list[JsonValue] = [{"arm": "TR", "module": "alignment", "counted_as_passed": True}]
    summary = composition.composition_summary(
        [trunk(1, "r1", False, {}, composition=rows, absent_probes=probes)], CLAUDE
    )
    assert [(s.module, s.criteria, s.mismatches) for s in summary] == [
        (Module.ALIGNMENT, 2, 1),
        (Module.DISCRIMINATING_POWER, 1, 0),
    ]
    absent = composition.absent_module_summary([trunk(1, "r1", False, {}, absent_probes=probes)], CLAUDE)
    assert (absent[0].probes, absent[0].counted_as_passed) == (1, 1)


def test_selection_null_is_centred_on_zero_and_reproducible() -> None:
    trunks = [
        trunk(i, f"r{i}", defective, {"ALL": arm("pass", i % 2 == 0, truth(defective))})
        for i, defective in enumerate([True, False, True, False, True, False, False, True], start=1)
    ]
    first = selection_null(trunks, CLAUDE, Arm.ALL, 2000, 3)
    assert first == selection_null(trunks, CLAUDE, Arm.ALL, 2000, 3)
    assert abs(first.mean) < 0.15 and first.lo < first.mean < first.hi


def test_cluster_table_orders_rows_by_content() -> None:
    a = ClusterTable.build(["x", "y"], {"b": {"x": 2.0}, "a": {"x": 1.0, "y": 3.0}})
    b = ClusterTable.build(["x", "y"], {"zzz": {"x": 1.0, "y": 3.0}, "aaa": {"x": 2.0}})
    assert a == b


def test_wilson_interval() -> None:
    interval = wilson(0, 6)
    assert interval.lo == pytest.approx(0.0, abs=1e-9) and interval.hi == pytest.approx(0.390, abs=0.005)
    assert (wilson(0, 0).lo, wilson(0, 0).hi) == (0.0, 1.0)
    assert wilson(6, 6).hi == pytest.approx(1.0)
