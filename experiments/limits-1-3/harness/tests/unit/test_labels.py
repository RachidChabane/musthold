from __future__ import annotations

import pytest

from pilot.domain.labels import TestClass, TestLabel


def label(**fields):
    base = {"nodeid": "tests/t.py::a", "cites": "PARK-3~1", "label": "GOOD", "checks": ""}
    return TestLabel.from_mapping({**base, **fields})


def test_plain_label_has_no_second_criterion():
    parsed = label()
    assert (parsed.cites, parsed.test_class, parsed.other) == (3, TestClass.GOOD, None)


@pytest.mark.parametrize(
    ("text", "expected_other"),
    [
        ("GOOD_BROAD(PARK-3,PARK-4)", 4),
        ("GOOD_BROAD(3,4)", 4),
        ("GOOD_BROAD(4,3)", 4),
    ],
)
def test_good_broad_other_is_the_criterion_that_is_not_cited(text, expected_other):
    assert label(label=text).other == expected_other


@pytest.mark.parametrize("text", ["MIS_OTHER(PARK-3->PARK-8)", "MIS_OTHER(3→8)", "MIS_OTHER(PARK-3→PARK-8)"])
def test_mis_other_reads_the_arrow_target(text):
    assert label(label=text).other == 8


def test_other_falls_back_to_the_checks_field():
    assert label(label="MIS_OTHER", checks="PARK-5 behaviour").other == 5


def test_missing_second_criterion_is_an_error():
    with pytest.raises(ValueError):
        label(label="MIS_OTHER")


def test_unknown_class_is_an_error():
    with pytest.raises(ValueError):
        label(label="STRONG")


def test_gold_good_membership():
    assert TestClass.GOOD.is_gold_good and TestClass.GOOD_BROAD.is_gold_good
    assert not TestClass.WEAK.is_gold_good
