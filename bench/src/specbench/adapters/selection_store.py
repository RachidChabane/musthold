"""What a selection leaves in the repository: the funnel, one `task.json` per task and the upstream licence notices.

The upstream `LICENSE` and `NOTICE` of each task travel beside its `task.json`, as ADR 0004 requires of every
task that will later carry upstream test code, even sealed.
"""

from __future__ import annotations

import json
from collections.abc import Callable, Iterable
from pathlib import Path
from typing import Final

from specbench.adapters.offline_probe import read_verdict, verdict_path
from specbench.domain.candidates import AttritionEntry, Candidate, attrition_counts, task_id_for
from specbench.domain.canonical import JsonValue, canonical_json
from specbench.domain.jsonread import RecordError, enum_of, mapping, sequence, text
from specbench.domain.mining import Rejection, Stage
from specbench.domain.selection import ProbedTask, Selection
from specbench.domain.task import TASK_FILE_NAME

SELECTION_FILE: Final = "selection.json"
SELECTION_ATTRITION_FILE: Final = "attrition.jsonl"
TASKS_DIRECTORY: Final = "tasks"
SELECTION_SCHEMA: Final = "specbench.selection/1"
ENCODING: Final = "utf-8"

LicenceReader = Callable[[ProbedTask, str], str | None]


def read_candidates(path: Path) -> list[Candidate]:
    return [
        Candidate.from_record(mapping(json.loads(line), str(path)))
        for line in path.read_text(encoding=ENCODING).splitlines()
        if line.strip()
    ]


def read_attrition(path: Path) -> list[AttritionEntry]:
    entries: list[AttritionEntry] = []
    for line in path.read_text(encoding=ENCODING).splitlines():
        if line.strip():
            record = mapping(json.loads(line), str(path))
            entries.append(
                AttritionEntry(
                    text(record.get("subject"), "subject"),
                    enum_of(Stage, record.get("stage"), "stage"),
                    text(record.get("reason"), "reason"),
                )
            )
    return entries


def selection_record(
    seed: int, selection: Selection, mining_attrition: Iterable[AttritionEntry]
) -> dict[str, JsonValue]:
    funnel = attrition_counts([*mining_attrition, *selection.attrition])
    return {
        "funnel": dict(funnel),
        "reserves": [t.to_record() for t in selection.reserves],
        "schema": SELECTION_SCHEMA,
        "seed": seed,
        "tasks": [t.to_record() for t in selection.tasks],
    }


def write_selection(
    directory: Path,
    seed: int,
    selection: Selection,
    mining_attrition: Iterable[AttritionEntry],
    licences: LicenceReader,
) -> None:
    directory.mkdir(parents=True, exist_ok=True)
    (directory / SELECTION_FILE).write_text(
        canonical_json(selection_record(seed, selection, mining_attrition)), encoding=ENCODING
    )
    (directory / SELECTION_ATTRITION_FILE).write_text(
        "".join(canonical_json(a.to_record()) for a in selection.attrition), encoding=ENCODING
    )
    for task in (*selection.tasks, *selection.reserves):
        _write_task(directory / TASKS_DIRECTORY / task.task_id, task, licences)


def _write_task(directory: Path, task: ProbedTask, licences: LicenceReader) -> None:
    directory.mkdir(parents=True, exist_ok=True)
    (directory / TASK_FILE_NAME).write_text(task.to_task().to_canonical_json(), encoding=ENCODING)
    names = [task.candidate.licence_path]
    if task.candidate.notice_path is not None:
        names.append(task.candidate.notice_path)
    for name in names:
        content = licences(task, name)
        if content is not None:
            (directory / Path(name).name).write_text(content, encoding=ENCODING)


def read_selection(selection_dir: Path, candidates: list[Candidate], work: Path) -> Selection:
    """The tasks and reserves of a written selection, rebuilt from the candidates and the probe verdicts."""
    record = mapping(json.loads((selection_dir / SELECTION_FILE).read_text(encoding=ENCODING)), "selection")
    by_id = {task_id_for(c): c for c in candidates}

    def probed(entries: object) -> tuple[ProbedTask, ...]:
        found: list[ProbedTask] = []
        for entry in sequence(entries, "tasks"):
            candidate = by_id[text(mapping(entry, "task").get("task_id"), "task_id")]
            outcome = read_verdict(candidate, json.loads(verdict_path(work, candidate).read_text(encoding=ENCODING)))
            if isinstance(outcome, Rejection):
                raise RecordError(f"{candidate.label} was selected but its verdict is a rejection")
            found.append(outcome)
        return tuple(found)

    return Selection(probed(record.get("tasks")), probed(record.get("reserves")), ())
