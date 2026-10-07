"""`ModelHub` over the Hub's revision endpoint: the commit sha and the size of every file, from metadata only."""

from __future__ import annotations

import json
from typing import Final
from urllib.parse import quote

from calibration.application.pin_ports import HubFile, HubRepository
from calibration.domain.jsonread import RecordError, integer, mapping, sequence, text
from calibration.domain.ports import HttpTransport, TransportFailure

DEFAULT_HUB_URL: Final = "https://huggingface.co"
DEFAULT_REVISION: Final = "main"
REVISION_PATH: Final = "/api/models/{repository}/revision/{revision}?blobs=true"
HTTP_OK: Final = 200
METADATA_TIMEOUT_SECONDS: Final = 60.0
SHA_FIELD: Final = "sha"
SIBLINGS_FIELD: Final = "siblings"
PATH_FIELD: Final = "rfilename"
SIZE_FIELD: Final = "size"


class HubError(Exception):
    """The Hub answered with something other than a well-formed revision document, or did not answer."""


class HttpModelHub:
    """Implements `ModelHub`: one GET per repository."""

    def __init__(self, transport: HttpTransport, base_url: str = DEFAULT_HUB_URL) -> None:
        self._transport = transport
        self._base_url = base_url.rstrip("/")

    def repository(self, repository: str, revision: str | None = None) -> HubRepository:
        url = self._base_url + REVISION_PATH.format(
            repository=quote(repository, safe="/"), revision=quote(revision or DEFAULT_REVISION, safe="")
        )
        body = self._fetch(url)
        try:
            return self._parse(repository, body)
        except (RecordError, ValueError) as error:
            raise HubError(f"{url}: malformed revision document: {error}") from error

    def _fetch(self, url: str) -> bytes:
        try:
            reply = self._transport.get(url, METADATA_TIMEOUT_SECONDS)
        except TransportFailure as error:
            raise HubError(f"{url}: {error.reason}") from error
        if reply.status != HTTP_OK:
            raise HubError(f"{url}: HTTP {reply.status}")
        return reply.body

    @staticmethod
    def _parse(repository: str, body: bytes) -> HubRepository:
        document = mapping(json.loads(body), "revision")
        siblings = sequence(document.get(SIBLINGS_FIELD), SIBLINGS_FIELD)
        files: list[HubFile] = []
        for position, sibling in enumerate(siblings):
            entry = mapping(sibling, f"{SIBLINGS_FIELD}[{position}]")
            files.append(
                HubFile(
                    path=text(entry.get(PATH_FIELD), f"{SIBLINGS_FIELD}[{position}].{PATH_FIELD}"),
                    size_bytes=integer(entry.get(SIZE_FIELD), f"{SIBLINGS_FIELD}[{position}].{SIZE_FIELD}"),
                )
            )
        return HubRepository(
            repository=repository, revision=text(document.get(SHA_FIELD), SHA_FIELD), files=tuple(files)
        )
