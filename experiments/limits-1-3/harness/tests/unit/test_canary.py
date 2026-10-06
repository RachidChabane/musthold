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
    load_models_lock,
)
from pilot.adapters.process import ProcessResult
from pilot.application.canary import CanaryFailed, CanarySpec, run_canary
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


class FakeModel:
    def __init__(self, outcomes):
        self.outcomes = outcomes
        self.requests = []

    def call(self, request, baseline):
        self.requests.append((request, baseline))
        return self.outcomes[request.pin.key]


class FakeProbe:
    def __init__(self, versions):
        self.versions = versions

    def installed_version(self, cli):
        return self.versions[cli]


class MemoryStore:
    def __init__(self):
        self.saved = {}

    def save(self, pin, baseline, cli_version):
        self.saved[pin.key] = (baseline, cli_version)


def ok(tokens, turns, payload=None):
    return ModelResult(payload if payload is not None else {"ok": True}, TokenUsage(tokens, 5), None, turns, 1, ModelProvenance.REPORTED)


def spec(pins=(SONNET, TERRA), versions=None):
    return CanarySpec(
        pins=pins,
        locked_cli_versions=versions or {Cli.CLAUDE: "2.1.292", Cli.CODEX: "0.155.1"},
        effort=Effort.MEDIUM,
        system_prompt="system",
        item="item",
        schema_text=SCHEMA,
    )


VERSIONS = {Cli.CLAUDE: "2.1.292", Cli.CODEX: "0.155.1"}


def test_canary_records_a_baseline_per_pair_without_checking_the_bound():
    model = FakeModel({SONNET.key: ok(1174, 2), TERRA.key: ok(8627, None)})
    store = MemoryStore()
    baselines = run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), store, spec())
    assert baselines == {SONNET.key: CanaryBaseline(1174, 2), TERRA.key: CanaryBaseline(8627, None)}
    assert store.saved[SONNET.key] == (CanaryBaseline(1174, 2), "2.1.292")
    assert all(baseline is None for _, baseline in model.requests)
    assert {r.call_id for r, _ in model.requests} == {f"canary/{SONNET.key}", f"canary/{TERRA.key}"}


def test_canary_aborts_on_a_cli_version_mismatch():
    model = FakeModel({SONNET.key: ok(1, 1)})
    with pytest.raises(CanaryFailed, match="claude installed 2.1.293, locked 2.1.292"):
        run_canary(model, FakeProbe({Cli.CLAUDE: "2.1.293"}), JsonSchemaValidator(), MemoryStore(), spec((SONNET,)))
    assert model.requests == []


def test_canary_aborts_when_the_call_is_missing():
    missing = MissingResult(("claude exited 1",), ("modelUsage keys",))
    model = FakeModel({SONNET.key: missing})
    with pytest.raises(CanaryFailed, match="modelUsage keys"):
        run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(), spec((SONNET,)))


def test_canary_aborts_when_the_payload_violates_the_schema():
    model = FakeModel({SONNET.key: ok(1, 1, payload={"ok": "yes"})})
    with pytest.raises(CanaryFailed, match="violates schema"):
        run_canary(model, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(), spec((SONNET,)))


def test_router_dispatches_by_cli():
    claude, codex = FakeModel({SONNET.key: ok(1, 1)}), FakeModel({TERRA.key: ok(2, None)})
    router = RoutingModelPort({Cli.CLAUDE: claude, Cli.CODEX: codex})
    run_canary(router, FakeProbe(VERSIONS), JsonSchemaValidator(), MemoryStore(), spec())
    assert len(claude.requests) == 1 and len(codex.requests) == 1


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


def test_canary_store_writes_one_file_per_pin(tmp_path):
    DirectoryCanaryStore(tmp_path / "canary").save(TERRA, CanaryBaseline(8627, None), "0.155.1")
    saved = json.loads((tmp_path / "canary" / "gpt-5.6-terra__codex.json").read_text())
    assert saved == {
        "model": "gpt-5.6-terra", "cli": "codex", "cli_version": "0.155.1",
        "provenance": "requested_only", "input_tokens": 8627, "num_turns": None,
    }


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
