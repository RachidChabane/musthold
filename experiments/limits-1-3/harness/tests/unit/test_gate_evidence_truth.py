"""Substituting the isolation link by the oracle changes the veto only, never the judged validity (PREREG 9.4)."""

from __future__ import annotations

from pilot.analysis.model import EvidenceTruth, gate_evidence
from pilot.domain.gate import MutantResults
from pilot.domain.run_records import MutantClassView

C = 3


def view(mutant_id, valid_x):
    return MutantClassView(mutant_id, "T", C, {C: "VALID_W"}, frozenset({C}) if valid_x else frozenset(), {C: 0.9},
                           False, False, False, False, C, False, False)


class Stub:
    """The slice of the analysis context `gate_evidence` reads."""

    def __init__(self, views, oracle_valid, oracle_failed, judged_vetoed):
        self.views, self._valid, self._failed, self._vetoed = views, oracle_valid, oracle_failed, judged_vetoed

    def arm_t_evaluated(self):
        return self.views

    def results_of(self, mutant_id):
        return MutantResults(frozenset({mutant_id}))

    def oracle_valid_for(self, mutant_id, criterion):
        return mutant_id in self._valid

    def oracle_vetoes(self, mutant_id, criterion):
        return any(f != criterion for f in self._failed.get(mutant_id, ()))

    def vetoed(self, mutant_id, criterion):
        return mutant_id in self._vetoed


def names(results):
    return sorted(next(iter(r.killed)) for r in results)


def stub():
    views = [view("silent", True), view("valid", True), view("collateral", True), view("vetoed", False)]
    return Stub(views, oracle_valid={"valid", "vetoed"}, oracle_failed={"collateral": (C, 5), "vetoed": (C,)}, judged_vetoed={"vetoed"})


def test_isolation_by_truth_keeps_an_oracle_silent_valid_w_mutant_in_the_blocking_pool():
    evidence = gate_evidence(stub(), C, EvidenceTruth.ISOLATION)  # type: ignore[arg-type]
    assert names(evidence.valid_x) == ["silent", "valid", "vetoed"]
    assert names(evidence.valid_w_only) == ["collateral"]


def test_validity_by_truth_replaces_the_judged_set_then_runs_the_veto():
    evidence = gate_evidence(stub(), C, EvidenceTruth.VALIDITY)  # type: ignore[arg-type]
    assert names(evidence.valid_x) == ["valid"] and names(evidence.valid_w_only) == ["vetoed"]


def test_all_links_by_truth_is_the_oracle_valid_set():
    evidence = gate_evidence(stub(), C, EvidenceTruth.ALL)  # type: ignore[arg-type]
    assert names(evidence.valid_x) == ["valid", "vetoed"] and evidence.valid_w_only == ()


def test_without_substitution_the_pipeline_classes_decide():
    evidence = gate_evidence(stub(), C, EvidenceTruth.NONE)  # type: ignore[arg-type]
    assert names(evidence.valid_x) == ["collateral", "silent", "valid"] and names(evidence.valid_w_only) == ["vetoed"]
