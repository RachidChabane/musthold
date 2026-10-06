"""Gate rules (PREREG 9.2) and matrix classifier (PREREG 8)."""

from __future__ import annotations

import pytest
from hypothesis import given, strategies as st

from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.gate import (
    G0,
    G1,
    G2,
    G3,
    G3_LENIENT,
    G3_STRICT,
    POLICIES,
    CriterionEvidence,
    GateInputs,
    GateOutcome,
    Rule,
    evaluate_gate,
)
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


def inputs(valid=(), undetermined=(), alignment=None, trace=True, reference=None, tests=(A, B, C)):
    return GateInputs(
        trace,
        reference if reference is not None else {t: True for t in tests},
        CriterionEvidence(tuple(frozenset(k) for k in valid), tuple(frozenset(k) for k in undetermined)),
        alignment if alignment is not None else {t: ALIGNED for t in tests},
    )


def decide(policy, tests, **kw):
    return evaluate_gate(policy, tests, inputs(**kw))


def test_a_clean_scenario_passes_under_every_policy():
    for policy in POLICIES.values():
        assert decide(policy, {A}, valid=[{A}]).outcome is GateOutcome.PASS


def test_rule_1_blocks_when_traceability_fails_even_under_g0():
    assert decide(G0, {A}, valid=[{A}], trace=False).outcome is GateOutcome.BLOCK


def test_g0_ignores_everything_but_traceability():
    result = decide(G0, {A}, valid=[{B}], alignment={A: MISALIGNED}, reference={A: False})
    assert result.outcome is GateOutcome.PASS and result.fired == ()


def test_rule_2_blocks_when_the_reference_fails_a_test_in_the_scenario():
    result = decide(G1, {A, B}, valid=[{A}], reference={A: True, B: False})
    assert result.outcome is GateOutcome.BLOCK and Rule.REFERENCE_PASSES in result.fired


def test_rule_3_blocks_when_a_valid_mutant_survives_every_test_in_the_scenario():
    result = decide(G1, {A}, valid=[{A}, {B}])
    assert result.outcome is GateOutcome.BLOCK and Rule.SURVIVING_VALID in result.fired


def test_rule_4_blocks_when_no_valid_mutant_is_killed_and_also_when_there_is_none():
    assert Rule.NO_KILLED_VALID in decide(G1, {A}, valid=[{B}]).fired
    assert decide(G1, {A}, valid=[]).outcome is GateOutcome.BLOCK


def test_rule_5_escalates_on_a_surviving_undetermined_mutant_but_not_a_killed_one():
    assert decide(G1, {A}, valid=[{A}], undetermined=[{B}]).outcome is GateOutcome.ESCALATE
    assert decide(G1, {A}, valid=[{A}], undetermined=[{A}]).outcome is GateOutcome.PASS


@pytest.mark.parametrize("verdict", [MISALIGNED, UNDETERMINED])
def test_rule_6_escalates_when_a_test_is_not_aligned(verdict):
    result = decide(G3, {A, B}, valid=[{A}], alignment={A: ALIGNED, B: verdict})
    assert result.outcome is GateOutcome.ESCALATE and result.fired == (Rule.ALIGNMENT,)


def test_a_test_without_an_alignment_verdict_counts_as_undetermined():
    assert decide(G3, {A}, valid=[{A}], alignment={}).outcome is GateOutcome.ESCALATE


def test_g2_reads_only_traceability_and_alignment():
    result = decide(G2, {A}, valid=[{B}], alignment={A: MISALIGNED})
    assert result.fired == (Rule.ALIGNMENT,)


def test_block_takes_precedence_over_escalate_in_the_same_scenario():
    result = decide(G3, {A}, valid=[{B}], alignment={A: MISALIGNED})
    assert result.outcome is GateOutcome.BLOCK
    assert Rule.ALIGNMENT in result.fired


def test_strict_turns_escalation_into_block_and_lenient_into_pass():
    kw = dict(valid=[{A}], alignment={A: MISALIGNED})
    assert decide(G3, {A}, **kw).outcome is GateOutcome.ESCALATE
    assert decide(G3_STRICT, {A}, **kw).outcome is GateOutcome.BLOCK
    assert decide(G3_LENIENT, {A}, **kw).outcome is GateOutcome.PASS


def test_lenient_and_strict_keep_blocks_as_blocks():
    assert decide(G3_LENIENT, {A}, valid=[{B}]).outcome is GateOutcome.BLOCK


kill_sets = st.lists(st.frozensets(st.sampled_from([A, B, C])), max_size=4)
verdicts = st.sampled_from(list(AlignmentVerdict))


@given(
    scenario=st.frozensets(st.sampled_from([A, B, C]), min_size=1),
    valid=kill_sets,
    undetermined=kill_sets,
    alignment=st.fixed_dictionaries({A: verdicts, B: verdicts, C: verdicts}),
    trace=st.booleans(),
    reference=st.fixed_dictionaries({A: st.booleans(), B: st.booleans(), C: st.booleans()}),
)
def test_gate_precedence_block_over_escalate_over_pass(scenario, valid, undetermined, alignment, trace, reference):
    data = GateInputs(trace, reference, CriterionEvidence(tuple(valid), tuple(undetermined)), alignment)
    decision = evaluate_gate(G3, scenario, data)
    fired_outcomes = {rule.outcome for rule in decision.fired}
    if GateOutcome.BLOCK in fired_outcomes:
        assert decision.outcome is GateOutcome.BLOCK
    elif GateOutcome.ESCALATE in fired_outcomes:
        assert decision.outcome is GateOutcome.ESCALATE
    else:
        assert decision.outcome is GateOutcome.PASS
    assert evaluate_gate(G3, scenario, data) == decision
    assert evaluate_gate(G3_STRICT, scenario, data).outcome == (
        GateOutcome.BLOCK if decision.outcome is not GateOutcome.PASS else GateOutcome.PASS
    )
    assert evaluate_gate(G3_LENIENT, scenario, data).outcome == (
        GateOutcome.BLOCK if decision.outcome is GateOutcome.BLOCK else GateOutcome.PASS
    )


@given(
    scenario=st.frozensets(st.sampled_from([A, B, C]), min_size=1),
    valid=kill_sets,
    undetermined=kill_sets,
    alignment=st.fixed_dictionaries({A: verdicts, B: verdicts, C: verdicts}),
)
def test_g1_and_g2_decisions_never_exceed_g3(scenario, valid, undetermined, alignment):
    data = GateInputs(True, {A: True, B: True, C: True}, CriterionEvidence(tuple(valid), tuple(undetermined)), alignment)
    g3 = evaluate_gate(G3, scenario, data).outcome
    assert evaluate_gate(G1, scenario, data).outcome <= g3
    assert evaluate_gate(G2, scenario, data).outcome <= g3
    assert evaluate_gate(G0, scenario, data).outcome <= g3


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
