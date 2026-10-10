from __future__ import annotations

import ast

import pytest

from specbench.domain.go_target import GoTarget
from specbench.domain.h_tests import (
    POOL_SIZE,
    HFile,
    HWriting,
    TestKind,
    candidate_tests,
    go_collision_problems,
    go_top_level_identifiers,
    h_layout_of,
    h_problems,
    python_import_problems,
    python_pruned,
)
from specbench.domain.languages import Language

MODULE = "example.com/acme"
PYTHON = h_layout_of(Language.PYTHON, "", ["pkg.fee", "pkg.rates"])
GO = h_layout_of(Language.GO, MODULE, [f"{MODULE}/fee"])
PROPERTY = (
    "@settings(derandomize=True, max_examples=10, deadline=None)\n"
    "@given(st.integers(min_value=0, max_value=9))\n"
    "def test_c{n}_free_minutes(n):\n    assert fee(n) == 0\n\n\n"
)
EXAMPLES = "".join(f"def test_c{{n}}_example_{k}():\n    assert fee({k}) == 0\n\n\n" for k in range(1, 4))
HEADER = "from hypothesis import given, settings, strategies as st\nfrom pkg.fee import fee\n\n\n"


def python_file(number: int, prefix: str = "", body: str | None = None) -> HFile:
    content = HEADER + (body if body is not None else PROPERTY + EXAMPLES).replace("{n}", str(number))
    return HFile(f"tests/spec_h/test_spec_h_{prefix}c{number}_fee.py", content)


def go_file(number: int, helper: str = "", prefix: str = "") -> HFile:
    tests = "".join(
        f"func TestH{number}_{prefix.upper()}Example{k}(t *testing.T) {{\n\tif fee.Fee({k}) != 0 {{\n\t\tt.Fatal(k)\n\t}}\n}}\n\n"
        for k in range(1, 4)
    )
    prop = (
        f"func TestH{number}_{prefix.upper()}Free(t *testing.T) {{\n"
        "\tconfig := &quick.Config{Rand: rand.New(rand.NewSource(7))}\n"
        "\tif err := quick.Check(func(n uint8) bool { return fee.Fee(int(n%10)) == 0 }, config); err != nil {\n"
        "\t\tt.Fatal(err)\n\t}\n}\n\n"
    )
    content = (
        'package fee_test\n\nimport (\n\t"math/rand"\n\t"testing"\n\t"testing/quick"\n\n'
        f'\t"{MODULE}/fee"\n)\n\n{helper}{prop}{tests}'
    )
    return HFile(f"fee/h_{prefix}c{number}_free_test.go", content)


def test_a_pool_file_per_criterion_passes_and_each_test_knows_its_criterion_kind_file_and_round() -> None:
    problems, tests = h_problems(PYTHON, HWriting((1, 2)), [python_file(1), python_file(2)])
    assert problems == []
    assert [(t.number, t.kind) for t in tests if t.path.endswith("c1_fee.py")] == [
        (1, TestKind.PROPERTY),
        (1, TestKind.EXAMPLE),
        (1, TestKind.EXAMPLE),
        (1, TestKind.EXAMPLE),
    ]
    assert {t.round for t in tests} == {0}


def test_a_refill_writing_names_its_files_with_its_round_and_answers_only_its_criteria() -> None:
    problems, tests = h_problems(PYTHON, HWriting((2,), 1), [python_file(2, "r1_")])
    assert problems == [] and {t.round for t in tests} == {1}
    problems, _ = h_problems(PYTHON, HWriting((2,), 1), [python_file(2)])
    assert any("test_spec_h_r1_c<n>_<name>.py" in p for p in problems)
    problems, _ = h_problems(PYTHON, HWriting((2,), 1), [python_file(2, "r1_"), python_file(1, "r1_")])
    assert sum("which was not asked for" in p for p in problems) == POOL_SIZE


def test_a_file_never_mixes_criteria_and_every_test_states_its_criterion() -> None:
    mixed = python_file(
        1, body=PROPERTY + EXAMPLES + "def test_c2_other():\n    pass\n\n\ndef test_misc():\n    pass\n"
    )
    problems, _ = h_problems(PYTHON, HWriting((1, 2)), [mixed, python_file(2)])
    text = "\n".join(problems)
    assert "test_c2_other: criterion 2 in a file of criterion 1" in text
    assert "test_misc: name must state its criterion" in text


@pytest.mark.parametrize(
    ("body", "expected"),
    [
        (EXAMPLES + "def test_c{n}_extra():\n    pass\n\n\n", "has no property test"),
        (PROPERTY + EXAMPLES + PROPERTY.replace("free_minutes", "again"), "has 5 H tests, 4 required"),
        (
            PROPERTY
            + PROPERTY.replace("free_minutes", "b")
            + PROPERTY.replace("free_minutes", "c")
            + "def test_c{n}_example_1():\n    assert fee(1) == 0\n",
            "has 1 example tests, 2 required",
        ),
    ],
)
def test_the_pool_holds_four_candidates_per_criterion_with_a_property_and_two_examples(
    body: str, expected: str
) -> None:
    problems, _ = h_problems(PYTHON, HWriting((1,)), [python_file(1, body=body)])
    assert any(expected in p for p in problems), problems


def test_python_imports_stay_in_the_standard_library_pytest_hypothesis_and_the_interface() -> None:
    allowed = (
        "import json\nimport pytest\nfrom hypothesis import strategies\nfrom pkg.rates import rate\nimport pkg.fee\n"
    )
    assert python_import_problems(HFile("tests/spec_h/a.py", allowed), PYTHON.python_packages) == []
    for source, word in (
        ("from agents import RunContextWrapper\n", "imports agents"),
        ("from .helpers import x\n", "relative import"),
        ("def f():\n    import requests\n", "imports requests"),
        ("def broken(:\n", "does not parse"),
    ):
        problems = python_import_problems(HFile("tests/spec_h/a.py", source), PYTHON.python_packages)
        assert any(word in p for p in problems), source


def test_hypothesis_tests_must_be_derandomized() -> None:
    loose = python_file(1, body=(PROPERTY + EXAMPLES).replace("derandomize=True, ", ""))
    problems, _ = h_problems(PYTHON, HWriting((1,)), [loose])
    assert any("derandomize=True" in p for p in problems)


def test_go_files_stay_in_interface_packages_import_the_module_only_and_detect_property_tests() -> None:
    problems, tests = h_problems(GO, HWriting((1,)), [go_file(1)])
    assert problems == []
    assert (tests[0].test_id, tests[0].kind) == ("fee::TestH1_Free", TestKind.PROPERTY)
    assert {t.kind for t in tests[1:]} == {TestKind.EXAMPLE}
    outside = HFile("fee/h_c1_free_test.go", go_file(1).content.replace(f"{MODULE}/fee", "github.com/other/lib"))
    problems, _ = h_problems(GO, HWriting((1,)), [outside])
    assert any("the third-party modules named in the Interface only" in p for p in problems)
    elsewhere = HFile("elsewhere/h_c1_free_test.go", go_file(1).content)
    problems, _ = h_problems(GO, HWriting((1,)), [elsewhere])
    assert any("package directory of the Interface" in p for p in problems)


def test_go_top_level_identifiers_cover_functions_types_variables_constants_and_groups() -> None:
    source = (
        "package fee_test\n\nconst c1limit = 3\nvar c1a, c1b = 1, 2\ntype c1meter struct{}\n\n"
        "var (\n\tc1cases = []int{\n\t\t1,\n\t}\n\tc1other int\n)\n\n"
        "func (m c1meter) Read() int { return 1 }\n\nfunc init() {}\n\nfunc c1helper[T any](v T) T { return v }\n"
    )
    assert sorted(go_top_level_identifiers(source)) == sorted(
        ["c1limit", "c1a", "c1b", "c1meter", "c1cases", "c1other", "c1helper"]
    )


def test_a_go_identifier_declared_twice_in_a_package_is_a_problem_also_against_kept_files() -> None:
    helper = "func newClient() int { return 1 }\n\n"
    problems, _ = h_problems(GO, HWriting((1, 2)), [go_file(1, helper), go_file(2, helper)])
    assert any("top-level identifier newClient is declared by another H file of package fee" in p for p in problems)
    kept = [go_file(1, "func c1newClient() int { return 1 }\n\n")]
    refill = go_file(2, "func c1newClient() int { return 1 }\n\n", prefix="r1_")
    assert go_collision_problems([refill], kept) != []
    assert go_collision_problems([go_file(2, "func r1c2newClient() int { return 1 }\n\n", prefix="r1_")], kept) == []


def test_a_refill_never_repeats_a_test_name_or_identifier_of_any_earlier_writing() -> None:
    pool = [go_file(1), go_file(2)]
    reused = go_file(2, prefix="r1_")
    renamed = HFile(reused.path, reused.content.replace("TestH2_R1_", "TestH2_"))
    problems, _ = h_problems(GO, HWriting((2,), 1), [renamed], pool)
    assert any("fee::TestH2_Free: repeats the id of a test of an earlier writing" in p for p in problems)
    assert any("top-level identifier TestH2_Free is declared by another H file" in p for p in problems)
    assert h_problems(GO, HWriting((2,), 1), [reused], pool)[0] == []


def test_pruning_keeps_the_decorators_of_kept_tests_and_removes_those_of_pruned_ones() -> None:
    file = python_file(1)
    keep = frozenset({f"{file.path}::test_c1_free_minutes", f"{file.path}::test_c1_example_3"})
    pruned = python_pruned(file.path, file.content, keep)
    assert pruned is not None
    names = [n.name for n in ast.parse(pruned).body if isinstance(n, ast.FunctionDef)]
    assert names == ["test_c1_free_minutes", "test_c1_example_3"]
    assert pruned.count("@settings(derandomize=True") == 1 and pruned.count("@given(") == 1
    without_property = python_pruned(file.path, file.content, frozenset({f"{file.path}::test_c1_example_1"}))
    assert without_property is not None and "@given" not in without_property and "@settings" not in without_property
    assert python_pruned(file.path, file.content, frozenset()) is None


def test_pruning_a_class_of_pruned_tests_removes_the_class_and_keeps_the_file_parseable() -> None:
    source = (
        "import pytest\n\n\n@pytest.mark.slow\nclass TestGroup:\n    def test_c1_a(self):\n        pass\n\n"
        "    def test_c1_b(self):\n        pass\n\n\nclass TestKept:\n    def test_c1_c(self):\n        pass\n"
    )
    path = "tests/spec_h/test_spec_h_c1_x.py"
    pruned = python_pruned(path, source, frozenset({f"{path}::TestKept::test_c1_c"}))
    assert pruned is not None and "TestGroup" not in pruned and "slow" not in pruned
    ast.parse(pruned)


def test_candidate_tests_of_a_file_without_a_layout_name_still_carry_their_path() -> None:
    (test,) = candidate_tests(Language.PYTHON, HFile("tests/x.py", "def test_c3_a():\n    pass\n"))
    assert (test.number, test.path, test.round) == (3, "tests/x.py", 0)


def test_a_path_like_string_literal_in_a_go_test_is_not_an_import() -> None:
    source = go_file(1)
    with_literal = HFile(source.path, source.content + 'func c1dot() string {\n\treturn "."\n}\n')
    problems, _ = h_problems(GO, HWriting((1,)), [with_literal])
    assert problems == []


def test_aliased_dot_blank_and_single_foreign_imports_are_still_refused() -> None:
    source = go_file(1)
    for spec in ('\tother "example.org/lib"\n', '\t. "example.org/lib"\n', '\t_ "example.org/lib"\n'):
        grouped = HFile(source.path, source.content.replace(f'\t"{MODULE}/fee"\n', f'\t"{MODULE}/fee"\n{spec}'))
        problems, _ = h_problems(GO, HWriting((1,)), [grouped])
        assert any("example.org/lib" in p for p in problems), spec
    single = HFile(source.path, source.content.replace("import (", 'import "example.org/lib"\nimport (', 1))
    problems, _ = h_problems(GO, HWriting((1,)), [single])
    assert any("example.org/lib" in p for p in problems)


ROUTE_MODULE = "example.com/acme/routedns"
ROUTE_TARGET = GoTarget(ROUTE_MODULE, {".": "rdns"}, frozenset({"example.com/miekg/dns"}))
ROUTE_LAYOUT = h_layout_of(Language.GO, ROUTE_MODULE, [ROUTE_MODULE], ROUTE_TARGET)


def route_file(package: str, import_line: str, extra_import: str = "") -> HFile:
    tests = go_file(1).content.split(f'\t"{MODULE}/fee"\n)\n\n', 1)[1].replace("fee.Fee", "rdns.Fee")
    imports = f'\t"math/rand"\n\t"testing"\n\t"testing/quick"\n\n{import_line}{extra_import}'
    return HFile("h_c1_free_test.go", f"package {package}\n\nimport (\n{imports})\n\n{tests}")


def test_the_external_test_package_and_the_import_qualifier_must_follow_the_declared_package_name() -> None:
    good = route_file("rdns_test", f'\trdns "{ROUTE_MODULE}"\n')
    assert h_problems(ROUTE_LAYOUT, HWriting((1,)), [good])[0] == []
    wrong_package = route_file("routedns_test", f'\trdns "{ROUTE_MODULE}"\n')
    problems, _ = h_problems(ROUTE_LAYOUT, HWriting((1,)), [wrong_package])
    assert any("declare `package rdns_test`" in p and "not `package routedns_test`" in p for p in problems)
    unaliased = route_file("rdns_test", f'\t"{ROUTE_MODULE}"\n')
    problems, _ = h_problems(ROUTE_LAYOUT, HWriting((1,)), [unaliased])
    assert any(f'import it as `rdns "{ROUTE_MODULE}"`' in p for p in problems)
    misnamed = route_file("rdns_test", f'\troutedns "{ROUTE_MODULE}"\n')
    problems, _ = h_problems(ROUTE_LAYOUT, HWriting((1,)), [misnamed])
    assert any("declares package rdns" in p for p in problems)


def test_a_directory_without_a_declared_package_name_is_not_checked_for_one() -> None:
    assert h_problems(GO, HWriting((1,)), [go_file(1)])[0] == []


def test_third_party_modules_named_by_the_interface_may_be_imported_and_others_may_not() -> None:
    dns = route_file("rdns_test", f'\trdns "{ROUTE_MODULE}"\n', '\t"example.com/miekg/dns"\n')
    assert h_problems(ROUTE_LAYOUT, HWriting((1,)), [dns])[0] == []
    other = route_file("rdns_test", f'\trdns "{ROUTE_MODULE}"\n', '\t"example.com/stretchr/testify"\n')
    problems, _ = h_problems(ROUTE_LAYOUT, HWriting((1,)), [other])
    assert any("testify" in p for p in problems)
