"""A SYNTHETIC sealed run-01, with the file set the import expects and nothing else."""

from __future__ import annotations

import json
from pathlib import Path

from pilot.adapters.run_files import file_digests
from pilot.domain.import_manifest import (
    EXPECTED_CALL_ROLES,
    EXPECTED_CANARY_FILES,
    OPUS_GENERATION_IDS,
    RETIRED_CALL_IDS,
    TERRA_GENERATION_IDS,
    render_sums,
)

GENERATION_IDS = OPUS_GENERATION_IDS + TERRA_GENERATION_IDS + RETIRED_CALL_IDS


def write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def build_run01(directory: Path, *, seal: bool = True) -> Path:
    for call_id in EXPECTED_CALL_ROLES:
        write(directory / "raw" / call_id / "result.json", json.dumps({"status": "ok", "call": call_id}))
        write(directory / "raw" / call_id / "attempt-1" / "request.txt", f"request of {call_id}")
    for call_id in RETIRED_CALL_IDS:
        write(directory / "raw" / call_id / "attempt-1" / "request.txt", f"request of {call_id}")
    for name in EXPECTED_CANARY_FILES:
        write(directory / "canary" / name, json.dumps({"input_tokens": 100, "num_turns": 1, "cli_version": "x"}))
        write(directory / "raw" / "canary" / name.removesuffix(".json") / "result.json", json.dumps({"status": "ok"}))
    plan = {"arm_t": [{"call_id": c, "criterion": c.split("__")[1], "model": "m", "cli": "claude", "nonce": f"nonce-{c}"} for c in GENERATION_IDS]}
    write(directory / "plan.json", json.dumps(plan))
    write(directory / "DEVIATIONS.md", "# Deviations\n")
    write(directory / "manifest.json", json.dumps({"run_id": "run-01"}))
    if seal:
        write(directory / "SHA256SUMS", render_sums(file_digests(directory, exclude=("SHA256SUMS",))))
    return directory
