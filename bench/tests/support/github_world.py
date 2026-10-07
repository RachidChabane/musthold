"""An in-memory GitHub that answers the requests `GitHubSource` makes, so mining tests never touch the network."""

from __future__ import annotations

import base64
import re
from collections.abc import Mapping
from dataclasses import dataclass, field

from specbench.adapters.gh_miner import GhNotFound, request_key
from specbench.domain.canonical import JsonValue

MIT_TEXT = (
    "MIT License\n\nCopyright (c) 2026 Example\n\nPermission is hereby granted, free of charge, to any person "
    'obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software.'
)
GPL_TEXT = "GNU GENERAL PUBLIC LICENSE Version 3, 29 June 2007"
BASE_SHA = "a" * 40
MERGE_SHA = "b" * 40


@dataclass
class WorldPull:
    number: int
    title: str = "feat: add a parser option"
    labels: tuple[str, ...] = ()
    merged_at: str = "2026-08-12T10:00:00Z"
    files: tuple[tuple[str, str, int, int], ...] = (
        ("pkg/parser.py", "modified", 40, 10),
        ("tests/test_parser.py", "modified", 25, 0),
    )


@dataclass
class WorldRepo:
    full_name: str
    language: str = "Python"
    spdx_id: str = "MIT"
    stars: int = 500
    size_kb: int = 2_000
    licence_text: str = MIT_TEXT
    root_files: tuple[str, ...] = ("LICENSE", "uv.lock", "pyproject.toml")
    workflows: tuple[str, ...] = ("ci.yml",)
    pulls: list[WorldPull] = field(default_factory=lambda: [WorldPull(1), WorldPull(2), WorldPull(3)])


class FakeGitHub:
    def __init__(self, repos: list[WorldRepo]) -> None:
        self._repos = {r.full_name: r for r in repos}
        self.requests: list[str] = []
        self.paths: list[str] = []

    def get(self, path: str, params: Mapping[str, str] | None = None) -> JsonValue:
        params = params or {}
        self.requests.append(request_key(path, params))
        self.paths.append(path)
        if path == "search/repositories":
            return self._search_repositories(params)
        if path == "search/issues":
            return self._search_issues(params)
        match = re.match(r"^repos/([^/]+/[^/]+)/(.*)$", path)
        if match is None or match.group(1) not in self._repos:
            raise GhNotFound(path)
        return self._repo_resource(self._repos[match.group(1)], match.group(2), params)

    def _search_repositories(self, params: Mapping[str, str]) -> JsonValue:
        query = params["q"]
        language = re.search(r"language:(\S+)", query)
        licence = re.search(r"license:(\S+)", query)
        assert language is not None and licence is not None
        wanted = {
            "mit": "MIT",
            "apache-2.0": "Apache-2.0",
            "bsd-2-clause": "BSD-2-Clause",
            "bsd-3-clause": "BSD-3-Clause",
            "isc": "ISC",
        }
        page = int(params["page"])
        items = [
            {
                "full_name": r.full_name,
                "stargazers_count": r.stars,
                "license": {"spdx_id": r.spdx_id},
                "default_branch": "main",
                "size": r.size_kb,
            }
            for r in self._repos.values()
            if r.language == language.group(1) and r.spdx_id == wanted[licence.group(1)]
        ]
        return {"items": items if page == 1 else []}

    def _search_issues(self, params: Mapping[str, str]) -> JsonValue:
        repo = re.search(r"repo:(\S+)", params["q"])
        assert repo is not None
        pulls = self._repos[repo.group(1)].pulls if params["page"] == "1" else []
        return {
            "items": [
                {"number": p.number, "title": p.title, "labels": [{"name": label} for label in p.labels]} for p in pulls
            ]
        }

    def _repo_resource(self, repo: WorldRepo, rest: str, params: Mapping[str, str]) -> JsonValue:
        pull = re.match(r"^pulls/(\d+)(/files)?$", rest)
        if pull is not None:
            found = next((p for p in repo.pulls if p.number == int(pull.group(1))), None)
            if found is None:
                raise GhNotFound(rest)
            if pull.group(2):
                files = found.files if params["page"] == "1" else ()
                return [{"filename": n, "status": s, "additions": a, "deletions": d} for n, s, a, d in files]
            return {
                "title": found.title,
                "labels": [{"name": label} for label in found.labels],
                "merged_at": found.merged_at,
                "merge_commit_sha": MERGE_SHA,
                "base": {"sha": BASE_SHA},
            }
        if rest in ("contents", "contents/"):
            return [{"name": name} for name in repo.root_files]
        if rest == "contents/.github/workflows":
            if not repo.workflows:
                raise GhNotFound(rest)
            return [{"name": name} for name in repo.workflows]
        name = rest.removeprefix("contents/")
        if name in repo.root_files:
            return {"content": base64.b64encode(repo.licence_text.encode()).decode()}
        raise GhNotFound(rest)
