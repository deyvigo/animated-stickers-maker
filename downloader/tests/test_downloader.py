from __future__ import annotations

from pathlib import Path

import pytest
import yt_dlp
from fastapi.testclient import TestClient

from app import main
from app.main import Settings, create_app

JOB = "a" * 32
URL = "https://www.tiktok.com/@x/video/1"


@pytest.fixture
def settings(tmp_path: Path) -> Settings:
    return Settings(
        storage_dir=tmp_path,
        max_source_filesize_mb=80,
        socket_timeout_seconds=5,
        allowed_hosts=main.DEFAULT_ALLOWED_HOSTS,
    )


@pytest.fixture
def client(settings: Settings) -> TestClient:
    return TestClient(create_app(settings))


def _fail_with(monkeypatch: pytest.MonkeyPatch, exc: Exception) -> None:
    def _boom(self, url, download=True):  # noqa: ARG001
        raise exc

    monkeypatch.setattr(yt_dlp.YoutubeDL, "extract_info", _boom)


@pytest.mark.parametrize("url", [
    "https://www.tiktok.com/@user/video/123",
    "https://vm.tiktok.com/abc123",
    "https://vt.tiktok.com/abc123",
    "https://m.tiktok.com/v/123.html",
])
def test_allowed_hosts_accept_tiktok(url: str) -> None:
    main.assert_allowed_host(url, main.DEFAULT_ALLOWED_HOSTS)  # must not raise


@pytest.mark.parametrize("url", [
    "https://youtube.com/watch?v=abc",
    "https://evil.com/tiktok.com",
    "https://tiktok.com.evil.com/video",
    "https://tiktok.com@evil.com/video",
    "ftp://tiktok.com/video",
    "not a url",
])
def test_disallowed_hosts_rejected(url: str) -> None:
    with pytest.raises(main.DownloadError) as exc:
        main.assert_allowed_host(url, main.DEFAULT_ALLOWED_HOSTS)
    assert exc.value.code in ("host_not_allowed", "invalid_url")


@pytest.mark.parametrize("message,code", [
    ("ERROR: [TikTok] 123: Video is private", "video_private"),
    ("ERROR: [TikTok] 123: Video unavailable", "video_unavailable"),
    ("ERROR: File is larger than max-filesize", "video_too_large"),
    ("ERROR: some brand new TikTok layout broke the extractor", "extractor_error"),
])
def test_error_mapping_over_http(client, monkeypatch, message, code) -> None:
    _fail_with(monkeypatch, yt_dlp.utils.DownloadError(message))
    r = client.post("/download", json={"job_id": JOB, "url": URL})
    assert r.status_code >= 400
    assert r.json()["code"] == code


def test_geo_restriction_maps_to_unavailable(client, monkeypatch) -> None:
    _fail_with(monkeypatch, yt_dlp.utils.GeoRestrictedError("geo"))
    assert client.post("/download", json={"job_id": JOB, "url": URL}).json()["code"] == "video_unavailable"


def test_unexpected_exception_maps_to_download_failed(client, monkeypatch) -> None:
    _fail_with(monkeypatch, RuntimeError("kaboom"))
    r = client.post("/download", json={"job_id": JOB, "url": URL})
    assert r.status_code == 500 and r.json()["code"] == "download_failed"


def test_success_returns_path_inside_job_dir(client, settings, monkeypatch) -> None:
    def _fake(self, url, download=True):  # noqa: ARG001
        tmpl = self.params["outtmpl"]
        tmpl = tmpl["default"] if isinstance(tmpl, dict) else tmpl  # newer yt-dlp wraps it
        out = Path(tmpl.replace("%(ext)s", "mp4"))
        out.write_bytes(b"video")
        return {"id": "1"}

    monkeypatch.setattr(yt_dlp.YoutubeDL, "extract_info", _fake)
    r = client.post("/download", json={"job_id": JOB, "url": URL})
    assert r.status_code == 200
    assert Path(r.json()["path"]) == settings.job_dir(JOB) / "source.mp4"


def test_rejects_foreign_host_without_calling_ytdlp(client, monkeypatch) -> None:
    called = []
    monkeypatch.setattr(yt_dlp.YoutubeDL, "extract_info", lambda *a, **k: called.append(1))
    r = client.post("/download", json={"job_id": JOB, "url": "https://youtube.com/watch?v=1"})
    assert r.status_code == 400 and r.json()["code"] == "host_not_allowed"
    assert not called


@pytest.mark.parametrize("job_id", ["../../etc", "short", "A" * 32, "a" * 33])
def test_job_id_must_be_32_hex(client, job_id) -> None:
    r = client.post("/download", json={"job_id": job_id, "url": URL})
    assert r.status_code == 422 and r.json()["code"] == "invalid_request"
