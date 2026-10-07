"""The files a mining run leaves: candidates, attrition log, sample and manifest, all canonical JSON."""

from __future__ import annotations

from collections.abc import Iterable
from pathlib import Path
from typing import Final

from specbench.application.mine import MiningResult
from specbench.domain.candidates import task_id_for
from specbench.domain.canonical import JsonValue, canonical_json

CANDIDATES_FILE: Final = "candidates.jsonl"
ATTRITION_FILE: Final = "attrition.jsonl"
SAMPLE_FILE: Final = "sample.json"
MINING_FILE: Final = "mining.json"
ENCODING: Final = "utf-8"


def _lines(records: Iterable[dict[str, JsonValue]]) -> str:
    return "".join(canonical_json(record) for record in records)


def write_mining(result: MiningResult, directory: Path) -> list[Path]:
    """Write the four outputs of a mining run and return their paths."""
    directory.mkdir(parents=True, exist_ok=True)
    sample = [{"task_id": task_id_for(c), **c.to_record()} for c in result.sample]
    contents = {
        CANDIDATES_FILE: _lines(c.to_record() for c in result.candidates),
        ATTRITION_FILE: _lines(a.to_record() for a in result.attrition),
        SAMPLE_FILE: canonical_json(list(sample)),
        MINING_FILE: canonical_json(result.manifest_record()),
    }
    paths: list[Path] = []
    for name, content in contents.items():
        path = directory / name
        path.write_text(content, encoding=ENCODING)
        paths.append(path)
    return paths
