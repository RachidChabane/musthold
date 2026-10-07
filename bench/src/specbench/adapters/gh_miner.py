"""GitHub access for mining: `gh api` with every response archived, replay from the archive, and the `MiningSource` that reads it."""

from __future__ import annotations

import base64
import json
from collections.abc import Mapping
from datetime import date
from pathlib import Path
from typing import Final, cast

from specbench.application.ports import GhClient, ProcessRunner
from specbench.domain import licence
from specbench.domain.canonical import JsonValue, canonical_json, sha256_hex, stable_json
from specbench.domain.jsonread import integer, mapping, optional_text, sequence, text
from specbench.domain.languages import GITHUB_LANGUAGE, Language
from specbench.domain.mining import (
    MERGE_WINDOW_START,
    MIN_STARS,
    WORKFLOW_DIRECTORY,
    BaseSnapshot,
    ChangedFile,
    PullRequest,
    Repository,
)

PER_PAGE: Final = 100
SEARCH_PAGES: Final = 2
FILE_PAGES: Final = 3
STATUS_OK: Final = 200
STATUS_NOT_FOUND: Final = 404
NOT_FOUND_MARKER: Final = "HTTP 404"
RESPONSES_DIRECTORY: Final = "responses"
REQUEST_KEY_LENGTH: Final = 32


class GhError(RuntimeError):
    """`gh api` failed for a reason other than a missing resource."""


class GhNotFound(GhError):
    """The requested resource does not exist (HTTP 404)."""


class MissingRecording(GhError):
    """A dry run asked for a response that was never archived."""


def request_key(path: str, params: Mapping[str, str] | None) -> str:
    request: dict[str, JsonValue] = {"params": dict(sorted((params or {}).items())), "path": path}
    return sha256_hex(canonical_json(request).encode())[:REQUEST_KEY_LENGTH]


class GhCliClient:
    """`gh api --method GET`: the only code that reaches the network."""

    def __init__(self, runner: ProcessRunner) -> None:
        self._runner = runner

    def get(self, path: str, params: Mapping[str, str] | None = None) -> JsonValue:
        argv = ["gh", "api", "--method", "GET", path]
        for name, value in sorted((params or {}).items()):
            argv += ["-f", f"{name}={value}"]
        result = self._runner.run(argv)
        if result.returncode != 0:
            if NOT_FOUND_MARKER in result.stderr:
                raise GhNotFound(path)
            raise GhError(f"gh api {path} failed: {result.stderr.strip()}")
        return json.loads(result.stdout)


class RecordingGhClient:
    """Decorator that archives every response, 404 included, under `<archive>/responses/<request key>.json`."""

    def __init__(self, inner: GhClient, archive: Path) -> None:
        self._inner = inner
        self._directory = archive / RESPONSES_DIRECTORY

    def get(self, path: str, params: Mapping[str, str] | None = None) -> JsonValue:
        self._directory.mkdir(parents=True, exist_ok=True)
        status, response = STATUS_OK, None
        try:
            response = self._inner.get(path, params)
        except GhNotFound:
            status = STATUS_NOT_FOUND
        document: dict[str, JsonValue] = {
            "params": dict(sorted((params or {}).items())),
            "path": path,
            "response": response,
            "status": status,
        }
        (self._directory / f"{request_key(path, params)}.json").write_text(stable_json(document), encoding="utf-8")
        if status == STATUS_NOT_FOUND:
            raise GhNotFound(path)
        return response


class ReplayGhClient:
    """Serves archived responses; a request that was never archived is an error, never a network call."""

    def __init__(self, archive: Path) -> None:
        self._directory = archive / RESPONSES_DIRECTORY

    def get(self, path: str, params: Mapping[str, str] | None = None) -> JsonValue:
        file = self._directory / f"{request_key(path, params)}.json"
        if not file.is_file():
            raise MissingRecording(f"no recording for {path} {dict(params or {})}")
        document = mapping(json.loads(file.read_text(encoding="utf-8")), str(file))
        if document.get("status") == STATUS_NOT_FOUND:
            raise GhNotFound(path)
        return cast(JsonValue, document["response"])


def _search_query(language: Language, licence_key: str) -> str:
    return (
        f"language:{GITHUB_LANGUAGE[language]} license:{licence_key} stars:>={MIN_STARS} "
        f"fork:false archived:false pushed:>={MERGE_WINDOW_START.isoformat()}"
    )


def _repository(item: object, language: Language) -> Repository:
    body = mapping(item, "repository")
    license_body = body.get("license")
    spdx = None if license_body is None else optional_text(mapping(license_body, "license").get("spdx_id"), "spdx_id")
    return Repository(
        full_name=text(body.get("full_name"), "full_name"),
        language=language,
        stars=integer(body.get("stargazers_count"), "stargazers_count"),
        spdx_id=spdx,
        default_branch=text(body.get("default_branch"), "default_branch"),
    )


class GitHubSource:
    """`MiningSource` over a `GhClient`: decodes GitHub payloads into the facts the filters judge."""

    def __init__(self, client: GhClient) -> None:
        self._client = client

    def repositories(self, language: Language) -> list[Repository]:
        found: dict[str, Repository] = {}
        for licence_key in licence.SEARCH_LICENCE_KEYS:
            for page in range(1, SEARCH_PAGES + 1):
                response = self._client.get(
                    "search/repositories",
                    {
                        "q": _search_query(language, licence_key),
                        "sort": "stars",
                        "order": "desc",
                        "per_page": str(PER_PAGE),
                        "page": str(page),
                    },
                )
                items = sequence(mapping(response, "search").get("items"), "items")
                for item in items:
                    repository = _repository(item, language)
                    found.setdefault(repository.full_name, repository)
                if len(items) < PER_PAGE:
                    break
        return list(found.values())

    def merged_pull_numbers(self, repository: Repository, since: date) -> list[int]:
        numbers: list[int] = []
        for page in range(1, SEARCH_PAGES + 1):
            response = self._client.get(
                "search/issues",
                {
                    "q": f"repo:{repository.full_name} is:pr is:merged merged:>={since.isoformat()}",
                    "per_page": str(PER_PAGE),
                    "page": str(page),
                },
            )
            items = sequence(mapping(response, "search").get("items"), "items")
            numbers += [integer(mapping(i, "issue").get("number"), "number", 1) for i in items]
            if len(items) < PER_PAGE:
                break
        return numbers

    def _changed_files(self, base_path: str) -> list[object]:
        files: list[object] = []
        for page in range(1, FILE_PAGES + 1):
            batch = sequence(
                self._client.get(f"{base_path}/files", {"per_page": str(PER_PAGE), "page": str(page)}), "files"
            )
            files += batch
            if len(batch) < PER_PAGE:
                break
        return files

    def pull_request(self, repository: Repository, number: int) -> PullRequest | None:
        base_path = f"repos/{repository.full_name}/pulls/{number}"
        try:
            detail = mapping(self._client.get(base_path), "pull request")
            files = self._changed_files(base_path)
        except GhNotFound:
            return None
        merged_at, merge_sha = detail.get("merged_at"), detail.get("merge_commit_sha")
        if not isinstance(merged_at, str) or not isinstance(merge_sha, str):
            return None
        base = mapping(detail.get("base"), "base")
        labels = frozenset(
            text(mapping(label, "label").get("name"), "label name").lower()
            for label in sequence(detail.get("labels", []), "labels")
        )
        return PullRequest(
            number=number,
            title=text(detail.get("title"), "title"),
            labels=labels,
            merged_on=date.fromisoformat(merged_at[:10]),
            base_sha=text(base.get("sha"), "base.sha"),
            merge_sha=merge_sha,
            files=tuple(
                ChangedFile(
                    path=text(mapping(f, "file").get("filename"), "filename"),
                    status=text(mapping(f, "file").get("status"), "status"),
                    additions=integer(mapping(f, "file").get("additions"), "additions"),
                    deletions=integer(mapping(f, "file").get("deletions"), "deletions"),
                )
                for f in files
            ),
        )

    def _listing(self, repository: Repository, path: str, base_sha: str) -> frozenset[str]:
        try:
            entries = sequence(
                self._client.get(f"repos/{repository.full_name}/contents/{path}".rstrip("/"), {"ref": base_sha}),
                "contents",
            )
        except GhNotFound:
            return frozenset()
        return frozenset(text(mapping(e, "entry").get("name"), "entry name") for e in entries)

    def _file_text(self, repository: Repository, name: str, base_sha: str) -> str:
        document = mapping(self._client.get(f"repos/{repository.full_name}/contents/{name}", {"ref": base_sha}), "file")
        content = text(document.get("content"), "content")
        return base64.b64decode(content).decode("utf-8", errors="replace")

    def base_snapshot(self, repository: Repository, base_sha: str) -> BaseSnapshot:
        root = self._listing(repository, "", base_sha)
        licence_path = next((name for name in licence.LICENCE_FILE_NAMES if name in root), None)
        licence_spdx = (
            None
            if licence_path is None
            else licence.classify_licence_text(self._file_text(repository, licence_path, base_sha))
        )
        return BaseSnapshot(
            root_files=root,
            workflow_files=self._listing(repository, WORKFLOW_DIRECTORY, base_sha),
            licence_spdx=licence_spdx,
            licence_path=licence_path,
            notice_path=next((name for name in licence.NOTICE_FILE_NAMES if name in root), None),
        )
