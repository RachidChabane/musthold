from __future__ import annotations

import re

from hypothesis import given
from hypothesis import strategies as st

from pilot.domain.edits import Edit, EditSet
from pilot.domain.ids import canonical_json, mutant_id, normalized_edit_set, slugify
from pilot.domain.spec import canonical_statement, parse_spec, statement_digest

SPEC = """# Title

- PARK-1~1: WHEN the exit time precedes the entry time THE SYSTEM SHALL raise
  ValueError.
- PARK-10~2: WHEN the amount is 0.00 THE SYSTEM SHALL print the receipt as "FREE".

Prose that is not a requirement.
"""


def edit_set(source, qualname="f", file="src/p/m.py"):
    return EditSet((Edit(file, qualname, source),))


def test_mutant_id_is_sixteen_hex_characters_of_the_sha256_of_the_canonical_normalized_edit_set():
    import hashlib

    es = edit_set("def f():\n    return 1\n")
    canonical = canonical_json(normalized_edit_set(es))
    assert mutant_id(es) == hashlib.sha256(canonical).hexdigest()[:16]
    assert re.fullmatch(r"[0-9a-f]{16}", mutant_id(es))


def test_mutant_id_ignores_comments_docstrings_and_layout():
    plain = edit_set("def f():\n    return 1\n")
    noisy = edit_set('def f():\n    """Doc."""\n    # note\n    return   1\n')
    assert mutant_id(plain) == mutant_id(noisy)


def test_mutant_id_changes_with_the_behaviour():
    assert mutant_id(edit_set("def f():\n    return 1\n")) != mutant_id(edit_set("def f():\n    return 2\n"))


def test_mutant_id_does_not_depend_on_the_order_of_edits():
    first, second = Edit("a.py", "f", "def f():\n    return 1\n"), Edit("b.py", "g", "def g():\n    return 2\n")
    assert mutant_id(EditSet((first, second))) == mutant_id(EditSet((second, first)))


def test_unparseable_new_source_still_has_an_id():
    assert re.fullmatch(r"[0-9a-f]{16}", mutant_id(edit_set("def f(:\n")))


def test_slug_replaces_every_character_outside_the_safe_set():
    assert slugify("tests/test_a.py::test_x[1]") == "tests_test_a.py__test_x_1_"


def test_spec_parsing_joins_continuation_lines_and_keeps_revisions():
    criteria = parse_spec(SPEC)
    assert [c.ref for c in criteria] == ["PARK-1~1", "PARK-10~2"]
    assert criteria[0].statement == "WHEN the exit time precedes the entry time THE SYSTEM SHALL raise ValueError."


def test_the_real_spec_has_ten_criteria(experiment_root):
    text = (experiment_root / "fixture" / "parkfee" / "spec" / "parking.md").read_text()
    assert [c.number for c in parse_spec(text)] == list(range(1, 11))


@given(st.text(alphabet=st.characters(blacklist_categories=("Cs",)), min_size=1))
def test_harmless_reformatting_keeps_the_statement_digest(text):
    reformatted = "  " + text.replace(" ", "   ").replace("\n", "\r\n") + "\n"
    assert statement_digest(reformatted) == statement_digest(text)


@given(st.text(alphabet="abc ", min_size=1), st.text(alphabet="xyz", min_size=1))
def test_a_real_edit_changes_the_statement_digest(text, addition):
    assert statement_digest(text + " " + addition) != statement_digest(text)


def test_canonical_statement_is_nfc_with_collapsed_whitespace():
    assert canonical_statement("é  x\n y ") == "é x y"
