from __future__ import annotations

import hashlib
from pathlib import Path

import pytest
from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from calibration.adapters.pilot_run_reader import (
    ALLOWED_PATH,
    AllowlistedPilotRunReader,
    ForbiddenPilotFile,
    is_allowed,
    seal_digests,
)

MUTANT_ID = "0123456789abcdef"
ALLOWED = ["bindings.json", "processing.jsonl", "differential.jsonl", "SHA256SUMS", f"mutants/{MUTANT_ID}.json"]
FORBIDDEN = [
    "oracle_classes.json",
    "relabel.jsonl",
    "kills.jsonl",
    "mutant_classes.json",
    "alignment.jsonl",
    "alignment_aggregate.json",
    "validity.jsonl",
    "raw/call-1/result.json",
    "raw",
    "../bindings.json",
    "mutants/../kills.jsonl",
    "mutants/../bindings.json",
    f"mutants/{MUTANT_ID}.json/",
    f"mutants/{MUTANT_ID[:-1]}.json",
    f"mutants/{MUTANT_ID}0.json",
    f"mutants/{MUTANT_ID.upper()}.json",
    f"mutants/{MUTANT_ID}.jsonl",
    f"mutants/sub/{MUTANT_ID}.json",
    f"./mutants/{MUTANT_ID}.json",
    "./bindings.json",
    "bindings.json\n",
    f"mutants/{MUTANT_ID}.json\n",
    "mutants",
    "mutants/",
    "",
    " bindings.json",
    "Bindings.json",
]


def sha256(content: bytes) -> str:
    return hashlib.sha256(content).hexdigest()


def populate(run_dir: Path) -> dict[str, bytes]:
    files = {name: f"content of {name}".encode() for name in ALLOWED}
    for name in [*FORBIDDEN[:7], "raw/call-1/result.json"]:
        files[name] = b"forbidden"
    for name, content in files.items():
        (run_dir / name).parent.mkdir(parents=True, exist_ok=True)
        (run_dir / name).write_bytes(content)
    return files


@pytest.fixture
def run_dir(tmp_path: Path) -> Path:
    directory = tmp_path / "run-02"
    directory.mkdir()
    populate(directory)
    return directory


@pytest.mark.parametrize("relative", ALLOWED)
def test_an_allowlisted_file_is_read_and_digested(run_dir: Path, relative: str) -> None:
    reader = AllowlistedPilotRunReader(run_dir)
    assert reader.exists(relative)
    assert reader.read_text(relative) == f"content of {relative}"
    assert reader.digests() == {relative: sha256(f"content of {relative}".encode())}


@pytest.mark.parametrize("relative", FORBIDDEN)
def test_any_other_path_is_refused_before_any_io(run_dir: Path, relative: str) -> None:
    reader = AllowlistedPilotRunReader(run_dir)
    with pytest.raises(ForbiddenPilotFile):
        reader.read_text(relative)
    with pytest.raises(ForbiddenPilotFile):
        reader.exists(relative)
    assert reader.digests() == {}


def test_a_refusal_happens_even_when_the_run_directory_does_not_exist(tmp_path: Path) -> None:
    reader = AllowlistedPilotRunReader(tmp_path / "absent")
    with pytest.raises(ForbiddenPilotFile):
        reader.read_text("kills.jsonl")
    assert not reader.exists("bindings.json")


def test_an_absolute_path_is_refused(run_dir: Path) -> None:
    reader = AllowlistedPilotRunReader(run_dir)
    with pytest.raises(ForbiddenPilotFile):
        reader.read_text(str(run_dir / "bindings.json"))


def test_a_symbolic_link_is_refused_even_under_an_allowed_name(run_dir: Path, tmp_path: Path) -> None:
    target = tmp_path / "elsewhere.json"
    target.write_text("{}")
    link = run_dir / "mutants" / "ffffffffffffffff.json"
    link.symlink_to(target)
    with pytest.raises(ForbiddenPilotFile):
        AllowlistedPilotRunReader(run_dir).read_text("mutants/ffffffffffffffff.json")


def test_exists_is_false_for_an_allowed_file_that_is_absent(run_dir: Path) -> None:
    assert not AllowlistedPilotRunReader(run_dir).exists("mutants/ffffffffffffffff.json")


def test_digests_are_sorted_and_cover_every_read(run_dir: Path) -> None:
    reader = AllowlistedPilotRunReader(run_dir)
    reader.read_text("processing.jsonl")
    reader.read_text("bindings.json")
    reader.exists("differential.jsonl")
    assert list(reader.digests()) == ["bindings.json", "processing.jsonl"]


@settings(suppress_health_check=[HealthCheck.function_scoped_fixture], max_examples=300)
@given(relative=st.text(max_size=40).filter(lambda text: ALLOWED_PATH.fullmatch(text) is None))
def test_any_path_outside_the_allowlist_pattern_is_refused(run_dir: Path, relative: str) -> None:
    reader = AllowlistedPilotRunReader(run_dir)
    with pytest.raises(ForbiddenPilotFile):
        reader.read_text(relative)
    with pytest.raises(ForbiddenPilotFile):
        reader.exists(relative)


@given(
    prefix=st.sampled_from(["", "./", "../", "/", "mutants/", "raw/"]),
    name=st.sampled_from(
        [
            "oracle_classes.json",
            "relabel.jsonl",
            "kills.jsonl",
            "mutant_classes.json",
            "alignment.jsonl",
            "validity.jsonl",
        ]
    ),
)
def test_no_truth_or_judgment_file_name_is_allowed_under_any_prefix(prefix: str, name: str) -> None:
    assert not is_allowed(prefix + name)


@given(identifier=st.text(alphabet="0123456789abcdef", min_size=16, max_size=16))
def test_every_sixteen_hex_mutant_file_is_allowed(identifier: str) -> None:
    assert is_allowed(f"mutants/{identifier}.json")


@given(identifier=st.text(alphabet="0123456789abcdefABCDEFg", min_size=0, max_size=24).filter(lambda t: len(t) != 16))
def test_a_mutant_file_whose_identifier_is_not_sixteen_hex_digits_is_refused(identifier: str) -> None:
    assert not is_allowed(f"mutants/{identifier}.json")


def test_the_seal_verifies_when_every_read_digest_matches(run_dir: Path) -> None:
    files = {name: (run_dir / name).read_bytes() for name in ["bindings.json", "processing.jsonl"]}
    seal = "".join(f"{sha256(content)}  {name}\n" for name, content in files.items())
    (run_dir / "SHA256SUMS").write_text(seal)
    reader = AllowlistedPilotRunReader(run_dir)
    for name in files:
        reader.read_text(name)
    assert reader.verify_against_seal() == []


def test_the_seal_reports_a_changed_file_and_a_file_it_does_not_list(run_dir: Path) -> None:
    sealed = f"{sha256(b'original')}  bindings.json\n"
    (run_dir / "SHA256SUMS").write_text(sealed)
    reader = AllowlistedPilotRunReader(run_dir)
    reader.read_text("bindings.json")
    reader.read_text("processing.jsonl")
    mismatches = reader.verify_against_seal()
    assert len(mismatches) == 2
    assert mismatches[0].startswith("bindings.json: read ")
    assert sha256(b"original") in mismatches[0]
    assert mismatches[1].startswith("processing.jsonl: ")
    assert "absent from the seal" in mismatches[1]


def test_without_a_seal_nothing_is_reported(run_dir: Path) -> None:
    (run_dir / "SHA256SUMS").unlink()
    reader = AllowlistedPilotRunReader(run_dir)
    reader.read_text("bindings.json")
    assert reader.verify_against_seal() == []
    assert reader.digests().keys() == {"bindings.json"}


def test_seal_lines_accept_the_binary_marker_and_a_current_directory_prefix() -> None:
    first, second, ignored = "a" * 64, "b" * 64, "not a seal line"
    text = f"{first}  ./bindings.json\n{second} *processing.jsonl\n{ignored}\n"
    assert seal_digests(text) == {"bindings.json": first, "processing.jsonl": second}
