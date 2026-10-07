"""The machine-safety summary (PREREG 15.2 item 12): deferrals, pressure stops, heavy-work waits, pauses, blocks."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from pathlib import Path

from calibration.analysis.safety_summary import safety_summary
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import SAFETY_FILE
from calibration.domain.deviations import DeviationEntry, DeviationKind
from calibration.domain.jsonread import json_object
from calibration.domain.models import MODEL_IDS


def _write(run_dir: Path, events: Sequence[Mapping[str, object]], *, blank_line: bool = False) -> None:
    lines = [json.dumps(event) for event in events]
    if blank_line:
        lines.insert(1, "  ")
    (run_dir / SAFETY_FILE).write_text("\n".join(lines) + "\n", encoding="utf-8")


def _model(summary: Mapping[str, JsonValue], model: str) -> Mapping[str, JsonValue]:
    return json_object(json_object(summary["per_model"], "per_model")[model], model)


def _entry(kind: DeviationKind, scope: list[str]) -> DeviationEntry:
    return DeviationEntry.create(kind, scope, "a text")


class TestSafetySummary:
    def test_a_run_without_a_safety_log_has_zero_counts_for_every_model(self, tmp_path: Path) -> None:
        summary = safety_summary(tmp_path, [])
        assert set(json_object(summary["per_model"], "per_model")) == set(MODEL_IDS)
        assert _model(summary, "M1") == {"deferrals": 0, "pressure_stops": 0, "blocked": False}
        assert (summary["pauses"], summary["heavy_work_waits"]) == (0, 0)

    def test_only_deferred_fit_checks_count_as_deferrals(self, tmp_path: Path) -> None:
        events = [
            {"event": "fit-check", "model": "M2", "decision": "defer"},
            {"event": "fit-check", "model": "M2", "decision": "defer"},
            {"event": "fit-check", "model": "M2", "decision": "run"},
            {"event": "fit-check", "model": "M3", "decision": "defer"},
        ]
        _write(tmp_path, events)
        summary = safety_summary(tmp_path, [])
        assert _model(summary, "M2")["deferrals"] == 2
        assert _model(summary, "M3")["deferrals"] == 1

    def test_pressure_stops_are_counted_per_model(self, tmp_path: Path) -> None:
        _write(tmp_path, [{"event": "pressure-stop", "model": "M4"}, {"event": "pressure-stop", "model": "M4"}])
        assert _model(safety_summary(tmp_path, []), "M4")["pressure_stops"] == 2

    def test_only_unclear_heavy_work_checks_are_waits(self, tmp_path: Path) -> None:
        events = [
            {"event": "heavy-work-check", "clear": False},
            {"event": "heavy-work-check", "clear": True},
            {"event": "heavy-work-check", "clear": False},
        ]
        _write(tmp_path, events)
        assert safety_summary(tmp_path, [])["heavy_work_waits"] == 2

    def test_blank_lines_and_unknown_events_are_ignored(self, tmp_path: Path) -> None:
        _write(
            tmp_path, [{"event": "window", "model": "M1"}, {"event": "pressure-stop", "model": "M1"}], blank_line=True
        )
        assert _model(safety_summary(tmp_path, []), "M1")["pressure_stops"] == 1

    def test_an_event_without_a_model_counts_for_no_model(self, tmp_path: Path) -> None:
        _write(tmp_path, [{"event": "pressure-stop"}, {"event": "fit-check", "decision": "defer"}])
        summary = safety_summary(tmp_path, [])
        assert all(_model(summary, model)["pressure_stops"] == 0 for model in MODEL_IDS)


class TestBlocksAndPauses:
    def test_a_blocked_entry_marks_the_models_of_its_scope(self, tmp_path: Path) -> None:
        summary = safety_summary(tmp_path, [_entry(DeviationKind.BLOCKED, ["M5", "K3"])])
        assert [m for m in MODEL_IDS if _model(summary, m)["blocked"]] == ["M5"]

    def test_a_blocked_entry_for_all_marks_every_model(self, tmp_path: Path) -> None:
        summary = safety_summary(tmp_path, [DeviationEntry.create(DeviationKind.BLOCKED, ["all"], "no pilot input")])
        assert all(_model(summary, m)["blocked"] for m in MODEL_IDS)

    def test_other_kinds_do_not_block(self, tmp_path: Path) -> None:
        entries = [_entry(DeviationKind.PROTOCOL, ["M1"]), _entry(DeviationKind.PROVENANCE, ["none"])]
        summary = safety_summary(tmp_path, entries)
        assert not any(_model(summary, m)["blocked"] for m in MODEL_IDS)

    def test_pauses_are_counted_over_the_whole_log(self, tmp_path: Path) -> None:
        pause = _entry(DeviationKind.PAUSE, ["none"])
        assert safety_summary(tmp_path, [pause, pause, _entry(DeviationKind.BLOCKED, ["M1"])])["pauses"] == 2
