"""The Go builders compiled once from the working tree and driven end to end: no model, no server, no network."""

from __future__ import annotations

import hashlib
import json
import posixpath
import re
import shutil
import tempfile
from collections.abc import Iterator, Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final, cast
from urllib.parse import urlsplit

import pytest

from calibration.adapters.alignment_driver import BinaryAlignmentDriver
from calibration.adapters.go_builders import BinaryBuilders, BinaryProductReader, BuilderFailure
from calibration.adapters.go_layout import BUILDER_TARGETS, PRODUCT_ALIGNMENT_BINARY
from calibration.adapters.go_toolchain import GoBuilderToolchain
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.application.build_ports import BuilderBuild
from calibration.application.builder_check import run_builder_check
from calibration.domain.canonical import JsonValue
from calibration.domain.ports import DecodeRequest, ScreenReadRequest
from repository_paths import REPOSITORY_ROOT

pytestmark = [pytest.mark.integration, pytest.mark.go]

REPO_ROOT: Final = REPOSITORY_ROOT
BUILDER_DIR: Final = REPO_ROOT / "experiments" / "calibration" / "builder"
PRODUCT_MODULES: Final = ("format", "alignment", "discpower", "s1relay")
COPY_IGNORED: Final = shutil.ignore_patterns(".git", "node_modules", "__pycache__")
WORKSPACE_FILE: Final = "go.work"
GO_VERSION_LINE: Final = "go 1.26.4"
GO_ENV: Final = {"GOPROXY": "off"}
BUILD_PROCS: Final = 1
RECORDED_SCRIPT: Final = Path("s1relay/testdata/alignment/shallnot-recorded.sh")
RECORDED_REPORT: Final = Path("s1relay/testdata/alignment/shallnot-report.json")
RECORDED_REPORT_VARIABLE: Final = "SHALLNOT_RECORDED_REPORT"
FIXTURE_PROJECT: Final = Path("s1relay/testdata/alignment/project")
OLLAMA_MODEL: Final = "tev1:0.8b"
KEV_MODEL: Final = "kev-latest"
TOKEN_PATTERN: Final = re.compile(r"^K-[0-9a-f]{4}$")
NONE_OPTION: Final = "NONE"
COVERAGE_LEVELS: Final = ("NONE", "WEAK", "PARTIAL", "FULL")
RUN_TIMEOUT_SECONDS: Final = 1800

RECORDED_OLLAMA_CHOICE_REQUEST: Final = (
    '{"model":"tev1:0.8b","state":"the sky is green","questions":{"color":{"type":"choice","instructions":'
    '"Which color does the text say the sky is?","criteria":{"blue":"blue","green":"green","grey":"grey"}}}}'
)
RECORDED_OLLAMA_CHOICE_REPLY: Final = (
    '{"model":"tev1:0.8b","answers":{"color":{"type":"choice","choice":"green","probabilities":'
    '{"blue":0.0028416608947914386,"green":0.9948621078449252,"grey":0.0022962312602833944},'
    '"confidence":0.9674686077174837}},"usage":{"input_tokens":125,"output_tokens":1}}'
)
RECORDED_OLLAMA_NOUL_REPLY: Final = (
    '{"model":"tev1:0.8b","answers":{"sky_is_green":{"type":"noul","noul":0.9861792840259738}},'
    '"usage":{"input_tokens":136,"output_tokens":1}}'
)
RECORDED_KEV_CHOICE_REPLY: Final = (
    '{"model":"kev-latest","answers":{"color":{"type":"choice","choice":"green","confidence":0.8802,'
    '"probabilities":{"blue":0.0422,"green":0.9202,"grey":0.0377}}},"usage":{"input_tokens":32,'
    '"output_tokens":63},"latency_ms":29.9}'
)
SKY_CLOSED: Final = (
    '{"state":"the sky is green","questions":[{"id":"color","type":"choice","instructions":'
    '"Which color does the text say the sky is?","options":[{"name":"blue","description":"blue"},'
    '{"name":"green","description":"green"},{"name":"grey","description":"grey"}]}]}'
)
NOUL_CLOSED: Final = (
    '{"state":"the sky is green","questions":[{"id":"sky_is_green","type":"noul","instructions":"Is the sky green?"}]}'
)
COVERAGE_CLOSED: Final = (
    '{"state":"a test","questions":[{"id":"criterion","type":"choice","instructions":"Which?","options":'
    '[{"name":"K-1","description":"first"},{"name":"NONE","description":"none"}]},'
    '{"id":"coverage","type":"score","instructions":"How much?","options":[{"name":"NONE","description":"none"},'
    '{"name":"WEAK","description":"weak"},{"name":"PARTIAL","description":"partial"},'
    '{"name":"FULL","description":"full"}]}]}'
)
OUT_OF_RANGE_SCORE_REPLY: Final = (
    '{"model":"m","answers":{"criterion":{"type":"choice","choice":"K-1"},"coverage":{"type":"score","score":4}}}'
)

SYNTHETIC_REQUIREMENTS: Final = (
    ("SYN-1", "WHEN a line is priced THE SYSTEM SHALL honour SYN-2 and multiply the unit price by the quantity."),
    ("SYN-2", "WHEN a parcel is shipped THE SYSTEM SHALL add the surcharge of SYN-2~1."),
    ("SYN-3", "WHEN the quantity is zero THE SYSTEM SHALL reject the line, as SYN-1 does."),
)
SYNTHETIC_REF_FORMAT: Final = "{id}~1"
SYNTHETIC_DIGEST_FORMAT: Final = "{index:064x}"


@dataclass(frozen=True)
class GoWorld:
    export_dir: Path
    binaries_dir: Path
    runner: SubprocessRunner
    build: BuilderBuild
    builders: BinaryBuilders
    reader: BinaryProductReader
    driver: BinaryAlignmentDriver

    @property
    def recorded_script(self) -> Path:
        return self.export_dir / RECORDED_SCRIPT

    @property
    def recorded_report(self) -> Path:
        return self.export_dir / RECORDED_REPORT


def _copy_product_tree(destination: Path) -> None:
    for module in PRODUCT_MODULES:
        shutil.copytree(REPO_ROOT / module, destination / module, ignore=COPY_IGNORED)
    uses = "".join(f"\t./{module}\n" for module in sorted(PRODUCT_MODULES))
    (destination / WORKSPACE_FILE).write_text(f"{GO_VERSION_LINE}\n\nuse (\n{uses})\n", encoding="utf-8")


@pytest.fixture(scope="module")
def world() -> Iterator[GoWorld]:
    scratch = Path(tempfile.mkdtemp(prefix="calibration-go-"))
    try:
        export_dir, binaries_dir = scratch / "export", scratch / "bin"
        _copy_product_tree(export_dir)
        runner = SubprocessRunner()
        built = GoBuilderToolchain(runner, [], GO_ENV).build(export_dir, BUILDER_DIR, binaries_dir, BUILD_PROCS)
        recorded_env = {RECORDED_REPORT_VARIABLE: str(export_dir / RECORDED_REPORT)}
        yield GoWorld(
            export_dir=export_dir,
            binaries_dir=binaries_dir,
            runner=runner,
            build=built,
            builders=BinaryBuilders(runner, binaries_dir, recorded_env),
            reader=BinaryProductReader(runner, binaries_dir),
            driver=BinaryAlignmentDriver(runner, binaries_dir),
        )
    finally:
        shutil.rmtree(scratch, ignore_errors=True)


def _fresh_project(world: GoWorld, destination: Path) -> Path:
    project = destination / "project"
    shutil.copytree(world.export_dir / FIXTURE_PROJECT, project)
    return project


def _object(value: JsonValue | object, where: str) -> Mapping[str, JsonValue]:
    assert isinstance(value, Mapping), where
    return cast(Mapping[str, JsonValue], value)


def _items(value: JsonValue | object) -> list[Mapping[str, JsonValue]]:
    assert isinstance(value, list)
    return [_object(entry, "item") for entry in cast(list[object], value)]


def _closed_questions(closed: JsonValue) -> list[Mapping[str, JsonValue]]:
    assert isinstance(closed, str)
    return _items(_object(json.loads(closed), "closed")["questions"])


def _option_names(question: Mapping[str, JsonValue]) -> list[str]:
    return [cast(str, _object(option, "option")["name"]) for option in cast(list[JsonValue], question["options"])]


def _canonical_endpoint(address: str) -> str:
    parts = urlsplit(address)
    return f"{parts.scheme.lower()}://{parts.netloc.lower()}{posixpath.normpath(parts.path)}"


class TestBuild:
    def test_the_toolchain_digests_every_source_and_binary(self, world: GoWorld) -> None:
        assert set(world.build.source_digests) == {target.source for target in BUILDER_TARGETS}
        assert set(world.build.binary_digests) == {target.binary for target in BUILDER_TARGETS}
        for target in BUILDER_TARGETS:
            source = (BUILDER_DIR / target.source).read_bytes()
            assert world.build.source_digests[target.source] == hashlib.sha256(source).hexdigest()
            assert (world.export_dir / target.destination).read_bytes() == source
        assert (world.binaries_dir / PRODUCT_ALIGNMENT_BINARY).is_file()

    def test_the_builders_are_gofmt_clean_and_vet_clean_inside_the_export(self, world: GoWorld) -> None:
        sources = [str(world.export_dir / target.destination) for target in BUILDER_TARGETS]
        listed = world.runner.run(["gofmt", "-l", *sources], timeout_seconds=RUN_TIMEOUT_SECONDS)
        assert listed.returncode == 0
        assert listed.stdout == b""
        for target in BUILDER_TARGETS:
            vetted = world.runner.run(
                ["go", "vet", "-p", str(BUILD_PROCS), f"./cmd/{target.command_dir}"],
                cwd=world.export_dir / target.module,
                env={**GO_ENV, "GOMAXPROCS": "2"},
                timeout_seconds=RUN_TIMEOUT_SECONDS,
            )
            assert vetted.returncode == 0, vetted.stderr.decode()


class TestAlignmentBuilder:
    def test_the_first_wave_export_is_rebuilt_closed_set_for_closed_set(self, world: GoWorld, tmp_path: Path) -> None:
        export = world.driver.first_wave_export(world.export_dir, world.recorded_script, world.recorded_report)
        exported = json.loads(export)
        assert exported
        project = _fresh_project(world, tmp_path)
        result = world.builders.compare_export(project, world.recorded_script, export)
        assert result["exported"] == len(exported)
        assert result["matched"] == len(exported)
        assert result["unmatched_exported"] == []
        left_over = cast(list[str], result["unmatched_rebuilt"])
        assert cast(int, result["rebuilt"]) == len(exported) + len(left_over)
        assert set(left_over) <= self._fixture_labels(world, tmp_path)

    def test_the_builder_check_of_the_build_step_passes_with_the_real_driver(self, world: GoWorld) -> None:
        check = run_builder_check(world.driver, world.builders, world.export_dir)
        assert check.passed
        assert check.matched == check.exported > 0

    @staticmethod
    def _fixture_labels(world: GoWorld, tmp_path: Path) -> set[str]:
        built = world.builders.alignment_build(
            _fresh_project(world, tmp_path / "labels"), world.recorded_script, [OLLAMA_MODEL]
        )
        return {cast(str, item["label"]) for item in _items(built["items"])}

    def test_a_failing_driver_names_the_command_and_its_error(self, world: GoWorld, tmp_path: Path) -> None:
        missing = tmp_path / "no-such-report.json"
        with pytest.raises(BuilderFailure):
            world.driver.first_wave_export(world.export_dir, world.recorded_script, missing)

    def test_build_renders_the_items_the_way_render_closed_does(self, world: GoWorld, tmp_path: Path) -> None:
        identifiers = [OLLAMA_MODEL, KEV_MODEL]
        project = _fresh_project(world, tmp_path)
        built = world.builders.alignment_build(project, world.recorded_script, identifiers)
        defaults = world.reader.defaults()
        assert built["template_digest"] == defaults.alignment_template_digest
        items = [item for item in _items(built["items"]) if item["rejected"] is None]
        assert len(items) >= 2
        for item in items:
            self._check_item(world, item, identifiers, defaults.alignment_template_digest)

    @staticmethod
    def _check_item(world: GoWorld, item: Mapping[str, JsonValue], identifiers: list[str], digest: str) -> None:
        closed = cast(str, item["closed"])
        assert item["template_digest"] == digest
        rendered = _object(world.builders.render_closed(closed, identifiers)["bodies"], "bodies")
        assert _object(item["bodies"], "bodies") == rendered
        tokens = _object(item["token_to_ref"], "token_to_ref")
        assert tokens
        assert all(TOKEN_PATTERN.match(token) for token in tokens)
        criterion, coverage = _closed_questions(closed)
        names = _option_names(criterion)
        assert criterion["id"] == "criterion"
        assert names[-1] == NONE_OPTION
        assert sorted(names[:-1]) == sorted(tokens)
        assert coverage["id"] == "coverage"
        assert coverage["type"] == "score"
        assert tuple(_option_names(coverage)) == COVERAGE_LEVELS

    def test_the_sky_closed_set_reproduces_the_recorded_ollama_request_byte_for_byte(self, world: GoWorld) -> None:
        bodies = _object(world.builders.render_closed(SKY_CLOSED, [OLLAMA_MODEL])["bodies"], "bodies")
        assert bodies[OLLAMA_MODEL] == RECORDED_OLLAMA_CHOICE_REQUEST

    def test_a_closed_set_the_module_cannot_render_is_a_failure(self, world: GoWorld) -> None:
        with pytest.raises(BuilderFailure):
            world.builders.render_closed('{"state":"x","questions":[]}', [OLLAMA_MODEL])

    def test_defaults_name_the_route_threshold_and_the_template_digest(self, world: GoWorld) -> None:
        defaults = world.builders.alignment_defaults()
        assert 0.0 < cast(float, defaults["tau_route"]) <= 1.0
        assert world.reader.defaults().tau_route == defaults["tau_route"]


class TestDecode:
    def _decode(self, world: GoWorld, closed: str, reply: str) -> tuple[bool, str | None]:
        (outcome,) = world.reader.decode([DecodeRequest("k", closed, reply)])
        return outcome.accepted, outcome.stage

    def test_the_recorded_ollama_choice_reply_is_accepted_with_its_probabilities(self, world: GoWorld) -> None:
        (outcome,) = world.reader.decode([DecodeRequest("ollama", SKY_CLOSED, RECORDED_OLLAMA_CHOICE_REPLY)])
        assert outcome.accepted
        assert outcome.model == OLLAMA_MODEL
        (answer,) = outcome.answers
        assert (answer.question_id, answer.choice, answer.confidence) == ("color", "green", 0.9674686077174837)
        assert answer.probabilities == {
            "blue": 0.0028416608947914386,
            "green": 0.9948621078449252,
            "grey": 0.0022962312602833944,
        }

    def test_the_recorded_kev_choice_reply_is_accepted(self, world: GoWorld) -> None:
        (outcome,) = world.reader.decode([DecodeRequest("kev", SKY_CLOSED, RECORDED_KEV_CHOICE_REPLY)])
        assert outcome.accepted
        assert outcome.model == KEV_MODEL
        (answer,) = outcome.answers
        assert (answer.choice, answer.confidence) == ("green", 0.8802)
        assert answer.probabilities == {"blue": 0.0422, "green": 0.9202, "grey": 0.0377}

    def test_several_requests_come_back_in_order_with_their_keys(self, world: GoWorld) -> None:
        requests = [
            DecodeRequest("first", SKY_CLOSED, RECORDED_KEV_CHOICE_REPLY),
            DecodeRequest("second", SKY_CLOSED, RECORDED_OLLAMA_NOUL_REPLY),
        ]
        outcomes = world.reader.decode(requests)
        assert [outcome.key for outcome in outcomes] == ["first", "second"]
        assert [outcome.accepted for outcome in outcomes] == [True, False]
        assert world.reader.decode([]) == []

    def test_a_noul_reply_to_a_choice_is_refused_with_a_stage(self, world: GoWorld) -> None:
        accepted, stage = self._decode(world, SKY_CLOSED, RECORDED_OLLAMA_NOUL_REPLY)
        assert not accepted
        assert stage

    def test_a_score_outside_the_levels_is_refused_with_a_stage(self, world: GoWorld) -> None:
        accepted, stage = self._decode(world, COVERAGE_CLOSED, OUT_OF_RANGE_SCORE_REPLY)
        assert not accepted
        assert stage

    def test_a_closed_set_the_relay_cannot_carry_is_refused_at_the_closed_stage(self, world: GoWorld) -> None:
        accepted, stage = self._decode(world, NOUL_CLOSED, RECORDED_OLLAMA_NOUL_REPLY)
        assert not accepted
        assert stage == "closed"


class TestEndpointDigest:
    @pytest.mark.parametrize("address", ["http://127.0.0.1:11435/v1/systemone", "http://localhost:11434/v1/systemone"])
    def test_the_digest_is_the_hash_of_the_canonical_address(self, world: GoWorld, address: str) -> None:
        result = world.reader.endpoint_digest(address, OLLAMA_MODEL)
        digest = hashlib.sha256(_canonical_endpoint(address).encode()).hexdigest()
        assert result.address == address
        assert result.digest == digest
        assert result.source == f"system-one:{OLLAMA_MODEL}@{digest}"

    def test_an_address_that_is_not_http_is_a_failure(self, world: GoWorld) -> None:
        with pytest.raises(BuilderFailure):
            world.reader.endpoint_digest("ftp://localhost/v1/systemone", OLLAMA_MODEL)


def _synthetic_report() -> str:
    requirements: list[dict[str, JsonValue]] = [
        {
            "id": identifier,
            "revision": 1,
            "ref": SYNTHETIC_REF_FORMAT.format(id=identifier),
            "statement": statement,
            "status": "active",
            "statement_digest": SYNTHETIC_DIGEST_FORMAT.format(index=index),
            "tests": [],
        }
        for index, (identifier, statement) in enumerate(SYNTHETIC_REQUIREMENTS, start=1)
    ]
    return json.dumps({"schema_version": "1.2", "tool": "shallnot", "requirements": requirements})


def _synthetic_shallnot(directory: Path) -> tuple[Path, Path]:
    """A project directory and a `shallnot` that writes the synthetic report wherever `--json-out` points."""
    report = directory / "report.json"
    report.write_text(_synthetic_report(), encoding="utf-8")
    script = directory / "shallnot"
    script.write_text(
        '#!/bin/sh\nwhile [ "$#" -gt 0 ]; do\n  if [ "$1" = "--json-out" ]; then out="$2"; fi\n  shift\ndone\n'
        f'cp "{report}" "$out"\n',
        encoding="utf-8",
    )
    script.chmod(0o755)
    project = directory / "project"
    project.mkdir()
    return project, script


WITNESS: Final[dict[str, JsonValue]] = {
    "call": {"operation": "price", "args": {"quantity": {"type": "integer", "value": "3"}}},
    "reference": {"kind": "returns", "type": "integer", "value": "6"},
    "mutant": {"kind": "raises", "value": "ValueError"},
}
EDIT_LATE: Final[dict[str, JsonValue]] = {
    "file": "src/pkg/shipping.py",
    "qualname": "surcharge",
    "before": "def surcharge(zone):\n    return 5\n",
    "after": "def surcharge(zone):\n    return 0\n",
}
EDIT_EARLY: Final[dict[str, JsonValue]] = {
    "file": "src/pkg/pricing.py",
    "qualname": "Cart.total",
    "before": "def total(self):\n    return 1\n",
    "after": "def total(self):\n    return 2\n",
}
MUTANTS: Final[list[Mapping[str, JsonValue]]] = [
    {"mutant_id": "aaaaaaaaaaaaaaaa", "edits": [EDIT_LATE, EDIT_EARLY], "witnesses": [WITNESS]},
    {"mutant_id": "bbbbbbbbbbbbbbbb", "edits": [EDIT_EARLY], "witnesses": []},
]


class TestScreenBuilder:
    def test_a_screen_request_asks_one_question_per_scrubbed_statement(self, world: GoWorld, tmp_path: Path) -> None:
        project, script = _synthetic_shallnot(tmp_path)
        identifiers = [OLLAMA_MODEL, KEV_MODEL]
        built = world.builders.screen_build(project, script, identifiers, MUTANTS)
        digest = world.reader.defaults().screen_template_digest
        assert built["template_digest"] == digest
        assert built["refs"] == [SYNTHETIC_REF_FORMAT.format(id=identifier) for identifier, _ in SYNTHETIC_REQUIREMENTS]
        statements = cast(list[str], built["statements"])
        assert len(statements) == len(SYNTHETIC_REQUIREMENTS)
        assert not any(identifier in statement for statement in statements for identifier, _ in SYNTHETIC_REQUIREMENTS)
        late, plain = _items(built["items"])
        assert [late["mutant_id"], plain["mutant_id"]] == ["aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"]
        for item in (late, plain):
            self._check_questions(world, item, identifiers, digest)
        self._check_change(late, plain)

    @staticmethod
    def _check_questions(world: GoWorld, item: Mapping[str, JsonValue], identifiers: list[str], digest: str) -> None:
        assert item["template_digest"] == digest
        assert item["question_to_ref"] == {
            f"q{index}": SYNTHETIC_REF_FORMAT.format(id=identifier)
            for index, (identifier, _) in enumerate(SYNTHETIC_REQUIREMENTS, start=1)
        }
        questions = _closed_questions(item["closed"])
        assert [question["id"] for question in questions] == ["q1", "q2", "q3"]
        assert all(_option_names(question) == ["VIOLATES", "CONSISTENT"] for question in questions)
        rendered = world.builders.render_closed(cast(str, item["closed"]), identifiers)["bodies"]
        assert item["bodies"] == rendered

    @staticmethod
    def _check_change(late: Mapping[str, JsonValue], plain: Mapping[str, JsonValue]) -> None:
        state = cast(str, _object(json.loads(cast(str, late["closed"])), "closed")["state"])
        assert state.index("src/pkg/pricing.py :: Cart.total") < state.index("src/pkg/shipping.py :: surcharge")
        assert "- price(quantity=3): original 6, mutant raises ValueError" in state
        assert "Observed differences" not in cast(
            str, _object(json.loads(cast(str, plain["closed"])), "closed")["state"]
        )

    def test_the_defaults_name_the_screen_threshold(self, world: GoWorld) -> None:
        defaults = world.builders.screen_defaults()
        assert defaults["tau_screen"] == world.reader.defaults().tau_screen
        assert world.reader.defaults().tau_screen == 0.20

    def test_two_edits_of_one_unit_are_a_failure(self, world: GoWorld, tmp_path: Path) -> None:
        project, script = _synthetic_shallnot(tmp_path)
        duplicated: Mapping[str, JsonValue] = {"mutant_id": "cccccccccccccccc", "edits": [EDIT_EARLY, EDIT_EARLY]}
        with pytest.raises(BuilderFailure):
            world.builders.screen_build(project, script, [OLLAMA_MODEL], [duplicated])


def _answer_document(question: str, choice: str, probabilities: Mapping[str, float] | None) -> str:
    entry: dict[str, object] = {"id": question, "choice": choice}
    if probabilities is not None:
        entry["probabilities"] = dict(probabilities)
    return json.dumps({"answers": [entry]})


class TestScreenRead:
    def test_the_probability_is_the_stated_one_or_the_choice_as_zero_or_one(self, world: GoWorld) -> None:
        stated = {"VIOLATES": 0.8, "CONSISTENT": 0.2}
        requests = [
            ScreenReadRequest("stated", _answer_document("q1", "CONSISTENT", stated), "q1"),
            ScreenReadRequest("violates", _answer_document("q1", "VIOLATES", None), "q1"),
            ScreenReadRequest("consistent", _answer_document("q1", "CONSISTENT", None), "q1"),
        ]
        stated_reading, violates, consistent = world.reader.read_screen(requests)
        assert (stated_reading.error, stated_reading.has_probability) == (None, True)
        assert (stated_reading.violates_returned, stated_reading.violation_probability) == (0.8, 0.8)
        assert stated_reading.choice == "CONSISTENT"
        assert (violates.has_probability, violates.violation_probability, violates.choice) == (False, 1.0, "VIOLATES")
        assert (consistent.has_probability, consistent.violation_probability) == (False, 0.0)
        assert [reading.key for reading in (stated_reading, violates, consistent)] == [
            "stated",
            "violates",
            "consistent",
        ]

    def test_an_unknown_question_is_an_error_of_that_request_only(self, world: GoWorld) -> None:
        document = _answer_document("q1", "VIOLATES", None)
        unknown, known = world.reader.read_screen(
            [ScreenReadRequest("unknown", document, "q9"), ScreenReadRequest("known", document, "q1")]
        )
        assert unknown.error
        assert known.error is None
        assert world.reader.read_screen([]) == []

    def test_a_document_that_is_not_json_is_a_failure(self, world: GoWorld) -> None:
        with pytest.raises(BuilderFailure):
            world.reader.read_screen([ScreenReadRequest("bad", "{", "q1")])


THRESHOLD_SOURCE: Final = "system-one:m@d"


class TestCheckThreshold:
    def test_an_entry_resolves_its_own_source_and_leaves_every_other_at_the_default(self, world: GoWorld) -> None:
        check = world.reader.check_threshold(f'"{THRESHOLD_SOURCE}": 0.15\n', 0.20, ["system-one:other@d"])
        assert check.default == 0.20
        assert check.by_source == {THRESHOLD_SOURCE: 0.15}
        assert check.resolved == {THRESHOLD_SOURCE: 0.15, "system-one:other@d": 0.20}

    def test_a_bare_number_sets_the_default(self, world: GoWorld) -> None:
        check = world.reader.check_threshold("0.3\n", 0.20, [THRESHOLD_SOURCE])
        assert (check.default, check.resolved) == (0.3, {THRESHOLD_SOURCE: 0.3})

    @pytest.mark.parametrize(
        "entry", [f'"{THRESHOLD_SOURCE}": 1.5\n', f'"{THRESHOLD_SOURCE}": -0.1\n', '"made-up:x": 0.1\n']
    )
    def test_a_value_outside_the_unit_interval_or_an_unknown_source_kind_is_rejected(
        self, world: GoWorld, entry: str
    ) -> None:
        with pytest.raises(BuilderFailure):
            world.reader.check_threshold(entry, 0.20, [THRESHOLD_SOURCE])
