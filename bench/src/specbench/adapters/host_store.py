"""The requests a clean room's host store holds, read to tell which module and kind a ticket belongs to."""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.domain.jsonread import mapping, text
from specbench.domain.product import PROOF_MODULES, ProofModule

STORE_ROOT: Final = "musthold"
HOST_DIRECTORY: Final = "host"
REQUESTS_DIRECTORY: Final = "requests"


@dataclass(frozen=True)
class StoredRequest:
    module: ProofModule
    kind: str


class HostStoreRequests:
    """Reads `<state>/musthold/<module>/<project>/host/requests/<request id>.json` of every proof module."""

    def __init__(self, state: Path) -> None:
        self._state = state

    def find(self, request_id: str) -> StoredRequest | None:
        """The stored request, or `None` when the store holds none under that id."""
        for module in PROOF_MODULES:
            for path in (self._state / STORE_ROOT / module.host_ref_name).glob(
                f"*/{HOST_DIRECTORY}/{REQUESTS_DIRECTORY}/{request_id}.json"
            ):
                body = mapping(json.loads(path.read_text(encoding="utf-8")), f"request {request_id}")
                return StoredRequest(module, text(body.get("kind"), f"request {request_id} kind"))
        return None
