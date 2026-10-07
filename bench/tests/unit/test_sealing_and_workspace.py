from __future__ import annotations

import pytest

from specbench.domain.canonical import sha256_hex
from specbench.domain.languages import Language
from specbench.domain.oracle import Part, oracle_from_bundle
from specbench.domain.sealing import (
    EMPTY_LOCK,
    CorpusLock,
    SealedBundle,
    SealingError,
    lock_entry_for,
    pack_bundle,
    parse_lock,
    sealed_leaks,
    unpack_bundle,
    validate_bundle_path,
    verify_bundle,
    verify_task_json,
)
from specbench.domain.workspace import WorkspaceLeak, build_workspace, protected_files
from support.builders import TASK_ID, make_bundle, make_task, task_json


def test_pack_is_deterministic_and_round_trips() -> None:
    bundle = make_bundle()
    assert pack_bundle(bundle) == pack_bundle(SealedBundle(dict(reversed(list(bundle.files.items())))))
    assert unpack_bundle(pack_bundle(bundle)).files == bundle.files


@pytest.mark.parametrize("path", ["", "/abs", "../up", "a/../b", "a//b", "a\\b", "./a"])
def test_unsafe_bundle_paths_are_refused(path: str) -> None:
    with pytest.raises(SealingError):
        validate_bundle_path(path)


def test_lock_entry_records_plaintext_digests_and_detects_tampering() -> None:
    bundle = make_bundle()
    entry = lock_entry_for(TASK_ID, Language.PYTHON, task_json(), bundle)
    assert entry.files["SPEC.md"] == sha256_hex(bundle.files["SPEC.md"])
    assert verify_bundle(entry, bundle) == []
    changed = SealedBundle({**bundle.files, "SPEC.md": b"changed"})
    assert any("SPEC.md digest differs" in p for p in verify_bundle(entry, changed))
    extra = SealedBundle({**bundle.files, "extra.txt": b"x"})
    assert any("extra.txt is in the bundle but not in the lock" in p for p in verify_bundle(entry, extra))
    missing = SealedBundle({k: v for k, v in bundle.files.items() if k != "SPEC.md"})
    assert any("SPEC.md is locked but absent" in p for p in verify_bundle(entry, missing))
    assert verify_task_json(entry, task_json()) == []
    assert verify_task_json(entry, task_json() + " ")


def test_lock_serializes_canonically_and_round_trips() -> None:
    entry = lock_entry_for(TASK_ID, Language.PYTHON, task_json(), make_bundle())
    other = lock_entry_for("ACME-OTHER-3", Language.GO, task_json(task_id="ACME-OTHER-3"), make_bundle())
    lock = EMPTY_LOCK.with_entry(other).with_entry(entry)
    assert [e.task_id for e in lock.entries] == ["ACME-OTHER-3", TASK_ID]
    assert parse_lock(lock.to_canonical_json()) == lock
    assert lock.with_entry(entry) == lock
    assert CorpusLock(()).digest != lock.digest


def test_lock_refuses_an_inconsistent_bundle_digest() -> None:
    entry = lock_entry_for(TASK_ID, Language.PYTHON, task_json(), make_bundle())
    text = CorpusLock((entry,)).to_canonical_json().replace(entry.bundle_sha256, "0" * 64)
    with pytest.raises(SealingError):
        parse_lock(text)


def test_oracle_is_read_from_the_bundle() -> None:
    oracle = oracle_from_bundle(make_bundle())
    assert [t.part for t in oracle.tests] == [Part.U, Part.R, Part.H, Part.O]
    assert len(oracle.violations) == 4 and len(oracle.planted) == 2
    assert {t.test_id for t in oracle.criterion_tests(f"{TASK_ID}-1")} == {"tests/test_a.py::test_u1"}
    overlay = oracle.overlay_paths(make_bundle(), Part.U, Part.H)
    assert overlay == {"tests/test_a.py": "oracle/U/tests/test_a.py", "tests/test_h.py": "oracle/H/tests/test_h.py"}


def test_workspace_has_the_spec_and_init_output_but_no_sealed_file() -> None:
    bundle = make_bundle()
    base = {"pkg/a.py": b"x = 1\n", "tests/test_h.py": bundle.files["oracle/H/tests/test_h.py"], "README.md": b"hello"}
    workspace = build_workspace(base, bundle.files["SPEC.md"], {"AGENTS.md": b"rules"}, bundle)
    assert workspace.omitted == ("tests/test_h.py",)
    assert set(workspace.files) == {"pkg/a.py", "README.md", "SPEC.md", "AGENTS.md"}
    assert sealed_leaks(workspace.files.items(), protected_files(bundle)) == []


def test_workspace_refuses_init_output_that_quotes_a_sealed_digest() -> None:
    bundle = make_bundle()
    digest = sha256_hex(bundle.files["oracle/U/tests/test_a.py"])
    with pytest.raises(WorkspaceLeak):
        build_workspace({}, b"spec", {"notes.txt": f"see {digest}".encode()}, bundle)


def test_task_helpers_use_one_task() -> None:
    assert make_task().task_id == TASK_ID
