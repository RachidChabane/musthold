from __future__ import annotations

from dataclasses import replace

import pytest

from specbench.domain.canonical import canonical_json
from specbench.domain.gaps import GAP_CRITERIA_COUNT, draw_gap_criteria, gaps_record
from specbench.domain.hunks import CodeHunk, changed_regions
from specbench.domain.languages import Language
from specbench.domain.sealing import (
    BUNDLE_VERSION_PATH,
    CURRENT_BUNDLE_VERSION,
    LEGACY_BUNDLE_VERSION,
    SealedBundle,
    SealingError,
    lock_entry_for,
    pack_bundle,
    require_current_version,
    unpack_bundle,
    version_record,
    versioned_bundle,
)
from specbench.domain.spec import Dimension, InterfaceEntry, SpecCriterion, SpecDraft, SpecError, spec_problems
from specbench.domain.task import CriterionStatus
from support.builders import TASK_ID, make_bundle, task_json

DIFF = "+++ b/pkg/fee.py\n@@ -1,2 +1,9 @@\n def fee(m):\n+1\n+2\n+3\n+4\n+5\n+6\n+7\n"
REGIONS = changed_regions(Language.PYTHON, DIFF)
STATEMENT = "WHEN a stay is short THE SYSTEM SHALL charge nothing"


def criterion(number: int, status: CriterionStatus, hunks: tuple[CodeHunk, ...]) -> SpecCriterion:
    return SpecCriterion(number, STATEMENT, ("fee(minutes=7) => 0",), status, Dimension.BOUNDARY, hunks)


def spec(*criteria: SpecCriterion) -> SpecDraft:
    return SpecDraft("t", "o", (InterfaceEntry("pkg.fee", "fee(m)", ""),), criteria)


HUNK = (CodeHunk("pkg/fee.py", 2, 3),)
SIX = spec(
    *(criterion(n, CriterionStatus.NEW if n % 2 else CriterionStatus.REVISED, HUNK) for n in range(1, 7)),
)


def test_a_bundle_without_a_version_record_is_version_one_and_a_versioned_one_carries_its_version() -> None:
    assert SealedBundle({"SPEC.md": b"x"}).schema_version == LEGACY_BUNDLE_VERSION == 1
    assert versioned_bundle({"SPEC.md": b"x"}).schema_version == CURRENT_BUNDLE_VERSION == 2
    assert versioned_bundle({"SPEC.md": b"x"}, 7).schema_version == 7


@pytest.mark.parametrize(
    "content",
    [
        b"",
        b"not json",
        b"[2]",
        b'{"schema_version":"2"}',
        b'{"schema_version":true}',
        b'{"schema_version": 2}',
        b'{"schema_version":0}',
    ],
)
def test_a_malformed_version_record_is_refused_when_the_bundle_is_built(content: bytes) -> None:
    with pytest.raises(SealingError):
        SealedBundle({"SPEC.md": b"x", BUNDLE_VERSION_PATH: content})


def test_the_version_record_is_canonical_and_a_bundle_file_so_the_lock_covers_it() -> None:
    bundle = make_bundle()
    assert bundle.files[BUNDLE_VERSION_PATH] == version_record(CURRENT_BUNDLE_VERSION) == b'{"schema_version":2}\n'
    entry = lock_entry_for(TASK_ID, Language.PYTHON, task_json(), bundle)
    assert BUNDLE_VERSION_PATH in entry.files
    assert bundle.bundle_digest != make_bundle(version=3).bundle_digest


def test_the_version_survives_the_archive() -> None:
    assert unpack_bundle(pack_bundle(make_bundle())).schema_version == CURRENT_BUNDLE_VERSION
    assert unpack_bundle(pack_bundle(make_bundle(version=1))).schema_version == LEGACY_BUNDLE_VERSION


def test_only_the_current_version_or_a_newer_one_may_be_sealed() -> None:
    require_current_version(make_bundle())
    require_current_version(make_bundle(version=CURRENT_BUNDLE_VERSION + 1))
    with pytest.raises(SealingError, match="below 2"):
        require_current_version(make_bundle(version=1))
    with pytest.raises(SealingError, match="version 1"):
        require_current_version(SealedBundle({"SPEC.md": b"x"}))


def test_the_draw_picks_two_distinct_eligible_criteria_in_ascending_order() -> None:
    drawn = draw_gap_criteria(20261007, TASK_ID, SIX)
    assert len(drawn) == GAP_CRITERIA_COUNT == 2
    assert list(drawn) == sorted(set(drawn)) and set(drawn) <= {1, 2, 3, 4, 5, 6}


def test_the_draw_is_deterministic_and_depends_on_the_seed_and_the_task() -> None:
    assert draw_gap_criteria(1, TASK_ID, SIX) == draw_gap_criteria(1, TASK_ID, SIX)
    by_seed = {draw_gap_criteria(seed, TASK_ID, SIX) for seed in range(40)}
    by_task = {draw_gap_criteria(1, f"ACME-T-{n}", SIX) for n in range(40)}
    assert len(by_seed) > 1 and len(by_task) > 1


def test_the_draw_skips_unchanged_criteria_and_criteria_without_hunks() -> None:
    mixed = spec(
        criterion(1, CriterionStatus.UNCHANGED, HUNK),
        criterion(2, CriterionStatus.NEW, HUNK),
        criterion(3, CriterionStatus.REVISED, HUNK),
        criterion(4, CriterionStatus.NEW, ()),
    )
    assert {draw_gap_criteria(seed, TASK_ID, mixed) for seed in range(20)} == {(2, 3)}


def test_a_draw_with_fewer_than_two_eligible_criteria_is_an_error() -> None:
    one = spec(criterion(1, CriterionStatus.NEW, HUNK), criterion(2, CriterionStatus.UNCHANGED, HUNK))
    with pytest.raises(SpecError):
        draw_gap_criteria(1, TASK_ID, one)


def test_the_gap_record_names_the_drawn_criteria_and_holds_no_dimension_or_hunk() -> None:
    record = gaps_record(20261007, TASK_ID, (2, 5))
    assert record == {"criteria": [f"{TASK_ID}-2", f"{TASK_ID}-5"], "seed": 20261007}
    assert "pkg/fee.py" not in canonical_json(record)


def test_a_new_or_revised_criterion_must_cite_a_hunk_and_the_hunk_must_lie_in_the_diff() -> None:
    assert spec_problems(SIX, REGIONS) == []
    bare = spec(criterion(1, CriterionStatus.NEW, ()), criterion(2, CriterionStatus.REVISED, HUNK))
    assert any("criterion 1" in p and "implementing hunk" in p for p in spec_problems(bare, REGIONS))
    outside = spec(
        criterion(1, CriterionStatus.NEW, (CodeHunk("pkg/fee.py", 8, 20),)), criterion(2, CriterionStatus.REVISED, HUNK)
    )
    assert any("criterion 1" in p and "pkg/fee.py:8-20" in p for p in spec_problems(outside, REGIONS))


def test_an_unchanged_criterion_may_cite_no_hunk_but_a_cited_one_must_still_lie_in_the_diff() -> None:
    base = (criterion(1, CriterionStatus.NEW, HUNK), criterion(2, CriterionStatus.REVISED, HUNK))
    quiet = spec(*base, criterion(3, CriterionStatus.UNCHANGED, ()))
    assert spec_problems(quiet, REGIONS) == []
    elsewhere = spec(*base, criterion(3, CriterionStatus.UNCHANGED, (CodeHunk("pkg/other.py", 1, 1),)))
    assert any("criterion 3" in p for p in spec_problems(elsewhere, REGIONS))


def test_two_gap_eligible_criteria_are_required_so_a_task_that_cannot_be_gapped_is_dropped_at_the_spec_stage() -> None:
    one_changing = spec(criterion(1, CriterionStatus.NEW, HUNK), criterion(2, CriterionStatus.UNCHANGED, ()))
    assert any("at least 2 criteria" in p for p in spec_problems(one_changing, REGIONS))


def test_the_dimension_is_one_of_the_five_kinds_of_adr_0004() -> None:
    assert [d.value for d in Dimension] == ["error", "boundary", "permission", "concurrency", "compatibility"]
    assert replace(SIX.criteria[0], dimension=Dimension.PERMISSION).dimension is Dimension.PERMISSION
