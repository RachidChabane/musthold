from __future__ import annotations

from pilot.domain.constants import SHALLNOT_CONFTEST_HOOK
from pilot.domain.leak_rules import check_test_files

CLEAN = '''import pytest

from parkfee import quote


def helper():
    return 1


@pytest.mark.verifies("PARK-1~1")
def test_one():
    assert quote(helper()) == 1


@pytest.mark.verifies("PARK-2~1")
def test_two():
    assert quote(2) == 2
'''
FILES = ("test_a.py",)


HOOK = SHALLNOT_CONFTEST_HOOK + "\n"


def messages(sources, files=FILES, count=2, conftest=HOOK):
    return [str(f) for f in check_test_files(sources, files, count, conftest)]


def test_clean_file_has_no_finding():
    assert messages({"test_a.py": CLEAN}) == []


def test_comment_is_reported_with_its_line():
    source = CLEAN.replace("    return 1\n", "    return 1  # note\n", 1)
    assert messages({"test_a.py": source}) == ["test_a.py:7: comment"]


def test_docstrings_are_reported_at_module_function_and_class_level():
    source = '"""Module."""\n' + CLEAN.replace("def helper():\n", 'def helper():\n    """Doc."""\n')
    found = messages({"test_a.py": source})
    assert len([m for m in found if m.endswith("docstring")]) == 2


def test_string_expression_after_the_first_statement_is_not_a_docstring():
    source = CLEAN.replace("    return 1\n", "    x = 1\n    'text'\n    return x\n", 1)
    assert messages({"test_a.py": source}) == []


def test_fixture_and_parametrize_are_reported():
    source = CLEAN + '''

@pytest.fixture
def made():
    return 1


@pytest.mark.parametrize("x", [1])
@pytest.mark.verifies("PARK-3~1")
def test_three(x):
    assert x
'''
    found = messages({"test_a.py": source}, count=3)
    assert any("forbidden decorator fixture" in m for m in found)
    assert any("forbidden decorator parametrize" in m for m in found)


def test_fixture_call_form_is_reported():
    source = CLEAN + "\n\n@pytest.fixture(scope='module')\ndef made():\n    return 1\n"
    assert any("fixture" in m for m in messages({"test_a.py": source}))


def test_test_without_marker_or_with_two_markers_is_reported():
    none = CLEAN.replace('@pytest.mark.verifies("PARK-2~1")\n', "")
    two = CLEAN.replace(
        '@pytest.mark.verifies("PARK-2~1")\n', '@pytest.mark.verifies("PARK-2~1")\n@pytest.mark.verifies("PARK-3~1")\n'
    )
    assert any("test_two has 0 verifies markers" in m for m in messages({"test_a.py": none}))
    assert any("test_two has 2 verifies markers" in m for m in messages({"test_a.py": two}))


def test_malformed_marker_is_reported():
    for bad in ('"PARK-2"', '"PARK-2~1", "PARK-3~1"', "ref"):
        source = CLEAN.replace('"PARK-2~1"', bad)
        assert any("malformed verifies marker" in m for m in messages({"test_a.py": source}))


def test_marker_on_a_helper_is_reported():
    source = CLEAN.replace("def helper():", '@pytest.mark.verifies("PARK-9~1")\ndef helper():')
    assert any("helper helper carries a marker" in m for m in messages({"test_a.py": source}))


def test_file_set_must_match_exactly():
    found = messages({"test_a.py": CLEAN, "test_extra.py": CLEAN}, files=("test_a.py", "test_b.py"), count=4)
    assert "test_b.py: missing test file" in found
    assert "test_extra.py: unexpected test file" in found


def test_every_file_needs_a_test():
    assert "test_a.py: contains no test" in messages({"test_a.py": "VALUE = 1\n"}, count=0)


def test_total_test_count_is_checked():
    assert messages({"test_a.py": CLEAN}, count=3) == ["*: 2 tests, expected 3"]


def test_unparseable_file_is_reported():
    assert any("does not parse" in m for m in messages({"test_a.py": "def broken(:\n"}, count=0))


def test_conftest_must_be_the_documented_hook_verbatim():
    assert messages({"test_a.py": CLEAN}, conftest=None) == ["conftest.py: missing"]
    altered = HOOK.replace("verifies", "verifies2")
    assert messages({"test_a.py": CLEAN}, conftest=altered) == ["conftest.py: is not Shallnot's documented hook verbatim"]
    assert messages({"test_a.py": CLEAN}, conftest="\n" + HOOK + "\n\n") == []


def test_tests_must_take_no_parameters():
    source = CLEAN.replace("def test_one():", "def test_one(monkeypatch):")
    assert messages({"test_a.py": source}) == ["test_a.py:11: test_one takes parameters"]


def test_usefixtures_and_module_level_pytestmark_are_reported():
    marked = CLEAN.replace("def test_one", '@pytest.mark.usefixtures("anything")\ndef test_one')
    assert any("forbidden decorator usefixtures" in m for m in messages({"test_a.py": marked}))
    module_mark = "pytestmark = pytest.mark.verifies('PARK-1~1')\n" + CLEAN
    assert any("module-level pytestmark" in m for m in messages({"test_a.py": module_mark}))
