"""The admission test of one cell (PREREG section 10): unreachable, inconclusive and the K4 variants."""

from __future__ import annotations

from typing import Final

import pytest

from calibration.analysis.admission_cell import cell_admission, item_set_could_supply
from calibration.analysis.model import CellData, Kind
from calibration.analysis.stats.admission import MIN_FIT_CLUSTERS, MIN_FIT_DECISIONS, AdmissionOutcome
from support.metric_cases import SETTINGS, event_decision, make_cell, mixed_event_cell, mixed_top_label_cell

MANY_CLUSTERS: Final = 20
UNITS_PER_CLUSTER: Final = 25
VARIANTS: Final = ("bootstrap_upper", "predicting_violates", "predicting_consistent", "majority_rule")


def _cluster_names(count: int) -> list[str]:
    return [f"c{index:02d}" for index in range(count)]


def _wide_unit_clusters(per_cluster: int) -> list[str]:
    return [name for name in _cluster_names(MANY_CLUSTERS) for _ in range(per_cluster)]


def _sparse_cell(kind: Kind, per_cluster: int) -> CellData:
    names = _cluster_names(MANY_CLUSTERS)
    decisions = [
        event_decision(index, name, 0.9 if index % 2 else 0.1, truth=bool(index % 2))
        for index, name in enumerate(names)
    ]
    return make_cell(kind, decisions, unit_clusters=_wide_unit_clusters(per_cluster))


@pytest.mark.parametrize("kind", [Kind.K1, Kind.K2, Kind.K3])
def test_alignment_kinds_are_unreachable_on_a_small_item_set(kind: Kind) -> None:
    cell = mixed_top_label_cell() if kind is Kind.K1 else mixed_event_cell(kind)
    record, outcome = cell_admission(cell, SETTINGS)
    assert outcome is AdmissionOutcome.UNREACHABLE
    assert record["item_set_could_supply"] is False
    assert "variants" not in record


def test_a_small_item_set_cannot_supply_the_fit_half() -> None:
    assert not item_set_could_supply(mixed_event_cell(Kind.K3))


def test_the_item_set_can_supply_the_fit_half_from_the_minimum() -> None:
    assert MIN_FIT_CLUSTERS == 10
    assert item_set_could_supply(_sparse_cell(Kind.K3, UNITS_PER_CLUSTER))
    assert MIN_FIT_DECISIONS == 200


def test_an_item_set_just_short_of_the_minimum_cannot_supply() -> None:
    assert not item_set_could_supply(_sparse_cell(Kind.K3, 19))


def test_missing_data_make_the_outcome_inconclusive_when_the_items_could_supply() -> None:
    record, outcome = cell_admission(_sparse_cell(Kind.K3, UNITS_PER_CLUSTER), SETTINGS)
    assert outcome is AdmissionOutcome.INCONCLUSIVE
    assert record["item_set_could_supply"] is True
    main = record["main"]
    assert isinstance(main, dict)
    assert main["outcome"] == "INCONCLUSIVE"


def test_k4_reports_its_variants_beside_the_main_test() -> None:
    record, outcome = cell_admission(mixed_event_cell(Kind.K4), SETTINGS)
    assert outcome is AdmissionOutcome.UNREACHABLE
    variants = record["variants"]
    assert isinstance(variants, dict)
    assert tuple(variants) == VARIANTS


def test_each_k4_variant_is_a_test_record_except_the_majority_rule() -> None:
    record, _ = cell_admission(mixed_event_cell(Kind.K4), SETTINGS)
    variants = record["variants"]
    assert isinstance(variants, dict)
    for name in VARIANTS[:3]:
        variant = variants[name]
        assert isinstance(variant, dict)
        assert variant["outcome"] == "UNREACHABLE"
    majority = variants["majority_rule"]
    assert isinstance(majority, dict)
    assert set(majority) == {"errors", "n", "rate"}


def test_the_main_record_counts_the_fit_and_held_out_halves() -> None:
    record, _ = cell_admission(mixed_event_cell(Kind.K3), SETTINGS)
    main = record["main"]
    assert isinstance(main, dict)
    assert sum(int(str(main[key])) for key in ("fit_decisions", "held_out_decisions")) == 12
    assert sum(int(str(main[key])) for key in ("fit_clusters", "held_out_clusters")) == 4
