from __future__ import annotations

import pytest

from specbench.domain.go_source import GoImport, go_imports, go_package_name

BODY_WITH_PATH_STRINGS = (
    'func normalise(name string) string {\n\tif name == "" {\n\t\treturn "."\n\t}\n\treturn "example.com/x/y"\n}\n'
    '\nvar quoted = `import "fake.example/raw"`\n'
)


def test_a_string_literal_that_looks_like_a_path_is_not_an_import() -> None:
    source = f'package rdns_test\n\nimport "testing"\n\n{BODY_WITH_PATH_STRINGS}'
    assert go_imports(source) == [GoImport("testing")]


def test_single_grouped_aliased_dot_and_blank_imports_are_all_read() -> None:
    source = (
        'package p\n\nimport "fmt"\nimport rdns "example.com/acme/routedns"\n'
        'import (\n\t"testing"\n\n\tdns "example.com/miekg/dns" // the wire format\n\t. "example.com/dot"\n'
        '\t_ "example.com/blank"\n\t/* c */ "example.com/plain"\n)\n\nfunc F() {}\n'
    )
    assert go_imports(source) == [
        GoImport("fmt"),
        GoImport("example.com/acme/routedns", "rdns"),
        GoImport("testing"),
        GoImport("example.com/miekg/dns", "dns"),
        GoImport("example.com/dot", "."),
        GoImport("example.com/blank", "_"),
        GoImport("example.com/plain"),
    ]


def test_an_import_looking_line_after_the_declarations_is_ignored() -> None:
    source = 'package p\n\nimport "a"\n\nfunc F() {\n\tx := 1\n}\n\nimport "b"\n'
    assert go_imports(source) == [GoImport("a")]


def test_the_qualifier_is_the_alias_else_the_last_path_element() -> None:
    assert GoImport("example.com/acme/routedns", "rdns").qualifier == "rdns"
    assert GoImport("example.com/acme/routedns").qualifier == "routedns"


@pytest.mark.parametrize(
    ("source", "expected"),
    [
        ("package rdns\n", "rdns"),
        ("// Package rdns does things.\n/* package other */\npackage rdns // trailing\n", "rdns"),
        ("func f() {}\n", None),
        ("", None),
    ],
)
def test_the_package_clause_is_read_past_comments(source: str, expected: str | None) -> None:
    assert go_package_name(source) == expected
