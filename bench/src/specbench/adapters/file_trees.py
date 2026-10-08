"""Materializes a tree from a mapping of relative paths to bytes in a scratch directory."""

from __future__ import annotations

import shutil
import tempfile
from collections.abc import Generator, Mapping
from contextlib import contextmanager
from pathlib import Path

from specbench.domain.sealing import validate_bundle_path


class DiskFileTreeFactory:
    """`FileTreeFactory` over the local disk; the scratch directory is removed when the tree is released."""

    def __init__(self, scratch_root: Path | None = None) -> None:
        self._scratch_root = scratch_root

    @contextmanager
    def materialize_files(self, files: Mapping[str, bytes]) -> Generator[Path]:
        scratch = Path(tempfile.mkdtemp(prefix="specbench-oracle-", dir=self._scratch_root))
        try:
            for relative, content in files.items():
                target = scratch / validate_bundle_path(relative)
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(content)
            yield scratch
        finally:
            shutil.rmtree(scratch, ignore_errors=True)
