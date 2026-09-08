"""Generates the editor preview assets: a small H.264 proxy for smooth
scrubbing, and a sprite sheet of thumbnails for the trim timeline.
"""
from __future__ import annotations

import asyncio
import logging
import math
from pathlib import Path

from app.core.errors import AppError
from app.models.schemas import ErrorCode

logger = logging.getLogger("stickers.preview")

PROXY_HEIGHT = 480
SPRITE_COLUMNS = 8
SPRITE_TILE_WIDTH = 120


async def _run_ffmpeg(args: list[str], timeout: float) -> None:
    proc = await asyncio.create_subprocess_exec(
        "ffmpeg", "-y", *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        _, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        raise AppError(ErrorCode.TIMEOUT, "La generación de la vista previa tardó demasiado.") from None

    if proc.returncode != 0:
        raise AppError(
            ErrorCode.RENDER_FAILED,
            "No se pudo generar la vista previa del video.",
            detail=stderr.decode(errors="ignore")[-800:],
        )


async def generate_proxy(source: Path, out_path: Path, timeout: float) -> None:
    await _run_ffmpeg(
        [
            "-i", str(source),
            "-vf", f"scale=-2:{PROXY_HEIGHT}",
            "-c:v", "libx264", "-preset", "veryfast", "-crf", "28",
            "-c:a", "aac", "-b:a", "96k",
            "-movflags", "+faststart",
            str(out_path),
        ],
        timeout,
    )


async def generate_sprite(source: Path, out_path: Path, duration: float, timeout: float, frame_count: int = 32) -> None:
    """One JPEG grid of evenly-spaced thumbnails, used as the timeline
    background so scrubbing doesn't need to re-decode the source video."""
    if duration <= 0:
        raise AppError(ErrorCode.EXTRACTOR_ERROR, "El video no tiene una duración válida.")
    fps = frame_count / duration
    rows = math.ceil(frame_count / SPRITE_COLUMNS)
    await _run_ffmpeg(
        [
            "-i", str(source),
            "-vf",
            f"fps={fps:.6f},scale={SPRITE_TILE_WIDTH}:-1,tile={SPRITE_COLUMNS}x{rows}",
            "-frames:v", "1",
            "-q:v", "4",
            str(out_path),
        ],
        timeout,
    )
