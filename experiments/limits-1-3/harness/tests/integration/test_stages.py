from __future__ import annotations

import hashlib
import json
import shutil
import subprocess
import sys
from pathlib import Path

import pytest
from test_core_run import FakeModel, Provenance, Versions, build, make_world

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.adapters.git_drift import PreregistrationTagMissing
from pilot.adapters.provenance import GitProvenance, tree_digest
from pilot.adapters.witness_runner import SubprocessWitnessExecutor
from pilot.application.leak_audit_stage import LeakAuditMissing, run_leak_audit
from pilot.application.relabel_stage import run_relabel
from pilot.application.traceability_stage import TraceabilityFailed, ensure_traceability
from pilot.domain.analysis_constants import PREREG_TAG, SUPERSEDED_PREREG_TAG
from pilot.domain.call_type import CallType
from pilot.domain.edits import Edit, EditSet
from pilot.domain.model_call import CanaryBaseline, MissingResult
from pilot.domain.witness import Output, OutputKind, Witness

pytestmark = pytest.mark.integration
TARIFF = "src/parkfee/tariff.py"


@pytest.fixture
def deps(mini_root, tmp_path):
    world = make_world(mini_root, tmp_path)
    model = FakeModel()
    built, _ = build(world, model, tmp_path)
    return built, model, world


def test_relabel_makes_one_call_per_rater_over_all_tests_with_links_from_the_bindings(deps):
    built, model, _ = deps
    bindings = ensure_traceability(built.ctx.store, built.traceability, 8, 3)
    assert run_relabel(built.ctx, bindings) == []
    requests = [r for r in model.requests if r.call_id.startswith("relabel")]
    assert sorted(r.call_id for r in requests) == ["relabel__R-AUTHOR", "relabel__R-OAI"]
    assert {r.pin.model for r in requests} == {"claude-opus-5-5", "gpt-5.6-terra"} and all(r.effort.value == "high" for r in requests)
    item = requests[0].item
    assert item.count("# nodeid:") == 8 and "pytest.mark.verifies" not in item
    rows = built.ctx.store.read_jsonl("relabel.jsonl")
    assert len(rows) == 16 and {r["rater"] for r in rows} == {"R-OAI", "R-AUTHOR"}
    assert set(rows[0]) == {"rater", "nodeid", "label", "checks"}
    for rater in ("R-OAI", "R-AUTHOR"):
        assert {r["nodeid"] for r in rows if r["rater"] == rater} == set(bindings)


def test_relabel_reports_a_rater_without_an_answer(deps):
    built, model, _ = deps
    bindings = ensure_traceability(built.ctx.store, built.traceability, 8, 3)

    class Refusing(FakeModel):
        def call(self, request, baseline):
            if request.call_id == "relabel__R-OAI":
                return MissingResult(("x",), ())
            return super().call(request, baseline)

    refusing, _ = build(deps[2], Refusing(), deps[2].parent / "w2")
    unanswered = run_relabel(refusing.ctx, bindings)
    assert [r.value for r in unanswered] == ["R-OAI"]
    assert {r["rater"] for r in refusing.ctx.store.read_jsonl("relabel.jsonl")} == {"R-AUTHOR"}


def test_traceability_stage_writes_the_report_and_node_id_bindings_once(deps):
    built, _, world = deps
    bindings = ensure_traceability(built.ctx.store, built.traceability, 8, 3)
    run = world / "runs" / "r1"
    assert json.loads((run / "bindings.json").read_text()) == bindings
    assert set(json.loads((run / "traceability.json").read_text())) >= {"requirements", "summary", "verdict"}
    assert bindings["tests/test_mini.py::test_charges_two_hours_for_sixty_one_minutes"] == "PARK-3~1"

    class Unused:
        def raw_report(self):
            raise AssertionError("must not rerun")

    assert ensure_traceability(built.ctx.store, Unused(), 8, 3) == bindings


def test_traceability_stage_aborts_when_the_null_gate_precondition_fails(deps):
    built, _, _ = deps
    with pytest.raises(TraceabilityFailed, match="expected 38/38"):
        ensure_traceability(built.ctx.store, built.traceability, 38, 3)


def test_leak_audit_calls_the_cross_family_model_once_without_a_baseline(deps):
    built, model, _ = deps
    written = []
    hints = run_leak_audit(
        built.ctx.caller.__class__(model, {}, "sys", lambda m: None, 4, require_baseline=False),
        built.ctx.prompts.template(CallType.LEAK_AUDIT), built.ctx.prompts.schema(CallType.LEAK_AUDIT),
        {"spec/p.md": "S", "tests/t.py": "T"}, written.append,
    )
    assert hints == 1 and written[0]["hints"][0]["file"] == "tests/test_mini.py"
    request = [r for r in model.requests if r.call_id == "leak_audit"][0]
    assert request.pin.model == "gpt-5.6-terra" and request.effort.value == "high"
    assert "=== spec/p.md ===\nS" in request.item and "=== tests/t.py ===\nT" in request.item


def test_leak_audit_without_an_answer_raises(deps):
    built, _, _ = deps

    class Mute:
        def call(self, request, baseline):
            return MissingResult(("x",), ())

    with pytest.raises(LeakAuditMissing):
        run_leak_audit(built.ctx.caller.__class__(Mute(), {}, "s", lambda m: None, 4, require_baseline=False), "{files}", "{}", {}, lambda p: None)


@pytest.fixture
def witnesses(mini_root, tmp_path):
    project = mini_root / "fixture" / "parkfee"
    workspace = ProjectWorkspace(project, tmp_path / "work", AstEditApplier())
    return SubprocessWitnessExecutor(workspace, SubprocessRunner())


def quote_witness(**changes):
    values = {"function": "quote", "entry": "2026-03-02T09:00", "exit": "2026-03-02T09:16", "lost_ticket": None,
              "validated": None, "amount": None, "expected": Output(OutputKind.RETURNS, "2.50")}
    return Witness(**{**values, **changes})


RATE_MUTANT = EditSet((Edit(TARIFF, "_hourly_charge", 'def _hourly_charge(minutes):\n    return Decimal("2.00") * -(-minutes // 60)\n'),))


def test_witness_is_executed_on_reference_and_mutant(witnesses):
    (execution,) = witnesses.execute(RATE_MUTANT, [quote_witness()])
    assert execution.error is None
    assert execution.reference == Output(OutputKind.RETURNS, "2.5") and execution.mutant == Output(OutputKind.RETURNS, "2")


def test_a_witness_raising_in_the_reference_is_recorded_as_raises(witnesses):
    (execution,) = witnesses.execute(RATE_MUTANT, [quote_witness(entry="2026-03-02T09:16", exit="2026-03-02T09:00")])
    assert execution.reference == Output(OutputKind.RAISES, "ValueError")


def test_a_receipt_witness_runs_the_receipt_function(witnesses):
    receipt = Witness("format_receipt", None, None, None, None, "0", Output(OutputKind.RETURNS, "FREE"))
    (execution,) = witnesses.execute(RATE_MUTANT, [receipt])
    assert execution.reference == Output(OutputKind.RETURNS, "FREE")


def test_unparseable_arguments_are_an_error_not_an_evaluation(witnesses):
    (execution,) = witnesses.execute(RATE_MUTANT, [quote_witness(entry="__import__('os').system('true')")])
    assert execution.error and execution.reference is None


def test_an_invalid_witness_is_reported(witnesses):
    (execution,) = witnesses.execute(RATE_MUTANT, [quote_witness(entry=None)])
    assert execution.error == "invalid witness: quote needs entry and exit"


def test_a_mutant_that_never_returns_hits_the_five_second_timeout(witnesses, monkeypatch):
    from pilot.domain import pipeline_constants

    monkeypatch.setattr("pilot.adapters.witness_runner.WITNESS_TIMEOUT_SECONDS", 1)
    hang = EditSet((Edit(TARIFF, "_hourly_charge", "def _hourly_charge(minutes):\n    while True:\n        pass\n"),))
    (execution,) = witnesses.execute(hang, [quote_witness()])
    assert execution.error == "timeout after 1s" and pipeline_constants.WITNESS_TIMEOUT_SECONDS == 5


def test_tree_digest_follows_content_and_ignores_caches(tmp_path):
    (tmp_path / "a.py").write_text("x")
    first = tree_digest(tmp_path)
    (tmp_path / "__pycache__").mkdir()
    (tmp_path / "__pycache__" / "a.pyc").write_text("noise")
    assert tree_digest(tmp_path) == first
    (tmp_path / "a.py").write_text("y")
    assert tree_digest(tmp_path) != first


GIT_ENV = {"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "PATH": "/usr/bin:/bin:/opt/homebrew/bin:/usr/local/bin"}


def git(cwd, *args):
    subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, env=GIT_ENV)


def test_prereg_commit_is_the_commit_the_annotated_tag_peels_to(tmp_path):
    git(tmp_path, "init", "-q")
    (tmp_path / "prereg").mkdir()
    provenance = GitProvenance(SubprocessRunner(), tmp_path, tmp_path, tmp_path)
    (tmp_path / "prereg" / "PREREG.md").write_text("v1")
    with pytest.raises(PreregistrationTagMissing):
        provenance.prereg_commit()
    git(tmp_path, "add", "."), git(tmp_path, "commit", "-qm", "prereg")
    frozen = git_output(tmp_path, "rev-parse", "HEAD")
    git(tmp_path, "tag", "-a", "-m", "freeze", PREREG_TAG)
    git(tmp_path, "tag", "-a", "-m", "freeze", SUPERSEDED_PREREG_TAG)
    (tmp_path / "prereg" / "PREREG.md").write_text("v2")
    git(tmp_path, "add", "."), git(tmp_path, "commit", "-qm", "later edit of the document")
    assert provenance.prereg_commit() == frozen
    assert provenance.superseded_prereg_commit() == frozen
    assert provenance.prereg_tag() == PREREG_TAG and provenance.superseded_prereg_tag() == SUPERSEDED_PREREG_TAG


def test_imports_digest_is_the_hash_of_imports_json_when_present(tmp_path):
    run_dir = tmp_path / "run"
    run_dir.mkdir()
    provenance = GitProvenance(SubprocessRunner(), tmp_path, tmp_path, tmp_path, run_dir)
    assert provenance.imports_digest() is None
    (run_dir / "imports.json").write_text("{}")
    assert provenance.imports_digest() == hashlib.sha256(b"{}").hexdigest()


def git_output(cwd, *args):
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True, env=GIT_ENV).stdout.strip()
