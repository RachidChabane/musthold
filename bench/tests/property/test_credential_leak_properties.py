"""Whatever the token is, no record, event, manifest, log or result of a session holds its value."""

from __future__ import annotations

import tempfile
from pathlib import Path

from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from specbench.adapters.claude_credentials import OAUTH_TOKEN_VARIABLE
from specbench.adapters.claude_session import MISSING_TOKEN_MESSAGE
from specbench.application.ports import CredentialSourceFailed
from specbench.domain.sessions import SessionOutcome
from support.coder_rigs import make_claude_rig
from support.credentials import StaticCredentialSource
from support.fake_claude import recorded_session_calls

TOKENS = st.from_regex(r"sk-ant-oat01-[A-Za-z0-9_-]{24,64}", fullmatch=True)
SESSIONS_PER_PROPERTY = 6
RESPONSE = {"events": [{"text": "working"}, {"edit": "pkg/a.py"}], "final": "done"}


def files_holding(root: Path, value: str) -> list[Path]:
    needle = value.encode()
    return [path for path in root.rglob("*") if path.is_file() and needle in path.read_bytes()]


@settings(max_examples=SESSIONS_PER_PROPERTY, deadline=None, suppress_health_check=[HealthCheck.too_slow])
@given(TOKENS)
def test_no_file_of_a_run_and_no_part_of_a_coder_result_holds_the_token(token: str) -> None:
    with tempfile.TemporaryDirectory() as directory:
        rig = make_claude_rig(
            Path(directory), [RESPONSE], environ={"PATH": "/usr/bin:/bin", OAUTH_TOKEN_VARIABLE: token}
        )

        result = rig.harness.run_session(rig.request("t1"))

        assert recorded_session_calls(rig.fake_dir)[0]["token"] == token
        assert files_holding(rig.layout.root, token) == []
        reported = [*result.events, result.final_message, result.stderr, *result.problems, repr(result)]
        assert not [part for part in reported if token in part]


@settings(max_examples=SESSIONS_PER_PROPERTY, deadline=None, suppress_health_check=[HealthCheck.too_slow])
@given(TOKENS)
def test_a_coder_session_that_cannot_read_its_token_reports_nothing_of_it(token: str) -> None:
    class Unreadable:
        def token(self) -> str | None:
            raise CredentialSourceFailed("the keychain tool exited with code 51")

    with tempfile.TemporaryDirectory() as directory:
        rig = make_claude_rig(
            Path(directory), [RESPONSE], environ={OAUTH_TOKEN_VARIABLE: token}, credentials=Unreadable()
        )

        result = rig.harness.run_session(rig.request("t1"))

        assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED
        assert token not in repr(result) and files_holding(rig.layout.root, token) == []


@settings(max_examples=SESSIONS_PER_PROPERTY, deadline=None, suppress_health_check=[HealthCheck.too_slow])
@given(TOKENS)
def test_an_absent_token_is_reported_without_any_value(token: str) -> None:
    with tempfile.TemporaryDirectory() as directory:
        rig = make_claude_rig(
            Path(directory), [RESPONSE], environ={OAUTH_TOKEN_VARIABLE: token}, credentials=StaticCredentialSource(None)
        )

        result = rig.harness.run_session(rig.request("t1"))

        assert result.problems == (MISSING_TOKEN_MESSAGE,) and token not in repr(result)
