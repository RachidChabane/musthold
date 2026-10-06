from __future__ import annotations

import json
import threading
import time
from pathlib import Path

import pytest

from pilot.adapters.models_lock import (
    DirectoryCanaryStore,
    InstalledCliVersions,
    JsonSchemaValidator,
    load_baselines,
    load_models_lock,
    record_cli_versions,
)
from pilot.adapters.process import ProcessResult
from pilot.adapters.prompt_files import DirectoryPromptLibrary
from pilot.application.canary import CanaryFailed, CanaryTask, plan_canary, run_canary
from pilot.domain.call_type import CallType
from pilot.domain.canary_items import CANARY_TOKENS
from pilot.application.model_router import RoutingModelPort
from pilot.domain.model_call import (
    CanaryBaseline,
    Cli,
    Effort,
    MissingResult,
    ModelPin,
    ModelProvenance,
    ModelRequest,
    ModelResult,
    TokenUsage,
)

SCHEMA = '{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}'
SONNET = ModelPin("claude-sonnet-5-5", Cli.CLAUDE)
TERRA = ModelPin("gpt-5.6-terra", Cli.CODEX)
VERSIONS = {Cli.CLAUDE: "2.1.292", Cli.CODEX: "0.155.1"}
SYSTEM = "system"


class FakeModel:
    def __init__(self, outcomes):
        self.outcomes = outcomes
        self.requests = []

    def call(self, request, baseline):
        self.requests.append((request, baseline))
        return self.outcomes[(request.pin.key, request.call_id.rsplit("__", 1)[1])]


class FakeProbe:
    def __init__(self, versions):
        self.versions = versions

    def installed_version(self, cli):
        return self.versions[cli]


class MemoryStore:
    def __init__(self):
        self.saved = {}

    def save(self, pin, call_type, baseline, cli_version):
        self.saved[(pin.key, call_type)] = (baseline, cli_version)


PREREG = Path(__file__).resolve().parents[3] / "prereg"


def real_prompts():
    return DirectoryPromptLibrary(
        PREREG / "prompts", PREREG / "schemas", PREREG / "prompts" / "system.txt", PREREG / "codebook.md"
    )


def ok(tokens, turns, payload=None):
    return ModelResult(payload if payload is not None else {"ok": True}, TokenUsage(tokens, 5), None, turns, 1, ModelProvenance.REPORTED)


def task(pin, call_type):
    return CanaryTask(pin, call_type, "item", SCHEMA)


def test_canary_records_a_baseline_per_model_and_call_type_without_checking_the_bound():
    model = FakeModel({
        (SONNET.key, "probe"): ok(1174, 2), (SONNET.key, "validity"): ok(7000, 2), (TERRA.key, "probe"): ok(8627, None),
    })
    store = MemoryStore()
    tasks = [task(SONNET, CallType.PROBE), task(SONNET, CallType.VALIDITY), task(TERRA, CallType.PROBE)]
    report = run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), store, tasks, SYSTEM)
    assert report.baselines == {
        (SONNET.key, CallType.PROBE): CanaryBaseline(1174, 2),
        (SONNET.key, CallType.VALIDITY): CanaryBaseline(7000, 2),
        (TERRA.key, CallType.PROBE): CanaryBaseline(8627, None),
    }
    assert store.saved[(TERRA.key, CallType.PROBE)] == (CanaryBaseline(8627, None), "0.155.1")
    assert report.versions == VERSIONS
    assert all(baseline is None for _, baseline in model.requests)
    assert {r.call_id for r, _ in model.requests} == {
        f"canary/{SONNET.key}__probe", f"canary/{SONNET.key}__validity", f"canary/{TERRA.key}__probe",
    }


def test_canary_uses_the_effort_of_the_call_type():
    model = FakeModel({(SONNET.key, "generate"): ok(1, 1), (SONNET.key, "probe"): ok(1, 1)})
    run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(),
               [task(SONNET, CallType.GENERATE), task(SONNET, CallType.PROBE)], SYSTEM)
    assert [r.effort for r, _ in model.requests] == [Effort.HIGH, Effort.MEDIUM]


def test_a_version_different_from_the_lock_is_recorded_not_an_abort():
    model = FakeModel({(SONNET.key, "probe"): ok(1, 1)})
    report = run_canary(model, FakeProbe({Cli.CLAUDE: "2.1.293"}), JsonSchemaValidator(), MemoryStore(),
                        [task(SONNET, CallType.PROBE)], SYSTEM)
    assert report.versions == {Cli.CLAUDE: "2.1.293"}


def test_canary_aborts_when_the_call_is_missing():
    missing = MissingResult(("claude exited 1",), ("modelUsage keys",))
    model = FakeModel({(SONNET.key, "probe"): missing})
    with pytest.raises(CanaryFailed, match="modelUsage keys"):
        run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(), [task(SONNET, CallType.PROBE)], SYSTEM)


def test_canary_aborts_when_the_payload_violates_the_schema():
    model = FakeModel({(SONNET.key, "probe"): ok(1, 1, payload={"ok": "yes"})})
    with pytest.raises(CanaryFailed, match="violates schema"):
        run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(), [task(SONNET, CallType.PROBE)], SYSTEM)


def test_router_dispatches_by_cli():
    claude, codex = FakeModel({(SONNET.key, "probe"): ok(1, 1)}), FakeModel({(TERRA.key, "probe"): ok(2, None)})
    router = RoutingModelPort({Cli.CLAUDE: claude, Cli.CODEX: codex})
    run_canary(router, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(),
               [task(SONNET, CallType.PROBE), task(TERRA, CallType.PROBE)], SYSTEM)
    assert len(claude.requests) == 1 and len(codex.requests) == 1


ROLES = {
    SONNET: ("validity_judge_v_s", "alignment_judge_j_sonnet"),
    TERRA: ("mutant_generator", "relabel_oai", "leak_audit", "alignment_judge_j_gpt"),
}


def test_the_plan_covers_the_call_types_each_model_actually_uses():
    tasks = plan_canary(ROLES, real_prompts())
    assert [(t.pin.model, t.call_type.value) for t in tasks] == [
        ("claude-sonnet-5-5", "validity"), ("claude-sonnet-5-5", "alignment"), ("claude-sonnet-5-5", "probe"),
        ("gpt-5.6-terra", "generate"), ("gpt-5.6-terra", "alignment"), ("gpt-5.6-terra", "probe"),
        ("gpt-5.6-terra", "relabel"), ("gpt-5.6-terra", "leak_audit"),
    ]


def test_the_plan_can_be_narrowed_by_model_and_call_type():
    tasks = plan_canary(ROLES, real_prompts(), models=["gpt-5.6-terra"], call_types=[CallType.PROBE])
    assert [(t.pin.model, t.call_type) for t in tasks] == [("gpt-5.6-terra", CallType.PROBE)]


def test_the_alignment_canary_uses_the_ten_canary_tokens_in_its_schema():
    (alignment,) = plan_canary(ROLES, real_prompts(), models=["claude-sonnet-5-5"], call_types=[CallType.ALIGNMENT])
    enum = json.loads(alignment.schema_text)["properties"]["verifies"]["enum"]
    assert enum == [*CANARY_TOKENS, "NONE"] and enum[0] == "K-0001" and enum[9] == "K-0010"


def test_no_canary_item_mentions_the_fixture():
    for task in plan_canary(ROLES, real_prompts()):
        if task.call_type is not CallType.GENERATE:
            assert "parkfee" not in task.item.lower()


def test_models_lock_loads_pins_and_versions(tmp_path):
    path = tmp_path / "models.lock.json"
    path.write_text(json.dumps({
        "cli_versions": {"claude": "2.1.292", "codex": "0.155.1"},
        "models": [{"model": "claude-sonnet-5-5", "cli": "claude", "roles": []}],
    }))
    lock = load_models_lock(path)
    assert lock.cli_versions == VERSIONS and lock.pins == (SONNET,)


def test_committed_models_lock_matches_the_pins_of_the_protocol(experiment_root):
    lock = load_models_lock(experiment_root / "prereg" / "models.lock.json")
    assert {(p.model, p.cli) for p in lock.pins} == {
        ("claude-opus-5-5", Cli.CLAUDE),
        ("claude-sonnet-5-5", Cli.CLAUDE),
        ("gpt-5.6-terra", Cli.CODEX),
    }


def test_canary_store_writes_one_file_per_model_cli_and_call_type(tmp_path):
    DirectoryCanaryStore(tmp_path / "canary").save(TERRA, CallType.AUDIT, CanaryBaseline(8627, None), "0.155.1")
    saved = json.loads((tmp_path / "canary" / "gpt-5.6-terra__codex__audit.json").read_text())
    assert saved == {"input_tokens": 8627, "num_turns": None, "cli_version": "0.155.1"}
    assert load_baselines(tmp_path / "canary") == {(TERRA.key, CallType.AUDIT): CanaryBaseline(8627, None)}


def test_cli_versions_are_recorded_in_the_lock_only_when_they_differ(tmp_path):
    lock = tmp_path / "models.lock.json"
    lock.write_text(json.dumps({"cli_versions": {"claude": "2.1.292", "codex": "0.155.1"}, "models": []}))
    assert not record_cli_versions(lock, {Cli.CLAUDE: "2.1.292"})
    assert record_cli_versions(lock, {Cli.CLAUDE: "2.1.300"})
    assert json.loads(lock.read_text())["cli_versions"] == {"claude": "2.1.300", "codex": "0.155.1"}


class OneShotProcess:
    def __init__(self, result):
        self.result, self.argv = result, None

    def run(self, argv, cwd, **_):
        self.argv = list(argv)
        return self.result


@pytest.mark.parametrize(
    ("cli", "stdout", "expected"),
    [(Cli.CLAUDE, "2.1.292 (Claude Code)\n", "2.1.292"), (Cli.CODEX, "codex-cli 0.155.1\n", "0.155.1")],
)
def test_installed_version_is_parsed_from_the_cli_banner(cli, stdout, expected):
    process = OneShotProcess(ProcessResult(0, stdout, ""))
    assert InstalledCliVersions(process, Path(".")).installed_version(cli) == expected
    assert process.argv == [cli.value, "--version"]


def test_unreadable_version_banner_is_an_error():
    with pytest.raises(RuntimeError):
        InstalledCliVersions(OneShotProcess(ProcessResult(0, "no digits", "")), Path(".")).installed_version(Cli.CLAUDE)


def test_calls_in_flight_never_exceed_the_bound_across_both_clis():
    lock = threading.Lock()
    state = {"now": 0, "peak": 0}

    class SlowModel:
        def call(self, request, baseline):
            with lock:
                state["now"] += 1
                state["peak"] = max(state["peak"], state["now"])
            time.sleep(0.03)
            with lock:
                state["now"] -= 1
            return ok(1, 1)

    router = RoutingModelPort({Cli.CLAUDE: SlowModel(), Cli.CODEX: SlowModel()}, max_in_flight=4)
    requests = [ModelRequest(f"c{i}", pin, Effort.MEDIUM, "i", SCHEMA, "s") for i, pin in enumerate([SONNET, TERRA] * 6)]
    threads = [threading.Thread(target=router.call, args=(request, None)) for request in requests]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    assert state["peak"] == 4
