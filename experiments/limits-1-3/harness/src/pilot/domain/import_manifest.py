"""What `import-run-01` may carry from the sealed run-01 into run-02, and the record it leaves (`imports.json`).

The expected set is exact: an import that finds more or less than it refuses, so a
run-01 that is not the sealed run-01 can never enter the decisional run.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Any, Final

from pilot.domain.call_ids import GENERATE_CALL_ID, RELABEL_CALL_ID
from pilot.domain.constants import CANARY_DIRNAME, RAW_DIRNAME

SOURCE_RUN_ID: Final = "run-01"
IMPORTS_FILENAME: Final = "imports.json"
IMPORTS_DIRNAME: Final = "imports"
SHA256SUMS_FILENAME: Final = "SHA256SUMS"
RESULT_FILENAME: Final = "result.json"
PLAN_FILENAME: Final = "plan.json"
IMPORTED_CANARY_DIRECTORY: Final = f"{IMPORTS_DIRNAME}/{SOURCE_RUN_ID}/{CANARY_DIRNAME}"
SUMS_LINE: Final = re.compile(r"^(?P<digest>[0-9a-f]{64})  (?P<path>\S.*)$")


class ImportRole(Enum):
    CONTRAST_GENERATION = "contrast-generation"
    TERRA_GENERATION = "terra-generation"
    RELABEL = "relabel"
    CANARY_BASELINE = "canary-baseline"


def _generation_ids(slots: Mapping[int, tuple[int, ...]]) -> tuple[str, ...]:
    return tuple(
        GENERATE_CALL_ID.format(ref=f"PARK-{number}~1", index=index)
        for number, indices in slots.items()
        for index in indices
    )


OPUS_GENERATION_IDS: Final = _generation_ids({1: (1, 2, 3), 2: (1, 2, 3), 3: (1, 2, 3), 4: (1,)})
TERRA_GENERATION_IDS: Final = _generation_ids({1: (4, 5), 2: (4, 5), 3: (4,)})
RETIRED_CALL_IDS: Final = _generation_ids({3: (5,)})
RELABEL_IDS: Final = tuple(RELABEL_CALL_ID.format(rater=rater) for rater in ("R-AUTHOR", "R-OAI"))
EXPECTED_CALL_ROLES: Final = {
    **{call_id: ImportRole.CONTRAST_GENERATION for call_id in OPUS_GENERATION_IDS},
    **{call_id: ImportRole.TERRA_GENERATION for call_id in TERRA_GENERATION_IDS},
    **{call_id: ImportRole.RELABEL for call_id in RELABEL_IDS},
}
EXPECTED_CANARY_FILES: Final = tuple(
    sorted(
        f"{model}__{call_type}.json"
        for model, call_types in {
            "claude-opus-5-5__claude": ("generate", "relabel", "audit"),
            "claude-sonnet-5-5__claude": ("validity", "alignment", "probe"),
            "gpt-5.6-terra__codex": ("generate", "validity", "alignment", "probe", "relabel", "leak_audit"),
        }.items()
        for call_type in call_types
    )
)


class ImportRefused(Exception):
    """The source or the target is not in the state an import requires."""


@dataclass(frozen=True)
class ImportEntry:
    source_path: str
    target_path: str
    digest: str
    role: ImportRole
    call_id: str | None

    def to_mapping(self) -> dict[str, Any]:
        return {
            "source_path": self.source_path,
            "target_path": self.target_path,
            "digest": self.digest,
            "role": self.role.value,
            "call_id": self.call_id,
        }


@dataclass(frozen=True)
class ImportPlan:
    entries: tuple[ImportEntry, ...]
    retired: tuple[str, ...]
    nonces: Mapping[str, str]

    def to_mapping(self, sums_digest: str) -> dict[str, Any]:
        return {
            "source_run": SOURCE_RUN_ID,
            "sums_digest": sums_digest,
            "entries": [entry.to_mapping() for entry in self.entries],
            "retired": list(self.retired),
            "nonces": dict(sorted(self.nonces.items())),
        }


def parse_sums(text: str) -> dict[str, str]:
    """`sha256sum` lines: digest, two spaces, relative path."""
    sums: dict[str, str] = {}
    for line in text.splitlines():
        if not line.strip():
            continue
        match = SUMS_LINE.match(line)
        if match is None:
            raise ImportRefused(f"{SHA256SUMS_FILENAME}: malformed line {line!r}")
        sums[match.group("path")] = match.group("digest")
    return sums


def render_sums(sums: Mapping[str, str]) -> str:
    return "".join(f"{digest}  {path}\n" for path, digest in sorted(sums.items()))


def plan_import(
    listing: Mapping[str, str], sums: Mapping[str, str], source_plan: Mapping[str, Any]
) -> ImportPlan:
    """The files to carry over, or a refusal naming every way the source differs from the sealed set."""
    problems = _seal_problems(listing, sums)
    completed, attempts_only = _call_directories(listing)
    problems += _set_problems("completed calls", completed, set(EXPECTED_CALL_ROLES))
    problems += _set_problems("attempt-only calls", attempts_only, set(RETIRED_CALL_IDS))
    canary = {path.rsplit("/", 1)[1] for path in listing if path.startswith(f"{CANARY_DIRNAME}/")}
    problems += _set_problems("canary baselines", canary, set(EXPECTED_CANARY_FILES))
    nonces = _nonces(source_plan)
    problems += [f"no nonce recorded for {call_id}" for call_id in sorted(set(OPUS_GENERATION_IDS + TERRA_GENERATION_IDS) - set(nonces))]
    if problems:
        raise ImportRefused("; ".join(problems))
    entries = [_entry(path, listing[path]) for path in sorted(listing) if _entry_role(path) is not None]
    wanted = {call_id: nonces[call_id] for call_id in OPUS_GENERATION_IDS + TERRA_GENERATION_IDS}
    return ImportPlan(tuple(entries), tuple(sorted(attempts_only)), wanted)


def _seal_problems(listing: Mapping[str, str], sums: Mapping[str, str]) -> list[str]:
    problems = [f"modified or missing: {path}" for path in sorted(sums) if listing.get(path) != sums[path]]
    return problems + [f"not covered by {SHA256SUMS_FILENAME}: {path}" for path in sorted(set(listing) - set(sums))]


def _call_directories(listing: Mapping[str, str]) -> tuple[set[str], set[str]]:
    prefix = f"{RAW_DIRNAME}/"
    directories = {
        path.split("/")[1]
        for path in listing
        if path.startswith(prefix) and path.count("/") >= 2 and path.split("/")[1] != CANARY_DIRNAME
    }
    completed = {name for name in directories if f"{prefix}{name}/{RESULT_FILENAME}" in listing}
    return completed, directories - completed


def _set_problems(what: str, found: set[str], expected: set[str]) -> list[str]:
    problems = []
    if found - expected:
        problems.append(f"unexpected {what}: {sorted(found - expected)}")
    if expected - found:
        problems.append(f"missing {what}: {sorted(expected - found)}")
    return problems


def _nonces(source_plan: Mapping[str, Any]) -> dict[str, str]:
    return {entry["call_id"]: entry["nonce"] for entry in source_plan.get("arm_t", [])}


def _entry_role(path: str) -> ImportRole | None:
    parts = path.split("/")
    if parts[0] == RAW_DIRNAME and len(parts) >= 3:
        return EXPECTED_CALL_ROLES.get(parts[1])
    if parts[0] == CANARY_DIRNAME and len(parts) == 2:
        return ImportRole.CANARY_BASELINE
    return None


def _entry(path: str, digest: str) -> ImportEntry:
    role = _entry_role(path)
    assert role is not None
    if role is ImportRole.CANARY_BASELINE:
        return ImportEntry(path, f"{IMPORTED_CANARY_DIRECTORY}/{path.split('/', 1)[1]}", digest, role, None)
    return ImportEntry(path, path, digest, role, path.split("/")[1])
