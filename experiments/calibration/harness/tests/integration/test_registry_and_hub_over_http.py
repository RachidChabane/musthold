from __future__ import annotations

import hashlib
import json
import threading
import urllib.error
import urllib.request
from collections.abc import Iterator, Mapping
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from calibration.adapters.hub_catalog import HttpModelHub, HubError
from calibration.adapters.ollama_registry import MANIFEST_MEDIA_TYPES, HttpOllamaRegistry, RegistryError
from calibration.domain.ports import HttpReply, TransportFailure

pytestmark = pytest.mark.integration

LOOPBACK = "127.0.0.1"
EPHEMERAL_PORT = 0
SHA = "a" * 40
MANIFEST_BYTES = json.dumps(
    {
        "schemaVersion": 2,
        "mediaType": MANIFEST_MEDIA_TYPES[0],
        "config": {"digest": "sha256:11", "size": 490},
        "layers": [{"digest": "sha256:22", "size": 3_300_000_000}, {"digest": "sha256:33", "size": 1_100}],
    },
    indent=2,
).encode()
HUB_BYTES = json.dumps(
    {
        "sha": SHA,
        "siblings": [
            {"rfilename": "adapter_model.safetensors", "size": 4096},
            {"rfilename": "head.pt", "size": 512},
            {"rfilename": "adapter_config.json", "size": 800},
        ],
    }
).encode()


class UrllibHeaderedTransport:
    """Test-side HTTP GET with headers over the standard library."""

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        request = urllib.request.Request(url, headers=dict(headers or {}))
        try:
            with urllib.request.urlopen(request, timeout=timeout_seconds) as reply:
                return HttpReply(reply.status, reply.read())
        except urllib.error.HTTPError as error:
            return HttpReply(error.code, error.read())
        except urllib.error.URLError as error:
            raise TransportFailure(str(error.reason)) from error

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply:
        raise AssertionError("never posted")


class StubHandler(BaseHTTPRequestHandler):
    seen: list[tuple[str, str | None]]

    def do_GET(self) -> None:
        self.seen.append((self.path, self.headers.get("Accept")))
        routes = {
            "/v2/library/gemma3/manifests/4b": (200, MANIFEST_BYTES),
            "/api/models/owner/kev-4b/revision/main?blobs=true": (200, HUB_BYTES),
            "/api/models/owner/kev-4b/revision/" + SHA + "?blobs=true": (200, HUB_BYTES),
        }
        status, body = routes.get(self.path, (404, b'{"error": "not found"}'))
        self.send_response(status)
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: object) -> None:
        return


@pytest.fixture
def stub() -> Iterator[tuple[str, list[tuple[str, str | None]]]]:
    seen: list[tuple[str, str | None]] = []
    handler = type("Handler", (StubHandler,), {"seen": seen})
    server = ThreadingHTTPServer((LOOPBACK, EPHEMERAL_PORT), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://{LOOPBACK}:{server.server_address[1]}", seen
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def test_registry_digest_and_size_over_a_real_socket(stub: tuple[str, list[tuple[str, str | None]]]) -> None:
    base_url, seen = stub
    manifest = HttpOllamaRegistry(UrllibHeaderedTransport(), base_url=base_url).manifest("gemma3", "4b")
    assert manifest.digest == hashlib.sha256(MANIFEST_BYTES).hexdigest()
    assert manifest.size_bytes == 490 + 3_300_000_000 + 1_100
    assert seen == [("/v2/library/gemma3/manifests/4b", ", ".join(MANIFEST_MEDIA_TYPES))]


def test_registry_unknown_tag_is_a_typed_error(stub: tuple[str, list[tuple[str, str | None]]]) -> None:
    base_url, _ = stub
    with pytest.raises(RegistryError, match="HTTP 404"):
        HttpOllamaRegistry(UrllibHeaderedTransport(), base_url=base_url).manifest("gemma3", "missing")


def test_registry_unreachable_is_a_typed_error() -> None:
    with pytest.raises(RegistryError):
        HttpOllamaRegistry(UrllibHeaderedTransport(), base_url=f"http://{LOOPBACK}:1").manifest("gemma3", "4b")


@pytest.mark.parametrize("revision", [None, SHA])
def test_hub_revision_and_file_sizes_over_a_real_socket(
    stub: tuple[str, list[tuple[str, str | None]]], revision: str | None
) -> None:
    base_url, _ = stub
    repository = HttpModelHub(UrllibHeaderedTransport(), base_url=base_url).repository("owner/kev-4b", revision)
    assert repository.revision == SHA
    assert sum(f.size_bytes for f in repository.weight_files) == 4096 + 512


def test_hub_unknown_repository_is_a_typed_error(stub: tuple[str, list[tuple[str, str | None]]]) -> None:
    base_url, _ = stub
    with pytest.raises(HubError, match="HTTP 404"):
        HttpModelHub(UrllibHeaderedTransport(), base_url=base_url).repository("owner/absent")
