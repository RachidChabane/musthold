from __future__ import annotations

from pilot.domain.mutant_classes import CriterionClass, JudgeFindings, MutantEvidence, classify_mutant
from pilot.domain.mutants import DifferentialKind
from pilot.domain.outcomes import KillResult
from pilot.domain.validity_payload import CriterionVerdict, ValidityVerdict, Verdict

REFS = ["PARK-1~1", "PARK-2~1", "PARK-3~1"]
KILLED = {"t1": KillResult.KILLED, "t2": KillResult.SURVIVED}
SURVIVED = {"t1": KillResult.SURVIVED, "t2": KillResult.FLAKY}


def verdict(violated=(), confidence=0.9, equivalent=False, equivalent_confidence=0.9, not_violated_confidence=0.9):
    return ValidityVerdict(
        equivalent, equivalent_confidence,
        {
            ref: CriterionVerdict(Verdict.VIOLATED, confidence, None) if ref in violated
            else CriterionVerdict(Verdict.NOT_VIOLATED, not_violated_confidence, None)
            for ref in REFS
        },
    )


BINDINGS = {"t1": "PARK-3~1", "t2": "PARK-3~1"}


def evidence(judges, kills=KILLED, differential=DifferentialKind.DIFFERS, target="PARK-3~1", bindings=BINDINGS):
    return MutantEvidence("m", "T", target, kills, differential, judges, bindings)


def both(v, confirmed=("PARK-3~1",)):
    return [JudgeFindings(v, frozenset(confirmed)), JudgeFindings(v)]


def test_valid_needs_both_judges_above_tau_and_a_confirmed_witness_and_takes_the_minimum_confidence():
    low = verdict(violated=["PARK-3~1"], confidence=0.75)
    result = classify_mutant(evidence([JudgeFindings(verdict(violated=["PARK-3~1"]), frozenset({"PARK-3~1"})), JudgeFindings(low)]), REFS)
    assert result.per_criterion["PARK-3~1"] is CriterionClass.VALID_W
    assert result.valid_confidence["PARK-3~1"] == 0.75
    assert result.primary_included and result.primary_criterion == "PARK-3~1"
    assert not (result.collateral or result.off_target or result.unattributed or result.equivalent)


def test_an_unconfirmed_witness_blocks_valid():
    result = classify_mutant(evidence(both(verdict(violated=["PARK-3~1"]), confirmed=())), REFS)
    assert result.per_criterion["PARK-3~1"] is CriterionClass.UNDETERMINED and not result.primary_included


def test_confidence_below_tau_blocks_valid():
    result = classify_mutant(evidence(both(verdict(violated=["PARK-3~1"], confidence=0.69))), REFS)
    assert result.per_criterion["PARK-3~1"] is CriterionClass.UNDETERMINED


def test_a_missing_judge_leaves_everything_undetermined():
    result = classify_mutant(evidence([JudgeFindings(verdict(violated=["PARK-3~1"]), frozenset({"PARK-3~1"})), JudgeFindings(None)]), REFS)
    assert set(result.per_criterion.values()) == {CriterionClass.UNDETERMINED} and not result.primary_included


def test_both_judges_not_violated_is_not_violating():
    result = classify_mutant(evidence(both(verdict()), kills=SURVIVED, differential=DifferentialKind.NO_DIFF_FOUND), REFS)
    assert set(result.per_criterion.values()) == {CriterionClass.NOT_VIOLATING}


def test_collateral_when_valid_for_two_criteria_and_not_in_the_primary_matrix():
    result = classify_mutant(evidence(both(verdict(violated=["PARK-3~1", "PARK-2~1"]), confirmed=REFS)), REFS)
    assert result.collateral and not result.primary_included and result.primary_criterion is None


def test_off_target_when_valid_elsewhere_but_not_for_the_target():
    result = classify_mutant(evidence(both(verdict(violated=["PARK-1~1"]), confirmed=["PARK-1~1"])), REFS)
    assert result.off_target and not result.primary_included


def test_equivalent_needs_no_reproduced_kill_no_difference_and_both_judges_confident():
    kwargs = dict(kills=SURVIVED, differential=DifferentialKind.NO_DIFF_FOUND)
    judges = both(verdict(equivalent=True))
    assert classify_mutant(evidence(judges, **kwargs), REFS).equivalent
    assert classify_mutant(evidence(judges, kills=SURVIVED, differential=DifferentialKind.SKIPPED), REFS).equivalent
    assert not classify_mutant(evidence(judges, kills=KILLED, differential=DifferentialKind.NO_DIFF_FOUND), REFS).equivalent
    assert not classify_mutant(evidence(judges, kills=SURVIVED, differential=DifferentialKind.DIFFERS), REFS).equivalent
    assert not classify_mutant(evidence(both(verdict(equivalent=True, equivalent_confidence=0.6)), **kwargs), REFS).equivalent


def test_unattributed_needs_a_difference_and_both_judges_confident_that_nothing_is_violated():
    assert classify_mutant(evidence(both(verdict())), REFS).unattributed
    assert not classify_mutant(evidence(both(verdict()), kills=SURVIVED, differential=DifferentialKind.NO_DIFF_FOUND), REFS).unattributed
    assert not classify_mutant(evidence(both(verdict(not_violated_confidence=0.5))), REFS).unattributed


def test_a_mutant_that_survived_every_test_and_is_equivalent_or_not_violating_for_its_target_is_discarded():
    survived = dict(kills=SURVIVED, differential=DifferentialKind.NO_DIFF_FOUND)
    assert classify_mutant(evidence(both(verdict()), **survived), REFS).discarded
    assert classify_mutant(evidence(both(verdict(equivalent=True)), **survived), REFS).discarded
    assert not classify_mutant(evidence(both(verdict()), kills=KILLED, differential=DifferentialKind.NO_DIFF_FOUND), REFS).discarded
    valid = classify_mutant(evidence(both(verdict(violated=["PARK-3~1"])), **survived), REFS)
    assert valid.survived_all and not valid.discarded


def test_an_arm_p_mutant_has_no_target_and_enters_no_primary_matrix():
    planted = MutantEvidence("m", "P", None, KILLED, DifferentialKind.DIFFERS, both(verdict(violated=["PARK-3~1"])), BINDINGS)
    result = classify_mutant(planted, REFS)
    assert result.per_criterion["PARK-3~1"] is CriterionClass.VALID_W
    assert not (result.primary_included or result.off_target) and result.target is None


def test_mapping_follows_the_run_record_contract():
    result = classify_mutant(evidence(both(verdict(violated=["PARK-3~1"]))), REFS).to_mapping()
    assert set(result) == {
        "mutant_id", "arm", "target", "per_criterion", "valid_x", "valid_confidence", "equivalent", "unattributed",
        "off_target", "collateral", "primary", "discarded", "survived_all",
    }
    assert result["primary"] == {"included": True, "criterion": "PARK-3~1"}


def test_valid_x_is_valid_w_without_a_kill_or_error_on_a_test_bound_to_another_criterion():
    valid = both(verdict(violated=["PARK-3~1"]))
    isolated = classify_mutant(evidence(valid), REFS)
    assert isolated.valid_x == {"PARK-3~1"} and isolated.per_criterion["PARK-3~1"] is CriterionClass.VALID_W
    vetoed = classify_mutant(evidence(valid, bindings={"t1": "PARK-1~1", "t2": "PARK-3~1"}), REFS)
    assert vetoed.per_criterion["PARK-3~1"] is CriterionClass.VALID_W and vetoed.valid_x == frozenset()
    errored = classify_mutant(evidence(valid, kills={"t1": KillResult.SURVIVED, "t2": KillResult.ERROR},
                                       bindings={"t1": "PARK-3~1", "t2": "PARK-1~1"}), REFS)
    assert errored.valid_x == frozenset()
    flaky = classify_mutant(evidence(valid, kills={"t1": KillResult.SURVIVED, "t2": KillResult.FLAKY},
                                     bindings={"t1": "PARK-3~1", "t2": "PARK-1~1"}), REFS)
    assert flaky.valid_x == {"PARK-3~1"}
    assert vetoed.primary_included and vetoed.to_mapping()["valid_x"] == []
