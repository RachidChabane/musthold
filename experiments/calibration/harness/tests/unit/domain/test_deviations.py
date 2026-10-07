from __future__ import annotations

import pytest

from calibration.domain.deviations import (
    DEVIATIONS_TITLE,
    KIND_IDENTIFIERS,
    MODEL_IDS,
    OPERATOR_KINDS,
    RULE_IDENTIFIERS,
    SCOPE_ALL,
    SCOPE_IDENTIFIERS,
    SCOPE_NONE,
    SOURCE_IDENTIFIERS,
    DeviationEntry,
    DeviationKind,
    InvalidDeviation,
    counts_by_kind,
    parse_deviations,
    parse_entry,
    parse_scope,
    render_deviations,
    standing_of,
)

FOREIGN_LINE = "the operator restarted the machine"


def _entry(kind: DeviationKind, scope: tuple[str, ...], text: str = "something happened") -> DeviationEntry:
    return DeviationEntry.create(kind, scope, text)


class TestScope:
    def test_the_identifiers_are_the_kinds_models_sources_and_rules_of_the_log(self) -> None:
        assert (*KIND_IDENTIFIERS, *MODEL_IDS, *SOURCE_IDENTIFIERS, *RULE_IDENTIFIERS) == SCOPE_IDENTIFIERS
        assert KIND_IDENTIFIERS == ("K1", "K2", "K3", "K4")
        assert MODEL_IDS == ("M1", "M2", "M3", "M4", "M5", "M6")
        assert SOURCE_IDENTIFIERS == ("JH5", "JH3", "VH2")
        assert RULE_IDENTIFIERS == ("CS1", "CS2", "CS3")

    def test_none_and_all_are_scopes(self) -> None:
        assert parse_scope(SCOPE_NONE) == (SCOPE_NONE,)
        assert parse_scope(SCOPE_ALL) == (SCOPE_ALL,)

    def test_identifiers_are_split_on_commas_and_put_in_canonical_order(self) -> None:
        assert parse_scope("M4 , K1,VH2") == ("K1", "M4", "VH2")

    def test_repeated_identifiers_are_kept_once(self) -> None:
        assert parse_scope("M1, M1") == ("M1",)

    @pytest.mark.parametrize("scope", ["M7", "K1, nowhere", "", "all, M1", "none, M1"])
    def test_a_scope_outside_the_closed_set_is_refused(self, scope: str) -> None:
        with pytest.raises(InvalidDeviation):
            parse_scope(scope)


class TestEntryGrammar:
    def test_an_entry_renders_in_the_documented_line_form(self) -> None:
        entry = _entry(DeviationKind.BLOCKED, ("M6", "K1"), "memory-fit")
        assert entry.render() == "- blocked | scope: K1, M6 | memory-fit"

    @pytest.mark.parametrize(
        "entry",
        [
            _entry(DeviationKind.PROVENANCE, (SCOPE_NONE,)),
            _entry(DeviationKind.PAUSE, (SCOPE_NONE,), "run window closed"),
            _entry(DeviationKind.BLOCKED, ("M5", "M6")),
            _entry(DeviationKind.PROTOCOL, (SCOPE_ALL,)),
            _entry(DeviationKind.PROTOCOL, ("CS1", "JH3")),
        ],
    )
    def test_rendering_then_parsing_round_trips(self, entry: DeviationEntry) -> None:
        assert parse_entry(entry.render()) == entry

    def test_a_document_round_trips(self) -> None:
        entries = [
            _entry(DeviationKind.PAUSE, (SCOPE_NONE,), "run window closed"),
            _entry(DeviationKind.PROTOCOL, (SCOPE_ALL,)),
        ]
        document = render_deviations(entries)
        assert document.startswith(DEVIATIONS_TITLE + "\n")
        assert parse_deviations(document) == entries

    def test_an_empty_document_has_no_entries(self) -> None:
        assert parse_deviations(render_deviations([])) == []

    def test_the_text_is_flattened_to_one_line(self) -> None:
        assert _entry(DeviationKind.BLOCKED, ("M1",), "two\nlines   here").text == "two lines here"

    def test_an_entry_needs_a_text(self) -> None:
        with pytest.raises(InvalidDeviation):
            _entry(DeviationKind.BLOCKED, ("M1",), "   ")

    @pytest.mark.parametrize("kind", [DeviationKind.PROVENANCE, DeviationKind.PAUSE])
    def test_pipeline_kinds_touch_no_output(self, kind: DeviationKind) -> None:
        with pytest.raises(InvalidDeviation):
            _entry(kind, ("M1",))

    @pytest.mark.parametrize("line", ["- unknown | scope: none | text", "blocked | scope: none | text", FOREIGN_LINE])
    def test_a_line_that_is_not_an_entry_is_refused_by_parse_entry(self, line: str) -> None:
        with pytest.raises(InvalidDeviation):
            parse_entry(line)


class TestFailClosedReading:
    def test_a_foreign_line_is_a_protocol_entry_with_scope_all(self) -> None:
        (entry,) = parse_deviations(f"{DEVIATIONS_TITLE}\n\n{FOREIGN_LINE}\n")
        assert (entry.kind, entry.scope, entry.text) == (DeviationKind.PROTOCOL, (SCOPE_ALL,), FOREIGN_LINE)
        assert not entry.well_formed

    def test_a_malformed_entry_is_read_the_same_way(self) -> None:
        (entry,) = parse_deviations("- blocked | scope: M9 | nowhere")
        assert entry.kind is DeviationKind.PROTOCOL
        assert entry.covers_all

    def test_the_title_and_blank_lines_are_not_entries(self) -> None:
        assert parse_deviations(f"{DEVIATIONS_TITLE}\n\n   \n") == []

    def test_a_foreign_line_makes_every_output_protocol_standing(self) -> None:
        entries = parse_deviations(FOREIGN_LINE)
        assert standing_of(entries, ["K2", "M1"]).protocol


class TestOperatorKinds:
    def test_only_blocked_and_protocol_are_operator_kinds(self) -> None:
        assert OPERATOR_KINDS == (DeviationKind.BLOCKED, DeviationKind.PROTOCOL)
        assert set(DeviationKind) - set(OPERATOR_KINDS) == {DeviationKind.PROVENANCE, DeviationKind.PAUSE}


class TestStanding:
    def test_no_entry_leaves_an_output_unaffected(self) -> None:
        standing = standing_of([], ["K1", "M1"])
        assert (standing.blocked, standing.protocol) == (False, False)

    def test_a_blocked_entry_in_scope_blocks_the_output(self) -> None:
        standing = standing_of([_entry(DeviationKind.BLOCKED, ("M6",))], ["K1", "M6", "JH5"])
        assert (standing.blocked, standing.protocol) == (True, False)

    def test_a_protocol_entry_in_scope_marks_the_output(self) -> None:
        standing = standing_of([_entry(DeviationKind.PROTOCOL, ("K4",))], ["K4", "M1"])
        assert (standing.blocked, standing.protocol) == (False, True)

    def test_an_entry_out_of_scope_has_no_effect(self) -> None:
        standing = standing_of([_entry(DeviationKind.BLOCKED, ("M6",))], ["K1", "M1"])
        assert not standing.blocked

    def test_scope_all_covers_every_output(self) -> None:
        standing = standing_of([_entry(DeviationKind.PROTOCOL, (SCOPE_ALL,))], ["K3", "M2"])
        assert standing.protocol

    def test_provenance_and_pause_entries_have_no_effect(self) -> None:
        entries = [_entry(DeviationKind.PAUSE, (SCOPE_NONE,)), _entry(DeviationKind.PROVENANCE, (SCOPE_NONE,))]
        standing = standing_of(entries, ["K1"])
        assert (standing.blocked, standing.protocol) == (False, False)

    def test_an_output_is_in_scope_when_any_of_its_identifiers_is_named(self) -> None:
        entry = _entry(DeviationKind.BLOCKED, ("JH3",))
        assert entry.touches(["K2", "JH3"])
        assert not entry.touches(["K2", "JH5"])


class TestCounts:
    def test_every_kind_is_counted_including_the_absent_ones(self) -> None:
        entries = [_entry(DeviationKind.BLOCKED, ("M1",)), _entry(DeviationKind.BLOCKED, ("M2",))]
        assert counts_by_kind(entries) == {"provenance": 0, "pause": 0, "blocked": 2, "protocol": 0}
