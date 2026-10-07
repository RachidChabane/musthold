"""Corpus filters of ADR 0004 as pure rules over facts read from GitHub: no I/O, no clock."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from datetime import date, timedelta
from enum import Enum
from typing import Final

from specbench.domain import licence
from specbench.domain.languages import Language, is_documentation_path, is_non_test_source_path, is_test_path

MIN_STARS: Final = 100
MERGE_WINDOW_START: Final = date(2026, 7, 1)
FREEZE_MARGIN_DAYS: Final = 30
MIN_NON_TEST_FILES: Final = 1
MAX_NON_TEST_FILES: Final = 6
MIN_NON_TEST_LINES: Final = 30
MAX_NON_TEST_LINES: Final = 400
MAX_TASKS_PER_REPOSITORY: Final = 2
SAMPLE_TASKS_PER_LANGUAGE: Final = 3
WORKFLOW_DIRECTORY: Final = ".github/workflows"
WORKFLOW_SUFFIXES: Final = (".yml", ".yaml")
CHANGED_TEST_STATUSES: Final = frozenset({"added", "modified", "renamed", "changed"})

PYTHON_LOCKFILES: Final = ("uv.lock", "requirements.txt")
GO_LOCKFILES: Final = ("go.mod", "go.sum")
FEATURE_TITLE: Final = re.compile(
    r"^\s*(feat|feature|add|adds|added|support|supports|implement|implements|introduce|allow|enable)\b[(:!\s]",
    re.IGNORECASE,
)
NON_FEATURE_TITLE: Final = re.compile(
    r"^\s*(fix|fixes|docs?|chore|ci|build|test|tests|deps?|bump|refactor|style|revert|release|perf|typo)\b[(:!\s]",
    re.IGNORECASE,
)
FEATURE_LABELS: Final = frozenset(
    {"feature", "enhancement", "feature request", "new feature", "type: feature", "kind/feature"}
)


class Stage(Enum):
    """The stage at which a candidate was rejected; the attrition log counts them."""

    LANGUAGE = "language"
    LICENCE_API = "licence-api"
    STARS = "stars"
    LICENCE_FILE = "licence-file"
    CI_WORKFLOW = "ci-workflow"
    LOCKFILE = "lockfile"
    MERGE_WINDOW = "merge-window"
    FEATURE_SIGNAL = "feature-signal"
    NON_TEST_FILES = "non-test-files"
    NON_TEST_LINES = "non-test-lines"
    TESTS_CHANGED = "tests-changed"
    REPOSITORY_CAP = "repository-cap"
    NOT_SELECTED = "not-selected"


STAGE_ORDER: Final = tuple(Stage)


@dataclass(frozen=True)
class MiningConfig:
    seed: int
    freeze_date: date | None
    languages: tuple[Language, ...]
    candidates_per_language: int

    @property
    def merge_window_end(self) -> date | None:
        return None if self.freeze_date is None else self.freeze_date - timedelta(days=FREEZE_MARGIN_DAYS)


@dataclass(frozen=True)
class Repository:
    full_name: str
    language: Language
    stars: int
    spdx_id: str | None
    default_branch: str


@dataclass(frozen=True)
class BaseSnapshot:
    """What the repository holds at a pull request's base commit."""

    root_files: frozenset[str]
    workflow_files: frozenset[str]
    licence_spdx: str | None
    licence_path: str | None
    notice_path: str | None


@dataclass(frozen=True)
class ChangedFile:
    path: str
    status: str
    additions: int
    deletions: int

    @property
    def lines(self) -> int:
        return self.additions + self.deletions


@dataclass(frozen=True)
class PullRequest:
    number: int
    title: str
    labels: frozenset[str]
    merged_on: date
    base_sha: str
    merge_sha: str
    files: tuple[ChangedFile, ...]


@dataclass(frozen=True)
class Rejection:
    stage: Stage
    reason: str


def seeded_key(seed: int, label: str) -> str:
    """The sort key of a candidate under the recorded seed: a hash, so the order is stable and unbiased."""
    return hashlib.sha256(f"{seed}:{label}".encode()).hexdigest()


def check_repository(repository: Repository, config: MiningConfig) -> Rejection | None:
    if not licence.is_permissive(repository.spdx_id):
        return Rejection(Stage.LICENCE_API, f"licence API reports {repository.spdx_id!r}")
    if repository.stars < MIN_STARS:
        return Rejection(Stage.STARS, f"{repository.stars} stars, at least {MIN_STARS} required")
    if repository.language not in config.languages:
        return Rejection(Stage.LANGUAGE, f"language {repository.language.value} is not sampled")
    return None


def lockfiles_for(language: Language) -> tuple[str, ...]:
    return PYTHON_LOCKFILES if language is Language.PYTHON else GO_LOCKFILES


def has_lockfiles(language: Language, root_files: frozenset[str]) -> bool:
    """Python needs `uv.lock` or a `requirements.txt`; Go needs `go.mod` and `go.sum`."""
    if language is Language.PYTHON:
        return any(name in root_files for name in PYTHON_LOCKFILES)
    return all(name in root_files for name in GO_LOCKFILES)


def has_ci_workflow(workflow_files: frozenset[str]) -> bool:
    return any(name.endswith(WORKFLOW_SUFFIXES) for name in workflow_files)


def check_base(repository: Repository, base: BaseSnapshot) -> Rejection | None:
    if not licence.is_permissive(base.licence_spdx):
        return Rejection(Stage.LICENCE_FILE, f"LICENSE at base reads {base.licence_spdx!r}")
    if base.licence_spdx != repository.spdx_id:
        return Rejection(
            Stage.LICENCE_FILE, f"LICENSE at base ({base.licence_spdx}) differs from the API ({repository.spdx_id})"
        )
    if not has_ci_workflow(base.workflow_files):
        return Rejection(Stage.CI_WORKFLOW, f"no workflow under {WORKFLOW_DIRECTORY} at base")
    if not has_lockfiles(repository.language, base.root_files):
        return Rejection(Stage.LOCKFILE, f"missing lockfiles {lockfiles_for(repository.language)} at base")
    return None


def is_feature_pull_request(title: str, labels: frozenset[str]) -> bool:
    """The feature signal: a feature label, or a feature title that no maintenance prefix contradicts."""
    if labels & FEATURE_LABELS:
        return True
    return FEATURE_TITLE.match(title) is not None and NON_FEATURE_TITLE.match(title) is None


def non_test_changes(language: Language, files: tuple[ChangedFile, ...]) -> tuple[ChangedFile, ...]:
    return tuple(f for f in files if is_non_test_source_path(language, f.path))


def changed_tests(language: Language, files: tuple[ChangedFile, ...]) -> tuple[ChangedFile, ...]:
    return tuple(
        f
        for f in files
        if is_test_path(language, f.path)
        and not is_documentation_path(f.path)
        and f.status in CHANGED_TEST_STATUSES
        and f.additions > 0
    )


def check_pull_request(language: Language, pull: PullRequest, config: MiningConfig) -> Rejection | None:
    if pull.merged_on < MERGE_WINDOW_START or (
        config.merge_window_end is not None and pull.merged_on > config.merge_window_end
    ):
        return Rejection(Stage.MERGE_WINDOW, f"merged {pull.merged_on.isoformat()} is outside the merge window")
    if not is_feature_pull_request(pull.title, pull.labels):
        return Rejection(Stage.FEATURE_SIGNAL, "neither a feature label nor a feature title")
    sources = non_test_changes(language, pull.files)
    if not MIN_NON_TEST_FILES <= len(sources) <= MAX_NON_TEST_FILES:
        return Rejection(
            Stage.NON_TEST_FILES,
            f"{len(sources)} non-test files, {MIN_NON_TEST_FILES} to {MAX_NON_TEST_FILES} required",
        )
    lines = sum(f.lines for f in sources)
    if not MIN_NON_TEST_LINES <= lines <= MAX_NON_TEST_LINES:
        return Rejection(
            Stage.NON_TEST_LINES, f"{lines} non-test lines, {MIN_NON_TEST_LINES} to {MAX_NON_TEST_LINES} required"
        )
    if not changed_tests(language, pull.files):
        return Rejection(Stage.TESTS_CHANGED, "the pull request adds or modifies no test")
    return None
