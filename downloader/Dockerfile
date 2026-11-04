# syntax=docker/dockerfile:1
FROM python:3.12-slim AS builder
RUN pip install --no-cache-dir uv
WORKDIR /build
COPY pyproject.toml ./
COPY app ./app
RUN uv venv /venv && uv pip install --python /venv/bin/python .

FROM python:3.12-slim

# ffmpeg is only needed by yt-dlp to merge audio+video streams. When TikTok
# breaks the extractor, `docker compose build --no-cache downloader` pulls
# the latest yt-dlp release — that is the whole fix.
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
