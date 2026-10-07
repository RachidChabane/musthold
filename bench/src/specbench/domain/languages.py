"""The two languages of the samples and the path conventions that tell tests from sources."""

from __future__ import annotations

import posixpath
from enum import Enum
from typing import Final


class Language(Enum):
    PYTHON = "python"
    GO = "go"

    @property
    def github_name(self) -> str:
        return GITHUB_LANGUAGE[self]

    @property
    def source_suffix(self) -> str:
        return SOURCE_SUFFIX[self]


GITHUB_LANGUAGE: Final = {Language.PYTHON: "Python", Language.GO: "Go"}
SOURCE_SUFFIX: Final = {Language.PYTHON: ".py", Language.GO: ".go"}

DOCUMENTATION_SUFFIXES: Final = (".md", ".rst", ".txt", ".adoc")
DOCUMENTATION_DIRECTORIES: Final = ("docs", "doc", "documentation")
PYTHON_TEST_DIRECTORIES: Final = ("tests", "test")
PYTHON_TEST_PREFIX: Final = "test_"
PYTHON_TEST_SUFFIX: Final = "_test.py"
PYTHON_CONFTEST: Final = "conftest.py"
GO_TEST_SUFFIX: Final = "_test.go"
GO_TEST_DATA_DIRECTORY: Final = "testdata"


def is_test_path(language: Language, path: str) -> bool:
    """Whether a repository-relative path is part of the language's test code."""
    parts = path.split("/")
    name = posixpath.basename(path)
    if language is Language.PYTHON:
        return (
            any(part in PYTHON_TEST_DIRECTORIES for part in parts[:-1])
            or name.startswith(PYTHON_TEST_PREFIX)
            or name.endswith(PYTHON_TEST_SUFFIX)
            or name == PYTHON_CONFTEST
        )
    return name.endswith(GO_TEST_SUFFIX) or GO_TEST_DATA_DIRECTORY in parts[:-1]


def is_documentation_path(path: str) -> bool:
    parts = path.split("/")
    return path.lower().endswith(DOCUMENTATION_SUFFIXES) or parts[0] in DOCUMENTATION_DIRECTORIES


def is_non_test_source_path(language: Language, path: str) -> bool:
    """A file the size filter counts: a source file of the language that is neither a test nor documentation."""
    return (
        path.endswith(language.source_suffix) and not is_test_path(language, path) and not is_documentation_path(path)
    )
