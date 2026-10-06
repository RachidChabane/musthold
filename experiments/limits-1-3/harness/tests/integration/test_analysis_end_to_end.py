from __future__ import annotations

import argparse
import json
from pathlib import Path

import pytest
from synthetic_run import RUN_ID, build_root

from pilot.cli.analysis_verbs import register
from pilot.analysis.lint import lint_report
from pilot.analysis.report import LIMITS_PARAGRAPH
from pilot.domain.analysis_constants import ENDPOINT_IDS

SMALL_B = "150"


def build_parser() -> tuple[argparse.ArgumentParser, dict]:
    parser = argparse.ArgumentParser()
    verbs = parser.add_subparsers(dest="verb", required=True)
    handlers: dict = {}

    def verb(name, handler, help_text):
        sub = verbs.add_parser(name, help=help_text)
        sub.add_argument("--root", type=Path, required=True)
        handlers[name] = handler
        return sub

    register(verb)
    return parser, handlers


def run_verb(*argv: str) -> int:
    parser, handlers = build_parser()
    args = parser.parse_args(list(argv))
    return handlers[args.verb](args)


def analyze(root: Path) -> int:
    return run_verb("analyze", "--root", str(root), "--run-id", RUN_ID, "--allow-drift",
                    "--resamples", SMALL_B, "--permutations", SMALL_B)


@pytest.fixture(scope="module")
def analysed(tmp_path_factory):
    root = tmp_path_factory.mktemp("synthetic-root")
    build_root(root)
    assert analyze(root) == 0
    return root


def test_analyze_writes_decisions_endpoints_tables_and_report(analysed):
    results = analysed / "results"
    decisions = json.loads((results / "decisions.json").read_text())
    assert [e["id"] for e in decisions["endpoints"]] == list(ENDPOINT_IDS)
    for endpoint in decisions["endpoints"]:
        assert {"id", "n", "point", "lo", "hi", "band_go", "band_nogo", "outcome", "capped_by", "attribution"} <= set(endpoint)
        assert endpoint["outcome"] in ("GO", "NO-GO", "INCONCLUSIVE")
    assert decisions["composite"]["verdict"] in ("REDESIGN", "PROCEED_TO_POWERED_STUDY", "INCONCLUSIVE")
    assert decisions["feasibility"]["verdict"] in ("PROTOCOL_READY", "AMEND", "STOP_AND_REDESIGN_PROTOCOL")
    assert decisions["run_id"] == RUN_ID
    assert (results / "endpoints.json").is_file()
    for name in ("confusion_label_x_signature_arm_T", "full_set_outcomes", "inter_rater_agreement", "cost", "failure_modes"):
        assert (results / f"{name}.csv").is_file()


def test_scenario_universe_is_134_plus_10_full_sets(analysed):
    report = (analysed / "results" / "REPORT.md").read_text()
    assert "134 (including 10 full sets)" in report


def test_report_has_every_section_the_limits_paragraph_and_three_template_forms(analysed):
    report = (analysed / "results" / "REPORT.md").read_text()
    assert LIMITS_PARAGRAPH in report
    headings = [line for line in report.splitlines() if line.startswith("## ")]
    assert [h.split(". ", 1)[1] for h in headings] == [
        "Provenance", "Conclusions", "Decision table", "Feasibility", "Preconditions",
        "Confusion matrices and descriptive measures", "Error multiplication and attribution", "Failure modes",
        "Cost", "Sensitivity analyses", "Limits", "Discussion (exploratory)",
    ]
    assert "<endpoint>" not in report and "<n_needed>" not in report and "<component>" not in report


def test_report_passes_lint(analysed):
    report = (analysed / "results" / "REPORT.md").read_text()
    assert lint_report(report) == []
    assert run_verb("report-lint", "--root", str(analysed)) == 0


def test_every_consensus_line_carries_its_primary_figure(analysed):
    report = (analysed / "results" / "REPORT.md").read_text()
    lines = [line for line in report.splitlines() if "CONSENSUS" in line]
    assert lines and all("PRIMARY" in line for line in lines)


def test_analysis_is_deterministic(analysed, tmp_path):
    other = tmp_path / "again"
    build_root(other)
    assert analyze(other) == 0
    first = json.loads((analysed / "results" / "decisions.json").read_text())
    second = json.loads((other / "results" / "decisions.json").read_text())
    assert first == second


def test_list_form_alignment_aggregate_is_read_like_the_nested_form(analysed, tmp_path):
    nested = tmp_path / "listed"
    build_root(nested, list_alignment=True)
    assert analyze(nested) == 0
    first = json.loads((analysed / "results" / "endpoints.json").read_text())
    second = json.loads((nested / "results" / "endpoints.json").read_text())
    assert first["endpoints"] == second["endpoints"]


def test_analyze_refuses_without_oracle_records(tmp_path):
    build_root(tmp_path)
    (tmp_path / "runs" / RUN_ID / "oracle_classes.json").unlink()
    assert analyze(tmp_path) == 1


def test_analyze_refuses_when_the_preregistration_is_not_committed(tmp_path):
    build_root(tmp_path)
    assert run_verb("analyze", "--root", str(tmp_path), "--run-id", RUN_ID) == 1


def test_analyze_refuses_non_default_resamples_without_allow_drift(tmp_path):
    build_root(tmp_path)
    assert run_verb("analyze", "--root", str(tmp_path), "--run-id", RUN_ID, "--resamples", SMALL_B) == 1


def test_audit_sample_is_seeded_stratified_and_includes_the_planted_items(tmp_path):
    run = build_root(tmp_path)
    assert run_verb("audit-sample", "--root", str(tmp_path), "--run-id", RUN_ID) == 0
    sample = json.loads((run / "audit_sample.json").read_text())
    assert sample["seed"] == 20261006
    planted = {m for m, s in sample["strata"].items() if s == "planted"}
    assert len(planted) == 30
    arm_t = [m for m, s in sample["strata"].items() if s != "planted"]
    assert 20 <= len(arm_t) <= 20 + 40
    assert set(sample["mutant_ids"]) == set(sample["strata"])
    first = (run / "audit_sample.json").read_text()
    run_verb("audit-sample", "--root", str(tmp_path), "--run-id", RUN_ID)
    assert (run / "audit_sample.json").read_text() == first


def test_precision_table_verb_writes_the_pre_registered_grid(tmp_path):
    out = tmp_path / "precision.csv"
    assert run_verb("precision-table", "--root", str(tmp_path), "--out", str(out)) == 0
    rows = out.read_text().splitlines()
    assert rows[0].startswith("table,n,requested_proportion,k,proportion")
    assert len(rows) == 1 + 10 * 4 + sum(n + 1 for n in (3, 7, 14, 24, 38))


def test_report_lint_flags_forbidden_words_and_unpaired_consensus(tmp_path):
    report = tmp_path / "REPORT.md"
    report.write_text("The pilot works.\nCONSENSUS accuracy 0.9\nPRIMARY and CONSENSUS 0.8 0.9\n")
    assert run_verb("report-lint", "--root", str(tmp_path), "--report", str(report)) == 1
    findings = [str(f) for f in lint_report(report.read_text())]
    assert any("works" in f for f in findings)
    assert any("line 2" in f for f in findings)
    assert not any("line 3" in f for f in findings)


def test_a_protocol_deviation_with_forbidden_words_is_not_echoed_and_caps_the_endpoints(tmp_path):
    run = build_root(tmp_path)
    (run / "DEVIATIONS.md").write_text(
        "# Deviations\n\n- protocol | endpoints: all | harness_digest changed from a to b; this works and was confirmed\n"
    )
    assert analyze(tmp_path) == 0
    report = (tmp_path / "results" / "REPORT.md").read_text()
    assert lint_report(report) == []
    assert "this works" not in report
    decisions = json.loads((tmp_path / "results" / "decisions.json").read_text())
    p6 = next(p for p in decisions["preconditions"] if p["id"] == "P6")
    assert p6["status"] == "fail" and p6["affects"][0] == "D1"
    assert all("P6" in e["capped_by"] for e in decisions["endpoints"])


def test_a_quota_blocked_entry_forces_its_endpoints_inconclusive(tmp_path):
    run = build_root(tmp_path)
    (run / "DEVIATIONS.md").write_text(
        "# Deviations\n\n- quota-blocked | endpoints: D9 | alignment stage could not complete\n"
    )
    assert analyze(tmp_path) == 0
    decisions = json.loads((tmp_path / "results" / "decisions.json").read_text())
    d9 = next(e for e in decisions["endpoints"] if e["id"] == "D9")
    assert d9["outcome"] == "INCONCLUSIVE" and d9["capped_by"] == ["P7"]
    assert all(not e["capped_by"] for e in decisions["endpoints"] if e["id"] != "D9")


def test_routine_provenance_and_quota_pause_entries_cap_nothing(tmp_path):
    run = build_root(tmp_path)
    (run / "DEVIATIONS.md").write_text(
        "# Deviations\n\n"
        "- provenance | endpoints: none | CLI claude changed from 2.1.292 to 2.1.293\n"
        "- quota-pause | endpoints: none | run-core stopped at a quota or rate limit; a rerun resumes it\n"
    )
    assert analyze(tmp_path) == 0
    decisions = json.loads((tmp_path / "results" / "decisions.json").read_text())
    statuses = {p["id"]: p["status"] for p in decisions["preconditions"]}
    assert statuses["P6"] == "pass" and statuses["P7"] == "pass"
    assert all(not e["capped_by"] for e in decisions["endpoints"])


@pytest.mark.parametrize(
    ("filename", "old", "new"),
    [
        ("processing.jsonl", '"status": "OK"', '"status": "evaluated"'),
        ("kills.jsonl", '"killed"', '"dead"'),
        ("reference.json", '"passed"', '"pass"'),
    ],
)
def test_unknown_record_values_are_rejected_instead_of_read_as_something_else(tmp_path, filename, old, new):
    run = build_root(tmp_path)
    path = run / filename
    path.write_text(path.read_text().replace(old, new))
    assert analyze(tmp_path) == 1
    assert not (tmp_path / "results" / "REPORT.md").exists()


def test_oracle_classify_and_audit_sample_fail_cleanly_on_an_unreadable_run(tmp_path):
    run = build_root(tmp_path)
    (run / "processing.jsonl").write_text('{"mutant_id": "x", "stillborn": false, "status": "evaluated"}\n')
    assert run_verb("oracle-classify", "--root", str(tmp_path), "--run-id", RUN_ID) == 1
    assert run_verb("audit-sample", "--root", str(tmp_path), "--run-id", RUN_ID) == 1
    (run / "processing.jsonl").unlink()
    assert run_verb("audit-sample", "--root", str(tmp_path), "--run-id", RUN_ID) == 1
