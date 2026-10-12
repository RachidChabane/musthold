"""The verbs: preconditions, the machine's lifecycle, the summary and its lint."""

import json
from collections.abc import Sequence
from pathlib import Path, PurePosixPath

import pytest

from demos1.adapters.local import LocalFiles
from demos1.application.attempt_run import AttemptContext, AttemptVerb
from demos1.application.attempts import Target
from demos1.application.command_runner import RefusalBudget
from demos1.application.environment import Builds
from demos1.application.reporting import SummarizeVerb, SummaryPaths, report_lint
from demos1.application.select_run import CANDIDATES_FILE, Refused, SelectContext, SelectVerb, candidate_pool
from demos1.domain.deviations import counts_by_kind
from demos1.domain.ecosystem import Ecosystem
from demos1.domain.layout import WorkLayout
from demos1.domain.lists import ListName, Lists, Stratum
from demos1.domain.report_lint import lint
from demos1.domain.summary import SummaryInputs, build_summary
from support.attempt_flow import PickScript, attempts_for, b_entry, root_of
from support.fakes import FakeClock, FakeMachine, RecordedDeviations, ScriptedEngine

REPO_ROOT = Path(__file__).resolve().parents[5]


class Repo:
    def __init__(self, tags: dict[str, str], ancestor: bool = True, visibility: str = "PRIVATE") -> None:
        self.tags = tags
        self.ancestor = ancestor
        self.visibility = visibility

    def commit_of(self, ref: str) -> str | None:
        return self.tags.get(ref)

    def is_ancestor(self, ancestor: str, descendant: str) -> bool:
        return self.ancestor

    def commits_touching(self, since: str, until: str, paths: Sequence[str]) -> Sequence[str]:
        return []

    def head(self) -> str:
        return "c" * 40

    def origin_visibility(self) -> str:
        return self.visibility


class Probe:
    def __init__(self, facts: dict[str, str]) -> None:
        self.facts_given = facts
        self.calls = 0

    def facts(self, work: WorkLayout, builds: Builds, stamp: str) -> dict[str, str]:
        self.calls += 1
        return dict(self.facts_given)


def test_the_candidate_pool_is_the_frozen_benchmark_file():
    pool = candidate_pool(LocalFiles(), PurePosixPath(REPO_ROOT))
    assert len(pool) == 48
    assert all(name == name.lower() for name in pool)


def test_the_pool_is_refused_when_its_digest_differs(tmp_path: Path):
    target = tmp_path / CANDIDATES_FILE
    target.parent.mkdir(parents=True)
    target.write_text('{"repository": "north/ledger"}\n')
    with pytest.raises(Refused):
        candidate_pool(LocalFiles(), PurePosixPath(tmp_path))


def select_verb(tmp_path: Path, repo: Repo, machine: FakeMachine) -> tuple[SelectVerb, SelectContext]:
    work = WorkLayout(PurePosixPath(tmp_path / "work"), PurePosixPath(tmp_path / "product"))
    verb = SelectVerb(
        LocalFiles(),
        repo,
        Probe({}),  # type: ignore[arg-type]
        lambda directory: None,  # type: ignore[arg-type, return-value]
        FakeClock(),
        RecordedDeviations(),
        machine,
    )
    return verb, SelectContext(PurePosixPath(REPO_ROOT), work)


def test_select_refuses_without_the_product_tag(tmp_path: Path):
    machine = FakeMachine()
    verb, context = select_verb(tmp_path, Repo({"prereg-demos-1": "a" * 40}), machine)
    with pytest.raises(Refused, match="demos-1-product"):
        verb.run(context)
    assert machine.events == []


def test_select_refuses_a_product_that_does_not_descend_from_the_preregistration(tmp_path: Path):
    repo = Repo({"prereg-demos-1": "a" * 40, "demos-1-product": "b" * 40}, ancestor=False)
    verb, context = select_verb(tmp_path, repo, FakeMachine())
    with pytest.raises(Refused, match="descend"):
        verb.run(context)


def test_select_refuses_a_product_built_from_another_commit(tmp_path: Path):
    repo = Repo({"prereg-demos-1": "a" * 40, "demos-1-product": "b" * 40})
    verb, context = select_verb(tmp_path, repo, FakeMachine())
    files = LocalFiles()
    files.write_json(context.work.product_build, {"commit": "d" * 40})
    files.write_json(context.work.image_build, {"image_id": "sha256:image"})
    with pytest.raises(Refused, match="built from"):
        verb.run(context)


def test_attempt_starts_the_machine_and_stops_it_whatever_the_exit(tmp_path: Path):
    work = WorkLayout(PurePosixPath(tmp_path / "work"), PurePosixPath(tmp_path / "product"))
    machine = FakeMachine()
    verb = AttemptVerb(
        LocalFiles(),
        ScriptedEngine(lambda command: None),  # type: ignore[arg-type, return-value]
        FakeClock(),
        RecordedDeviations(),
        Probe({}),  # type: ignore[arg-type]
        lambda persist: None,  # type: ignore[arg-type, return-value]
        machine,
        RefusalBudget(),
    )
    with pytest.raises(Refused):
        verb.run(AttemptContext(work, 502, 20))
    assert machine.events == ["start", "stop"]


def played_summary(tmp_path: Path) -> SummaryInputs:
    work = WorkLayout(PurePosixPath(tmp_path / "work"), PurePosixPath(tmp_path / "product"))
    attempts, _, _ = attempts_for(work, PickScript(root_of(work)))
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    lists = Lists(b={Ecosystem.PYTEST: [b_entry()], Ecosystem.GO: []}, a={Ecosystem.PYTEST: [], Ecosystem.GO: []})
    return SummaryInputs(
        selection_date="2026-10-13",
        manifest={"prereg_commit": "a" * 40, "product_commit": "b" * 40, "tooling_commit": "c" * 40},
        manifest_sha256="1" * 64,
        lists=lists,
        lists_sha256="2" * 64,
        attempts=[record],
        deviation_counts=counts_by_kind(""),
    )


def test_the_summary_reports_every_count_over_its_denominator_and_passes_its_own_lint(tmp_path: Path):
    summary = build_summary(played_summary(tmp_path))
    markdown = summary.markdown
    assert "2/30 verify nothing" in markdown
    assert "21/30 caught ≥ 1 change" in markdown
    assert "8/38 not sampled" in markdown
    assert "2/40 tests not green" in markdown
    assert "1/2 findings no longer hold on replay" in markdown
    assert "0/2 replays failed" in markdown
    assert "file added by the pull request" in markdown
    assert "file not changed by the pull request" in markdown
    assert "1/2 picks" in markdown
    assert "#1` of the scan `change-full`" in markdown
    assert "11/134 drawn" in markdown
    fractions = summary.document["fractions"]
    assert isinstance(fractions, list)
    assert lint(markdown, [str(value) for value in fractions]) == []
    digests = summary.document["digests"]
    assert isinstance(digests, dict) and set(digests["scan.json"]) == {  # type: ignore[arg-type]
        "B-pytest-01/change",
        "B-pytest-01/quick",
        "B-pytest-01/change-full",
    }


def test_a_zero_is_stated_with_what_stays_unjudged():
    inputs = SummaryInputs(None, {}, "", Lists(), "", [], counts_by_kind(""))
    markdown = build_summary(inputs).markdown
    assert "No attempt was made." in markdown
    assert "0/0" in markdown


def test_summarize_refuses_unless_the_origin_is_private(tmp_path: Path):
    paths = SummaryPaths(PurePosixPath(REPO_ROOT))
    work = WorkLayout(PurePosixPath(tmp_path / "work"), PurePosixPath(tmp_path / "product"))
    with pytest.raises(Refused, match="visibility"):
        SummarizeVerb(LocalFiles(), Repo({}, visibility="PUBLIC")).run(work, paths)  # type: ignore[arg-type]


def test_summarize_then_report_lint_on_disk(tmp_path: Path):
    inputs = played_summary(tmp_path)
    work = WorkLayout(PurePosixPath(tmp_path / "work"), PurePosixPath(tmp_path / "product"))
    files = LocalFiles()
    files.write_json(work.manifest, {"selection_date": "2026-10-13", "prereg_commit": "a" * 40})
    files.write_json(work.lists, inputs.lists.to_json())
    root = tmp_path / "repo"
    (root / "notes").mkdir(parents=True)
    (root / "notes" / "public-history-report.md").write_text(
        "Command: `git filter-repo --invert-paths --path notes/ --force`\n"
    )
    paths = SummaryPaths(PurePosixPath(root))
    SummarizeVerb(files, Repo({})).run(work, paths)  # type: ignore[arg-type]
    assert report_lint(files, paths) == []
    document = json.loads((root / "notes/demos-1/summary.json").read_text())
    assert document["attempts"][0]["attempt"] == "B-pytest-01"
    (root / "notes/demos-1/SUMMARY.md").write_text("A safe result, 3/9.\n")
    assert {violation.text for violation in report_lint(files, paths)} == {"safe", "3/9"}
