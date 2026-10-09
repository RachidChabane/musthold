"""Properties of the coder-session command lines, homes and the Codex trust cleanup, over the whole input space."""

from __future__ import annotations

import tempfile
from pathlib import Path

from hypothesis import given
from hypothesis import strategies as st

from specbench.adapters.claude_homes import ClaudeConfigHomes
from specbench.adapters.claude_session import claude_argv, claude_environment
from specbench.adapters.codex_session import codex_argv
from specbench.adapters.session_environment import neutral_environment
from specbench.domain.arm_policy import policy_for
from specbench.domain.arms import FROZEN_MODEL, PLUGIN_HOST_ARMS, Arm, Harness
from specbench.domain.codex_config import names_project, remove_project_table
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import SessionKind, SessionRequest
from support.pinned_claude import UNUSED_EXECUTABLE

HONEST_ARMS = [arm for arm in Arm if arm not in PLUGIN_HOST_ARMS]
SEGMENT = st.from_regex(r"[a-z][a-z0-9-]{0,12}", fullmatch=True)
SESSION_ID = st.from_regex(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", fullmatch=True)
PROMPTS = st.text(max_size=80)


@st.composite
def sessions(draw: st.DrawFn, harness: Harness, arms: list[Arm]) -> SessionRequest:
    resume = draw(st.none() | SESSION_ID)
    fork = resume is not None and draw(st.booleans())
    workspace = Path("/work") / draw(SEGMENT) / draw(SEGMENT)
    return SessionRequest(
        harness,
        FROZEN_MODEL[harness],
        SessionKind.BRANCH if fork else draw(st.sampled_from([SessionKind.TRUNK, SessionKind.BRANCH])),
        draw(SEGMENT),
        workspace,
        draw(PROMPTS),
        resume,
        fork,
        draw(SEGMENT),
        draw(st.sampled_from(arms)),
    )


@given(sessions(Harness.CLAUDE_CODE, HONEST_ARMS))
def test_an_honest_arm_never_reads_project_settings_and_never_loads_a_plugin(request: SessionRequest) -> None:
    assert request.arm is not None
    argv = claude_argv(
        UNUSED_EXECUTABLE, request, policy_for(request.arm), Path("/runs/r1/settings/T.json"), (Path("/repo/plugin"),)
    )

    sources = argv[argv.index("--setting-sources") + 1].split(",")
    assert sources == ["user"] and "project" not in sources and "local" not in sources
    assert "--plugin-dir" not in argv


@given(sessions(Harness.CLAUDE_CODE, list(PLUGIN_HOST_ARMS)))
def test_a_plugin_host_arm_reads_exactly_user_and_project(request: SessionRequest) -> None:
    assert request.arm is not None
    argv = claude_argv(UNUSED_EXECUTABLE, request, policy_for(request.arm), Path("/runs/r1/settings/cheat.json"), ())
    assert argv[argv.index("--setting-sources") + 1] == "user,project"


@given(sessions(Harness.CLAUDE_CODE, list(Arm)))
def test_no_option_of_a_claude_session_points_into_the_workspace(request: SessionRequest) -> None:
    assert request.arm is not None
    argv = claude_argv(
        UNUSED_EXECUTABLE, request, policy_for(request.arm), Path("/runs/r1/settings/x.json"), (Path("/repo/plugin"),)
    )
    assert not [word for word in argv if word.startswith(str(request.workdir) + "/") or word == str(request.workdir)]


@given(sessions(Harness.CODEX, list(Arm)))
def test_a_codex_resume_or_fork_never_carries_s_or_c_and_a_fresh_session_always_does(request: SessionRequest) -> None:
    assert request.arm is not None
    argv = codex_argv(request, policy_for(request.arm), request.workdir)
    has_flags = "-s" in argv and "-C" in argv
    assert has_flags is (request.resume_session_id is None)
    assert ("-s" in argv) is ("-C" in argv)
    assert any(word.startswith("web_search=disabled") for word in argv)
    assert "sandbox_workspace_write.network_access=false" in argv
    assert "--ignore-user-config" not in argv
    assert argv[-1] == "-"


@given(sessions(Harness.CLAUDE_CODE, list(Arm)), SEGMENT)
def test_a_configuration_home_is_never_inside_the_workspace(request: SessionRequest, token: str) -> None:
    with tempfile.TemporaryDirectory() as raw:
        root = Path(raw).resolve()
        workspace = root / "work" / "ws"
        workspace.mkdir(parents=True)
        layout = RunLayout(root / "run")
        homes = ClaudeConfigHomes(layout)
        placed = SessionRequest(
            request.harness,
            request.model,
            request.kind,
            request.key,
            workspace,
            request.prompt,
            request.resume_session_id,
            request.fork,
            request.trunk_id,
            request.arm,
        )
        if placed.kind is SessionKind.BRANCH and placed.fork:
            homes.trunk(placed.trunk_id).mkdir(parents=True)
            homes.snapshot(placed.trunk_id)

        home = homes.home_for(placed, workspace)
        environment = claude_environment({"PATH": "/bin"}, home, token)

        assert workspace not in home.resolve().parents and home.resolve() != workspace
        assert environment["CLAUDE_CONFIG_DIR"] == str(home)
        assert layout.root in home.parents


TOML_LINES = st.sampled_from(["a = 1", "[tui]", 'theme = "dark"', "", "# note", "[[hooks]]", "k = 'v'"])
PROJECT = "/private/var/folders/x/T/ws-1"


def render(lines: list[str]) -> str:
    return "".join(f"{line}\n" for line in lines)


@given(st.lists(TOML_LINES, max_size=12))
def test_removing_the_table_codex_appended_gives_back_the_owner_file(owner_lines: list[str]) -> None:
    owner = render(owner_lines)
    table = f'[projects."{PROJECT}"]\ntrust_level = "trusted"\n'
    appended = owner + ("\n" if owner else "") + table

    removal = remove_project_table(appended, PROJECT)

    assert removal.found and removal.cleaned == owner and removal.table == table
    assert not names_project(removal.cleaned, PROJECT)


@given(st.lists(TOML_LINES, max_size=12), st.lists(TOML_LINES, max_size=12))
def test_a_table_between_owner_sections_leaves_every_other_line_in_place(before: list[str], after: list[str]) -> None:
    table = f'[projects."{PROJECT}"]\ntrust_level = "trusted"\n'
    sentinel = "[sentinel]\n"
    content = render(before) + table + sentinel + render(after)

    removal = remove_project_table(content, PROJECT)

    assert removal.found and removal.cleaned == render(before) + sentinel + render(after)


@given(st.text(alphabet=st.characters(codec="utf-8", exclude_categories=["Cs"]), max_size=200))
def test_a_file_that_does_not_name_the_project_comes_back_unchanged(content: str) -> None:
    removal = remove_project_table(content.replace(PROJECT, ""), PROJECT)
    assert not removal.found and removal.cleaned == content.replace(PROJECT, "")


OWNER_HOME = "/Users/owner"
ENV_NAMES = st.from_regex(r"[A-Z][A-Z0-9_]{0,15}", fullmatch=True).filter(lambda name: name not in {"HOME", "PATH"})
ENV_VALUES = st.one_of(
    st.text(max_size=20).filter(lambda value: not value.startswith(OWNER_HOME)),
    SEGMENT.map(lambda segment: f"{OWNER_HOME}/{segment}"),
    st.just(OWNER_HOME),
)


@given(st.dictionaries(ENV_NAMES, ENV_VALUES, max_size=10), st.none() | SEGMENT)
def test_no_value_of_a_session_environment_points_into_the_owners_home(
    inherited: dict[str, str], task: str | None
) -> None:
    environment = neutral_environment(
        {**inherited, "HOME": OWNER_HOME, "PATH": f"{OWNER_HOME}/bin:/bin"},
        Path("/runs/r1/homes/t/home"),
        Path("/work/envs") / task if task is not None else None,
    )

    pointing = {
        name
        for name, value in environment.items()
        if name != "PATH" and (value == OWNER_HOME or value.startswith(OWNER_HOME + "/"))
    }
    assert pointing == set()


PATH_ENTRIES = st.one_of(
    SEGMENT.map(lambda name: f"/opt/{name}/bin"),
    SEGMENT.map(lambda name: f"/opt/{name}/shims"),
    st.just("/Users/owner/dev-env/1-tools/shims"),
    st.just("/usr/bin"),
)


@given(
    st.lists(PATH_ENTRIES, min_size=1, max_size=8),
    st.lists(SEGMENT.map(lambda name: f"/installs/{name}/bin"), unique=True, max_size=4),
)
def test_a_session_path_never_contains_a_shims_directory_and_starts_with_the_tool_directories(
    entries: list[str], tool_dirs: list[str]
) -> None:
    path = neutral_environment(
        {"HOME": OWNER_HOME, "PATH": ":".join(entries)}, Path("/runs/r1/homes/t/home"), None, tool_dirs
    )["PATH"].split(":")

    assert not [entry for entry in path if Path(entry).name == "shims"]
    assert path[: len(tool_dirs)] == list(dict.fromkeys(tool_dirs))
