from __future__ import annotations

import pytest

from specbench.domain.assignment import Classification, assign
from specbench.domain.edits import Change, Edit, applicable, apply_edits, select_planted, select_violations
from specbench.domain.h_tests import HFile, h_problems
from specbench.domain.languages import Language
from specbench.domain.leaks import (
    MAX_EXCESS_PROBE_HITS,
    ProbeOutcome,
    go_tuples,
    in_sources,
    literal_leaks,
    normalize_literal,
    oracle_tuples,
    predicted_tuples,
    probe_hits,
    python_tuples,
    spec_tuples,
)
from specbench.domain.public_api import go_api, public_api, python_api, python_module_path
from specbench.domain.spec import (
    InterfaceEntry,
    SpecCriterion,
    SpecDraft,
    criteria_of,
    parse_example,
    render_spec,
    spec_from_record,
    spec_problems,
    spec_to_record,
)
from specbench.domain.task import CriterionStatus

TASK = "ACME-PARKFEE-12"


def draft(*examples: str, statement: str = "WHEN a stay lasts under 10 minutes THE SYSTEM SHALL charge 0") -> SpecDraft:
    return SpecDraft(
        title="Parking fee",
        overview="A fee for a stay.",
        interface=(InterfaceEntry("pkg.fee", "fee(minutes: int) -> int", "the fee of a stay"),),
        criteria=(
            SpecCriterion(1, statement, examples or ("fee(minutes=7) => 0",), CriterionStatus.NEW),
            SpecCriterion(
                2,
                "WHEN a stay lasts 10 minutes or more THE SYSTEM SHALL charge 5",
                ("fee(minutes=60) => 5",),
                CriterionStatus.UNCHANGED,
            ),
        ),
    )


def test_a_conforming_spec_has_no_problem_and_renders_the_adr_format() -> None:
    assert spec_problems(draft()) == []
    text = render_spec(TASK, draft())
    assert "- ACME-PARKFEE-12-1~1: WHEN a stay lasts under 10 minutes THE SYSTEM SHALL charge 0\n" in text
    assert "  - Example: fee(minutes=7) => 0\n" in text
    assert "## Interface\n\n- `pkg.fee` `fee(minutes: int) -> int`: the fee of a stay\n" in text
    assert [c.criterion_id for c in criteria_of(TASK, draft())] == [f"{TASK}-1", f"{TASK}-2"]


def test_a_spec_round_trips_through_its_structured_record() -> None:
    assert spec_from_record(spec_to_record(draft())) == draft()


@pytest.mark.parametrize(
    ("example", "operation", "arguments", "outcome"),
    [
        (
            "quote(entry=2026-03-02T10:00, exit=2026-03-02T10:15) => 0.00",
            "quote",
            ("2026-03-02T10:00", "2026-03-02T10:15"),
            "0.00",
        ),
        ('label(text="a, b", flag=true) => "x"', "label", ('"a, b"', "true"), '"x"'),
        ("parse(text=null) => raises ValueError", "parse", ("null",), "raises ValueError"),
    ],
)
def test_examples_follow_the_closed_grammar(
    example: str, operation: str, arguments: tuple[str, ...], outcome: str
) -> None:
    parsed = parse_example(example)
    assert parsed is not None
    assert (parsed.operation, parsed.arguments, parsed.outcome) == (operation, arguments, outcome)


@pytest.mark.parametrize(
    "example", ["fee(7) => 0", "fee(minutes=x) => 0", "fee(minutes=7) -> 0", "fee(minutes=7) => zero"]
)
def test_examples_outside_the_grammar_are_refused(example: str) -> None:
    assert parse_example(example) is None


def test_each_writing_rule_names_its_problem() -> None:
    assert any("one line" in p for p in spec_problems(draft(statement="the fee is zero")))
    assert any("outside the grammar" in p for p in spec_problems(draft("fee 7 gives 0")))
    assert any(
        "examples" in p
        for p in spec_problems(
            draft("fee(minutes=1) => 0", "fee(minutes=2) => 0", "fee(minutes=3) => 0", "fee(minutes=4) => 0")
        )
    )
    single = SpecDraft("t", "o", draft().interface, draft().criteria[:1])
    assert any("criteria" in p for p in spec_problems(single))
    unchanged = SpecDraft(
        "t",
        "o",
        draft().interface,
        tuple(SpecCriterion(c.number, c.statement, c.examples, CriterionStatus.UNCHANGED) for c in draft().criteria),
    )
    assert any("new or revised" in p for p in spec_problems(unchanged))
    assert any("Interface" in p for p in spec_problems(SpecDraft("t", "o", (), draft().criteria)))


def test_literals_are_normalised_across_spec_python_and_go() -> None:
    assert normalize_literal("5.0") == normalize_literal("5") == "n:5"
    assert normalize_literal('"x"') == normalize_literal("'x'") == normalize_literal("`x`") == "s:x"
    assert {normalize_literal(t) for t in ("true", "True")} == {"b:true"}
    assert normalize_literal("nil") == normalize_literal("None") == "null"
    assert normalize_literal("abc") is None


def test_python_tuples_come_from_literal_only_calls_and_parametrize_rows() -> None:
    source = (
        "import pytest\n\n"
        "@pytest.mark.parametrize('n', [4, 9])\n"
        "def test_a(n):\n"
        "    assert fee(minutes=17, tag='x') == 0\n"
        "    assert fee(minutes=value()) == 1\n"
    )
    found = python_tuples(source)
    assert frozenset({"n:17", "s:x"}) in found
    assert frozenset({"n:4"}) in found and frozenset({"n:9"}) in found


def test_go_tuples_come_from_literal_only_calls_and_table_rows() -> None:
    source = (
        'func TestFee(t *testing.T) {\n\tcases := []struct{ in int }{{in: 17}, {in: 23}}\n\tFee(31, "x")\n\tFee(n)\n}\n'
    )
    found = go_tuples(source)
    assert {frozenset({"n:17"}), frozenset({"n:23"}), frozenset({"n:31", "s:x"})} <= found
    assert frozenset({"n:n"}) not in found


def test_trivial_tuples_are_not_significant() -> None:
    assert oracle_tuples(Language.PYTHON, {"t": "assert f(0, 1, True)\nassert f(42)"}) == {frozenset({"n:42"})}


def test_a_spec_example_that_spells_an_oracle_tuple_is_a_leak_unless_a_public_source_states_it() -> None:
    oracle = oracle_tuples(Language.PYTHON, {"t": "def test_a():\n    assert fee(7) == 0\n    assert fee(60) == 5\n"})
    spec = draft("fee(minutes=7) => 0")
    assert spec_tuples(spec) == {frozenset({"n:7"}), frozenset({"n:60"})}
    leaks = literal_leaks(oracle, spec, [])
    assert sorted(sorted(values) for values in leaks) == [["n:60"], ["n:7"]]
    assert literal_leaks(oracle, spec, ["Charge 7 minutes and 60 minutes as stated in the issue."]) == []
    assert literal_leaks(oracle, draft("fee(minutes=8) => 0"), []) == [frozenset({"n:60"})]


def test_in_sources_needs_every_literal_in_one_text() -> None:
    assert in_sources(frozenset({"n:7", "s:ab"}), ["a stay of 7 minutes with ab"])
    assert not in_sources(frozenset({"n:7", "s:ab"}), ["7 only", "ab only"])
    assert not in_sources(frozenset({"n:7"}), ["17 minutes"])


def test_probe_scoring_counts_distinct_oracle_tuples_the_model_predicted() -> None:
    oracle = {frozenset({"n:7"}), frozenset({"n:60"}), frozenset({"n:90"})}
    predictions = [["7"], ["60"], ["60"], ["0"], ["x"]]
    assert predicted_tuples(predictions) == {frozenset({"n:7"}), frozenset({"n:60"})}
    assert probe_hits(oracle, predictions) == 2
    assert not ProbeOutcome(1, 1 + MAX_EXCESS_PROBE_HITS).leaks
    assert ProbeOutcome(1, 2 + MAX_EXCESS_PROBE_HITS).leaks


def test_the_public_api_lists_signatures_for_both_languages() -> None:
    python = "def fee(minutes: int, *, tax: bool = False) -> int:\n    '''Charge.\n\n    More.'''\n\ndef _hidden(): ...\n\nclass Meter:\n    def read(self) -> int: ...\n    def _p(self): ...\n"
    lines = python_api("src/pkg/fee.py", python)
    assert lines[0] == "pkg.fee: def fee(minutes: int, *, tax: bool=False) -> int  # Charge."
    assert "pkg.fee: Meter.def read(self) -> int" in lines and not any(
        "_hidden" in line or "_p" in line for line in lines
    )
    assert python_module_path("pkg/__init__.py") == "pkg"
    go = "package fee\n\nfunc Fee(minutes int) int {\n\treturn 1\n}\n\nfunc hidden() {}\n\nfunc (m *Meter) Read() int {\n\treturn 1\n}\n\ntype Meter struct {\n"
    assert go_api("example.com/acme", "fee/fee.go", go) == [
        "example.com/acme/fee: func Fee(minutes int) int",
        "example.com/acme/fee: func (m *Meter) Read() int",
        "example.com/acme/fee: type Meter struct",
    ]
    assert public_api(Language.PYTHON, "", {"pkg/a.py": "def a(): ..."}) == "pkg.a: def a()"


PYTHON_H = (
    "from hypothesis import given, settings, strategies as st\nfrom pkg.fee import fee\n\n\n"
    "@settings(derandomize=True)\n@given(st.integers(0, 9))\ndef test_c1_free(n):\n    assert fee(n) == 0\n\n\n"
    "def test_c2_paid():\n    assert fee(60) == 5\n"
)


def test_h_files_are_checked_for_layout_determinism_and_coverage() -> None:
    problems, tests = h_problems(
        Language.PYTHON, [HFile("tests/spec_h/test_spec_h_fee.py", PYTHON_H)], 2, frozenset(), ""
    )
    assert problems == [] and [(t.test_id, t.number) for t in tests] == [
        ("tests/spec_h/test_spec_h_fee.py::test_c1_free", 1),
        ("tests/spec_h/test_spec_h_fee.py::test_c2_paid", 2),
    ]
    bad = [
        HFile("tests/other/test_fee.py", PYTHON_H),
        HFile("tests/spec_h/test_spec_h_x.py", PYTHON_H.replace("derandomize=True", "max_examples=5")),
        HFile("tests/spec_h/test_spec_h_y.py", "def test_misc():\n    pass\n\n\ndef test_c9_far():\n    pass\n"),
    ]
    problems, _ = h_problems(Language.PYTHON, bad, 2, frozenset(), "")
    text = "\n".join(problems)
    assert "live at tests/spec_h" in text and "derandomize" in text and "must state its criterion" in text
    assert "criterion 9, which does not exist" in text and "criterion 2 has no H test" not in text


def test_go_h_files_stay_in_interface_packages_and_import_only_the_standard_library() -> None:
    good = 'package fee_test\n\nimport (\n\t"testing"\n\t"example.com/acme/fee"\n)\n\nfunc TestH1_Free(t *testing.T) {\n\t_ = fee.Fee(1)\n}\n'
    problems, tests = h_problems(
        Language.GO, [HFile("fee/h_free_test.go", good)], 1, frozenset({"fee"}), "example.com/acme"
    )
    assert problems == [] and tests[0].test_id == "fee::TestH1_Free"
    outside = good.replace('"example.com/acme/fee"', '"github.com/other/lib"')
    problems, _ = h_problems(
        Language.GO, [HFile("fee/h_free_test.go", outside)], 1, frozenset({"fee"}), "example.com/acme"
    )
    assert any("standard library and the module only" in p for p in problems)
    problems, _ = h_problems(
        Language.GO, [HFile("elsewhere/h_free_test.go", good)], 1, frozenset({"fee"}), "example.com/acme"
    )
    assert any("package directory" in p for p in problems)


FILES = {"pkg/fee.py": "def fee(m):\n    if m < 10:\n        return 0\n    return 5\n"}


def test_edits_apply_only_when_the_snippet_is_unique_and_something_changes() -> None:
    allowed = frozenset(FILES)
    assert apply_edits(FILES, [Edit("pkg/fee.py", "return 0", "return 1")], allowed) == {
        "pkg/fee.py": "def fee(m):\n    if m < 10:\n        return 1\n    return 5\n"
    }
    assert "exactly once" in str(apply_edits(FILES, [Edit("pkg/fee.py", "return", "x")], allowed))
    assert "exactly once" in str(apply_edits(FILES, [Edit("pkg/fee.py", "absent", "x")], allowed))
    assert "not a file" in str(apply_edits(FILES, [Edit("tests/test_fee.py", "a", "b")], allowed))
    assert "nothing" in str(apply_edits(FILES, [Edit("pkg/fee.py", "return 0", "return 0")], allowed))


def test_two_violations_per_criterion_and_two_refactors_are_required() -> None:
    def change(criterion: int | None, search: str, replace: str) -> Change:
        return Change(criterion, f"{criterion}:{replace}", (Edit("pkg/fee.py", search, replace),))

    changes = [change(1, "return 0", "return 1"), change(1, "m < 10", "m < 9"), change(2, "return 5", "return 6")]
    applied, problems = applicable(changes + [change(2, "absent", "x")], FILES, frozenset(FILES))
    assert len(applied) == 3 and len(problems) == 1
    chosen, short = select_violations(applied, 2)
    assert len(chosen) == 3 and short == ["criterion 2 has 1 applicable violations, 2 required"]
    assert select_planted(applied)[1] == []
    assert select_planted(applied[:1])[1] != []


def classification(implied: bool, criterion: int | None) -> Classification:
    return Classification("t", implied, criterion, "")


def test_a_test_stays_in_u_only_when_both_models_agree_it_is_implied_by_the_same_criterion() -> None:
    ids = ["a", "b", "c", "d", "e"]
    opus = {
        "a": classification(True, 1),
        "b": classification(True, 1),
        "c": classification(False, None),
        "d": classification(True, 9),
    }
    haiku = {
        "a": classification(True, 1),
        "b": classification(True, 2),
        "c": classification(False, None),
        "d": classification(True, 9),
    }
    result = assign(ids, opus, haiku, 2)
    assert dict(result.u) == {"a": 1}
    assert result.o == ("b", "c", "d", "e")
    assert result.disagreements == ("b", "e")
