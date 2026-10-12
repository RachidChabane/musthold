import json
from datetime import UTC, datetime

from demos1.domain.deviations import Deviation, DeviationKind, counts_by_kind
from demos1.domain.environment import drift, pin_mismatches
from demos1.domain.probe import cpython_by_minor, pretty_name
from demos1.domain.report_lint import lint
from demos1.domain.scan_output import finding_file, parse_headline, statement_facts
from support.scans import HEADLINE, finding, statement


class TestScanOutputs:
    def test_the_headline_gives_what_scan_json_does_not_hold(self):
        headline = parse_headline(HEADLINE)
        assert (headline.depth, headline.budget, headline.workers) == ("full", "1h0m0s", 2)
        assert (headline.sampled, headline.green) == (30, 38)
        assert (headline.mutants_ran, headline.mutants_proven) == (120, 64)
        assert (headline.mutants_not_run, headline.mutants_did_not_build, headline.mutants_drawn) == (11, 3, 134)
        assert headline.no_model and headline.promises_not_checked
        first, second = headline.findings
        assert first.test == "tests/test_totals.py::test_total_accepts_any_rate"
        assert first.sentence == "passes on all 6 proven changes in lines it ran"
        assert (first.site_file, first.site_line) == ("ledger/totals.py", 41)
        assert second.test == "tests/test_rates.py::test_rate[high value]"
        assert second.sentence.endswith("; 1 more stopped it from starting")

    def test_a_quick_scan_without_budget_and_partial(self):
        text = "x @ 1 · go · 3 tests\n  quick scan · no budget · 1s · 3 of 3 green tests sampled · 1 worker — run with --full for the whole suite\n  partial: the budget stopped the scan before every mutant ran\n"
        headline = parse_headline(text)
        assert (headline.depth, headline.budget, headline.workers, headline.partial) == ("quick", "no budget", 1, True)
        assert not headline.no_model

    def test_scan_json_facts(self):
        facts = statement_facts(json.loads(statement()))
        assert (facts.sampled_green, facts.green, facts.tests) == (30, 38, 40)
        assert (facts.mutants_ran, facts.mutants_proven) == (106, 64)
        assert facts.seed == "7"
        assert not facts.partial

    def test_finding_json_keeps_the_test_and_the_route(self):
        record = finding_file(json.loads(finding("tests/test_a.py::test_a", killer="tests/test_a.py::test_b")))
        assert record.test == "tests/test_a.py::test_a"
        assert record.route.kind == "killed_by_test"
        assert record.route.test == "tests/test_a.py::test_b"


class TestReportLint:
    def test_quoted_scan_output_is_exempt(self):
        markdown = "\n".join(
            [
                "```text",
                "the bug is safe 12/40",
                "```",
                "> clean 9/9 %",
                "A `pkg/safe/x_test.go` path and `3/4` in code.",
            ]
        )
        assert lint(markdown, []) == []

    def test_words_percent_and_fractions(self):
        markdown = "\n".join(
            [
                "This is a Bug.",
                "Totally clean-up.",
                "The PASS rate.",
                "pass is fine in lower case.",
                "Opened by Copilot.",
                "It was written by someone.",
                "50% of it.",
                "2/30 verify nothing, 5/7 not listed.",
            ]
        )
        violations = lint(markdown, ["2/30"])
        found = {(violation.line, violation.text) for violation in violations}
        assert (1, "bug") in found
        assert (2, "clean") in found
        assert (3, "PASS") in found
        assert not any(line == 4 for line, _ in found)
        assert (5, "by Copilot") in found
        assert (6, "written by") in found
        assert (7, "%") in found
        assert (8, "5/7") in found
        assert (8, "2/30") not in found


class TestDeviationsAndEnvironment:
    def test_counts_per_kind_from_the_file(self):
        at = datetime(2026, 10, 13, 9, tzinfo=UTC)
        document = "# Deviations of demos-1\n\n" + "".join(
            Deviation(at, kind, "B-go-01", "says", "happened").render()
            for kind in (DeviationKind.WAIT_BOUND, DeviationKind.WAIT_BOUND, DeviationKind.DEPARTURE)
        )
        counts = counts_by_kind(document)
        assert counts["wait-bound"] == 2
        assert counts["departure"] == 1
        assert counts["select-stopped"] == 0

    def test_pins_and_drift(self):
        facts = {"image_go_version": "go1.26.4", "image_uv_version": "0.11.23", "gh_version": "2.93.0"}
        assert pin_mismatches(facts) == []
        assert [mismatch.fact for mismatch in pin_mismatches({**facts, "gh_version": "2.94.0"})] == ["gh_version"]
        changed = drift({**facts, "image_digest": "sha256:a"}, {**facts, "image_digest": "sha256:b"})
        assert [mismatch.fact for mismatch in changed] == ["image_digest"]

    def test_uv_releases_per_minor(self):
        listing = json.dumps(
            [
                {"implementation": "cpython", "version": "3.13.7", "variant": "default"},
                {"implementation": "cpython", "version": "3.13.6", "variant": "default"},
                {"implementation": "cpython", "version": "3.13.8", "variant": "freethreaded"},
                {"implementation": "cpython", "version": "3.14.0rc2", "variant": "default"},
                {"implementation": "pypy", "version": "3.11.13", "variant": "default"},
                {"implementation": "cpython", "version": "3.10.18", "variant": "default"},
            ]
        )
        assert cpython_by_minor(listing) == {"3.13": "3.13.7", "3.10": "3.10.18"}
        assert pretty_name('NAME="Debian"\nPRETTY_NAME="Debian GNU/Linux 12 (bookworm)"\n') == (
            "Debian GNU/Linux 12 (bookworm)"
        )
