from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.adapters.file_corpus import FileCorpus, read_plain_task
from specbench.adapters.sealed_store import (
    AgeSealStore,
    InsideRepositoryError,
    generate_identity,
    read_identity_file,
    write_identity_file,
)
from specbench.application.corpus import seal_task, unseal_task, verify_corpus
from specbench.domain.canonical import sha256_hex
from specbench.domain.gaps import GAP_FILE_NAME
from specbench.domain.languages import Language
from specbench.domain.sealing import BUNDLE_VERSION_PATH, CURRENT_BUNDLE_VERSION, SealedBundle, SealingError
from support.builders import TASK_ID, make_bundle, task_json

pytestmark = pytest.mark.integration


@pytest.fixture
def corpus(tmp_path: Path) -> tuple[FileCorpus, str]:
    identity, recipient = generate_identity()
    root = tmp_path / "corpus"
    root.mkdir()
    (root / "recipients.txt").write_text(f"# benchmark recipients\n{recipient}\n")
    return FileCorpus(root), identity


def test_seal_then_unseal_returns_the_same_files_and_checks_digests(corpus: tuple[FileCorpus, str]) -> None:
    repository, identity = corpus
    bundle = make_bundle()
    seal_task(repository, AgeSealStore(), task_json(), bundle)
    unsealed = unseal_task(repository, AgeSealStore(), identity, TASK_ID)
    assert unsealed.bundle.files == bundle.files
    assert unsealed.task.task_id == TASK_ID
    assert verify_corpus(repository, AgeSealStore(), identity) == []
    assert verify_corpus(repository, AgeSealStore(), None) == []


def test_ciphertext_holds_no_plaintext(corpus: tuple[FileCorpus, str]) -> None:
    repository, _ = corpus
    seal_task(repository, AgeSealStore(), task_json(), make_bundle())
    ciphertext = repository.read_sealed(TASK_ID)
    assert b"parking fee" not in ciphertext and b"test_u1" not in ciphertext


def test_a_wrong_identity_cannot_unseal(corpus: tuple[FileCorpus, str]) -> None:
    repository, _ = corpus
    seal_task(repository, AgeSealStore(), task_json(), make_bundle())
    stranger, _ = generate_identity()
    with pytest.raises(SealingError):
        unseal_task(repository, AgeSealStore(), stranger, TASK_ID)


def test_a_tampered_task_json_or_lock_is_reported(corpus: tuple[FileCorpus, str]) -> None:
    repository, identity = corpus
    seal_task(repository, AgeSealStore(), task_json(), make_bundle())
    tampered = repository.read_task_json(TASK_ID).replace("acme/parkfee", "acme/other")
    repository.write_task(Language.PYTHON, TASK_ID, tampered, repository.read_sealed(TASK_ID))
    problems = verify_corpus(repository, AgeSealStore(), identity)
    assert any("task.json digest differs" in p for p in problems)
    with pytest.raises(SealingError):
        unseal_task(repository, AgeSealStore(), identity, TASK_ID)


def test_a_bundle_that_differs_from_the_lock_is_refused(corpus: tuple[FileCorpus, str]) -> None:
    repository, identity = corpus
    store = AgeSealStore()
    seal_task(repository, store, task_json(), make_bundle())
    altered = SealedBundle({**make_bundle().files, "SPEC.md": b"another spec"})
    ciphertext = store.seal(altered, repository.read_recipients())
    repository.write_task(Language.PYTHON, TASK_ID, task_json(), ciphertext)
    problems = verify_corpus(repository, store, identity)
    assert any("SPEC.md digest differs" in p for p in problems)


def test_sealing_without_a_recipient_is_refused(tmp_path: Path) -> None:
    (tmp_path / "recipients.txt").write_text("# none\n")
    with pytest.raises(SealingError):
        seal_task(FileCorpus(tmp_path), AgeSealStore(), task_json(), make_bundle())


def test_identity_files_stay_outside_the_repository(tmp_path: Path) -> None:
    repository_root = tmp_path / "repo"
    repository_root.mkdir()
    identity, _ = generate_identity()
    with pytest.raises(InsideRepositoryError):
        write_identity_file(repository_root / "key.txt", identity, repository_root)
    outside = tmp_path / "keys" / "age.key"
    write_identity_file(outside, identity, repository_root)
    assert outside.stat().st_mode & 0o777 == 0o600
    assert read_identity_file(outside, repository_root) == identity
    (repository_root / "copy.key").write_text(identity)
    with pytest.raises(InsideRepositoryError):
        read_identity_file(repository_root / "copy.key", repository_root)


def test_plain_task_directory_is_read_as_task_json_plus_bundle(tmp_path: Path) -> None:
    for path, content in make_bundle().files.items():
        target = tmp_path / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
    (tmp_path / "task.json").write_text(task_json())
    text, files = read_plain_task(tmp_path)
    assert text == task_json()
    assert files == make_bundle().files


def test_sealing_refuses_a_version_one_bundle_and_writes_nothing(corpus: tuple[FileCorpus, str]) -> None:
    repository, _ = corpus
    legacy = make_bundle(version=1)
    assert legacy.schema_version == 1
    with pytest.raises(SealingError, match="below 2"):
        AgeSealStore().seal(legacy, repository.read_recipients())
    with pytest.raises(SealingError, match="below 2"):
        seal_task(repository, AgeSealStore(), task_json(), legacy)
    assert repository.task_ids() == [] and repository.read_lock().entries == ()


def test_a_version_two_bundle_round_trips_through_age_with_its_spec_gap_data(corpus: tuple[FileCorpus, str]) -> None:
    repository, identity = corpus
    bundle = make_bundle()
    assert bundle.schema_version == CURRENT_BUNDLE_VERSION
    entry = seal_task(repository, AgeSealStore(), task_json(), bundle)
    assert entry.files[BUNDLE_VERSION_PATH] == sha256_hex(bundle.files[BUNDLE_VERSION_PATH])
    unsealed = unseal_task(repository, AgeSealStore(), identity, TASK_ID)
    assert unsealed.bundle.schema_version == CURRENT_BUNDLE_VERSION
    assert unsealed.bundle.files == bundle.files
    assert json.loads(unsealed.bundle.files[GAP_FILE_NAME])["criteria"] == [f"{TASK_ID}-1", f"{TASK_ID}-2"]
    assert verify_corpus(repository, AgeSealStore(), identity) == []


def test_a_bundle_whose_version_record_was_changed_after_sealing_is_reported(corpus: tuple[FileCorpus, str]) -> None:
    repository, identity = corpus
    store = AgeSealStore()
    seal_task(repository, store, task_json(), make_bundle())
    newer = make_bundle(version=CURRENT_BUNDLE_VERSION + 1)
    repository.write_task(Language.PYTHON, TASK_ID, task_json(), store.seal(newer, repository.read_recipients()))
    assert any(f"{BUNDLE_VERSION_PATH} digest differs" in p for p in verify_corpus(repository, store, identity))
