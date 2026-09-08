"""Downloads a TikTok video with yt-dlp into a job's directory.

Uses yt-dlp's Python API (not a subprocess) so we get structured errors and
a progress hook instead of scraping stdout.
"""
from __future__ import annotations

import asyncio
import logging
from pathlib import Path
from urllib.parse import urlparse

import yt_dlp

from app.core.config import Settings
from app.core.errors import AppError
from app.models.schemas import ErrorCode

logger = logging.getLogger("stickers.download")


def assert_allowed_host(url: str, settings: Settings) -> None:
    """Reject anything that isn't a TikTok host *before* touching yt-dlp.

    This is the SSRF/abuse boundary: without it, this endpoint is a generic
    "download any URL on the internet" service.
    """
    try:
        parsed = urlparse(url)
        host = (parsed.hostname or "").lower()
    except ValueError as exc:
        raise AppError(ErrorCode.INVALID_URL, "El link no es válido.") from exc
    if not host or parsed.scheme not in ("http", "https"):
        raise AppError(ErrorCode.INVALID_URL, "El link no es válido.")
    if not any(host == allowed or host.endswith("." + allowed) for allowed in settings.allowed_hosts):
        raise AppError(
            ErrorCode.HOST_NOT_ALLOWED,
            "Solo se admiten links de TikTok.",
            detail=f"host={host}",
        )


def _run_download_sync(url: str, job_dir: Path, settings: Settings) -> Path:
    outtmpl = str(job_dir / "source.%(ext)s")
    ydl_opts = {
        "outtmpl": outtmpl,
        "format": "mp4/bestvideo+bestaudio/best",
        "merge_output_format": "mp4",
        "noplaylist": True,
        "max_filesize": settings.max_source_filesize_mb * 1024 * 1024,
        "socket_timeout": settings.download_timeout_seconds,
        "retries": 2,
        "quiet": True,
        "no_warnings": True,
        "restrictfilenames": True,
        "overwrites": True,
    }

    try:
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(url, download=True)
    except yt_dlp.utils.GeoRestrictedError as exc:
        raise AppError(ErrorCode.VIDEO_UNAVAILABLE, "Este video no está disponible en tu región.") from exc
    except yt_dlp.utils.DownloadError as exc:
        msg = str(exc).lower()
        if "private" in msg:
            raise AppError(ErrorCode.VIDEO_PRIVATE, "Este video es privado.") from exc
        if "unavailable" in msg or "removed" in msg or "404" in msg:
            raise AppError(ErrorCode.VIDEO_UNAVAILABLE, "Este video ya no está disponible.") from exc
        if "file is larger than max-filesize" in msg or "too large" in msg:
            raise AppError(ErrorCode.VIDEO_TOO_LARGE, "El video es demasiado grande.") from exc
        raise AppError(
            ErrorCode.EXTRACTOR_ERROR,
            "No se pudo descargar el video. TikTok pudo haber cambiado algo; probá de nuevo más tarde.",
            detail=str(exc),
        ) from exc
    except Exception as exc:  # noqa: BLE001 - last line of defense at the boundary
        raise AppError(ErrorCode.DOWNLOAD_FAILED, "Falló la descarga del video.", detail=str(exc)) from exc

    if info is None:
        raise AppError(ErrorCode.DOWNLOAD_FAILED, "Falló la descarga del video.")

    downloaded = sorted(job_dir.glob("source.*"), key=lambda p: p.stat().st_mtime, reverse=True)
    if not downloaded:
        raise AppError(ErrorCode.DOWNLOAD_FAILED, "El video se procesó pero no se encontró el archivo.")
    return downloaded[0]


async def download_video(url: str, job_dir: Path, settings: Settings) -> Path:
    """Runs the blocking yt-dlp download in a worker thread."""
    return await asyncio.to_thread(_run_download_sync, url, job_dir, settings)
