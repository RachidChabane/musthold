"""Mined candidates, the attrition log and the seeded sample of distinct repositories."""

from __future__ import annotations

from collections import Counter
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from datetime import date

from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import enum_of, integer, sequence, text
from specbench.domain.languages import Language
from specbench.domain.mining import (
    RESERVE_TASKS_PER_LANGUAGE,
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

    @classmethod
    def from_record(cls, record: Mapping[str, object]) -> Candidate:
        notice = record.get("notice_path")
        return cls(
            repository=text(record.get("repository"), "repository"),
            language=enum_of(Language, record.get("language"), "language"),
            stars=integer(record.get("stars"), "stars"),
            spdx_id=text(record.get("spdx_id"), "spdx_id"),
            pull_number=integer(record.get("pull_number"), "pull_number", 1),
            title=text(record.get("title"), "title"),
            merged_on=date.fromisoformat(text(record.get("merged_on"), "merged_on")),
            base_sha=text(record.get("base_sha"), "base_sha"),
            merge_sha=text(record.get("merge_sha"), "merge_sha"),
            non_test_files=tuple(text(f, "non_test_files") for f in sequence(record.get("non_test_files"), "files")),
            non_test_lines=integer(record.get("non_test_lines"), "non_test_lines"),
            test_files=tuple(text(f, "test_files") for f in sequence(record.get("test_files"), "files")),
            licence_path=text(record.get("licence_path"), "licence_path"),
            notice_path=None if notice is None else text(notice, "notice_path"),
        )

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


def ranking_by_language(seed: int, candidates: Iterable[Candidate]) -> dict[Language, list[Candidate]]:
    """Candidates per language in seeded order, the first of each repository only."""
    ranking: dict[Language, list[Candidate]] = {}
    for language in Language:
        seen: set[str] = set()
        ranked: list[Candidate] = []
        for candidate in order_candidates(seed, (c for c in candidates if c.language is language)):
            if candidate.repository not in seen:
                seen.add(candidate.repository)
                ranked.append(candidate)
        ranking[language] = ranked
    return ranking


@dataclass(frozen=True)
class Sample:
    """The tasks and the reserves, each from a distinct repository; a reserve replaces a task of its language."""

    tasks: tuple[Candidate, ...]
    reserves: tuple[Candidate, ...]

    @property
    def everything(self) -> tuple[Candidate, ...]:
        return self.tasks + self.reserves


def select_sample(
    seed: int,
    candidates: Sequence[Candidate],
    per_language: int = SAMPLE_TASKS_PER_LANGUAGE,
    reserves_per_language: int = RESERVE_TASKS_PER_LANGUAGE,
) -> Sample:
    """Per language, the first `per_language` repositories in seeded order are tasks and the next ones reserves."""
    tasks: list[Candidate] = []
    reserves: list[Candidate] = []
    for ranked in ranking_by_language(seed, candidates).values():
        tasks += ranked[:per_language]
        reserves += ranked[per_language : per_language + reserves_per_language]
    return Sample(tuple(tasks), tuple(reserves))


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
