"""ffprobe wrappers: extract source video metadata, plus Pillow-based
validation of rendered WebP output.

Note on the Pillow dependency: the Debian-packaged ffmpeg here can *write*
animated WebP fine (`libwebp_anim` encoder), but its own `webp`
decoder/demuxer can't read the ANIM/ANMF chunks back (it only understands a
single still frame) — so `ffprobe` on our own animated stickers reports
width=0/height=0 and "image data not found", even though the file is a
perfectly valid animated WebP that WhatsApp and browsers open fine. Pillow
(built against libwebp) reads it correctly, so it's what we use to verify
our own output before calling a render "ready".
"""
from __future__ import annotations

import asyncio
import json
import logging
from pathlib import Path

from app.core.errors import AppError
from app.models.schemas import ErrorCode, JobMetadata

logger = logging.getLogger("stickers.probe")


async def _ffprobe_json(path: Path, timeout: float) -> dict:
    proc = await asyncio.create_subprocess_exec(
        "ffprobe",
        "-v", "error",
        "-print_format", "json",
        "-show_format",
        "-show_streams",
        str(path),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        raise AppError(ErrorCode.TIMEOUT, "El análisis del video tardó demasiado.") from None

    if proc.returncode != 0:
        raise AppError(
            ErrorCode.EXTRACTOR_ERROR,
            "No se pudo leer el video descargado.",
            detail=stderr.decode(errors="ignore")[:500],
        )
    return json.loads(stdout)


def _parse_fps(rate: str) -> float:
    if "/" in rate:
        num, den = rate.split("/")
        den = float(den)
        return float(num) / den if den else 0.0
    return float(rate)


async def probe_video(path: Path, timeout: float = 30.0) -> JobMetadata:
    data = await _ffprobe_json(path, timeout)
    video_streams = [s for s in data.get("streams", []) if s.get("codec_type") == "video"]
    if not video_streams:
        raise AppError(ErrorCode.EXTRACTOR_ERROR, "El archivo descargado no tiene una pista de video.")
    stream = video_streams[0]
    duration = float(data.get("format", {}).get("duration") or stream.get("duration") or 0.0)
    fps = _parse_fps(stream.get("avg_frame_rate") or stream.get("r_frame_rate") or "0/1")
    return JobMetadata(
        duration=duration,
        width=int(stream["width"]),
        height=int(stream["height"]),
        fps=fps,
    )


def _inspect_webp_sync(path: Path) -> dict:
    # Pillow's high-level WebP plugin (12.3.0 as tested) doesn't surface
    # per-frame `duration` in `Image.info` for files produced by ffmpeg's
    # libwebp_anim encoder in this environment — it silently reads back 0,
    # even though the frames' ANMF timestamps are correct (verified against
    # the raw RIFF bytes). The private `PIL._webp.WebPAnimDecoder` reads
    # real per-frame timestamps and works for both static and animated
    # WebP, so we use it directly instead of the public plugin API.
    from PIL import _webp

    try:
        data = path.read_bytes()
        (width, height), _loop_count, _bgcolor, frame_count, _mode = _webp.WebPAnimDecoder(data).get_info()
        last_timestamp_ms = 0
        if frame_count > 1:
            decoder = _webp.WebPAnimDecoder(data)
            for _ in range(frame_count):
                _pixels, last_timestamp_ms = decoder.get_next()
        return {
            "width": width,
            "height": height,
            "duration": last_timestamp_ms / 1000.0,
            "frames": frame_count,
        }
    except Exception as exc:  # noqa: BLE001 - any decode failure means an invalid sticker
        raise AppError(
            ErrorCode.RENDER_FAILED, "El sticker generado quedó inválido.", detail=str(exc)
        ) from exc


async def probe_output(path: Path) -> dict:
    """Validates a rendered WebP sticker before marking it ready. See the
    module docstring for why this uses Pillow instead of ffprobe."""
    return await asyncio.to_thread(_inspect_webp_sync, path)
