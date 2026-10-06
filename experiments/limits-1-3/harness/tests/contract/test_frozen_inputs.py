"""Contract: the prompts and schemas that exist at the superseded tag are byte-identical in the amended protocol."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from pilot.domain.preregistration import SUPERSEDED_PREREG_TAG

EXPERIMENT = Path(__file__).resolve().parents[3]
FROZEN_DIRECTORIES = ("prereg/prompts", "prereg/schemas")


def git(*args: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(["git", *args], cwd=EXPERIMENT, capture_output=True, check=False)


def frozen_files() -> list[str]:
    listing = git("ls-tree", "-r", "--name-only", SUPERSEDED_PREREG_TAG, "--", *FROZEN_DIRECTORIES)
    if listing.returncode != 0:
        pytest.skip(f"tag {SUPERSEDED_PREREG_TAG} is not available in this clone")
    return sorted(listing.stdout.decode().split())


def test_the_frozen_prompts_and_schemas_are_listed_at_the_superseded_tag():
    names = [Path(path).name for path in frozen_files()]
    assert {"generate.md", "validity.md", "alignment.md", "probe.md", "relabel.md", "system.txt", "generate.schema.json",
            "validity.schema.json", "alignment.tmpl.schema.json", "probe.schema.json", "relabel.schema.json"} <= set(names)


@pytest.mark.parametrize("relative", [p for p in (
    "prereg/prompts/generate.md", "prereg/prompts/validity.md", "prereg/prompts/alignment.md", "prereg/prompts/probe.md",
    "prereg/prompts/relabel.md", "prereg/prompts/leak_audit.md", "prereg/prompts/system.txt",
    "prereg/schemas/generate.schema.json", "prereg/schemas/validity.schema.json", "prereg/schemas/alignment.tmpl.schema.json",
    "prereg/schemas/probe.schema.json", "prereg/schemas/relabel.schema.json", "prereg/schemas/leak_audit.schema.json",
)])
def test_each_pre_existing_input_is_byte_identical_to_the_superseded_tag(relative):
    frozen_files()
    shown = git("show", f"{SUPERSEDED_PREREG_TAG}:./{relative}")
    assert shown.returncode == 0, shown.stderr.decode()
    assert (EXPERIMENT / relative).read_bytes() == shown.stdout


def test_every_file_under_the_frozen_directories_at_the_tag_is_unchanged():
    for path in frozen_files():
        shown = git("show", f"{SUPERSEDED_PREREG_TAG}:./{path}")
        assert (EXPERIMENT / path).read_bytes() == shown.stdout, path
