"""The sealed bundle of a task, its deterministic archive form and the corpus lock of plaintext digests."""

from __future__ import annotations

import io
import posixpath
import tarfile
from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from typing import Final, cast

from specbench.domain.canonical import JsonValue, canonical_bytes, canonical_json, parse_canonical, sha256_hex
from specbench.domain.languages import Language
from specbench.domain.task import DIGEST_PATTERN

LOCK_SCHEMA: Final = "specbench.corpus-lock/1"
LOCK_FILE_NAME: Final = "corpus.lock.json"
ARCHIVE_MODE: Final = 0o644
ARCHIVE_MTIME: Final = 0
MAX_BUNDLE_FILES: Final = 10_000


class SealingError(ValueError):
    """A bundle, archive or lock that is malformed or does not match its digests."""


def validate_bundle_path(path: str) -> str:
    """A bundle path is relative, normalized, without `..` and without a backslash."""
    clean = posixpath.normpath(path)
    if not path or clean != path or path.startswith("/") or ".." in path.split("/") or "\\" in path:
        raise SealingError(f"unsafe bundle path {path!r}")
    return path


@dataclass(frozen=True)
class SealedBundle:
    """Everything of a task that is sealed: the spec, the oracle parts, violations and classifications."""

    files: Mapping[str, bytes]

    def __post_init__(self) -> None:
        if not self.files or len(self.files) > MAX_BUNDLE_FILES:
            raise SealingError("a bundle holds between 1 and 10000 files")
        for path in self.files:
            validate_bundle_path(path)

    @property
    def paths(self) -> frozenset[str]:
        return frozenset(self.files)

    def file_digests(self) -> dict[str, str]:
        return {path: sha256_hex(self.files[path]) for path in sorted(self.files)}

    @property
    def digests(self) -> frozenset[str]:
        return frozenset(self.file_digests().values())

    @property
    def bundle_digest(self) -> str:
        return bundle_digest_of(self.file_digests())


def bundle_digest_of(file_digests: Mapping[str, str]) -> str:
    """The digest of a bundle: SHA-256 of the canonical record of its per-file plaintext digests."""
    return sha256_hex(canonical_bytes(dict(sorted(file_digests.items()))))


def pack_bundle(bundle: SealedBundle) -> bytes:
    """A deterministic tar of the bundle: sorted names, fixed owner, mode and time."""
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w", format=tarfile.USTAR_FORMAT) as archive:
        for path in sorted(bundle.files):
            info = tarfile.TarInfo(path)
            info.size = len(bundle.files[path])
            info.mode = ARCHIVE_MODE
            info.mtime = ARCHIVE_MTIME
            info.uid = info.gid = 0
            info.uname = info.gname = ""
            archive.addfile(info, io.BytesIO(bundle.files[path]))
    return buffer.getvalue()


def unpack_bundle(data: bytes) -> SealedBundle:
    files: dict[str, bytes] = {}
    with tarfile.open(fileobj=io.BytesIO(data), mode="r") as archive:
        for member in archive:
            if not member.isfile():
                raise SealingError(f"archive member {member.name!r} is not a regular file")
            handle = archive.extractfile(member)
            if handle is None:
                raise SealingError(f"archive member {member.name!r} is unreadable")
            files[validate_bundle_path(member.name)] = handle.read()
    return SealedBundle(files)


@dataclass(frozen=True)
class LockEntry:
    task_id: str
    language: Language
    task_json_sha256: str
    bundle_sha256: str
    files: Mapping[str, str]

    def __post_init__(self) -> None:
        for digest in (self.task_json_sha256, self.bundle_sha256, *self.files.values()):
            if DIGEST_PATTERN.match(digest) is None:
                raise SealingError(f"malformed digest in lock entry {self.task_id}")
        if bundle_digest_of(self.files) != self.bundle_sha256:
            raise SealingError(f"bundle digest of {self.task_id} does not match its file digests")

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "bundle_sha256": self.bundle_sha256,
            "files": dict(sorted(self.files.items())),
            "language": self.language.value,
            "task_id": self.task_id,
            "task_json_sha256": self.task_json_sha256,
        }


def lock_entry_for(task_id: str, language: Language, task_json: str, bundle: SealedBundle) -> LockEntry:
    return LockEntry(
        task_id=task_id,
        language=language,
        task_json_sha256=sha256_hex(task_json.encode()),
        bundle_sha256=bundle.bundle_digest,
        files=bundle.file_digests(),
    )


@dataclass(frozen=True)
class CorpusLock:
    entries: tuple[LockEntry, ...]

    def __post_init__(self) -> None:
        ids = [entry.task_id for entry in self.entries]
        if len(set(ids)) != len(ids):
            raise SealingError("lock entries repeat a task id")

    def with_entry(self, entry: LockEntry) -> CorpusLock:
        kept = [e for e in self.entries if e.task_id != entry.task_id]
        return CorpusLock(tuple(sorted([*kept, entry], key=lambda e: e.task_id)))

    def entry(self, task_id: str) -> LockEntry:
        for entry in self.entries:
            if entry.task_id == task_id:
                return entry
        raise SealingError(f"task {task_id} is not in the corpus lock")

    def to_record(self) -> dict[str, JsonValue]:
        return {"schema": LOCK_SCHEMA, "tasks": [e.to_record() for e in sorted(self.entries, key=lambda e: e.task_id)]}

    def to_canonical_json(self) -> str:
        return canonical_json(self.to_record())

    @property
    def digest(self) -> str:
        return sha256_hex(self.to_canonical_json().encode())


EMPTY_LOCK: Final = CorpusLock(())


def parse_lock(text: str) -> CorpusLock:
    record = parse_canonical(text)
    if not isinstance(record, dict) or record.get("schema") != LOCK_SCHEMA:
        raise SealingError(f"lock schema must be {LOCK_SCHEMA}")
    tasks = record.get("tasks")
    if not isinstance(tasks, list):
        raise SealingError("lock tasks must be a list")
    entries: list[LockEntry] = []
    for raw in cast(list[object], tasks):
        if not isinstance(raw, dict):
            raise SealingError("lock entry must be an object")
        entry = cast(dict[str, object], raw)
        files = cast(dict[str, str], entry["files"])
        entries.append(
            LockEntry(
                task_id=cast(str, entry["task_id"]),
                language=Language(entry["language"]),
                task_json_sha256=cast(str, entry["task_json_sha256"]),
                bundle_sha256=cast(str, entry["bundle_sha256"]),
                files=files,
            )
        )
    return CorpusLock(tuple(entries))


def verify_bundle(entry: LockEntry, bundle: SealedBundle) -> list[str]:
    """Problems found comparing an unsealed bundle with its lock entry; empty when every digest matches."""
    problems: list[str] = []
    actual = bundle.file_digests()
    for path in sorted(set(entry.files) | set(actual)):
        if path not in actual:
            problems.append(f"{entry.task_id}: {path} is locked but absent from the bundle")
        elif path not in entry.files:
            problems.append(f"{entry.task_id}: {path} is in the bundle but not in the lock")
        elif actual[path] != entry.files[path]:
            problems.append(f"{entry.task_id}: {path} digest differs from the lock")
    return problems


def verify_task_json(entry: LockEntry, task_json: str) -> list[str]:
    if sha256_hex(task_json.encode()) != entry.task_json_sha256:
        return [f"{entry.task_id}: task.json digest differs from the lock"]
    return []


def sealed_leaks(emitted: Iterable[tuple[str, bytes]], protected: Mapping[str, bytes]) -> list[str]:
    """Emitted files that share a path with a protected file, hold its bytes, or quote its digest."""
    leaks: list[str] = []
    digests = {sha256_hex(content) for content in protected.values()}
    for path, content in emitted:
        if path in protected:
            leaks.append(f"{path}: sealed path")
        elif sha256_hex(content) in digests:
            leaks.append(f"{path}: sealed content")
        elif any(digest.encode() in content for digest in digests):
            leaks.append(f"{path}: quotes a sealed digest")
    return leaks
