"""`run-blindspot` and `run-luna` over the mini fixture with a scripted model (ADR 0006)."""

from __future__ import annotations

import json
import re
import shutil
from datetime import UTC, datetime
from pathlib import Path

import pytest
from dataclasses import replace

from test_core_run import FakeModel, baselines, build, edits, make_world, rows

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.process import local_process_runner
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.adapters.pytest_runner import PytestJunitRunner
from pilot.adapters.suite_runner import WriterSuiteRunners
from pilot.application.blind_spot_stage import BlindSpotDependencies, BlindSpotPreconditionFailed, run_blindspot
from pilot.application.core_run import run_core
from pilot.application.exploratory_calls import ExploratoryLedger
from pilot.application.model_caller import ModelCaller
from pilot.application.luna_stage import LunaDependencies, run_luna
from pilot.application.test_writing_stage import load_suites
from pilot.domain.call_type import CallType
from pilot.domain.exploratory import Window
from pilot.domain.model_call import CanaryBaseline
from pilot.domain.model_slate import Writer

pytestmark = pytest.mark.integration
MINI = Path(__file__).resolve().parents[1] / "fixtures" / "mini"
IMPORTS = "import pytest\nfrom datetime import datetime, timedelta\nfrom decimal import Decimal\nfrom parkfee import quote\n"
START = "datetime(2026, 3, 2, 9, 0)"
SUITES = {
    "PARK-1~1": [
        ("test_exit_before_entry_raises", "    with pytest.raises(ValueError):\n        quote({s}, {s} - timedelta(minutes=1))\n"),
        ("test_equal_times_do_not_raise", "    assert quote({s}, {s}) == Decimal('0.00')\n"),
    ],
    "PARK-2~1": [
        ("test_free_at_fifteen_minutes", "    assert quote({s}, {s} + timedelta(minutes=15)) == Decimal('0.00')\n"),
        ("test_free_for_ten_minutes", "    assert quote({s}, {s} + timedelta(minutes=10)) == Decimal('0.00')\n"),
    ],
    "PARK-3~1": [
        ("test_one_hour_for_sixteen_minutes", "    assert quote({s}, {s} + timedelta(minutes=16)) == Decimal('2.50')\n"),
        ("test_two_hours_for_sixty_one_minutes", "    assert quote({s}, {s} + timedelta(minutes=61)) == Decimal('5.00')\n"),
        ("test_wrongly_expects_free_after_an_hour", "    assert quote({s}, {s} + timedelta(minutes=61)) == Decimal('0.00')\n"),
        ("test_imports_numpy", "    import numpy\n    assert numpy\n"),
    ],
}


def written(ref):
    tests = []
    for name, body in SUITES[ref]:
        text = (IMPORTS if "numpy" not in name else "") + f"\n\ndef {name}():\n" + body.format(s=START)
        tests.append({"name": name, "source": text})
    return {"tests": tests}


class WritingModel(FakeModel):
    """Adds the writer payloads; `offset` makes its generated mutants differ from another instance's."""

    def __init__(self, offset=0, results=None):
        super().__init__()
        self.offset = offset
        self.results = results if results is not None else {}

    def is_recorded(self, call_id):
        return call_id in self.results

    def call(self, request, baseline):
        if request.call_id not in self.results:
            self.results[request.call_id] = super().call(request, baseline)
        return self.results[request.call_id]

    def _generate(self, request):
        n = len(self.requests) + self.offset
        return {"mutants": [
            {"slot": "S1", "mechanism_summary": "rate", "edits": edits(
                "_hourly_charge", f'def _hourly_charge(minutes):\n    return Decimal("2.{10 + n}") * -(-minutes // 60)\n')},
            {"slot": "S2", "mechanism_summary": "boundary", "edits": edits(
                "quote", f"def quote(entry, exit):\n    if exit < entry:\n        raise ValueError('exit before entry')\n"
                f"    minutes = (exit - entry) // timedelta(minutes=1)\n    if minutes <= FREE_MINUTES + {n}:\n"
                f"        return Decimal('0.00')\n    return _hourly_charge(minutes)\n")},
            {"slot": "S3", "mechanism_summary": "receipt", "edits": edits(
                "format_receipt", f'def format_receipt(amount):\n    if amount == 0:\n        return "FREE"\n    return f"EUR{n} {{amount:.2f}}"\n')},
        ]}

    _xgen = _generate

    def _write_tests(self, request):
        return written(re.search(r"\n\n(PARK-\d+~1): ", request.item).group(1))


@pytest.fixture(scope="module")
def completed(tmp_path_factory):
    base = tmp_path_factory.mktemp("blindspot")
    world = make_world(MINI, base)
    deps, _ = build(world, WritingModel(), base)
    run_core(deps)
    return world


def blind(world, model, tmp_path, book=None):
    deps, logs = build(world, model, tmp_path)
    caller = ModelCaller(model, baselines() if book is None else book, "system", logs.append, 2, recorded=model.is_recorded)
    deps = replace(deps, ctx=replace(deps.ctx, caller=caller))
    workspace = ProjectWorkspace(world / "fixture" / "parkfee", tmp_path / "suite-work", AstEditApplier())
    return BlindSpotDependencies(deps, WriterSuiteRunners(workspace, PytestJunitRunner(local_process_runner()))), logs


@pytest.fixture(scope="module")
def ran(completed, tmp_path_factory):
    base = tmp_path_factory.mktemp("blind-run")
    world = base / "world"
    shutil.copytree(completed, world)
    model = WritingModel(offset=500)
    deps, logs = blind(world, model, base)
    run_blindspot(deps)
    return deps, model, world, logs


def test_the_blind_spot_calls_are_the_new_contrast_generations_and_the_two_writers(ran):
    _, model, _, _ = ran
    ids = [r.call_id for r in model.requests]
    assert sorted(i for i in ids if i.startswith("generate")) == sorted(f"generate__PARK-{n}~1__{k}" for n in (1, 2, 3) for k in (1, 2, 3))
    assert sorted(i for i in ids if i.startswith("write_tests")) == sorted(f"write_tests__PARK-{n}~1__{w}" for n in (1, 2, 3) for w in "SO")
    assert not any(i.startswith("xgen") or i.startswith("write_tests__PARK-1~1__L") for i in ids)
    writer_requests = [r for r in model.requests if r.call_id.startswith("write_tests")]
    assert all(r.effort.value == "high" for r in writer_requests)
    assert {r.pin.model for r in writer_requests if r.call_id.endswith("__S")} == {"claude-sonnet-5-5"}
    assert {r.pin.model for r in writer_requests if r.call_id.endswith("__O")} == {"claude-opus-5-5"}


def test_the_writer_input_has_the_spec_target_source_and_api_but_no_tests_or_labels(ran):
    _, model, _, _ = ran
    item = next(r.item for r in model.requests if r.call_id == "write_tests__PARK-2~1__S")
    assert "PARK-2~1: WHEN a stay lasts 15 minutes or less" in item and "=== src/parkfee/tariff.py ===" in item
    assert "=== Public API ===" in item and "def quote(entry: datetime, exit: datetime) -> Decimal:" in item
    assert "test_mini" not in item and "verifies" not in item and "oracle" not in item.lower()


def test_each_test_is_statically_checked_then_filtered_against_the_reference(ran):
    deps, _, _, _ = ran
    suites = load_suites(deps.core.ctx.store, Writer.SONNET)
    third = {t.name: (t.status.value, t.reason) for t in suites["PARK-3~1"].tests}
    assert third["test_one_hour_for_sixteen_minutes"][0] == "kept"
    assert third["test_wrongly_expects_free_after_an_hour"][0] == "dropped_reference"
    assert third["test_imports_numpy"][0] == "rejected_static" and "not allowed" in third["test_imports_numpy"][1]
    assert all(s.reference_checked for s in suites.values())
    assert [t.name for t in suites["PARK-3~1"].kept] == ["test_one_hour_for_sixteen_minutes", "test_two_hours_for_sixty_one_minutes"]


def test_the_suite_module_carries_the_marker_the_harness_adds(ran):
    deps, _, _, _ = ran
    module = load_suites(deps.core.ctx.store, Writer.OPUS)["PARK-2~1"].module()
    assert module.count("@pytest.mark.verifies('PARK-2~1')") == 2 and "import pytest" in module


def test_mutants_of_every_source_are_processed_through_f0_to_f2_without_a_differential(ran):
    deps, _, world, _ = ran
    store = deps.core.ctx.store
    documents = [json.loads(p.read_text()) for p in (world / "runs" / "r1" / "mutants").glob("*.json")]
    arms = {d["arm"] for d in documents}
    assert {"T", "P", "X", "O"} <= arms
    processed = {r["mutant_id"] for r in rows(store, "processing.jsonl")}
    assert {d["mutant_id"] for d in documents} <= processed
    differential = {r["mutant_id"] for r in rows(store, "differential.jsonl")}
    assert not differential & {d["mutant_id"] for d in documents if d["arm"] in {"X", "O", "R", "L"}}


def test_membership_is_recorded_per_source_even_for_ids_shared_with_another_arm(ran):
    deps, _, world, _ = ran
    sources = deps.core.ctx.store.read_json("mutant_sources.json")
    assert set(sources) == {"S", "O", "op", "terra"}
    arms = {json.loads(p.read_text())["mutant_id"]: json.loads(p.read_text())["arm"] for p in (world / "runs" / "r1" / "mutants").glob("*.json")}
    assert set(sources["S"]) == {m for m, a in arms.items() if a == "T"}
    assert len(sources["O"]) > 0 and len(sources["op"]) > 0 and sources["terra"] == []
    assert set(sources["O"]) | set(sources["op"]) <= set(arms)


def test_writer_kill_vectors_cover_every_evaluated_mutant_for_both_writers(ran):
    deps, _, _, _ = ran
    store = deps.core.ctx.store
    evaluated = {r["mutant_id"] for r in rows(store, "processing.jsonl") if not r["stillborn"] and r["status"] == "OK"}
    kill_rows = rows(store, "writer_kills.jsonl")
    assert {(r["writer"], r["mutant_id"]) for r in kill_rows} == {(w, m) for w in "SO" for m in evaluated}
    some = next(r for r in kill_rows if r["writer"] == "S")
    assert all(node.startswith("tests/test_park_") and "::" in node for node in some["results"])
    assert set(some["results"].values()) <= {"killed", "survived", "error", "flaky"}


def test_a_kill_of_the_written_tests_is_reproduced_on_the_mutant_that_breaks_the_rate(ran):
    deps, _, _, _ = ran
    store = deps.core.ctx.store
    rate = [r for r in rows(store, "kills.jsonl") if any(v == "killed" for v in r["results"].values())]
    killing = {r["mutant_id"] for r in rows(store, "writer_kills.jsonl") if r["writer"] == "S" and "killed" in r["results"].values()}
    assert killing and killing & {r["mutant_id"] for r in rate}


def test_a_second_run_makes_no_call_and_changes_no_record(ran, tmp_path):
    deps, _, world, _ = ran
    before = {p.name: p.read_text() for p in (world / "runs" / "r1").glob("*.json*") if p.name != "timing.json"}
    model = WritingModel(offset=500, results=deps.core.ctx.caller._port.results)
    again, logs = blind(world, model, tmp_path)
    run_blindspot(again)
    assert model.requests == []
    after = {p.name: p.read_text() for p in (world / "runs" / "r1").glob("*.json*") if p.name != "timing.json"}
    assert after == before


def test_the_stage_requires_the_core_run(tmp_path):
    world = make_world(MINI, tmp_path)
    deps, _ = blind(world, WritingModel(), tmp_path)
    with pytest.raises(BlindSpotPreconditionFailed, match="run run-core"):
        run_blindspot(deps)


NOW_OPEN = datetime(2026, 10, 12, tzinfo=UTC)
LUNA = Window(datetime(2026, 10, 10, 9, 1, tzinfo=UTC), datetime(2026, 10, 17, 23, 59, tzinfo=UTC))


def luna(world, model, tmp_path, now, canaried=None):
    book = baselines()

    def canary():
        if canaried is not None:
            canaried.append(True)
        book.update({("gpt-6-luna__codex", call_type): CanaryBaseline(100, None) for call_type in CallType})

    deps, _ = blind(world, model, tmp_path, book)
    store = deps.core.ctx.store
    return LunaDependencies(deps, ExploratoryLedger(store), canary, LUNA, lambda: now), store


def test_luna_runs_inside_its_window_calls_codex_with_the_requested_slug_and_records_status(ran, tmp_path):
    _, _, shared, _ = ran
    world = tmp_path / "world"
    shutil.copytree(shared, world)
    model = WritingModel()
    canaried = []
    luna_deps, store = luna(world, model, tmp_path, NOW_OPEN, canaried)
    run_luna(luna_deps)
    ids = [r.call_id for r in model.requests]
    assert sorted(ids) == sorted([f"write_tests__PARK-{n}~1__L" for n in (1, 2, 3)] + [f"xgen__PARK-{n}~1__L" for n in (1, 2, 3)])
    assert {r.pin.model for r in model.requests} == {"gpt-6-luna"} and {r.pin.cli.value for r in model.requests} == {"codex"}
    assert canaried == [True]
    status = store.read_json("exploratory_status.json")["luna"]
    assert set(status) == set(ids) and set(status.values()) == {"complete"}
    assert "luna" not in (world / "runs" / "r1" / "DEVIATIONS.md").read_text().lower()
    sources = store.read_json("mutant_sources.json")
    assert len(sources["luna"]) > 0
    writers = {r["writer"] for r in rows(store, "writer_kills.jsonl")}
    assert writers == {"S", "O", "L"}


def test_luna_refuses_before_its_window_opens_and_writes_nothing(ran, tmp_path):
    _, _, shared, _ = ran
    world = tmp_path / "world"
    shutil.copytree(shared, world)
    model = WritingModel()
    luna_deps, store = luna(world, model, tmp_path, datetime(2026, 10, 9, tzinfo=UTC))
    from pilot.application.exploratory_calls import WindowNotOpen

    with pytest.raises(WindowNotOpen):
        run_luna(luna_deps)
    assert model.requests == [] and not store.exists("exploratory_status.json")


def test_after_the_window_closes_no_call_is_made_and_every_luna_call_is_blocked(ran, tmp_path):
    _, _, shared, _ = ran
    world = tmp_path / "world"
    shutil.copytree(shared, world)
    model = WritingModel()
    luna_deps, store = luna(world, model, tmp_path, datetime(2026, 10, 18, tzinfo=UTC))
    run_luna(luna_deps)
    assert model.requests == []
    status = store.read_json("exploratory_status.json")["luna"]
    assert len(status) == 6 and set(status.values()) == {"blocked"}
    assert "luna" not in (world / "runs" / "r1" / "DEVIATIONS.md").read_text().lower()


def test_a_failed_luna_canary_blocks_every_luna_call_without_a_deviation_or_a_call(ran, tmp_path):
    from pilot.application.canary import CanaryFailed

    _, _, shared, _ = ran
    world = tmp_path / "world"
    shutil.copytree(shared, world)
    model = WritingModel()
    luna_deps, store = luna(world, model, tmp_path, NOW_OPEN)

    def failing():
        raise CanaryFailed("gpt-6-luna__codex/generate: payload violates schema")

    from dataclasses import replace as _replace

    run_luna(_replace(luna_deps, run_canary=failing))
    assert model.requests == []
    status = store.read_json("exploratory_status.json")["luna"]
    assert len(status) == 6 and set(status.values()) == {"blocked"}
    assert "luna" not in (world / "runs" / "r1" / "DEVIATIONS.md").read_text().lower()
