"""The pure amendment verdict, the declared-path matcher and the openssl reply parser (ADR 0006)."""

from __future__ import annotations

import hashlib
import subprocess
from datetime import UTC, datetime
from pathlib import Path

import pytest

from pilot.adapters.amendment_facts import parse_reply
from pilot.domain.amendment import (
    AMENDED_COMMIT_FILE,
    AMENDED_TOKEN_FILE,
    IMPORT_DIGESTS_PLACEHOLDER,
    SUPERSEDED_COMMIT_FILE,
    SUPERSEDED_TOKEN_FILE,
    AmendmentFacts,
    Timestamp,
    evaluate_amendment,
    is_declared,
    undeclared_paths,
)

OLD, NEW = "a" * 40, "b" * 40
OLD_DIGEST, NEW_DIGEST = hashlib.sha256(OLD.encode()).hexdigest(), hashlib.sha256(NEW.encode()).hexdigest()
SEALED = {"raw/x/result.json": "1" * 64, "bindings.json": "b" * 64, "reference.json": "c" * 64}
CANON = {"verdict": "pass", "summary": {}, "findings": [], "links": [("PARK-1~1", "t")]}


def facts(**changes):
    base = dict(
        superseded_tag="old", amended_tag="new", superseded_commit=OLD, amended_commit=NEW,
        changed_paths=("prereg/PREREG.md", "harness/src/pilot/a.py"),
        amendment={"superseded_tag": "old", "superseded_commit": OLD, "declared_paths": ["prereg/PREREG.md", "harness/"],
                   "import_digests": {"raw/x/result.json": "1" * 64}},
        recorded_commits={SUPERSEDED_COMMIT_FILE: OLD, AMENDED_COMMIT_FILE: NEW},
        commit_file_digests={SUPERSEDED_COMMIT_FILE: OLD_DIGEST, AMENDED_COMMIT_FILE: NEW_DIGEST},
        timestamps={SUPERSEDED_TOKEN_FILE: Timestamp(OLD_DIGEST, datetime(2026, 10, 6, tzinfo=UTC)),
                    AMENDED_TOKEN_FILE: Timestamp(NEW_DIGEST, datetime(2026, 10, 8, tzinfo=UTC))},
        earliest_started=datetime(2026, 10, 9, tzinfo=UTC),
        source_listing=dict(SEALED), source_sums=dict(SEALED), target_listing={"raw/x/result.json": "1" * 64},
        imports={"entries": [{"source_path": "raw/x/result.json", "target_path": "raw/x/result.json", "digest": "1" * 64}]},
        target_file_digests={"bindings.json": "b" * 64, "reference.json": "c" * 64},
        traceability=(CANON, CANON),
    )
    return AmendmentFacts(**{**base, **changes})


def test_a_consistent_amendment_has_no_problem():
    assert evaluate_amendment(facts()) == []


def test_a_missing_tag_is_a_problem():
    assert "tag new does not exist" in evaluate_amendment(facts(amended_commit=None))
    assert "tag old does not exist" in evaluate_amendment(facts(superseded_commit=None))


def test_a_path_changed_between_the_tags_and_not_declared_is_refused():
    problems = evaluate_amendment(facts(changed_paths=("prereg/PREREG.md", "fixture/parkfee/src/parkfee/tariff.py")))
    assert problems == ["changed between the tags without being declared: fixture/parkfee/src/parkfee/tariff.py"]


def test_an_amendment_file_naming_another_superseded_commit_is_refused():
    amendment = {"superseded_tag": "old", "superseded_commit": "c" * 40, "declared_paths": [], "import_digests": {}}
    assert any("superseded commit" in p for p in evaluate_amendment(facts(amendment=amendment, changed_paths=())))


def test_the_placeholder_of_import_digests_is_refused():
    amendment = {"superseded_tag": "old", "superseded_commit": OLD, "declared_paths": ["harness/"], "import_digests": IMPORT_DIGESTS_PLACEHOLDER}
    assert any("no import digests" in p for p in evaluate_amendment(facts(amendment=amendment)))


def test_a_commit_file_that_differs_from_its_peeled_tag_is_refused():
    recorded = {SUPERSEDED_COMMIT_FILE: OLD, AMENDED_COMMIT_FILE: "c" * 40}
    assert any(AMENDED_COMMIT_FILE in p and "peels" in p for p in evaluate_amendment(facts(recorded_commits=recorded)))
    assert any("is missing" in p for p in evaluate_amendment(facts(recorded_commits={SUPERSEDED_COMMIT_FILE: OLD, AMENDED_COMMIT_FILE: None})))


def test_a_token_whose_imprint_is_not_the_hash_of_its_commit_file_is_refused():
    stamps = {SUPERSEDED_TOKEN_FILE: Timestamp("d" * 64, datetime(2026, 10, 6, tzinfo=UTC)),
              AMENDED_TOKEN_FILE: Timestamp(NEW_DIGEST, datetime(2026, 10, 8, tzinfo=UTC))}
    assert any(SUPERSEDED_TOKEN_FILE in p and "stamps" in p for p in evaluate_amendment(facts(timestamps=stamps)))


def test_an_unreadable_token_is_refused():
    stamps = {SUPERSEDED_TOKEN_FILE: None, AMENDED_TOKEN_FILE: Timestamp(NEW_DIGEST, datetime(2026, 10, 8, tzinfo=UTC))}
    assert any("unreadable" in p for p in evaluate_amendment(facts(timestamps=stamps)))


def test_the_amended_token_must_precede_the_first_stage_of_the_run():
    late = datetime(2026, 10, 8, tzinfo=UTC)
    assert any("not before the first run stage" in p for p in evaluate_amendment(facts(earliest_started=late)))
    assert any("not before" in p for p in evaluate_amendment(facts(earliest_started=datetime(2026, 10, 7, tzinfo=UTC))))
    assert any("no stage start" in p for p in evaluate_amendment(facts(earliest_started=None)))


def test_the_sealed_run_must_match_its_sums_and_exist():
    assert any("sealed run" in p for p in evaluate_amendment(facts(source_listing={"raw/x/result.json": "2" * 64})))
    assert any("no SHA256SUMS" in p for p in evaluate_amendment(facts(source_sums=None)))


def test_bindings_reference_and_traceability_of_the_two_runs_must_be_equal():
    assert any("bindings.json of the decisional run differs" in p for p in evaluate_amendment(facts(target_file_digests={"bindings.json": "0" * 64, "reference.json": "c" * 64})))
    assert any("reference.json" in p and "differs" in p for p in evaluate_amendment(facts(target_file_digests={"bindings.json": "b" * 64, "reference.json": "0" * 64})))
    assert any("no bindings.json to compare" in p for p in evaluate_amendment(facts(target_file_digests={"reference.json": "c" * 64})))
    other = {**CANON, "links": [("PARK-2~1", "t")]}
    assert any("traceability.json" in p and "differs" in p for p in evaluate_amendment(facts(traceability=(CANON, other))))
    assert any("traceability.json" in p and "missing" in p for p in evaluate_amendment(facts(traceability=(None, CANON))))


def test_canonical_traceability_ignores_paths_and_order():
    from pilot.domain.amendment import canonical_traceability

    a = {"verdict": "pass", "summary": {"n": 1}, "findings": [{"category": "b"}, {"category": "a"}], "results": "/tmp/x/junit.xml",
         "requirements": [{"ref": "PARK-2~1", "tests": [{"name": "t2"}]}, {"ref": "PARK-1~1", "tests": [{"name": "t1"}]}]}
    b = {**a, "results": "/elsewhere/junit.xml", "findings": [{"category": "a"}, {"category": "b"}], "requirements": list(reversed(a["requirements"]))}
    assert canonical_traceability(a) == canonical_traceability(b)


def test_git_failing_to_list_the_changed_paths_is_a_problem_and_a_non_mapping_digest_object_is_refused():
    assert any("could not list" in p for p in evaluate_amendment(facts(changed_paths=None)))
    bad = {"superseded_tag": "old", "superseded_commit": OLD, "declared_paths": ["harness/", "prereg/PREREG.md"], "import_digests": "ab" * 32}
    assert any("no import digests" in p for p in evaluate_amendment(facts(amendment=bad)))


def test_the_imports_must_match_the_run_and_the_amendment():
    assert any("is missing" in p for p in evaluate_amendment(facts(target_listing={})))
    assert any("differs from its digest" in p for p in evaluate_amendment(facts(target_listing={"raw/x/result.json": "9" * 64})))
    assert any("no imports.json" in p for p in evaluate_amendment(facts(imports=None)))
    other = {"superseded_tag": "old", "superseded_commit": OLD, "declared_paths": ["harness/", "prereg/PREREG.md"],
             "import_digests": {"raw/y/result.json": "1" * 64}}
    assert any("import digests differ" in p for p in evaluate_amendment(facts(amendment=other)))


@pytest.mark.parametrize(
    ("path", "declared", "expected"),
    [
        ("Makefile", ["Makefile"], True),
        ("script/run-pilot", ["script/"], True),
        ("script/run-pilot", ["script"], False),
        ("freeze/prereg-commit.tsr", ["freeze/prereg-commit.*"], True),
        ("freeze/a1-commit.tsr", ["freeze/prereg-commit.*"], False),
        ("harness/src/pilot/x.py", ["harness/"], True),
        ("harnessx/a.py", ["harness/"], False),
        ("fixture/parkfee/a.py", ["harness/", "prereg/PREREG.md"], False),
    ],
)
def test_declared_paths_are_exact_directory_prefixes_or_globs(path, declared, expected):
    assert is_declared(path, declared) is expected


def test_undeclared_paths_are_sorted():
    assert undeclared_paths(["b", "a", "c"], ["c"]) == ["a", "b"]


FREEZE = Path(__file__).resolve().parents[3] / "freeze"


def test_the_real_reply_of_the_superseded_freeze_parses_and_its_imprint_is_the_hash_of_the_commit_file():
    reply = subprocess.run(
        ["openssl", "ts", "-reply", "-in", str(FREEZE / "prereg-commit.tsr"), "-text"], capture_output=True, text=True, check=True
    ).stdout
    stamp = parse_reply(reply)
    assert stamp.imprint == hashlib.sha256((FREEZE / "prereg-commit.txt").read_bytes()).hexdigest()
    assert stamp.generated_at == datetime(2026, 10, 6, 21, 49, 9, tzinfo=UTC)


def test_a_reply_without_an_imprint_or_a_time_does_not_parse():
    with pytest.raises(ValueError):
        parse_reply("Status: Granted.\n")
