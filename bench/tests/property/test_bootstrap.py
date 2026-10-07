from __future__ import annotations

import random

import numpy as np
from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.analysis.bootstrap import (
    ClusterTable,
    Columns,
    Floats,
    bootstrap_interval,
    resample,
    safe_divide,
    wilson,
)
from specbench.analysis.estimate import AnalysisConfig
from specbench.analysis.results import analyze
from specbench.domain.records import trunk_from_record
from support.synthetic_run import synthetic_trunks

counts = st.dictionaries(
    st.text("abcdefgh", min_size=1, max_size=3),
    st.fixed_dictionaries({"hits": st.integers(0, 9), "total": st.integers(1, 9)}),
    min_size=1,
    max_size=8,
)


def hit_rate(columns: Columns) -> Floats:
    return safe_divide(columns["hits"], columns["total"])


@settings(max_examples=60, deadline=None)
@given(counts, st.randoms(use_true_random=False), st.integers(1, 10**6))
def test_interval_is_invariant_to_cluster_relabelling_and_row_order(
    clusters: dict[str, dict[str, int]], rng: random.Random, seed: int
) -> None:
    columns = ("hits", "total")
    reference = ClusterTable.build(columns, {k: {c: float(v) for c, v in row.items()} for k, row in clusters.items()})
    labels = list(clusters)
    shuffled = labels[:]
    rng.shuffle(shuffled)
    renamed = {f"repo-{index}-{label}": clusters[label] for index, label in enumerate(shuffled)}
    other = ClusterTable.build(columns, {k: {c: float(v) for c, v in row.items()} for k, row in renamed.items()})
    assert reference == other
    assert bootstrap_interval(reference, hit_rate, 200, seed) == bootstrap_interval(other, hit_rate, 200, seed)
    np.testing.assert_array_equal(resample(reference, hit_rate, 100, seed), resample(other, hit_rate, 100, seed))


@settings(max_examples=60, deadline=None)
@given(counts, st.integers(1, 10**6))
def test_a_resampled_total_is_a_sum_of_whole_clusters(clusters: dict[str, dict[str, int]], seed: int) -> None:
    table = ClusterTable.build(
        ("hits", "total"), {k: {c: float(v) for c, v in row.items()} for k, row in clusters.items()}
    )
    sums = resample(table, lambda columns: columns["total"], 50, seed)
    assert np.all(sums >= table.size * min(row[1] for row in table.rows) - 1e-9)
    assert np.all(sums <= table.size * max(row[1] for row in table.rows) + 1e-9)


@given(st.integers(0, 40), st.integers(1, 40))
def test_wilson_interval_contains_the_proportion_and_stays_in_the_unit_interval(successes: int, total: int) -> None:
    successes = min(successes, total)
    interval = wilson(successes, total)
    assert 0.0 <= interval.lo <= successes / total + 1e-12
    assert successes / total - 1e-12 <= interval.hi <= 1.0


@settings(max_examples=6, deadline=None)
@given(st.randoms(use_true_random=False))
def test_analysis_does_not_depend_on_repository_names_or_record_order(rng: random.Random) -> None:
    records = synthetic_trunks()
    names = ["parkfee", "ledger", "tzkit", "gopher", "router", "shelf"]
    fresh = list(range(len(names)))
    rng.shuffle(fresh)
    rename = {f"acme/{name}": f"org-{number}/x" for name, number in zip(names, fresh, strict=True)}
    relabelled = [{**r, "repository": rename[str(r["repository"])]} for r in records]
    rng.shuffle(relabelled)
    config = AnalysisConfig(resamples=200, seed=11)
    first = analyze([trunk_from_record(r) for r in records], config, permutations=100)
    second = analyze([trunk_from_record(r) for r in relabelled], config, permutations=100)
    assert first == second
