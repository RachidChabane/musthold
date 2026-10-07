from __future__ import annotations

from datetime import date

from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.domain.canonical import JsonValue, canonical_json, parse_canonical, sha256_hex
from specbench.domain.languages import Language
from specbench.domain.sealing import SealedBundle, pack_bundle, sealed_leaks, unpack_bundle
from specbench.domain.task import Criterion, CriterionStatus, Task, Toolchain, parse_task
from specbench.domain.workspace import WorkspaceLeak, build_workspace, protected_files

json_leaves = st.none() | st.booleans() | st.integers(min_value=-(2**53), max_value=2**53) | st.text(max_size=12)
json_values = st.recursive(
    json_leaves,
    lambda inner: st.lists(inner, max_size=4) | st.dictionaries(st.text(max_size=6), inner, max_size=4),
    max_leaves=12,
)
safe_segment = st.from_regex(r"[a-z][a-z0-9_]{0,6}", fullmatch=True)
safe_paths: st.SearchStrategy[str] = st.lists(safe_segment, min_size=1, max_size=3).map(lambda parts: "/".join(parts))  # noqa: PLW0108
file_contents = st.binary(max_size=40)
file_trees = st.dictionaries(safe_paths, file_contents, min_size=0, max_size=6)
bundles = st.dictionaries(safe_paths, file_contents, min_size=1, max_size=6).map(
    lambda files: SealedBundle(dict(files))
)


@given(json_values)
def test_canonical_text_round_trips_and_is_idempotent(value: JsonValue) -> None:
    text = canonical_json(value)
    assert parse_canonical(text) == value
    assert canonical_json(parse_canonical(text)) == text


@given(bundles)
def test_bundle_archive_round_trips_whatever_the_insertion_order(bundle: SealedBundle) -> None:
    reordered = SealedBundle(dict(reversed(list(bundle.files.items()))))
    assert pack_bundle(bundle) == pack_bundle(reordered)
    assert unpack_bundle(pack_bundle(bundle)).files == bundle.files


@settings(max_examples=150)
@given(file_trees, file_trees, bundles, st.booleans())
def test_workspace_builder_never_emits_a_sealed_path_digest_or_content(
    base: dict[str, bytes], init: dict[str, bytes], bundle: SealedBundle, collide: bool
) -> None:
    protected = protected_files(bundle)
    hostile_base = {**base, **({next(iter(protected)): b"copy"} if collide and protected else {})}
    try:
        workspace = build_workspace(hostile_base, b"spec text", init, bundle)
    except WorkspaceLeak:
        return
    assert sealed_leaks(workspace.files.items(), protected) == []
    digests = {sha256_hex(c) for c in protected.values()}
    assert not any(path in protected for path in workspace.files)
    assert all(not any(d.encode() in content for d in digests) for content in workspace.files.values())


names = st.from_regex(r"[A-Z][A-Z0-9]{1,6}", fullmatch=True)


@given(
    names,
    st.integers(min_value=1, max_value=99999),
    st.dictionaries(safe_paths, st.text("0123456789abcdef", min_size=64, max_size=64), max_size=3),
    st.sampled_from(list(Language)),
    st.sampled_from(list(CriterionStatus)),
    st.dates(min_value=date(2026, 7, 1), max_value=date(2030, 1, 1)),
)
def test_task_canonical_round_trip(
    name: str, number: int, lockfiles: dict[str, str], language: Language, status: CriterionStatus, merged_on: date
) -> None:
    task_id = f"{name}-{number}"
    task = Task(
        task_id=task_id,
        language=language,
        repository="owner/repo",
        pull_number=number,
        merged_on=merged_on,
        spdx_id="Apache-2.0",
        licence_path="LICENSE",
        notice_path="NOTICE",
        base_sha="a" * 40,
        merge_sha="b" * 40,
        lockfile_digests=lockfiles,
        toolchain=Toolchain("go", "1.26.1"),
        criteria=(Criterion(f"{task_id}-1", status),),
    )
    assert parse_task(task.to_canonical_json()) == task
