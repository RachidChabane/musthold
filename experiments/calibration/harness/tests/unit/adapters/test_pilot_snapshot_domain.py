from __future__ import annotations

import ast
import hashlib
from collections.abc import Mapping
from pathlib import Path

import pytest
import rfc8785
from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from calibration.adapters.pilot_snapshot import PilotSnapshotDomain, UnpackedSnapshot
from calibration.application.build_ports import UnitPair
from pilot_layout import unpack_snapshot

TARIFF = "src/parkfee/tariff.py"
MUTANT_ID_LENGTH = 16
QUOTE_SIGNATURE = (
    "def quote(entry: datetime, exit: datetime, *, lost_ticket: bool=False, validated: bool=False) -> Decimal:"
)
BREAKDOWN_SIGNATURE = (
    "def breakdown(entry: datetime, exit: datetime, *, lost_ticket: bool=False, validated: bool=False) -> Breakdown:"
)
QUOTE_MUTANT = f"{QUOTE_SIGNATURE}\n    return breakdown(entry, exit, lost_ticket=lost_ticket, validated=validated).amount + Decimal(1)"
BREAKDOWN_MUTANT = f"{BREAKDOWN_SIGNATURE}\n    return Breakdown(Rule.GRACE, exit - entry, FREE)"
SOURCE_POOL = [QUOTE_MUTANT, BREAKDOWN_MUTANT, "def other():\n    return 1", "class Box:\n    value = 2"]


def edit(qualname: str, new_source: str, file: str = TARIFF) -> dict[str, str]:
    return {"file": file, "qualname": qualname, "new_source": new_source}


def pilot_digest(edits: list[dict[str, str]]) -> str:
    return hashlib.sha256(rfc8785.dumps({"edits": edits})).hexdigest()[:MUTANT_ID_LENGTH]


def original_definition(source: str, qualname: str) -> str:
    module = ast.parse(source)
    node = next(n for n in module.body if isinstance(n, ast.FunctionDef) and n.name == qualname)
    return ast.unparse(ast.parse(ast.get_source_segment(source, node) or ""))


@pytest.fixture(scope="module")
def snapshot(tmp_path_factory: pytest.TempPathFactory) -> UnpackedSnapshot:
    return UnpackedSnapshot(unpack_snapshot(tmp_path_factory.mktemp("snapshot")))


@pytest.fixture(scope="module")
def domain(snapshot: UnpackedSnapshot) -> PilotSnapshotDomain:
    return PilotSnapshotDomain(snapshot)


def test_the_snapshot_locates_the_project_the_pilot_source_and_the_fixture_sources(snapshot: UnpackedSnapshot) -> None:
    assert snapshot.project_dir.name == "parkfee"
    assert (snapshot.pilot_source_root / "pilot" / "domain" / "normalization.py").is_file()
    sources = snapshot.fixture_sources()
    assert TARIFF in sources
    assert set(sources) == {f"src/parkfee/{name}.py" for name in ("__init__", "models", "rates", "receipt", "tariff")}
    assert sources[TARIFF] == (snapshot.project_dir / TARIFF).read_text()


def test_the_fixture_sources_exclude_the_tests(snapshot: UnpackedSnapshot) -> None:
    assert not [name for name in snapshot.fixture_sources() if name.startswith("tests/")]


def test_mutant_id_is_the_first_sixteen_hex_characters_of_the_canonical_edit_set_digest(
    domain: PilotSnapshotDomain,
) -> None:
    edits = [edit("quote", QUOTE_MUTANT), edit("breakdown", BREAKDOWN_MUTANT)]
    ordered = sorted(edits, key=lambda e: (e["file"], e["qualname"]))
    assert domain.mutant_id(edits) == pilot_digest(ordered)


@settings(suppress_health_check=[HealthCheck.function_scoped_fixture], max_examples=60)
@given(
    sources=st.lists(st.sampled_from(SOURCE_POOL), min_size=1, max_size=3, unique=True),
    data=st.data(),
)
def test_mutant_id_is_independent_of_the_order_of_the_edits(
    domain: PilotSnapshotDomain, sources: list[str], data: st.DataObject
) -> None:
    edits = [edit(f"name{index}", source) for index, source in enumerate(sources)]
    shuffled = data.draw(st.permutations(edits))
    assert domain.mutant_id(shuffled) == domain.mutant_id(edits) == pilot_digest(edits)


def test_unit_pairs_are_the_normalized_definitions_before_and_after_the_edit_set_in_canonical_order(
    domain: PilotSnapshotDomain, snapshot: UnpackedSnapshot
) -> None:
    sources = snapshot.fixture_sources()
    edits = [edit("quote", QUOTE_MUTANT), edit("breakdown", BREAKDOWN_MUTANT)]
    pairs = domain.unit_pairs(sources, edits)
    assert pairs == [
        UnitPair(
            TARIFF,
            "breakdown",
            original_definition(sources[TARIFF], "breakdown"),
            ast.unparse(ast.parse(BREAKDOWN_MUTANT)),
        ),
        UnitPair(TARIFF, "quote", original_definition(sources[TARIFF], "quote"), ast.unparse(ast.parse(QUOTE_MUTANT))),
    ]


def test_unit_pairs_do_not_modify_the_sources(domain: PilotSnapshotDomain, snapshot: UnpackedSnapshot) -> None:
    sources: Mapping[str, str] = snapshot.fixture_sources()
    before = dict(sources)
    domain.unit_pairs(sources, [edit("quote", QUOTE_MUTANT)])
    assert dict(sources) == before


def test_unit_pairs_of_an_unknown_definition_raise(domain: PilotSnapshotDomain, snapshot: UnpackedSnapshot) -> None:
    with pytest.raises(Exception, match="absent_qualname"):
        domain.unit_pairs(snapshot.fixture_sources(), [edit("missing", "def missing():\n    return 1")])


def test_two_domains_of_two_snapshots_do_not_mix(tmp_path: Path) -> None:
    first = PilotSnapshotDomain(UnpackedSnapshot(unpack_snapshot(tmp_path / "first")))
    second = PilotSnapshotDomain(UnpackedSnapshot(unpack_snapshot(tmp_path / "second")))
    edits = [edit("quote", QUOTE_MUTANT)]
    assert first.mutant_id(edits) == second.mutant_id(edits)
