from __future__ import annotations

from hypothesis import given
from hypothesis import strategies as st

from calibration.adapters.pilot_git import names_value

RUN_ID = "run-02"
KEYS = st.text(max_size=6)
LEAVES = st.one_of(st.none(), st.booleans(), st.integers(), st.text(max_size=8).filter(lambda text: text != RUN_ID))
DOCUMENTS = st.recursive(
    LEAVES,
    lambda children: st.one_of(st.lists(children, max_size=3), st.dictionaries(KEYS, children, max_size=3)),
    max_leaves=12,
)


@given(document=DOCUMENTS)
def test_a_document_without_the_value_does_not_name_it(document: object) -> None:
    assert not names_value(document, RUN_ID)


@given(document=DOCUMENTS, path=st.lists(st.one_of(KEYS, st.just(0)), max_size=4))
def test_the_value_is_found_at_any_depth(document: object, path: list[str | int]) -> None:
    nested: object = RUN_ID
    for step in reversed(path):
        nested = [nested] if step == 0 else {step: nested}
    assert names_value({"sibling": document, "target": nested}, RUN_ID)


def test_keys_do_not_count_as_names() -> None:
    assert not names_value({RUN_ID: 1}, RUN_ID)


def test_a_longer_string_does_not_name_the_value() -> None:
    assert not names_value(["run-021", "xrun-02"], RUN_ID)
