"""The report the benchmark reads, against the schema of the pinned Shallnot binary itself (`shallnot schema report`)."""

from __future__ import annotations

import shutil
from pathlib import Path

import pytest

from specbench.adapters.process import SubprocessRunner
from specbench.adapters.schemas import JsonSchemaChecker
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.domain.canonical import parse_stable_json, stable_json
from specbench.domain.gate import GateOutcome, GateStatus
from specbench.domain.jsonread import mapping, sequence, text
from specbench.domain.shallnot_config import ProofTrust, arm_shallnot_config
from support.product_world import BINARIES, PARCEL_FIXTURE

SHALLNOT = BINARIES / "shallnot"
pytestmark = pytest.mark.skipif(not SHALLNOT.is_file(), reason="Shallnot is not built: run script/shallnot-build")
READ_BY_THE_BENCHMARK = ("schema_version", "verdict", "exit_code", "summary", "requirements", "findings", "extensions")
VERDICTS = {"pass", "fail"}


def cli() -> ShallnotCli:
    return ShallnotCli(SubprocessRunner(), SHALLNOT, JsonSchemaChecker())


def required_fields() -> list[str]:
    schema = mapping(cli().report_schema(Path.cwd(), {}), "schema")
    return [text(name, "required field") for name in sequence(schema["required"], "required")]


def test_the_schema_the_binary_prints_accepts_report_1_2_and_refuses_1_1() -> None:
    schema = cli().report_schema(Path.cwd(), {})
    checker = JsonSchemaChecker()
    minimal = dict.fromkeys(required_fields())
    versions = {version: checker.problems(schema, {**minimal, "schema_version": version}) for version in ("1.2", "1.1")}
    assert not any("schema_version" in problem for problem in versions["1.2"])
    assert any("schema_version" in problem for problem in versions["1.1"])


def test_the_schema_requires_every_field_the_benchmark_reads() -> None:
    assert set(READ_BY_THE_BENCHMARK) <= set(required_fields())


def test_the_report_of_a_real_gate_conforms_and_survives_an_archive_round_trip(tmp_path: Path) -> None:
    project = tmp_path / "parcel"
    shutil.copytree(PARCEL_FIXTURE, project)
    shallnot = cli()
    shallnot.init(project, {})
    config = project / "shallnot.contract.yaml"
    base = (project / "shallnot.yaml").read_text(encoding="utf-8")
    config.write_text(arm_shallnot_config(base, (), ProofTrust.SIGNED), encoding="utf-8")

    gate = shallnot.gate(project, config, tmp_path / "report.json", {})

    body = mapping(gate.report, "report")
    assert set(READ_BY_THE_BENCHMARK) <= set(body)
    assert text(body["schema_version"], "schema_version").startswith("1.")
    assert body["verdict"] in VERDICTS
    assert body["exit_code"] == gate.exit_code
    outcome = GateOutcome(gate.report, GateStatus.COMPLETE, None, 0, ())
    archived = parse_stable_json(stable_json(outcome.to_record()))
    assert isinstance(archived, dict)
    assert GateOutcome.from_record(archived) == outcome
