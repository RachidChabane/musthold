"""The prompt of each construction role, built from exactly the inputs ADR 0004 gives that role.

Blindness is structural: every builder takes a value object that carries only what its role may see. `AuthorView`
has no field for a test; `h_writer_request` takes the rendered spec and nothing else of the task, and
`h_refill_request` adds only criterion numbers and a round, never an outcome of an earlier writing.
"""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from specbench.domain.canonical import JsonValue
from specbench.domain.construction import ModelRequest, Role
from specbench.domain.h_tests import MIN_EXAMPLE_TESTS, MIN_PROPERTY_TESTS, POOL_SIZE
from specbench.domain.languages import Language
from specbench.domain.prompts import Prompt, PromptLibrary, fill
from specbench.domain.spec import SpecDraft, spec_to_record

NO_ISSUE = "No issue is linked to the pull request."
NO_FINDINGS = "None."
SchemaLibrary = Mapping[str, JsonValue]


@dataclass(frozen=True)
class AuthorView:
    """What the spec author sees: the linked issue, the pull request description, the non-test diff and the public API."""

    task_id: str
    language: Language
    issue: str
    pull_title: str
    pull_body: str
    diff: str
    public_api: str


def _author_inputs(prompts: PromptLibrary, view: AuthorView) -> str:
    return fill(
        prompts[Prompt.SPEC_AUTHOR_USER],
        task_id=view.task_id,
        language=view.language.value,
        issue=view.issue or NO_ISSUE,
        pull_title=view.pull_title,
        pull_body=view.pull_body or "(no description)",
        diff=view.diff,
        public_api=view.public_api,
    )


def spec_author_request(prompts: PromptLibrary, schemas: SchemaLibrary, view: AuthorView, key: str) -> ModelRequest:
    return ModelRequest(
        Role.SPEC_AUTHOR, key, prompts[Prompt.SPEC_AUTHOR_SYSTEM], _author_inputs(prompts, view), schemas["spec"]
    )


def spec_revision_request(
    prompts: PromptLibrary,
    schemas: SchemaLibrary,
    view: AuthorView,
    draft: SpecDraft,
    findings: Sequence[str],
    key: str,
) -> ModelRequest:
    prompt = fill(
        prompts[Prompt.SPEC_REVISE_USER],
        inputs=_author_inputs(prompts, view).rstrip(),
        spec=json.dumps(spec_to_record(draft), indent=2, sort_keys=True),
        findings="\n".join(f"- {finding}" for finding in findings) or NO_FINDINGS,
    )
    return ModelRequest(Role.SPEC_AUTHOR, key, prompts[Prompt.SPEC_AUTHOR_SYSTEM], prompt, schemas["spec"])


def _annotations(draft: SpecDraft) -> str:
    """Per criterion, the dimension and the implementing hunks the spec author cited; the agent never sees them."""
    return json.dumps(
        [
            {
                "criterion": c.number,
                "dimension": c.dimension.value,
                "implementingHunks": [h.to_record() for h in c.implementing_hunks],
            }
            for c in draft.criteria
        ],
        indent=2,
        sort_keys=True,
    )


def reviewer_request(
    prompts: PromptLibrary, schemas: SchemaLibrary, spec_text: str, draft: SpecDraft, diff: str, key: str
) -> ModelRequest:
    """The reviewer sees the rendered spec, the dimension and hunks cited per criterion, and the non-test diff."""
    prompt = fill(prompts[Prompt.REVIEWER_USER], spec=spec_text, annotations=_annotations(draft), diff=diff)
    return ModelRequest(Role.REVIEWER, key, prompts[Prompt.REVIEWER_SYSTEM], prompt, schemas["review"])


def _h_writer_system(prompts: PromptLibrary, language: Language) -> str:
    layout = prompts[Prompt.H_LAYOUT_PYTHON if language is Language.PYTHON else Prompt.H_LAYOUT_GO]
    return fill(
        prompts[Prompt.H_WRITER_SYSTEM],
        layout_rules=layout.rstrip(),
        pool_size=str(POOL_SIZE),
        min_property=str(MIN_PROPERTY_TESTS),
        min_example=str(MIN_EXAMPLE_TESTS),
    )


def h_writer_request(
    prompts: PromptLibrary, schemas: SchemaLibrary, language: Language, spec_text: str, repair: str, key: str
) -> ModelRequest:
    """The H writer sees the rendered spec, which holds the Interface, and the layout rules of its language."""
    prompt = fill(
        prompts[Prompt.H_WRITER_USER], language=language.value, spec=spec_text, pool_size=str(POOL_SIZE), repair=repair
    )
    return ModelRequest(Role.H_WRITER, key, _h_writer_system(prompts, language), prompt, schemas["h-tests"])


def h_refill_request(
    prompts: PromptLibrary,
    schemas: SchemaLibrary,
    language: Language,
    spec_text: str,
    criteria: Sequence[int],
    writing_round: int,
    repair: str,
    key: str,
) -> ModelRequest:
    """A fresh, independent H writing for some criteria: the rendered spec, their numbers, the pool size and the round.

    Nothing about earlier writings reaches it: no test, no outcome, no count of what was kept.
    """
    prompt = fill(
        prompts[Prompt.H_REFILL_USER],
        language=language.value,
        spec=spec_text,
        pool_size=str(POOL_SIZE),
        criteria=", ".join(str(n) for n in criteria),
        round=str(writing_round),
        repair=repair,
    )
    return ModelRequest(Role.H_WRITER, key, _h_writer_system(prompts, language), prompt, schemas["h-tests"])


def _sources_block(files: Mapping[str, str]) -> str:
    return "\n\n".join(f"### {path}\n\n```\n{files[path]}\n```" for path in sorted(files))


def violation_request(
    prompts: PromptLibrary,
    schemas: SchemaLibrary,
    spec_text: str,
    criteria_count: int,
    files: Mapping[str, str],
    repair: str,
    key: str,
) -> ModelRequest:
    prompt = fill(
        prompts[Prompt.VIOLATION_AUTHOR_USER],
        criteria_count=str(criteria_count),
        spec=spec_text,
        sources=_sources_block(files),
        repair=repair,
    )
    return ModelRequest(
        Role.VIOLATION_AUTHOR, key, prompts[Prompt.VIOLATION_AUTHOR_SYSTEM], prompt, schemas["violations"]
    )


def planted_request(
    prompts: PromptLibrary, schemas: SchemaLibrary, spec_text: str, files: Mapping[str, str], repair: str, key: str
) -> ModelRequest:
    prompt = fill(prompts[Prompt.PLANTED_AUTHOR_USER], spec=spec_text, sources=_sources_block(files), repair=repair)
    return ModelRequest(Role.PLANTED_AUTHOR, key, prompts[Prompt.PLANTED_AUTHOR_SYSTEM], prompt, schemas["refactors"])


def assignment_request(
    prompts: PromptLibrary,
    schemas: SchemaLibrary,
    role: Role,
    spec_text: str,
    criteria_count: int,
    test_sources: Mapping[str, str],
    key: str,
) -> ModelRequest:
    tests = "\n\n".join(f"### {test_id}\n\n```\n{test_sources[test_id]}\n```" for test_id in sorted(test_sources))
    prompt = fill(prompts[Prompt.ASSIGNER_USER], criteria_count=str(criteria_count), spec=spec_text, tests=tests)
    return ModelRequest(role, key, prompts[Prompt.ASSIGNER_SYSTEM], prompt, schemas["assignment"])


def probe_request(
    prompts: PromptLibrary, schemas: SchemaLibrary, description: str, public_api: str, key: str
) -> ModelRequest:
    prompt = fill(prompts[Prompt.LEAK_PROBE_USER], description=description, public_api=public_api)
    return ModelRequest(Role.LEAK_PROBE, key, prompts[Prompt.LEAK_PROBE_SYSTEM], prompt, schemas["probe"])
