from __future__ import annotations

from pathlib import Path

import pytest

from app.core.config import Settings
from app.core.errors import AppError
from app.models.schemas import CropRect
from app.services import probe
from app.services.webp import CropPixels, crop_to_pixels, fit_animated, fit_static


def test_crop_to_pixels_is_even_and_in_bounds() -> None:
    crop = CropRect(x=0.1, y=0.2, width=0.5, height=0.5)
    px = crop_to_pixels(crop, source_width=721, source_height=1281)
    assert px.width % 2 == 0
    assert px.height % 2 == 0
    assert px.x + px.width <= 721
    assert px.y + px.height <= 1281


def test_crop_to_pixels_does_not_clamp_out_of_bounds() -> None:
    # The editor's zoom/pan UI can legitimately ask for a crop that extends
    # outside the source frame (e.g. "whole portrait video visible" inside a
    # square viewport) — crop_to_pixels must pass that through unclamped;
    # _crop_scale_pad_filter is what turns the overflow into black padding.
    crop = CropRect(x=-0.4, y=0.0, width=1.8, height=1.0)
    px = crop_to_pixels(crop, source_width=1000, source_height=1000)
    assert px.x < 0
    assert px.x + px.width > 1000


def _square_crop(width: int, height: int) -> CropPixels:
    side = min(width, height)
    return CropPixels(x=(width - side) // 2, y=0, width=side, height=side)


@pytest.mark.asyncio
async def test_fit_animated_meets_whatsapp_budget(sample_video: Path, settings: Settings, tmp_path: Path) -> None:
    crop_px = _square_crop(720, 1280)
    out_path = tmp_path / "sticker.webp"

    fit = await fit_animated(
        sample_video, out_path, start=0.0, end=2.0, crop_px=crop_px,
        source_width=720, source_height=1280, settings=settings,
    )

    assert out_path.stat().st_size <= settings.animated_max_bytes
    info = await probe.probe_output(out_path)
    assert info["width"] == settings.sticker_dimension
    assert info["height"] == settings.sticker_dimension
    assert fit.duration == pytest.approx(2.0, abs=0.05)


@pytest.mark.asyncio
async def test_fit_animated_clamps_duration_to_whatsapp_max(
    sample_video: Path, settings: Settings, tmp_path: Path
) -> None:
    """A range longer than WhatsApp's 10s cap must be clamped, never passed
    straight through to ffmpeg."""
    crop_px = _square_crop(720, 1280)
    out_path = tmp_path / "sticker.webp"

    fit = await fit_animated(
        sample_video, out_path, start=0.0, end=18.0, crop_px=crop_px,
        source_width=720, source_height=1280, settings=settings,
    )

    assert fit.duration <= settings.max_clip_seconds
    info = await probe.probe_output(out_path)
    assert info["duration"] < settings.max_clip_seconds


@pytest.mark.asyncio
async def test_fit_static_meets_whatsapp_budget(sample_video: Path, settings: Settings, tmp_path: Path) -> None:
    crop_px = _square_crop(720, 1280)
    out_path = tmp_path / "sticker.webp"

    await fit_static(
        sample_video, out_path, frame_at=1.0, crop_px=crop_px,
        source_width=720, source_height=1280, settings=settings,
    )

    assert out_path.stat().st_size <= settings.static_max_bytes
    info = await probe.probe_output(out_path)
    assert info["width"] == settings.sticker_dimension
    assert info["height"] == settings.sticker_dimension
    assert info["frames"] == 1


@pytest.mark.asyncio
async def test_fit_animated_raises_render_failed_on_impossible_budget(
    sample_video: Path, settings: Settings, tmp_path: Path
) -> None:
    """An absurdly small budget must surface a translated RENDER_FAILED
    instead of silently handing back an oversized/invalid sticker."""
    settings.animated_max_bytes = 10  # impossible to hit
    crop_px = _square_crop(720, 1280)
    out_path = tmp_path / "sticker.webp"

    with pytest.raises(AppError) as exc_info:
        await fit_animated(
            sample_video, out_path, start=0.0, end=1.0, crop_px=crop_px,
            source_width=720, source_height=1280, settings=settings,
        )
    assert exc_info.value.code.value == "render_failed"


@pytest.mark.asyncio
async def test_fit_static_pads_out_of_bounds_crop_with_transparency(
    sample_video: Path, settings: Settings, tmp_path: Path
) -> None:
    """The editor's 'whole video visible, letterboxed' default sends a crop
    wider than the source (portrait video contained in a square) — this must
    render as a valid 512x512 sticker with *transparent* bars (a proper
    sticker cutout, not an opaque black box), not fail or get silently
    clamped back to a smaller crop."""
    # Source is 720x1280 (portrait). A crop as wide as the height, centered,
    # overflows left/right — exactly the "fit portrait video into a square"
    # case from the editor's zoom/pan UI.
    side = 1280
    crop_px = CropPixels(x=(720 - side) // 2, y=0, width=side, height=side)
    out_path = tmp_path / "sticker.webp"

    await fit_static(
        sample_video, out_path, frame_at=1.0, crop_px=crop_px,
        source_width=720, source_height=1280, settings=settings,
    )

    info = await probe.probe_output(out_path)
    assert info["width"] == settings.sticker_dimension
    assert info["height"] == settings.sticker_dimension

    from PIL import Image

    with Image.open(out_path) as img:
        img = img.convert("RGBA")
        # Near the left edge, well inside the overflowed (padded) margin.
        assert img.getpixel((2, settings.sticker_dimension // 2))[3] == 0
