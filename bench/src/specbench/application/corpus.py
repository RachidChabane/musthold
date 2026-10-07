"""Sealing, unsealing and verifying the corpus: ciphertext at rest, plaintext digests in `corpus.lock.json`."""

from __future__ import annotations

from dataclasses import dataclass

from specbench.application.ports import CorpusRepository, SealStore
from specbench.domain.sealing import (
    LockEntry,
    SealedBundle,
    SealingError,
    lock_entry_for,
    verify_bundle,
    verify_task_json,
)
from specbench.domain.task import Task, parse_task


@dataclass(frozen=True)
class UnsealedTask:
    task: Task
    bundle: SealedBundle


def seal_task(corpus: CorpusRepository, seals: SealStore, task_json: str, bundle: SealedBundle) -> LockEntry:
    """Seal a task's bundle to the corpus recipients, write it beside its `task.json`, and record its digests in the lock."""
    task = parse_task(task_json)
    if not task.is_constructed:
        raise SealingError(f"task {task.task_id} has no criteria: it is selected but not constructed")
    ciphertext = seals.seal(bundle, corpus.read_recipients())
    corpus.write_task(task.language, task.task_id, task_json, ciphertext)
    entry = lock_entry_for(task.task_id, task.language, task_json, bundle)
    corpus.write_lock(corpus.read_lock().with_entry(entry))
    return entry


def unseal_task(corpus: CorpusRepository, seals: SealStore, identity: str, task_id: str) -> UnsealedTask:
    """Decrypt a task and check every digest against the lock; a mismatch is an error, never a warning."""
    entry = corpus.read_lock().entry(task_id)
    task_json = corpus.read_task_json(task_id)
    bundle = seals.unseal(corpus.read_sealed(task_id), identity)
    problems = [*verify_task_json(entry, task_json), *verify_bundle(entry, bundle)]
    if problems:
        raise SealingError("; ".join(problems))
    return UnsealedTask(parse_task(task_json), bundle)


def verify_corpus(corpus: CorpusRepository, seals: SealStore, identity: str | None) -> list[str]:
    """R0 corpus check. Without an identity only the plaintext `task.json` digests are checked."""
    problems: list[str] = []
    lock = corpus.read_lock()
    on_disk = set(corpus.task_ids())
    locked = {entry.task_id for entry in lock.entries}
    problems += [f"{task_id}: on disk but not in the lock" for task_id in sorted(on_disk - locked)]
    problems += [f"{task_id}: locked but not on disk" for task_id in sorted(locked - on_disk)]
    for task_id in sorted(on_disk & locked):
        entry = lock.entry(task_id)
        problems += verify_task_json(entry, corpus.read_task_json(task_id))
        if identity is not None:
            try:
                problems += verify_bundle(entry, seals.unseal(corpus.read_sealed(task_id), identity))
            except SealingError as error:
                problems.append(f"{task_id}: {error}")
    return problems
