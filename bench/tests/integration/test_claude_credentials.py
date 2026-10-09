"""The keychain and environment credential sources against a fake `security` on the PATH, and what the coder and
gate-host sessions do with them. No `security` of the machine is run and no model is called."""

from __future__ import annotations

import os
from pathlib import Path

import pytest

from specbench.adapters.claude_credentials import (
    KEYCHAIN_SERVICE,
    OAUTH_TOKEN_VARIABLE,
    EnvironmentCredentialSource,
    FirstCredentialSource,
    KeychainCredentialSource,
    login_name,
)
from specbench.adapters.claude_session import MISSING_TOKEN_MESSAGE
from specbench.application.ports import CredentialSourceFailed
from specbench.domain.sessions import SessionOutcome
from support.coder_rigs import make_claude_rig
from support.credentials import (
    FAKE_SECURITY_FAILURE_EXIT,
    FAKE_TOKEN,
    FailingCredentialSource,
    KeychainItem,
    install_fake_security,
    security_calls,
)
from support.fake_claude import recorded_session_calls

ACCOUNT = "someone"


def put_on_path(monkeypatch: pytest.MonkeyPatch, tmp_path: Path, item: KeychainItem) -> Path:
    directory = tmp_path / "bin"
    install_fake_security(directory, item)
    monkeypatch.setenv("PATH", f"{directory}{os.pathsep}{os.environ['PATH']}")
    return directory


def test_a_present_item_is_read_with_its_service_and_account(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    directory = put_on_path(monkeypatch, tmp_path, KeychainItem.PRESENT)

    assert KeychainCredentialSource(account=ACCOUNT).token() == FAKE_TOKEN
    assert security_calls(directory)[0] == f"find-generic-password -s {KEYCHAIN_SERVICE} -a {ACCOUNT} -w"


def test_the_account_defaults_to_the_macos_login(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    directory = put_on_path(monkeypatch, tmp_path, KeychainItem.PRESENT)

    KeychainCredentialSource().token()

    assert security_calls(directory)[0].split()[4] == login_name()


def test_exit_44_is_an_absent_item(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    put_on_path(monkeypatch, tmp_path, KeychainItem.ABSENT)
    assert KeychainCredentialSource(account=ACCOUNT).token() is None


def test_any_other_failure_names_the_exit_code_and_nothing_the_tool_printed(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    put_on_path(monkeypatch, tmp_path, KeychainItem.FAILING)

    with pytest.raises(CredentialSourceFailed) as failure:
        KeychainCredentialSource(account=ACCOUNT).token()

    assert str(FAKE_SECURITY_FAILURE_EXIT) in str(failure.value) and FAKE_TOKEN not in str(failure.value)
    assert failure.value.__cause__ is None and failure.value.__context__ is None


def test_a_missing_security_tool_is_a_failure_not_an_absence(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PATH", str(tmp_path))
    with pytest.raises(CredentialSourceFailed, match="not found"):
        KeychainCredentialSource(account=ACCOUNT).token()


def test_the_security_tool_does_not_inherit_the_token_of_the_benchs_own_environment(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    directory = put_on_path(monkeypatch, tmp_path, KeychainItem.PRESENT)
    monkeypatch.setenv(OAUTH_TOKEN_VARIABLE, "from-the-environment")

    KeychainCredentialSource(account=ACCOUNT).token()

    assert "inherited-token=unset" in security_calls(directory)


def test_the_environment_comes_before_the_keychain(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    directory = put_on_path(monkeypatch, tmp_path, KeychainItem.PRESENT)
    first = FirstCredentialSource(
        [EnvironmentCredentialSource({OAUTH_TOKEN_VARIABLE: "from-the-environment"}), KeychainCredentialSource()]
    )
    assert first.token() == "from-the-environment" and security_calls(directory) == []

    fallback = FirstCredentialSource([EnvironmentCredentialSource({}), KeychainCredentialSource(account=ACCOUNT)])
    assert fallback.token() == FAKE_TOKEN


def test_an_empty_variable_is_no_token() -> None:
    assert EnvironmentCredentialSource({OAUTH_TOKEN_VARIABLE: ""}).token() is None


def test_a_coder_session_receives_the_keychain_token_in_its_own_environment_only(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    put_on_path(monkeypatch, tmp_path, KeychainItem.PRESENT)
    (tmp_path / "rig").mkdir()
    rig = make_claude_rig(
        tmp_path / "rig",
        [{}],
        environ={"PATH": os.environ["PATH"]},
        credentials=KeychainCredentialSource(account=ACCOUNT),
    )

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.COMPLETED
    assert recorded_session_calls(rig.fake_dir)[0]["token"] == FAKE_TOKEN


def test_a_coder_session_with_no_token_in_the_keychain_is_misconfigured_as_before(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    put_on_path(monkeypatch, tmp_path, KeychainItem.ABSENT)
    (tmp_path / "rig").mkdir()
    rig = make_claude_rig(
        tmp_path / "rig",
        [{}],
        environ={"PATH": os.environ["PATH"]},
        credentials=KeychainCredentialSource(account=ACCOUNT),
    )

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.problems == (MISSING_TOKEN_MESSAGE,)
    assert not (rig.fake_dir / "state" / "calls").exists() or not list((rig.fake_dir / "state" / "calls").iterdir())


def test_a_coder_session_whose_source_fails_never_starts_and_reports_no_token(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}], credentials=FailingCredentialSource())

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.executions == 0
    assert "code 51" in result.problems[0] and result.events == ()
    assert not (rig.fake_dir / "state" / "calls").exists() or not list((rig.fake_dir / "state" / "calls").iterdir())
