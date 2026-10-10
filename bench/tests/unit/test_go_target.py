from __future__ import annotations

from specbench.domain.go_target import GoTarget, direct_requirements, go_directory_of, go_target_of
from specbench.domain.languages import Language

MODULE = "example.com/acme/routedns"
GO_MOD = f"""module {MODULE}

go 1.21

require (
	example.com/miekg/dns v1.1.58
	example.com/other/lib v1.0.0
	example.org/x/net v0.20.0 // indirect
)

require example.com/single/dep v0.1.0
require example.com/indirect/dep v0.1.0 // indirect
"""
SOURCES = {
    "rule.go": 'package rdns\n\nimport (\n\t"example.com/miekg/dns"\n\t"example.com/other/lib"\n)\n\nvar _ = lib.X\n',
    "sub/sub.go": "package inner\n",
    "sub/sub_test.go": "package ignored\n",
}


def test_direct_requirements_skip_indirect_ones_in_blocks_and_single_lines() -> None:
    assert direct_requirements(GO_MOD) == {
        "example.com/miekg/dns",
        "example.com/other/lib",
        "example.com/single/dep",
    }


def test_the_directory_of_an_import_path_is_relative_to_the_module() -> None:
    assert go_directory_of(MODULE, MODULE) == "."
    assert go_directory_of(f"{MODULE}/sub", MODULE) == "sub"
    assert go_directory_of(f"{MODULE}x/sub", MODULE) is None


def test_the_target_names_each_package_and_the_third_party_modules_the_signatures_use() -> None:
    entries = [(MODULE, "func (r *Rule) Match(msg *dns.Msg) bool"), (f"{MODULE}/sub", "func New() *Thing")]
    target = go_target_of(Language.GO, MODULE, entries, SOURCES, GO_MOD)
    assert target.package_by_directory == {".": "rdns", "sub": "inner"}
    assert target.third_party_modules == {"example.com/miekg/dns"}
    assert target.allows_third_party("example.com/miekg/dns") is True
    assert target.allows_third_party("example.com/miekg/dns/dnsutil") is True
    assert target.allows_third_party("example.com/other/lib") is False


def test_a_requirement_named_by_its_import_path_in_a_signature_is_allowed() -> None:
    entries = [(MODULE, "func Use(example.com/single/dep.T)")]
    target = go_target_of(Language.GO, MODULE, entries, {}, GO_MOD)
    assert target.third_party_modules == {"example.com/single/dep"}


def test_disagreeing_package_clauses_leave_the_directory_unnamed_and_python_has_no_target() -> None:
    sources = {"a.go": "package one\n", "b.go": "package two\n"}
    assert go_target_of(Language.GO, MODULE, [(MODULE, "func F()")], sources, "").package_by_directory == {}
    assert go_target_of(Language.PYTHON, "", [("pkg", "def f()")], {"a.go": "package one\n"}, GO_MOD) == GoTarget()


def test_the_statement_gives_the_external_test_package_line_and_the_import_alias() -> None:
    target = GoTarget(MODULE, {".": "rdns", "sub": "inner"})
    assert target.statement().splitlines() == [
        f"- Package `rdns`, import path `{MODULE}`: declare `package rdns_test` in its files and import it as "
        f'`rdns "{MODULE}"`.',
        f"- Package `inner`, import path `{MODULE}/sub`: declare `package inner_test` in its files and import it as "
        f'`inner "{MODULE}/sub"`.',
    ]
    assert GoTarget().statement() == ""
