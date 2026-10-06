"""Mutant sources and the kill vectors of the written suites against every evaluated mutant (ADR 0006)."""

from __future__ import annotations

from collections.abc import Iterable, Sequence
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.mutant_records import evaluated_mutant_ids, load_edit_set
from pilot.application.ports import RunRequest, RunStore, SuiteRunners
from pilot.application.process_mutants import kill_vectors
from pilot.application.test_writing_stage import load_suites
from pilot.domain.arms import ARMS
from pilot.domain.constants import PYTEST_EXIT_INTERRUPTED
from pilot.domain.model_slate import MutantSource, Writer
from pilot.domain.outcomes import KillResult
from pilot.domain.pipeline_constants import MUTANT_SOURCES_FILENAME, WRITER_KILLS_FILENAME
from pilot.domain.suites import STATUS_OK, SuiteRecord, suite_files

WRITER_KILL_BATCH = 40
ROW_KEY = ("writer", "mutant_id")


def load_sources(store: RunStore) -> dict[str, list[str]]:
    return dict(store.read_json(MUTANT_SOURCES_FILENAME)) if store.exists(MUTANT_SOURCES_FILENAME) else {}


def record_sources(store: RunStore, source: MutantSource, mutant_ids: Iterable[str]) -> None:
    """Membership is per source: a mutant shared with another source's arm is listed under both."""
    sources = load_sources(store)
    sources[source.value] = sorted({*sources.get(source.value, []), *mutant_ids})
    store.write_json(MUTANT_SOURCES_FILENAME, dict(sorted(sources.items())))


def ensure_writer_kills(ctx: PipelineContext, runners: SuiteRunners, writers: Sequence[Writer]) -> None:
    """Kill vectors of each writer's reference-filtered suites against every evaluated mutant not yet run."""
    evaluated = evaluated_mutant_ids(ctx.store, ARMS)
    for writer in writers:
        suites = [suite for suite in load_suites(ctx.store, writer).values() if suite.status == STATUS_OK]
        if not suites or not all(suite.reference_checked for suite in suites):
            continue
        files = suite_files(suites)
        if not files:
            continue
        done = {row["mutant_id"] for row in _rows(ctx.store) if row["writer"] == writer.value}
        pending = [mutant_id for mutant_id in evaluated if mutant_id not in done]
        for start in range(0, len(pending), WRITER_KILL_BATCH):
            _run_batch(ctx, runners, writer, files, suites, pending[start : start + WRITER_KILL_BATCH])
        ctx.log(f"[writer-kills:{writer.value}] {len(pending)} mutant(s) run, {len(done)} already recorded")


def _rows(store: RunStore) -> list[dict[str, Any]]:
    return store.read_jsonl(WRITER_KILLS_FILENAME) if store.exists(WRITER_KILLS_FILENAME) else []


def _run_batch(
    ctx: PipelineContext,
    runners: SuiteRunners,
    writer: Writer,
    files: dict[str, str],
    suites: Sequence[SuiteRecord],
    mutant_ids: Sequence[str],
) -> None:
    runner = runners.runner(files)
    nodeids = [nodeid for suite in suites for nodeid in suite.nodeids]
    edit_sets = {mutant_id: load_edit_set(ctx.store, mutant_id) for mutant_id in mutant_ids}
    results = runner.run_many({mutant_id: RunRequest(edit_set) for mutant_id, edit_set in edit_sets.items()})
    broken = {mid for mid, result in results.items() if result.returncode == PYTEST_EXIT_INTERRUPTED}
    vectors = kill_vectors(runner, {mid: (edit_sets[mid], results[mid]) for mid in mutant_ids if mid not in broken}, nodeids)
    vectors.update({mid: {nodeid: KillResult.ERROR for nodeid in nodeids} for mid in broken})
    rows = [
        {"writer": writer.value, "mutant_id": mid, "results": {nodeid: result.value for nodeid, result in sorted(vectors[mid].items())}}
        for mid in mutant_ids
    ]
    ctx.store.write_jsonl(WRITER_KILLS_FILENAME, [*_rows(ctx.store), *rows], ROW_KEY)
