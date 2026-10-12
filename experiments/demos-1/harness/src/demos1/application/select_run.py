"""The `select` verb: preconditions of PREREG 3, 5 and 15, the manifest of 11, the selection, and its records."""

from __future__ import annotations

import json
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final, cast

from demos1.application.environment import Builds, EnvironmentProbe, read_builds
from demos1.application.machine_session import machine_running
from demos1.application.ports import Clock, DeviationLog, Files, GitHubApi, Machine, Repository
from demos1.application.search import Requests, Searcher, SearchFailed
from demos1.application.selection import SelectionResult, Selector
from demos1.domain.deviations import Deviation, DeviationKind
from demos1.domain.environment import PREREG_TAG, PRODUCT_PATHS, PRODUCT_TAG, pin_mismatches
from demos1.domain.filters import exclusion_pool
from demos1.domain.jsonvalue import JsonObject, JsonValue, encode_line
from demos1.domain.layout import WorkLayout

CANDIDATES_FILE: Final = "bench/mining/20261007/candidates.jsonl"
CANDIDATES_SHA256: Final = "06d6cfcaeca245047f6b16d2e8c621400561f9c3f239002a465b2e7cf33c4cd7"
FREEZE_TIMESTAMP: Final = "experiments/demos-1/freeze/prereg-commit.tsr"
STAMP_FORMAT: Final = "%Y%m%dT%H%M%SZ"


class Refused(RuntimeError):
    """A precondition of the verb does not hold: nothing was run."""


def candidate_pool(files: Files, repo_root: PurePosixPath) -> frozenset[str]:
    """PREREG 3: the repositories of the benchmark's candidate pool, refused unless its digest matches."""
    path = repo_root / CANDIDATES_FILE
    digest = files.sha256(path)
    if digest != CANDIDATES_SHA256:
        raise Refused(f"{CANDIDATES_FILE} has SHA-256 {digest}, not {CANDIDATES_SHA256}")
    names: list[str] = []
    for line in (files.read_text(path) or "").splitlines():
        if not line.strip():
            continue
        record: object = json.loads(line)
        if isinstance(record, dict):
            repository = cast(dict[str, object], record).get("repository")
            if isinstance(repository, str):
                names.append(repository)
    return exclusion_pool(names)


def write_selection(files: Files, directory: WorkLayout, result: SelectionResult) -> None:
    files.write_text(directory.frame, "".join(encode_line(record) for record in result.frame))
    files.write_text(directory.outside_window, "".join(encode_line(record) for record in result.outside_window))
    files.write_json(directory.lists, result.lists.to_json())


def run_selection(api: GitHubApi, clock: Clock, pool: frozenset[str]) -> SelectionResult:
    requests = Requests(api, clock)
    return Selector(requests, Searcher(requests, clock), pool).run()


@dataclass(frozen=True)
class SelectContext:
    repo_root: PurePosixPath
    work: WorkLayout


class SelectVerb:
    def __init__(
        self,
        files: Files,
        repository: Repository,
        probe: EnvironmentProbe,
        recording_api: Callable[[PurePosixPath], GitHubApi],
        clock: Clock,
        deviations: DeviationLog,
        machine: Machine,
    ) -> None:
        self._machine = machine
        self._files = files
        self._repository = repository
        self._probe = probe
        self._recording_api = recording_api
        self._clock = clock
        self._deviations = deviations

    def run(self, context: SelectContext) -> SelectionResult:
        work = context.work
        pool = candidate_pool(self._files, context.repo_root)
        prereg, product, builds = self._preconditions(context)
        self._set_aside_stopped_run(work)
        started = self._clock.now()
        with machine_running(self._machine, work.mounts()):
            facts = self._probe.facts(work, builds, started.strftime(STAMP_FORMAT))
        for mismatch in pin_mismatches(facts):
            self._deviate(
                DeviationKind.TOOL_VERSION,
                f"PREREG 2 pins {mismatch.fact} at {mismatch.expected}.",
                f"select observed {mismatch.observed}.",
            )
        manifest: dict[str, JsonValue] = {
            "prereg_tag": PREREG_TAG,
            "prereg_commit": prereg,
            "product_tag": PRODUCT_TAG,
            "product_commit": product,
            "product_path_commits": list(self._repository.commits_touching(prereg, product, PRODUCT_PATHS)),
            "tooling_commit": self._repository.head(),
            "product_build": builds.product,
            "image_build": builds.image,
            "facts": dict(facts),
            "selection_date": started.date().isoformat(),
            "selection_started_at": started.isoformat(),
            "selection_ended_at": None,
        }
        self._files.write_json(work.manifest, manifest)
        try:
            result = run_selection(self._recording_api(work.responses), self._clock, pool)
        except SearchFailed as failed:
            self._deviate(
                DeviationKind.SELECT_STOPPED,
                "PREREG 5: a search request that answers non-2xx after its retries stops select.",
                f"{failed}; the stopped run is kept and select runs again from the start.",
            )
            raise
        write_selection(self._files, work, result)
        ended = self._clock.now()
        manifest["selection_ended_at"] = ended.isoformat()
        self._files.write_json(work.manifest, manifest)
        if ended.date() != started.date():
            self._deviate(
                DeviationKind.DEPARTURE,
                "PREREG 5: select runs within one UTC calendar date.",
                f"select started on {started.date()} and ended on {ended.date()}.",
            )
        return result

    def _preconditions(self, context: SelectContext) -> tuple[str, str, Builds]:
        if not self._files.exists(context.repo_root / FREEZE_TIMESTAMP):
            raise Refused(f"the timestamp of PREREG 15 is missing: {FREEZE_TIMESTAMP}")
        prereg = self._repository.commit_of(PREREG_TAG)
        product = self._repository.commit_of(PRODUCT_TAG)
        if prereg is None or product is None:
            raise Refused(f"both tags {PREREG_TAG} and {PRODUCT_TAG} must exist before select runs")
        if not self._repository.is_ancestor(prereg, product):
            raise Refused(f"{PRODUCT_TAG} does not descend from {PREREG_TAG}")
        builds = read_builds(self._files, context.work)
        if builds.product_commit != product:
            raise Refused(f"the product was built from {builds.product_commit}, not from {PRODUCT_TAG} ({product})")
        if self._files.exists(context.work.lists):
            raise Refused(f"select has already run: {context.work.lists} exists")
        return prereg, product, builds

    def _set_aside_stopped_run(self, work: WorkLayout) -> None:
        """A stopped run is kept beside the new one, which starts from the beginning."""
        if not self._files.exists(work.selection):
            return
        index = 1
        while self._files.exists(work.stopped_selection(index)):
            index += 1
        self._files.move(work.selection, work.stopped_selection(index))

    def _deviate(self, kind: DeviationKind, says: str, happened: str) -> None:
        self._deviations.append(Deviation(self._clock.now(), kind, None, says, happened))


def manifest_facts(manifest: JsonObject) -> dict[str, str]:
    facts = manifest.get("facts")
    if not isinstance(facts, dict):
        return {}
    return {str(key): str(value) for key, value in cast(dict[object, object], facts).items()}
