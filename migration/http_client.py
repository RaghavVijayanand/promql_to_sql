"""
Shared HTTP client with retry logic, timeout, and auth header injection.
"""

import time
import logging
import requests
from typing import Any, Dict, Optional

from migration import config

log = logging.getLogger("migration.http")


class APIError(Exception):
    """Raised when an HTTP API returns a non-success status."""

    def __init__(self, url: str, status: int, body: str):
        self.url = url
        self.status = status
        self.body = body
        super().__init__(f"HTTP {status} from {url}: {body[:300]}")


def _build_session(
    token: Optional[str] = None,
    bearer: bool = True,
) -> requests.Session:
    s = requests.Session()
    s.headers["Accept"] = "application/json"
    if token:
        prefix = "Bearer" if bearer else "Token"
        s.headers["Authorization"] = f"{prefix} {token}"
    return s


def fetch_json(
    url: str,
    *,
    params: Optional[Dict[str, Any]] = None,
    token: Optional[str] = None,
    bearer: bool = True,
    timeout: Optional[int] = None,
    retries: Optional[int] = None,
    retry_delay: Optional[int] = None,
) -> Any:
    """GET *url* and return parsed JSON, with retries on transient errors."""
    _timeout = timeout or config.HTTP_TIMEOUT
    _retries = retries if retries is not None else config.HTTP_RETRIES
    _delay = retry_delay if retry_delay is not None else config.HTTP_RETRY_DELAY

    session = _build_session(token, bearer)
    last_exc: Optional[Exception] = None

    for attempt in range(1, _retries + 1):
        try:
            resp = session.get(url, params=params, timeout=_timeout)
            if resp.status_code >= 500:
                # Server error — retryable
                raise APIError(url, resp.status_code, resp.text)
            if resp.status_code >= 400:
                # Client error (401, 403, 404…) — NOT retryable
                raise APIError(url, resp.status_code, resp.text)
            return resp.json()
        except APIError as exc:
            if exc.status < 500:
                # 4xx → fail immediately, do NOT retry
                raise
            last_exc = exc
            if attempt < _retries:
                wait = _delay * attempt
                log.warning("Attempt %d/%d failed for %s — retrying in %ds: %s",
                            attempt, _retries, url, wait, exc)
                time.sleep(wait)
            else:
                log.error("All %d attempts failed for %s", _retries, url)
        except (requests.ConnectionError, requests.Timeout) as exc:
            last_exc = exc
            if attempt < _retries:
                wait = _delay * attempt
                log.warning("Attempt %d/%d failed for %s — retrying in %ds: %s",
                            attempt, _retries, url, wait, exc)
                time.sleep(wait)
            else:
                log.error("All %d attempts failed for %s", _retries, url)

    raise last_exc  # type: ignore[misc]


def post_json(
    url: str,
    payload: Any,
    *,
    token: Optional[str] = None,
    bearer: bool = True,
    timeout: Optional[int] = None,
) -> Any:
    """POST JSON to *url* and return the response body."""
    _timeout = timeout or config.HTTP_TIMEOUT
    session = _build_session(token, bearer)
    session.headers["Content-Type"] = "application/json"
    resp = session.post(url, json=payload, timeout=_timeout)
    if resp.status_code >= 400:
        raise APIError(url, resp.status_code, resp.text)
    try:
        return resp.json()
    except ValueError:
        return {"status": resp.status_code, "text": resp.text}


def put_json(
    url: str,
    payload: Any,
    *,
    token: Optional[str] = None,
    bearer: bool = True,
    timeout: Optional[int] = None,
) -> Any:
    """PUT JSON to *url* and return the response body."""
    _timeout = timeout or config.HTTP_TIMEOUT
    session = _build_session(token, bearer)
    session.headers["Content-Type"] = "application/json"
    resp = session.put(url, json=payload, timeout=_timeout)
    if resp.status_code >= 400:
        raise APIError(url, resp.status_code, resp.text)
    try:
        return resp.json()
    except ValueError:
        return {"status": resp.status_code, "text": resp.text}


def delete_json(
    url: str,
    *,
    token: Optional[str] = None,
    bearer: bool = True,
    timeout: Optional[int] = None,
) -> Any:
    """DELETE against *url*."""
    _timeout = timeout or config.HTTP_TIMEOUT
    session = _build_session(token, bearer)
    resp = session.delete(url, timeout=_timeout)
    if resp.status_code >= 400:
        raise APIError(url, resp.status_code, resp.text)
    try:
        return resp.json()
    except ValueError:
        return {"status": resp.status_code}
