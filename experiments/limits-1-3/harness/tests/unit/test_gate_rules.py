"""Gate rules 1 to 7, policies and sensitivities (ADR 0006, PREREG 9.2)."""

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
    G3_NOISO,
    G3_PREREG,
    G3_STRICT,
    POLICIES,
    CriterionEvidence,
    EvidenceBasis,
    GateInputs,
    GateOutcome,
    MutantResults,
    Rule,
    evaluate_gate,
)

A, B, C = "tests/t.py::a", "tests/t.py::b", "tests/t.py::c"
ALIGNED, MISALIGNED, UNDETERMINED = AlignmentVerdict.ALIGNED, AlignmentVerdict.MISALIGNED, AlignmentVerdict.UNDETERMINED


def results(kill_sets):
    return tuple(k if isinstance(k, MutantResults) else MutantResults(frozenset(k)) for k in kill_sets)


def inputs(valid_x=(), valid_w_only=(), undetermined=(), alignment=None, trace=True, reference=None, tests=(A, B, C)):
    return GateInputs(
        trace,
        reference if reference is not None else {t: True for t in tests},
        CriterionEvidence(results(valid_x), results(valid_w_only), results(undetermined)),
        alignment if alignment is not None else {t: ALIGNED for t in tests},
    )


def decide(policy, tests, **kw):
    return evaluate_gate(policy, tests, inputs(**kw))


def test_the_policy_table_names_every_policy_and_labels_the_inadmissible_one():
    assert list(POLICIES) == ["G0", "G1", "G2", "G3", "G3-prereg", "G3-noiso", "G3-strict", "G3-lenient"]
    assert G3_STRICT.label == "G3-strict (not an admissible product policy)"
    assert all(p.admissible for p in POLICIES.values() if p is not G3_STRICT)


def test_component_policies_read_the_rules_the_adr_assigns_them():
    assert G1.rules == {Rule.TRACEABILITY, Rule.REFERENCE_PASSES, Rule.SURVIVING_VALID, Rule.NO_VALID_MUTANT, Rule.SURVIVING_UNDETERMINED}
    assert G2.rules == {Rule.TRACEABILITY, Rule.REFERENCE_PASSES, Rule.ALIGNMENT}
    assert G3.rules == G1.rules | {Rule.ALIGNMENT}
    assert G3.basis is EvidenceBasis.VALID_X and G3_PREREG.basis is G3_NOISO.basis is EvidenceBasis.VALID_W


def test_an_error_is_neither_a_kill_nor_a_survival():
    errored = MutantResults(frozenset(), frozenset({A}))
    assert not decide(G1, {A}, valid_x=[errored, {A}]).fired
    assert decide(G1, {A}, valid_x=[errored]).fired == (Rule.NO_VALID_MUTANT,)
    assert decide(G1, {A}, valid_x=[errored]).outcome is GateOutcome.ESCALATE
    assert decide(G1, {A}, valid_x=[{A}], undetermined=[errored]).outcome is GateOutcome.PASS
    assert decide(G1, {A, B}, valid_x=[{A}], undetermined=[errored]).outcome is GateOutcome.PASS
    assert decide(G1, {B}, valid_x=[{B}], undetermined=[errored]).outcome is GateOutcome.ESCALATE


def test_a_clean_scenario_passes_under_every_policy():
    for policy in POLICIES.values():
        assert decide(policy, {A}, valid_x=[{A}]).outcome is GateOutcome.PASS


def test_rule_1_blocks_when_traceability_fails_even_under_g0():
    assert decide(G0, {A}, valid_x=[{A}], trace=False).outcome is GateOutcome.BLOCK


def test_g0_ignores_everything_but_traceability():
    result = decide(G0, {A}, valid_x=[{B}], alignment={A: MISALIGNED}, reference={A: False})
    assert result.outcome is GateOutcome.PASS and result.fired == ()


def test_rule_2_blocks_when_the_reference_fails_a_test_in_the_scenario():
    result = decide(G1, {A, B}, valid_x=[{A}], reference={A: True, B: False})
    assert result.outcome is GateOutcome.BLOCK and Rule.REFERENCE_PASSES in result.fired


def test_rule_3_blocks_when_a_valid_x_mutant_survives_every_test_in_the_scenario():
    result = decide(G1, {A}, valid_x=[{A}, {B}])
    assert result.outcome is GateOutcome.BLOCK and Rule.SURVIVING_VALID in result.fired


def test_rule_4_escalates_when_no_valid_x_mutant_exists_and_blocks_nothing():
    result = decide(G3, {A}, valid_x=[])
    assert result.outcome is GateOutcome.ESCALATE and result.fired == (Rule.NO_VALID_MUTANT,)
    assert Rule.NO_VALID_MUTANT.value == "no_valid_mutant"


def test_rule_4a_is_implied_by_rule_3_when_valid_x_mutants_exist_and_none_is_killed():
    result = decide(G3, {A}, valid_x=[{B}])
    assert result.outcome is GateOutcome.BLOCK and Rule.SURVIVING_VALID in result.fired


def test_rule_5_escalates_on_a_surviving_undetermined_or_valid_w_only_mutant_but_not_a_killed_one():
    assert decide(G1, {A}, valid_x=[{A}], undetermined=[{B}]).outcome is GateOutcome.ESCALATE
    assert decide(G1, {A}, valid_x=[{A}], valid_w_only=[{B}]).outcome is GateOutcome.ESCALATE
    assert decide(G1, {A}, valid_x=[{A}], undetermined=[{A}]).outcome is GateOutcome.PASS
    assert decide(G1, {A}, valid_x=[{A}], valid_w_only=[{A}]).outcome is GateOutcome.PASS


@pytest.mark.parametrize("verdict", [MISALIGNED, UNDETERMINED])
def test_rule_6_escalates_when_a_test_is_not_aligned(verdict):
    result = decide(G3, {A, B}, valid_x=[{A}], alignment={A: ALIGNED, B: verdict})
    assert result.outcome is GateOutcome.ESCALATE and result.fired == (Rule.ALIGNMENT,)


def test_a_test_without_an_alignment_verdict_counts_as_undetermined():
    assert decide(G3, {A}, valid_x=[{A}], alignment={}).outcome is GateOutcome.ESCALATE


def test_g2_reads_only_traceability_the_reference_and_alignment():
    result = decide(G2, {A}, valid_x=[{B}], alignment={A: MISALIGNED})
    assert result.fired == (Rule.ALIGNMENT,)


def test_block_takes_precedence_over_escalate_in_the_same_scenario():
    result = decide(G3, {A}, valid_x=[{B}], alignment={A: MISALIGNED})
    assert result.outcome is GateOutcome.BLOCK
    assert Rule.ALIGNMENT in result.fired


def test_strict_turns_escalation_into_block_and_lenient_into_pass():
    kw = dict(valid_x=[{A}], alignment={A: MISALIGNED})
    assert decide(G3, {A}, **kw).outcome is GateOutcome.ESCALATE
    assert decide(G3_STRICT, {A}, **kw).outcome is GateOutcome.BLOCK
    assert decide(G3_LENIENT, {A}, **kw).outcome is GateOutcome.PASS


def test_lenient_and_strict_keep_blocks_as_blocks():
    assert decide(G3_LENIENT, {A}, valid_x=[{B}]).outcome is GateOutcome.BLOCK


def test_g3_prereg_blocks_where_no_valid_mutant_is_killed_and_reads_valid_w_in_rules_3_and_4():
    assert decide(G3_PREREG, {A}, valid_x=[]).outcome is GateOutcome.BLOCK
    assert decide(G3_PREREG, {A}, valid_x=[{A}], valid_w_only=[{B}]).outcome is GateOutcome.BLOCK
    assert decide(G3, {A}, valid_x=[{A}], valid_w_only=[{B}]).outcome is GateOutcome.ESCALATE
    assert decide(G3_PREREG, {A}, valid_x=[], valid_w_only=[{A}]).outcome is GateOutcome.PASS


def test_g3_noiso_takes_valid_w_in_place_of_valid_x_and_keeps_rule_4_escalating():
    assert decide(G3_NOISO, {A}, valid_x=[{A}], valid_w_only=[{B}]).outcome is GateOutcome.BLOCK
    assert decide(G3_NOISO, {A}).outcome is GateOutcome.ESCALATE
    assert decide(G3_NOISO, {A}, valid_w_only=[{A}]).outcome is GateOutcome.PASS


kill_sets = st.lists(st.frozensets(st.sampled_from([A, B, C])), max_size=4)
verdicts = st.sampled_from(list(AlignmentVerdict))
scenarios = st.frozensets(st.sampled_from([A, B, C]), min_size=1)
alignments = st.fixed_dictionaries({A: verdicts, B: verdicts, C: verdicts})


@given(scenario=scenarios, valid=kill_sets, valid_w=kill_sets, undetermined=kill_sets, alignment=alignments,
       trace=st.booleans(), reference=st.fixed_dictionaries({A: st.booleans(), B: st.booleans(), C: st.booleans()}))
def test_gate_precedence_block_over_escalate_over_pass(scenario, valid, valid_w, undetermined, alignment, trace, reference):
    data = GateInputs(trace, reference, CriterionEvidence(results(valid), results(valid_w), results(undetermined)), alignment)
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


@given(scenario=scenarios, valid=kill_sets, valid_w=kill_sets, undetermined=kill_sets, alignment=alignments)
def test_g1_and_g2_decisions_never_exceed_g3(scenario, valid, valid_w, undetermined, alignment):
    data = GateInputs(True, {A: True, B: True, C: True}, CriterionEvidence(results(valid), results(valid_w), results(undetermined)), alignment)
    g3 = evaluate_gate(G3, scenario, data).outcome
    assert evaluate_gate(G1, scenario, data).outcome <= g3
    assert evaluate_gate(G2, scenario, data).outcome <= g3
    assert evaluate_gate(G0, scenario, data).outcome <= g3


@given(scenario=scenarios, valid=kill_sets, valid_w=kill_sets, undetermined=kill_sets, alignment=alignments,
       trace=st.booleans(), reference=st.fixed_dictionaries({A: st.booleans(), B: st.booleans(), C: st.booleans()}))
def test_g3_blocks_are_a_subset_of_g3_prereg_blocks(scenario, valid, valid_w, undetermined, alignment, trace, reference):
    data = GateInputs(trace, reference, CriterionEvidence(results(valid), results(valid_w), results(undetermined)), alignment)
    if evaluate_gate(G3, scenario, data).outcome is GateOutcome.BLOCK:
        assert evaluate_gate(G3_PREREG, scenario, data).outcome is GateOutcome.BLOCK


@given(scenario=scenarios, alignment=alignments, undetermined=kill_sets, valid_w=kill_sets)
def test_no_block_without_an_executed_input(scenario, alignment, undetermined, valid_w):
    """With traceability passing, the reference passing and no VALID_x mutant, G3 never blocks."""
    data = GateInputs(True, {A: True, B: True, C: True}, CriterionEvidence((), results(valid_w), results(undetermined)), alignment)
    assert evaluate_gate(G3, scenario, data).outcome is not GateOutcome.BLOCK


@given(scenario=scenarios, valid=kill_sets, valid_w=kill_sets, undetermined=kill_sets, alignment=alignments)
def test_every_block_of_g3_names_an_executed_rule(scenario, valid, valid_w, undetermined, alignment):
    data = GateInputs(True, {A: True, B: True, C: True}, CriterionEvidence(results(valid), results(valid_w), results(undetermined)), alignment)
    decision = evaluate_gate(G3, scenario, data)
    if decision.outcome is GateOutcome.BLOCK:
        assert Rule.SURVIVING_VALID in decision.fired
        assert any(mutant.survives_every(scenario) for mutant in results(valid))
