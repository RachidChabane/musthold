from __future__ import annotations

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.go_source import GoImport, go_imports

import_paths = st.from_regex(r"[a-z][a-z0-9]{0,6}(\.[a-z]{2,3})?(/[a-z][a-z0-9]{0,6}){0,3}", fullmatch=True)
aliases = st.one_of(st.none(), st.sampled_from([".", "_"]), st.from_regex(r"[a-z][a-z0-9]{0,5}", fullmatch=True))
specs = st.builds(GoImport, import_paths, aliases)
literals = st.text(alphabet=st.characters(blacklist_characters='"\\\n\r`', blacklist_categories=("Cs",)), max_size=20)


def rendered(spec: GoImport) -> str:
    alias = f"{spec.alias} " if spec.alias else ""
    return f'{alias}"{spec.path}"'


def declarations_of(found: list[GoImport], grouped: bool) -> str:
    if grouped and found:
        return "import (\n" + "".join(f"\t{rendered(s)}\n" for s in found) + ")\n"
    return "".join(f"import {rendered(s)}\n" for s in found)


@given(st.lists(specs, max_size=6), st.booleans(), st.lists(literals, max_size=5))
def test_imports_round_trip_and_string_literals_after_them_never_add_any(
    found: list[GoImport], grouped: bool, literal_values: list[str]
) -> None:
    body = "func f() {\n" + "".join(f'\t_ = "{v}"\n\t_ = `{v}`\n\treturn "."\n' for v in literal_values) + "}\n"
    assert go_imports(f"package p\n\n{declarations_of(found, grouped)}\n{body}") == found
