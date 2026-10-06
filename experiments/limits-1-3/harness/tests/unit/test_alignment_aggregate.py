from __future__ import annotations

from hypothesis import given
from hypothesis import strategies as st

from pilot.domain.alignment_aggregate import AlignmentVerdict, Repetition, aggregate_alignment

D = "PARK-3~1"


def reps(*triples):
    return [Repetition(ref, coverage, confidence) for ref, coverage, confidence in triples]


def test_unanimous_declared_full_is_aligned():
    result = aggregate_alignment(reps(*[(D, "FULL", 80)] * 5), D)
    assert (result.modal_ref, result.modal_share, result.modal_coverage) == (D, 1.0, "FULL")
    assert result.verdict is AlignmentVerdict.ALIGNED and result.verdict_s1 is AlignmentVerdict.ALIGNED
    assert result.mean_confidence == 80 and result.missing_reps == 0


def test_partial_is_aligned_but_undetermined_under_sensitivity_s1():
    result = aggregate_alignment(reps(*[(D, "PARTIAL", 70)] * 5), D)
    assert result.verdict is AlignmentVerdict.ALIGNED and result.verdict_s1 is AlignmentVerdict.UNDETERMINED


def test_weak_coverage_on_the_declared_criterion_is_misaligned():
    assert aggregate_alignment(reps(*[(D, "WEAK", 60)] * 5), D).verdict is AlignmentVerdict.MISALIGNED


def test_another_modal_criterion_or_none_is_misaligned():
    assert aggregate_alignment(reps(*[("PARK-5~1", "FULL", 90)] * 5), D).verdict is AlignmentVerdict.MISALIGNED
    result = aggregate_alignment(reps(*[("NONE", "NONE", 90)] * 5), D)
    assert result.verdict is AlignmentVerdict.MISALIGNED and result.modal_coverage == "NONE"


def test_a_share_below_four_fifths_is_undetermined():
    result = aggregate_alignment(reps((D, "FULL", 90), (D, "FULL", 90), (D, "FULL", 90), ("PARK-1~1", "FULL", 50), ("PARK-2~1", "FULL", 50)), D)
    assert result.modal_share == 0.6 and result.verdict is AlignmentVerdict.UNDETERMINED


def test_four_of_five_meets_the_threshold():
    result = aggregate_alignment(reps(*[(D, "FULL", 90)] * 4, ("PARK-1~1", "FULL", 50)), D)
    assert result.modal_share == 0.8 and result.verdict is AlignmentVerdict.ALIGNED


def test_missing_repetitions_lower_the_share_over_the_five_planned():
    result = aggregate_alignment(reps(*[(D, "FULL", 90)] * 3), D)
    assert result.modal_share == 0.6 and result.missing_reps == 2 and result.verdict is AlignmentVerdict.UNDETERMINED


def test_no_answered_repetition_is_undetermined():
    result = aggregate_alignment([], D)
    assert result.modal_ref is None and result.verdict is AlignmentVerdict.UNDETERMINED and result.missing_reps == 5


def test_modal_ties_go_to_the_higher_mean_confidence_then_lexicographic_order():
    higher = aggregate_alignment(reps(("PARK-1~1", "FULL", 60), ("PARK-1~1", "FULL", 60), ("PARK-2~1", "FULL", 90), ("PARK-2~1", "FULL", 90)), D)
    assert higher.modal_ref == "PARK-2~1"
    lexical = aggregate_alignment(reps(("PARK-2~1", "FULL", 70), ("PARK-1~1", "FULL", 70)), D)
    assert lexical.modal_ref == "PARK-1~1"


def test_modal_coverage_counts_only_repetitions_on_the_modal_criterion():
    result = aggregate_alignment(reps((D, "FULL", 80), (D, "FULL", 80), (D, "WEAK", 80), (D, "WEAK", 80), ("PARK-1~1", "WEAK", 80), ), D)
    assert result.modal_coverage == "FULL"


@given(st.lists(st.tuples(st.sampled_from([D, "PARK-1~1", "NONE"]), st.sampled_from(["FULL", "PARTIAL", "WEAK"]), st.integers(0, 100)), min_size=1, max_size=5))
def test_the_modal_share_is_the_modal_count_over_five(items):
    result = aggregate_alignment([Repetition(*i) for i in items], D)
    counts = {ref: sum(1 for i in items if i[0] == ref) for ref, _, _ in items}
    assert result.modal_share == max(counts.values()) / 5
    assert result.missing_reps == 5 - len(items)
