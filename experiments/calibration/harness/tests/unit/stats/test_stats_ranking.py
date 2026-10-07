from __future__ import annotations

import math

import pytest

from calibration.analysis.stats.ranking import auroc, average_ranks, cohen_kappa, spearman
from stats_arrays import flags, floats


def _naive_average_ranks(values: list[float]) -> list[float]:
    return [
        sum(1 + sum(other < value for other in values) + k for k in range(sum(other == value for other in values)))
        / sum(other == value for other in values)
        for value in values
    ]


def _naive_spearman(a: list[float], b: list[float]) -> float:
    ra, rb = _naive_average_ranks(a), _naive_average_ranks(b)
    mean_a, mean_b = sum(ra) / len(ra), sum(rb) / len(rb)
    covariance = sum((x - mean_a) * (y - mean_b) for x, y in zip(ra, rb, strict=True))
    spread = math.sqrt(sum((x - mean_a) ** 2 for x in ra) * sum((y - mean_b) ** 2 for y in rb))
    return covariance / spread


def test_average_ranks_share_the_rank_of_ties() -> None:
    assert average_ranks(floats([10, 20, 20, 30, 20])).tolist() == [1.0, 3.0, 3.0, 5.0, 3.0]
    assert average_ranks(floats([])).tolist() == []


def test_auroc_of_the_classic_example() -> None:
    assert auroc(floats([0.1, 0.4, 0.35, 0.8]), flags([0, 0, 1, 1])) == 0.75


def test_auroc_counts_ties_as_one_half() -> None:
    assert auroc(floats([0.5, 0.5, 0.5, 0.5]), flags([0, 1, 0, 1])) == 0.5
    assert auroc(floats([0.2, 0.5, 0.5, 0.9]), flags([0, 0, 1, 1])) == 0.875


def test_auroc_of_a_perfect_and_an_inverted_ranking() -> None:
    assert auroc(floats([0.1, 0.9]), flags([0, 1])) == 1.0
    assert auroc(floats([0.9, 0.1]), flags([0, 1])) == 0.0


@pytest.mark.parametrize("truth", [[1, 1, 1], [0, 0, 0], []])
def test_auroc_with_a_single_class_or_no_data_is_undefined(truth: list[int]) -> None:
    assert math.isnan(auroc(floats([0.3] * len(truth)), flags(truth)))


def test_spearman_of_monotone_series() -> None:
    assert spearman(floats([1, 2, 3, 4]), floats([10, 20, 30, 40])) == pytest.approx(1.0)
    assert spearman(floats([1, 2, 3, 4]), floats([4, 3, 2, 1])) == pytest.approx(-1.0)


def test_spearman_matches_a_naive_reference_with_ties() -> None:
    a = [3.0, 1.0, 4.0, 1.0, 5.0, 9.0, 2.0, 6.0, 5.0, 3.0]
    b = [2.0, 7.0, 1.0, 8.0, 2.0, 8.0, 1.0, 8.0, 2.0, 8.0]
    assert spearman(floats(a), floats(b)) == pytest.approx(_naive_spearman(a, b), abs=1e-12)


def test_spearman_is_undefined_for_constant_or_tiny_input() -> None:
    assert math.isnan(spearman(floats([1, 1, 1]), floats([1, 2, 3])))
    assert math.isnan(spearman(floats([1]), floats([1])))
    assert math.isnan(spearman(floats([]), floats([])))


def test_kappa_of_the_classic_example() -> None:
    assert cohen_kappa(((20, 5), (10, 15))) == pytest.approx(0.4)


def test_kappa_of_perfect_agreement_and_of_chance() -> None:
    assert cohen_kappa(((10, 0), (0, 10))) == 1.0
    assert cohen_kappa(((5, 5), (5, 5))) == 0.0


def test_kappa_is_undefined_without_data_or_without_variation() -> None:
    assert math.isnan(cohen_kappa(((0, 0), (0, 0))))
    assert math.isnan(cohen_kappa(((7, 0), (0, 0))))
