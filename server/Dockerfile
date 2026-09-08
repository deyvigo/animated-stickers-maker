# syntax=docker/dockerfile:1
FROM python:3.12-slim AS builder

RUN pip install --no-cache-dir uv

WORKDIR /build
COPY pyproject.toml ./
COPY app ./app

# Build a self-contained venv so the final stage doesn't need uv or a
# compiler toolchain.
RUN uv venv /venv && \
    uv pip install --python /venv/bin/python .

FROM python:3.12-slim

# ffmpeg is the actual workhorse here (crop/scale/pad/webp encode). When
# TikTok breaks yt-dlp's extractor, `docker compose build --no-cache` after
# bumping the yt-dlp version pin in pyproject.toml is the fix — yt-dlp
# ships frequent point releases for exactly this.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg && \
    rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1000 appuser
WORKDIR /app

COPY --from=builder /venv /venv
COPY app ./app

ENV PATH="/venv/bin:${PATH}" \
    PYTHONUNBUFFERED=1 \
    STORAGE_DIR=/data/storage

RUN mkdir -p /data/storage && chown -R appuser:appuser /data /app
USER appuser

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
