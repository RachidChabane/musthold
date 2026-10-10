from __future__ import annotations

from specbench.domain.oracle import Outcome, folded_outcome, requested_outcomes

CODES = "tests/spec_h/test_spec_h_source_type_codes.py"


def test_a_parameter_free_id_takes_the_folded_outcome_of_its_parametrized_cases() -> None:
    reported = {
        f"{CODES}::test_c1_new_member_from_value[EXCEL-excel]": Outcome.PASSED,
        f"{CODES}::test_c1_new_member_from_value[GMAIL-gmail]": Outcome.PASSED,
        f"{CODES}::test_c5_android_content_type_decodes[SOURCE_CONTENT_TYPE_EXCEL]": Outcome.PASSED,
        f"{CODES}::test_c5_android_content_type_decodes[SOURCE_CONTENT_TYPE_GMAIL]": Outcome.FAILED,
        f"{CODES}::test_c1_examples": Outcome.PASSED,
    }
    requested = [
        f"{CODES}::test_c1_new_member_from_value",
        f"{CODES}::test_c5_android_content_type_decodes",
        f"{CODES}::test_c1_examples",
        f"{CODES}::test_c6_display_labels",
    ]
    assert requested_outcomes(requested, reported) == {
        requested[0]: Outcome.PASSED,
        requested[1]: Outcome.FAILED,
        requested[2]: Outcome.PASSED,
        requested[3]: Outcome.MISSING,
    }


def test_an_id_with_parameters_matches_only_its_own_case() -> None:
    reported = {
        "tests/test_app_boundary.py::test_matcher_allows_neutral_imports[from .. import artifacts\\n]": Outcome.PASSED,
        "tests/test_app_boundary.py::test_matcher_allows_neutral_imports[from .. import types\\n]": Outcome.FAILED,
    }
    requested = ["tests/test_app_boundary.py::test_matcher_allows_neutral_imports[from .. import artifacts\\n]"]
    assert requested_outcomes(requested, reported) == {requested[0]: Outcome.PASSED}


def test_a_class_test_folds_its_cases_under_the_class() -> None:
    reported = {
        "tests/test_dynamics.py::TestEffectiveStrength::test_floor[0.1]": Outcome.PASSED,
        "tests/test_dynamics.py::TestEffectiveStrength::test_floor[0.9]": Outcome.ERROR,
    }
    requested = ["tests/test_dynamics.py::TestEffectiveStrength::test_floor"]
    assert requested_outcomes(requested, reported) == {requested[0]: Outcome.ERROR}


def test_a_fold_is_passed_only_when_a_case_ran_and_every_case_passed() -> None:
    assert folded_outcome([Outcome.PASSED, Outcome.PASSED]) is Outcome.PASSED
    assert folded_outcome([]) is Outcome.MISSING
    assert folded_outcome([Outcome.PASSED, Outcome.MISSING]) is Outcome.MISSING
    assert folded_outcome([Outcome.MISSING, Outcome.ERROR, Outcome.PASSED]) is Outcome.ERROR
    assert folded_outcome([Outcome.ERROR, Outcome.FAILED]) is Outcome.FAILED
