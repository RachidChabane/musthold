"""A tiny upstream repository with a base and a merge commit, and the sealed bundle that verifies against it."""

from __future__ import annotations

import difflib
import os
import subprocess
from datetime import date
from pathlib import Path

from specbench.domain.canonical import canonical_json
from specbench.domain.languages import Language
from specbench.domain.sealing import SealedBundle, versioned_bundle
from specbench.domain.task import Criterion, CriterionStatus, Task, Toolchain

TASK = "ACME-PARKFEE-12"
BASE_FEE = "def fee(minutes):\n    return 5\n"
MERGE_FEE = "def fee(minutes):\n    if minutes < 10:\n        return 0\n    return 5\n"
PLAIN_H = b"from pkg.fee import fee\n\n\ndef test_h1():\n    assert fee(9) == 0 and fee(10) == 5\n"
PARAMETRIZED_H = (
    b"import pytest\n\nfrom pkg.fee import fee\n\n\n"
    b'@pytest.mark.parametrize("minutes, expected", [(9, 0), (10, 5)])\n'
    b"def test_h1(minutes, expected):\n    assert fee(minutes) == expected\n"
)
GIT_ENV = {
    "GIT_AUTHOR_NAME": "t",
    "GIT_AUTHOR_EMAIL": "t@example.com",
    "GIT_COMMITTER_NAME": "t",
    "GIT_COMMITTER_EMAIL": "t@example.com",
    "GIT_CONFIG_GLOBAL": os.devnull,
    "GIT_CONFIG_SYSTEM": os.devnull,
}


def git(repo: Path, *arguments: str) -> str:
    done = subprocess.run(
        ["git", "-C", str(repo), *arguments], capture_output=True, text=True, check=True, env={**os.environ, **GIT_ENV}
    )
    return done.stdout.strip()


def patch(path: str, old: str, new: str) -> bytes:
    return "".join(difflib.unified_diff(old.splitlines(True), new.splitlines(True), f"a/{path}", f"b/{path}")).encode()


def commit_all(repo: Path, message: str) -> str:
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", message)
    return git(repo, "rev-parse", "HEAD")


def bundle(violation_one: str, h_test: bytes = PLAIN_H) -> SealedBundle:
    tests = [
        {"id": "tests/test_u.py::test_u1", "part": "U", "criterion": f"{TASK}-1"},
        {"id": "tests/test_r.py::test_r1", "part": "R", "criterion": None},
        {"id": "tests/test_h.py::test_h1", "part": "H", "criterion": f"{TASK}-2"},
        {"id": "tests/test_o.py::test_o1", "part": "O", "criterion": None},
    ]
    variants = {
        "V1": violation_one,
        "V2": "    if minutes < 10:\n        return 2 if minutes == 5 else 0\n    return 5\n",
        "V3": "    if minutes < 9:\n        return 0\n    return 5\n",
        "V4": "    if minutes < 11:\n        return 0\n    return 5\n",
    }
    criterion_of = {"V1": 1, "V2": 1, "V3": 2, "V4": 2}
    files: dict[str, bytes] = {
        "SPEC.md": b"# Spec\n",
        "oracle/manifest.json": canonical_json(tests).encode(),  # type: ignore[arg-type]
        "oracle/U/tests/test_u.py": b"from pkg.fee import fee\n\n\ndef test_u1():\n    assert fee(5) == 0\n",
        "oracle/H/tests/test_h.py": h_test,
        "oracle/O/tests/test_o.py": b"from pkg.fee import fee\n\n\ndef test_o1():\n    assert isinstance(fee(1), int)\n",
    }
    index: list[dict[str, str]] = []
    for name, body in variants.items():
        files[f"violations/{name}.patch"] = patch("pkg/fee.py", MERGE_FEE, "def fee(minutes):\n" + body)
        index.append({"id": name, "criterion": f"{TASK}-{criterion_of[name]}", "patch": f"violations/{name}.patch"})
    files["violations/index.json"] = canonical_json(index).encode()  # type: ignore[arg-type]
    planted = {
        "P1": "def fee(minutes):\n    return 0 if minutes < 10 else 5\n",
        "P2": "FREE_MINUTES = 10\n\n\ndef fee(minutes):\n    if minutes < FREE_MINUTES:\n        return 0\n    return 5\n",
    }
    for name, content in planted.items():
        files[f"planted/{name}.patch"] = patch("pkg/fee.py", MERGE_FEE, content)
    files["planted/index.json"] = canonical_json([{"id": n, "patch": f"planted/{n}.patch"} for n in planted]).encode()  # type: ignore[arg-type]
    return versioned_bundle(files)


def task(base: str, merge: str) -> Task:
    return Task(
        task_id=TASK,
        language=Language.PYTHON,
        repository="acme/parkfee",
        pull_number=12,
        merged_on=date(2026, 8, 3),
        spdx_id="MIT",
        licence_path="LICENSE",
        notice_path=None,
        base_sha=base,
        merge_sha=merge,
        lockfile_digests={},
        toolchain=Toolchain("python", "3.13"),
        criteria=(Criterion(f"{TASK}-1", CriterionStatus.NEW), Criterion(f"{TASK}-2", CriterionStatus.NEW)),
    )


def make_upstream(root: Path) -> tuple[Path, str, str]:
    repo = root / "upstream"
    (repo / "pkg").mkdir(parents=True)
    (repo / "tests").mkdir()
    (repo / "pkg" / "__init__.py").write_text("")
    (repo / "pkg" / "fee.py").write_text(BASE_FEE)
    (repo / "tests" / "test_r.py").write_text("from pkg.fee import fee\n\n\ndef test_r1():\n    assert fee(60) == 5\n")
    git(repo, "init", "-q", "-b", "main")
    base = commit_all(repo, "base")
    (repo / "pkg" / "fee.py").write_text(MERGE_FEE)
    merge = commit_all(repo, "merge")
    return repo, base, merge


GOOD_V1 = "    if minutes < 10:\n        return 1 if minutes == 5 else 0\n    return 5\n"
STRAY_V1 = "    if minutes < 10:\n        return 1\n    return 5\n"
