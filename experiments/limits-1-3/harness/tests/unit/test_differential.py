from __future__ import annotations

from datetime import datetime
from decimal import Decimal

from pilot.adapters.differential import (
    call_graph,
    edited_names,
    generate_cases,
    public_signatures,
    reachable,
)
from pilot.domain.edits import Edit, EditSet

SOURCE = '''
from datetime import datetime
from decimal import Decimal


def _inner(x):
    return x


def _middle(x):
    return _inner(x)


def quote(entry: datetime, exit: datetime, *, flag: bool = False) -> Decimal:
    return _middle(1)


def format_receipt(amount: Decimal) -> str:
    return "FREE"


def unrelated():
    return 1
'''


def write_package(tmp_path):
    package = tmp_path / "pkg"
    package.mkdir()
    (package / "mod.py").write_text(SOURCE)
    return package


def test_signatures_come_from_annotations(tmp_path):
    found = public_signatures(write_package(tmp_path), ("quote", "format_receipt", "absent"))
    assert found == {
        "quote": [("entry", "datetime"), ("exit", "datetime"), ("flag", "bool")],
        "format_receipt": [("amount", "Decimal")],
    }


def test_public_functions_reach_helpers_through_the_call_graph(tmp_path):
    graph = call_graph(write_package(tmp_path))
    assert reachable(graph, "quote") >= {"quote", "_middle", "_inner"}
    assert "_inner" not in reachable(graph, "format_receipt")


def test_edited_names_include_class_and_method():
    edits = EditSet((Edit("a.py", "Meter.read", "x"), Edit("a.py", "helper", "y")))
    assert edited_names(edits) == {"Meter", "read", "helper"}


def test_generated_cases_are_deterministic_and_within_bounds():
    params = [("entry", "datetime"), ("exit", "datetime"), ("flag", "bool"), ("amount", "Decimal")]
    first, second = generate_cases(params), generate_cases(params)
    assert first == second and len(first) > 100
    for case in first:
        entry = datetime.fromisoformat(case["entry"])
        assert entry.second == 0 and 2020 <= entry.year <= 2030
        assert abs((datetime.fromisoformat(case["exit"]) - entry).total_seconds()) <= 96 * 3600
        assert isinstance(case["flag"], bool)
        assert Decimal("0") <= Decimal(case["amount"]) <= Decimal("1000")
        assert Decimal(case["amount"]).as_tuple().exponent == -2
