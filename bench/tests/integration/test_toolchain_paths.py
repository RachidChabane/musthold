"""Toolchain directories resolved by the shared script against a fake `mise`, recorded in the run, put on a session PATH."""

from __future__ import annotations

import json
import stat
from pathlib import Path

from specbench.adapters.process import SubprocessRunner
from specbench.adapters.toolchain_paths import RESOLVER_SCRIPT, ToolchainPaths
from specbench.domain.manifest import RunLayout
from support.coder_rigs import TEST_TOKEN, make_claude_rig
from support.fake_claude import recorded_session_calls

FAKE_MISE = """#!/bin/bash
installs={installs}
case "$1" in
  which)
    [[ "$2" == go ]] && echo "$installs/go/bin/go" && exit 0
    exit 1 ;;
  ls)
    [[ "$4" == node ]] && echo '[{{"version": "20.1"}}, {{"version": "22.0"}}]' || echo '[]'
    exit 0 ;;
  where)
    [[ "$2" == node@22.0 ]] && echo "$installs/node/22.0" && exit 0
    exit 1 ;;
esac
exit 1
"""


def executable(path: Path, content: str = "#!/bin/sh\n") -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


def world(tmp_path: Path) -> tuple[Path, dict[str, str]]:
    installs = tmp_path / "installs"
    executable(installs / "go" / "bin" / "go")
    executable(installs / "node" / "22.0" / "bin" / "node")
    shims = tmp_path / "shims"
    executable(shims / "go")
    path_dir = tmp_path / "usr-bin"
    executable(path_dir / "uv")
    executable(tmp_path / "fakebin" / "mise", FAKE_MISE.format(installs=installs))
    environ = {"PATH": f"{tmp_path / 'fakebin'}:{shims}:{path_dir}:/usr/bin:/bin"}
    return installs, environ


def test_each_tool_resolves_through_mise_then_the_newest_install_then_the_path(tmp_path: Path) -> None:
    installs, environ = world(tmp_path)
    layout = RunLayout(tmp_path / "run")

    resolved = ToolchainPaths(SubprocessRunner(), layout, environ, tools=("go", "node", "uv", "ghost")).resolve()

    assert resolved == {
        "go": str(installs / "go" / "bin"),
        "node": str(installs / "node" / "22.0" / "bin"),
        "uv": str(tmp_path / "usr-bin"),
    }


def test_a_tool_found_only_in_a_shims_directory_is_not_resolved(tmp_path: Path) -> None:
    _, environ = world(tmp_path)
    executable(tmp_path / "shims" / "shimmed")

    resolved = ToolchainPaths(SubprocessRunner(), RunLayout(tmp_path / "run"), environ, tools=("shimmed",)).resolve()

    assert resolved == {}


def test_the_resolution_is_recorded_and_a_resumed_run_reuses_it(tmp_path: Path) -> None:
    installs, environ = world(tmp_path)
    layout = RunLayout(tmp_path / "run")
    first = ToolchainPaths(SubprocessRunner(), layout, environ, tools=("go",))
    assert first.resolve() == {"go": str(installs / "go" / "bin")}
    assert json.loads(layout.toolchain_paths.read_text()) == {"tools": {"go": str(installs / "go" / "bin")}}

    (installs / "go" / "bin" / "go").unlink()
    resumed = ToolchainPaths(SubprocessRunner(), layout, {"PATH": "/usr/bin:/bin"}, tools=("go",))

    assert resumed.resolve() == {"go": str(installs / "go" / "bin")}
    assert resumed.directories() == (str(installs / "go" / "bin"),)


def test_the_shipped_resolver_script_is_the_one_the_night_jobs_use() -> None:
    assert RESOLVER_SCRIPT.is_file()
    installer = RESOLVER_SCRIPT.parent / "install-night-jobs"
    assert "script/resolve-tool-dirs" in installer.read_text()


def test_a_coder_session_gets_the_tool_directories_first_and_no_shims_directory(tmp_path: Path) -> None:
    installs, environ = world(tmp_path)
    tool_dirs = ToolchainPaths(SubprocessRunner(), RunLayout(tmp_path / "tools"), environ, tools=("go",)).directories()
    session_environ = {"PATH": environ["PATH"], "HOME": str(tmp_path / "owner"), "CLAUDE_CODE_OAUTH_TOKEN": TEST_TOKEN}
    (tmp_path / "rig").mkdir()
    rig = make_claude_rig(tmp_path / "rig", [{}], environ=session_environ, tool_dirs=tool_dirs)

    rig.harness.run_session(rig.request("t1"))

    path = recorded_session_calls(rig.fake_dir)[0]["env"]["PATH"].split(":")
    assert path[0] == str(installs / "go" / "bin")
    assert str(tmp_path / "shims") not in path
