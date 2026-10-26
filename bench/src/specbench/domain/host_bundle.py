"""The manifest of a pushed host bundle (`refs/musthold/host/<module>/<head sha>`): the commit it was pushed for, the
responses it lists, and the manifest of the bundle that holds the responses of two."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from specbench.domain.jsonread import RecordError, integer, mapping, sequence, text

MANIFEST_FILE: Final = "manifest.json"
RESPONSES_DIRECTORY: Final = "responses"


class ForeignBundle(ValueError):
    """A bundle was pushed for another commit than the one CI proves."""


@dataclass(frozen=True)
class BundleManifest:
    head_sha: str
    responses: frozenset[tuple[str, int]]
    """The (request id, vote) of every response the bundle lists."""

    def require_head(self, head_sha: str) -> None:
        if self.head_sha != head_sha:
            raise ForeignBundle(f"the bundle was pushed for {self.head_sha}, not for {head_sha}")


def merge_bundle_manifests(base: str, overlay: str) -> str:
    """The manifest of the bundle that holds the responses of both: the overlay's fields, and every entry of either, the
    overlay's entry winning where both list the same response. Entries are sorted by request id and vote."""
    first, second = _document(base), _document(overlay)
    entries: dict[tuple[str, int], Mapping[str, object]] = {}
    for document in (first, second):
        for item in sequence(document.get("responses"), "responses"):
            entry = mapping(item, "response entry")
            entries[(text(entry.get("requestId"), "requestId"), integer(entry.get("vote"), "vote"))] = entry
    merged = {**second, "responses": [entries[key] for key in sorted(entries)]}
    return json.dumps(merged, indent=2) + "\n"


def _document(content: str) -> Mapping[str, object]:
    try:
        return mapping(json.loads(content), MANIFEST_FILE)
    except json.JSONDecodeError as error:
        raise RecordError(f"{MANIFEST_FILE} is not JSON: {error}") from error


def parse_bundle_manifest(content: str) -> BundleManifest:
    try:
        body = mapping(json.loads(content), MANIFEST_FILE)
    except json.JSONDecodeError as error:
        raise RecordError(f"{MANIFEST_FILE} is not JSON: {error}") from error
    entries = [mapping(item, f"{MANIFEST_FILE} response") for item in sequence(body.get("responses"), "responses")]
    return BundleManifest(
        text(body.get("headSha"), f"{MANIFEST_FILE} headSha"),
        frozenset((text(e.get("requestId"), "requestId"), integer(e.get("vote"), "vote")) for e in entries),
    )
