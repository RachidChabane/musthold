from __future__ import annotations

import pytest

from specbench.domain.toolchain import declared_requirement, pinned_python, python_for, satisfies


@pytest.mark.parametrize(
    ("version_file", "requires", "expected"),
    [
        ("3.12\n", ">=3.9", (3, 12)),
        ("3.11.4\n", None, (3, 11)),
        ("# comment\n3.10\n", ">=3.9", (3, 10)),
        ("pypy3.10\n", ">=3.9", (3, 13)),
        (None, None, (3, 13)),
        (None, ">=3.11", (3, 13)),
        (None, ">=3.9,<3.13", (3, 9)),
        (None, ">=3.10,<3.12", (3, 10)),
        (None, "~=3.11", (3, 13)),
        (None, "==3.11.*", (3, 11)),
        (None, ">3.12", (3, 13)),
        (None, "<3.9", (3, 13)),
    ],
)
def test_the_pin_wins_then_the_default_then_the_oldest_compatible(
    version_file: str | None, requires: str | None, expected: tuple[int, int]
) -> None:
    assert python_for(version_file, requires) == expected


def test_specifiers_compare_at_minor_granularity() -> None:
    assert satisfies((3, 11), ">=3.11") and not satisfies((3, 10), ">=3.11")
    assert satisfies((3, 12), "!=3.11") and not satisfies((3, 11), "!=3.11")
    assert pinned_python("\n\n3.9.1") == (3, 9)


def test_the_requirement_is_read_from_the_lock_before_the_project_metadata() -> None:
    lock = 'version = 1\nrequires-python = ">=3.12"\n'
    project = '[project]\nrequires-python = ">=3.9"\n'
    assert declared_requirement(lock, project) == ">=3.12"
    assert declared_requirement(None, project) == ">=3.9"
    assert declared_requirement(None, None) is None
