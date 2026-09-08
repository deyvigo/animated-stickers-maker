from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

from app.core.config import Settings


@pytest.fixture(scope="session")
def sample_video(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """A synthetic 20s portrait video, generated locally with ffmpeg's
    `testsrc2` source — no network access needed, and it compresses well
    enough that fitter tests stay fast."""
    out_dir = tmp_path_factory.mktemp("fixtures")
    out_path = out_dir / "sample.mp4"
    subprocess.run(
        [
            "ffmpeg", "-y",
            "-f", "lavfi", "-i", "testsrc2=size=720x1280:rate=30:duration=20",
            "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
            str(out_path),
        ],
        check=True,
        capture_output=True,
    )
    return out_path


@pytest.fixture
def settings(tmp_path: Path) -> Settings:
    return Settings(storage_dir=tmp_path)
