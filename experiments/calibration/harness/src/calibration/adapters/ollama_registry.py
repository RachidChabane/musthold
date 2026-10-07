"""`OllamaRegistry` over the registry's Docker/OCI manifest endpoint; the digest is that of the exact bytes served."""

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Final, Protocol

from calibration.application.pin_ports import OllamaManifest
from calibration.domain.canonical import sha256_hex
from calibration.domain.jsonread import RecordError, integer, mapping, sequence
from calibration.domain.ports import HttpReply, TransportFailure

DEFAULT_REGISTRY_URL: Final = "https://registry.ollama.ai"
MANIFEST_PATH: Final = "/v2/{library}/manifests/{tag}"
OFFICIAL_NAMESPACE: Final = "library"
ACCEPT_HEADER: Final = "Accept"
MANIFEST_MEDIA_TYPES: Final = (
    "application/vnd.docker.distribution.manifest.v2+json",
    "application/vnd.oci.image.manifest.v1+json",
)
MEDIA_TYPE_SEPARATOR: Final = ", "
HTTP_OK: Final = 200
METADATA_TIMEOUT_SECONDS: Final = 60.0
CONFIG_FIELD: Final = "config"
LAYERS_FIELD: Final = "layers"
SIZE_FIELD: Final = "size"


class RegistryError(Exception):
    """The registry answered with something other than a well-formed manifest, or did not answer."""


class HeaderedTransport(Protocol):
    """An HTTP GET that can carry request headers."""

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply: ...


def manifest_size(body: bytes) -> int:
    """The config size plus the sizes of every layer, from a manifest body."""
    try:
        document = mapping(json.loads(body), "manifest")
        config = mapping(document.get(CONFIG_FIELD), CONFIG_FIELD)
        layers = sequence(document.get(LAYERS_FIELD), LAYERS_FIELD)
        total = integer(config.get(SIZE_FIELD), "config.size")
        for position, layer in enumerate(layers):
            total += integer(mapping(layer, f"layers[{position}]").get(SIZE_FIELD), f"layers[{position}].size")
    except (RecordError, ValueError) as error:
        raise RegistryError(f"malformed manifest: {error}") from error
    return total


class HttpOllamaRegistry:
    """Implements `OllamaRegistry`: GET of one manifest, nothing is downloaded beyond it."""

    def __init__(
        self,
        transport: HeaderedTransport,
        base_url: str = DEFAULT_REGISTRY_URL,
        accept: tuple[str, ...] = MANIFEST_MEDIA_TYPES,
    ) -> None:
        self._transport = transport
        self._base_url = base_url.rstrip("/")
        self._accept = MEDIA_TYPE_SEPARATOR.join(accept)

    def manifest(self, library: str, tag: str) -> OllamaManifest:
        url = self._base_url + MANIFEST_PATH.format(library=f"{OFFICIAL_NAMESPACE}/{library}", tag=tag)
        reply = self._fetch(url)
        return OllamaManifest(
            library=library, tag=tag, digest=sha256_hex(reply.body), size_bytes=manifest_size(reply.body)
        )

    def _fetch(self, url: str) -> HttpReply:
        try:
            reply = self._transport.get(url, METADATA_TIMEOUT_SECONDS, {ACCEPT_HEADER: self._accept})
        except TransportFailure as error:
            raise RegistryError(f"{url}: {error.reason}") from error
        if reply.status != HTTP_OK:
            raise RegistryError(f"{url}: HTTP {reply.status}")
        return reply
