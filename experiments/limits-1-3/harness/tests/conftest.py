from __future__ import annotations

from pathlib import Path

import pytest

TESTS_ROOT = Path(__file__).parent
EXPERIMENT_ROOT = TESTS_ROOT.parents[1]

collect_ignore = ["fixtures"]
MINI_ROOT = TESTS_ROOT / "fixtures" / "mini"
RESPONSES = TESTS_ROOT / "fixtures" / "model_responses"


@pytest.fixture
def mini_root() -> Path:
    return MINI_ROOT


@pytest.fixture
def responses() -> Path:
    return RESPONSES


@pytest.fixture
def experiment_root() -> Path:
    return EXPERIMENT_ROOT
