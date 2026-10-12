import os
import stat
from pathlib import Path, PurePosixPath

from demos1.adapters.engine import DockerEngine
from demos1.adapters.github import GhCliApi, parse_included
from demos1.adapters.local import LocalFiles
from demos1.domain.container import ContainerCommand
from demos1.domain.queries import search_issues
from support.fakes import START


def test_an_included_response_gives_status_headers_and_body():
    output = 'HTTP/2.0 403 Forbidden\r\nX-Ratelimit-Remaining: 0\r\nX-Ratelimit-Reset: 1791850000\r\n\r\n{"message": "limit"}'
    response = parse_included(output, START)
    assert response.status == 403
    assert response.header("X-RateLimit-Reset") == "1791850000"
    assert response.is_rate_limited()
    assert response.json() == {"message": "limit"}


def test_a_request_without_a_response_has_no_status():
    assert parse_included("error connecting to api.github.com", START).status == 0


def test_the_gh_command_line_is_a_get_with_fields():
    argv = GhCliApi(lambda: START).argv(search_issues("is:pr language:Go", 2))
    assert argv[:6] == ["gh", "api", "--include", "--method", "GET", "search/issues"]
    assert "q=is:pr language:Go" in argv


def test_the_host_command_wraps_docker_in_slots_and_the_guard():
    engine = DockerEngine(
        "unix:///h/.colima/spec-proof-demos/docker.sock", PurePosixPath("/repo/script"), lambda: START
    )
    command = ContainerCommand(
        "A-go-01-full-r1", "sha256:i", "/w/a", "/p/bin", "/w/a/clone", False, {}, ["x"], "/w/a/r", None, 502, 20
    )
    argv = engine.host_argv(command, 2)
    assert argv[:8] == ["/repo/script/with-slot", "2", "--wait", "3600", "--", "/repo/script/mem-guard", "run", "--"]
    assert argv[8:10] == ["docker", "run"]


def test_a_tree_with_read_only_entries_is_removed(tmp_path: Path):
    module = tmp_path / "cache" / "go" / "mod" / "example.com" / "q@v1.0.0"
    module.mkdir(parents=True)
    (module / "q.go").write_text("package q\n")
    (module / "q.go").chmod(stat.S_IRUSR)
    module.chmod(stat.S_IRUSR | stat.S_IXUSR)
    files = LocalFiles()
    files.remove_tree(PurePosixPath(tmp_path / "cache"))
    assert not (tmp_path / "cache").exists()
    files.remove_tree(PurePosixPath(tmp_path / "absent"))


def test_tree_size_counts_allocated_bytes_once_per_inode(tmp_path: Path):
    (tmp_path / "a").write_bytes(b"0" * 100_000)
    os.link(tmp_path / "a", tmp_path / "b")
    size = LocalFiles().tree_size(PurePosixPath(tmp_path))
    assert 100_000 <= size < 200_000


def test_the_wrapper_record_is_read_back(tmp_path: Path):
    prefix = tmp_path / "suite-2-r1"
    Path(str(prefix) + ".measure.json").write_text(
        '{"exit_status": 137, "limit_seconds": 60, "memory_peak_bytes": 734003200, "timed_out": true, "wall_seconds": 60.01}\n'
    )
    measure = LocalFiles().read_measure(PurePosixPath(prefix))
    assert measure is not None
    assert (measure.exit_status, measure.timed_out, measure.memory_peak_bytes) == (137, True, 734003200)
    assert LocalFiles().read_measure(PurePosixPath(tmp_path / "missing")) is None
