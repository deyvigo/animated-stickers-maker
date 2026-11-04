"""yt-dlp downloader service.

The Go API does everything except the download itself: yt-dlp is a Python
library with no Go equivalent, and it is the part that breaks whenever
TikTok changes something, so it lives in its own small service that can be
rebuilt (`docker compose build --no-cache downloader`) without touching the
rest.

Contract:  POST /download  {"job_id": "<32 hex>", "url": "https://..."}
  200  {"path": "<file>"}                       (file is in the shared volume)
  4xx/5xx  {"code": ..., "message": ..., "detail": ...}   (same codes as the API)

Only the job id travels over HTTP: the target directory is derived here, so
a caller can never make this service write outside the storage volume.
"""
from __future__ import annotations

import logging
import os
import re
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlparse

import yt_dlp
from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

logger = logging.getLogger("downloader")
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")

JOB_ID_RE = r"^[0-9a-f]{32}$"
DEFAULT_ALLOWED_HOSTS = (
    "tiktok.com", "www.tiktok.com", "m.tiktok.com", "vm.tiktok.com", "vt.tiktok.com",
)


@dataclass(frozen=True)
class Settings:
    storage_dir: Path
    max_source_filesize_mb: int
    socket_timeout_seconds: float
    allowed_hosts: tuple[str, ...]

    @classmethod
    def from_env(cls) -> "Settings":
        hosts = os.environ.get("ALLOWED_HOSTS", "")
        return cls(
            storage_dir=Path(os.environ.get("STORAGE_DIR", "/data/storage")),
            max_source_filesize_mb=int(os.environ.get("MAX_SOURCE_FILESIZE_MB", "80")),
            socket_timeout_seconds=float(os.environ.get("DOWNLOAD_TIMEOUT_SECONDS", "120")),
            allowed_hosts=tuple(h.strip().lower() for h in hosts.split(",") if h.strip())
            or DEFAULT_ALLOWED_HOSTS,
        )

    def job_dir(self, job_id: str) -> Path:
        return self.storage_dir / "jobs" / job_id


class DownloadRequest(BaseModel):
    job_id: str = Field(pattern=JOB_ID_RE)
    url: str


class DownloadError(Exception):
    def __init__(self, code: str, message: str, detail: str | None = None, status_code: int = 400):
        self.code, self.message, self.detail, self.status_code = code, message, detail, status_code
        super().__init__(message)


def assert_allowed_host(url: str, allowed_hosts: tuple[str, ...]) -> None:
    """Reject anything that isn't an allowed host *before* touching yt-dlp.

    This is the SSRF/abuse boundary: without it, this service is a generic
    "download any URL on the internet" endpoint. (The Go API checks too;
    this copy sits next to yt-dlp so it holds even if the API is bypassed.)
    """
    try:
        parsed = urlparse(url)
        host = (parsed.hostname or "").lower()
    except ValueError as exc:
        raise DownloadError("invalid_url", "El link no es válido.") from exc
    if not host or parsed.scheme not in ("http", "https"):
        raise DownloadError("invalid_url", "El link no es válido.")
    if not any(host == a or host.endswith("." + a) for a in allowed_hosts):
        raise DownloadError("host_not_allowed", "Solo se admiten links de TikTok.", detail=f"host={host}")


def download_video(url: str, job_dir: Path, settings: Settings) -> Path:
    """Blocking yt-dlp download into job_dir as source.<ext>."""
    job_dir.mkdir(parents=True, exist_ok=True)
    ydl_opts = {
        "outtmpl": str(job_dir / "source.%(ext)s"),
        "format": "mp4/bestvideo+bestaudio/best",
        "merge_output_format": "mp4",
        "noplaylist": True,
        "max_filesize": settings.max_source_filesize_mb * 1024 * 1024,
        "socket_timeout": settings.socket_timeout_seconds,
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
        raise DownloadError("video_unavailable", "Este video no está disponible en tu región.") from exc
    except yt_dlp.utils.DownloadError as exc:
        msg = str(exc).lower()
        if "private" in msg:
            raise DownloadError("video_private", "Este video es privado.") from exc
        if "unavailable" in msg or "removed" in msg or "404" in msg:
            raise DownloadError("video_unavailable", "Este video ya no está disponible.") from exc
        if "file is larger than max-filesize" in msg or "too large" in msg:
            raise DownloadError("video_too_large", "El video es demasiado grande.") from exc
        raise DownloadError(
            "extractor_error",
            "No se pudo descargar el video. TikTok pudo haber cambiado algo; probá de nuevo más tarde.",
            detail=str(exc),
        ) from exc
    except Exception as exc:  # noqa: BLE001 - last line of defense at the boundary
        raise DownloadError("download_failed", "Falló la descarga del video.", detail=str(exc),
                            status_code=500) from exc

    if info is None:
        raise DownloadError("download_failed", "Falló la descarga del video.", status_code=500)

    downloaded = sorted(
        (p for p in job_dir.glob("source.*") if p.suffix != ".tmp"),
        key=lambda p: p.stat().st_mtime,
        reverse=True,
    )
    if not downloaded:
        raise DownloadError("download_failed", "El video se procesó pero no se encontró el archivo.",
                            status_code=500)
    return downloaded[0]


def create_app(settings: Settings | None = None) -> FastAPI:
    app = FastAPI(title="Stickers downloader", version="0.1.0")
    cfg = settings or Settings.from_env()

    @app.exception_handler(DownloadError)
    async def _download_error(request: Request, exc: DownloadError) -> JSONResponse:
        logger.warning("download failed: %s (%s)", exc.message, exc.detail)
        return JSONResponse(
            status_code=exc.status_code,
            content={"code": exc.code, "message": exc.message, "detail": exc.detail},
        )

    @app.exception_handler(RequestValidationError)
    async def _validation_error(request: Request, exc: RequestValidationError) -> JSONResponse:
        return JSONResponse(
            status_code=422,
            content={"code": "invalid_request", "message": "Solicitud inválida.", "detail": str(exc.errors())},
        )

    @app.get("/health")
    def health() -> dict:
        return {"message": "ok"}

    # Plain `def` on purpose: FastAPI runs it in a worker thread, so the
    # blocking yt-dlp call doesn't stall the event loop. Concurrency is
    # bounded by the Go API's download semaphore.
    @app.post("/download")
    def download(body: DownloadRequest) -> dict:
        assert_allowed_host(body.url, cfg.allowed_hosts)
        path = download_video(body.url, cfg.job_dir(body.job_id), cfg)
        return {"path": str(path)}

    return app


app = create_app() if os.environ.get("DOWNLOADER_SKIP_APP") != "1" else None
