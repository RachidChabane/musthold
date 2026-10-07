"""`task.json`: the plaintext record of one corpus task, and its canonical serialization."""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import date
from enum import Enum
from typing import Final

from specbench.domain.canonical import JsonValue, canonical_json, parse_canonical
from specbench.domain.jsonread import RecordError, enum_of, integer, mapping, optional_text, sequence, text
from specbench.domain.languages import Language
from specbench.domain.licence import PERMISSIVE_SPDX

TASK_SCHEMA: Final = "specbench.task/1"
TASK_FILE_NAME: Final = "task.json"
SEALED_FILE_NAME: Final = "sealed.age"
SHA_PATTERN: Final = re.compile(r"^[0-9a-f]{40}$")
DIGEST_PATTERN: Final = re.compile(r"^[0-9a-f]{64}$")
TASK_ID_PATTERN: Final = re.compile(r"^[A-Z0-9]+(?:-[A-Z0-9]+)*$")
CRITERION_ID_PATTERN: Final = re.compile(r"^(?P<task>[A-Z0-9]+(?:-[A-Z0-9]+)*)-(?P<number>[1-9][0-9]*)$")
REPOSITORY_PATTERN: Final = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")


TaskError = RecordError


class CriterionStatus(Enum):
    NEW = "new"
    REVISED = "revised"
    UNCHANGED = "unchanged"


@dataclass(frozen=True)
class Criterion:
    criterion_id: str
    status: CriterionStatus

    @property
    def is_new_or_revised(self) -> bool:
        return self.status in (CriterionStatus.NEW, CriterionStatus.REVISED)


@dataclass(frozen=True)
class Toolchain:
    name: str
    version: str


@dataclass(frozen=True)
class Task:
    task_id: str
    language: Language
    repository: str
    pull_number: int
    merged_on: date
    spdx_id: str
    licence_path: str
    notice_path: str | None
    base_sha: str
    merge_sha: str
    lockfile_digests: Mapping[str, str]
    toolchain: Toolchain
    criteria: tuple[Criterion, ...]

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "base_sha": self.base_sha,
            "criteria": [{"id": c.criterion_id, "status": c.status.value} for c in self.criteria],
            "language": self.language.value,
            "licence": {"notice_path": self.notice_path, "path": self.licence_path, "spdx_id": self.spdx_id},
            "lockfile_digests": dict(sorted(self.lockfile_digests.items())),
            "merge_sha": self.merge_sha,
            "merged_on": self.merged_on.isoformat(),
            "pull_number": self.pull_number,
            "repository": self.repository,
            "schema": TASK_SCHEMA,
            "task_id": self.task_id,
            "toolchain": {"name": self.toolchain.name, "version": self.toolchain.version},
        }

    def to_canonical_json(self) -> str:
        return canonical_json(self.to_record())


def _criteria(value: object, task_id: str) -> tuple[Criterion, ...]:
    entries = sequence(value, "criteria")
    if not entries:
        raise TaskError("criteria must not be empty")
    criteria: list[Criterion] = []
    for index, raw in enumerate(entries):
        entry = mapping(raw, f"criteria[{index}]")
        criterion_id = text(entry.get("id"), f"criteria[{index}].id", CRITERION_ID_PATTERN)
        match = CRITERION_ID_PATTERN.match(criterion_id)
        if match is None or match.group("task") != task_id:
            raise TaskError(f"criterion {criterion_id} does not belong to task {task_id}")
        criteria.append(
            Criterion(criterion_id, enum_of(CriterionStatus, entry.get("status"), f"criteria[{index}].status"))
        )
    ids = [c.criterion_id for c in criteria]
    if len(set(ids)) != len(ids):
        raise TaskError("criterion ids repeat")
    return tuple(criteria)


def task_from_record(record: JsonValue) -> Task:
    """A task from its decoded JSON, refusing every deviation from `specbench.task/1`."""
    body = mapping(record, "task")
    if body.get("schema") != TASK_SCHEMA:
        raise TaskError(f"schema must be {TASK_SCHEMA}")
    task_id = text(body.get("task_id"), "task_id", TASK_ID_PATTERN)
    licence = mapping(body.get("licence"), "licence")
    spdx_id = text(licence.get("spdx_id"), "licence.spdx_id")
    if spdx_id not in PERMISSIVE_SPDX:
        raise TaskError(f"licence {spdx_id} is not OSI-permissive")
    digests = mapping(body.get("lockfile_digests"), "lockfile_digests")
    toolchain = mapping(body.get("toolchain"), "toolchain")
    try:
        merged_on = date.fromisoformat(text(body.get("merged_on"), "merged_on"))
    except ValueError as error:
        raise TaskError(str(error)) from error
    return Task(
        task_id=task_id,
        language=enum_of(Language, body.get("language"), "language"),
        repository=text(body.get("repository"), "repository", REPOSITORY_PATTERN),
        pull_number=integer(body.get("pull_number"), "pull_number", 1),
        merged_on=merged_on,
        spdx_id=spdx_id,
        licence_path=text(licence.get("path"), "licence.path"),
        notice_path=optional_text(licence.get("notice_path"), "licence.notice_path"),
        base_sha=text(body.get("base_sha"), "base_sha", SHA_PATTERN),
        merge_sha=text(body.get("merge_sha"), "merge_sha", SHA_PATTERN),
        lockfile_digests={
            text(path, "lockfile path"): text(digest, f"lockfile_digests[{path}]", DIGEST_PATTERN)
            for path, digest in digests.items()
        },
        toolchain=Toolchain(
            text(toolchain.get("name"), "toolchain.name"), text(toolchain.get("version"), "toolchain.version")
        ),
        criteria=_criteria(body.get("criteria"), task_id),
    )


def parse_task(text_: str) -> Task:
    """A task from the text of a `task.json`, which must be in canonical form."""
    return task_from_record(parse_canonical(text_))
