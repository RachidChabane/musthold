"""Builds the host bundle CI imports from the answers of round 0 and the bundle a CI round trip pushed.

A bundle is the flat tree of a pushed host ref: a manifest and one file per response. The merged bundle holds every
response of either, the later bundle's file and manifest entry winning where both hold the same response, so the answers
the settle step replayed and the answers a gate-host session gave in a CI round are both there for CI to find.
"""

from __future__ import annotations

import shutil
from pathlib import Path

from specbench.domain.host_bundle import MANIFEST_FILE, merge_bundle_manifests


def merge_bundles(earlier: Path, later: Path, destination: Path) -> Path:
    """`destination` holds the union of the two bundles; it is rebuilt if it exists."""
    shutil.rmtree(destination, ignore_errors=True)
    shutil.copytree(earlier, destination)
    shutil.copytree(later, destination, dirs_exist_ok=True)
    manifest = (
        merge_bundle_manifests(
            (earlier / MANIFEST_FILE).read_text(encoding="utf-8"), (later / MANIFEST_FILE).read_text(encoding="utf-8")
        )
        if (earlier / MANIFEST_FILE).is_file()
        else (later / MANIFEST_FILE).read_text(encoding="utf-8")
    )
    (destination / MANIFEST_FILE).write_text(manifest, encoding="utf-8")
    return destination
