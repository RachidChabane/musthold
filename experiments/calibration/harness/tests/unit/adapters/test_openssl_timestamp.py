from __future__ import annotations

import re
import shutil
from collections.abc import Mapping, Sequence
from datetime import UTC, datetime
from pathlib import Path

import pytest

from calibration.adapters.openssl_timestamp import OpensslTimestampReader, parse_listing
from calibration.domain.ports import ProcessResult
from real_process_runner import RealProcessRunner
from repository_paths import CALIBRATION_REPLY, PILOT_REPLY

LISTING = """Using configuration from /opt/homebrew/etc/openssl@3/openssl.cnf
Status info:
Status: Granted.

TST info:
Version: 1
Hash Algorithm: sha256
Message data:
    0000 - ac 24 1d 1c 79 94 a3 26-8c e8 49 10 56 fd 3a 76   .$..y..&..I.V.:v
    0010 - af 3c d5 50 96 2d 92 8d-7b f5 46 09 7b 8d df 2c   .<.P.-..{.F.{..,
Serial number: 0xB894F54AAA999014582BB8FAEDF64F7B
Time stamp: Oct  7 10:27:05 2026 GMT
Accuracy: unspecified
"""
IMPRINT = "ac241d1c7994a3268ce8491056fd3a76af3cd550962d928d7bf546097b8ddf2c"
GENERATED = datetime(2026, 10, 7, 10, 27, 5, tzinfo=UTC)
SHA256_HEX = re.compile(r"[0-9a-f]{64}")

requires_openssl = pytest.mark.skipif(shutil.which("openssl") is None, reason="openssl is not installed")


class StubRunner:
    def __init__(self, result: ProcessResult) -> None:
        self.result = result
        self.calls: list[Sequence[str]] = []

    def run(
        self,
        argv: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        unset: Sequence[str] = (),
        input_bytes: bytes | None = None,
        timeout_seconds: float | None = None,
    ) -> ProcessResult:
        self.calls.append(list(argv))
        return self.result


def test_parse_listing_reads_the_imprint_and_the_generation_time() -> None:
    timestamp = parse_listing(LISTING)
    assert timestamp is not None
    assert timestamp.imprint == IMPRINT
    assert timestamp.generated_at == GENERATED
    assert timestamp.generated_at.tzinfo is UTC


@pytest.mark.parametrize(
    "listing",
    [
        "",
        LISTING.replace("Message data:", "Other data:"),
        LISTING.replace("Time stamp:", "Stamped:"),
        LISTING.replace("Oct", "Xyz"),
        LISTING.replace("10:27:05", "10:27:99"),
    ],
)
def test_parse_listing_is_none_without_a_complete_imprint_and_time(listing: str) -> None:
    assert parse_listing(listing) is None


def test_the_reader_runs_openssl_on_the_reply(tmp_path: Path) -> None:
    reply = tmp_path / "reply.tsr"
    reply.write_bytes(b"stub")
    runner = StubRunner(ProcessResult(0, LISTING.encode(), b""))
    timestamp = OpensslTimestampReader(runner).read(reply)
    assert runner.calls == [["openssl", "ts", "-reply", "-in", str(reply), "-text"]]
    assert timestamp is not None
    assert timestamp.imprint == IMPRINT


def test_the_reader_is_none_for_a_failed_command_and_for_a_missing_file(tmp_path: Path) -> None:
    reply = tmp_path / "reply.tsr"
    reply.write_bytes(b"stub")
    failing = StubRunner(ProcessResult(1, b"", b"error"))
    assert OpensslTimestampReader(failing).read(reply) is None
    assert OpensslTimestampReader(failing).read(tmp_path / "absent.tsr") is None
    assert len(failing.calls) == 1


@requires_openssl
def test_the_reader_is_none_for_garbage(tmp_path: Path) -> None:
    garbage = tmp_path / "garbage.tsr"
    garbage.write_bytes(b"this is not an RFC 3161 reply")
    assert OpensslTimestampReader(RealProcessRunner()).read(garbage) is None


@requires_openssl
def test_the_reader_reads_the_committed_calibration_reply() -> None:
    timestamp = OpensslTimestampReader(RealProcessRunner()).read(CALIBRATION_REPLY)
    assert timestamp is not None
    assert timestamp.generated_at == GENERATED
    assert SHA256_HEX.fullmatch(timestamp.imprint)


@requires_openssl
def test_the_reader_reads_the_committed_pilot_reply() -> None:
    timestamp = OpensslTimestampReader(RealProcessRunner()).read(PILOT_REPLY)
    assert timestamp is not None
    assert timestamp.generated_at.tzinfo is UTC
    assert SHA256_HEX.fullmatch(timestamp.imprint)
