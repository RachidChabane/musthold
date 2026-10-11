#!/usr/bin/env python3
"""Writes the recorded Shallnot reports (schema 1.2) the end-to-end tests read.

The parcel report describes `parcel/` (pytest) with the tests at the lines they
have in `parcel/tests/test_rates.py`; the Go, Jest and Vitest reports are derived
from the bindings recorded for their ecosystem fixtures and the examples of their
specs. Run it from anywhere:

    python3 discpower/fixtures/prove/generate_reports.py
"""
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
BINDINGS_DIR = HERE.parent.parent / "internal" / "testkit" / "bindings" / "testdata"
ECOSYSTEMS_DIR = HERE.parent / "ecosystems"
SPEC_FILE = "specs/parcel.md"
SCHEMA_VERSION = "1.2"
TOOL = {"name": "shallnot", "version": "v0.5.0"}

STATEMENTS = {
    "PAR-1": "WHEN an order has line items THE SYSTEM SHALL total each line as its unit price times its quantity.",
    "PAR-2": "WHEN a discount percentage is applied THE SYSTEM SHALL reduce the total by that percentage, limited to the range 0 to 100.",
    "PAR-3": "WHEN an order total reaches 50 THE SYSTEM SHALL waive the shipping fee.",
}


def sha256(text):
    return hashlib.sha256(text.encode()).hexdigest()


def pytest_test(name, classname, line):
    return {"name": name, "classname": classname, "suite": "pytest", "outcome": "passed",
            "source": {"file": "tests/test_rates.py", "line": line}}


def example(index, operation, args, value):
    return {"index": index, "operation": operation,
            "args": {name: {"type": "integer", "value": text} for name, text in args.items()},
            "expect": {"kind": "returns", "type": "integer", "value": value}}


def location(identifier, spec):
    """The spec file of a requirement and the line that declares it."""
    lines = spec.read_text().splitlines()
    line = next(index for index, text in enumerate(lines, start=1) if f"**{identifier}~" in text)
    return {"file": SPEC_FILE, "line": line}


def requirement(identifier, tests, examples, spec):
    statement = STATEMENTS[identifier]
    return {"id": identifier, "revision": 1, "ref": identifier + "~1", "statement": statement,
            "status": "active", "statement_digest": sha256(statement), "location": location(identifier, spec),
            "tests": tests, "examples": examples}


def report(requirements):
    return {"schema_version": SCHEMA_VERSION, "tool": TOOL, "requirements": requirements,
            "findings": [], "extensions": {}}


def parcel_report():
    spec = HERE / "parcel" / SPEC_FILE
    module = "tests.test_rates"
    limits = module + ".TestDiscountLimits"
    return report([
        requirement("PAR-1", [pytest_test("test_line_total_multiplies_price_by_quantity", module, 7)],
                    [example(0, "line_total", {"unit_price": "4", "quantity": "3"}, "12")], spec),
        requirement("PAR-2", [
            pytest_test("test_discount_reduces_total[0-19.99-19.99]", module, 17),
            pytest_test("test_discount_reduces_total[10-200-180.0]", module, 17),
            pytest_test("test_discount_reduces_total[50-80-40.0]", module, 17),
            pytest_test("test_negative_percent_is_ignored", limits, 26),
            pytest_test("test_percent_above_one_hundred_is_clamped", limits, 23),
        ], [
            example(0, "discounted", {"total": "200", "percent": "10"}, "180"),
            example(1, "discounted", {"total": "80", "percent": "150"}, "0"),
        ], spec),
        requirement("PAR-3", [pytest_test("test_large_orders_ship_free", module, 31),
                              pytest_test("test_small_orders_pay_the_fee", module, 36)], [], spec),
    ])


GO_EXAMPLES = {
    "PAR-1": [example(0, "line_total", {"unit_price": "4", "quantity": "3"}, "12")],
    "PAR-2": [example(0, "discounted", {"total": "20000", "percent": "10"}, "18000"),
              example(1, "discounted", {"total": "8000", "percent": "150"}, "0")],
    "PAR-3": [],
}


NODE_EXAMPLES = {
    "PAR-1": [example(0, "line_total", {"unit_price": "4", "quantity": "3"}, "12")],
    "PAR-2": [example(0, "discounted", {"total": "200", "percent": "10"}, "180"),
              example(1, "discounted", {"total": "80", "percent": "150"}, "0")],
    "PAR-3": [],
}


def bound_report(suite, bindings, examples):
    spec = ECOSYSTEMS_DIR / suite / SPEC_FILE
    bound = json.loads((BINDINGS_DIR / bindings).read_text())
    requirements = []
    for ref, tests in sorted(bound.items()):
        identifier = ref.split("~")[0]
        converted = [{"name": t["name"], "classname": t["classname"], "suite": suite, "outcome": "passed",
                      "source": {"file": t["file"], "line": t["line"]}} for t in tests]
        requirements.append(requirement(identifier, converted, examples[identifier], spec))
    return report(requirements)


def write(name, document):
    (HERE / name).write_text(json.dumps(document, indent=2) + "\n")


if __name__ == "__main__":
    write("parcel-report.json", parcel_report())
    write("go-report.json", bound_report("go", "go-bound-tests.json", GO_EXAMPLES))
    write("jest-report.json", bound_report("jest", "jest-bound-tests.json", NODE_EXAMPLES))
    write("vitest-report.json", bound_report("vitest", "vitest-bound-tests.json", NODE_EXAMPLES))
