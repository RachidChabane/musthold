"""PREREG 2: the toolchain and engine facts the manifest records, the pins they must match, and their drift."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

from demos1.domain.toolchains import GO_VERSION

UV_VERSION: Final = "0.11.23"
GH_VERSION: Final = "2.93.0"
GO_IMAGE: Final = "golang:1.26.4-bookworm"
UV_IMAGE: Final = "ghcr.io/astral-sh/uv:0.11.23"
PREREG_TAG: Final = "prereg-demos-1"
PRODUCT_TAG: Final = "demos-1-product"
PRODUCT_PATHS: Final = ("discpower/", "format/", "umbrella/")


class Fact(StrEnum):
    """Every fact recorded in the manifest and in each attempt record."""

    HOST_OS_NAME = "host_os_name"
    HOST_OS_VERSION = "host_os_version"
    HOST_ARCH = "host_arch"
    GH_VERSION = "gh_version"
    ENGINE_VERSION = "docker_engine_version"
    ENGINE_MEMORY_BYTES = "docker_engine_memory_bytes"
    ENGINE_CPUS = "docker_engine_cpus"
    COLIMA_VERSION = "colima_version"
    MACHINE_PROFILE = "machine_profile"
    MACHINE_CPUS = "machine_cpus"
    MACHINE_MEMORY_BYTES = "machine_memory_bytes"
    MACHINE_DISK_BYTES = "machine_disk_bytes"
    MACHINE_ARCH = "machine_arch"
    MACHINE_RUNTIME = "machine_runtime"
    MACHINE_MOUNTS = "machine_mounts"
    IMAGE_ID = "image_digest"
    GO_SOURCE_DIGEST = "go_source_image_digest"
    UV_SOURCE_DIGEST = "uv_source_image_digest"
    IMAGE_OS = "image_os"
    IMAGE_ARCH = "image_arch"
    IMAGE_GIT_VERSION = "image_git_version"
    IMAGE_GO_VERSION = "image_go_version"
    IMAGE_UV_VERSION = "image_uv_version"
    UV_CPYTHON = "uv_cpython_by_minor"
    PRODUCT_COMMIT = "product_commit"
    PRODUCT_VERSION = "spec_proof_version"
    SPEC_PROOF_SHA256 = "spec_proof_sha256"
    SPEC_PROOF_DP_SHA256 = "spec_proof_dp_sha256"


DRIFT_FACTS: Final = tuple(Fact)
"""Every recorded fact: a value that differs from the one recorded at `select` is a deviation."""


@dataclass(frozen=True)
class Pin:
    fact: Fact
    expected: str


PINS: Final = (
    Pin(Fact.IMAGE_GO_VERSION, f"go{GO_VERSION}"),
    Pin(Fact.IMAGE_UV_VERSION, UV_VERSION),
    Pin(Fact.GH_VERSION, GH_VERSION),
)


@dataclass(frozen=True)
class Mismatch:
    fact: str
    expected: str | None
    observed: str | None


def pin_mismatches(facts: Mapping[str, str]) -> list[Mismatch]:
    """The pinned versions of PREREG 2 the facts do not show."""
    return [
        Mismatch(pin.fact.value, pin.expected, facts.get(pin.fact.value))
        for pin in PINS
        if facts.get(pin.fact.value) != pin.expected
    ]


def drift(recorded: Mapping[str, str], current: Mapping[str, str]) -> list[Mismatch]:
    """Facts that differ from the ones recorded at `select`."""
    return [
        Mismatch(fact.value, recorded.get(fact.value), current.get(fact.value))
        for fact in DRIFT_FACTS
        if recorded.get(fact.value) != current.get(fact.value)
    ]


def go_version_word(go_version_output: str) -> str | None:
    """`go version go1.26.4 linux/arm64` gives `go1.26.4`."""
    words = go_version_output.split()
    return words[2] if len(words) > 2 else None


def uv_version_word(uv_version_output: str) -> str | None:
    """`uv 0.11.23 (...)` gives `0.11.23`."""
    words = uv_version_output.split()
    return words[1] if len(words) > 1 else None


def gh_version_word(gh_version_output: str) -> str | None:
    """`gh version 2.93.0 (2026-05-27)` gives `2.93.0`."""
    words = gh_version_output.split()
    return words[2] if len(words) > 2 else None
