from __future__ import annotations

import ast
from pathlib import Path

import pytest
from hypothesis import given, settings
from hypothesis import strategies as st

from pilot.domain.alignment_input import (
    Arm,
    UnknownTest,
    alignment_schema,
    build_test_unit,
    forbidden_strings_in,
    leaked_name_runs,
    public_signatures,
    tokenize_criteria,
)
from pilot.domain.prompts import alignment_item
from pilot.domain.spec import Criterion, parse_spec

MODULE = '''"""Module doc."""
from datetime import datetime, timedelta
from decimal import Decimal

import pytest

from parkfee import Breakdown, quote

BASE = datetime(2026, 3, 2, 9, 0)


def at(minutes):
    """Helper doc."""
    # a comment
    return BASE + timedelta(minutes=minutes)


def unrelated():
    return Decimal("1")


@pytest.mark.verifies("PARK-3~1")
def test_charges_two_hours_for_sixty_one_minutes():
    """Test doc."""
    assert quote(BASE, at(61)) == Decimal("5.00")  # trailing comment


@pytest.mark.verifies("PARK-1~1")
def test_other():
    with pytest.raises(ValueError):
        quote(BASE, at(-1))
'''
PACKAGE = {
    "src/parkfee/tariff.py": "def quote(entry: datetime, exit: datetime, *, lost_ticket: bool = False) -> Decimal:\n    '''Doc.'''\n    return secret_body\n",
    "src/parkfee/models.py": "@dataclass(frozen=True)\nclass Breakdown:\n    rule: Rule\n    amount: Decimal\n\n    def total(self) -> Decimal:\n        return self.amount\n",
}


def unit(arm, name="test_charges_two_hours_for_sixty_one_minutes"):
    return build_test_unit(MODULE, name, arm)


def test_shown_unit_keeps_names_and_drops_marker_comments_and_docstrings():
    source = unit(Arm.NAME_SHOWN).source
    assert "def test_charges_two_hours_for_sixty_one_minutes" in source and "def at(minutes)" in source
    for forbidden in ("verifies", "PARK", "comment", "doc", "Doc", "pytest"):
        assert forbidden not in source
    assert "unrelated" not in source and "Breakdown" not in source
    ast.parse(source)


def test_masked_unit_renames_the_test_and_every_helper_consistently():
    source = unit(Arm.NAME_MASKED).source
    assert "def test_case" in source and "def helper_2(minutes)" in source
    assert "helper_1 = datetime" in source and "def at" not in source and "BASE" not in source
    assert "helper_2(61)" in source
    ast.parse(source)


def test_helpers_come_before_the_test_in_source_order():
    source = unit(Arm.NAME_SHOWN).source
    assert source.index("BASE =") < source.index("def at") < source.index("def test_")


def test_an_unknown_test_is_rejected():
    with pytest.raises(UnknownTest):
        unit(Arm.NAME_SHOWN, "test_missing")


def test_public_signatures_show_imported_package_names_without_bodies():
    signatures = public_signatures(unit(Arm.NAME_SHOWN), "parkfee", PACKAGE)
    assert "def quote(entry: datetime, exit: datetime, *, lost_ticket: bool=False) -> Decimal:\n    ..." in signatures
    assert "secret_body" not in signatures and "Doc" not in signatures
    assert "Breakdown" not in signatures


def test_class_signatures_keep_fields_and_method_stubs():
    other = build_test_unit(MODULE.replace("quote(BASE, at(61))", "Breakdown(1, 2)"), "test_charges_two_hours_for_sixty_one_minutes", Arm.NAME_SHOWN)
    signatures = public_signatures(other, "parkfee", PACKAGE)
    assert "@dataclass(frozen=True)" in signatures and "rule: Rule" in signatures
    assert "def total(self) -> Decimal:\n        ..." in signatures


CRITERIA = [Criterion("PARK-1", 1, "first."), Criterion("PARK-2", 1, "second."), Criterion("PARK-3", 1, "third.")]


def test_tokens_are_opaque_unique_and_map_back_to_references():
    tokens = tokenize_criteria(CRITERIA, "run", "t::a", Arm.NAME_MASKED, 1)
    assert len(set(tokens.token_to_ref)) == 3 and all(t.startswith("K-") and len(t) == 6 for t in tokens.token_to_ref)
    assert sorted(tokens.token_to_ref.values()) == ["PARK-1~1", "PARK-2~1", "PARK-3~1"]
    assert "PARK" not in tokens.text and "~" not in tokens.text
    assert set(tokens.tokens) == set(tokens.token_to_ref)


def test_tokens_and_order_are_a_function_of_seed_test_arm_and_repetition_only():
    first = tokenize_criteria(CRITERIA, "run", "t::a", Arm.NAME_MASKED, 2)
    assert first == tokenize_criteria(CRITERIA, "run", "t::a", Arm.NAME_MASKED, 2)
    assert first != tokenize_criteria(CRITERIA, "run", "t::a", Arm.NAME_MASKED, 3)
    assert first.token_to_ref != tokenize_criteria(CRITERIA, "other-run", "t::a", Arm.NAME_MASKED, 2).token_to_ref


def test_colliding_tokens_are_redrawn_until_unique(monkeypatch):
    from pilot.domain import alignment_input

    real = alignment_input._field_hash
    monkeypatch.setattr(
        alignment_input, "_field_hash", lambda *f: "abcd" + real(*f) if f[-1] != "1" else real(*f)
    )
    tokens = tokenize_criteria(CRITERIA, "run", "t::a", Arm.NAME_MASKED, 1)
    assert len(set(tokens.token_to_ref)) == 3


@given(st.integers(min_value=1, max_value=5), st.text(min_size=1, max_size=20))
@settings(max_examples=60, deadline=None)
def test_tokens_are_unique_for_any_test_and_repetition(rep, uid):
    tokens = tokenize_criteria(parse_many(), "run", uid, Arm.NAME_SHOWN, rep)
    assert len(set(tokens.token_to_ref)) == 10


def parse_many():
    return [Criterion(f"PARK-{n}", 1, f"statement {n}.") for n in range(1, 11)]


def test_schema_tokens_replace_the_placeholder_and_keep_none():
    template = '{"properties": {"verifies": {"enum": ["__TOKENS__", "NONE"]}}}'
    import json

    assert json.loads(alignment_schema(template, ["K-1", "K-2"]))["properties"]["verifies"]["enum"] == ["K-1", "K-2", "NONE"]


def test_name_runs_are_found_only_for_three_or_more_consecutive_words():
    name = "test_night_stay_over_two_hours_costs_six_euros"
    assert leaked_name_runs("two hours costs", name) == ["two hours costs"]
    assert leaked_name_runs("two hours later costs six", name) == []
    assert leaked_name_runs("Night Stay Over", name) == ["night stay over"]


def test_forbidden_strings_are_reported():
    assert forbidden_strings_in("a verifies b PARK-1") == ["verifies", "PARK-"]
    assert forbidden_strings_in("clean") == []


def real_tests(experiment_root):
    project = experiment_root / "fixture" / "parkfee"
    tests = {f"tests/{p.name}": p.read_text() for p in sorted((project / "tests").glob("test_*.py"))}
    package = {p.relative_to(project).as_posix(): p.read_text() for p in sorted((project / "src").rglob("*.py"))}
    spec = parse_spec((project / "spec" / "parking.md").read_text())
    nodeids = [
        (file, node.name)
        for file, text in tests.items()
        for node in ast.parse(text).body
        if isinstance(node, ast.FunctionDef) and node.name.startswith("test_")
    ]
    return tests, package, spec, nodeids


EXPERIMENT = Path(__file__).resolve().parents[3]


@pytest.fixture
def real(experiment_root):
    return real_tests(experiment_root)


def test_the_leak_property_holds_for_the_masked_input_of_all_38_real_tests(real, experiment_root):
    tests, package, spec, nodeids = real
    template = (experiment_root / "prereg" / "prompts" / "alignment.md").read_text()
    assert len(nodeids) == 38
    for file, name in nodeids:
        built = build_test_unit(tests[file], name, Arm.NAME_MASKED)
        signatures = public_signatures(built, "parkfee", package)
        item = alignment_item(template, tokenize_criteria(spec, "run", f"{file}::{name}", Arm.NAME_MASKED, 1).text, built.source, signatures)
        assert forbidden_strings_in(item) == [], name
        assert leaked_name_runs(built.source + signatures, name) == [], name


@given(data=st.data(), rep=st.integers(min_value=1, max_value=5))
@settings(max_examples=40, deadline=None)
def test_the_leak_property_holds_for_any_repetition_and_seed(data, rep):
    tests, package, spec, nodeids = real_tests(EXPERIMENT)
    file, name = data.draw(st.sampled_from(nodeids))
    built = build_test_unit(tests[file], name, Arm.NAME_MASKED)
    item = built.source + public_signatures(built, "parkfee", package)
    item += tokenize_criteria(spec, str(rep), f"{file}::{name}", Arm.NAME_MASKED, rep).text
    assert forbidden_strings_in(item) == []
    assert not leaked_name_runs(built.source, name)


def test_shown_units_of_the_real_tests_parse_and_have_no_marker(real):
    tests, _, _, nodeids = real
    for file, name in nodeids:
        source = build_test_unit(tests[file], name, Arm.NAME_SHOWN).source
        ast.parse(source)
        assert "verifies" not in source and name in source
