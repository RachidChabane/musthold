"""A candidate that survived the feasibility probe, and the seeded selection of tasks and reserves."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from specbench.domain.candidates import AttritionEntry, Candidate, task_id_for
from specbench.domain.canonical import JsonValue
from specbench.domain.task import Task, Toolchain


@dataclass(frozen=True)
class ProbedTask:
    """What the probe established about a candidate: its true base, its environment and its human tests."""

    candidate: Candidate
    base_sha: str
    toolchain: Toolchain
    lockfile_digests: Mapping[str, str]
    u_count: int
    r_count: int
    merge_total: int
    ground_digest: str

    @property
    def task_id(self) -> str:
        return task_id_for(self.candidate)

    def to_task(self) -> Task:
        """The task as selected: every fact of `task.json` except the criteria, which the spec author supplies."""
        c = self.candidate
        return Task(
            task_id=self.task_id,
            language=c.language,
            repository=c.repository,
            pull_number=c.pull_number,
            merged_on=c.merged_on,
            spdx_id=c.spdx_id,
            licence_path=c.licence_path,
            notice_path=c.notice_path,
            base_sha=self.base_sha,
            merge_sha=c.merge_sha,
            lockfile_digests=self.lockfile_digests,
            toolchain=self.toolchain,
            criteria=(),
        )

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "api_base_sha": self.candidate.base_sha,
            "base_sha": self.base_sha,
            "ground_digest": self.ground_digest,
            "merge_total": self.merge_total,
            "r_count": self.r_count,
            "task_id": self.task_id,
            "title": self.candidate.title,
            "u_count": self.u_count,
        }


@dataclass(frozen=True)
class Selection:
    """Tasks and reserves per the seeded ranking, and every rejection and skipped candidate with its stage."""

    tasks: tuple[ProbedTask, ...]
    reserves: tuple[ProbedTask, ...]
    attrition: tuple[AttritionEntry, ...]
