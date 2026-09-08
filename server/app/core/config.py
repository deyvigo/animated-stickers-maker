"""Application settings, loaded from environment variables / .env.

Every knob that changes behavior between "quick local test" and "actually
serving my phone on the LAN" lives here, not sprinkled through the code.
"""
from __future__ import annotations

from functools import lru_cache
from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

    # Where job/render artifacts live. Inside the container this is a mounted
    # volume so jobs survive a container restart during development.
    storage_dir: Path = Path("/data/storage")

    # How long a job's files stick around before the cleanup task deletes them.
    job_ttl_hours: float = 6.0
    cleanup_interval_seconds: float = 900.0

    # Bounded concurrency for yt-dlp downloads and ffmpeg renders. These are
    # CPU/IO heavy; running too many at once on a dev machine just thrashes.
    max_concurrent_downloads: int = 2
    max_concurrent_renders: int = 2

    # Reject videos above this size before even trying to process them.
    max_source_filesize_mb: int = 80

    # Only these hosts may be handed to yt-dlp. Keeps this service from being
    # abused as a generic URL downloader (SSRF / TOS surface).
    allowed_hosts: tuple[str, ...] = (
        "tiktok.com",
        "www.tiktok.com",
        "m.tiktok.com",
        "vm.tiktok.com",
        "vt.tiktok.com",
    )

    # Simple per-device rate limit. The app sends a random UUID as
    # X-Device-Id on first launch; this is not auth, just abuse mitigation.
    rate_limit_per_hour: int = 60

    # ffmpeg/ffprobe/yt-dlp subprocess timeouts, in seconds.
    download_timeout_seconds: float = 120.0
    ffmpeg_timeout_seconds: float = 60.0

    # WhatsApp sticker hard limits (see plan doc / WhatsApp developer docs).
    sticker_dimension: int = 512
    static_max_bytes: int = 100 * 1024
    animated_max_bytes: int = 500 * 1024
    tray_dimension: int = 96
    tray_max_bytes: int = 50 * 1024
    max_clip_seconds: float = 10.0

    host: str = "0.0.0.0"
    port: int = 8000
    log_level: str = "info"

    def job_dir(self, job_id: str) -> Path:
        return self.storage_dir / "jobs" / job_id

    def render_dir(self, render_id: str) -> Path:
        # Flat, independent of job_id, so /api/v1/renders/{id} routes don't
        # need the job_id in the URL. The render's state file records which
        # job it came from, for locating the source video.
        return self.storage_dir / "renders" / render_id


@lru_cache
def get_settings() -> Settings:
    return Settings()
