"""Filesystem-backed job/render state.

No database in the MVP: each job is a directory containing a `job.json`
state file plus its media artifacts. This module is the only place that
reads/writes those directories, so job state can never drift out of sync
with what's actually on disk.
"""
from __future__ import annotations

import asyncio
import json
import logging
import shutil
import time
import uuid
from pathlib import Path
from typing import Any

from app.core.config import Settings

logger = logging.getLogger("stickers.storage")


def new_id() -> str:
    return uuid.uuid4().hex


class JobStore:
    """Async-safe read/write of job and render state on disk.

    A single `asyncio.Lock` per store instance serializes state file writes.
    This is deliberately simple: with a handful of concurrent jobs on a
    single-process dev server, lock contention is a non-issue, and it rules
    out a whole class of read-modify-write races on job.json.
    """

    def __init__(self, settings: Settings):
        self._settings = settings
        self._lock = asyncio.Lock()

    # ---- job lifecycle -------------------------------------------------

    def create_job_dir(self, job_id: str) -> Path:
        job_dir = self._settings.job_dir(job_id)
        job_dir.mkdir(parents=True, exist_ok=True)
        (job_dir / "renders").mkdir(exist_ok=True)
        return job_dir

    async def write_job(self, job_id: str, state: dict[str, Any]) -> None:
        job_dir = self._settings.job_dir(job_id)
        job_dir.mkdir(parents=True, exist_ok=True)
        state = {**state, "updated_at": time.time()}
        async with self._lock:
            _atomic_write_json(job_dir / "job.json", state)

    async def read_job(self, job_id: str) -> dict[str, Any] | None:
        path = self._settings.job_dir(job_id) / "job.json"
        if not path.exists():
            return None
        return json.loads(path.read_text())

    async def update_job(self, job_id: str, **fields: Any) -> dict[str, Any]:
        async with self._lock:
            path = self._settings.job_dir(job_id) / "job.json"
            state = json.loads(path.read_text()) if path.exists() else {"job_id": job_id}
            state.update(fields)
            state["updated_at"] = time.time()
            _atomic_write_json(path, state)
            return state

    # ---- render lifecycle ------------------------------------------------

    def find_source_file(self, job_id: str) -> Path | None:
        job_dir = self._settings.job_dir(job_id)
        candidates = [p for p in job_dir.glob("source.*") if p.suffix != ".tmp"]
        return candidates[0] if candidates else None

    def create_render_dir(self, render_id: str) -> Path:
        render_dir = self._settings.render_dir(render_id)
        render_dir.mkdir(parents=True, exist_ok=True)
        return render_dir

    async def write_render(self, render_id: str, state: dict[str, Any]) -> None:
        render_dir = self._settings.render_dir(render_id)
        render_dir.mkdir(parents=True, exist_ok=True)
        state = {**state, "updated_at": time.time()}
        async with self._lock:
            _atomic_write_json(render_dir / "render.json", state)

    async def read_render(self, render_id: str) -> dict[str, Any] | None:
        path = self._settings.render_dir(render_id) / "render.json"
        if not path.exists():
            return None
        return json.loads(path.read_text())

    async def update_render(self, render_id: str, **fields: Any) -> dict[str, Any]:
        async with self._lock:
            path = self._settings.render_dir(render_id) / "render.json"
            state = json.loads(path.read_text()) if path.exists() else {"render_id": render_id}
            state.update(fields)
            state["updated_at"] = time.time()
            _atomic_write_json(path, state)
            return state

    # ---- TTL cleanup -----------------------------------------------------

    def sweep_expired(self) -> int:
        """Delete job and render directories older than job_ttl_hours.
        Returns count removed.

        Runs synchronously (called from a background asyncio task via
        to_thread) since it's just filesystem stat/rmtree work.
        """
        cutoff = time.time() - self._settings.job_ttl_hours * 3600
        removed = 0
        for root_name in ("jobs", "renders"):
            root = self._settings.storage_dir / root_name
            if not root.exists():
                continue
            for entry_dir in root.iterdir():
                if not entry_dir.is_dir():
                    continue
                try:
                    mtime = entry_dir.stat().st_mtime
                    if mtime < cutoff:
                        shutil.rmtree(entry_dir, ignore_errors=True)
                        removed += 1
                except FileNotFoundError:
                    continue
        return removed


def _atomic_write_json(path: Path, data: dict[str, Any]) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(data, default=str))
    tmp.replace(path)


async def run_cleanup_loop(store: JobStore, settings: Settings) -> None:
    """Background task: periodically sweep expired jobs. Cancelled on shutdown."""
    while True:
        try:
            removed = await asyncio.to_thread(store.sweep_expired)
            if removed:
                logger.info("cleanup: removed %d expired job(s)", removed)
        except Exception:
            logger.exception("cleanup sweep failed")
        await asyncio.sleep(settings.cleanup_interval_seconds)
