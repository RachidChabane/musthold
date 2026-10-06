"""Test-writer output: payload checks, the AST import allowlist, the suite module, the public API (ADR 0006)."""

from __future__ import annotations

import ast
import json
from pathlib import Path

import pytest

from pilot.domain.test_writing import (
    MAX_TESTS,
    MIN_TESTS,
    WrittenTest,
    criterion_of_suite_nodeid,
    public_api,
    static_problems,
    suite_module,
    suite_nodeid,
    suite_path,
    writer_payload_problems,
)

PREREG = Path(__file__).resolve().parents[3] / "prereg"
GOOD = WrittenTest("test_free", "from parkfee import quote\n\n\ndef test_free():\n    assert quote(1, 2) == 0\n")


def test_the_schema_bounds_match_the_constants():
    schema = json.loads((PREREG / "schemas" / "write_tests.schema.json").read_text())
    tests = schema["properties"]["tests"]
    assert (tests["minItems"], tests["maxItems"]) == (MIN_TESTS, MAX_TESTS) == (2, 5)
    assert schema["additionalProperties"] is False and tests["items"]["required"] == ["name", "source"]


def test_names_must_be_test_functions_and_unique():
    assert writer_payload_problems({"tests": [{"name": "test_a", "source": ""}, {"name": "test_b", "source": ""}]}) == []
    problems = writer_payload_problems({"tests": [{"name": "helper", "source": ""}, {"name": "test_b", "source": ""}, {"name": "test_b", "source": ""}]})
    assert len(problems) == 2


def test_a_function_with_allowed_imports_is_accepted():
    assert static_problems(GOOD) == []
    inside = WrittenTest("test_x", "def test_x():\n    import decimal\n    import pytest\n    from parkfee.tariff import quote\n")
    assert static_problems(inside) == []


@pytest.mark.parametrize("module", ["numpy", "hypothesis", "requests", "yaml"])
def test_third_party_imports_are_rejected(module):
    problems = static_problems(WrittenTest("test_x", f"import {module}\n\n\ndef test_x():\n    pass\n"))
    assert problems and "not allowed" in problems[0]


def test_an_import_nested_in_the_function_body_is_checked_too():
    assert static_problems(WrittenTest("test_x", "def test_x():\n    import numpy\n"))


def test_relative_imports_and_dotted_third_party_modules_are_rejected():
    assert static_problems(WrittenTest("test_x", "from . import x\n\n\ndef test_x():\n    pass\n"))
    assert static_problems(WrittenTest("test_x", "import os.path\nimport numpy.linalg\n\n\ndef test_x():\n    pass\n"))


def test_the_source_must_be_exactly_the_named_function_plus_imports():
    assert static_problems(WrittenTest("test_x", "x = 1\n\n\ndef test_x():\n    pass\n"))
    assert static_problems(WrittenTest("test_x", "def test_y():\n    pass\n"))
    assert static_problems(WrittenTest("test_x", "def test_x():\n    pass\n\n\ndef test_z():\n    pass\n"))
    assert static_problems(WrittenTest("test_x", "def test_x(:\n"))


def test_the_suite_module_marks_every_test_with_its_criterion_and_merges_imports():
    other = WrittenTest("test_paid", "from parkfee import quote\nimport decimal\n\n\ndef test_paid():\n    assert decimal.Decimal(1)\n")
    module = suite_module("PARK-3~1", [GOOD, other])
    tree = ast.parse(module)
    imports = [ast.unparse(n) for n in tree.body if isinstance(n, ast.Import | ast.ImportFrom)]
    assert imports.count("from parkfee import quote") == 1 and "import pytest" in imports
    decorated = [ast.unparse(d) for n in tree.body if isinstance(n, ast.FunctionDef) for d in n.decorator_list]
    assert decorated == ["pytest.mark.verifies('PARK-3~1')"] * 2


def test_suite_paths_and_node_ids_name_the_criterion():
    refs = [f"PARK-{n}~1" for n in range(1, 11)]
    assert suite_path("PARK-10~1") == "tests/test_park_10.py"
    assert suite_nodeid("PARK-3~1", "test_free") == "tests/test_park_3.py::test_free"
    assert criterion_of_suite_nodeid("tests/test_park_10.py::test_x", refs) == "PARK-10~1"
    with pytest.raises(ValueError):
        criterion_of_suite_nodeid("tests/other.py::test_x", refs)


def test_the_public_api_lists_the_package_signatures_without_bodies():
    source = {
        "src/parkfee/__init__.py": "from parkfee.tariff import quote\n",
        "src/parkfee/tariff.py": "def quote(entry, exit, *, lost_ticket=False):\n    return 1\n",
        "src/parkfee/receipt.py": "def format_receipt(amount):\n    return str(amount)\n",
        "src/parkfee/models.py": "class Breakdown:\n    total: int\n",
    }
    api = public_api(source)
    assert "def quote(entry, exit, *, lost_ticket=False):\n    ..." in api and "return" not in api
    assert "def format_receipt(amount):" in api and "class Breakdown" in api
