import dataclasses
from pathlib import PurePosixPath

import pytest

from demos1.domain.commands import (
    ProjectFacts,
    ScanName,
    clone_command,
    install_command,
    pyproject_tables,
    replay_command,
    scan_command,
    suite_command,
)
from demos1.domain.container import ContainerCommand
from demos1.domain.ecosystem import Ecosystem
from demos1.domain.layout import SetupStage, WorkLayout, attempt_id, replay_container, scan_container, setup_container
from demos1.domain.lists import ListName, Stratum
from demos1.domain.machine import GIB, MachineMounts, over_cap, scratch_paths, start_argv
from demos1.domain.outcomes import (
    CommandEnd,
    ReplayResult,
    ScanOutcome,
    SetupOutcome,
    replay_result,
    scan_outcome,
    setup_failed,
)
from demos1.domain.suites import SecondRun, SuiteCounts, go_counts, junit_counts, second_run_outcome
from demos1.domain.toolchains import (
    GoDirectives,
    PythonRule,
    go_directives,
    go_exceeds_image,
    installed_cpython,
    python_request,
)

UV = {"3.13": "3.13.7", "3.12": "3.12.11", "3.11": "3.11.13", "3.10": "3.10.18"}


def end(**overrides: object) -> CommandEnd:
    values: dict[str, object] = {
        "exit_code": 0,
        "oom_killed": False,
        "timed_out": False,
        "killed_at_cap": False,
        "interrupted_out": False,
    }
    values.update(overrides)
    return CommandEnd(**values)  # type: ignore[arg-type]


class TestGoToolchain:
    def test_directives_read_go_and_toolchain_lines(self):
        directives = go_directives("module example.com/q\n\ngo 1.25.0 // floor\n\ntoolchain go1.26.4\n")
        assert directives == GoDirectives("1.25.0", "1.26.4")

    @pytest.mark.parametrize(
        ("go", "toolchain", "exceeds"),
        [
            ("1.26", None, False),
            ("1.26.4", None, False),
            ("1.26.5", None, True),
            ("1.27rc1", None, True),
            ("1.24", "1.26.5", True),
            ("1.24", "1.26.4", False),
        ],
    )
    def test_a_version_past_the_image_go_is_a_setup_toolchain(self, go: str, toolchain: str | None, exceeds: bool):
        assert go_exceeds_image(GoDirectives(go, toolchain)) is exceeds


class TestPythonRequest:
    def test_the_python_version_file_comes_first(self):
        request = python_request("\n3.12\n", '[project]\nrequires-python = ">=3.13"\n', None, UV)
        assert (request.rule, request.request) == (PythonRule.PYTHON_VERSION_FILE, "3.12")

    def test_requires_python_takes_the_highest_satisfying_minor(self):
        request = python_request(None, '[project]\nrequires-python = ">=3.9,<3.13"\n', None, UV)
        assert (request.rule, request.request) == (PythonRule.REQUIRES_PYTHON, "3.12")

    def test_the_minor_is_judged_by_the_release_uv_selects(self):
        request = python_request(None, '[project]\nrequires-python = "<3.13.5"\n', None, UV)
        assert request.request == "3.12"

    def test_setup_cfg_when_pyproject_has_no_specifier(self):
        setup_cfg = "[options]\npython_requires = >=3.10, <3.12\n"
        request = python_request(None, "[project]\nname = 'x'\n", setup_cfg, UV)
        assert (request.rule, request.request) == (PythonRule.PYTHON_REQUIRES, "3.11")

    def test_no_listed_version_satisfies(self):
        request = python_request(None, '[project]\nrequires-python = ">=3.14"\n', None, UV)
        assert request.request is None

    def test_default_is_3_13(self):
        assert python_request(None, None, None, UV).request == "3.13"

    def test_an_unparsable_pyproject_is_named(self):
        request = python_request(None, "[project\n", None, UV)
        assert request.unreadable == "pyproject.toml"
        assert request.request == "3.13"

    def test_installed_versions_come_from_directory_names(self):
        names = ["cpython-3.13.7-linux-aarch64-gnu", ".lock", "cpython-3.13.7+freethreaded-linux-aarch64-gnu"]
        assert installed_cpython(names) == ["3.13.7"]


class TestRecipes:
    def test_go_downloads_its_modules(self):
        assert install_command(Ecosystem.GO, ProjectFacts(frozenset(), None), None) == ["go", "mod", "download"]

    def test_uv_lock_syncs_frozen(self):
        script = install_command(Ecosystem.PYTEST, ProjectFacts(frozenset({"uv.lock"}), None), "3.12")[-1]
        assert "uv sync --frozen --all-extras --all-groups --python 3.12" in script
        assert 'if ! .venv/bin/python -c "import pytest"' in script

    def test_a_project_installs_its_listed_extras_and_groups_in_order(self):
        pyproject = (
            "[project]\nname='x'\n[project.optional-dependencies]\ndev=['a']\ntest=['pytest']\ndocs=['b']\n"
            "[dependency-groups]\ntesting=['c']\nlint=['d']\n"
        )
        facts = ProjectFacts(frozenset({"pyproject.toml", "requirements.txt", "requirements/test.txt"}), pyproject)
        lines = install_command(Ecosystem.PYTEST, facts, "3.13")[-1].splitlines()
        assert lines[1] == "uv venv .venv --python 3.13"
        assert lines[2] == "uv pip install --python .venv/bin/python -e '.[test,dev]'"
        assert lines[3] == "uv pip install --python .venv/bin/python --group testing"
        assert lines[4] == "uv pip install --python .venv/bin/python -r requirements/test.txt"
        assert not any("requirements.txt" in line and "-r requirements.txt" in line for line in lines)

    def test_without_a_project_requirements_txt_is_installed(self):
        facts = ProjectFacts(frozenset({"requirements.txt", "requirements-dev.txt"}), None)
        lines = install_command(Ecosystem.PYTEST, facts, "3.13")[-1].splitlines()
        assert "uv pip install --python .venv/bin/python -r requirements.txt" in lines
        assert "uv pip install --python .venv/bin/python -r requirements-dev.txt" in lines
        assert not any(" -e " in line for line in lines)

    def test_unparsable_pyproject_tables(self):
        assert pyproject_tables("[project").unreadable

    def test_the_clone_checks_the_base_for_a_b_pick(self):
        script = clone_command("https://github.com/north/ledger.git", "/w/clone", "m" * 40, "p" * 40)[-1]
        assert "git clone --no-checkout https://github.com/north/ledger.git /w/clone" in script
        assert f"git -C /w/clone checkout --detach {'m' * 40}" in script
        assert "submodule update --init --recursive" in script
        assert f"cat-file -e '{'p' * 40}^{{commit}}'" in script

    def test_suite_and_scan_commands(self):
        assert suite_command(Ecosystem.PYTEST, "/w/suite-2.xml") == [
            ".venv/bin/python",
            "-m",
            "pytest",
            "-p",
            "no:cacheprovider",
            "-q",
            "--junitxml=/w/suite-2.xml",
        ]
        assert suite_command(Ecosystem.GO, "/w/suite-1.jsonl")[-1].endswith(
            "go test -count=1 -p 2 -json ./... > /w/suite-1.jsonl"
        )
        base = "b" * 40
        assert scan_command("/w/clone", ScanName.CHANGE_FULL, base)[-6:] == [
            "text",
            "--base",
            base,
            "--full",
            "--budget",
            "60m",
        ]
        assert scan_command("/w/clone", ScanName.QUICK, None)[1:] == [
            "scan",
            "--root",
            "/w/clone",
            "--seed",
            "7",
            "--workers",
            "2",
            "--format",
            "text",
        ]
        with pytest.raises(ValueError):
            scan_command("/w/clone", ScanName.CHANGE, None)
        assert replay_command("/w/clone", "/w/scans/quick/moved/1") == [
            "spec-proof",
            "scan",
            "replay",
            "--root",
            "/w/clone",
            "/w/scans/quick/moved/1",
        ]


class TestSuites:
    def test_junit_counts(self):
        report = (
            "<testsuites><testsuite>"
            "<testcase name='a'/><testcase name='b'><failure/></testcase>"
            "<testcase name='c'><error/></testcase><testcase name='d'><skipped/></testcase>"
            "</testsuite></testsuites>"
        )
        assert junit_counts(report) == SuiteCounts(1, 1, 1, 1)
        assert not junit_counts("<oops").readable

    def test_go_counts_distinct_pairs_with_subtests(self):
        stream = "\n".join(
            [
                '{"Action":"pass","Package":"q","Test":"TestA"}',
                '{"Action":"pass","Package":"q","Test":"TestA/sub"}',
                '{"Action":"pass","Package":"q","Test":"TestA"}',
                '{"Action":"fail","Package":"q","Test":"TestB"}',
                '{"Action":"skip","Package":"q","Test":"TestC"}',
                '{"Action":"fail","Package":"q"}',
                '{"Action":"fail","Package":"broken"}',
                "not json",
            ]
        )
        suite = go_counts(stream)
        assert suite.counts == SuiteCounts(passing=2, failing=1, erroring=1, skipped=1)
        assert suite.named_pass

    @pytest.mark.parametrize(
        ("ecosystem", "run", "outcome"),
        [
            (Ecosystem.PYTEST, SecondRun(137, True, SuiteCounts(20, 0, 0, 0), False), SetupOutcome.SUITE_TOO_SLOW),
            (Ecosystem.PYTEST, SecondRun(2, False, SuiteCounts(20, 0, 0, 0), False), SetupOutcome.SUITE_DOES_NOT_RUN),
            (Ecosystem.PYTEST, SecondRun(1, False, SuiteCounts(11, 3, 0, 0), False), SetupOutcome.TOO_FEW_PASSING),
            (Ecosystem.PYTEST, SecondRun(1, False, SuiteCounts(12, 3, 0, 0), False), None),
            (Ecosystem.GO, SecondRun(1, False, SuiteCounts(0, 0, 1, 0), False), SetupOutcome.SUITE_DOES_NOT_RUN),
            (Ecosystem.GO, SecondRun(1, False, SuiteCounts(12, 1, 0, 0), True), None),
        ],
    )
    def test_second_run_outcomes(self, ecosystem: Ecosystem, run: SecondRun, outcome: SetupOutcome | None):
        assert second_run_outcome(ecosystem, run) == outcome


class TestOutcomes:
    def test_memory_limit_wins_over_any_exit(self):
        assert setup_failed(end(exit_code=0, oom_killed=True), SetupOutcome.INSTALL) is SetupOutcome.OOM
        assert scan_outcome(end(exit_code=0, oom_killed=True)) is ScanOutcome.OOM

    def test_three_interruptions(self):
        assert setup_failed(end(interrupted_out=True), SetupOutcome.CLONE) is SetupOutcome.INTERRUPTED
        assert scan_outcome(end(interrupted_out=True)) is ScanOutcome.INTERRUPTED
        assert replay_result(end(interrupted_out=True)) is ReplayResult.FAILED

    def test_scan_exits(self):
        assert scan_outcome(end()) is ScanOutcome.OK
        assert scan_outcome(end(exit_code=2)) is ScanOutcome.ERROR
        assert scan_outcome(end(exit_code=None, killed_at_cap=True)) is ScanOutcome.KILLED

    def test_replay_exits(self):
        assert replay_result(end()) is ReplayResult.HOLDS
        assert replay_result(end(exit_code=1)) is ReplayResult.NO_LONGER_HOLDS
        assert replay_result(end(exit_code=2)) is ReplayResult.FAILED
        assert replay_result(end(exit_code=0, oom_killed=True)) is ReplayResult.FAILED

    def test_a_limit_or_a_non_zero_exit_fails_the_stage(self):
        assert setup_failed(end(timed_out=True, exit_code=137), SetupOutcome.CLONE) is SetupOutcome.CLONE
        assert setup_failed(end(exit_code=128), SetupOutcome.CLONE) is SetupOutcome.CLONE
        assert setup_failed(end(), SetupOutcome.CLONE) is None


class TestContainer:
    def command(self, network: bool) -> ContainerCommand:
        return ContainerCommand(
            name="B-go-03-suite-2-r1",
            image="sha256:abc",
            attempt_dir="/work/attempts/B-go-03",
            binaries_dir="/product/bin",
            workdir="/work/attempts/B-go-03/clone",
            network=network,
            environment={"PATH": "/opt/spec-proof/bin:/usr/bin", "HOME": "/work/attempts/B-go-03/home"},
            argv=["go", "test"],
            record_prefix="/work/attempts/B-go-03/logs/suite-2-r1",
            limit_seconds=60,
            uid=502,
            gid=20,
        )

    def test_the_options_of_every_container_command(self):
        argv = self.command(network=False).docker_argv()
        assert "--rm" not in argv
        joined = " ".join(argv)
        for option in (
            "--name B-go-03-suite-2-r1",
            "--user 502:20",
            "--cap-drop ALL",
            "--security-opt no-new-privileges",
            "--cpus 2",
            "--memory 3g",
            "--memory-swap 3g",
            "--network none",
            "type=bind,source=/work/attempts/B-go-03,target=/work/attempts/B-go-03",
            "type=bind,source=/product/bin,target=/opt/spec-proof/bin,readonly",
        ):
            assert option in joined
        assert argv.count("--mount") == 2
        wrapped = argv[argv.index("sha256:abc") + 1 :]
        assert wrapped[:2] == ["/usr/bin/env", "-i"]
        assert wrapped[wrapped.index("--") + 1 :] == ["go", "test"]
        assert wrapped[wrapped.index("--limit") : wrapped.index("--limit") + 2] == ["--limit", "60"]

    def test_the_network_is_on_only_when_asked(self):
        assert "--network" not in self.command(network=True).docker_argv()

    def test_a_mount_path_with_a_comma_is_refused(self):
        with pytest.raises(ValueError):
            dataclasses.replace(self.command(True), attempt_dir="/work,odd")

    def test_the_environment_of_offline_clone_and_scan_commands(self):
        layout = WorkLayout(PurePosixPath("/work"), PurePosixPath("/product")).attempt("B-go-03")
        environment = layout.environment(PurePosixPath("/work/attempts/B-go-03/tmp/x"))
        offline = environment.variables(offline=True)
        assert offline["GOPROXY"] == "off"
        assert offline["UV_OFFLINE"] == "1"
        assert offline["GOFLAGS"] == "-p=2"
        assert offline["GOTOOLCHAIN"] == "local"
        assert "GIT_ALLOW_PROTOCOL" not in offline
        assert offline["GOMODCACHE"].startswith("/work/attempts/B-go-03/cache/go/")
        assert offline["UV_PYTHON_INSTALL_DIR"].startswith("/work/attempts/B-go-03/cache/uv/")
        clone = environment.variables(offline=False, clone_protocol="https")
        assert clone["GIT_ALLOW_PROTOCOL"] == "https"
        assert "GOPROXY" not in clone
        assert not any(name.endswith("TOKEN") for name in clone)


class TestNamesAndMachine:
    def test_names(self):
        identifier = attempt_id(ListName(Stratum.B, Ecosystem.GO), 3)
        assert identifier == "B-go-03"
        assert setup_container(identifier, SetupStage.CLONE, 1) == "B-go-03-clone-r1"
        assert scan_container(identifier, "change-full", 2) == "B-go-03-change-full-r2"
        assert replay_container(identifier, "quick", 4, 1) == "B-go-03-quick-replay-4-r1"

    def test_the_machine_neither_activates_a_context_nor_writes_ssh_config(self):
        argv = start_argv(MachineMounts(PurePosixPath("/home/w"), PurePosixPath("/home/p/bin")))
        assert argv[:4] == ["colima", "start", "--profile", "spec-proof-demos"]
        assert "--activate=false" in argv
        assert "--ssh-config=false" in argv
        assert argv[argv.index("--cpu") + 1] == "2"
        assert argv[argv.index("--memory") + 1] == "4"
        assert argv[argv.index("--disk") + 1] == "40"
        assert [argv[index + 1] for index, word in enumerate(argv) if word == "--mount"] == ["/home/w:w", "/home/p/bin"]

    def test_the_binaries_lie_outside_the_work_directory(self):
        with pytest.raises(ValueError):
            MachineMounts(PurePosixPath("/home/w"), PurePosixPath("/home/w/product/bin"))

    def test_the_disk_cap_is_strictly_above_15_gib(self):
        assert not over_cap(15 * GIB)
        assert over_cap(15 * GIB + 1)

    def test_scratch_paths_stay_inside_the_attempt(self):
        root = PurePosixPath("/w/attempts/A-go-01")
        paths = scratch_paths(root, root / "clone", root / "cache", root / "home", [str(root / "tmp" / "clone-r1")])
        assert root / "clone" in paths
        with pytest.raises(ValueError):
            scratch_paths(root, root / "clone", root / "cache", root / "home", ["/w/attempts"])
