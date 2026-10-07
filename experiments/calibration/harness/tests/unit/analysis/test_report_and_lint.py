from __future__ import annotations

import pytest

from calibration.analysis.lint import FORBIDDEN_WORDS, expected_band_rows, lint_report
from calibration.analysis.report import render_report
from calibration.analysis.report_text import (
    CONCLUSION_BEGIN,
    CONCLUSION_END,
    LIMITS,
    SECTIONS,
    SOURCE2_NOTE,
    VENDOR_FIGURES,
)
from support.decisions_fixture import decisions

EXPECTED_CELLS = 31


@pytest.fixture(scope="module")
def report() -> str:
    return render_report(decisions(), None)


def test_a_rendered_report_has_the_sixteen_sections_in_order(report: str) -> None:
    headings = [line for line in report.splitlines() if line.startswith("## ")]
    assert [heading.split(". ", 1)[1] for heading in headings] == list(SECTIONS)


def test_a_rendered_report_passes_its_own_lint(report: str) -> None:
    assert lint_report(report) == []


def test_the_band_table_holds_every_cell(report: str) -> None:
    assert len(expected_band_rows()) == EXPECTED_CELLS
    for kind, source, model in expected_band_rows():
        assert f"| {kind} | {source} | {model} |" in report


def test_the_limits_paragraph_is_verbatim(report: str) -> None:
    for limit in LIMITS:
        assert f"- {limit}" in report


def test_the_vendor_figures_stay_in_their_table(report: str) -> None:
    section = report.split("## 14. ")[1].split("## 15. ", maxsplit=1)[0]
    for _, _, figure, _ in VENDOR_FIGURES:
        assert figure in section
        assert figure not in report.replace(section, "")


def test_a_missing_band_row_fails_the_lint(report: str) -> None:
    lines = [line for line in report.splitlines() if not line.startswith("| K1 | S1 | M3 |")]
    findings = lint_report("\n".join(lines))
    assert any("K1 / S1 / M3" in finding.message for finding in findings)


@pytest.mark.parametrize("word", FORBIDDEN_WORDS)
def test_every_forbidden_word_fails_as_a_whole_word_in_any_case(report: str, word: str) -> None:
    findings = lint_report(report + f"\nThis {word.upper()} is not allowed.\n")
    assert any("forbidden word" in finding.message for finding in findings)


def test_a_forbidden_word_inside_another_word_is_allowed(report: str) -> None:
    assert lint_report(report + "\nThe confirmed rows are listed.\n") == []


def test_the_adr_phrase_and_conclusion_sentences_are_exempt(report: str) -> None:
    extra = f"\nADR 0003 says to confirm ALIGNED.\n{CONCLUSION_BEGIN}\n- this proves it\n{CONCLUSION_END}\n"
    assert lint_report(report + extra) == []


def test_a_consensus_row_without_its_primary_fails(report: str) -> None:
    broken = report + "\n\n| Variant |\n|---|\n| CONSENSUS |\n"
    assert any("CONSENSUS" in finding.message for finding in lint_report(broken))


def test_a_returned_probability_row_without_its_server_fails(report: str) -> None:
    broken = report + "\n\n| Measure |\n|---|\n| returned probability |\n"
    assert any("server" in finding.message for finding in lint_report(broken))


def test_a_source2_table_without_the_note_fails(report: str) -> None:
    broken = report.replace(SOURCE2_NOTE, "")
    assert any("fixed note" in finding.message for finding in lint_report(broken))


def test_a_vendor_figure_outside_its_table_fails(report: str) -> None:
    assert any("vendor" in finding.message for finding in lint_report(report + "\nAccuracy 73.3 % here.\n"))


def test_an_exploratory_discussion_beyond_three_hundred_words_fails(report: str) -> None:
    longer = report + " ".join(["word"] * 400)
    assert any("300 words" in finding.message for finding in lint_report(longer))


def test_a_missing_section_fails(report: str) -> None:
    broken = report.replace("## 12. Machine safety", "## 12. Something else")
    assert any("Machine safety" in finding.message for finding in lint_report(broken))


def test_the_report_with_proposals_lists_the_entries() -> None:
    policy = {
        "entries": [
            {
                "rule": "CS1",
                "model": "M3",
                "policyKey": "tauScreen",
                "source": "system-one:m@d",
                "override": {"system-one:m@d": 0.2},
            }
        ],
        "reported": [],
        "recommendation": {"model": "M1", "reason": "fixture"},
    }
    rendered = render_report(decisions(), policy)
    assert "tauScreen" in rendered
    assert lint_report(rendered) == []


def test_the_report_is_deterministic() -> None:
    assert render_report(decisions(), None) == render_report(decisions(), None)
