"""Audit allocation, oracle classes, lint, precision table, precondition helpers, template parsing."""

from __future__ import annotations

import pytest
from hypothesis import given, strategies as st

from pilot.analysis.audit import allocate, draw_sample, stratum_of
from pilot.analysis.lint import lint_report
from pilot.analysis.oracle import OracleKind, classify_oracle
from pilot.analysis.precision import count_at, precision_table
from pilot.analysis.preconditions import modal_guess, pins_of_lock
from pilot.analysis.report import TemplateError, parse_conclusion_templates
from pilot.analysis.stats import wilson
from pilot.domain.analysis_constants import REPORT_CONCLUSION_BEGIN, REPORT_CONCLUSION_END
from pilot.domain.refs import criterion_ref, parse_criterion, parse_criterion_or_none

# ---------------------------------------------------------------- oracle classification


def test_oracle_classes_follow_the_failing_criteria():
    assert classify_oracle((3,), None).valid_for(3)
    assert not classify_oracle((3,), None).valid_for(4)
    assert classify_oracle((3, 5), None).kind is OracleKind.COLLATERAL
    assert not classify_oracle((3, 5), None).valid_for(3)
    assert classify_oracle((), "NO_DIFF_FOUND").kind is OracleKind.SILENT_EQUIVALENT_PROBABLE
    assert classify_oracle((), "DIFFERS").kind is OracleKind.SILENT_DIFFERENT
    assert classify_oracle((), "SKIPPED").kind is OracleKind.SILENT_UNTESTED
    assert classify_oracle((), None).kind.is_silent
    assert classify_oracle((3, 3), None).valid_for(3)


# ---------------------------------------------------------------- audit sample


def test_allocation_gives_every_non_empty_cell_its_floor_and_shares_the_rest_proportionally():
    allocation = allocate({"a": 40, "b": 4, "c": 0, "d": 2}, total=20, floor=3)
    assert allocation["d"] == 2 and allocation["b"] >= 3 and "c" not in allocation
    assert sum(allocation.values()) == 20
    assert allocation["a"] > allocation["b"]


@given(
    sizes=st.dictionaries(st.sampled_from("abcd"), st.integers(0, 60), min_size=1),
    total=st.integers(0, 40),
)
def test_allocation_never_exceeds_cell_sizes_and_hits_the_total_when_possible(sizes, total):
    allocation = allocate(sizes, total, 3)
    assert all(0 < n <= sizes[cell] for cell, n in allocation.items())
    floors = sum(min(3, n) for n in sizes.values())
    capacity = sum(sizes.values())
    assert sum(allocation.values()) == max(floors, min(total, capacity))


def test_the_sample_is_seeded_and_adds_survivors_and_planted_items_with_their_strata():
    cells = {stratum_of(True, True): [f"v{i}" for i in range(30)], stratum_of(False, False): [f"n{i}" for i in range(10)]}
    first = draw_sample(cells, ["v1", "extra"], ["p1", "p2"], seed=20261006)
    again = draw_sample(cells, ["v1", "extra"], ["p1", "p2"], seed=20261006)
    other = draw_sample(cells, ["v1", "extra"], ["p1", "p2"], seed=1)
    assert first == again and first.mutant_ids != other.mutant_ids
    assert first.strata["p1"] == "planted" and first.strata["extra"] == "survived-all-valid"
    assert sum(1 for s in first.strata.values() if s.startswith("pipeline-")) >= 20
    assert list(first.mutant_ids) == sorted(first.mutant_ids)


# ---------------------------------------------------------------- lint


def test_lint_flags_each_forbidden_word_case_insensitively_as_whole_words():
    for word in ("works", "proves", "proven", "validated", "confirm", "human audit", "generalizes"):
        assert lint_report(f"It {word.upper()} here.")
    assert lint_report("The tool works.")
    assert not lint_report("tests/test_tickets.py::test_validated_ticket_covers_two_hour_stay")
    assert not lint_report("The witness was confirmed by execution and workshops ran.")


def test_lint_ignores_quoted_template_lines_but_not_the_lines_around_them():
    text = "<!-- quoted:begin -->\nthis works\n<!-- quoted:end -->\nthis works"
    findings = lint_report(text)
    assert [f.line for f in findings] == [4]


def test_lint_requires_primary_on_every_consensus_line():
    assert lint_report("CONSENSUS 0.8")
    assert not lint_report("CONSENSUS 0.8 against PRIMARY 0.7")
    assert not lint_report("| PRIMARY | CONSENSUS |")


def test_lint_bounds_the_exploratory_section_to_300_words():
    long_text = "# Report\n## Discussion (exploratory)\n" + "word " * 301 + "\n## Next\nshort"
    ok_text = "# Report\n## Discussion (exploratory)\n" + "word " * 300 + "\n## Next\nshort"
    assert lint_report(long_text) and not lint_report(ok_text)


# ---------------------------------------------------------------- precision table


def test_precision_table_has_the_grid_and_the_full_count_rows():
    table = precision_table()
    grid = [r for r in table.rows if r[0] == "grid"]
    full = [r for r in table.rows if r[0] == "full"]
    assert len(grid) == 10 * 4 and len(full) == sum(n + 1 for n in (3, 7, 14, 24, 38))
    assert {r[2] for r in grid} == {0.0, 0.5, 0.8, 1.0}
    assert {r[1] for r in grid} == {3, 7, 10, 14, 20, 24, 30, 38, 52, 82}
    row = next(r for r in full if r[1] == 38 and r[3] == 19)
    assert (row[5], row[6]) == pytest.approx((wilson(19, 38).lo, wilson(19, 38).hi))
    assert count_at(0.5, 3) == 2 and count_at(0.8, 10) == 8 and count_at(1.0, 52) == 52


# ---------------------------------------------------------------- helpers


def test_criterion_reference_helpers_round_trip():
    assert criterion_ref(10) == "PARK-10~1" and parse_criterion("PARK-10~1") == 10
    assert parse_criterion_or_none(None) is None and parse_criterion_or_none("junk") is None
    with pytest.raises(ValueError):
        parse_criterion("PARK-3")


def test_modal_guess_breaks_ties_by_mean_confidence_then_lexicographically():
    assert modal_guess([("TARGETED", 60), ("TARGETED", 50), ("OTHER", 90)]) == "TARGETED"
    assert modal_guess([("TARGETED", 60), ("OTHER", 70)]) == "OTHER"
    assert modal_guess([("TARGETED", 60), ("OTHER", 60)]) == "OTHER"


def test_pins_come_from_the_models_lock():
    lock = {"models": [{"model": "m1", "cli": "claude"}, {"model": "m2", "cli": "codex"}]}
    assert pins_of_lock(lock) == [("m1", "claude"), ("m2", "codex")]


def test_the_conclusion_templates_are_read_verbatim_from_the_template_file():
    from pathlib import Path

    template = (Path(__file__).resolve().parents[3] / "prereg" / "REPORT.tmpl.md").read_text()
    found = parse_conclusion_templates(template)
    assert found["GO"] == (
        "<endpoint> met its pre-registered band at pilot scale (estimate [lo, hi], n). The fixture is author-built "
        "(10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories."
    )
    assert found["NO-GO"] == (
        "<endpoint> fell in its pre-registered NO-GO band (estimate [lo, hi]); <component> must be redesigned "
        "before a powered study."
    )
    assert found["INCONCLUSIVE"] == (
        "The pilot cannot decide <endpoint> (estimate [lo, hi]); a decision needs about <n_needed> units."
    )


def test_a_template_missing_a_conclusion_form_is_rejected():
    broken = f'{REPORT_CONCLUSION_BEGIN}\n- GO: "x"\n{REPORT_CONCLUSION_END}\n'
    with pytest.raises(TemplateError):
        parse_conclusion_templates(broken)


def test_p4_requires_one_canary_per_pair_of_the_lock_not_one_per_pin():
    import json
    from pathlib import Path

    from pilot.analysis.preconditions import canary_pairs

    lock = json.loads((Path(__file__).resolve().parents[3] / "prereg" / "models.lock.json").read_text())
    pairs = canary_pairs(lock)
    assert len(pairs) == 9 and {m for m, _, _ in pairs} == {"claude-sonnet-5-5", "claude-opus-5-5"}
    assert ("claude-opus-5-5", "claude", "audit") in pairs and ("gpt-6-luna", "codex", "generate") not in pairs
