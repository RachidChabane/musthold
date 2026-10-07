"""Mined candidates, the attrition log and the seeded sample of distinct repositories."""

from __future__ import annotations

from collections import Counter
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from datetime import date

from specbench.domain.canonical import JsonValue
from specbench.domain.languages import Language
from specbench.domain.mining import (
    SAMPLE_TASKS_PER_LANGUAGE,
    STAGE_ORDER,
    BaseSnapshot,
    PullRequest,
    Repository,
    Stage,
    changed_tests,
    non_test_changes,
    seeded_key,
)

TASK_ID_SEPARATOR = "-"


@dataclass(frozen=True)
class Candidate:
    repository: str
    language: Language
    stars: int
    spdx_id: str
    pull_number: int
    title: str
    merged_on: date
    base_sha: str
    merge_sha: str
    non_test_files: tuple[str, ...]
    non_test_lines: int
    test_files: tuple[str, ...]
    licence_path: str
    notice_path: str | None

    @property
    def label(self) -> str:
        return f"{self.repository}#{self.pull_number}"

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "base_sha": self.base_sha,
            "language": self.language.value,
            "licence_path": self.licence_path,
            "merge_sha": self.merge_sha,
            "merged_on": self.merged_on.isoformat(),
            "non_test_files": list(self.non_test_files),
            "non_test_lines": self.non_test_lines,
            "notice_path": self.notice_path,
            "pull_number": self.pull_number,
            "repository": self.repository,
            "spdx_id": self.spdx_id,
            "stars": self.stars,
            "test_files": list(self.test_files),
            "title": self.title,
        }


@dataclass(frozen=True)
class AttritionEntry:
    subject: str
    stage: Stage
    reason: str

    def to_record(self) -> dict[str, JsonValue]:
        return {"reason": self.reason, "stage": self.stage.value, "subject": self.subject}


def build_candidate(repository: Repository, pull: PullRequest, base: BaseSnapshot) -> Candidate:
    """A candidate that passed every filter; the base snapshot supplies the licence and notice paths."""
    if base.licence_spdx is None or base.licence_path is None:
        raise ValueError("a candidate needs the licence found at base")
    sources = non_test_changes(repository.language, pull.files)
    return Candidate(
        repository=repository.full_name,
        language=repository.language,
        stars=repository.stars,
        spdx_id=base.licence_spdx,
        pull_number=pull.number,
        title=pull.title,
        merged_on=pull.merged_on,
        base_sha=pull.base_sha,
        merge_sha=pull.merge_sha,
        non_test_files=tuple(sorted(f.path for f in sources)),
        non_test_lines=sum(f.lines for f in sources),
        test_files=tuple(sorted(f.path for f in changed_tests(repository.language, pull.files))),
        licence_path=base.licence_path,
        notice_path=base.notice_path,
    )


def order_candidates(seed: int, candidates: Iterable[Candidate]) -> list[Candidate]:
    return sorted(candidates, key=lambda c: seeded_key(seed, c.label))


def select_sample(
    seed: int, candidates: Sequence[Candidate], per_language: int = SAMPLE_TASKS_PER_LANGUAGE
) -> list[Candidate]:
    """The sample: per language, the first `per_language` repositories in seeded order, one task each."""
    chosen: list[Candidate] = []
    for language in Language:
        seen: set[str] = set()
        for candidate in order_candidates(seed, (c for c in candidates if c.language is language)):
            if candidate.repository not in seen and len(seen) < per_language:
                seen.add(candidate.repository)
                chosen.append(candidate)
    return chosen


def attrition_counts(entries: Iterable[AttritionEntry]) -> Mapping[str, int]:
    """Rejections per stage, every stage present, in the stage order of the funnel."""
    counted = Counter(entry.stage for entry in entries)
    return {stage.value: counted.get(stage, 0) for stage in STAGE_ORDER}


def task_id_for(candidate: Candidate) -> str:
    """`<owner>-<repo>-<number>` in upper case, the prefix of the task's criterion ids."""
    slug = (
        candidate.repository.replace("/", TASK_ID_SEPARATOR)
        .replace("_", TASK_ID_SEPARATOR)
        .replace(".", TASK_ID_SEPARATOR)
    )
    return f"{slug.upper()}{TASK_ID_SEPARATOR}{candidate.pull_number}"
