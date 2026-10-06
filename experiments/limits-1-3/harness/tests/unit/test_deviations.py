"""DEVIATIONS.md grammar shared by the pipeline writer and the P6/P7 reader (PREREG 4.4, 10)."""

from __future__ import annotations

from types import SimpleNamespace

import pytest
from hypothesis import given, strategies as st

from pilot.adapters.run_directory import RunDirectory
from pilot.analysis.preconditions import Status, p6_deviations, p7_quota
from pilot.cli.main import main
from pilot.domain.deviations import (
    DeviationEntry,
    DeviationKind,
    InvalidDeviation,
    parse_deviations,
    parse_entry,
    parse_scope,
)
from pilot.domain.endpoint_ids import ENDPOINT_IDS

TITLE = "# Deviations\n\n"


def context(text: str) -> SimpleNamespace:
    return SimpleNamespace(run=SimpleNamespace(deviations=text))


def preconditions(*lines: str) -> tuple:
    ctx = context(TITLE + "".join(f"{line}\n" for line in lines))
    return p6_deviations(ctx), p7_quota(ctx)  # type: ignore[arg-type]


# ---------------------------------------------------------------- grammar


def test_each_kind_renders_on_one_line_with_its_scope():
    assert DeviationEntry.provenance("CLI claude changed").render() == "- provenance | endpoints: none | CLI claude changed"
    assert DeviationEntry.quota_pause("paused").render() == "- quota-pause | endpoints: none | paused"
    assert DeviationEntry.protocol_for_all("fixture changed").render() == "- protocol | endpoints: all | fixture changed"
    blocked = DeviationEntry.create(DeviationKind.QUOTA_BLOCKED, ("D11", "D9"), "J-H\nincomplete")
    assert blocked.render() == "- quota-blocked | endpoints: D9, D11 | J-H incomplete"


def test_scopes_are_none_all_or_known_endpoint_ids_in_endpoint_order():
    assert parse_scope("none") == ()
    assert parse_scope("all") == ENDPOINT_IDS
    assert parse_scope("D10,D3 , D3") == ("D3", "D10")
    for invalid in ("", "D12", "D3, every", "ALL"):
        with pytest.raises(InvalidDeviation):
            parse_scope(invalid)


@pytest.mark.parametrize(
    ("kind", "endpoints"),
    [
        (DeviationKind.PROVENANCE, ("D1",)),
        (DeviationKind.QUOTA_PAUSE, ENDPOINT_IDS),
        (DeviationKind.QUOTA_BLOCKED, ()),
    ],
)
def test_a_scope_the_kind_does_not_allow_is_rejected(kind, endpoints):
    with pytest.raises(InvalidDeviation):
        DeviationEntry.create(kind, endpoints, "text")


def test_an_empty_text_is_rejected():
    with pytest.raises(InvalidDeviation):
        DeviationEntry.provenance("  \n ")


def test_a_text_may_contain_the_field_separator():
    entry = DeviationEntry.create(DeviationKind.PROTOCOL, ("D3",), "prompt a | b changed")
    assert parse_entry(entry.render()) == entry


kinds_and_scopes = st.one_of(
    st.tuples(st.sampled_from([DeviationKind.PROVENANCE, DeviationKind.QUOTA_PAUSE]), st.just(())),
    st.tuples(
        st.sampled_from([DeviationKind.QUOTA_BLOCKED, DeviationKind.PROTOCOL]),
        st.sets(st.sampled_from(ENDPOINT_IDS), min_size=1).map(tuple),
    ),
    st.tuples(st.just(DeviationKind.PROTOCOL), st.just(())),
)
texts = st.text(st.characters(blacklist_categories=("Cs",)), min_size=1).filter(lambda t: t.split())


@given(kinds_and_scopes, texts)
def test_render_then_parse_returns_the_entry(kind_and_scope, text):
    kind, endpoints = kind_and_scope
    entry = DeviationEntry.create(kind, endpoints, text)
    assert parse_deviations(TITLE + entry.render() + "\n") == [entry]


def test_reading_fails_closed_on_any_line_outside_the_grammar():
    entries = parse_deviations(TITLE + "- D3 inputs changed\nplain prose\n- surprise | endpoints: D3 | x\n")
    assert [(e.kind, e.endpoints, e.well_formed) for e in entries] == [
        (DeviationKind.PROTOCOL, ENDPOINT_IDS, False)
    ] * 3


def test_the_title_and_blank_lines_are_not_entries():
    assert parse_deviations(TITLE + "\n   \n") == []


# ---------------------------------------------------------------- P6 and P7, one test per kind


def test_no_entry_passes_p6_and_p7():
    p6, p7 = preconditions()
    assert p6.status is Status.PASS and p7.status is Status.PASS


def test_a_provenance_record_affects_no_precondition():
    p6, p7 = preconditions("- provenance | endpoints: none | CLI claude changed from 2.1.292 to 2.1.293")
    assert (p6.status, p6.affects, p7.status) == (Status.PASS, (), Status.PASS)


def test_a_quota_pause_affects_no_precondition():
    p6, p7 = preconditions("- quota-pause | endpoints: none | run-core stopped at a quota or rate limit")
    assert (p6.status, p7.status, p7.affects) == (Status.PASS, Status.PASS, ())


def test_a_quota_blocked_entry_fails_p7_for_its_endpoints_only():
    p6, p7 = preconditions("- quota-blocked | endpoints: D9, D11 | J-H could not complete")
    assert p6.status is Status.PASS
    assert (p7.status, p7.affects, p7.forces_inconclusive) == (Status.FAIL, ("D9", "D11"), True)


def test_a_protocol_deviation_fails_p6_for_the_endpoints_it_names():
    p6, p7 = preconditions(
        "- protocol | endpoints: D3 | canary fix: effort flag renamed",
        "- protocol | endpoints: D10, D4 | another fix",
    )
    assert (p6.status, p6.affects, p7.status) == (Status.FAIL, ("D3", "D4", "D10"), Status.PASS)


def test_a_protocol_deviation_scoped_to_no_endpoint_passes_p6():
    p6, _ = preconditions("- protocol | endpoints: none | rerun after a P4 infrastructure failure")
    assert p6.status is Status.PASS


def test_an_unparseable_line_fails_p6_for_every_endpoint():
    p6, _ = preconditions("- quota exhausted, D9 affected")
    assert (p6.status, p6.affects) == (Status.FAIL, ENDPOINT_IDS)
    assert "unparseable" in p6.detail


# ---------------------------------------------------------------- operator verb


def test_log_deviation_appends_a_valid_entry_and_rejects_an_invalid_one(tmp_path):
    common = ["log-deviation", "--root", str(tmp_path), "--run-id", "r1"]
    assert main([*common, "--kind", "quota-blocked", "--endpoints", "D9", "--text", "J-X incomplete"]) == 0
    assert main([*common, "--kind", "protocol", "--endpoints", "D12", "--text", "x"]) == 1
    with pytest.raises(SystemExit):
        main([*common, "--kind", "provenance", "--endpoints", "none", "--text", "a routine record"])
    text = RunDirectory(tmp_path / "runs" / "r1").read_text("DEVIATIONS.md")
    assert parse_deviations(text) == [DeviationEntry.create(DeviationKind.QUOTA_BLOCKED, ("D9",), "J-X incomplete")]


def test_a_quota_stop_of_a_run_verb_is_logged_as_a_quota_pause(tmp_path):
    from argparse import Namespace

    from pilot.cli.pipeline_commands import _guarded, _run_quota_log
    from pilot.domain.quota import QuotaExhausted

    def stop() -> int:
        raise QuotaExhausted("validity")

    args = Namespace(root=tmp_path, run_id="r1")
    assert _guarded("run-core", stop, _run_quota_log(args)) != 0
    [entry] = parse_deviations(RunDirectory(tmp_path / "runs" / "r1").read_text("DEVIATIONS.md"))
    assert entry.kind is DeviationKind.QUOTA_PAUSE and entry.endpoints == ()
    assert "run-core" in entry.text and "validity" in entry.text
