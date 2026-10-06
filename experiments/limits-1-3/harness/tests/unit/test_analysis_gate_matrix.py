"""Matrix classifier (PREREG 8)."""

from __future__ import annotations

import pytest
from hypothesis import given, strategies as st

from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.matrix import (
    AMBIGUOUS_LABEL,
    NO_LABEL,
    PoolMutant,
    Signature,
    build_evidence,
    classify_test,
    classify_tests,
    mean_dc,
)

A, B, C = "tests/t.py::a", "tests/t.py::b", "tests/t.py::c"
ALIGNED, MISALIGNED, UNDETERMINED = AlignmentVerdict.ALIGNED, AlignmentVerdict.MISALIGNED, AlignmentVerdict.UNDETERMINED


# ----------------------------------------------------------------------------- matrix


def make(pool, unattributed=()):
    return build_evidence([A, B, C], [PoolMutant(c, frozenset(k)) for c, k in pool], [frozenset(k) for k in unattributed])


def sig(pool, test, declared, unattributed=()):
    return classify_test(make(pool, unattributed)[test], declared)


def test_signature_undefined_own_when_the_declared_criterion_has_no_valid_mutant():
    assert sig([(2, {A})], A, declared=1).signature is Signature.UNDEFINED_OWN


def test_signature_inert_when_the_test_kills_no_valid_or_unattributed_mutant():
    assert sig([(1, {B}), (2, {B})], A, declared=1).signature is Signature.INERT


def test_a_test_killing_only_unattributed_mutants_is_off_target_not_inert():
    result = sig([(1, {B})], A, declared=1, unattributed=[{A}])
    assert result.signature is Signature.OFF_TARGET and result.none == 1.0


def test_signature_off_target_when_it_kills_only_other_criteria():
    assert sig([(1, {B}), (2, {A})], A, declared=1).signature is Signature.OFF_TARGET


def test_signature_partial_below_theta_own():
    pool = [(1, {A}), (1, {B}), (1, {B}), (1, {B})]
    assert sig(pool, A, declared=1).signature is Signature.PARTIAL


def test_signature_specific_at_theta_own_with_low_other():
    pool = [(1, {A}), (1, {B}), (2, {B})]
    result = sig(pool, A, declared=1)
    assert result.signature is Signature.SPECIFIC and result.own == 0.5 and result.other == 0.0


def test_signature_overbroad_when_other_exceeds_theta_other():
    pool = [(1, {A}), (2, {A}), (2, {B})]
    result = sig(pool, A, declared=1)
    assert result.other == 0.5 and result.signature is Signature.OVERBROAD


def test_other_exactly_at_theta_other_is_still_specific():
    pool = [(1, {A}), (2, {A}), (2, {B}), (2, {B}), (2, {B}), (2, {B})]
    result = sig(pool, A, declared=1)
    assert result.other == pytest.approx(0.2) and result.signature is Signature.SPECIFIC


def test_binary_verdict_mapping():
    expected = {
        Signature.SPECIFIC: ALIGNED, Signature.OVERBROAD: ALIGNED, Signature.OFF_TARGET: MISALIGNED,
        Signature.INERT: MISALIGNED, Signature.PARTIAL: MISALIGNED, Signature.UNDEFINED_OWN: UNDETERMINED,
    }
    assert {s: s.verdict for s in Signature} == expected


def test_discriminating_contrast_is_own_minus_mean_of_the_other_criteria():
    pool = [(1, {A}), (1, {B}), (2, {A}), (2, {B}), (3, {B}), (3, {B})]
    result = sig(pool, A, declared=1)
    assert result.dc == pytest.approx(0.5 - (0.5 + 0.0) / 2)


def test_matrix_criterion_label_is_the_exclusive_criterion_none_or_ambiguous():
    pool = [(1, {A}), (2, {B}), (3, {B}), (3, {C})]
    evidence = make(pool)
    assert classify_test(evidence[A], 1).criterion_label == "1"
    assert classify_test(evidence[B], 2).criterion_label == AMBIGUOUS_LABEL
    assert classify_test(make([(1, {B})])[A], 1).criterion_label == NO_LABEL


def test_mean_dc_skips_tests_whose_criterion_has_no_valid_mutant():
    signatures = classify_tests(make([(1, {A}), (2, {B})]), {A: 1, B: 2, C: 3})
    assert mean_dc(signatures.values()) == pytest.approx(1.0)
    assert mean_dc([]) is None


@given(
    pool=st.lists(st.tuples(st.integers(1, 3), st.frozensets(st.sampled_from([A, B, C]))), max_size=8),
    declared=st.integers(1, 3),
)
def test_matrix_signature_rules_are_exhaustive_and_consistent(pool, declared):
    evidence = build_evidence([A, B, C], [PoolMutant(c, k) for c, k in pool], [])
    for test in (A, B, C):
        result = classify_test(evidence[test], declared)
        if result.signature is Signature.UNDEFINED_OWN:
            assert result.own is None and result.dc is None
        elif result.signature is Signature.INERT:
            assert not evidence[test].kills_anything
        elif result.signature is Signature.OFF_TARGET:
            assert result.own == 0 and evidence[test].kills_anything
        elif result.signature is Signature.PARTIAL:
            assert 0 < result.own < 0.5
        else:
            assert result.own >= 0.5
            assert (result.signature is Signature.SPECIFIC) == (result.other <= 0.2)


# ----------------------------------------------------------------------------- gate pools
