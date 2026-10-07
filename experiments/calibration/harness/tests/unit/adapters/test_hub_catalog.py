from __future__ import annotations

import json
from collections.abc import Mapping

import pytest

from calibration.adapters.hub_catalog import HttpModelHub, HubError
from calibration.domain.ports import HttpReply, TransportFailure

SHA = "0123456789abcdef0123456789abcdef01234567"


class FakeTransport:
    def __init__(self, reply: HttpReply | TransportFailure) -> None:
        self._reply = reply
        self.urls: list[str] = []

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        self.urls.append(url)
        if isinstance(self._reply, TransportFailure):
            raise self._reply
        return self._reply

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply:
        raise AssertionError("the Hub catalog never posts")


def revision_body(siblings: list[dict[str, object]]) -> bytes:
    return json.dumps({"_id": "x", "id": "owner/name", "sha": SHA, "siblings": siblings}).encode()


def test_the_revision_is_the_sha_and_files_carry_their_sizes() -> None:
    body = revision_body(
        [
            {"rfilename": "adapter_model.safetensors", "size": 1234},
            {"rfilename": "head.pt", "size": 99},
            {"rfilename": "README.md", "size": 7},
        ]
    )
    transport = FakeTransport(HttpReply(200, body))
    result = HttpModelHub(transport, base_url="http://hub.test/").repository("owner/name", "abc123")
    assert transport.urls == ["http://hub.test/api/models/owner/name/revision/abc123?blobs=true"]
    assert result.revision == SHA
    assert result.repository == "owner/name"
    assert [(f.path, f.size_bytes) for f in result.files] == [
        ("adapter_model.safetensors", 1234),
        ("head.pt", 99),
        ("README.md", 7),
    ]
    assert [f.path for f in result.weight_files] == ["adapter_model.safetensors", "head.pt"]


def test_the_default_revision_is_main() -> None:
    transport = FakeTransport(HttpReply(200, revision_body([])))
    HttpModelHub(transport).repository("owner/name")
    assert transport.urls == ["https://huggingface.co/api/models/owner/name/revision/main?blobs=true"]


def test_a_revision_with_a_slash_is_encoded_as_one_segment() -> None:
    transport = FakeTransport(HttpReply(200, revision_body([])))
    HttpModelHub(transport).repository("owner/name", "refs/pr/1")
    assert "/revision/refs%2Fpr%2F1?" in transport.urls[0]


def test_non_200_is_a_hub_error() -> None:
    with pytest.raises(HubError, match="HTTP 401"):
        HttpModelHub(FakeTransport(HttpReply(401, b"{}"))).repository("owner/name")


def test_transport_failure_is_a_hub_error() -> None:
    with pytest.raises(HubError, match="timed out"):
        HttpModelHub(FakeTransport(TransportFailure("timed out", timed_out=True))).repository("owner/name")


@pytest.mark.parametrize(
    "body",
    [
        b"nope",
        b"[]",
        b"{}",
        json.dumps({"sha": SHA}).encode(),
        json.dumps({"siblings": []}).encode(),
        revision_body([{"rfilename": "a.pt"}]),
        revision_body([{"size": 3}]),
        revision_body([{"rfilename": "a.pt", "size": -1}]),
    ],
)
def test_malformed_documents_are_refused(body: bytes) -> None:
    with pytest.raises(HubError, match="malformed"):
        HttpModelHub(FakeTransport(HttpReply(200, body))).repository("owner/name")
