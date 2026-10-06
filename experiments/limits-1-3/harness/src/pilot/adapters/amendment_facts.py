"""Gathers the facts of the amendment check from git, openssl and the experiment's files (ADR 0006)."""

from __future__ import annotations

import json
import re
from collections.abc import Mapping
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Final, Protocol

from pilot.adapters.git_drift import GIT, PreregistrationTagMissing, preregistration_commit
from pilot.adapters.process import ProcessRunner
from pilot.adapters.run_files import file_digest, file_digests, sealed_sums
from pilot.domain.amendment import (
    AMENDED_COMMIT_FILE,
    AMENDED_TOKEN_FILE,
    AMENDMENT_FILENAME,
    SUPERSEDED_COMMIT_FILE,
    SUPERSEDED_TOKEN_FILE,
    AmendmentFacts,
    Timestamp,
    sha256_hex,
)
from pilot.domain.constants import PREREG_DIRNAME, RUNS_DIRNAME
from pilot.domain.import_manifest import IMPORTS_FILENAME, SOURCE_RUN_ID, SHA256SUMS_FILENAME
from pilot.domain.pipeline_constants import TIMING_FILENAME
from pilot.domain.preregistration import PREREG_TAG, SUPERSEDED_PREREG_TAG

ENCODING = "utf-8"
OPENSSL: Final = "openssl"
HASH_LINE: Final = re.compile(r"^\s*[0-9a-f]{4} - (?P<bytes>.{47})")
HEX_BYTE: Final = re.compile(r"[0-9a-f]{2}")
TIME_LINE: Final = re.compile(r"^Time stamp:\s+(?P<month>[A-Za-z]{3})\s+(?P<day>\d+)\s+(?P<time>[\d:]+)\s+(?P<year>\d{4})\s+GMT\s*$")
MESSAGE_DATA: Final = "Message data:"
MONTHS: Final = ("Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec")


class TimestampReader(Protocol):
    def read(self, reply: Path) -> Timestamp | None: ...


def parse_reply(text: str) -> Timestamp:
    """The message imprint and genTime of an `openssl ts -reply -text` listing."""
    imprint: list[str] = []
    in_data = False
    generated: datetime | None = None
    for line in text.splitlines():
        if line.strip() == MESSAGE_DATA:
            in_data = True
            continue
        match = HASH_LINE.match(line) if in_data else None
        if match:
            imprint += HEX_BYTE.findall(match.group("bytes"))
            continue
        in_data = False
        time_match = TIME_LINE.match(line)
        if time_match:
            hours, minutes, seconds = (int(part) for part in time_match.group("time").split(":"))
            generated = datetime(
                int(time_match.group("year")), MONTHS.index(time_match.group("month")) + 1,
                int(time_match.group("day")), hours, minutes, seconds, tzinfo=UTC,
            )
    if not imprint or generated is None:
        raise ValueError("no message imprint or time stamp in the reply")
    return Timestamp("".join(imprint), generated)


class OpensslTimestampReader:
    """Implements `TimestampReader`: the imprint and the genTime, not the certificate chain."""

    def __init__(self, process: ProcessRunner, cwd: Path) -> None:
        self._process = process
        self._cwd = cwd

    def read(self, reply: Path) -> Timestamp | None:
        if not reply.is_file():
            return None
        result = self._process.run([OPENSSL, "ts", "-reply", "-in", str(reply), "-text"], self._cwd)
        if result.returncode != 0:
            return None
        try:
            return parse_reply(result.stdout)
        except ValueError:
            return None


def _tag_commit(root: Path, tag: str, process: ProcessRunner) -> str | None:
    try:
        return preregistration_commit(root, tag, process)
    except PreregistrationTagMissing:
        return None


def _changed_paths(root: Path, process: ProcessRunner, old: str | None, new: str | None) -> tuple[str, ...] | None:
    if old is None or new is None:
        return None
    result = process.run([GIT, "diff", "--name-only", "--relative", SUPERSEDED_PREREG_TAG, PREREG_TAG, "--", "."], root)
    return tuple(line for line in result.stdout.splitlines() if line.strip())


def _read_text(path: Path) -> str | None:
    return path.read_text(encoding=ENCODING) if path.is_file() else None


def _earliest_started(timing_path: Path) -> datetime | None:
    if not timing_path.is_file():
        return None
    timing = json.loads(timing_path.read_text(encoding=ENCODING))
    starts = [datetime.fromisoformat(entry["started"]) for entry in timing.values() if "started" in entry]
    return min(starts) if starts else None


def collect_facts(
    root: Path, run_id: str, process: ProcessRunner, timestamps: TimestampReader, source_run_id: str = SOURCE_RUN_ID
) -> AmendmentFacts:
    superseded = _tag_commit(root, SUPERSEDED_PREREG_TAG, process)
    amended = _tag_commit(root, PREREG_TAG, process)
    run_dir = root / RUNS_DIRNAME / run_id
    amendment_text = _read_text(root / PREREG_DIRNAME / AMENDMENT_FILENAME)
    imports_text = _read_text(run_dir / IMPORTS_FILENAME)
    imports = json.loads(imports_text) if imports_text is not None else None
    commit_paths = (SUPERSEDED_COMMIT_FILE, AMENDED_COMMIT_FILE)
    recorded = {path: _read_text(root / path) for path in commit_paths}
    return AmendmentFacts(
        superseded_tag=SUPERSEDED_PREREG_TAG,
        amended_tag=PREREG_TAG,
        superseded_commit=superseded,
        amended_commit=amended,
        changed_paths=_changed_paths(root, process, superseded, amended),
        amendment=json.loads(amendment_text) if amendment_text is not None else None,
        recorded_commits=recorded,
        commit_file_digests={path: sha256_hex((root / path).read_bytes()) if (root / path).is_file() else None for path in commit_paths},
        timestamps={token: timestamps.read(root / token) for token in (SUPERSEDED_TOKEN_FILE, AMENDED_TOKEN_FILE)},
        earliest_started=_earliest_started(run_dir / TIMING_FILENAME),
        source_listing=file_digests(root / RUNS_DIRNAME / source_run_id, exclude=(SHA256SUMS_FILENAME,)),
        source_sums=sealed_sums(root / RUNS_DIRNAME / source_run_id),
        target_listing=_target_listing(run_dir, imports),
        imports=imports,
    )


def _target_listing(run_dir: Path, imports: Mapping[str, Any] | None) -> dict[str, str]:
    """Digests of the files the run's `imports.json` says it imported, where they exist."""
    if imports is None:
        return {}
    paths = [entry["target_path"] for entry in imports["entries"]]
    return {path: file_digest(run_dir / path) for path in paths if (run_dir / path).is_file()}
