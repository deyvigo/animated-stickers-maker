"""Bounded concurrency for the two expensive operations: yt-dlp downloads
and ffmpeg renders.

No task queue / Redis in the MVP — a process-local `asyncio.Semaphore` per
kind of work is enough for a single-instance dev server, and it's the
natural place to bump if this ever needs to scale to `arq`/Redis: swap what
`run_download`/`run_render` submit to, keep the call sites the same.
"""
from __future__ import annotations

import asyncio
from collections.abc import Awaitable, Callable
from typing import TypeVar

from app.core.config import Settings

T = TypeVar("T")


class WorkQueues:
    def __init__(self, settings: Settings):
        self.downloads = asyncio.Semaphore(settings.max_concurrent_downloads)
        self.renders = asyncio.Semaphore(settings.max_concurrent_renders)

    async def run_download(self, fn: Callable[[], Awaitable[T]]) -> T:
        async with self.downloads:
            return await fn()

    async def run_render(self, fn: Callable[[], Awaitable[T]]) -> T:
        async with self.renders:
            return await fn()
