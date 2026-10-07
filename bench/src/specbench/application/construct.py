"""Construction of one task: spec, leak controls, H tests, U to O assignment, violations and planted refactors.

Every model call goes through a `ConstructionModel`, which the composition root wraps with pacing, archiving and
resumption; a rerun therefore replays every call already made and spends nothing on it. The result is either the
files of the plain task, ready for `verify-task` and sealing, or a drop with its stage.

Order, with the reason for it:

1. the spec is drafted, checked, reviewed and revised once;
2. the leak controls run on the final spec, and a spec that leaks is fixed once or the task is dropped;
3. H tests are written from that final spec and its Interface only;
4. U assertions are classified against the same spec;
5. violations and planted refactors are written against the merge code.
"""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, replace
from enum import StrEnum
from typing import Final

from specbench.application.construction_requests import (
    AuthorView,
    SchemaLibrary,
    assignment_request,
    h_writer_request,
    planted_request,
    probe_request,
    reviewer_request,
    spec_author_request,
    spec_revision_request,
    violation_request,
)
from specbench.application.ports import ConstructionModel, PatchBuilder, SchemaChecker
from specbench.domain.assignment import Assignment, Classification, assign, classifications_from_record
from specbench.domain.canonical import JsonValue, canonical_json
from specbench.domain.changed_tests import without_parameters
from specbench.domain.construction import ModelRequest, Role
from specbench.domain.edits import (
    AppliedChange,
    applicable,
    refactors_from_record,
    select_planted,
    select_violations,
    violations_from_record,
)
from specbench.domain.h_tests import HFile, HTest, go_directory_of, h_files_from_record, h_problems
from specbench.domain.jsonread import mapping, optional_text, sequence, text
from specbench.domain.languages import Language
from specbench.domain.leaks import (
    ProbeOutcome,
    literal_leaks,
    oracle_tuples,
    probe_hits,
)
from specbench.domain.prompts import PromptLibrary
from specbench.domain.sealing import SealedBundle
from specbench.domain.selection import ProbedTask
from specbench.domain.spec import (
    SPEC_FILE_NAME,
    SpecDraft,
    criteria_of,
    criterion_id,
    render_spec,
    spec_from_record,
    spec_problems,
    spec_to_record,
)
from specbench.domain.task import Criterion, Task

ASSIGNMENT_BATCH_SIZE: Final = 30
FIX_ATTEMPTS: Final = 1
NO_REPAIR: Final = ""


class DropStage(StrEnum):
    """The stage at which a task leaves construction; each is logged with its reason."""

    GROUND = "ground"
    SPEC = "spec"
    LEAK = "leak"
    H_TESTS = "h-tests"
    VIOLATIONS = "violations"
    PLANTED = "planted"
    VERIFICATION = "verification"


@dataclass(frozen=True)
class HumanTests:
    """The human parts of the oracle as the probe found them: U and R ids, the source of each U test, the files to overlay."""

    u_ids: tuple[str, ...]
    r_ids: tuple[str, ...]
    u_sources: Mapping[str, str]
    overlay: Mapping[str, bytes]


@dataclass(frozen=True)
class ConstructionInputs:
    task: ProbedTask
    module: str
    issue: str
    pull_title: str
    pull_body: str
    diff: str
    public_api: str
    reference_texts: tuple[str, ...]
    merge_files: Mapping[str, str]
    human: HumanTests

    @property
    def language(self) -> Language:
        return self.task.candidate.language

    def author_view(self) -> AuthorView:
        """The spec author's view: no field of it can hold a test."""
        return AuthorView(
            self.task.task_id, self.language, self.issue, self.pull_title, self.pull_body, self.diff, self.public_api
        )

    @property
    def description(self) -> str:
        """The change as its authors described it, before any spec: the issue, else the pull request text."""
        return self.issue or f"{self.pull_title}\n\n{self.pull_body}"


@dataclass(frozen=True)
class Dropped:
    stage: DropStage
    reason: str


@dataclass(frozen=True)
class Constructed:
    task: Task
    bundle: SealedBundle


class ConstructionFailure(Exception):
    """A stage that cannot produce a valid artifact; the task is dropped at that stage."""

    def __init__(self, stage: DropStage, reason: str) -> None:
        super().__init__(reason)
        self.dropped = Dropped(stage, reason)


class ModelOutputInvalid(ConstructionFailure):
    pass


class TaskConstruction:
    def __init__(
        self,
        model: ConstructionModel,
        prompts: PromptLibrary,
        schemas: SchemaLibrary,
        checker: SchemaChecker,
        patches: PatchBuilder,
    ) -> None:
        self._model = model
        self._prompts = prompts
        self._schemas = schemas
        self._checker = checker
        self._patches = patches

    def build(self, inputs: ConstructionInputs) -> Constructed | Dropped:
        try:
            return self._build(inputs)
        except ConstructionFailure as failure:
            return failure.dropped

    def _build(self, inputs: ConstructionInputs) -> Constructed:
        task_id = inputs.task.task_id
        if not inputs.human.u_ids:
            raise ConstructionFailure(DropStage.GROUND, "no fail-to-pass test passes stably at merge")
        draft = self._final_spec(inputs)
        spec_text = render_spec(task_id, draft)
        h_files, h_tests = self._write_h_tests(inputs, draft, spec_text)
        assignment, first, second = self._assign(inputs, spec_text, len(draft.criteria))
        violations = self._violations(inputs, spec_text, len(draft.criteria))
        planted = self._planted(inputs, spec_text)
        task = replace_criteria(inputs.task.to_task(), criteria_of(task_id, draft))
        files = assemble_files(
            task_id,
            spec_text,
            draft,
            inputs,
            h_files,
            h_tests,
            assignment,
            (first, second),
            self._patch_files(inputs, violations, planted),
        )
        return Constructed(task, SealedBundle(files))

    # -- model calls ---------------------------------------------------------------------------------------------

    def _structured[T](self, request: ModelRequest, stage: DropStage, read: Callable[[JsonValue], T]) -> T:
        response = self._model.call(request)
        problems = self._checker.problems(request.schema, response.payload)
        if problems:
            raise ModelOutputInvalid(stage, f"{request.key}: {'; '.join(problems[:3])}")
        try:
            return read(response.payload)
        except (KeyError, TypeError, ValueError) as error:
            raise ModelOutputInvalid(stage, f"{request.key}: unreadable payload ({error})") from error

    # -- spec ----------------------------------------------------------------------------------------------------

    def _draft(self, inputs: ConstructionInputs, key: str) -> SpecDraft:
        request = spec_author_request(self._prompts, self._schemas, inputs.author_view(), key)
        return self._structured(request, DropStage.SPEC, spec_from_record)

    def _revise(self, inputs: ConstructionInputs, draft: SpecDraft, findings: Sequence[str], key: str) -> SpecDraft:
        request = spec_revision_request(self._prompts, self._schemas, inputs.author_view(), draft, findings, key)
        return self._structured(request, DropStage.SPEC, spec_from_record)

    def _valid(self, draft: SpecDraft) -> SpecDraft | list[str]:
        problems = spec_problems(draft)
        return problems or draft

    def _reviewed_spec(self, inputs: ConstructionInputs) -> SpecDraft:
        task_id = inputs.task.task_id
        draft = self._draft(inputs, f"{task_id}:spec-draft")
        checked = self._valid(draft)
        if isinstance(checked, list):
            draft = self._revise(inputs, draft, checked, f"{task_id}:spec-repair")
            checked = self._valid(draft)
            if isinstance(checked, list):
                raise ConstructionFailure(DropStage.SPEC, "; ".join(checked[:3]))
        flags = self._review(render_spec(task_id, draft), f"{task_id}:review")
        if not flags:
            return draft
        revised = self._revise(inputs, draft, flags, f"{task_id}:spec-revise")
        checked = self._valid(revised)
        if isinstance(checked, list):
            raise ConstructionFailure(DropStage.SPEC, "; ".join(checked[:3]))
        return revised

    def _review(self, spec_text: str, key: str) -> list[str]:
        request = reviewer_request(self._prompts, self._schemas, spec_text, key)

        def read(payload: JsonValue) -> list[str]:
            findings = sequence(mapping(payload, "review").get("findings"), "findings")
            return [_flag(mapping(f, "finding")) for f in findings]

        return self._structured(request, DropStage.SPEC, read)

    def _final_spec(self, inputs: ConstructionInputs) -> SpecDraft:
        task_id = inputs.task.task_id
        draft = self._reviewed_spec(inputs)
        oracle = oracle_tuples(inputs.language, inputs.human.u_sources)
        issue_hits: int | None = None
        for attempt in range(FIX_ATTEMPTS + 1):
            literal = literal_leaks(oracle, draft, inputs.reference_texts)
            findings = [literal_finding(literal)] if literal else []
            if not findings and oracle:
                if issue_hits is None:
                    issue_hits = self._hits(inputs, oracle, None, f"{task_id}:probe-issue")
                outcome = ProbeOutcome(issue_hits, self._hits(inputs, oracle, draft, f"{task_id}:probe-spec-{attempt}"))
                findings = [probe_finding(outcome)] if outcome.leaks else []
            if not findings:
                return draft
            if attempt == FIX_ATTEMPTS:
                raise ConstructionFailure(DropStage.LEAK, "; ".join(findings))
            draft = self._fixed(inputs, draft, findings, f"{task_id}:spec-fix")
        raise AssertionError("the loop returns or raises")  # pragma: no cover

    def _fixed(self, inputs: ConstructionInputs, draft: SpecDraft, findings: Sequence[str], key: str) -> SpecDraft:
        revised = self._revise(inputs, draft, findings, key)
        checked = self._valid(revised)
        if isinstance(checked, list):
            raise ConstructionFailure(DropStage.SPEC, "; ".join(checked[:3]))
        return revised

    def _hits(self, inputs: ConstructionInputs, oracle: set[frozenset[str]], draft: SpecDraft | None, key: str) -> int:
        """Predicted oracle tuples when the probe reads the issue alone (`draft` None) or the issue plus the spec."""
        description = inputs.description
        if draft is not None:
            description += "\n\n## Specification\n\n" + render_spec(inputs.task.task_id, draft)
        request = probe_request(self._prompts, self._schemas, description, inputs.public_api, key)

        def read(payload: JsonValue) -> int:
            predictions = sequence(mapping(payload, "probe").get("predictions"), "predictions")
            arguments = [
                [text(a, "argument") for a in sequence(mapping(p, "prediction").get("arguments"), "arguments")]
                for p in predictions
            ]
            return probe_hits(oracle, arguments)

        return self._structured(request, DropStage.LEAK, read)

    # -- H tests -------------------------------------------------------------------------------------------------

    def _write_h_tests(
        self, inputs: ConstructionInputs, draft: SpecDraft, spec_text: str
    ) -> tuple[tuple[HFile, ...], list[HTest]]:
        task_id = inputs.task.task_id
        directories = frozenset(
            d for e in draft.interface if (d := go_directory_of(e.import_path.strip(), inputs.module)) is not None
        )
        repair = NO_REPAIR
        problems: list[str] = []
        for attempt in range(FIX_ATTEMPTS + 1):
            request = h_writer_request(
                self._prompts, self._schemas, inputs.language, spec_text, repair, f"{task_id}:h-{attempt}"
            )
            files = self._structured(request, DropStage.H_TESTS, h_files_from_record)
            problems, tests = h_problems(inputs.language, files, len(draft.criteria), directories, inputs.module)
            if not problems:
                return files, tests
            repair = "\n## Problems with the previous attempt\n\n" + "\n".join(f"- {p}" for p in problems)
        raise ConstructionFailure(DropStage.H_TESTS, "; ".join(problems[:3]))

    # -- U to O --------------------------------------------------------------------------------------------------

    def _classify(
        self, inputs: ConstructionInputs, role: Role, spec_text: str, criteria_count: int, ids: Sequence[str]
    ) -> dict[str, Classification]:
        found: dict[str, Classification] = {}
        for start in range(0, len(ids), ASSIGNMENT_BATCH_SIZE):
            batch = ids[start : start + ASSIGNMENT_BATCH_SIZE]
            request = assignment_request(
                self._prompts,
                self._schemas,
                role,
                spec_text,
                criteria_count,
                {t: inputs.human.u_sources[t] for t in batch},
                f"{inputs.task.task_id}:assign-{start // ASSIGNMENT_BATCH_SIZE}",
            )
            found |= self._structured(request, DropStage.SPEC, classifications_from_record)
        return found

    def _assign(
        self, inputs: ConstructionInputs, spec_text: str, criteria_count: int
    ) -> tuple[Assignment, dict[str, Classification], dict[str, Classification]]:
        ids = sorted({without_parameters(t) for t in inputs.human.u_ids})
        first = self._classify(inputs, Role.ASSIGNER_OPUS, spec_text, criteria_count, ids)
        second = self._classify(inputs, Role.ASSIGNER_HAIKU, spec_text, criteria_count, ids)
        return assign(ids, first, second, criteria_count), first, second

    # -- violations and planted refactors -------------------------------------------------------------------------

    def _violations(self, inputs: ConstructionInputs, spec_text: str, criteria_count: int) -> list[AppliedChange]:
        task_id = inputs.task.task_id
        allowed = frozenset(inputs.merge_files)
        repair = NO_REPAIR
        short: list[str] = []
        for attempt in range(FIX_ATTEMPTS + 1):
            request = violation_request(
                self._prompts,
                self._schemas,
                spec_text,
                criteria_count,
                inputs.merge_files,
                repair,
                f"{task_id}:violations-{attempt}",
            )
            changes = self._structured(request, DropStage.VIOLATIONS, violations_from_record)
            applied, problems = applicable(changes, inputs.merge_files, allowed)
            chosen, short = select_violations(applied, criteria_count)
            if not short:
                return chosen
            repair = _repair_text(problems + short)
        raise ConstructionFailure(DropStage.VIOLATIONS, "; ".join(short[:3]))

    def _planted(self, inputs: ConstructionInputs, spec_text: str) -> list[AppliedChange]:
        task_id = inputs.task.task_id
        allowed = frozenset(inputs.merge_files)
        repair = NO_REPAIR
        short: list[str] = []
        for attempt in range(FIX_ATTEMPTS + 1):
            request = planted_request(
                self._prompts, self._schemas, spec_text, inputs.merge_files, repair, f"{task_id}:planted-{attempt}"
            )
            changes = self._structured(request, DropStage.PLANTED, refactors_from_record)
            applied, problems = applicable(changes, inputs.merge_files, allowed)
            chosen, short = select_planted(applied)
            if not short:
                return chosen
            repair = _repair_text(problems + short)
        raise ConstructionFailure(DropStage.PLANTED, "; ".join(short[:3]))

    def _patch_files(
        self, inputs: ConstructionInputs, violations: Sequence[AppliedChange], planted: Sequence[AppliedChange]
    ) -> dict[str, bytes]:
        task_id = inputs.task.task_id
        files: dict[str, bytes] = {}
        violation_index: list[JsonValue] = []
        counters: dict[int, int] = {}
        for applied in violations:
            number = applied.change.criterion or 0
            counters[number] = counters.get(number, 0) + 1
            name = f"V{number}-{counters[number]}"
            files[f"violations/{name}.patch"] = self._patch(inputs, applied)
            violation_index.append(
                {"criterion": criterion_id(task_id, number), "id": name, "patch": f"violations/{name}.patch"}
            )
        planted_index: list[JsonValue] = []
        for position, applied in enumerate(planted, start=1):
            name = f"P{position}"
            files[f"planted/{name}.patch"] = self._patch(inputs, applied)
            planted_index.append({"id": name, "patch": f"planted/{name}.patch"})
        files["violations/index.json"] = canonical_json(violation_index).encode()
        files["planted/index.json"] = canonical_json(planted_index).encode()
        return files

    def _patch(self, inputs: ConstructionInputs, applied: AppliedChange) -> bytes:
        before = {path: inputs.merge_files[path] for path in applied.files}
        return self._patches.build(before, applied.files)


def _flag(entry: Mapping[str, object]) -> str:
    where = entry.get("criterion")
    scope = "the overview or the Interface" if where is None else f"criterion {where}"
    return f"{scope} ({text(entry.get('kind'), 'kind')}): {optional_text(entry.get('comment'), 'comment') or ''}"


def literal_finding(literal: Sequence[frozenset[str]]) -> str:
    rendered = ", ".join("(" + ", ".join(sorted(values)) + ")" for values in literal[:5])
    return (
        f"the examples spell out input values that no public source states: {rendered}; "
        "replace them with different representative values, or state the rule in words and drop the example"
    )


def probe_finding(outcome: ProbeOutcome) -> str:
    return (
        "the specification makes boundary inputs predictable: a reader predicts "
        f"{outcome.excess} more of the inputs a test would use than from the description alone; "
        "state the rules in words without exact boundary values and keep examples to representative values"
    )


def _repair_text(problems: Sequence[str]) -> str:
    return "\n## Problems with the previous attempt\n\n" + "\n".join(f"- {p}" for p in problems[:10])


def replace_criteria(task: Task, criteria: tuple[Criterion, ...]) -> Task:
    return replace(task, criteria=criteria)


def assemble_files(
    task_id: str,
    spec_text: str,
    draft: SpecDraft,
    inputs: ConstructionInputs,
    h_files: Sequence[HFile],
    h_tests: Sequence[HTest],
    assignment: Assignment,
    classifications: tuple[Mapping[str, Classification], Mapping[str, Classification]],
    patch_files: Mapping[str, bytes],
) -> dict[str, bytes]:
    """The files of the plain task bundle: spec, oracle parts with their manifest, patches and the sealed records."""
    manifest: list[JsonValue] = []
    for test_id in sorted(inputs.human.u_ids):
        base = without_parameters(test_id)
        if base in assignment.u:
            manifest.append({"criterion": criterion_id(task_id, assignment.u[base]), "id": test_id, "part": "U"})
        else:
            manifest.append({"criterion": None, "id": test_id, "part": "O"})
    manifest += [{"criterion": None, "id": test_id, "part": "R"} for test_id in sorted(inputs.human.r_ids)]
    manifest += [
        {"criterion": criterion_id(task_id, t.number), "id": t.test_id, "part": "H"}
        for t in sorted(h_tests, key=lambda t: t.test_id)
    ]
    files: dict[str, bytes] = {
        SPEC_FILE_NAME: spec_text.encode(),
        "oracle/manifest.json": canonical_json(manifest).encode(),
        "construction/spec.json": canonical_json(spec_to_record(draft)).encode(),
        "construction/assignment.json": canonical_json(
            assignment.to_record(classifications[0], classifications[1])
        ).encode(),
        **patch_files,
    }
    has_o = bool(assignment.o)
    for path, content in inputs.human.overlay.items():
        files[f"oracle/U/{path}"] = content
        if has_o:
            files[f"oracle/O/{path}"] = content
    for file in h_files:
        files[f"oracle/H/{file.path}"] = file.content.encode()
    return files
