"""Renders a crop/time range of the source video into a WhatsApp-compliant
sticker: exactly 512x512 WebP, animated (<500KB) or static (<100KB).

This is the piece that makes or breaks sticker quality. 500KB for a 512x512
animated WebP is tight, so `fit_animated`/`fit_static` run ffmpeg repeatedly
along a fixed quality/fps ladder — never fuzzy binary search, always a
deterministic, bounded number of attempts — until the file is under budget
or we run out of ladder. The caller always gets back exactly which
parameters won, so the app can tell the user "se redujo a 10 fps".
"""
from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass
from pathlib import Path

from app.core.errors import AppError
from app.core.config import Settings
from app.models.schemas import CropRect, ErrorCode, RenderFitInfo
from app.services import probe

logger = logging.getLogger("stickers.webp")

# (fps, quality) ladder for animated renders, tried in order until the
# output fits animated_max_bytes. Ordered to prefer keeping fps first
# (motion smoothness reads more than a bit of compression noise), then
# giving up fps once quality is already low.
#
# For long clips (close to WhatsApp's 10s cap), 500KB just isn't enough
# bytes/frame to hold much quality at 15fps — crushing the quantizer to the
# floor (q=20) produces visible blocking on real footage. Trading more fps
# for less quantizer damage reads better: a slightly choppier loop looks
# more like "a sticker" than a blocky one. So once quality drops below ~35,
# give up fps further instead of continuing to crush the quantizer.
_ANIMATED_LADDER: list[tuple[int, int]] = [
    (15, 75),
    (15, 60),
    (15, 45),
    (12, 50),
    (12, 38),
    (10, 45),
    (10, 35),
    (8, 40),
    (8, 30),
    (6, 35),
    (6, 26),
    (5, 30),
    (5, 22),
]

# Quality ladder for static stickers (libwebp -q:v), against 100KB.
_STATIC_LADDER: list[int] = [90, 80, 70, 60, 50, 40, 30, 20]

# If even the bottom of the fps/quality ladder doesn't fit, shrink the clip
# duration by this factor and restart the ladder, down to a floor duration.
_DURATION_SHRINK_FACTOR = 0.7
_MIN_CLIP_SECONDS = 0.6
_MAX_DURATION_RETRIES = 4


@dataclass
class CropPixels:
    # May be negative, and x+width / y+height may exceed the source frame —
    # that's the point (see CropRect's docstring): the editor's zoom/pan UI
    # can legitimately ask for a crop bigger than or offset outside the
    # actual video, expecting the uncovered area to come back black.
    x: int
    y: int
    width: int
    height: int


def crop_to_pixels(crop: CropRect, source_width: int, source_height: int) -> CropPixels:
    """Converts a normalized crop rect to even pixel coordinates. Deliberately
    does NOT clamp to the source frame — see CropPixels' docstring. Even
    dimensions avoid odd-size chroma subsampling issues in the scale/pad
    filter chain."""

    def even(n: float) -> int:
        # Rounds toward an even integer. No floor here — x/y can legitimately
        # be negative (or zero), unlike width/height below.
        i = int(n)
        return i - (i % 2)

    def even_size(n: float) -> int:
        return max(2, even(n))

    x = even(crop.x * source_width)
    y = even(crop.y * source_height)
    w = even_size(crop.width * source_width)
    h = even_size(crop.height * source_height)
    return CropPixels(x=x, y=y, width=w, height=h)


def _crop_scale_pad_filter(
    crop_px: CropPixels, source_width: int, source_height: int, dimension: int, fps: int | None
) -> str:
    # A crop can extend past the source frame on any side (see CropPixels).
    # ffmpeg's crop filter can't read pixels that don't exist, so when that
    # happens we first pad the source out to a big-enough transparent canvas
    # and shift the crop origin to match, then crop from that canvas instead
    # of the raw source. When the crop is fully inside the frame (the common
    # case), all four margins are 0 and this pad is a no-op. Transparent
    # (not black) so the letterboxed area reads as a proper sticker
    # cutout — it shows through as whatever's behind it in the chat.
    left = max(0, -crop_px.x)
    top = max(0, -crop_px.y)
    right = max(0, (crop_px.x + crop_px.width) - source_width)
    bottom = max(0, (crop_px.y + crop_px.height) - source_height)

    parts = []
    crop_x, crop_y = crop_px.x, crop_px.y
    if left or top or right or bottom:
        padded_w = source_width + left + right
        padded_h = source_height + top + bottom
        # format=yuva420p forces an alpha plane onto the (opaque) decoded
        # source before pad runs — without it, pad's transparent color is
        # silently discarded and the letterboxed area comes back opaque
        # black regardless of what color we ask for.
        parts.append("format=yuva420p")
        parts.append(f"pad={padded_w}:{padded_h}:{left}:{top}:color=#00000000")
        crop_x, crop_y = crop_px.x + left, crop_px.y + top

    parts.append(f"crop={crop_px.width}:{crop_px.height}:{crop_x}:{crop_y}")
    if fps is not None:
        parts.append(f"fps={fps}")
    parts.append(
        f"scale={dimension}:{dimension}:force_original_aspect_ratio=decrease:flags=lanczos"
    )
    parts.append(
        f"pad={dimension}:{dimension}:(ow-iw)/2:(oh-ih)/2:color=#00000000"
    )
    return ",".join(parts)


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
        raise AppError(ErrorCode.TIMEOUT, "La generación del sticker tardó demasiado.") from None

    if proc.returncode != 0:
        raise AppError(
            ErrorCode.RENDER_FAILED,
            "No se pudo generar el sticker.",
            detail=stderr.decode(errors="ignore")[-800:],
        )


async def _render_animated_attempt(
    source: Path,
    out_path: Path,
    start: float,
    end: float,
    crop_px: CropPixels,
    source_width: int,
    source_height: int,
    fps: int,
    quality: int,
    dimension: int,
    timeout: float,
) -> None:
    vf = _crop_scale_pad_filter(crop_px, source_width, source_height, dimension, fps)
    await _run_ffmpeg(
        [
            "-ss", f"{start:.3f}",
            "-to", f"{end:.3f}",
            "-i", str(source),
            "-vf", vf,
            "-an",
            "-loop", "0",
            "-c:v", "libwebp_anim",
            "-lossless", "0",
            "-q:v", str(quality),
            # libwebp's compression_level trades encode speed for size, not
            # visual quality. 6 (max effort) measured >60s for a full 10s
            # clip in a constrained container — well past a single ladder
            # rung's timeout budget, and the ladder already has 10 rungs to
            # get through. 4 (the encoder's own default) is markedly
            # faster with only a marginal size cost.
            "-compression_level", "4",
            "-fps_mode", "passthrough",
            str(out_path),
        ],
        timeout,
    )


async def fit_animated(
    source: Path,
    out_path: Path,
    start: float,
    end: float,
    crop_px: CropPixels,
    source_width: int,
    source_height: int,
    settings: Settings,
) -> RenderFitInfo:
    duration = end - start
    duration = min(duration, settings.max_clip_seconds)
    degraded = False
    note_parts: list[str] = []

    for duration_attempt in range(_MAX_DURATION_RETRIES + 1):
        clip_end = start + duration
        for fps, quality in _ANIMATED_LADDER:
            await _render_animated_attempt(
                source, out_path, start, clip_end, crop_px, source_width, source_height,
                fps, quality, settings.sticker_dimension, settings.ffmpeg_timeout_seconds,
            )
            size = out_path.stat().st_size
            if size <= settings.animated_max_bytes:
                if (fps, quality) != _ANIMATED_LADDER[0] or duration_attempt > 0:
                    degraded = True
                if duration_attempt > 0:
                    note_parts.append(f"duración recortada a {duration:.1f}s")
                await _validate_output(out_path, settings, expect_animated=True)
                return RenderFitInfo(
                    fps=fps, quality=quality, duration=duration, degraded=degraded,
                    note="; ".join(note_parts) or None,
                )
        # Bottom of the quality/fps ladder still too big: shrink duration and retry.
        duration = max(_MIN_CLIP_SECONDS, duration * _DURATION_SHRINK_FACTOR)

    raise AppError(
        ErrorCode.RENDER_FAILED,
        "No se pudo comprimir el sticker animado dentro del límite de WhatsApp (500 KB). "
        "Probá con un recorte más corto o una zona con menos detalle.",
    )


async def fit_static(
    source: Path,
    out_path: Path,
    frame_at: float,
    crop_px: CropPixels,
    source_width: int,
    source_height: int,
    settings: Settings,
) -> RenderFitInfo:
    vf = _crop_scale_pad_filter(crop_px, source_width, source_height, settings.sticker_dimension, fps=None)
    for quality in _STATIC_LADDER:
        await _run_ffmpeg(
            [
                "-ss", f"{frame_at:.3f}",
                "-i", str(source),
                "-vf", vf,
                "-frames:v", "1",
                "-c:v", "libwebp",
                "-q:v", str(quality),
                str(out_path),
            ],
            settings.ffmpeg_timeout_seconds,
        )
        size = out_path.stat().st_size
        if size <= settings.static_max_bytes:
            degraded = quality != _STATIC_LADDER[0]
            await _validate_output(out_path, settings, expect_animated=False)
            return RenderFitInfo(quality=quality, degraded=degraded)

    raise AppError(
        ErrorCode.RENDER_FAILED,
        "No se pudo comprimir el sticker dentro del límite de WhatsApp (100 KB).",
    )


async def render_tray_icon(
    source: Path,
    out_path: Path,
    frame_at: float,
    crop_px: CropPixels,
    source_width: int,
    source_height: int,
    settings: Settings,
) -> None:
    """96x96 tray icon derived from the same frame/crop as the sticker.

    Always WebP (WhatsApp accepts PNG or WebP for the tray icon) so the
    output format matches `out_path`'s extension regardless of which rung
    of the quality ladder it takes to fit the budget.
    """
    vf = _crop_scale_pad_filter(crop_px, source_width, source_height, settings.tray_dimension, fps=None)
    for quality in _STATIC_LADDER:
        await _run_ffmpeg(
            [
                "-ss", f"{frame_at:.3f}",
                "-i", str(source),
                "-vf", vf,
                "-frames:v", "1",
                "-c:v", "libwebp",
                "-q:v", str(quality),
                str(out_path),
            ],
            settings.ffmpeg_timeout_seconds,
        )
        if out_path.stat().st_size <= settings.tray_max_bytes:
            return

    raise AppError(ErrorCode.RENDER_FAILED, "No se pudo generar el ícono del pack.")


async def _validate_output(path: Path, settings: Settings, *, expect_animated: bool) -> None:
    """Never hand back a file WhatsApp would reject — verify dimensions and
    duration with ffprobe rather than trusting the ffmpeg filter graph."""
    info = await probe.probe_output(path)
    if info["width"] != settings.sticker_dimension or info["height"] != settings.sticker_dimension:
        raise AppError(
            ErrorCode.RENDER_FAILED,
            "El sticker generado no quedó en 512x512.",
            detail=str(info),
        )
    # WhatsApp's own limit is "duration must not exceed 10000ms" (strictly
    # greater is invalid, per the official validator) — use > here, not >=,
    # so a render landing at exactly the cap isn't rejected.
    if expect_animated and info["duration"] > settings.max_clip_seconds:
        raise AppError(
            ErrorCode.RENDER_FAILED,
            "El sticker animado quedó más largo de lo permitido.",
            detail=str(info),
        )
