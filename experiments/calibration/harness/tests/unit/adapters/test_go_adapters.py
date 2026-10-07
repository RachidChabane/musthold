"""Command construction of the Go adapters against a fake process runner: no Go, no subprocess."""

from __future__ import annotations

import json
import os
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import Final

import pytest
from hypothesis import given
from hypothesis import strategies as st

from calibration.adapters.alignment_driver import BinaryAlignmentDriver
from calibration.adapters.go_builders import BinaryBuilders, BinaryProductReader, BuilderFailure, screen_read_input
from calibration.adapters.go_layout import (
    ALIGNMENT_BUILDER_BINARY,
    BUILDER_TARGETS,
    DECODE_BINARY,
    DISCPOWER_BUILDER_BINARY,
    PRODUCT_ALIGNMENT_BINARY,
)
from calibration.adapters.go_toolchain import GoBuilderToolchain
from calibration.domain.canonical import JsonValue, sha256_hex
from calibration.domain.ports import DecodeRequest, ProcessResult, ScreenReadRequest

BIN: Final = Path("/opt/bin")
GUARD: Final = ["/repo/script/mem-guard", "run", "--"]
CLOSED_WITH_UNICODE: Final = '{"state":"café \\" \\n ☃","questions":[]}'


@dataclass
class Call:
    argv: list[str]
    cwd: Path | None
    env: Mapping[str, str] | None
    stdin: bytes | None


@dataclass
class FakeRunner:
    responder: Callable[[Call], ProcessResult] = lambda call: ProcessResult(0, b"{}", b"")
    calls: list[Call] = field(default_factory=lambda: [])

    def run(
        self,
        argv: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        unset: Sequence[str] = (),
        input_bytes: bytes | None = None,
        timeout_seconds: float | None = None,
    ) -> ProcessResult:
        call = Call(list(argv), cwd, env, input_bytes)
        self.calls.append(call)
        return self.responder(call)


def json_reply(value: object) -> Callable[[Call], ProcessResult]:
    return lambda call: ProcessResult(0, json.dumps(value).encode(), b"")


class TestGoBuilderToolchain:
    @staticmethod
    def _tree(tmp_path: Path) -> tuple[Path, Path, Path]:
        builder_dir, export_dir, output_dir = tmp_path / "builder", tmp_path / "export", tmp_path / "bin"
        for target in BUILDER_TARGETS:
            source = builder_dir / target.source
            source.parent.mkdir(parents=True, exist_ok=True)
            source.write_text(f"package main // {target.source}\n", encoding="utf-8")
        export_dir.mkdir()
        return builder_dir, export_dir, output_dir

    @staticmethod
    def _compiling(call: Call) -> ProcessResult:
        Path(call.argv[call.argv.index("-o") + 1]).write_bytes(b"binary " + call.argv[-1].encode())
        return ProcessResult(0, b"", b"")

    def test_each_builder_is_copied_beside_its_module_and_compiled_under_the_guard(self, tmp_path: Path) -> None:
        builder_dir, export_dir, output_dir = self._tree(tmp_path)
        runner = FakeRunner(self._compiling)
        toolchain = GoBuilderToolchain(runner, GUARD, {"GOPROXY": "off"})

        built = toolchain.build(export_dir, builder_dir, output_dir, 3)

        assert (export_dir / "alignment/cmd/calibration-build/main.go").read_text().endswith("alignment/main.go\n")
        assert (export_dir / "discpower/cmd/calibration-build/main.go").is_file()
        assert (export_dir / "s1relay/cmd/calibration-decode/main.go").is_file()
        assert [call.argv[len(GUARD) :] for call in runner.calls] == [
            ["nice", "-n", "10", "go", "build", "-p", "3", "-trimpath", "-o", str(output_dir / binary), package]
            for binary, package in (
                (ALIGNMENT_BUILDER_BINARY, "./cmd/calibration-build"),
                (DISCPOWER_BUILDER_BINARY, "./cmd/calibration-build"),
                (DECODE_BINARY, "./cmd/calibration-decode"),
                (PRODUCT_ALIGNMENT_BINARY, "./cmd/spec-proof-alignment"),
            )
        ]
        assert all(call.argv[: len(GUARD)] == GUARD for call in runner.calls)
        assert [call.cwd for call in runner.calls] == [
            export_dir / "alignment",
            export_dir / "discpower",
            export_dir / "s1relay",
            export_dir / "alignment",
        ]
        assert all(call.env == {"GOMAXPROCS": "2", "GOPROXY": "off"} for call in runner.calls)
        assert set(built.source_digests) == {"alignment/main.go", "discpower/main.go", "s1relay/main.go"}
        assert built.source_digests["s1relay/main.go"] == sha256_hex(b"package main // s1relay/main.go\n")
        assert set(built.binary_digests) == {ALIGNMENT_BUILDER_BINARY, DISCPOWER_BUILDER_BINARY, DECODE_BINARY}
        expected = sha256_hex(b"binary ./cmd/calibration-decode")
        assert built.binary_digests[DECODE_BINARY] == expected

    def test_an_empty_guard_leaves_the_command_under_nice_only(self, tmp_path: Path) -> None:
        builder_dir, export_dir, output_dir = self._tree(tmp_path)
        runner = FakeRunner(self._compiling)
        GoBuilderToolchain(runner, [], {}).build(export_dir, builder_dir, output_dir, 1)
        assert runner.calls[0].argv[:5] == ["nice", "-n", "10", "go", "build"]

    def test_a_compiler_failure_carries_the_compiler_output(self, tmp_path: Path) -> None:
        builder_dir, export_dir, output_dir = self._tree(tmp_path)
        runner = FakeRunner(lambda call: ProcessResult(1, b"", b"main.go:3: undefined: Foo"))
        with pytest.raises(BuilderFailure, match="undefined: Foo"):
            GoBuilderToolchain(runner, [], {}).build(export_dir, builder_dir, output_dir, 1)

    def test_a_missing_builder_source_is_an_error(self, tmp_path: Path) -> None:
        _, export_dir, output_dir = self._tree(tmp_path)
        with pytest.raises(FileNotFoundError):
            GoBuilderToolchain(FakeRunner(), [], {}).build(export_dir, tmp_path / "nowhere", output_dir, 1)


class TestBinaryBuilders:
    def test_alignment_build_passes_the_gateway_flags_and_the_environment(self, tmp_path: Path) -> None:
        runner = FakeRunner(json_reply({"items": []}))
        builders = BinaryBuilders(runner, BIN, {"SHALLNOT_RECORDED_REPORT": "/r.json"})
        result = builders.alignment_build(tmp_path / "p", tmp_path / "s", ["tev1:0.8b", "kev-latest"])
        (call,) = runner.calls
        assert result == {"items": []}
        assert call.argv == [
            str(BIN / ALIGNMENT_BUILDER_BINARY),
            "build",
            "--project",
            str(tmp_path / "p"),
            "--shallnot",
            str(tmp_path / "s"),
            "--models",
            "tev1:0.8b,kev-latest",
        ]
        assert call.env == {"SHALLNOT_RECORDED_REPORT": "/r.json"}

    def test_render_closed_sends_the_closed_text_verbatim_and_returns_the_bodies_verbatim(self) -> None:
        body = '{"model":"m","state":"café"}'
        runner = FakeRunner(json_reply({"bodies": {"m": body}}))
        result = BinaryBuilders(runner, BIN).render_closed(CLOSED_WITH_UNICODE, ["m"])
        (call,) = runner.calls
        assert call.stdin == CLOSED_WITH_UNICODE.encode()
        assert call.argv[1:] == ["render", "--models", "m"]
        assert result == {"bodies": {"m": body}}

    def test_compare_export_sends_the_export_text_as_it_is(self, tmp_path: Path) -> None:
        runner = FakeRunner(json_reply({"matched": 1}))
        BinaryBuilders(runner, BIN).compare_export(tmp_path, tmp_path / "s", '[ {"ticket":"t"} ]')
        assert runner.calls[0].stdin == b'[ {"ticket":"t"} ]'
        assert runner.calls[0].argv[1] == "compare-export"

    def test_screen_build_sends_the_mutants_to_the_discpower_builder(self, tmp_path: Path) -> None:
        runner = FakeRunner(json_reply({"items": []}))
        mutants: list[Mapping[str, JsonValue]] = [{"mutant_id": "a" * 16, "edits": [], "witnesses": []}]
        BinaryBuilders(runner, BIN).screen_build(tmp_path, tmp_path / "s", ["m"], mutants)
        (call,) = runner.calls
        assert call.argv[0] == str(BIN / DISCPOWER_BUILDER_BINARY)
        assert call.stdin is not None
        assert json.loads(call.stdin) == {"mutants": mutants}

    def test_the_defaults_modes_take_no_input(self) -> None:
        runner = FakeRunner(json_reply({"tau_route": 0.5}))
        builders = BinaryBuilders(runner, BIN)
        builders.alignment_defaults()
        builders.screen_defaults()
        assert [(call.argv[0], call.argv[1:], call.stdin) for call in runner.calls] == [
            (str(BIN / ALIGNMENT_BUILDER_BINARY), ["defaults"], None),
            (str(BIN / DISCPOWER_BUILDER_BINARY), ["defaults"], None),
        ]

    def test_a_non_zero_exit_is_a_failure_with_the_error_output(self, tmp_path: Path) -> None:
        runner = FakeRunner(lambda call: ProcessResult(1, b"", b"calibration-build: no criteria"))
        with pytest.raises(BuilderFailure, match="no criteria") as raised:
            BinaryBuilders(runner, BIN).alignment_build(tmp_path, tmp_path, ["m"])
        assert raised.value.stderr == "calibration-build: no criteria"

    @pytest.mark.parametrize("stdout", [b"not json", b"[1, 2]", b""])
    def test_an_output_that_is_not_a_json_object_is_a_failure(self, tmp_path: Path, stdout: bytes) -> None:
        runner = FakeRunner(lambda call: ProcessResult(0, stdout, b""))
        with pytest.raises(BuilderFailure):
            BinaryBuilders(runner, BIN).alignment_build(tmp_path, tmp_path, ["m"])


DECODE_REPLY: Final = [
    {
        "key": "a",
        "status": "ok",
        "model": "m",
        "document": "{}",
        "answers": [
            {"id": "q", "choice": "x", "confidence": 0.5, "probabilities": {"x": 0.5, "y": 0.5}},
            {"id": "r", "choice": "y", "confidence": None},
        ],
    },
    {"key": "b", "status": "relay_refused", "stage": "reply", "error": "bad"},
]


class TestBinaryProductReader:
    def test_decode_maps_every_result_and_sends_the_texts_unchanged(self) -> None:
        runner = FakeRunner(json_reply(DECODE_REPLY))
        requests = [DecodeRequest("a", CLOSED_WITH_UNICODE, '{"r": 1.0}'), DecodeRequest("b", "c", "r")]
        accepted, refused = BinaryProductReader(runner, BIN).decode(requests)
        (call,) = runner.calls
        assert call.argv == [str(BIN / DECODE_BINARY), "decode"]
        assert call.stdin is not None
        assert json.loads(call.stdin) == [
            {"key": "a", "closed": CLOSED_WITH_UNICODE, "reply": '{"r": 1.0}'},
            {"key": "b", "closed": "c", "reply": "r"},
        ]
        assert (accepted.accepted, accepted.stage, accepted.error, accepted.model) == (True, None, None, "m")
        first, second = accepted.answers
        assert (first.question_id, first.choice, first.confidence) == ("q", "x", 0.5)
        assert dict(first.probabilities) == {"x": 0.5, "y": 0.5}
        assert (second.confidence, dict(second.probabilities)) == (None, {})
        assert (refused.accepted, refused.stage, refused.error, refused.answers) == (False, "reply", "bad", ())

    def test_no_request_runs_no_binary(self) -> None:
        runner = FakeRunner()
        reader = BinaryProductReader(runner, BIN)
        assert reader.decode([]) == []
        assert reader.read_screen([]) == []
        assert runner.calls == []

    def test_read_screen_splices_each_document_as_the_text_it_is(self) -> None:
        reply = [
            {
                "key": "k",
                "choice": "VIOLATES",
                "has_probability": False,
                "violates_returned": 0,
                "violation_probability": 1,
            }
        ]
        runner = FakeRunner(json_reply(reply))
        document = '{"answers":[{"id":"q1","choice":"VIOLATES","confidence":1.0e0}]}'
        (reading,) = BinaryProductReader(runner, BIN).read_screen([ScreenReadRequest("k", document, "q1")])
        (call,) = runner.calls
        assert call.stdin == f'[{{"key":"k","document":{document},"question":"q1"}}]'.encode()
        assert (reading.key, reading.error, reading.choice, reading.has_probability) == ("k", None, "VIOLATES", False)
        assert (reading.violates_returned, reading.violation_probability) == (0.0, 1.0)

    def test_read_screen_refuses_a_document_that_is_not_json(self) -> None:
        with pytest.raises(BuilderFailure, match="bad"):
            BinaryProductReader(FakeRunner(), BIN).read_screen([ScreenReadRequest("bad", "{", "q1")])

    def test_read_screen_keeps_the_error_of_an_unanswered_question(self) -> None:
        reply = [
            {
                "key": "k",
                "error": "the document does not answer q9",
                "has_probability": False,
                "violates_returned": 0,
                "violation_probability": 0,
            }
        ]
        (reading,) = BinaryProductReader(FakeRunner(json_reply(reply)), BIN).read_screen(
            [ScreenReadRequest("k", "{}", "q9")]
        )
        assert reading.error == "the document does not answer q9"
        assert reading.choice is None

    def test_endpoint_digest_passes_the_address_and_the_model(self) -> None:
        runner = FakeRunner(json_reply({"address": "http://a/v1/systemone", "digest": "d", "source": "system-one:m@d"}))
        result = BinaryProductReader(runner, BIN).endpoint_digest("http://a", "m")
        assert runner.calls[0].argv[1:] == ["endpoint-digest", "--address", "http://a", "--model", "m"]
        assert (result.address, result.digest, result.source) == ("http://a/v1/systemone", "d", "system-one:m@d")

    def test_check_threshold_sends_the_entry_and_the_default_and_the_sources(self) -> None:
        reply = {"default": 0.2, "by_source": {"s": 0.15}, "resolved": {"s": 0.15, "o": 0.2}}
        runner = FakeRunner(json_reply(reply))
        check = BinaryProductReader(runner, BIN).check_threshold('"s": 0.15\n', 0.2, ["s", "o"])
        (call,) = runner.calls
        assert call.argv == [
            str(BIN / DISCPOWER_BUILDER_BINARY),
            "check-threshold",
            "--default",
            "0.2",
            "--sources",
            "s,o",
        ]
        assert call.stdin == b'"s": 0.15\n'
        assert (check.default, dict(check.by_source), dict(check.resolved)) == (0.2, {"s": 0.15}, {"s": 0.15, "o": 0.2})

    def test_check_threshold_failure_is_a_builder_failure(self) -> None:
        runner = FakeRunner(lambda call: ProcessResult(1, b"", b"threshold out of range"))
        with pytest.raises(BuilderFailure, match="out of range"):
            BinaryProductReader(runner, BIN).check_threshold("1.5", 0.2, [])

    def test_defaults_combines_the_two_builders(self) -> None:
        def respond(call: Call) -> ProcessResult:
            if call.argv[0].endswith(ALIGNMENT_BUILDER_BINARY):
                return json_reply({"tau_route": 0.6, "template_digests": {"decide.closed": "aligned"}})(call)
            return json_reply({"tau_screen": 0.2, "template_digests": {"decide.closed": "screened"}})(call)

        defaults = BinaryProductReader(FakeRunner(respond), BIN).defaults()
        assert (defaults.tau_route, defaults.tau_screen) == (0.6, 0.2)
        assert (defaults.alignment_template_digest, defaults.screen_template_digest) == ("aligned", "screened")


class TestBinaryAlignmentDriver:
    @staticmethod
    def _export(tmp_path: Path) -> Path:
        project = tmp_path / "export" / "s1relay" / "testdata" / "alignment" / "project"
        project.mkdir(parents=True)
        (project / "marker.txt").write_text("fixture", encoding="utf-8")
        return tmp_path / "export"

    def test_prove_then_export_run_in_a_private_copy_with_the_product_environment(self, tmp_path: Path) -> None:
        seen: list[tuple[list[str], Path | None, Mapping[str, str] | None, bool]] = []

        def respond(call: Call) -> ProcessResult:
            assert call.cwd is not None
            seen.append((call.argv, call.cwd, call.env, (call.cwd / "marker.txt").is_file()))
            return ProcessResult(3 if call.argv[1] == "prove" else 0, b'[{"ticket":"t"}]', b"")

        script, report = tmp_path / "shallnot.sh", tmp_path / "report.json"
        text = BinaryAlignmentDriver(FakeRunner(respond), BIN).first_wave_export(self._export(tmp_path), script, report)
        assert text == '[{"ticket":"t"}]'
        binary = str(BIN / PRODUCT_ALIGNMENT_BINARY)
        assert [entry[0] for entry in seen] == [[binary, "prove", "--all"], [binary, "systemone", "export"]]
        assert seen[0][1] == seen[1][1]
        assert all(entry[3] for entry in seen)
        env = seen[0][2]
        assert env is not None
        assert env["PATH"].startswith(str(BIN) + os.pathsep)
        assert env["SPEC_PROOF_SHALLNOT"] == str(script)
        assert env["SHALLNOT_RECORDED_REPORT"] == str(report)
        scratch = Path(env["HOME"]).parent
        assert env["XDG_STATE_HOME"] == str(scratch / "state")
        assert env["XDG_CACHE_HOME"] == str(scratch / "cache")
        assert not scratch.exists()

    @pytest.mark.parametrize(("prove_exit", "export_exit"), [(0, 0), (2, 0), (3, 1)])
    def test_an_unexpected_exit_code_is_a_failure_with_the_error_output(
        self, tmp_path: Path, prove_exit: int, export_exit: int
    ) -> None:
        def respond(call: Call) -> ProcessResult:
            return ProcessResult(prove_exit if call.argv[1] == "prove" else export_exit, b"", b"tool said no")

        driver = BinaryAlignmentDriver(FakeRunner(respond), BIN)
        with pytest.raises(BuilderFailure, match="tool said no"):
            driver.first_wave_export(self._export(tmp_path), tmp_path / "s", tmp_path / "r")


KEYS: Final = st.text(min_size=1, max_size=12)
DOCUMENTS: Final = st.recursive(
    st.none() | st.booleans() | st.integers() | st.floats(allow_nan=False, allow_infinity=False) | st.text(),
    lambda inner: st.lists(inner, max_size=3) | st.dictionaries(st.text(max_size=6), inner, max_size=3),
    max_leaves=8,
)


@given(st.lists(st.tuples(KEYS, DOCUMENTS, KEYS), min_size=1, max_size=4))
def test_thescreen_read_input_is_the_list_of_its_requests(entries: list[tuple[str, object, str]]) -> None:
    requests = [ScreenReadRequest(key, json.dumps(document), question) for key, document, question in entries]
    parsed = json.loads(screen_read_input(requests))
    assert parsed == [{"key": key, "document": document, "question": question} for key, document, question in entries]
