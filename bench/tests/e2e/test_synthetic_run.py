from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any, cast

import pytest

from specbench.adapters.run_store import RunStore
from specbench.adapters.schemas import SchemaName, schema_problems
from specbench.analysis.lint import lint_report
from specbench.analysis.report import render_report
from specbench.analysis.results import analyze
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.canonical import JsonValue, canonical_results_json
from specbench.domain.manifest import Manifest
from support.builders import DIGEST, PROVENANCE
from support.synthetic_run import synthetic_trunks, write_synthetic_run

GOLDEN = Path(__file__).parent.parent / "golden"
UPDATE = os.environ.get("SPECBENCH_UPDATE_GOLDEN") == "1"


def manifest() -> Manifest:
    return Manifest(
        run_id="sample-synthetic-01",
        seed=7,
        prereg_tag="prereg-sample-1",
        provenance=PROVENANCE,
        corpus_lock_digest=DIGEST,
        prices_lock_digest=DIGEST,
        os_name="darwin",
        toolchains={"go": "1.26.1", "python": "3.13.14"},
        harness_models={h: FROZEN_MODEL[h] for h in Harness},
        sample_tasks=tuple(f"T{i}" for i in range(1, 7)),
    )


def run_pipeline(root: Path) -> tuple[str, str]:
    layout = write_synthetic_run(root, manifest())
    store = RunStore(layout)
    results = analyze(store.read_trunks())
    results_json = canonical_results_json(results)
    report = render_report(parse_canonical_results(results_json))
    store.write_results(results_json, report)
    return layout.results.read_text(), layout.report.read_text()


def parse_canonical_results(text: str) -> dict[str, JsonValue]:
    return cast(dict[str, JsonValue], json.loads(text))


def test_synthetic_records_conform_to_the_trunk_schema() -> None:
    assert all(schema_problems(SchemaName.TRUNK, record) == [] for record in synthetic_trunks())


def test_the_pipeline_is_byte_identical_across_runs(tmp_path: Path) -> None:
    first = run_pipeline(tmp_path / "one")
    second = run_pipeline(tmp_path / "two")
    assert first == second


def test_results_and_report_match_the_golden_files(tmp_path: Path) -> None:
    results, report = run_pipeline(tmp_path / "run")
    if UPDATE:
        GOLDEN.mkdir(exist_ok=True)
        (GOLDEN / "results.json").write_text(results)
        (GOLDEN / "REPORT.md").write_text(report)
    assert results == (GOLDEN / "results.json").read_text()
    assert report == (GOLDEN / "REPORT.md").read_text()


def test_results_are_canonical_and_descriptive_per_harness(tmp_path: Path) -> None:
    results, _ = run_pipeline(tmp_path / "run")
    document = parse_canonical_float_free(results)
    assert document["label"] == "descriptive"
    assert set(document["harnesses"]) == {"claude-code", "codex"}
    assert document["sample"]["repositories"] == 6


def parse_canonical_float_free(text: str) -> dict[str, Any]:
    return cast(dict[str, Any], json.loads(text))


def test_the_report_passes_its_lint_and_never_pools_harnesses(tmp_path: Path) -> None:
    _, report = run_pipeline(tmp_path / "run")
    assert lint_report(report) == []
    assert "## Harness claude-code" in report and "## Harness codex" in report
    assert "## Harness by arm interaction" in report
    assert report.count("Sample: ") >= 3


@pytest.mark.parametrize(
    ("text", "fragment"),
    [
        ("# R\n\nSample: 1 task; descriptive.\n\nThe gate proves the effect.\n", "forbidden word 'proves'"),
        ("## S\n\n| a |\n|---|\n| 1 |\n", "table without a sample line"),
        ("## S\n\nSample: 1 task.\n\n| a |\n|---|\n| 1 |\n", "not labelled descriptive"),
        ("## S\n\nSample: 1 task; descriptive.\n\nA pooled figure over harnesses.\n", "across harnesses"),
    ],
)
def test_lint_findings(text: str, fragment: str) -> None:
    assert any(fragment in str(finding) for finding in lint_report(text))


def test_quoted_passages_are_exempt_from_the_word_list() -> None:
    quoted = "# R\n\n<!-- quoted:begin -->\nThe vendor says it proves everything.\n<!-- quoted:end -->\n"
    assert lint_report(quoted) == []
