"""The filters of PREREG 6.4 and 7.1, the exclusions of 3 and the licence rule of 4, on API metadata alone."""

from __future__ import annotations

from collections.abc import Collection, Iterable, Sequence
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

from specbench.domain.licence import PERMISSIVE_SPDX  # pyright: ignore[reportMissingTypeStubs]

from demos1.domain.ecosystem import ChangedFile, Ecosystem, is_source_path, is_test_path
from demos1.domain.jsonvalue import (
    JsonObject,
    as_object,
    children,
    integer,
    optional_boolean,
    optional_child,
    optional_integer,
    optional_text,
    text,
)

MIN_STARS: Final = 1000
MAX_CHANGED_FILES: Final = 100
EXCLUDED_OWNER: Final = "RachidChabane"


class Filter(StrEnum):
    """The filter names a frame record gives as a candidate's first failing filter."""

    SIGNAL = "signal"
    EXCLUDED = "excluded"
    REPOSITORY = "repository"
    PULL_REQUEST = "pull-request"
    TESTS_CHANGED = "tests-changed"
    SOURCE_CHANGED = "source-changed"
    MERGE_DIFF = "merge-diff"
    ECOSYSTEM = "ecosystem"
    COMMIT = "commit"


def licence_of(repository: JsonObject) -> str | None:
    licence = optional_child(repository, "license")
    if licence is None:
        return None
    return optional_text(licence, "spdx_id")


def is_permissive(spdx_id: str | None) -> bool:
    """PREREG 4: one of PERMISSIVE_SPDX; a missing licence, NOASSERTION or any other id is not."""
    return spdx_id in PERMISSIVE_SPDX


@dataclass(frozen=True)
class Exclusions:
    """PREREG 3: the benchmark pool, the owner's repositories and every repository already on a list."""

    pool: frozenset[str]
    listed: frozenset[str] = frozenset()

    def excludes(self, full_name: str) -> bool:
        lowered = full_name.lower()
        owner = lowered.split("/", 1)[0]
        return lowered in self.pool or owner == EXCLUDED_OWNER.lower() or lowered in self.listed

    def with_listed(self, full_name: str) -> Exclusions:
        return Exclusions(self.pool, self.listed | {full_name.lower()})


def exclusion_pool(repositories: Iterable[str]) -> frozenset[str]:
    return frozenset(name.lower() for name in repositories)


def repository_is_eligible(repository: JsonObject, require_stars: bool) -> bool:
    """Filter 3 of 6.4 (with stars) and filter 2 of 7.1 (without): public, not a fork, not archived, permissive."""
    for flag in ("fork", "archived", "private"):
        if optional_boolean(repository, flag) is not False:
            return False
    if require_stars and (optional_integer(repository, "stargazers_count") or 0) < MIN_STARS:
        return False
    return is_permissive(licence_of(repository))


def pull_request_is_eligible(pull: JsonObject) -> bool:
    """Filter 4: merged, with a merge commit, and at most 100 changed files."""
    changed = optional_integer(pull, "changed_files")
    return (
        optional_boolean(pull, "merged") is True
        and bool(optional_text(pull, "merge_commit_sha"))
        and changed is not None
        and changed <= MAX_CHANGED_FILES
    )


def changed_files(files: Sequence[JsonObject]) -> list[ChangedFile]:
    return [ChangedFile(text(entry, "filename"), text(entry, "status")) for entry in files]


def changes_a_test(ecosystem: Ecosystem, files: Iterable[ChangedFile]) -> bool:
    """Filter 5: an added, modified or renamed file the scan's test globs match."""
    return any(entry.counts_as_changed() and is_test_path(ecosystem, entry.path) for entry in files)


def changes_a_source(ecosystem: Ecosystem, files: Iterable[ChangedFile]) -> bool:
    """Filter 6: an added, modified or renamed source file that is neither a test nor conftest.py."""
    return any(entry.counts_as_changed() and is_source_path(ecosystem, entry.path) for entry in files)


def merge_parent(commit: JsonObject) -> str | None:
    parents = children(commit, "parents")
    if not parents:
        return None
    return optional_text(parents[0], "sha")


def merge_diff_covers(files: Iterable[ChangedFile], commit: JsonObject) -> bool:
    """Filter 7: every filename of the pull request's file list appears among the merge commit's files."""
    commit_files = {text(as_object(entry), "filename") for entry in children(commit, "files")}
    return all(entry.path in commit_files for entry in files)


def root_names(listing: Sequence[JsonObject]) -> Collection[str]:
    return {text(entry, "name") for entry in listing}


def stars_of(repository: JsonObject) -> int:
    return integer(repository, "stargazers_count")
