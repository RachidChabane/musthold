"""Contract: shapes of imports.json, exploratory_status.json, call_budget.json and the pause entries."""

from __future__ import annotations

import json

from pilot.domain.call_budget import CALL_BUDGET_PAUSE_TEXT
from pilot.domain.deviations import DeviationEntry, DeviationKind, parse_deviations
from pilot.domain.endpoint_ids import ENDPOINT_IDS
from pilot.domain.exploratory import STREAMS, CallStatus
from pilot.domain.import_manifest import ImportEntry, ImportPlan, ImportRole
from pilot.domain.run_window import RUN_WINDOW_PAUSE_TEXT


def test_imports_json_has_the_documented_keys():
    entry = ImportEntry("raw/a/result.json", "raw/a/result.json", "0" * 64, ImportRole.RELABEL, "a")
    record = ImportPlan((entry,), ("retired",), {"a": "n"}).to_mapping("1" * 64)
    assert set(record) == {"source_run", "sums_digest", "entries", "retired", "nonces"}
    assert set(record["entries"][0]) == {"source_path", "target_path", "digest", "role", "call_id"}
    json.dumps(record)


def test_exploratory_status_values_and_streams_are_closed_sets():
    assert {s.value for s in CallStatus} == {"complete", "blocked", "pending", "missing"} and STREAMS == ("luna", "secondary")


def test_pause_entries_follow_the_deviation_grammar_and_touch_no_endpoint():
    for text in (CALL_BUDGET_PAUSE_TEXT, RUN_WINDOW_PAUSE_TEXT):
        line = DeviationEntry.quota_pause(text).render()
        (parsed,) = parse_deviations(f"# Deviations\n\n{line}\n")
        assert parsed.kind is DeviationKind.QUOTA_PAUSE and parsed.endpoints == () and parsed.well_formed
    assert ENDPOINT_IDS == tuple(f"D{n}" for n in range(1, 12))
    assert CALL_BUDGET_PAUSE_TEXT == "call budget reached" and RUN_WINDOW_PAUSE_TEXT == "run window closed"
