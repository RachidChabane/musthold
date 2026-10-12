"""The tooling steps of PREREG 2: the linux build of the product binaries, and the image built from digests."""

from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path, PurePosixPath
from typing import Final

from demos1.domain.environment import GO_IMAGE, UV_IMAGE
from demos1.domain.jsonvalue import JsonObject, encode

PRODUCT_BINARIES: Final = (("umbrella", "spec-proof"), ("discpower", "spec-proof-dp"))
"""The module directory and the binary of each product binary; both are mounted in one directory."""
BUILD_GOMAXPROCS: Final = "2"
IMAGE_NAME: Final = "spec-proof-demos-1"
BUILD_TIMEOUT_SECONDS: Final = 3600
READ_CHUNK: Final = 1 << 20


class BuildFailed(RuntimeError):
    pass


def _run(argv: list[str], *, env: Mapping[str, str] | None = None, cwd: Path | None = None) -> str:
    completed = subprocess.run(
        argv, capture_output=True, text=True, env=env, cwd=cwd, timeout=BUILD_TIMEOUT_SECONDS, check=False
    )
    if completed.returncode != 0:
        raise BuildFailed(f"{' '.join(argv)}: {completed.stderr.strip()[-3000:]}")
    return completed.stdout.strip()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        while chunk := handle.read(READ_CHUNK):
            digest.update(chunk)
    return digest.hexdigest()


def ldflags(commit: str) -> str:
    return f"-X main.version={commit} -X main.commit={commit}"


@dataclass(frozen=True)
class ProductBuild:
    repo_root: Path
    product_root: Path

    def build(self, ref: str) -> JsonObject:
        """`CGO_ENABLED=0 GOOS=linux GOARCH=<host> go build -ldflags ...` of both binaries, from the ref's tree."""
        commit = _run(["git", "-C", str(self.repo_root), "rev-parse", "--verify", f"{ref}^{{commit}}"])
        source = self.product_root / "src"
        binaries = self.product_root / "bin"
        shutil.rmtree(source, ignore_errors=True)
        source.mkdir(parents=True)
        binaries.mkdir(parents=True, exist_ok=True)
        archive = subprocess.Popen(
            ["git", "-C", str(self.repo_root), "archive", "--format=tar", commit], stdout=subprocess.PIPE
        )
        subprocess.run(["tar", "-x", "-C", str(source)], stdin=archive.stdout, check=True)
        if archive.wait() != 0:
            raise BuildFailed(f"git archive {commit} failed")
        arch = _run(["go", "env", "GOHOSTARCH"])
        environment = {
            **os.environ,
            "CGO_ENABLED": "0",
            "GOOS": "linux",
            "GOARCH": arch,
            "GOTOOLCHAIN": "local",
            "GOMAXPROCS": BUILD_GOMAXPROCS,
        }
        commands: list[list[str]] = []
        digests: dict[str, str] = {}
        for module, binary in PRODUCT_BINARIES:
            command = ["go", "build", "-ldflags", ldflags(commit), "-o", str(binaries / binary), f"./cmd/{binary}"]
            _run(command, env=environment, cwd=source / module)
            commands.append(command)
            digests[binary] = sha256_file(binaries / binary)
        shutil.rmtree(source, ignore_errors=True)
        document: JsonObject = {
            "ref": ref,
            "commit": commit,
            "goos": "linux",
            "goarch": arch,
            "go_version": _run(["go", "version"]),
            "commands": [list(command) for command in commands],
            "sha256": dict(digests),
            "built_at": datetime.now(UTC).isoformat(),
        }
        (self.product_root / "build.json").write_text(encode(document), encoding="utf-8")
        return document


@dataclass(frozen=True)
class ImageBuild:
    """PREREG 2: both source images resolved to digests, then the image built from those digests."""

    context: Path
    record: PurePosixPath
    socket: str
    tag: str = IMAGE_NAME

    def _docker(self, *arguments: str) -> str:
        return _run(["docker", *arguments], env={**os.environ, "DOCKER_HOST": self.socket})

    def digest_of(self, image: str) -> str:
        self._docker("pull", "--quiet", image)
        return self._docker("image", "inspect", "--format", "{{index .RepoDigests 0}}", image)

    def build(self) -> JsonObject:
        go_digest = self.digest_of(GO_IMAGE)
        uv_digest = self.digest_of(UV_IMAGE)
        self._docker(
            "build",
            "--build-arg",
            f"GO_IMAGE={go_digest}",
            "--build-arg",
            f"UV_IMAGE={uv_digest}",
            "--tag",
            self.tag,
            str(self.context),
        )
        document: JsonObject = {
            "go_image": GO_IMAGE,
            "go_digest": go_digest,
            "uv_image": UV_IMAGE,
            "uv_digest": uv_digest,
            "tag": self.tag,
            "image_id": self._docker("image", "inspect", "--format", "{{.Id}}", self.tag),
            "built_at": datetime.now(UTC).isoformat(),
        }
        path = Path(self.record)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(encode(document), encoding="utf-8")
        return document
