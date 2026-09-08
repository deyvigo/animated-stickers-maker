from __future__ import annotations

import pytest
import yt_dlp

from app.core.config import Settings
from app.core.errors import AppError
from app.models.schemas import ErrorCode
from app.services import download


def test_allowed_hosts_accept_tiktok(settings: Settings) -> None:
    for url in [
        "https://www.tiktok.com/@user/video/123",
        "https://vm.tiktok.com/abc123",
        "https://vt.tiktok.com/abc123",
        "https://m.tiktok.com/v/123.html",
    ]:
        download.assert_allowed_host(url, settings)  # must not raise


@pytest.mark.parametrize(
    "url",
    [
        "https://youtube.com/watch?v=abc",
        "https://evil.com/tiktok.com",
        "https://tiktok.com.evil.com/video",
        "ftp://tiktok.com/video",
        "not a url",
    ],
)
def test_disallowed_hosts_rejected(settings: Settings, url: str) -> None:
    with pytest.raises(AppError) as exc_info:
        download.assert_allowed_host(url, settings)
    assert exc_info.value.code in (ErrorCode.HOST_NOT_ALLOWED, ErrorCode.INVALID_URL)


@pytest.mark.asyncio
async def test_download_video_maps_private_error(tmp_path, settings, monkeypatch) -> None:
    def _boom(self, url, download=True):  # noqa: ARG001
        raise yt_dlp.utils.DownloadError("ERROR: [TikTok] 123: Video is private")

    monkeypatch.setattr(yt_dlp.YoutubeDL, "extract_info", _boom)

    with pytest.raises(AppError) as exc_info:
        await download.download_video("https://www.tiktok.com/@x/video/1", tmp_path, settings)
    assert exc_info.value.code == ErrorCode.VIDEO_PRIVATE


@pytest.mark.asyncio
async def test_download_video_maps_unavailable_error(tmp_path, settings, monkeypatch) -> None:
    def _boom(self, url, download=True):  # noqa: ARG001
        raise yt_dlp.utils.DownloadError("ERROR: [TikTok] 123: Video unavailable")

    monkeypatch.setattr(yt_dlp.YoutubeDL, "extract_info", _boom)

    with pytest.raises(AppError) as exc_info:
        await download.download_video("https://www.tiktok.com/@x/video/1", tmp_path, settings)
    assert exc_info.value.code == ErrorCode.VIDEO_UNAVAILABLE


@pytest.mark.asyncio
async def test_download_video_maps_unknown_error_to_extractor_error(tmp_path, settings, monkeypatch) -> None:
    def _boom(self, url, download=True):  # noqa: ARG001
        raise yt_dlp.utils.DownloadError("ERROR: some brand new TikTok layout broke the extractor")

    monkeypatch.setattr(yt_dlp.YoutubeDL, "extract_info", _boom)

    with pytest.raises(AppError) as exc_info:
        await download.download_video("https://www.tiktok.com/@x/video/1", tmp_path, settings)
    assert exc_info.value.code == ErrorCode.EXTRACTOR_ERROR
