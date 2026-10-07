"""`mine`: deterministic candidate mining under ADR 0004's filters, with an attrition log and a seeded sample."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from specbench.application.ports import MiningSource
from specbench.domain.candidates import (
    AttritionEntry,
    Candidate,
    Sample,
    attrition_counts,
    build_candidate,
    select_sample,
    task_id_for,
)
from specbench.domain.canonical import JsonValue
from specbench.domain.mining import (
    MAX_TASKS_PER_REPOSITORY,
    MERGE_WINDOW_START,
    MiningConfig,
    PullRequest,
    Rejection,
    Repository,
    Stage,
    check_base,
    check_pull_request,
    check_repository,
    check_summary,
    seeded_key,
)

MINING_SCHEMA = "specbench.mining/1"


@dataclass(frozen=True)
class MiningResult:
    config: MiningConfig
    candidates: tuple[Candidate, ...]
    attrition: tuple[AttritionEntry, ...]
    sample: Sample
    unexamined_repositories: Mapping[str, int]

    def manifest_record(self) -> dict[str, JsonValue]:
        return {
            "attrition": dict(attrition_counts(self.attrition)),
            "candidates": len(self.candidates),
            "freeze_date": None if self.config.freeze_date is None else self.config.freeze_date.isoformat(),
            "languages": [language.value for language in self.config.languages],
            "reserves": [{"task_id": task_id_for(c), "repository": c.repository} for c in self.sample.reserves],
            "sample": [{"task_id": task_id_for(c), "repository": c.repository} for c in self.sample.tasks],
            "schema": MINING_SCHEMA,
            "seed": self.config.seed,
            "unexamined_repositories": dict(sorted(self.unexamined_repositories.items())),
        }


def _evaluate(
    source: MiningSource, repository: Repository, pull: PullRequest, config: MiningConfig
) -> Candidate | Rejection:
    rejection = check_pull_request(repository.language, pull, config)
    if rejection is not None:
        return rejection
    base = source.base_snapshot(repository, pull.base_sha)
    return check_base(repository, base) or build_candidate(repository, pull, base)


def _mine_repository(
    source: MiningSource, repository: Repository, config: MiningConfig
) -> tuple[list[Candidate], list[AttritionEntry]]:
    accepted: list[Candidate] = []
    attrition: list[AttritionEntry] = []
    summaries = sorted(
        source.merged_pull_summaries(repository, MERGE_WINDOW_START),
        key=lambda s: seeded_key(config.seed, f"{repository.full_name}#{s.number}"),
    )
    for summary in summaries:
        label = f"{repository.full_name}#{summary.number}"
        if len(accepted) >= MAX_TASKS_PER_REPOSITORY:
            attrition.append(
                AttritionEntry(label, Stage.REPOSITORY_CAP, f"{MAX_TASKS_PER_REPOSITORY} task already taken")
            )
            continue
        rejection = check_summary(summary)
        if rejection is not None:
            attrition.append(AttritionEntry(label, rejection.stage, rejection.reason))
            continue
        pull = source.pull_request(repository, summary.number)
        if pull is None:
            attrition.append(AttritionEntry(label, Stage.FEATURE_SIGNAL, "pull request not readable"))
            continue
        outcome = _evaluate(source, repository, pull, config)
        if isinstance(outcome, Rejection):
            attrition.append(AttritionEntry(label, outcome.stage, outcome.reason))
        else:
            accepted.append(outcome)
    return accepted, attrition


def mine(source: MiningSource, config: MiningConfig) -> MiningResult:
    """Walk each language's repositories in seeded order until enough distinct repositories yield a candidate."""
    candidates: list[Candidate] = []
    attrition: list[AttritionEntry] = []
    unexamined: dict[str, int] = {}
    for language in config.languages:
        pool = {r.full_name: r for r in source.repositories(language)}
        ordered = sorted(pool.values(), key=lambda r: seeded_key(config.seed, r.full_name))
        productive = 0
        for position, repository in enumerate(ordered):
            if productive >= config.candidates_per_language:
                unexamined[language.value] = len(ordered) - position
                break
            rejection = check_repository(repository, config)
            if rejection is not None:
                attrition.append(AttritionEntry(repository.full_name, rejection.stage, rejection.reason))
                continue
            found, dropped = _mine_repository(source, repository, config)
            attrition.extend(dropped)
            candidates.extend(found)
            productive += 1 if found else 0
    sample = select_sample(config.seed, candidates)
    return MiningResult(
        config=config,
        candidates=tuple(candidates),
        attrition=tuple(attrition),
        sample=sample,
        unexamined_repositories=unexamined,
    )
