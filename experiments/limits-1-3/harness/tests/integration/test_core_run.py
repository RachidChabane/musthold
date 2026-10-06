from __future__ import annotations

import json
import re
import shutil
from pathlib import Path

import pytest

from pilot.adapters.arm_p_items import DirectoryArmPItems
from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.differential import HypothesisDifferential
from pilot.adapters.fixture_sources import DirectoryFixtureSources
from pilot.adapters.import_smoke import ProjectImportSmoke
from pilot.adapters.models_lock import JsonSchemaValidator, load_models_lock
from pilot.adapters.prereg_files import load_edit_sets, write_arm_p
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.adapters.prompt_files import DirectoryPromptLibrary
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.adapters.run_directory import RunDirectory
from pilot.adapters.shallnot import ShallnotTraceability
from pilot.adapters.witness_runner import SubprocessWitnessExecutor
from pilot.application.assemble_arm_p import assemble_items
from pilot.application.context import PipelineContext
from pilot.application.core_run import CoreDependencies, run_core
from pilot.application.model_caller import ModelCaller
from pilot.application.process_mutants import MutantProcessor
from pilot.application.provenance import ManifestInputs
from pilot.application.exploratory_calls import ExploratoryLedger
from pilot.application.secondary_run import SecondaryDependencies, SecondaryPreconditionFailed, run_secondary
from pilot.application.stage_timer import StageTimer
from pilot.domain.call_type import CallType
from pilot.domain.model_call import CanaryBaseline, ModelProvenance, ModelResult, TokenUsage
from pilot.domain.quota import QuotaExhausted
from pilot.domain.variants import VariantKind

pytestmark = pytest.mark.integration
REAL_PREREG = Path(__file__).resolve().parents[3] / "prereg"
STATEMENT_WORDS = "started hour"
WITNESS = {
    "function": "quote", "entry": "2026-03-02T09:00", "exit": "2026-03-02T09:16", "lost_ticket": None,
    "validated": None, "amount": None, "expected": {"kind": "RETURNS", "value": "2.50"},
}
TESTS_PER_JUDGE = 8


def edits(qualname, source):
    return [{"file": "src/parkfee/tariff.py", "qualname": qualname, "new_source": source}]


class FakeModel:
    """A scripted model: every payload is validated against the request's schema and semantic check."""

    def __init__(self, quota_after=None):
        self.requests, self.quota_after = [], quota_after
        self.validator = JsonSchemaValidator()

    def call(self, request, baseline):
        if self.quota_after is not None and len(self.requests) >= self.quota_after:
            raise QuotaExhausted("fake limit")
        self.requests.append(request)
        payload = self._payload(request)
        assert self.validator.violations(request.schema_text, payload) == [], request.call_id
        if request.validate is not None:
            assert request.validate(payload) == [], request.call_id
        return ModelResult(payload, TokenUsage(10, 5), None, 1, 1, ModelProvenance.REPORTED)

    def _payload(self, request):
        kind = request.call_id.split("__")[0]
        return getattr(self, f"_{kind}")(request)

    def _generate(self, request):
        n = len(self.requests)
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

    def _validity(self, request):
        refs = [f"PARK-{n}~1" for n in range(1, 11)]
        return {
            "equivalent": {"value": False, "confidence": 0.9},
            "requirements": {
                ref: ({"verdict": "VIOLATED", "confidence": 0.9, "witness": WITNESS} if ref == "PARK-3~1"
                      else {"verdict": "NOT_VIOLATED", "confidence": 0.9, "witness": None})
                for ref in refs
            },
        }

    _audit = _validity

    def _align(self, request):
        lines = request.item.split("=== Requirements ===\n", 1)[1].split("\n=== Test ===")[0].splitlines()
        token = next(line.split(":")[0] for line in lines if STATEMENT_WORDS in line)
        return {"verifies": token, "coverage": "FULL", "confidence": 80, "rationale": "checks the charge"}

    def _probe(self, request):
        return {"guess": "TARGETED", "confidence": 60}

    def _relabel(self, request):
        links = re.findall(r"# nodeid: (\S+)  declares: (\S+)", request.item)
        return {"tests": [{"nodeid": nodeid, "label": "GOOD", "checks": [ref]} for nodeid, ref in links]}

    def _leak_audit(self, request):
        return {"hints": [{"file": "tests/test_mini.py", "location": "line 3", "hint": "name", "why": "reason"}]}


def baselines():
    pins = [("claude-opus-5-5__claude"), ("claude-sonnet-5-5__claude"), ("gpt-5.6-terra__codex")]
    return {(pin, call_type): CanaryBaseline(100, None) for pin in pins for call_type in CallType}


class Provenance:
    def prereg_tag(self):
        return "tag-a1"

    def prereg_commit(self):
        return "c0ffee"

    def superseded_prereg_tag(self):
        return "tag-1"

    def superseded_prereg_commit(self):
        return "c0ffe0"

    def imports_digest(self):
        return None

    def harness_digest(self):
        return "h"

    def fixture_digest(self):
        return "f"


class Versions:
    def installed_version(self, cli):
        return {"claude": "2.1.292", "codex": "0.155.1"}[cli.value]


def make_world(mini_root, base):
    root = base / "mini"
    shutil.copytree(mini_root, root)
    for name in ("prompts", "schemas"):
        shutil.copytree(REAL_PREREG / name, root / "prereg" / name)
    for name in ("models.lock.json", "codebook.md"):
        shutil.copy(REAL_PREREG / name, root / "prereg" / name)
    items = assemble_items(
        load_edit_sets(root / "prereg" / "violations", VariantKind.VIOLATION),
        load_edit_sets(root / "prereg" / "planted", VariantKind.PLANTED),
    )
    write_arm_p(root / "prereg" / "arm_p" / "items", root / "prereg" / "arm_p" / "labels.yaml", items)
    return root


@pytest.fixture
def world(mini_root, tmp_path):
    return make_world(mini_root, tmp_path)


def build(world, model, tmp_path):
    project = world / "fixture" / "parkfee"
    process = SubprocessRunner()
    applier = AstEditApplier()
    workspace = ProjectWorkspace(project, tmp_path / "work", applier)
    pytest_runner = PytestJunitRunner(process)
    runner = WorkspaceVariantRunner(workspace, pytest_runner.run)
    prereg = world / "prereg"
    prompts = DirectoryPromptLibrary(prereg / "prompts", prereg / "schemas", prereg / "prompts" / "system.txt", prereg / "codebook.md")
    store = RunDirectory(world / "runs" / "r1")
    logs = []
    caller = ModelCaller(model, baselines(), "system", logs.append, 4)
    ctx = PipelineContext("r1", store, DirectoryFixtureSources(project, project / "spec"), prompts, caller, logs.append)
    lock = load_models_lock(prereg / "models.lock.json")
    deps = CoreDependencies(
        ctx=ctx, timer=StageTimer(store), provenance=Provenance(), versions=Versions(),
        manifest_inputs=ManifestInputs("r1", sorted(lock.pins, key=lambda p: p.key), lock.cli_versions),
        traceability=ShallnotTraceability(workspace, pytest_runner, process), runner=runner,
        processor=MutantProcessor(runner, applier, project, ProjectImportSmoke(workspace, process), HypothesisDifferential(workspace, process, project)),
        witnesses=SubprocessWitnessExecutor(workspace, process),
        arm_p=DirectoryArmPItems(prereg / "arm_p" / "items"),
        pins={pin.model: pin for pin in lock.pins}, expected_tests=8, expected_criteria=3,
    )
    return deps, logs


def rows(store, name):
    return store.read_jsonl(name)


@pytest.fixture(scope="module")
def completed(mini_root_module, tmp_path_factory):
    base = tmp_path_factory.mktemp("core")
    world = make_world(mini_root_module, base)
    model = FakeModel()
    deps, logs = build(world, model, base)
    run_core(deps)
    return deps, model, logs, world


@pytest.fixture(scope="module")
def mini_root_module():
    return Path(__file__).resolve().parents[1] / "fixtures" / "mini"


def test_core_run_writes_every_record_of_the_contract(completed):
    deps, model, _, world = completed
    run = world / "runs" / "r1"
    for name in (
        "manifest.json", "DEVIATIONS.md", "traceability.json", "bindings.json", "reference.json", "plan.json",
        "processing.jsonl", "kills.jsonl", "differential.jsonl", "validity.jsonl",
        "witness.jsonl", "mutant_classes.json", "alignment.jsonl", "alignment_aggregate.json", "probe.jsonl", "timing.json",
    ):
        assert (run / name).is_file(), name
    manifest = json.loads((run / "manifest.json").read_text())
    assert set(manifest) == {
        "run_id", "prereg_tag", "prereg_commit", "superseded_prereg_tag", "superseded_prereg_commit", "imports_digest",
        "harness_digest", "fixture_digest", "cli_versions", "models",
    }
    assert manifest["prereg_commit"] == "c0ffee" and manifest["cli_versions"] == {"claude": "2.1.292", "codex": "0.155.1"}
    bindings = json.loads((run / "bindings.json").read_text())
    assert len(bindings) == 8 and bindings["tests/test_mini.py::test_free_at_fifteen_minutes"] == "PARK-2~1"
    reference = json.loads((run / "reference.json").read_text())
    assert reference["passed_all"] and len(reference["runs"]) == 3


def test_arm_t_plan_has_five_calls_per_criterion_with_unique_nonces_and_the_model_split(completed):
    _, model, _, world = completed
    plan = json.loads((world / "runs" / "r1" / "plan.json").read_text())["arm_t"]
    assert len(plan) == 15 and len({c["nonce"] for c in plan}) == 15
    per_criterion = [c["model"] for c in plan if c["criterion"] == "PARK-1~1"]
    assert per_criterion == ["claude-sonnet-5-5"] * 5
    generation = [r for r in model.requests if r.call_id.startswith("generate")]
    assert len(generation) == 15 and all(r.effort.value == "high" for r in generation)
    assert all(c["nonce"] in r.item for c, r in zip(plan, sorted(generation, key=lambda r: r.call_id), strict=False) if c["call_id"] == r.call_id)


def test_generation_input_has_the_spec_target_and_source_but_never_tests_or_conftest(completed):
    _, model, _, _ = completed
    item = next(r.item for r in model.requests if r.call_id == "generate__PARK-3~1__6")
    assert "PARK-3~1: WHEN a stay lasts more than 15 minutes" in item and "=== src/parkfee/tariff.py ===" in item
    assert "test_mini" not in item and "verifies" not in item and "conftest" not in item


def test_mutants_carry_provenance_and_arm_p_enters_without_target(completed):
    deps, _, _, world = completed
    run = world / "runs" / "r1"
    documents = [json.loads(p.read_text()) for p in sorted((run / "mutants").glob("*.json"))]
    arm_t = [d for d in documents if d["arm"] == "T"]
    arm_p = [d for d in documents if d["arm"] == "P"]
    assert len(arm_p) == 8 and len(arm_t) == 45
    sample = arm_t[0]
    assert set(sample) == {"mutant_id", "arm", "target", "slot", "generator", "edits", "normalized_edits", "mechanism_summary", "merged_from"}
    assert sample["slot"] in {"S1", "S2", "S3"} and set(sample["generator"]) == {"model", "cli", "call_id", "nonce"}
    assert all(d["target"] is None and d["generator"] is None for d in arm_p)
    assert len(documents) == len(list((run / "mutants").glob("*.json")))


def test_processing_kills_and_differential_records_follow_the_contract(completed):
    deps, _, _, _ = completed
    processing = rows(deps.ctx.store, "processing.jsonl")
    assert {"mutant_id", "stillborn", "stillborn_reason", "status", "duplicate_of", "changed_lines"} == set(processing[0])
    evaluated = {r["mutant_id"] for r in processing if not r["stillborn"] and r["status"] == "OK"}
    kills = rows(deps.ctx.store, "kills.jsonl")
    assert {k["mutant_id"] for k in kills} == evaluated
    assert set(kills[0]["results"].values()) <= {"killed", "survived", "error", "flaky"}
    differential = rows(deps.ctx.store, "differential.jsonl")
    assert {d["mutant_id"] for d in differential} == evaluated
    assert [r["mutant_id"] for r in processing] == sorted(r["mutant_id"] for r in processing)


def test_both_validity_judges_judge_every_evaluated_mutant_and_witnesses_are_executed(completed):
    deps, model, _, _ = completed
    validity = rows(deps.ctx.store, "validity.jsonl")
    kills = rows(deps.ctx.store, "kills.jsonl")
    assert len(validity) == 2 * len(kills) and {r["judge"] for r in validity} == {"V-H1", "V-H2"}
    witnesses = rows(deps.ctx.store, "witness.jsonl")
    assert witnesses and {w["judge"] for w in witnesses} == {"V-H1", "V-H2"}
    assert any(w["confirmed"] for w in witnesses) and any(not w["confirmed"] for w in witnesses)
    confirmed = next(w for w in witnesses if w["confirmed"])
    assert confirmed["reference_out"]["kind"] == "RETURNS" and confirmed["error"] is None
    judge_requests = [r for r in model.requests if r.call_id.startswith("validity")]
    assert all(r.effort.value == "medium" for r in judge_requests)
    for request in judge_requests:
        for hidden in ("test_mini", "mechanism", "arm", "slot"):
            assert hidden not in request.item.split("=== Specification ===", 1)[1].split("=== Original functions ===")[1]


def test_mutant_classes_mark_confirmed_valid_mutants_primary(completed):
    deps, _, _, _ = completed
    classes = deps.ctx.store.read_json("mutant_classes.json")
    assert [c["mutant_id"] for c in classes] == sorted(c["mutant_id"] for c in classes)
    valid = [c for c in classes if c["per_criterion"]["PARK-3~1"] == "VALID_W"]
    assert valid and all(c["valid_confidence"]["PARK-3~1"] == 0.9 for c in valid)
    assert all(set(c["valid_x"]) <= {"PARK-3~1"} for c in classes)
    primary = [c for c in classes if c["primary"]["included"]]
    assert primary and all(c["arm"] == "T" and c["target"] == "PARK-3~1" for c in primary)
    assert all(c["arm"] == "P" and not c["primary"]["included"] for c in classes if c["arm"] == "P")


def test_alignment_runs_five_masked_repetitions_per_test_with_token_maps_and_proofs(completed):
    deps, model, _, world = completed
    run = world / "runs" / "r1"
    alignment = rows(deps.ctx.store, "alignment.jsonl")
    assert len(alignment) == TESTS_PER_JUDGE * 5 and {(r["judge"], r["arm"]) for r in alignment} == {("J-H", "NAME-MASKED")}
    assert set(alignment[0]) == {"judge", "arm", "nodeid", "rep", "call_id", "status", "verifies_ref", "coverage", "confidence", "rationale"}
    assert all(r["verifies_ref"] == "PARK-3~1" for r in alignment)
    maps = sorted((run / "token_maps" / "J-H" / "NAME-MASKED").glob("*/rep-*.json"))
    assert len(maps) == 40 and all(len(json.loads(m.read_text())) == 3 for m in maps)
    aggregate = deps.ctx.store.read_json("alignment_aggregate.json")["J-H"]["NAME-MASKED"]
    verdicts = {nodeid: entry["verdict"] for nodeid, entry in aggregate.items()}
    assert verdicts["tests/test_mini.py::test_charges_two_hours_for_sixty_one_minutes"] == "ALIGNED"
    assert verdicts["tests/test_mini.py::test_free_at_fifteen_minutes"] == "MISALIGNED"
    proofs = sorted((run / "proofs" / "alignment" / "J-H" / "NAME-MASKED").glob("*.intoto.json"))
    assert len(proofs) == 8
    first = json.loads(proofs[0].read_text())
    assert first["predicateType"].endswith("/alignment/v1") and first["predicate"]["requirement"]["statementCanonicalization"] == "c1"
    for request in (r for r in model.requests if r.call_id.startswith("align")):
        assert "verifies" not in request.item and "PARK-" not in request.item and "test_case" in request.item


def test_probe_asks_three_masked_repetitions_per_test_without_criteria(completed):
    deps, model, _, _ = completed
    probe = rows(deps.ctx.store, "probe.jsonl")
    assert len(probe) == TESTS_PER_JUDGE * 3 and {r["judge"] for r in probe} == {"J-H"}
    assert {"judge", "nodeid", "rep", "call_id", "status", "guess", "confidence"} == set(probe[0])
    request = next(r for r in model.requests if r.call_id.startswith("probe"))
    assert "SHALL" not in request.item and "test_case" in request.item


def test_discriminating_power_proofs_are_written_per_criterion(completed):
    _, _, _, world = completed
    directory = world / "runs" / "r1" / "proofs" / "discriminating-power"
    assert sorted(p.name for p in directory.glob("*.json")) == ["PARK-1~1.intoto.json", "PARK-2~1.intoto.json", "PARK-3~1.intoto.json"]
    statement = json.loads((directory / "PARK-3~1.intoto.json").read_text())
    predicate = statement["predicate"]
    assert statement["predicateType"].endswith("/discriminating-power/v1") and predicate["correctCodePasses"]["value"] is True
    assert len(predicate["tests"]) == 4 and any(m["validity"]["value"] in {"VALID_W", "VALID_X"} for m in predicate["mutants"])
    assert "new_source" not in json.dumps(statement)


def test_timing_records_every_stage(completed):
    deps, _, logs, _ = completed
    timing = deps.ctx.store.read_json("timing.json")
    assert set(timing) == {"traceability", "reference", "generate", "process", "validity", "alignment", "probe", "proofs"}
    assert all(set(entry) == {"started", "finished", "seconds"} for entry in timing.values())
    assert any(line.startswith("[generate:T] 15/15 calls done") for line in logs)


def test_a_second_run_reuses_the_plan_and_the_processing_without_new_nonces_or_pytest(completed, tmp_path):
    _, _, _, world = completed
    plan_before = (world / "runs" / "r1" / "plan.json").read_text()
    processing_before = (world / "runs" / "r1" / "processing.jsonl").read_text()
    again, logs = build(world, FakeModel(), tmp_path / "again")
    run_core(again)
    assert (world / "runs" / "r1" / "plan.json").read_text() == plan_before
    assert (world / "runs" / "r1" / "processing.jsonl").read_text() == processing_before
    assert "[process] already complete" in logs and "[validity] already complete" in logs


def test_a_quota_stop_is_resumable_and_keeps_the_plan(world, tmp_path):
    deps, _ = build(world, FakeModel(quota_after=7), tmp_path)
    with pytest.raises(QuotaExhausted):
        run_core(deps)
    run = world / "runs" / "r1"
    assert (run / "plan.json").is_file() and not (run / "processing.jsonl").exists()
    plan = (run / "plan.json").read_text()
    resumed, _ = build(world, FakeModel(), tmp_path / "resumed")
    run_core(resumed)
    assert (run / "plan.json").read_text() == plan and (run / "mutant_classes.json").is_file()


def test_run_secondary_requires_the_audit_sample(completed, tmp_path):
    _, _, _, world = completed
    copied = tmp_path / "copy"
    shutil.copytree(world, copied)
    deps, _ = build(copied, FakeModel(), tmp_path / "w")
    with pytest.raises(SecondaryPreconditionFailed, match="audit_sample.json"):
        run_secondary(SecondaryDependencies(deps, ExploratoryLedger(deps.ctx.store)))


def test_run_secondary_adds_j_h_name_shown_j_x_masked_and_the_audit_and_records_per_call_status(completed, tmp_path):
    _, _, _, shared = completed
    world = tmp_path / "copy"
    shutil.copytree(shared, world)
    again, _ = build(world, FakeModel(), tmp_path / "w")
    store = again.ctx.store
    kills = rows(store, "kills.jsonl")
    sample = [kills[0]["mutant_id"], kills[1]["mutant_id"]]
    store.write_json("audit_sample.json", {"seed": 1, "mutant_ids": sample, "strata": {m: "s" for m in sample}})
    model = FakeModel()
    again, _ = build(world, model, tmp_path / "secondary")
    run_secondary(SecondaryDependencies(again, ExploratoryLedger(store)))
    alignment = rows(store, "alignment.jsonl")
    assert {(r["judge"], r["arm"]) for r in alignment} == {("J-H", "NAME-MASKED"), ("J-H", "NAME-SHOWN"), ("J-X", "NAME-MASKED")}
    assert len(alignment) == 3 * TESTS_PER_JUDGE * 5
    assert {r["judge"] for r in rows(store, "probe.jsonl")} == {"J-H"}
    aggregate = store.read_json("alignment_aggregate.json")
    assert set(aggregate) == {"J-X", "J-H"} and set(aggregate["J-X"]) == {"NAME-MASKED"}
    audit = [r for r in rows(store, "validity.jsonl") if r["judge"] == "A-OPUS"]
    assert sorted(r["mutant_id"] for r in audit) == sorted(sample)
    assert all(r["call_id"].startswith("audit__") for r in audit)
    audit_requests = [r for r in model.requests if r.call_id.startswith("audit")]
    assert len(audit_requests) == 2 and all(r.effort.value == "high" for r in audit_requests)
    assert not any(json.loads(p.read_text())["arm"] == "O" for p in (world / "runs" / "r1" / "mutants").glob("*.json"))
    status = store.read_json("exploratory_status.json")["secondary"]
    assert set(status.values()) == {"complete"} and len(status) == 2 * TESTS_PER_JUDGE * 5 + 2
