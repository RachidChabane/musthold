from __future__ import annotations

import ast
import tempfile
from pathlib import Path

import pytest
from hypothesis import given, settings
from hypothesis import strategies as st

from pilot.adapters.ast_edits import AstEditApplier, definition_source, replace_definition
from pilot.domain.edits import Edit, EditError, EditFailure, EditSet
from pilot.domain.normalization import normalize

MODULE = '''import functools


def helper(x):
    return x + 1


@functools.cache
def decorated(x):
    """Doc."""
    return x * 2


class Meter:
    scale = 2

    def read(self, x):
        return x * self.scale

    @property
    def unit(self):
        return "m"


def last():
    return helper(1)
'''


def edit(qualname, source, file="mod.py"):
    return Edit(file, qualname, source)


def test_replaces_a_module_level_function_only():
    result = replace_definition(MODULE, edit("helper", "def helper(x):\n    return x + 2\n"))
    assert "return x + 2" in result
    assert result.replace("x + 2", "x + 1") == MODULE


def test_replaces_a_method_and_keeps_its_indentation():
    result = replace_definition(
        MODULE, edit("Meter.read", "def read(self, x):\n    return x * 3\n")
    )
    assert "    def read(self, x):\n        return x * 3\n" in result
    ast.parse(result)


def test_replaces_a_module_level_class_with_its_decorator():
    module = "from dataclasses import dataclass\n\n\n@dataclass(frozen=True)\nclass Box:\n    size: int\n\n\nVALUE = 1\n"
    source = "@dataclass(frozen=True)\nclass Box:\n    size: int\n    label: str = ''\n"
    result = replace_definition(module, edit("Box", source))
    assert "label: str" in result and result.endswith("VALUE = 1\n")
    assert result.count("@dataclass") == 1


def test_replacement_starts_at_the_first_decorator():
    result = replace_definition(MODULE, edit("decorated", "def decorated(x):\n    return 0\n"))
    assert "functools.cache" not in result.split("class Meter")[0]
    assert "def decorated(x):\n    return 0\n" in result


def test_a_replacement_may_bring_its_own_decorator():
    source = "@functools.cache\ndef helper(x):\n    return 0\n"
    result = replace_definition(MODULE, edit("helper", source))
    assert "@functools.cache\ndef helper" in result


def test_indented_new_source_is_dedented_first():
    result = replace_definition(MODULE, edit("helper", "    def helper(x):\n        return 9\n"))
    assert "def helper(x):\n    return 9\n" in result


@pytest.mark.parametrize("qualname", ["missing", "Meter.missing", "Missing.read", "a.b.c"])
def test_absent_qualname_is_an_error(qualname):
    with pytest.raises(EditError) as caught:
        replace_definition(MODULE, edit(qualname, f"def {qualname.rsplit('.', 1)[-1]}(): pass"))
    assert caught.value.failure is EditFailure.ABSENT_QUALNAME


def test_unparseable_new_source_is_an_error():
    with pytest.raises(EditError) as caught:
        replace_definition(MODULE, edit("helper", "def helper(x:\n"))
    assert caught.value.failure is EditFailure.UNPARSEABLE


@pytest.mark.parametrize(
    "source",
    ["x = 1\n", "def other(x):\n    return 1\n", "def helper(x):\n    return 1\n\ndef extra():\n    pass\n"],
)
def test_new_source_must_be_exactly_the_named_definition(source):
    with pytest.raises(EditError) as caught:
        replace_definition(MODULE, edit("helper", source))
    assert caught.value.failure is EditFailure.NOT_A_SINGLE_DEFINITION


def test_definition_source_returns_dedented_text_with_decorators():
    assert definition_source(MODULE, "mod.py", "Meter.unit") == '@property\ndef unit(self):\n    return "m"\n'
    assert definition_source(MODULE, "mod.py", "decorated").startswith("@functools.cache\n")


@pytest.fixture
def tree(tmp_path):
    (tmp_path / "pkg").mkdir()
    (tmp_path / "pkg" / "mod.py").write_text(MODULE)
    return tmp_path


def test_applier_rewrites_files_and_reports_originals(tree):
    edit_set = EditSet((edit("helper", "def helper(x):\n    return 0\n", "pkg/mod.py"),))
    applier = AstEditApplier()
    preview = applier.preview(tree, edit_set)
    assert preview["pkg/mod.py"][0] == MODULE
    assert (tree / "pkg" / "mod.py").read_text() == MODULE
    applier.apply(tree, edit_set)
    assert "return 0" in (tree / "pkg" / "mod.py").read_text()


def test_applier_applies_several_edits_to_one_file(tree):
    edit_set = EditSet(
        (
            edit("helper", "def helper(x):\n    return 0\n", "pkg/mod.py"),
            edit("Meter.read", "def read(self, x):\n    return -x\n", "pkg/mod.py"),
            edit("last", "def last():\n    return 5\n", "pkg/mod.py"),
        )
    )
    AstEditApplier().apply(tree, edit_set)
    text = (tree / "pkg" / "mod.py").read_text()
    assert "return -x" in text and "return 5" in text and "return 0" in text


@pytest.mark.parametrize("file", ["/etc/passwd", "../outside.py", "pkg/../../x.py"])
def test_applier_rejects_unsafe_paths(tree, file):
    with pytest.raises(EditError) as caught:
        AstEditApplier().preview(tree, EditSet((edit("helper", "def helper(x):\n    pass\n", file),)))
    assert caught.value.failure is EditFailure.UNSAFE_PATH


def test_applier_rejects_missing_files(tree):
    with pytest.raises(EditError) as caught:
        AstEditApplier().preview(tree, EditSet((edit("helper", "def helper(x):\n    pass\n", "pkg/none.py"),)))
    assert caught.value.failure is EditFailure.MISSING_FILE


def test_a_failing_edit_leaves_the_tree_untouched(tree):
    edit_set = EditSet(
        (
            edit("helper", "def helper(x):\n    return 0\n", "pkg/mod.py"),
            edit("absent", "def absent():\n    pass\n", "pkg/mod.py"),
        )
    )
    with pytest.raises(EditError):
        AstEditApplier().apply(tree, edit_set)
    assert (tree / "pkg" / "mod.py").read_text() == MODULE


BODIES = st.sampled_from(["return 0", "return x", "return x + 1", "return -x", "y = x\n    return y"])
TARGETS = st.sampled_from(["helper", "decorated", "Meter.read", "Meter.unit", "last"])


@st.composite
def edit_sets(draw):
    qualnames = draw(st.lists(TARGETS, min_size=1, max_size=3, unique=True))
    edits = []
    for qualname in qualnames:
        name = qualname.rsplit(".", 1)[-1]
        params = "self, x" if "." in qualname else "x"
        edits.append(Edit("pkg/mod.py", qualname, f"def {name}({params}):\n    {draw(BODIES)}\n"))
    return EditSet(tuple(edits))


@settings(max_examples=60, database=None, deadline=None)
@given(edit_sets())
def test_applying_then_unapplying_restores_the_tree(edit_set):
    with tempfile.TemporaryDirectory() as raw:
        root = Path(raw)
        (root / "pkg").mkdir()
        target = root / "pkg" / "mod.py"
        target.write_text(MODULE)
        applier = AstEditApplier()
        inverse = applier.inverse(root, edit_set)
        applier.apply(root, edit_set)
        applier.apply(root, inverse)
        assert target.read_text() == MODULE


@settings(max_examples=60, database=None, deadline=None)
@given(edit_sets())
def test_applied_edits_always_leave_a_parseable_module(edit_set):
    with tempfile.TemporaryDirectory() as raw:
        root = Path(raw)
        (root / "pkg").mkdir()
        (root / "pkg" / "mod.py").write_text(MODULE)
        preview = AstEditApplier().preview(root, edit_set)
        assert normalize(preview["pkg/mod.py"][1])
