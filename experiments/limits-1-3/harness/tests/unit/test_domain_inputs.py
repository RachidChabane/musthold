from __future__ import annotations

import ast
import json

import pytest

from pilot.domain.call_graph import callers_of, call_graph, definition_names
from pilot.domain.definitions import apply_edit_set, is_verifies_decorator
from pilot.domain.differential_text import differential_sentence, render_output
from pilot.domain.edits import Edit, EditError, EditSet
from pilot.domain.mutants import DifferentialKind, DifferentialResult
from pilot.domain.operator_mutants import operator_mutants
from pilot.domain.prompts import generation_item, render_files
from pilot.domain.relabel_input import relabel_problems, render_tests
from pilot.domain.validity_input import build_validity_input

SOURCES = {
    "src/p/m.py": (
        "def quote(a, b):\n    return helper(a) + b\n\n\n"
        "def helper(a):\n    if a > 15:\n        return a * 2\n    return 0\n\n\n"
        "def other():\n    return 1\n\n\n"
        "class K:\n    def method(self, x):\n        return x - 1\n"
    )
}
EDIT = EditSet((Edit("src/p/m.py", "helper", "def helper(a):\n    if a >= 15:\n        return a * 2\n    return 0\n"),))


def test_validity_input_shows_edited_and_reaching_functions_and_a_diff_without_extras():
    built = build_validity_input(SOURCES, EDIT)
    assert built.original.index(":: helper") < built.original.index(":: quote")
    assert "def helper(a):\n    if a > 15:" in built.original and "def other" not in built.original
    assert "if a >= 15:" in built.mutant and ":: quote" not in built.mutant
    assert "--- a/src/p/m.py" in built.diff and "-    if a > 15:" in built.diff and "+    if a >= 15:" in built.diff


def test_validity_input_normalizes_away_comments_and_docstrings():
    sources = {"src/p/m.py": SOURCES["src/p/m.py"].replace("    if a > 15:", '    """Doc."""\n    # why\n    if a > 15:')}
    assert "Doc" not in build_validity_input(sources, EDIT).original and "why" not in build_validity_input(sources, EDIT).original


def test_a_method_edit_is_located_by_its_qualified_name():
    method = EditSet((Edit("src/p/m.py", "K.method", "def method(self, x):\n    return x + 1\n"),))
    assert ":: K.method" in build_validity_input(SOURCES, method).mutant


def test_an_edit_naming_an_absent_definition_is_rejected():
    with pytest.raises(EditError):
        apply_edit_set(SOURCES, EditSet((Edit("src/p/m.py", "nothing", "def nothing():\n    pass\n"),)))


def test_call_graph_helpers_find_the_callers_of_an_edited_name():
    graph = call_graph(SOURCES.values())
    assert callers_of(graph, {"helper"}) == {"quote"}
    assert definition_names(SOURCES)["method"] == [("src/p/m.py", "K.method")]


def test_differential_sentence_only_for_differs():
    differs = DifferentialResult(
        DifferentialKind.DIFFERS, "quote", {"entry": "2026-03-02T09:00", "lost_ticket": True},
        {"ok": ["decimal", "40"]}, {"exc": "ValueError"},
    )
    assert differential_sentence(differs) == (
        "An automated comparison found this distinguishing call: quote(entry='2026-03-02T09:00', lost_ticket=True) "
        "returned 40 before the change and raised ValueError after it."
    )
    assert differential_sentence(DifferentialResult(DifferentialKind.NO_DIFF_FOUND)) == ""
    assert differential_sentence(None) == ""
    assert render_output({"ok": ["str", "EUR 1.00"]}) == "'EUR 1.00'"


def test_operator_mutants_cover_each_operator_family_and_parse():
    mutants = operator_mutants(SOURCES)
    sources = [m.edit_set.edits[0].new_source for m in mutants]
    for new_source in sources:
        ast.parse(new_source)
    assert any("a < 15" in s for s in sources) and any("a >= 15" in s for s in sources)
    assert any("a + 2" in s for s in sources) and any("a * 20" in s for s in sources) and any("a * 3" in s for s in sources)
    assert any("not a > 15" in s for s in sources)
    assert any("pass" in s and "a * 2" not in s for s in sources)
    assert any("x + 1" in s for s in sources)
    assert {m.edit_set.edits[0].qualname for m in mutants} == {"quote", "helper", "other", "K.method"}


def test_operator_mutants_are_deterministic_and_differ_from_the_original():
    first, second = operator_mutants(SOURCES), operator_mutants(SOURCES)
    assert first == second
    unchanged = "def helper(a):\n    if a > 15:\n        return a * 2\n    return 0\n"
    assert all(m.edit_set.edits[0].new_source != unchanged for m in first)
    assert len({m.edit_set for m in first}) == len(first)


def test_generation_item_lists_source_files_as_headed_blocks_in_path_order():
    rendered = render_files({"src/b.py": "B", "src/a.py": "A"})
    assert rendered == "=== src/a.py ===\nA\n\n=== src/b.py ===\nB"
    item = generation_item("{target_ref}|{target_statement}|{nonce}|{spec}|{source}", "S", "PARK-1~1", "T", {"src/a.py": "A"}, "n0")
    assert item == "PARK-1~1|T|n0|S|=== src/a.py ===\nA"


TESTS = {"tests/test_a.py": 'import pytest\n\n\n@pytest.mark.verifies("PARK-9~1")\ndef test_x():\n    assert 1\n\n\ndef helper():\n    return 2\n'}


def test_relabel_input_takes_declared_links_from_the_bindings_not_from_the_decorator():
    rendered = render_tests(TESTS, {"tests/test_a.py::test_x": "PARK-3~1"})
    assert "# nodeid: tests/test_a.py::test_x  declares: PARK-3~1" in rendered
    assert "verifies" not in rendered and "PARK-9" not in rendered and "def helper" in rendered


def test_relabel_payload_must_cover_exactly_the_expected_tests():
    check = relabel_problems(["a", "b"])
    assert check({"tests": [{"nodeid": "a"}, {"nodeid": "b"}]}) == []
    assert "missing ['b']" in check({"tests": [{"nodeid": "a"}]})[0]
    assert "unexpected ['c']" in check({"tests": [{"nodeid": "a"}, {"nodeid": "b"}, {"nodeid": "c"}]})[0]
    assert check({"tests": [{"nodeid": "a"}, {"nodeid": "a"}, {"nodeid": "b"}]})


def test_verifies_decorators_are_recognised_in_both_spellings():
    for text in ('pytest.mark.verifies("X")', 'verifies("X")', "pytest.mark.verifies"):
        assert is_verifies_decorator(ast.parse(text, mode="eval").body)
    assert not is_verifies_decorator(ast.parse('pytest.mark.slow("X")', mode="eval").body)


def test_edit_sets_survive_a_json_round_trip():
    assert EditSet.from_mapping(json.loads(json.dumps(EDIT.to_mapping()))) == EDIT
