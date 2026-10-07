"""The amendment check: what must hold before `analyze` reads a run (ADR 0006, "Freeze, tag, timestamp, drift").

The facts are gathered by an adapter (git, openssl, files); the verdict is pure, so every
refusal is testable without a repository or a timestamp token.
"""

from __future__ import annotations

import fnmatch
import hashlib
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Final

from pilot.domain.import_manifest import seal_problems

AMENDMENT_FILENAME: Final = "amendment.json"
RECOMPUTED_FILES: Final = ("bindings.json", "reference.json")
TRACEABILITY_FILE: Final = "traceability.json"
IMPORT_DIGESTS_PLACEHOLDER: Final = "to be filled at the run-01 seal"
SUPERSEDED_COMMIT_FILE: Final = "freeze/prereg-commit.txt"
AMENDED_COMMIT_FILE: Final = "freeze/a1-commit.txt"
SUPERSEDED_TOKEN_FILE: Final = "freeze/prereg-commit.tsr"
AMENDED_TOKEN_FILE: Final = "freeze/a1-commit.tsr"
DIRECTORY_SUFFIX: Final = "/"
GLOB_CHARACTERS: Final = ("*", "?", "[")


@dataclass(frozen=True)
class Timestamp:
    """A parsed RFC 3161 reply: the message imprint (hex) and the time the authority stamped."""

    imprint: str
    generated_at: datetime


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def is_declared(path: str, declared: Sequence[str]) -> bool:
    """A declared entry is an exact path, a directory prefix ending in `/`, or a glob."""
    for entry in declared:
        if path == entry:
            return True
        if entry.endswith(DIRECTORY_SUFFIX) and path.startswith(entry):
            return True
        if any(char in entry for char in GLOB_CHARACTERS) and fnmatch.fnmatchcase(path, entry):
            return True
    return False


def undeclared_paths(changed: Sequence[str], declared: Sequence[str]) -> list[str]:
    return sorted(path for path in changed if not is_declared(path, declared))


@dataclass(frozen=True)
class AmendmentFacts:
    superseded_tag: str
    amended_tag: str
    superseded_commit: str | None
    amended_commit: str | None
    changed_paths: tuple[str, ...] | None
    amendment: Mapping[str, Any] | None
    recorded_commits: Mapping[str, str | None]
    commit_file_digests: Mapping[str, str | None]
    timestamps: Mapping[str, Timestamp | None]
    earliest_started: datetime | None
    source_listing: Mapping[str, str]
    source_sums: Mapping[str, str] | None
    target_listing: Mapping[str, str]
    imports: Mapping[str, Any] | None
    target_file_digests: Mapping[str, str | None]
    traceability: tuple[Mapping[str, Any] | None, Mapping[str, Any] | None]
    """The canonical traceability of the sealed run and of the decisional run."""


def evaluate_amendment(facts: AmendmentFacts) -> list[str]:
    """Every reason the amendment cannot be relied on; empty when all checks hold."""
    problems = _tag_problems(facts)
    problems += _declared_path_problems(facts)
    problems += _commit_file_problems(facts)
    problems += _timestamp_problems(facts)
    problems += _run_seal_problems(facts)
    problems += _import_problems(facts)
    problems += _recomputation_problems(facts)
    return problems


def canonical_traceability(report: Mapping[str, Any]) -> dict[str, Any]:
    """The part of a Shallnot report that does not depend on where it ran: verdict, summary, findings, links."""
    return {
        "verdict": report["verdict"],
        "summary": report["summary"],
        "findings": sorted(str(finding["category"]) for finding in report["findings"]),
        "links": sorted((r["ref"], t["name"]) for r in report["requirements"] for t in r["tests"]),
    }


def _recomputation_problems(facts: AmendmentFacts) -> list[str]:
    """Traceability, bindings and reference are recomputed in the decisional run and must equal the sealed run's."""
    if facts.source_sums is None:
        return []
    problems = []
    for name in RECOMPUTED_FILES:
        ours, theirs = facts.target_file_digests.get(name), facts.source_sums.get(name)
        if ours is None:
            problems.append(f"the decisional run has no {name} to compare with the sealed run")
        elif theirs is None:
            problems.append(f"the sealed run records no digest of {name}")
        elif ours != theirs:
            problems.append(f"{name} of the decisional run differs from the sealed run's")
    sealed, current = facts.traceability
    if current is None or sealed is None:
        problems.append(f"{TRACEABILITY_FILE} is missing in the decisional or the sealed run")
    elif current != sealed:
        problems.append(f"{TRACEABILITY_FILE} (verdict, summary, findings, links) differs from the sealed run's")
    return problems


def _tag_problems(facts: AmendmentFacts) -> list[str]:
    missing = [tag for tag, commit in ((facts.superseded_tag, facts.superseded_commit), (facts.amended_tag, facts.amended_commit)) if commit is None]
    return [f"tag {tag} does not exist" for tag in missing]


def _declared_path_problems(facts: AmendmentFacts) -> list[str]:
    if facts.amendment is None:
        return [f"{AMENDMENT_FILENAME} is missing"]
    problems = []
    if facts.amendment.get("superseded_tag") != facts.superseded_tag:
        problems.append(f"{AMENDMENT_FILENAME} names {facts.amendment.get('superseded_tag')} as the superseded tag")
    if facts.superseded_commit is not None and facts.amendment.get("superseded_commit") != facts.superseded_commit:
        problems.append(f"{AMENDMENT_FILENAME} records superseded commit {facts.amendment.get('superseded_commit')}, the tag peels to {facts.superseded_commit}")
    if not isinstance(facts.amendment.get("import_digests"), Mapping):
        problems.append(f"{AMENDMENT_FILENAME} holds no import digests")
    if facts.changed_paths is None and facts.superseded_commit and facts.amended_commit:
        problems.append("git could not list the paths changed between the tags")
    if facts.changed_paths is not None:
        declared = list(facts.amendment.get("declared_paths", []))
        problems += [f"changed between the tags without being declared: {path}" for path in undeclared_paths(facts.changed_paths, declared)]
    return problems


def _commit_file_problems(facts: AmendmentFacts) -> list[str]:
    expected = {SUPERSEDED_COMMIT_FILE: facts.superseded_commit, AMENDED_COMMIT_FILE: facts.amended_commit}
    problems = []
    for path, commit in expected.items():
        recorded = facts.recorded_commits.get(path)
        if recorded is None:
            problems.append(f"{path} is missing")
        elif commit is not None and recorded.strip() != commit:
            problems.append(f"{path} holds {recorded.strip()}, the tag peels to {commit}")
    return problems


def _timestamp_problems(facts: AmendmentFacts) -> list[str]:
    problems = []
    for token_path, commit_path in ((SUPERSEDED_TOKEN_FILE, SUPERSEDED_COMMIT_FILE), (AMENDED_TOKEN_FILE, AMENDED_COMMIT_FILE)):
        stamp, digest = facts.timestamps.get(token_path), facts.commit_file_digests.get(commit_path)
        if stamp is None:
            problems.append(f"{token_path} is missing or unreadable")
        elif digest is not None and stamp.imprint != digest:
            problems.append(f"{token_path} stamps {stamp.imprint}, {commit_path} hashes to {digest}")
    amended = facts.timestamps.get(AMENDED_TOKEN_FILE)
    if amended is not None and facts.earliest_started is not None and not amended.generated_at < facts.earliest_started:
        problems.append(
            f"{AMENDED_TOKEN_FILE} was stamped at {amended.generated_at.isoformat()}, not before the first run stage at "
            f"{facts.earliest_started.isoformat()}"
        )
    if facts.earliest_started is None:
        problems.append("the decisional run records no stage start")
    return problems


def _run_seal_problems(facts: AmendmentFacts) -> list[str]:
    if facts.source_sums is None:
        return ["the sealed run has no SHA256SUMS"]
    return [f"sealed run: {problem}" for problem in seal_problems(facts.source_listing, facts.source_sums)]


def _import_problems(facts: AmendmentFacts) -> list[str]:
    if facts.imports is None:
        return ["the decisional run has no imports.json"]
    problems = []
    for entry in facts.imports["entries"]:
        actual = facts.target_listing.get(entry["target_path"])
        if actual != entry["digest"]:
            problems.append(f"imported file {entry['target_path']} {'is missing' if actual is None else 'differs from its digest'}")
    recorded = facts.imports.get("entries", [])
    digests = {entry["source_path"]: entry["digest"] for entry in recorded}
    if facts.amendment is not None and isinstance(facts.amendment.get("import_digests"), Mapping):
        if dict(facts.amendment["import_digests"]) != digests:
            problems.append(f"{AMENDMENT_FILENAME} import digests differ from imports.json")
    return problems
