from __future__ import annotations

from specbench.adapters.prompt_files import load_prompts
from specbench.application.construction_requests import h_refill_request, h_writer_request
from specbench.domain.canonical import JsonValue
from specbench.domain.go_target import GoTarget
from specbench.domain.languages import Language

MODULE = "example.com/acme/routedns"
TARGET = GoTarget(MODULE, {".": "rdns"}, frozenset())
SCHEMAS: dict[str, JsonValue] = {"h-tests": {"type": "object"}}
SPEC = "## Interface\n\n- `Match(*dns.Msg) bool`"


def test_the_go_writer_is_told_the_package_name_for_the_test_package_line_and_the_import_alias() -> None:
    request = h_writer_request(load_prompts(), SCHEMAS, Language.GO, SPEC, "", "t:h-0", TARGET)
    assert f'declare `package rdns_test` in its files and import it as `rdns "{MODULE}"`' in request.system_prompt
    assert "$" not in request.system_prompt


def test_a_refill_writer_is_told_the_same_package_names() -> None:
    request = h_refill_request(load_prompts(), SCHEMAS, Language.GO, SPEC, (2,), 1, "", "t:h-r1-0", TARGET)
    assert "declare `package rdns_test`" in request.system_prompt


def test_the_python_writer_prompt_carries_no_go_package_facts() -> None:
    request = h_writer_request(load_prompts(), SCHEMAS, Language.PYTHON, SPEC, "", "t:h-0", GoTarget())
    assert "_test" not in request.system_prompt
