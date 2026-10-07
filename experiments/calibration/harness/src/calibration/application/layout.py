"""Where each record of a run directory lives (§15.1)."""

from __future__ import annotations

from calibration.domain.canonical import slugify
from calibration.domain.constants import (
    CANARY_DIR,
    ITEMS_DIR,
    META_FILE,
    RAW_DIR,
    REQUEST_FILE,
    RESPONSE_FILE,
    SERVER_DIR,
)
from calibration.domain.schedule import Pass, request_slug


def item_path(request_id: str) -> str:
    return f"{ITEMS_DIR}/{request_slug(request_id)}.json"


def raw_dir(model_id: str, which: Pass, request_id: str) -> str:
    return f"{RAW_DIR}/{model_id}/{which.value}/{request_slug(request_id)}"


def raw_request_path(model_id: str, which: Pass, request_id: str) -> str:
    return f"{raw_dir(model_id, which, request_id)}/{REQUEST_FILE}"


def raw_response_path(model_id: str, which: Pass, request_id: str) -> str:
    return f"{raw_dir(model_id, which, request_id)}/{RESPONSE_FILE}"


def raw_meta_path(model_id: str, which: Pass, request_id: str) -> str:
    return f"{raw_dir(model_id, which, request_id)}/{META_FILE}"


def canary_path(model_id: str) -> str:
    return f"{CANARY_DIR}/{model_id}.json"


def server_card_path(model_id: str, block_start: str) -> str:
    return f"{SERVER_DIR}/{model_id}/{slugify(block_start)}.json"
