from __future__ import annotations

import pytest

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.differential import HypothesisDifferential
from pilot.adapters.import_smoke import ProjectImportSmoke
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.application.process_mutants import MutantProcessor
from pilot.domain.edits import Edit, EditSet
from pilot.domain.mutants import Candidate, DifferentialKind, MutantStatus
from pilot.domain.outcomes import KillResult

pytestmark = pytest.mark.integration
TARIFF = "src/parkfee/tariff.py"
KILLED_BY_BOTH = "tests/test_mini.py::test_charges_two_hours_for_sixty_one_minutes"


def candidate(mutant_id, qualname, source, arm="T"):
    return Candidate(arm, mutant_id, EditSet((Edit(TARIFF, qualname, source),)))


@pytest.fixture
def processor(mini_root, tmp_path):
    process = SubprocessRunner()
    project = mini_root / "fixture" / "parkfee"
    workspace = ProjectWorkspace(project, tmp_path / "work", AstEditApplier())
    runner = WorkspaceVariantRunner(workspace, PytestJunitRunner(process).run)
    return MutantProcessor(
        runner, AstEditApplier(), project, ProjectImportSmoke(workspace, process),
        HypothesisDifferential(workspace, process, project),
    )


def by_id(records):
    return {r.candidate.mutant_id: r for r in records}


def test_isolated_import_smoke_can_see_the_source_directory(mini_root, tmp_path):
    process = SubprocessRunner()
    workspace = ProjectWorkspace(mini_root / "fixture" / "parkfee", tmp_path / "work", AstEditApplier())
    smoke = ProjectImportSmoke(workspace, process)
    good = EditSet((Edit(TARIFF, "format_receipt", 'def format_receipt(amount):\n    return "X"\n'),))
    assert smoke.passes(good)


def test_import_smoke_fails_when_the_package_cannot_import(mini_root, tmp_path):
    workspace = ProjectWorkspace(mini_root / "fixture" / "parkfee", tmp_path / "work", AstEditApplier())
    bad = EditSet((Edit(TARIFF, "format_receipt", "@missing_decorator\ndef format_receipt(amount):\n    pass\n"),))
    assert not ProjectImportSmoke(workspace, SubprocessRunner()).passes(bad)


def test_full_pipeline_on_the_mini_fixture(processor):
    pending = [
        candidate("rate", "_hourly_charge", 'def _hourly_charge(minutes):\n    return Decimal("2.00") * -(-minutes // 60)\n'),
        candidate("equivalent", "_hourly_charge", "def _hourly_charge(minutes):\n    # same\n    return HOURLY_RATE * ((minutes + 59) // 60)\n"),
        candidate("duplicate", "_hourly_charge", 'def _hourly_charge(minutes):\n    return Decimal("2.00")*-(-minutes//60)\n'),
        candidate("absent", "nothing", "def nothing():\n    pass\n"),
        candidate("trivial", "format_receipt", 'def format_receipt(amount: Decimal) -> str:\n    """Doc."""\n    if amount == 0:\n        return "FREE"\n    return f"EUR {amount:.2f}"\n'),
        candidate("receipt", "format_receipt", 'def format_receipt(amount):\n    return "FREE"\n'),
    ]
    records = by_id(processor.process(pending))
    assert records["rate"].status is MutantStatus.EVALUATED
    assert records["rate"].kill_vector[KILLED_BY_BOTH] is KillResult.KILLED
    assert records["rate"].killed_by == [
        "tests/test_mini.py::test_charges_first_hour_after_free_window",
        KILLED_BY_BOTH,
    ]
    assert records["rate"].differential.kind is DifferentialKind.DIFFERS
    assert records["rate"].differential.function == "quote"
    assert records["duplicate"].status is MutantStatus.DUPLICATE
    assert records["rate"].merged_ids == ["duplicate"]
    assert records["absent"].reason == "absent_qualname"
    assert records["trivial"].status is MutantStatus.TRIVIALLY_EQUIVALENT
    assert records["equivalent"].status is MutantStatus.EVALUATED
    assert records["equivalent"].killed_by == []
    assert records["equivalent"].differential.kind is DifferentialKind.NO_DIFF_FOUND
    assert records["receipt"].differential.function == "format_receipt"
    assert records["receipt"].differential.kind is DifferentialKind.DIFFERS


def test_mutant_that_breaks_the_import_is_stillborn(processor):
    broken = candidate("broken", "format_receipt", "@missing_decorator\ndef format_receipt(amount):\n    pass\n")
    record = processor.process([broken])[0]
    assert (record.status, record.reason) == (MutantStatus.STILLBORN, "import_smoke_failed")
    assert record.differential is None
