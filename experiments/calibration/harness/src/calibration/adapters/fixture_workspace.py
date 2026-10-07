"""`FixtureWorkspace`: a copy of the pilot fixture with the `shallnot.yaml` its gate needs."""

from __future__ import annotations

import shutil
from pathlib import Path
from typing import Final

import yaml

from calibration.domain.constants import ENCODING

CONFIG_FILE: Final = "shallnot.yaml"
SPECS_DIR: Final = "spec"
TESTS_DIR: Final = "tests"
JUNIT_FILE: Final = "junit.xml"
CONFIG_VERSION: Final = 1
TEST_TIMEOUT_SECONDS: Final = 10
IGNORED_NAMES: Final = ("__pycache__", ".pytest_cache", "*.pyc", "junit*.xml")


def config_text(python: str) -> str:
    command = f"{python} -m pytest -p no:cacheprovider --timeout={TEST_TIMEOUT_SECONDS} --junitxml={JUNIT_FILE}"
    document = {
        "version": CONFIG_VERSION,
        "specs": [SPECS_DIR],
        "focus": [SPECS_DIR],
        "tests": [TESTS_DIR],
        "results": [JUNIT_FILE],
        "test_commands": [command],
    }
    return yaml.safe_dump(document, sort_keys=False)


class PytestFixtureWorkspace:
    """Implements `FixtureWorkspace`; `python` is the interpreter the gate runs pytest with."""

    def __init__(self, python: str) -> None:
        self._python = python

    def materialize(self, fixture_dir: Path, destination: Path) -> Path:
        shutil.copytree(fixture_dir, destination, ignore=shutil.ignore_patterns(*IGNORED_NAMES))
        (destination / CONFIG_FILE).write_text(config_text(self._python), encoding=ENCODING)
        return destination
