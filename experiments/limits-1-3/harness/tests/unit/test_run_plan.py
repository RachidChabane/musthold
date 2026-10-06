"""The run plan: call ids and nonces fixed before any call, no id or nonce shared (ADR 0006)."""

from __future__ import annotations

import itertools

from hypothesis import given, strategies as st

from pilot.domain.import_manifest import (
    OPUS_GENERATION_IDS,
    RETIRED_CALL_IDS,
    TERRA_GENERATION_IDS,
)
from pilot.domain.run_plan import SECTIONS, RunPlan, build_plan

REFS = [f"PARK-{n}~1" for n in range(1, 11)]
IMPORTED = {call_id: f"original-{call_id}" for call_id in OPUS_GENERATION_IDS + TERRA_GENERATION_IDS}


def counter():
    numbers = itertools.count()
    return lambda: f"fresh-{next(numbers)}"


def test_the_plan_has_the_calls_of_the_adr_table():
    plan = build_plan(REFS, IMPORTED, counter())
    assert len(plan.arm_t) == 50 and {c.model for c in plan.arm_t} == {"claude-sonnet-5-5"}
    assert sorted({c.call_id.rsplit("__", 1)[1] for c in plan.arm_t}) == ["10", "6", "7", "8", "9"]
    assert len(plan.contrast_x) == 30 and sum(c.imported for c in plan.contrast_x) == 10
    assert len(plan.terra_x) == 5 and all(c.imported and c.cli == "codex" for c in plan.terra_x)
    assert len(plan.writers) == 20 and {c.writer for c in plan.writers} == {"S", "O"}
    assert len(plan.luna) == 20 and {c.model for c in plan.luna} == {"gpt-6-luna"}
    assert {c.call_id for c in plan.luna if c.kind == "write_tests"} == {f"write_tests__{r}__L" for r in REFS}
    assert {c.call_id for c in plan.luna if c.kind == "generate"} == {f"xgen__{r}__L" for r in REFS}


def test_imported_entries_keep_their_original_nonces_and_new_entries_get_fresh_ones():
    plan = build_plan(REFS, IMPORTED, counter())
    for call in plan.contrast_x + plan.terra_x:
        assert (call.nonce == IMPORTED.get(call.call_id)) == call.imported
    assert [c.call_id for c in plan.contrast_x if c.imported] == [
        c for c in sorted(OPUS_GENERATION_IDS, key=lambda i: (REFS.index(i.split("__")[1]), i))
    ]


def test_no_planned_call_id_equals_an_imported_or_retired_raw_directory_except_the_imported_entries():
    plan = build_plan(REFS, IMPORTED, counter())
    fresh = [c.call_id for c in plan.all_calls() if not c.imported]
    assert not set(fresh) & (set(IMPORTED) | set(RETIRED_CALL_IDS))
    assert len(fresh) == len(set(fresh))


def test_the_retired_call_is_never_planned_again():
    plan = build_plan(REFS, IMPORTED, counter())
    assert not {c.call_id for c in plan.all_calls()} & set(RETIRED_CALL_IDS)


def test_every_call_id_and_every_nonce_is_unique_across_sections():
    plan = build_plan(REFS, IMPORTED, counter())
    calls = plan.all_calls()
    assert len({c.call_id for c in calls}) == len(calls) == 50 + 30 + 5 + 20 + 20
    assert len({c.nonce for c in calls}) == len(calls)


def test_without_imports_every_contrast_call_is_new_and_there_is_no_terra_source():
    plan = build_plan(REFS, {}, counter())
    assert len(plan.contrast_x) == 30 and not any(c.imported for c in plan.contrast_x) and plan.terra_x == ()


def test_the_plan_round_trips_through_its_json_form():
    plan = build_plan(REFS, IMPORTED, counter())
    assert RunPlan.from_mapping(plan.to_mapping()) == plan
    assert set(plan.to_mapping()) == set(SECTIONS)


@given(st.lists(st.sampled_from(REFS), min_size=1, max_size=10, unique=True))
def test_a_plan_over_any_criteria_has_no_nonce_reuse(refs):
    plan = build_plan(refs, {}, counter())
    nonces = [c.nonce for c in plan.all_calls()]
    assert len(nonces) == len(set(nonces))
