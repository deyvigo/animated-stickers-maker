"""FastAPI dependency wiring. Everything long-lived (settings, on-disk job
store, concurrency semaphores, rate limiter) is created once in `main.py`
and stashed on `app.state`; these functions just fetch it back out for
route handlers, and enforce the rate limit.
"""
from __future__ import annotations

from fastapi import Header, Request

from app.core.config import Settings
from app.core.errors import AppError
from app.core.queue import WorkQueues
from app.core.ratelimit import RateLimiter
from app.core.storage import JobStore
from app.models.schemas import ErrorCode


def get_settings(request: Request) -> Settings:
    return request.app.state.settings


def get_store(request: Request) -> JobStore:
    return request.app.state.store


def get_queues(request: Request) -> WorkQueues:
    return request.app.state.queues


def enforce_rate_limit(
    request: Request,
    x_device_id: str | None = Header(default=None),
) -> None:
    limiter: RateLimiter = request.app.state.rate_limiter
    client_ip = request.client.host if request.client else "unknown"
    key = f"{x_device_id or 'no-device'}:{client_ip}"
    if not limiter.check(key):
        raise AppError(
            ErrorCode.RATE_LIMITED,
            "Demasiadas solicitudes. Probá de nuevo en un rato.",
            status_code=429,
        )
