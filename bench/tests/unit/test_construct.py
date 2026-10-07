from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass, field
from datetime import date
from string import Template
from typing import Any

import pytest

from specbench.adapters.prompt_files import construction_schema, load_prompts
from specbench.adapters.schemas import JsonSchemaChecker, SchemaName
from specbench.application.construct import (
    Constructed,
    ConstructionInputs,
    Dropped,
    DropStage,
    HumanTests,
    TaskConstruction,
)
from specbench.domain.candidates import Candidate
from specbench.domain.canonical import JsonValue
from specbench.domain.construction import ModelRequest, ModelResponse, Role
from specbench.domain.languages import Language
from specbench.domain.prompts import Prompt, fill
from specbench.domain.selection import ProbedTask
from specbench.domain.task import Toolchain

Answers = dict[str, list[Any]]
SCHEMAS = {
    "spec": SchemaName.SPEC,
    "review": SchemaName.REVIEW,
    "h-tests": SchemaName.H_TESTS,
    "violations": SchemaName.VIOLATIONS,
    "refactors": SchemaName.REFACTORS,
    "assignment": SchemaName.ASSIGNMENT,
    "probe": SchemaName.PROBE,
}
U_SOURCE = "def test_free():\n    assert fee(5) == 0\n"
MERGE_FILES = {"pkg/fee.py": "def fee(m):\n    if m < 10:\n        return 0\n    return 5\n"}


def spec(first_example: str = "fee(minutes=7) => 0") -> dict[str, JsonValue]:
    return {
        "title": "Parking fee",
        "overview": "The fee of one stay.",
        "interface": [{"import_path": "pkg.fee", "signature": "fee(minutes: int) -> int", "note": ""}],
        "criteria": [
            {
                "number": 1,
                "statement": "WHEN a stay is short THE SYSTEM SHALL charge nothing",
                "examples": [first_example],
                "status": "new",
            },
            {
                "number": 2,
                "statement": "WHEN a stay is long THE SYSTEM SHALL charge 5",
                "examples": ["fee(minutes=60) => 5"],
                "status": "unchanged",
            },
        ],
    }


H_FILE = (
    "from pkg.fee import fee\n\n\ndef test_c1_short():\n    assert fee(1) == 0\n\n\n"
    "def test_c2_long():\n    assert fee(60) == 5\n"
)


def edit(search: str, replace: str) -> dict[str, str]:
    return {"path": "pkg/fee.py", "search": search, "replace": replace}


VIOLATIONS = {
    "violations": [
        {"criterion": 1, "title": "a", "edits": [edit("return 0", "return 1")]},
        {"criterion": 1, "title": "b", "edits": [edit("m < 10", "m < 6")]},
        {"criterion": 2, "title": "c", "edits": [edit("return 5", "return 6")]},
        {"criterion": 2, "title": "d", "edits": [edit("m < 10", "m < 11")]},
    ]
}
REFACTORS = {
    "refactors": [
        {"title": "p", "edits": [edit("m < 10", "m < 10 + 0")]},
        {"title": "q", "edits": [edit("return 5", "return 2 + 3")]},
    ]
}


def classification(implied: bool = True, criterion: int | None = 1) -> dict[str, JsonValue]:
    return {
        "classifications": [
            {"test_id": "tests/test_fee.py::test_free", "implied": implied, "criterion": criterion, "reason": "r"}
        ]
    }


@dataclass
class ScriptedModel:
    """Answers by call key, in order of appearance when a key has several answers, and records every request."""

    answers: Answers
    requests: list[ModelRequest] = field(default_factory=list[ModelRequest])

    def call(self, request: ModelRequest) -> ModelResponse:
        self.requests.append(request)
        queue = self.answers[request.key.split(":", 1)[1]]
        payload = queue.pop(0) if len(queue) > 1 else queue[0]
        return ModelResponse(payload, (request.model,), json.dumps(payload), 1, 1)

    @property
    def keys(self) -> list[str]:
        return [r.key.split(":", 1)[1] for r in self.requests]


class FakePatches:
    def build(self, before: Mapping[str, str], after: Mapping[str, str]) -> bytes:
        return ("".join(f"{path}:{after[path]}\n" for path in sorted(after))).encode()


def inputs(u_sources: Mapping[str, str] | None = None) -> ConstructionInputs:
    candidate = Candidate(
        "acme/parkfee",
        Language.PYTHON,
        500,
        "MIT",
        12,
        "feat: free",
        date(2026, 8, 3),
        "a" * 40,
        "b" * 40,
        ("pkg/fee.py",),
        40,
        ("tests/test_fee.py",),
        "LICENSE",
        None,
    )
    task = ProbedTask(candidate, "c" * 40, Toolchain("python", "3.13.14"), {"uv.lock": "d" * 64}, 1, 1, 2, "e" * 64)
    human = HumanTests(
        ("tests/test_fee.py::test_free",),
        ("tests/test_base.py::test_base",),
        u_sources if u_sources is not None else {"tests/test_fee.py::test_free": U_SOURCE},
        {"tests/test_fee.py": U_SOURCE.encode()},
    )
    return ConstructionInputs(
        task,
        "",
        "Short stays should be free.",
        "feat: free",
        "Closes #3",
        "diff --git a/pkg/fee.py b/pkg/fee.py\n+    if m < 10:",
        "pkg.fee: def fee(m)",
        ("Short stays should be free.",),
        MERGE_FILES,
        human,
    )


def happy() -> Answers:
    return {
        "spec-draft": [spec()],
        "review": [{"findings": []}],
        "probe-issue": [{"predictions": [{"operation": "fee", "arguments": ["3"]}]}],
        "probe-spec-0": [{"predictions": [{"operation": "fee", "arguments": ["3"]}]}],
        "h-0": [{"files": [{"path": "tests/spec_h/test_spec_h_fee.py", "content": H_FILE}]}],
        "assign-0": [classification()],
        "violations-0": [VIOLATIONS],
        "planted-0": [REFACTORS],
    }


def construction(answers: Answers) -> tuple[TaskConstruction, ScriptedModel]:
    model = ScriptedModel(answers)
    return (
        TaskConstruction(
            model,
            load_prompts(),
            {key: construction_schema(name) for key, name in SCHEMAS.items()},
            JsonSchemaChecker(),
            FakePatches(),
        ),
        model,
    )


def build(answers: Answers, given: ConstructionInputs | None = None) -> tuple[Constructed | Dropped, ScriptedModel]:
    pipeline, model = construction(answers)
    return pipeline.build(given or inputs()), model


def test_a_task_is_built_from_the_nine_calls_of_the_happy_path() -> None:
    built, model = build(happy())
    assert isinstance(built, Constructed)
    assert model.keys == [
        "spec-draft",
        "review",
        "probe-issue",
        "probe-spec-0",
        "h-0",
        "assign-0",
        "assign-0",
        "violations-0",
        "planted-0",
    ]
    assert [r.role for r in model.requests][5:7] == [Role.ASSIGNER_OPUS, Role.ASSIGNER_HAIKU]
    assert built.task.is_constructed and len(built.task.criteria) == 2
    files = built.bundle.files
    manifest = json.loads(files["oracle/manifest.json"])
    assert {(m["part"], m["criterion"]) for m in manifest if m["part"] in ("U", "H")} == {
        ("U", "ACME-PARKFEE-12-1"),
        ("H", "ACME-PARKFEE-12-1"),
        ("H", "ACME-PARKFEE-12-2"),
    }
    assert [p for p in files if p.startswith("violations/V")] == [
        f"violations/V{n}-{k}.patch" for n, k in ((1, 1), (1, 2), (2, 1), (2, 2))
    ]
    assert "oracle/O/tests/test_fee.py" not in files


def test_the_blind_inputs_hold_for_every_role() -> None:
    _, model = build(happy())
    prompts = {request.key.split(":", 1)[1]: request.system_prompt + request.prompt for request in model.requests}
    for key in ("spec-draft", "review", "h-0", "probe-issue", "probe-spec-0", "violations-0", "planted-0"):
        assert "assert fee(5)" not in prompts[key] and "test_free" not in prompts[key], key
    assert "assert fee(5) == 0" in prompts["assign-0"]
    assert "if m < 10" in prompts["spec-draft"] and "if m < 10" in prompts["violations-0"]
    assert "if m < 10" not in prompts["h-0"] and "if m < 10" not in prompts["review"]
    assert "Short stays should be free." not in prompts["h-0"] and "Closes #3" not in prompts["h-0"]
    assert "ACME-PARKFEE-12-1~1" in prompts["h-0"] and "## Interface" in prompts["h-0"]


def test_a_draft_that_breaks_the_writing_rules_is_repaired_once_then_the_task_is_dropped() -> None:
    broken = spec("fee 7 gives 0")
    answers = happy() | {"spec-draft": [broken], "spec-repair": [spec()]}
    built, model = build(answers)
    assert isinstance(built, Constructed) and model.keys[:3] == ["spec-draft", "spec-repair", "review"]
    assert "outside the grammar" in model.requests[1].prompt
    dropped, _ = build(happy() | {"spec-draft": [broken], "spec-repair": [broken]})
    assert isinstance(dropped, Dropped) and dropped.stage is DropStage.SPEC


def test_reviewer_findings_trigger_exactly_one_revision() -> None:
    flagged = {"findings": [{"criterion": 2, "kind": "ambiguous", "comment": "Say what long means."}]}
    revised = spec()
    revised["overview"] = "The fee of one stay; long means ten minutes or more."
    built, model = build(happy() | {"review": [flagged], "spec-revise": [revised]})
    assert isinstance(built, Constructed)
    assert model.keys.count("spec-revise") == 1
    revise = next(r for r in model.requests if r.key.endswith("spec-revise"))
    assert "criterion 2 (ambiguous): Say what long means." in revise.prompt
    assert b"long means ten minutes" in built.bundle.files["SPEC.md"]


def test_a_spec_example_with_a_human_test_input_is_fixed_once_then_the_task_is_dropped() -> None:
    leaking = spec("fee(minutes=5) => 0")
    fixed, model = build(
        happy()
        | {
            "spec-draft": [leaking],
            "spec-fix": [spec()],
            "probe-spec-1": [{"predictions": [{"operation": "fee", "arguments": ["3"]}]}],
        }
    )
    assert isinstance(fixed, Constructed) and "spec-fix" in model.keys
    fix = next(r for r in model.requests if r.key.endswith("spec-fix"))
    assert "no public source states" in fix.prompt and "n:5" in fix.prompt
    dropped, _ = build(happy() | {"spec-draft": [leaking], "spec-fix": [leaking]})
    assert isinstance(dropped, Dropped) and dropped.stage is DropStage.LEAK


def test_a_probe_that_predicts_more_oracle_inputs_with_the_spec_than_without_it_is_a_leak() -> None:
    oracle_sources = {
        "tests/test_fee.py::test_free": "def test_free():\n    assert fee(31) == 0\n    assert fee(32) == 0\n    assert fee(33) == 0\n    assert fee(34) == 0\n",
    }
    predictions = {"predictions": [{"operation": "fee", "arguments": [str(n)]} for n in (31, 32, 33, 34)]}
    answers = happy() | {
        "probe-spec-0": [predictions],
        "probe-spec-1": [predictions],
        "spec-fix": [spec("fee(minutes=8) => 0")],
    }
    dropped, model = build(answers, inputs(oracle_sources))
    assert isinstance(dropped, Dropped) and dropped.stage is DropStage.LEAK
    assert (
        model.keys.count("probe-issue") == 1
        and "predictable" in next(r for r in model.requests if r.key.endswith("spec-fix")).prompt
    )


def test_h_tests_that_break_the_layout_are_rewritten_once_then_the_task_is_dropped() -> None:
    wrong = {"files": [{"path": "tests/elsewhere/test_fee.py", "content": H_FILE}]}
    repaired, model = build(happy() | {"h-0": [wrong], "h-1": [happy()["h-0"][0]]})
    assert isinstance(repaired, Constructed) and "live at tests/spec_h" in model.requests[5].prompt
    dropped, _ = build(happy() | {"h-0": [wrong], "h-1": [wrong]})
    assert isinstance(dropped, Dropped) and dropped.stage is DropStage.H_TESTS


def test_a_test_the_two_models_disagree_on_moves_to_o_and_its_files_are_sealed_under_o() -> None:
    answers = happy() | {"assign-0": [classification(), classification(implied=False, criterion=None)]}
    built, _ = build(answers)
    assert isinstance(built, Constructed)
    manifest = json.loads(built.bundle.files["oracle/manifest.json"])
    assert {m["part"] for m in manifest} == {"O", "R", "H"}
    assert "oracle/O/tests/test_fee.py" in built.bundle.files
    record = json.loads(built.bundle.files["construction/assignment.json"])
    assert record["o"] == ["tests/test_fee.py::test_free"] and record["disagreements"] == [
        "tests/test_fee.py::test_free"
    ]


def test_too_few_applicable_violations_are_asked_for_again_then_the_task_is_dropped() -> None:
    short = {
        "violations": VIOLATIONS["violations"][:2] + [{"criterion": 2, "title": "z", "edits": [edit("absent", "x")]}]
    }
    repaired, model = build(happy() | {"violations-0": [short], "violations-1": [VIOLATIONS]})
    assert isinstance(repaired, Constructed) and "criterion 2 has 0 applicable violations" in model.requests[-2].prompt
    dropped, _ = build(happy() | {"violations-0": [short], "violations-1": [short]})
    assert isinstance(dropped, Dropped) and dropped.stage is DropStage.VIOLATIONS


def test_a_payload_that_breaks_its_schema_drops_the_task_at_its_stage() -> None:
    dropped, _ = build(happy() | {"review": [{"findings": [{"criterion": 1}]}]})
    assert isinstance(dropped, Dropped) and dropped.stage is DropStage.SPEC and "review" in dropped.reason


@pytest.mark.parametrize("prompt", list(Prompt))
def test_every_committed_prompt_is_ascii_and_its_placeholders_all_fill(prompt: Prompt) -> None:
    template = load_prompts()[prompt]
    names = {name for m in Template.pattern.finditer(template) if (name := m.group("named") or m.group("braced"))}
    assert "$" not in fill(template, **dict.fromkeys(names, "x"))
    assert template.isascii() and template.strip()
