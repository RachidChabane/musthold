from __future__ import annotations

import ast

from hypothesis import given, settings
from hypothesis import strategies as st

from pilot.domain.normalization import changed_line_count, normalize, normalized_digest

IDENTIFIERS = st.sampled_from(["alpha", "beta", "gamma", "delta", "total", "rate"])
EXPRESSIONS = st.sampled_from(["1", "x + 1", "x * 2", "-x", "x if x > 0 else 0", "[x, 1]", "'text'"])
COMMENTS = st.sampled_from(["", "  # trailing note", "  # another"])
DOCSTRINGS = st.one_of(st.none(), st.sampled_from(['"""Doc."""', "'one'", '"""two\n    lines"""']))


@st.composite
def function_sources(draw):
    name = draw(IDENTIFIERS)
    lines = [f"def {name}(x):"]
    docstring = draw(DOCSTRINGS)
    if docstring:
        lines.append(f"    {docstring}")
    for _ in range(draw(st.integers(1, 4))):
        lines.append(f"    # {draw(IDENTIFIERS)}")
        lines.append(f"    {draw(IDENTIFIERS)} = {draw(EXPRESSIONS)}{draw(COMMENTS)}")
    lines.append(f"    return {draw(EXPRESSIONS)}")
    return "\n".join(lines) + "\n"


@st.composite
def module_sources(draw):
    functions = draw(st.lists(function_sources(), min_size=1, max_size=3))
    header = draw(st.sampled_from(["", '"""Module doc."""\n', "# header\n"]))
    return header + "\n\n".join(functions)


@settings(max_examples=200, database=None)
@given(module_sources())
def test_normalize_is_idempotent(source):
    once = normalize(source)
    assert normalize(once) == once


@settings(max_examples=200, database=None)
@given(module_sources())
def test_normalize_removes_comments_and_docstrings(source):
    tree = ast.parse(normalize(source))
    for node in ast.walk(tree):
        if isinstance(node, ast.Module | ast.FunctionDef):
            assert ast.get_docstring(node) is None
    assert "#" not in normalize(source)


@settings(max_examples=100, database=None)
@given(module_sources())
def test_comment_and_docstring_changes_do_not_change_the_digest(source):
    decorated = '"""Added doc."""\n# added\n' + source
    assert normalized_digest({"m.py": source}) == normalized_digest({"m.py": decorated})


def test_docstring_only_body_becomes_pass():
    assert normalize('def f():\n    """doc"""\n') == "def f():\n    pass"


def test_leading_string_statements_are_all_stripped():
    assert normalize('def f():\n    "a"\n    "b"\n    return 1\n') == "def f():\n    return 1"


def test_formatting_differences_normalize_equal():
    assert normalize("x   =  (1+2)") == normalize("x = 1 + 2")


def test_changed_line_count_counts_a_one_line_edit_once():
    before = "def f(x):\n    return x + 1\n"
    after = "def f(x):\n    return x + 2\n"
    assert changed_line_count(before, after) == 1


def test_changed_line_count_ignores_comment_only_edits():
    assert changed_line_count("x = 1\n", "# note\nx = 1  # trailing\n") == 0


def test_changed_line_count_counts_pure_insertions():
    assert changed_line_count("x = 1\n", "x = 1\ny = 2\nz = 3\n") == 2


def test_changed_line_count_uses_the_larger_side_of_a_replaced_block():
    before = "a = 1\nb = 2\n"
    after = "a = 1\nb = 2\nc = 3\n".replace("b = 2\nc = 3", "p = 1\nq = 2\nr = 3")
    assert changed_line_count(before, after) == 3
