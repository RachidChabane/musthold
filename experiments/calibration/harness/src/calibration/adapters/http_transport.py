"""`HttpTransport` over `urllib`: no proxy, no redirect, every HTTP status is a reply and the body bytes travel as given."""

from __future__ import annotations

import http.client
import urllib.error
import urllib.request
from collections.abc import Mapping
from typing import Final

from calibration.domain.constants import JSON_CONTENT_TYPE
from calibration.domain.ports import HttpReply, TransportFailure

CONTENT_TYPE_HEADER: Final = "Content-Type"
POST_METHOD: Final = "POST"
GET_METHOD: Final = "GET"


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Leaves a 3xx reply as it is, so that it surfaces as an `HTTPError` carrying its status."""

    def redirect_request(
        self,
        req: urllib.request.Request,
        fp: object,
        code: int,
        msg: str,
        headers: object,
        newurl: str,
    ) -> None:
        return None


def _opener() -> urllib.request.OpenerDirector:
    return urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect())


def _failure(error: BaseException) -> TransportFailure:
    reason = error.reason if isinstance(error, urllib.error.URLError) else error
    timed_out = isinstance(reason, TimeoutError)
    return TransportFailure(f"{type(reason).__name__}: {reason}", timed_out=timed_out)


class UrllibTransport:
    def __init__(self) -> None:
        self._opener = _opener()

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply:
        request = urllib.request.Request(
            url, data=body, method=POST_METHOD, headers={CONTENT_TYPE_HEADER: JSON_CONTENT_TYPE}
        )
        return self._exchange(request, timeout_seconds)

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        request = urllib.request.Request(url, method=GET_METHOD, headers=dict(headers or {}))
        return self._exchange(request, timeout_seconds)

    def _exchange(self, request: urllib.request.Request, timeout_seconds: float) -> HttpReply:
        try:
            with self._opener.open(request, timeout=timeout_seconds) as response:
                return HttpReply(response.status, response.read())
        except urllib.error.HTTPError as error:
            return self._error_reply(error)
        except (urllib.error.URLError, http.client.HTTPException, OSError, ValueError) as error:
            raise _failure(error) from error

    @staticmethod
    def _error_reply(error: urllib.error.HTTPError) -> HttpReply:
        try:
            with error:
                return HttpReply(error.code, error.read())
        except (http.client.HTTPException, OSError) as failure:
            raise _failure(failure) from failure
