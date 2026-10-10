"""Builders of valid domain values for tests; every default is a plausible task of the corpus."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from datetime import date

from specbench.domain.canonical import JsonValue, canonical_json
from specbench.domain.h_selection import H_SELECTION_PATH, H_SELECTION_SCHEMA
from specbench.domain.languages import Language
from specbench.domain.manifest import CODING_AGENT_CLIS, PRODUCT_ARTIFACTS, Provenance
from specbench.domain.sealing import CURRENT_BUNDLE_VERSION, SealedBundle, versioned_bundle
from specbench.domain.task import Criterion, CriterionStatus, Task, Toolchain

TASK_ID = "ACME-PARKFEE-12"
BASE = "1" * 40
MERGE = "2" * 40
DIGEST = "3" * 64
PROVENANCE = Provenance(
    product_digests={name: f"{index:x}" * 64 for index, name in enumerate(PRODUCT_ARTIFACTS, start=1)},
    shallnot_commit="c" * 40,
    cli_versions={cli: f"{cli} 1.0.0" for cli in CODING_AGENT_CLIS},
)


def make_task(**changes: object) -> Task:
    fields: dict[str, object] = {
        "task_id": TASK_ID,
        "language": Language.PYTHON,
        "repository": "acme/parkfee",
        "pull_number": 12,
        "merged_on": date(2026, 8, 3),
        "spdx_id": "MIT",
        "licence_path": "LICENSE",
        "notice_path": None,
        "base_sha": BASE,
        "merge_sha": MERGE,
        "lockfile_digests": {"uv.lock": DIGEST},
        "toolchain": Toolchain("python", "3.13.14"),
        "criteria": (
            Criterion(f"{TASK_ID}-1", CriterionStatus.NEW),
            Criterion(f"{TASK_ID}-2", CriterionStatus.REVISED),
        ),
    }
    return Task(**{**fields, **changes})  # type: ignore[arg-type]


def task_json(**changes: object) -> str:
    return make_task(**changes).to_canonical_json()


H_TEST_ID = "tests/test_h.py::test_h1"
FAKE_CALL_ID = "build-" + "0" * 20


def selection_record_for(kept: Sequence[str], task_id: str = TASK_ID) -> dict[str, JsonValue]:
    """A selection record that kept exactly `kept`, each a test of criterion 2 that passed at merge and failed at base."""
    criterion = f"{task_id}-2"
    return {
        "candidates": [
            {
                "base": ["failed"] * 3,
                "criterion": criterion,
                "file": test_id.split("::")[0],
                "id": test_id,
                "kind": "example",
                "merge": ["passed"] * 3,
                "round": 0,
                "verdict": "kept",
            }
            for test_id in kept
        ],
        "criteria": [
            {"criterion": criterion, "exposed": False, "kept": len(kept), "merge_rejects": 0, "refilled": False}
        ],
        "divergences": [],
        "rounds": [{"calls": [FAKE_CALL_ID], "criteria": [criterion], "round": 0}],
        "schema": H_SELECTION_SCHEMA,
        "seed": 20261007,
        "task_id": task_id,
    }


def oracle_files(task_id: str = TASK_ID) -> dict[str, bytes]:
    """A sealed bundle's files: spec, oracle manifest, U/H/O tests, violations and planted refactors."""
    manifest = [
        {"id": "tests/test_a.py::test_u1", "part": "U", "criterion": f"{task_id}-1"},
        {"id": "tests/test_a.py::test_r1", "part": "R", "criterion": None},
        {"id": H_TEST_ID, "part": "H", "criterion": f"{task_id}-2"},
        {"id": "tests/test_o.py::test_o1", "part": "O", "criterion": None},
    ]
    violations = [
        {"id": "V1", "criterion": f"{task_id}-1", "patch": "violations/V1.patch"},
        {"id": "V2", "criterion": f"{task_id}-1", "patch": "violations/V2.patch"},
        {"id": "V3", "criterion": f"{task_id}-2", "patch": "violations/V3.patch"},
        {"id": "V4", "criterion": f"{task_id}-2", "patch": "violations/V4.patch"},
    ]
    planted = [{"id": "P1", "patch": "planted/P1.patch"}, {"id": "P2", "patch": "planted/P2.patch"}]
    files: dict[str, bytes] = {
        "SPEC.md": b"# Spec\n\nA parking fee is waived under ten minutes.\n",
        "oracle/manifest.json": canonical_json(manifest).encode(),  # type: ignore[arg-type]
        "oracle/U/tests/test_a.py": b"def test_u1():\n    assert True\n",
        "oracle/H/tests/test_h.py": b"def test_h1():\n    assert True\n",
        "oracle/O/tests/test_o.py": b"def test_o1():\n    assert True\n",
        "violations/index.json": canonical_json(violations).encode(),  # type: ignore[arg-type]
        "planted/index.json": canonical_json(planted).encode(),  # type: ignore[arg-type]
        H_SELECTION_PATH: canonical_json(selection_record_for([H_TEST_ID], task_id)).encode(),
        "construction/gaps.json": canonical_json(
            {"seed": 20261007, "criteria": [f"{task_id}-1", f"{task_id}-2"]}
        ).encode(),  # type: ignore[arg-type]
    }
    for patch in ("violations/V1", "violations/V2", "violations/V3", "violations/V4", "planted/P1", "planted/P2"):
        files[f"{patch}.patch"] = f"--- {patch}\n".encode()
    return files


def make_bundle(
    extra: Mapping[str, bytes] | None = None, version: int = CURRENT_BUNDLE_VERSION, without: Sequence[str] = ()
) -> SealedBundle:
    """A bundle of the given schema version; version 1 carries no version record, as bundles built before 2 did."""
    files = {path: content for path, content in {**oracle_files(), **(extra or {})}.items() if path not in without}
    return SealedBundle(files) if version == 1 else versioned_bundle(files, version)
