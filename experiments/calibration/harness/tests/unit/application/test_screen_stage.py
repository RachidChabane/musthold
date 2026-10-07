from __future__ import annotations

from pathlib import Path

import pytest

from calibration.application.build_ports import PilotDomain, SnapshotSource
from calibration.application.deviation_log import DeviationLog
from calibration.application.screen_items import BLOCKED_SCREEN, WITNESS_MAPPED, WITNESS_UNMAPPED, ScreenAssembly
from calibration.application.screen_stage import K4_SCOPE, ScreenStage, identity_problems
from calibration.domain.constants import PILOT_DIFFERENTIAL_FILE, PILOT_MUTANTS_DIR, PILOT_PROCESSING_FILE
from calibration.domain.deviations import DeviationKind
from calibration.domain.pilot_records import MutantDocument
from support.build_fakes import FakeBuilders, FakePilotDomain, FakePilotRunReader, FakeSnapshotSource, InMemoryRunStore
from support.build_scenario import (
    FIXTURE_SOURCES,
    SCREEN_TEMPLATE_DIGEST,
    PilotMutant,
    differs,
    pilot_run_files,
)

PROJECT = Path("/work/project")
SHALLNOT = Path("/export/script/shallnot")
MAPPED = PilotMutant("mapped_target", "T", differential=differs(mapped=True))
UNMAPPED = PilotMutant("unmapped_pocket", "P", differential=differs(mapped=False))
PLAIN = PilotMutant("no_differential", "T")
STILLBORN = PilotMutant("stillborn_one", "T", stillborn=True)
OTHER_ARM = PilotMutant("operator_arm", "X")


class _Stage:
    def __init__(self, mutants: tuple[PilotMutant, ...]) -> None:
        self.reader = FakePilotRunReader(pilot_run_files((), mutants))
        self.store = InMemoryRunStore()
        self.domain = FakePilotDomain()
        self.builders = FakeBuilders({}, {}, SCREEN_TEMPLATE_DIGEST)
        self.domain_requests = 0
        self.stage = ScreenStage(self.reader, DeviationLog(self.store), self._domain_of)

    def _domain_of(self, source: SnapshotSource) -> PilotDomain:
        self.domain_requests += 1
        return self.domain

    def build(self, problems: list[str] | None = None, template: str = SCREEN_TEMPLATE_DIGEST) -> ScreenAssembly:
        source = FakeSnapshotSource(PROJECT, FIXTURE_SOURCES)
        return self.stage.build(
            source, self.builders, PROJECT, SHALLNOT, template, [] if problems is None else problems
        )


class TestMissingRecords:
    @pytest.mark.parametrize("missing", [PILOT_PROCESSING_FILE, PILOT_DIFFERENTIAL_FILE])
    def test_a_missing_arm_record_blocks_k4_and_builds_nothing(self, missing: str) -> None:
        stage = _Stage((MAPPED,))
        del stage.reader.files[missing]

        assembly = stage.build()

        assert assembly is BLOCKED_SCREEN
        assert stage.builders.screen_calls == []
        assert [(e.kind, e.scope) for e in DeviationLog(stage.store).entries()] == [
            (DeviationKind.BLOCKED, tuple(K4_SCOPE))
        ]

    def test_a_missing_arm_record_is_detected_before_the_pilot_domain_is_loaded(self) -> None:
        stage = _Stage((MAPPED,))
        del stage.reader.files[PILOT_PROCESSING_FILE]

        stage.build()

        assert stage.domain_requests == 0
        assert stage.reader.read_paths == []

    def test_a_missing_mutant_document_blocks_k4_with_its_own_reason(self) -> None:
        stage = _Stage((MAPPED, PLAIN))
        del stage.reader.files[f"{PILOT_MUTANTS_DIR}/{PLAIN.mutant_id}.json"]

        assembly = stage.build()

        assert assembly.blocked
        (entry,) = DeviationLog(stage.store).entries()
        assert "mutant record" in entry.text
        assert stage.builders.screen_calls == []

    def test_a_stillborn_mutant_needs_its_document_too(self) -> None:
        stage = _Stage((MAPPED, STILLBORN))
        del stage.reader.files[f"{PILOT_MUTANTS_DIR}/{STILLBORN.mutant_id}.json"]

        assert stage.build().blocked


class TestBuiltScreen:
    def test_one_item_is_built_for_each_evaluated_mutant_of_arms_t_and_p(self) -> None:
        stage = _Stage((MAPPED, UNMAPPED, PLAIN, STILLBORN, OTHER_ARM))

        assembly = stage.build()

        assert not assembly.blocked
        assert sorted(c.mutant_id for c in assembly.candidates) == sorted(
            m.mutant_id for m in (MAPPED, UNMAPPED, PLAIN)
        )
        assert sorted(stage.builders.screen_calls[0]) == sorted(c.mutant_id for c in assembly.candidates)
        assert len(assembly.items) == 3

    def test_the_witness_status_of_each_item_follows_its_differential(self) -> None:
        stage = _Stage((MAPPED, UNMAPPED, PLAIN))

        assembly = stage.build()

        statuses = {item.mutant_id: item.witness_status for item in assembly.items}
        assert statuses[MAPPED.mutant_id] == WITNESS_MAPPED
        assert statuses[UNMAPPED.mutant_id] == WITNESS_UNMAPPED
        assert statuses[PLAIN.mutant_id] == "none"

    def test_only_the_unmapped_candidates_are_reported_unmapped(self) -> None:
        stage = _Stage((MAPPED, UNMAPPED, PLAIN))

        assembly = stage.build()

        assert [c.mutant_id for c in assembly.unmapped] == [UNMAPPED.mutant_id]

    def test_a_blocked_nothing_is_logged_when_the_records_are_complete(self) -> None:
        stage = _Stage((MAPPED,))

        stage.build()

        assert DeviationLog(stage.store).entries() == []

    def test_the_arm_and_normalized_edits_of_the_documents_are_all_that_is_read_of_a_mutant(self) -> None:
        stage = _Stage((MAPPED,))

        stage.build()

        assert set(stage.reader.read_paths) == {
            PILOT_PROCESSING_FILE,
            PILOT_DIFFERENTIAL_FILE,
            f"{PILOT_MUTANTS_DIR}/{MAPPED.mutant_id}.json",
        }


class TestProblems:
    def test_a_mutant_id_the_pilot_does_not_reproduce_is_a_problem(self) -> None:
        forged = PilotMutant("forged", "T", recorded_id="f" * 16)
        stage = _Stage((forged,))
        problems: list[str] = []

        stage.build(problems)

        assert problems == [f"mutant {'f' * 16} is not reproduced by the pilot's id from its normalized edits"]

    def test_a_template_digest_other_than_the_modules_is_a_problem(self) -> None:
        stage = _Stage((MAPPED,))
        problems: list[str] = []

        stage.build(problems, template="another")

        assert len(problems) == 1
        assert "template digest" in problems[0]

    def test_a_consistent_screen_adds_no_problem(self) -> None:
        stage = _Stage((MAPPED, UNMAPPED, PLAIN))
        problems: list[str] = []

        stage.build(problems)

        assert problems == []


class TestIdentity:
    def _documents(self, *mutants: PilotMutant) -> dict[str, MutantDocument]:
        return {m.mutant_id: MutantDocument(m.mutant_id, m.arm, tuple(m.edits)) for m in mutants}

    def test_ids_reproduced_from_their_edits_are_no_problem(self) -> None:
        assert identity_problems(self._documents(MAPPED, UNMAPPED), FakePilotDomain()) == []

    def test_an_arm_that_is_not_measured_is_not_checked(self) -> None:
        forged = PilotMutant("forged", "X", recorded_id="f" * 16)
        domain = FakePilotDomain()

        assert identity_problems(self._documents(forged), domain) == []
        assert domain.identity_checks == 0

    def test_problems_come_in_mutant_id_order(self) -> None:
        first = PilotMutant("one", "T", recorded_id="b" * 16)
        second = PilotMutant("two", "P", recorded_id="a" * 16)

        problems = identity_problems(self._documents(first, second), FakePilotDomain())

        assert [problem.split(" ")[1] for problem in problems] == ["a" * 16, "b" * 16]
