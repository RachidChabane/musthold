from __future__ import annotations

import hashlib
import json
from collections.abc import Mapping

import pytest
from hypothesis import given
from hypothesis import strategies as st

from calibration.adapters.ollama_registry import (
    MANIFEST_MEDIA_TYPES,
    HttpOllamaRegistry,
    RegistryError,
    manifest_size,
)
from calibration.domain.ports import HttpReply, TransportFailure

SIZES = st.integers(min_value=0, max_value=10**13)


def manifest_body(config_size: int, layer_sizes: list[int]) -> bytes:
    document = {
        "schemaVersion": 2,
        "mediaType": MANIFEST_MEDIA_TYPES[0],
        "config": {
            "mediaType": "application/vnd.docker.container.image.v1+json",
            "digest": "sha256:aa",
            "size": config_size,
        },
        "layers": [
            {"mediaType": "application/vnd.ollama.image.model", "digest": f"sha256:{i:02x}", "size": size}
            for i, size in enumerate(layer_sizes)
        ],
    }
    return json.dumps(document, indent=2).encode()


class FakeTransport:
    def __init__(self, reply: HttpReply | TransportFailure) -> None:
        self._reply = reply
        self.requests: list[tuple[str, Mapping[str, str] | None]] = []

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        self.requests.append((url, headers))
        if isinstance(self._reply, TransportFailure):
            raise self._reply
        return self._reply


def test_manifest_digest_and_size_come_from_the_served_bytes() -> None:
    body = manifest_body(500, [1000, 2000, 30])
    transport = FakeTransport(HttpReply(200, body))
    manifest = HttpOllamaRegistry(transport).manifest("gemma3", "4b")
    assert manifest.digest == hashlib.sha256(body).hexdigest()
    assert manifest.size_bytes == 3530
    assert (manifest.library, manifest.tag) == ("gemma3", "4b")


def test_request_targets_the_library_manifest_with_the_manifest_media_types() -> None:
    transport = FakeTransport(HttpReply(200, manifest_body(1, [])))
    HttpOllamaRegistry(transport, base_url="http://registry.test/").manifest("gemma3", "4b")
    [(url, headers)] = transport.requests
    assert url == "http://registry.test/v2/library/gemma3/manifests/4b"
    assert headers is not None
    assert headers["Accept"] == ", ".join(MANIFEST_MEDIA_TYPES)


def test_non_200_is_a_registry_error() -> None:
    with pytest.raises(RegistryError, match="HTTP 404"):
        HttpOllamaRegistry(FakeTransport(HttpReply(404, b"{}"))).manifest("x", "y")


def test_transport_failure_is_a_registry_error() -> None:
    with pytest.raises(RegistryError, match="refused"):
        HttpOllamaRegistry(FakeTransport(TransportFailure("refused"))).manifest("x", "y")


@pytest.mark.parametrize(
    "body",
    [
        b"not json",
        b"[]",
        b"{}",
        b'{"config": {"size": 1}}',
        b'{"config": {}, "layers": []}',
        b'{"config": {"size": 1}, "layers": [{"size": -3}]}',
        b'{"config": {"size": 1}, "layers": [{"size": true}]}',
        b'{"config": {"size": 1}, "layers": ["x"]}',
    ],
)
def test_malformed_manifests_are_refused(body: bytes) -> None:
    with pytest.raises(RegistryError, match="malformed manifest"):
        HttpOllamaRegistry(FakeTransport(HttpReply(200, body))).manifest("x", "y")


@given(config=SIZES, layers=st.lists(SIZES, max_size=20))
def test_size_is_config_plus_layers_and_never_negative(config: int, layers: list[int]) -> None:
    size = manifest_size(manifest_body(config, layers))
    assert size == config + sum(layers)
    assert size >= 0


@given(config=SIZES, layers=st.lists(SIZES, max_size=20))
def test_digest_is_stable_for_identical_bytes_and_differs_for_other_bytes(config: int, layers: list[int]) -> None:
    body = manifest_body(config, layers)
    first = HttpOllamaRegistry(FakeTransport(HttpReply(200, body))).manifest("m", "t")
    second = HttpOllamaRegistry(FakeTransport(HttpReply(200, body))).manifest("m", "t")
    other = HttpOllamaRegistry(FakeTransport(HttpReply(200, body + b"\n"))).manifest("m", "t")
    assert first.digest == second.digest
    assert first.digest != other.digest
