"""Very small in-memory fixed-window rate limiter, keyed by device id + IP.

Not meant to be robust against a determined attacker — it's abuse
mitigation for a stateless, account-less service, not a security boundary.
Resets if the process restarts; that's fine here.
"""
from __future__ import annotations

import time
from collections import defaultdict


class RateLimiter:
    def __init__(self, limit_per_hour: int):
        self._limit = limit_per_hour
        self._window_seconds = 3600.0
        self._hits: dict[str, list[float]] = defaultdict(list)

    def check(self, key: str) -> bool:
        """Returns True if the request is allowed, recording it if so."""
        now = time.monotonic()
        hits = self._hits[key]
        cutoff = now - self._window_seconds
        while hits and hits[0] < cutoff:
            hits.pop(0)
        if len(hits) >= self._limit:
            return False
        hits.append(now)
        return True
